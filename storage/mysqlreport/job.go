// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package mysqlreport

import (
	"context"
	"database/sql"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

const (
	// aggLockName is the MySQL named lock guarding the aggregate job across
	// replicated api instances sharing one MySQL.
	aggLockName = "report_agg_job"
	// partitionLockName guards the partition management cycle.
	partitionLockName = "report_partition_job"

	// defaultPartitionAheadDays is how many daily partitions are kept ahead
	// of the current day (data arriving before a partition exists is
	// rejected with Error 1526).
	defaultPartitionAheadDays = 3
	// defaultPartitionCheckInterval is the partition-management period.
	defaultPartitionCheckInterval = 6 * time.Hour
	// defaultDeleteBatchSize is the batch size of the DELETE fallback used
	// for non-partitioned tables.
	defaultDeleteBatchSize = 1000
)

// Job runs the background maintenance of the MySQL report database:
// the minute aggregate job (detail -> bfe_ai_metrics_1m, DELETE window +
// INSERT SELECT in one transaction, replay-idempotent) and the partition
// management job (rolling daily RANGE partitions, DELETE fallback for
// non-partitioned tables). It only runs for Backend = "mysql".
type Job struct {
	db       *sql.DB
	database string

	aggregateInterval      time.Duration
	retentionDays          int
	partitionAheadDays     int
	partitionCheckInterval time.Duration
	deleteBatchSize        int

	// nowFunc is injectable for tests (see model/quota Clock).
	nowFunc func() time.Time

	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	// partitionConn pins the named-lock connection for the duration of a
	// partition cycle (GET_LOCK is per-connection). It is only touched by
	// the single partition loop goroutine (or by direct test calls).
	partitionConn *sql.Conn
}

// JobOption customizes a Job.
type JobOption func(*Job)

func (j *Job) table(name string) string {
	if j.database == "" {
		return name
	}
	return j.database + "." + name
}

// WithNowFunc injects the clock used to compute aggregation windows.
func WithNowFunc(f func() time.Time) JobOption {
	return func(j *Job) {
		if f != nil {
			j.nowFunc = f
		}
	}
}

// WithPartitionCheckInterval overrides the partition-management period.
func WithPartitionCheckInterval(d time.Duration) JobOption {
	return func(j *Job) {
		if d > 0 {
			j.partitionCheckInterval = d
		}
	}
}

// NewJob creates a Job. aggregateInterval <= 0 disables the aggregate
// loop; retentionDays <= 0 disables the retention (drop/purge) behavior of
// the partition management loop.
func NewJob(db *sql.DB, database string, aggregateInterval time.Duration, retentionDays int, opts ...JobOption) *Job {
	j := &Job{
		db:                     db,
		database:               database,
		aggregateInterval:      aggregateInterval,
		retentionDays:          retentionDays,
		partitionAheadDays:     defaultPartitionAheadDays,
		partitionCheckInterval: defaultPartitionCheckInterval,
		deleteBatchSize:        defaultDeleteBatchSize,
		nowFunc:                time.Now,
		stopCh:                 make(chan struct{}),
	}
	for _, opt := range opts {
		opt(j)
	}
	return j
}

// Start launches the maintenance loops. Both loops run one cycle
// immediately (the aggregate job backfills the previous full minute and
// the partition job creates missing partitions up front), then tick with
// their configured intervals. It follows the Start/Stop convention of
// quota.QuotaResetScheduler.
func (j *Job) Start() {
	j.startOnce.Do(func() {
		if j.aggregateInterval > 0 {
			j.wg.Add(1)
			go j.aggregateLoop()
		}
		j.wg.Add(1)
		go j.partitionLoop()
	})
}

// Stop stops the maintenance loops and waits for them to return.
func (j *Job) Stop() {
	j.stopOnce.Do(func() {
		close(j.stopCh)
	})
	j.wg.Wait()
}

func (j *Job) aggregateLoop() {
	defer j.wg.Done()
	defer recoverPanic("report aggregate job")

	j.runAggregateCycle()

	ticker := time.NewTicker(j.aggregateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			j.runAggregateCycle()
		case <-j.stopCh:
			return
		}
	}
}

func (j *Job) partitionLoop() {
	defer j.wg.Done()
	defer recoverPanic("report partition job")

	j.runPartitionCycle()

	ticker := time.NewTicker(j.partitionCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			j.runPartitionCycle()
		case <-j.stopCh:
			return
		}
	}
}

func recoverPanic(name string) {
	if err := recover(); err != nil {
		stack := make([]byte, 1024*8)
		stack = stack[:runtime.Stack(stack, false)]
		jobErrorf("PANIC in %s: err=%v\n%s", name, err, string(stack))
	}
}

func (j *Job) runAggregateCycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := j.RunAggregateOnce(ctx); err != nil {
		jobErrorf("report aggregate cycle failed: %v", err)
	}
}

func (j *Job) runPartitionCycle() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := j.RunPartitionMgmtOnce(ctx); err != nil {
		jobErrorf("report partition cycle failed: %v", err)
	}
}

// jobErrorf / jobWarnf tolerate the nil loggers of unit tests; production
// always initializes them via stateful.InitLog.
func jobErrorf(format string, args ...interface{}) {
	if stateful.ExceptionLogger != nil {
		stateful.ExceptionLogger.Error(format, args...)
	}
}

func jobWarnf(format string, args ...interface{}) {
	if stateful.AccessLogger != nil {
		stateful.AccessLogger.Warn(format, args...)
	}
}

// RunAggregateOnce aggregates the previous full minute [minuteStart-1m,
// minuteStart) into bfe_ai_metrics_1m. The DELETE+INSERT SELECT runs in a
// single transaction so replaying the same window is idempotent; multiple
// api replicas serialize on the MySQL named lock and losers skip the
// cycle.
func (j *Job) RunAggregateOnce(ctx context.Context) error {
	minuteStart := j.nowFunc().Truncate(time.Minute)
	windowStart := minuteStart.Add(-time.Minute)
	windowEnd := minuteStart

	deleteSQL := buildAggregateDeleteSQL(j.table(tableMetrics))
	insertSQL := buildAggregateInsertSQL(j.table(tableDetail), j.table(tableMetrics))

	conn, err := j.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	locked, err := acquireNamedLock(ctx, conn, aggLockName)
	if err != nil {
		return err
	}
	if !locked {
		jobWarnf("report aggregate job: lock %s not acquired, skip cycle", aggLockName)
		return nil
	}
	defer releaseNamedLock(conn, aggLockName)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, deleteSQL, windowStart); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, insertSQL, windowStart, windowEnd); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// RunPartitionMgmtOnce keeps the daily RANGE partitions of both report
// tables: it creates the partitions for the next partitionAheadDays days,
// drops the partitions older than retentionDays and, when a table is not
// partitioned, falls back to batched DELETE purges.
func (j *Job) RunPartitionMgmtOnce(ctx context.Context) error {
	locked, err := j.lockForPartition(ctx)
	if err != nil {
		return err
	}
	if !locked {
		jobWarnf("report partition job: lock %s not acquired, skip cycle", partitionLockName)
		return nil
	}
	defer j.unlockForPartition()

	now := j.nowFunc()
	// Metadata queries take the BARE table name: information_schema stores
	// schema and table name in separate columns, so a schema-qualified name
	// would never match and the table would silently degrade to the DELETE
	// purge (issue #191). DDL statements qualify via j.table() so they keep
	// working when j.database overrides the connection schema.
	for _, target := range []struct {
		table   string
		timeCol string
	}{
		{tableDetail, "log_time"},
		{tableMetrics, "ts_min"},
	} {
		if err := j.manageTablePartitions(ctx, j.partitionConn, target.table, target.timeCol, now); err != nil {
			return err
		}
	}
	return nil
}

func (j *Job) lockForPartition(ctx context.Context) (bool, error) {
	conn, err := j.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	locked, err := acquireNamedLock(ctx, conn, partitionLockName)
	if err != nil || !locked {
		conn.Close()
		return false, err
	}
	j.partitionConn = conn
	return true, nil
}

func (j *Job) unlockForPartition() {
	if j.partitionConn != nil {
		releaseNamedLock(j.partitionConn, partitionLockName)
		j.partitionConn.Close()
		j.partitionConn = nil
	}
}

// manageTablePartitions reconciles one table: create-ahead, drop-expired,
// or purge in batches when the table has no partitions. table is the BARE
// table name; DDL statements are qualified with j.table() so they target
// j.database even when it differs from the connection schema.
func (j *Job) manageTablePartitions(ctx context.Context, conn *sql.Conn, table, timeCol string, now time.Time) error {
	parts, err := j.listPartitions(ctx, conn, table)
	if err != nil {
		return err
	}

	if len(parts) == 0 {
		// Non-partitioned shape (some RDS offerings): degrade to batched
		// DELETE. Skip when retention is disabled. Warn loudly: reaching
		// this branch means either a genuinely unpartitioned table or a
		// metadata mismatch, and both used to be silent (issue #191).
		if j.retentionDays <= 0 {
			return nil
		}
		jobWarnf("report partition job: table %s has no partitions, fallback to batched DELETE purge", j.table(table))
		return j.purgeBatches(ctx, conn, j.table(table), timeCol, retentionCutoff(now, j.retentionDays))
	}

	for _, day := range planPartitionAdds(now, j.partitionAheadDays, partitionBoundaries(parts)) {
		if _, err := conn.ExecContext(ctx, buildAddPartitionSQL(j.table(table), day)); err != nil {
			return err
		}
	}

	if j.retentionDays > 0 {
		for _, name := range planPartitionDrops(now, j.retentionDays, parts) {
			if _, err := conn.ExecContext(ctx, buildDropPartitionSQL(j.table(table), name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// partitionInfo is one row of information_schema.PARTITIONS.
type partitionInfo struct {
	Name        string
	Description string
}

var listPartitionsSQL = "SELECT PARTITION_NAME, PARTITION_DESCRIPTION" +
	" FROM information_schema.PARTITIONS" +
	" WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND PARTITION_NAME IS NOT NULL" +
	" ORDER BY PARTITION_ORDINAL_POSITION"

// listPartitionsInSchemaSQL pins TABLE_SCHEMA to j.database. It is a
// separate query (rather than routing DATABASE() through COALESCE) so both
// shapes stay plain, index-friendly equality predicates.
var listPartitionsInSchemaSQL = "SELECT PARTITION_NAME, PARTITION_DESCRIPTION" +
	" FROM information_schema.PARTITIONS" +
	" WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND PARTITION_NAME IS NOT NULL" +
	" ORDER BY PARTITION_ORDINAL_POSITION"

// listPartitions reads the RANGE partitions of one table; table must be the
// BARE table name — information_schema.PARTITIONS never carries schema
// qualifiers in TABLE_NAME, so a qualified name returns zero rows and the
// caller would mistake a partitioned table for an unpartitioned one
// (issue #191). The schema filter is j.database, falling back to the
// connection's current database when empty.
func (j *Job) listPartitions(ctx context.Context, conn *sql.Conn, table string) ([]partitionInfo, error) {
	query, args := listPartitionsSQL, []interface{}{table}
	if j.database != "" {
		query, args = listPartitionsInSchemaSQL, []interface{}{j.database, table}
	}
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	parts := make([]partitionInfo, 0, 16)
	for rows.Next() {
		var p partitionInfo
		if err := rows.Scan(&p.Name, &p.Description); err != nil {
			return nil, err
		}
		parts = append(parts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return parts, nil
}

// purgeBatches deletes rows older than cutoff in batches until a batch
// affects no row (non-partitioned fallback).
func (j *Job) purgeBatches(ctx context.Context, conn *sql.Conn, table, timeCol string, cutoff time.Time) error {
	query := buildPurgeSQL(table, timeCol, j.deleteBatchSize)
	for {
		res, err := conn.ExecContext(ctx, query, cutoff)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
	}
}

func acquireNamedLock(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
	var res sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&res); err != nil {
		return false, err
	}
	return res.Valid && res.Int64 == 1, nil
}

func releaseNamedLock(conn *sql.Conn, name string) {
	if _, err := conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", name); err != nil {
		jobErrorf("release lock %s failed: %v", name, err)
	}
}

// buildAggregateDeleteSQL deletes one minute window from the aggregate
// table so a replayed window produces identical rows.
func buildAggregateDeleteSQL(metricsTable string) string {
	return "DELETE FROM " + metricsTable + " WHERE ts_min = ?"
}

// aggregateInsertColumns is the explicit column list of bfe_ai_metrics_1m
// (40 dimensions + 24 metrics, same order as db_ddl_report_mysql.sql).
var aggregateInsertColumns = " (ts_min," +
	"hostid,ai_apikey_id,ai_requested_model,ai_target_model,ai_stream," +
	"product,cluster,sub_cluster,backend_info,method,res_status_code,err_code,header_host," +
	"ai_provider,ai_protocol,ai_mode,ai_cost_currency," +
	"level1Name,level1,level2Name,level2,level3Name,level3,level4Name,level4,level5Name,level5," +
	"rate_limit_policy_id,rate_limit_type,rate_limit_rule_name,ai_auth_reject_reason," +
	"ai_auth_reject_quota_plans_slot1,ai_auth_reject_quota_plans_slot2,ai_auth_reject_quota_plans_slot3," +
	"ai_auth_reject_quota_plans_slot4,ai_auth_reject_quota_plans_slot5," +
	"ai_cache_status,mirror_hit,ai_intent_answer," +
	"request_count,error_count,auth_reject_count,input_tokens,output_tokens,total_tokens," +
	"ttft_us_sum,tpot_us_sum,req_header_bytes,req_body_bytes,res_header_bytes,res_body_bytes," +
	"rate_limit_hits,backend_retries,all_time_sum,cluster_serve_sum,backend_serve_sum," +
	"ai_retry_count_sum,ai_cost_value_sum,cache_read_tokens,cache_write_tokens," +
	"ai_audio_input_tokens,ai_audio_output_tokens,ai_image_count)"

// aggregateGroupBy lists the 40 dimension aliases (ts_min + 39) of the
// aggregate insert, mirroring the Doris bfe_ai_metrics_1m_job.sql.
const aggregateGroupBy = " GROUP BY ts_min," +
	"hostid,ai_apikey_id,ai_requested_model,ai_target_model,ai_stream," +
	"product,cluster,sub_cluster,backend_info,method,res_status_code,err_code,header_host," +
	"ai_provider,ai_protocol,ai_mode,ai_cost_currency," +
	"level1Name,level1,level2Name,level2,level3Name,level3,level4Name,level4,level5Name,level5," +
	"rate_limit_policy_id,rate_limit_type,rate_limit_rule_name,ai_auth_reject_reason," +
	"ai_auth_reject_quota_plans_slot1,ai_auth_reject_quota_plans_slot2,ai_auth_reject_quota_plans_slot3," +
	"ai_auth_reject_quota_plans_slot4,ai_auth_reject_quota_plans_slot5," +
	"ai_cache_status,mirror_hit,ai_intent_answer"

// buildAggregateInsertSQL builds the INSERT SELECT aggregating one minute
// window of the detail table into the aggregate table. Dimension columns
// are normalized with IFNULL(col,”) (aligning with the Doris JOB's
// COALESCE); the rate-limit triple and the auth-reject quota-plan slots
// are flattened out of the JSON columns with JSON_EXTRACT.
func buildAggregateInsertSQL(detailTable, metricsTable string) string {
	return "INSERT INTO " + metricsTable + aggregateInsertColumns +
		" SELECT" +
		" DATE_FORMAT(log_time, '%Y-%m-%d %H:%i:00') AS ts_min," +
		" IFNULL(hostid,'') AS hostid," +
		" IFNULL(ai_apikey_id,'') AS ai_apikey_id," +
		" IFNULL(ai_requested_model,'') AS ai_requested_model," +
		" IFNULL(ai_target_model,'') AS ai_target_model," +
		" IFNULL(ai_stream,0) AS ai_stream," +
		" IFNULL(product,'') AS product," +
		" IFNULL(cluster,'') AS cluster," +
		" IFNULL(sub_cluster,'') AS sub_cluster," +
		" IFNULL(backend_info,'') AS backend_info," +
		" IFNULL(method,'') AS method," +
		" IFNULL(res_status_code,0) AS res_status_code," +
		" IFNULL(err_code,'') AS err_code," +
		" IFNULL(header_host,'') AS header_host," +
		" IFNULL(ai_provider,'') AS ai_provider," +
		" IFNULL(ai_protocol,'') AS ai_protocol," +
		" IFNULL(ai_mode,'') AS ai_mode," +
		" IFNULL(ai_cost_currency,'') AS ai_cost_currency," +
		" IFNULL(level1Name,'') AS level1Name," +
		" IFNULL(level1,'') AS level1," +
		" IFNULL(level2Name,'') AS level2Name," +
		" IFNULL(level2,'') AS level2," +
		" IFNULL(level3Name,'') AS level3Name," +
		" IFNULL(level3,'') AS level3," +
		" IFNULL(level4Name,'') AS level4Name," +
		" IFNULL(level4,'') AS level4," +
		" IFNULL(level5Name,'') AS level5Name," +
		" IFNULL(level5,'') AS level5," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_rate_limit_hits,'$[0].rate_limit_policy_id')),'') AS rate_limit_policy_id," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_rate_limit_hits,'$[0].rate_limit_type')),'') AS rate_limit_type," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_rate_limit_hits,'$[0].rule_names[0]')),'') AS rate_limit_rule_name," +
		" IFNULL(ai_auth_reject_reason,'') AS ai_auth_reject_reason," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_auth_reject_quota_plans,'$[0]')),'') AS ai_auth_reject_quota_plans_slot1," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_auth_reject_quota_plans,'$[1]')),'') AS ai_auth_reject_quota_plans_slot2," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_auth_reject_quota_plans,'$[2]')),'') AS ai_auth_reject_quota_plans_slot3," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_auth_reject_quota_plans,'$[3]')),'') AS ai_auth_reject_quota_plans_slot4," +
		" IFNULL(JSON_UNQUOTE(JSON_EXTRACT(ai_auth_reject_quota_plans,'$[4]')),'') AS ai_auth_reject_quota_plans_slot5," +
		" IFNULL(ai_cache_status,'') AS ai_cache_status," +
		" IFNULL(mirror_hit,0) AS mirror_hit," +
		" IFNULL(ai_intent_answer,'') AS ai_intent_answer," +
		" COUNT(1) AS request_count," +
		" SUM(CASE WHEN err_code != '' AND err_code IS NOT NULL THEN 1 ELSE 0 END) AS error_count," +
		" SUM(CASE WHEN ai_auth_reject_reason != '' AND ai_auth_reject_reason IS NOT NULL THEN 1 ELSE 0 END) AS auth_reject_count," +
		" SUM(IFNULL(ai_input_tokens,0)) AS input_tokens," +
		" SUM(IFNULL(ai_output_tokens,0)) AS output_tokens," +
		" SUM(IFNULL(ai_total_tokens,0)) AS total_tokens," +
		" SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum," +
		" SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_tpot_us,0) ELSE 0 END) AS tpot_us_sum," +
		" SUM(IFNULL(req_header_len,0)) AS req_header_bytes," +
		" SUM(IFNULL(req_body_len,0)) AS req_body_bytes," +
		" SUM(IFNULL(res_header_len,0)) AS res_header_bytes," +
		" SUM(IFNULL(res_body_len,0)) AS res_body_bytes," +
		" SUM(CASE WHEN IFNULL(JSON_LENGTH(ai_rate_limit_hits),0) > 0 THEN 1 ELSE 0 END) AS rate_limit_hits," +
		" SUM(IFNULL(backend_retry,0)) AS backend_retries," +
		" SUM(IFNULL(all_time,0)) AS all_time_sum," +
		" SUM(IFNULL(cluster_serve_time,0)) AS cluster_serve_sum," +
		" SUM(IFNULL(backend_serve_time,0)) AS backend_serve_sum," +
		" SUM(IFNULL(ai_retry_count,0)) AS ai_retry_count_sum," +
		" SUM(IFNULL(ai_cost_value,0)) AS ai_cost_value_sum," +
		" SUM(IFNULL(ai_cache_read_tokens,0)) AS cache_read_tokens," +
		" SUM(IFNULL(ai_cache_write_tokens,0)) AS cache_write_tokens," +
		" SUM(IFNULL(ai_audio_input_tokens,0)) AS ai_audio_input_tokens," +
		" SUM(IFNULL(ai_audio_output_tokens,0)) AS ai_audio_output_tokens," +
		" SUM(IFNULL(ai_image_count,0)) AS ai_image_count" +
		" FROM " + detailTable +
		" WHERE log_time >= ? AND log_time < ?" +
		aggregateGroupBy
}

// buildAddPartitionSQL adds the daily partition covering [day, day+1):
// RANGE partitions only accept new boundaries greater than the current
// maximum, so planPartitionAdds must emit ascending days.
func buildAddPartitionSQL(table string, day time.Time) string {
	boundary := day.AddDate(0, 0, 1)
	return "ALTER TABLE " + table + " ADD PARTITION (PARTITION p" + day.Format("20060102") +
		" VALUES LESS THAN (TO_DAYS('" + boundary.Format("2006-01-02") + "')))"
}

func buildDropPartitionSQL(table, partition string) string {
	return "ALTER TABLE " + table + " DROP PARTITION " + partition
}

// buildPurgeSQL builds one batch of the non-partitioned DELETE fallback;
// MySQL requires the LIMIT of DELETE to be a literal integer.
func buildPurgeSQL(table, timeCol string, batch int) string {
	return "DELETE FROM " + table + " WHERE " + timeCol + " < ? LIMIT " + strconv.Itoa(batch)
}

// retentionCutoff returns the earliest kept day: rows before
// today-retentionDays+1 are expired.
func retentionCutoff(now time.Time, retentionDays int) time.Time {
	return civilDay(now).AddDate(0, 0, -retentionDays+1)
}

// civilDay returns the UTC midnight of the civil (calendar) date of t, so
// date comparisons in the planning functions are independent of the
// process timezone and of the zone mixed into parsed partition
// descriptions.
func civilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// partitionBoundaries extracts the exclusive upper-day of every partition.
// Descriptions that carry no parseable date are skipped.
func partitionBoundaries(parts []partitionInfo) []time.Time {
	boundaries := make([]time.Time, 0, len(parts))
	for _, p := range parts {
		if day, ok := parseBoundaryDate(p.Description); ok {
			boundaries = append(boundaries, day)
		}
	}
	return boundaries
}

var (
	dateInTextRe  = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	toDaysValueRe = regexp.MustCompile(`^\d+$`)
)

// toDaysUnixEpoch is TO_DAYS('1970-01-01'): the offset between MySQL's
// TO_DAYS numbering and civil days. FROM_DAYS / TO_DAYS use the proleptic
// Gregorian calendar, the same arithmetic Go's time package uses, so
// FROM_DAYS(n) is exactly epoch + (n - toDaysUnixEpoch) days.
const toDaysUnixEpoch = 719528

// toDaysMinSane bounds the accepted TO_DAYS values to dates after ~1916 so
// a bogus small integer description cannot make the planner try to add a
// cascade of partitions.
const toDaysMinSane = 700000

// parseBoundaryDate resolves the exclusive upper-day of a RANGE partition
// from its PARTITION_DESCRIPTION. MySQL 8.x echoes the evaluated form, so
// three shapes are accepted:
//
//  1. plain integer — the evaluated TO_DAYS('YYYY-MM-DD') value, inverted
//     with FROM_DAYS arithmetic;
//  2. literal date text "TO_DAYS('YYYY-MM-DD')" or a bare 'YYYY-MM-DD'
//     (kept for other shapes/versions that echo text);
//  3. "MAXVALUE" — unbounded; treated as unparseable, which keeps the
//     partition out of DROP decisions (conservative, never drops blindly).
//
// Anything else is unparseable and skipped by the planners. The result is
// a civil day (UTC midnight) with no timezone attached.
func parseBoundaryDate(description string) (time.Time, bool) {
	text := strings.TrimSpace(description)
	if text == "" {
		return time.Time{}, false
	}
	if strings.EqualFold(text, "MAXVALUE") {
		return time.Time{}, false
	}

	unquoted := strings.Trim(text, `'"`)
	if toDaysValueRe.MatchString(unquoted) {
		n, err := strconv.Atoi(unquoted)
		if err != nil || n < toDaysMinSane {
			return time.Time{}, false
		}
		return time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n-toDaysUnixEpoch), true
	}

	dateText := dateInTextRe.FindString(text)
	if dateText == "" {
		return time.Time{}, false
	}
	day, err := time.Parse("2006-01-02", dateText)
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}

// planPartitionAdds returns the ascending days whose partitions must exist
// so that data up to now+partitionAheadDays is covered: the partition
// covering day D has boundary D+1, and RANGE ADD requires the new boundary
// to exceed the current maximum boundary. Days already covered are skipped.
func planPartitionAdds(now time.Time, ahead int, boundaries []time.Time) []time.Time {
	var maxBoundary time.Time
	for _, b := range boundaries {
		if b.After(maxBoundary) {
			maxBoundary = b
		}
	}

	today := civilDay(now)
	var days []time.Time
	for i := 0; i < ahead; i++ {
		day := today.AddDate(0, 0, i)
		if day.AddDate(0, 0, 1).After(maxBoundary) {
			days = append(days, day)
		}
	}
	return days
}

// planPartitionDrops returns the names of the expired partitions: a
// partition whose exclusive boundary is on or before the retention cutoff
// only contains expired data. The last remaining partition is never
// dropped (MySQL requires at least one). A non-positive retentionDays
// keeps everything.
func planPartitionDrops(now time.Time, retentionDays int, parts []partitionInfo) []string {
	if retentionDays <= 0 || len(parts) <= 1 {
		return nil
	}

	cutoff := retentionCutoff(now, retentionDays)
	var names []string
	for _, p := range parts {
		day, ok := parseBoundaryDate(p.Description)
		if !ok {
			continue
		}
		if !day.After(cutoff) {
			names = append(names, p.Name)
		}
	}
	return names
}
