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

package dorisreport

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/didi/gendry/builder"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ireport"
)

const (
	tableDetail  = "bfe_ai_request_log"
	tableMetrics = "bfe_ai_metrics_1m"
)

// ReportStorager implements ireport.ReportStorager against Doris
// (standard deployment). Doris is reached through the FE MySQL-protocol
// port as a regular entry of the Databases map; no extra client is
// introduced. The dialect differences vs storage/mysqlreport follow
// design-docs/modifications/2026-09-15-report-query-api/design-changes.md
// §6: Grafana-style time buckets, PERCENTILE_APPROX on the detail table for
// p50/p90/p99, and plain `col != ”` empty-dimension predicates.
//
// Phase 1 of the cache/mirror/intent fields (design-docs modifications
// 2026-09-27-report-cache-mirror-intent-fields) only implemented the
// detail-backed parts here: the overview cache/mirror/intent counts, the
// log projection/filters and the timeseries cache_tokens metric. Phase 2
// (design-docs modifications 2026-10-01-report-doris-cache-mirror-intent-alignment,
// after the Doris aggregate table gains the three KEY dimensions) aligns
// the capability set with the MySQL backend: the rankings / distribution /
// timeseries dimension branches are implemented below and the three
// dimensions are declared in Capabilities, so the manager gate no longer
// fires for them.
type ReportStorager struct {
	db       *sql.DB
	database string // optional schema override used as table prefix
	backend  string // backend identifier reported by Capabilities()
}

// New creates a new Doris report storager. database may be empty (tables
// are then resolved within the connection's default schema) or a schema
// name used to prefix table names. backend is the capability identifier
// injected by the container assembly (e.g. "doris").
func New(db *sql.DB, database, backend string) *ReportStorager {
	return &ReportStorager{db: db, database: database, backend: backend}
}

var _ ireport.ReportStorager = (*ReportStorager)(nil)

// Capabilities implements ireport.ReportStorager. Phase 2 aligns the
// dimension set with the MySQL backend: the aggregate table carries the
// cache/mirror/intent KEY dimensions, so all 13 dimensions are declared
// (see design-docs modifications
// 2026-10-01-report-doris-cache-mirror-intent-alignment). The manager gate
// against Capabilities stays as a defense-in-depth check for future
// dimensions.
func (s *ReportStorager) Capabilities() *ireport.BackendCaps {
	return &ireport.BackendCaps{
		Backend: s.backend,
		SupportedDimensions: []string{
			ireport.DimensionModel,
			ireport.DimensionRequestedModel,
			ireport.DimensionProvider,
			ireport.DimensionAPIKey,
			ireport.DimensionHost,
			ireport.DimensionStatus,
			ireport.DimensionProtocol,
			ireport.DimensionMode,
			ireport.DimensionStream,
			ireport.DimensionCacheStatus,
			ireport.DimensionMirrorHit,
			ireport.DimensionIntentAnswer,
		},
	}
}

func (s *ReportStorager) table(name string) string {
	if s.database == "" {
		return name
	}
	return s.database + "." + name
}

func toInterfaces(ss []string) []interface{} {
	vals := make([]interface{}, 0, len(ss))
	for _, one := range ss {
		vals = append(vals, one)
	}
	return vals
}

func toIntInterfaces(nums []int) []interface{} {
	vals := make([]interface{}, 0, len(nums))
	for _, one := range nums {
		vals = append(vals, one)
	}
	return vals
}

func streamValue(stream *bool) int8 {
	if stream != nil && *stream {
		return 1
	}
	return 0
}

// metricsWhere builds the shared WHERE map for the aggregate table.
func metricsWhere(f *ireport.Filter) map[string]interface{} {
	where := map[string]interface{}{
		"ts_min >=": f.Start,
		"ts_min <":  f.End,
	}
	if len(f.Models) > 0 {
		where["ai_target_model in"] = toInterfaces(f.Models)
	}
	if len(f.ApikeyIDs) > 0 {
		where["ai_apikey_id in"] = toInterfaces(f.ApikeyIDs)
	}
	if len(f.Providers) > 0 {
		where["ai_provider in"] = toInterfaces(f.Providers)
	}
	if len(f.Hosts) > 0 {
		where["hostid in"] = toInterfaces(f.Hosts)
	}
	if f.Stream != nil {
		where["ai_stream ="] = streamValue(f.Stream)
	}
	if len(f.StatusCodes) > 0 {
		where["res_status_code in"] = toIntInterfaces(f.StatusCodes)
	}
	return where
}

// detailWhere builds the WHERE map for the detail table from the shared
// filter fields (the detail table uses the same column names).
func detailWhere(f *ireport.Filter) map[string]interface{} {
	where := map[string]interface{}{
		"log_time >=": f.Start,
		"log_time <":  f.End,
	}
	if len(f.Models) > 0 {
		where["ai_target_model in"] = toInterfaces(f.Models)
	}
	if len(f.ApikeyIDs) > 0 {
		where["ai_apikey_id in"] = toInterfaces(f.ApikeyIDs)
	}
	if len(f.Providers) > 0 {
		where["ai_provider in"] = toInterfaces(f.Providers)
	}
	if len(f.Hosts) > 0 {
		where["hostid in"] = toInterfaces(f.Hosts)
	}
	if f.Stream != nil {
		where["ai_stream ="] = streamValue(f.Stream)
	}
	if len(f.StatusCodes) > 0 {
		where["res_status_code in"] = toIntInterfaces(f.StatusCodes)
	}
	return where
}

// logWhere extends detailWhere with the logs-only criteria.
func logWhere(f *ireport.LogFilter) map[string]interface{} {
	where := detailWhere(&f.Filter)
	if len(f.RequestedModels) > 0 {
		where["ai_requested_model in"] = toInterfaces(f.RequestedModels)
	}
	if f.ErrOnly {
		where["IFNULL(err_code,'') !="] = ""
	}
	if f.Keyword != "" {
		where["err_msg like"] = "%" + f.Keyword + "%"
	}
	if f.CacheStatus != nil {
		where["ai_cache_status ="] = *f.CacheStatus
	}
	if f.MirrorHit != nil {
		where["mirror_hit ="] = streamValue(f.MirrorHit)
	}
	if f.IntentQuestion != nil {
		where["ai_intent_question ="] = *f.IntentQuestion
	}
	if f.IntentAnswer != nil {
		where["ai_intent_answer ="] = *f.IntentAnswer
	}
	if f.IntentSource != nil {
		where["ai_intent_source ="] = *f.IntentSource
	}
	return where
}

// epochLiteral is the wall-clock UTC epoch used to turn a stored DATETIME
// into Unix seconds without any session-timezone interpretation
// (TIMESTAMPDIFF is pure calendar arithmetic; Doris' UNIX_TIMESTAMP reads
// the DATETIME in the session zone just like MySQL's). log-reader writes
// UTC wall clock, so the stored value IS the UTC wall clock.
const epochLiteral = "'1970-01-01 00:00:00'"

// bucketExpr is the Doris time-bucket expression aligned with the Grafana
// $__timeGroup rendering. The bucket width is server-controlled (one of
// 60/300/1800 computed by the manager) and inlined as an integer literal:
// gendry cannot bind parameters inside SELECT fields.
func bucketExpr(timeCol string, bucketSec int) string {
	b := strconv.Itoa(bucketSec)
	return "CAST(FLOOR(TIMESTAMPDIFF(SECOND, " + epochLiteral + ", " + timeCol + ")/" + b + ")*" + b + " AS SIGNED) AS time"
}

var overviewMetricFields = []string{
	"IFNULL(SUM(request_count),0) AS request_total",
	"IFNULL(SUM(error_count),0) AS error_total",
	"IFNULL(SUM(input_tokens),0) AS input_tokens",
	"IFNULL(SUM(output_tokens),0) AS output_tokens",
	"IFNULL(SUM(total_tokens),0) AS total_tokens",
	"IFNULL(SUM(all_time_sum),0) AS all_time_sum",
	"IFNULL(MAX(all_time_sum/request_count),0) AS latency_max",
	"IFNULL(SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END),0) AS ttft_us_sum",
	"IFNULL(SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END),0) AS stream_requests",
	"IFNULL(SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END),0) AS tpot_us_sum",
	"IFNULL(SUM(rate_limit_hits),0) AS rate_limit_hits",
	"IFNULL(SUM(auth_reject_count),0) AS auth_rejects",
	"IFNULL(SUM(cache_read_tokens),0) AS cache_read_tokens",
	"IFNULL(SUM(cache_write_tokens),0) AS cache_write_tokens",
}

// overviewDetailCountFields are the conditional detail-table COUNT columns
// of the cache/mirror/intent indicator groups (same calibers as the MySQL
// backend; IFNULL is accepted by Doris).
var overviewDetailCountFields = []string{
	"IFNULL(SUM(CASE WHEN ai_cache_status='hit' THEN 1 ELSE 0 END),0) AS cache_hit_count",
	"IFNULL(SUM(CASE WHEN ai_cache_status='miss' THEN 1 ELSE 0 END),0) AS cache_miss_count",
	"IFNULL(SUM(CASE WHEN ai_cache_status='skip' THEN 1 ELSE 0 END),0) AS cache_skip_count",
	"IFNULL(SUM(CASE WHEN IFNULL(mirror_hit,0)=1 THEN 1 ELSE 0 END),0) AS mirror_hit_count",
	"IFNULL(SUM(CASE WHEN IFNULL(ai_intent_answer,'')!='' AND ai_intent_answer!='unknown' THEN 1 ELSE 0 END),0) AS intent_classified_count",
	"IFNULL(SUM(CASE WHEN ai_intent_answer='unknown' THEN 1 ELSE 0 END),0) AS intent_unknown_count",
}

func buildOverviewMetricsSQL(metricsTable string, f *ireport.Filter) (string, []interface{}, error) {
	return builder.BuildSelect(metricsTable, metricsWhere(f), overviewMetricFields)
}

func buildOverviewDetailCountsSQL(detailTable string, f *ireport.Filter) (string, []interface{}, error) {
	return builder.BuildSelect(detailTable, detailWhere(f), overviewDetailCountFields)
}

func buildOverviewCostSQL(metricsTable string, f *ireport.Filter) (string, []interface{}, error) {
	where := metricsWhere(f)
	where["ai_cost_currency !="] = ""
	where["_groupby"] = "ai_cost_currency"
	return builder.BuildSelect(metricsTable, where, []string{
		"ai_cost_currency AS currency",
		"IFNULL(SUM(ai_cost_value_sum),0) AS value",
	})
}

// buildOverviewPercentileSQL computes the Doris-only p50/p90/p99 of
// all_time over the whole window from the detail table, aligning with the
// Grafana latency panel (PERCENTILE_APPROX on bfe_ai_request_log).
func buildOverviewPercentileSQL(detailTable string, f *ireport.Filter) (string, []interface{}, error) {
	where := detailWhere(f)
	where["all_time null"] = builder.IsNotNull
	return builder.BuildSelect(detailTable, where, []string{
		"PERCENTILE_APPROX(all_time, 0.5) AS p50",
		"PERCENTILE_APPROX(all_time, 0.9) AS p90",
		"PERCENTILE_APPROX(all_time, 0.99) AS p99",
	})
}

func buildLogsTotalSQL(detailTable string, f *ireport.Filter) (string, []interface{}, error) {
	return builder.BuildSelect(detailTable, detailWhere(f), []string{"COUNT(*)"})
}

// buildTimeSeriesSQL builds the per-metric time-series query over the
// aggregate table. The optional dimension (one of the cache/mirror/intent
// dimensions) splits the series per dimension value: the column renders as
// "name" and widens the GROUP BY. Value fields are raw per-bucket sums; the
// per-second division happens in rowToMetricPoint. cache_tokens has no
// column to group by, so it is rendered as a UNION ALL of the
// cache_read/cache_write arms, each carrying its literal kind.
func buildTimeSeriesSQL(metricsTable, metric, dimension string, f *ireport.Filter, bucketSec int) (string, []interface{}, error) {
	column := ""
	if dimension != "" {
		col, ok := ireport.DimensionColumns[dimension]
		if !ok {
			return "", nil, xerror.WrapParamErrorWithMsg("invalid dimension: %s", dimension)
		}
		column = col
	}

	if metric == ireport.MetricCacheTokens {
		return buildCacheTokensTimeSeriesSQL(metricsTable, column, f, bucketSec)
	}

	where := metricsWhere(f)
	groupBy := "time"
	if column != "" {
		groupBy = "time," + column
	}
	if metric == ireport.MetricCost {
		groupBy = "time,ai_cost_currency"
		if column != "" {
			groupBy = "time,ai_cost_currency," + column
		}
	}
	where["_groupby"] = groupBy
	where["_orderby"] = "time ASC"

	fields, err := timeSeriesMetricFields(metric, bucketSec, column)
	if err != nil {
		return "", nil, err
	}
	return builder.BuildSelect(metricsTable, where, fields)
}

// timeSeriesMetricFields is the SELECT field list of one metric arm; the
// optional dimension column is rendered as "name" right after the bucket.
func timeSeriesMetricFields(metric string, bucketSec int, column string) ([]string, error) {
	fields := []string{bucketExpr("ts_min", bucketSec)}
	if column != "" {
		fields = append(fields, column+" AS name")
	}
	switch metric {
	case ireport.MetricQPS:
		fields = append(fields, "SUM(request_count) AS total")
	case ireport.MetricTokens:
		fields = append(fields,
			"SUM(input_tokens) AS input",
			"SUM(output_tokens) AS output",
			"SUM(total_tokens) AS total")
	case ireport.MetricLatency:
		fields = append(fields,
			"SUM(all_time_sum) AS all_time_sum",
			"SUM(request_count) AS request_count",
			"MAX(all_time_sum/request_count) AS latency_max")
	case ireport.MetricTTFT, ireport.MetricTPOT:
		fields = append(fields,
			"SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END) AS ttft_us_sum",
			"SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END) AS tpot_us_sum",
			"SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END) AS stream_requests")
	case ireport.MetricCost:
		fields = append(fields,
			"ai_cost_currency AS currency",
			"SUM(ai_cost_value_sum) AS value")
	default:
		return nil, xerror.WrapParamErrorWithMsg("invalid metric: %s", metric)
	}
	return fields, nil
}

// buildCacheTokensTimeSeriesSQL renders the cache_read/cache_write series
// as a UNION ALL over the same WHERE, the same caliber as the MySQL
// backend (Doris accepts UNION ALL with a trailing ORDER BY). The optional
// dimension column is carried as "name" in each arm.
func buildCacheTokensTimeSeriesSQL(metricsTable, column string, f *ireport.Filter, bucketSec int) (string, []interface{}, error) {
	where := metricsWhere(f)
	groupBy := "time"
	if column != "" {
		groupBy = "time," + column
	}
	where["_groupby"] = groupBy

	arm := func(kind, sumField string) (string, []interface{}, error) {
		fields := []string{bucketExpr("ts_min", bucketSec), "'" + kind + "' AS kind"}
		if column != "" {
			fields = append(fields, column+" AS name")
		}
		fields = append(fields, "SUM("+sumField+") AS value")
		return builder.BuildSelect(metricsTable, where, fields)
	}

	readSQL, readArgs, err := arm("cache_read", "cache_read_tokens")
	if err != nil {
		return "", nil, err
	}
	writeSQL, writeArgs, err := arm("cache_write", "cache_write_tokens")
	if err != nil {
		return "", nil, err
	}

	orderBy := "time ASC"
	if column != "" {
		orderBy = "time ASC," + column + " ASC"
	}
	orderBy += ",kind ASC"
	query := readSQL + " UNION ALL " + writeSQL + " ORDER BY " + orderBy
	return query, append(readArgs, writeArgs...), nil
}

// buildLatencyPercentileSQL is the Doris-only companion of the latency
// time-series: per-bucket p50/p90/p99 of all_time from the detail table.
func buildLatencyPercentileSQL(detailTable string, f *ireport.Filter, bucketSec int) (string, []interface{}, error) {
	where := detailWhere(f)
	where["all_time null"] = builder.IsNotNull
	where["_groupby"] = "time"
	where["_orderby"] = "time ASC"
	return builder.BuildSelect(detailTable, where, []string{
		bucketExpr("log_time", bucketSec),
		"PERCENTILE_APPROX(all_time, 0.5) AS p50",
		"PERCENTILE_APPROX(all_time, 0.9) AS p90",
		"PERCENTILE_APPROX(all_time, 0.99) AS p99",
	})
}

// rankingEmptyPredicate excludes the empty marker of a dimension from
// top-N rankings (0 for the numeric status column, ” for strings), the
// same caliber as the MySQL backend. It returns the gendry where key and
// value: the operator is always "!=" and the value is bound as a
// parameter.
func rankingEmptyPredicate(dimension, column string) (string, interface{}) {
	if dimension == ireport.DimensionStatus {
		return column + " !=", 0
	}
	return column + " !=", ""
}

func buildRankingsSQL(metricsTable, dimension string, f *ireport.Filter, limit int) (string, []interface{}, error) {
	column, ok := ireport.DimensionColumns[dimension]
	if !ok {
		return "", nil, xerror.WrapParamErrorWithMsg("invalid dimension: %s", dimension)
	}

	field := column + " AS name"
	if dimension == ireport.DimensionStatus || dimension == ireport.DimensionStream || dimension == ireport.DimensionMirrorHit {
		field = "CAST(" + column + " AS CHAR) AS name"
	}

	where := metricsWhere(f)
	// mirror_hit is a numeric 0/1 dimension without an empty marker; both
	// buckets rank. String dimensions keep the empty-marker exclusion.
	if dimension != ireport.DimensionMirrorHit {
		emptyKey, emptyValue := rankingEmptyPredicate(dimension, column)
		where[emptyKey] = emptyValue
	}
	where["_groupby"] = column
	where["_orderby"] = "request_count DESC"

	query, args, err := builder.BuildSelect(metricsTable, where, []string{
		field,
		"SUM(request_count) AS request_count",
		"SUM(error_count) AS error_count",
		"SUM(input_tokens) AS input_tokens",
		"SUM(output_tokens) AS output_tokens",
	})
	if err != nil {
		return "", nil, err
	}
	// Doris cannot parse bound parameters in LIMIT (a prepared `LIMIT ?,?`
	// fails with "mismatched input 'LIMIT'"); limit is a server-derived
	// integer clamped by the manager, inlined as a literal like the
	// time-bucket width in bucketExpr.
	return fmt.Sprintf("%s LIMIT %d", query, limit), args, nil
}

// distributionNameExpr renders the dimension value as its display name,
// normalizing NULL/empty to 'unknown'. Numeric dimensions are cast to
// strings so the response shape is uniform.
func distributionNameExpr(dimension string) (string, error) {
	switch dimension {
	case ireport.DimensionStatus:
		return "CASE WHEN IFNULL(res_status_code,0)=0 THEN 'unknown' ELSE CAST(res_status_code AS CHAR) END AS name", nil
	case ireport.DimensionStream:
		return "CAST(ai_stream AS CHAR) AS name", nil
	case ireport.DimensionProtocol:
		return "CASE WHEN IFNULL(ai_protocol,'')='' THEN 'unknown' ELSE ai_protocol END AS name", nil
	case ireport.DimensionMode:
		return "CASE WHEN IFNULL(ai_mode,'')='' THEN 'unknown' ELSE ai_mode END AS name", nil
	case ireport.DimensionCacheStatus:
		return "CASE WHEN IFNULL(ai_cache_status,'')='' THEN 'unknown' ELSE ai_cache_status END AS name", nil
	case ireport.DimensionIntentAnswer:
		return "CASE WHEN IFNULL(ai_intent_answer,'')='' THEN 'unknown' ELSE ai_intent_answer END AS name", nil
	case ireport.DimensionMirrorHit:
		return "CAST(mirror_hit AS CHAR) AS name", nil
	default:
		return "", xerror.WrapParamErrorWithMsg("invalid dimension: %s", dimension)
	}
}

func buildDistributionSQL(metricsTable, dimension string, f *ireport.Filter) (string, []interface{}, error) {
	nameExpr, err := distributionNameExpr(dimension)
	if err != nil {
		return "", nil, err
	}

	where := metricsWhere(f)
	where["_groupby"] = "name"
	where["_orderby"] = "request_count DESC"

	return builder.BuildSelect(metricsTable, where, []string{
		nameExpr,
		"SUM(request_count) AS request_count",
	})
}

func buildLogsCountSQL(detailTable string, f *ireport.LogFilter) (string, []interface{}, error) {
	return builder.BuildSelect(detailTable, logWhere(f), []string{"COUNT(*)"})
}

// logRowFields is the display projection of the detail row, identical to
// the MySQL backend (the two tables share column names by design).
// log_time renders via TIMESTAMPDIFF so the value is session-timezone
// neutral on both backends.
var logRowFields = []string{
	"logid",
	"TIMESTAMPDIFF(SECOND, " + epochLiteral + ", log_time) AS log_time",
	"hostid",
	"product",
	"ai_apikey_id",
	"ai_requested_model",
	"ai_target_model",
	"ai_provider",
	"ai_protocol",
	"ai_mode",
	"ai_stream",
	"res_status_code",
	"err_code",
	"err_msg",
	"ai_input_tokens",
	"ai_output_tokens",
	"ai_total_tokens",
	"all_time",
	"ai_ttft_us",
	"ai_tpot_us",
	"ai_cost_value",
	"ai_cost_currency",
	"ai_rate_limit_hits",
	"ai_auth_reject_quota_plans",
	"level1Name",
	"level1",
	"level2Name",
	"level2",
	"level3Name",
	"level3",
	"level4Name",
	"level4",
	"level5Name",
	"level5",
	"client_ip",
	"header_host",
	"origin_uri",
	"req_headers",
	"res_headers",
	"ai_cache_status",
	"mirror_hit",
	"mirror_cluster",
	"ai_intent_question",
	"ai_intent_answer",
	"ai_intent_confidence",
	"ai_intent_source",
	"ai_intent_latency_us",
	"ai_intent_cache_hit",
	"ai_intent_questions_version",
}

func buildLogsSQL(detailTable string, f *ireport.LogFilter) (string, []interface{}, error) {
	offset := uint(0)
	if f.Page > 1 {
		offset = uint(f.Page-1) * uint(f.PageSize)
	}

	where := logWhere(f)
	where["_orderby"] = "log_time DESC"
	query, args, err := builder.BuildSelect(detailTable, where, logRowFields)
	if err != nil {
		return "", nil, err
	}
	// Doris cannot parse bound parameters in LIMIT (a prepared `LIMIT ?,?`
	// fails with "mismatched input 'LIMIT'"); offset/pageSize are
	// server-derived integers clamped by the manager, inlined as literals
	// like the time-bucket width in bucketExpr.
	return fmt.Sprintf("%s LIMIT %d OFFSET %d", query, uint(f.PageSize), offset), args, nil
}

// Overview implements ireport.ReportStorager. Compared to the MySQL
// backend it additionally fills latency_p50_ms / p90 / p99 from the detail
// table via PERCENTILE_APPROX.
func (s *ReportStorager) Overview(ctx context.Context, f *ireport.Filter) (*ireport.OverviewResult, error) {
	metricsTable := s.table(tableMetrics)
	detailTable := s.table(tableDetail)

	metricsSQL, metricsArgs, err := buildOverviewMetricsSQL(metricsTable, f)
	if err != nil {
		return nil, err
	}

	row := &overviewMetricsRow{}
	err = s.db.QueryRowContext(ctx, metricsSQL, metricsArgs...).Scan(
		&row.requestTotal,
		&row.errorTotal,
		&row.inputTokens,
		&row.outputTokens,
		&row.totalTokens,
		&row.allTimeSum,
		&row.latencyMax,
		&row.ttftUsSum,
		&row.streamRequests,
		&row.tpotUsSum,
		&row.rateLimitHits,
		&row.authRejects,
		&row.cacheReadTokens,
		&row.cacheWriteTokens,
	)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	costSQL, costArgs, err := buildOverviewCostSQL(metricsTable, f)
	if err != nil {
		return nil, err
	}
	cost, err := s.queryCost(ctx, costSQL, costArgs)
	if err != nil {
		return nil, err
	}

	logsSQL, logsArgs, err := buildLogsTotalSQL(detailTable, f)
	if err != nil {
		return nil, err
	}
	var logsTotal int64
	if err := s.db.QueryRowContext(ctx, logsSQL, logsArgs...).Scan(&logsTotal); err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	detailCountsSQL, detailCountsArgs, err := buildOverviewDetailCountsSQL(detailTable, f)
	if err != nil {
		return nil, err
	}
	detailCounts := &overviewDetailCounts{}
	if err := s.db.QueryRowContext(ctx, detailCountsSQL, detailCountsArgs...).Scan(
		&detailCounts.cacheHitCount,
		&detailCounts.cacheMissCount,
		&detailCounts.cacheSkipCount,
		&detailCounts.mirrorHitCount,
		&detailCounts.intentClassifiedCount,
		&detailCounts.intentUnknownCount,
	); err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	result := overviewResultFromRow(row, cost, logsTotal, detailCounts)

	percentileSQL, percentileArgs, err := buildOverviewPercentileSQL(detailTable, f)
	if err != nil {
		return nil, err
	}
	var p50, p90, p99 sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, percentileSQL, percentileArgs...).Scan(&p50, &p90, &p99); err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	if p50.Valid {
		v := p50.Float64
		result.LatencyP50Ms = &v
	}
	if p90.Valid {
		v := p90.Float64
		result.LatencyP90Ms = &v
	}
	if p99.Valid {
		v := p99.Float64
		result.LatencyP99Ms = &v
	}

	return result, nil
}

func (s *ReportStorager) queryCost(ctx context.Context, query string, args []interface{}) ([]*ireport.CostItem, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	items := make([]*ireport.CostItem, 0, 4)
	for rows.Next() {
		item := &ireport.CostItem{}
		var raw int64
		if err := rows.Scan(&item.Currency, &raw); err != nil {
			return nil, xerror.WrapDaoError(err)
		}
		item.Value = ireport.CostFixedPointToAmount(raw)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	return items, nil
}

// overviewMetricsRow is the scan target of the overview aggregate query.
type overviewMetricsRow struct {
	requestTotal     sql.NullInt64
	errorTotal       sql.NullInt64
	inputTokens      sql.NullInt64
	outputTokens     sql.NullInt64
	totalTokens      sql.NullInt64
	allTimeSum       sql.NullInt64
	latencyMax       sql.NullFloat64
	ttftUsSum        sql.NullInt64
	streamRequests   sql.NullInt64
	tpotUsSum        sql.NullInt64
	rateLimitHits    sql.NullInt64
	authRejects      sql.NullInt64
	cacheReadTokens  sql.NullInt64
	cacheWriteTokens sql.NullInt64
}

// overviewDetailCounts is the scan target of the overview conditional
// detail counts (cache status distribution, mirror hits, intent classified
// vs unknown).
type overviewDetailCounts struct {
	cacheHitCount         sql.NullInt64
	cacheMissCount        sql.NullInt64
	cacheSkipCount        sql.NullInt64
	mirrorHitCount        sql.NullInt64
	intentClassifiedCount sql.NullInt64
	intentUnknownCount    sql.NullInt64
}

// overviewResultFromRow assembles the overview card with the documented
// calibers (identical to the MySQL backend); the percentile fields stay
// nil here and are attached by Overview (Doris only).
func overviewResultFromRow(row *overviewMetricsRow, cost []*ireport.CostItem, logsTotal int64,
	detailCounts *overviewDetailCounts) *ireport.OverviewResult {
	requestTotal := row.requestTotal.Int64
	streamRequests := row.streamRequests.Int64

	result := &ireport.OverviewResult{
		RequestTotal:  requestTotal,
		ErrorTotal:    row.errorTotal.Int64,
		InputTokens:   row.inputTokens.Int64,
		OutputTokens:  row.outputTokens.Int64,
		TotalTokens:   row.totalTokens.Int64,
		LatencyMaxMs:  row.latencyMax.Float64,
		Cost:          cost,
		RateLimitHits: row.rateLimitHits.Int64,
		AuthRejects:   row.authRejects.Int64,
		LogsTotal:     logsTotal,

		Cache: ireport.CacheOverview{
			HitCount:    detailCounts.cacheHitCount.Int64,
			MissCount:   detailCounts.cacheMissCount.Int64,
			SkipCount:   detailCounts.cacheSkipCount.Int64,
			ReadTokens:  row.cacheReadTokens.Int64,
			WriteTokens: row.cacheWriteTokens.Int64,
		},
		Mirror: ireport.MirrorOverview{
			HitCount: detailCounts.mirrorHitCount.Int64,
		},
		Intent: ireport.IntentOverview{
			ClassifiedCount: detailCounts.intentClassifiedCount.Int64,
			UnknownCount:    detailCounts.intentUnknownCount.Int64,
		},
	}
	if requestTotal > 0 {
		result.ErrorRate = float64(row.errorTotal.Int64) / float64(requestTotal)
		result.LatencyAvgMs = float64(row.allTimeSum.Int64) / float64(requestTotal)
	}
	if streamRequests > 0 {
		result.TtftAvgMs = float64(row.ttftUsSum.Int64) / float64(streamRequests) / 1000
		result.TpotAvgMs = float64(row.tpotUsSum.Int64) / float64(streamRequests) / 1000
	}
	cacheHitMiss := result.Cache.HitCount + result.Cache.MissCount
	if cacheHitMiss > 0 {
		result.Cache.HitRate = float64(result.Cache.HitCount) / float64(cacheHitMiss)
	}
	intentTotal := result.Intent.ClassifiedCount + result.Intent.UnknownCount
	if intentTotal > 0 {
		result.Intent.UnknownRate = float64(result.Intent.UnknownCount) / float64(intentTotal)
	}
	return result
}

// TimeSeries implements ireport.ReportStorager. For the latency metric it
// additionally queries per-bucket percentiles from the detail table and
// merges them into the aggregate points (Doris only). Percentiles are
// whole-bucket calibers, so they are only attached to the plain
// single-series latency points; a dimension-split latency series carries
// avg/max per (bucket, name) only — the same degradation as the MySQL
// backend, which never returns percentiles.
func (s *ReportStorager) TimeSeries(ctx context.Context, metric, dimension string, f *ireport.Filter, bucketSec int) ([]*ireport.MetricPoint, error) {
	query, args, err := buildTimeSeriesSQL(s.table(tableMetrics), metric, dimension, f, bucketSec)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	points, err := scanMetricPoints(rows, metric, dimension, bucketSec)
	rows.Close()
	if err != nil {
		return nil, err
	}

	if metric != ireport.MetricLatency || dimension != "" {
		return points, nil
	}

	percentileSQL, percentileArgs, err := buildLatencyPercentileSQL(s.table(tableDetail), f, bucketSec)
	if err != nil {
		return nil, err
	}
	prows, err := s.db.QueryContext(ctx, percentileSQL, percentileArgs...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	err = mergeLatencyPercentiles(prows, points)
	prows.Close()
	if err != nil {
		return nil, err
	}
	return points, nil
}

// rowScanner abstracts *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// metricRowValues carries the scanned raw sums of one bucket.
type metricRowValues struct {
	name           string
	kind           string
	total          int64
	input          int64
	output         int64
	allTimeSum     int64
	requestCount   int64
	latencyMax     float64
	ttftUsSum      int64
	tpotUsSum      int64
	streamRequests int64
	value          int64
	currency       string
}

func scanMetricPoints(rows *sql.Rows, metric, dimension string, bucketSec int) ([]*ireport.MetricPoint, error) {
	points := make([]*ireport.MetricPoint, 0, 128)
	for rows.Next() {
		point, err := scanMetricPoint(rows, metric, dimension, bucketSec)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	if err := rows.Err(); err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	return points, nil
}

func scanMetricPoint(scanner rowScanner, metric, dimension string, bucketSec int) (*ireport.MetricPoint, error) {
	var (
		bucket         sql.NullInt64
		name           sql.NullString
		kind           sql.NullString
		total          sql.NullInt64
		input          sql.NullInt64
		output         sql.NullInt64
		allTimeSum     sql.NullInt64
		requestCount   sql.NullInt64
		latencyMax     sql.NullFloat64
		ttftUsSum      sql.NullInt64
		tpotUsSum      sql.NullInt64
		streamRequests sql.NullInt64
		value          sql.NullInt64
		currency       sql.NullString
	)

	// Scan order mirrors the SELECT list: bucket, optional dimension name,
	// then the metric-specific tail (cache_tokens carries kind before value).
	head := []interface{}{&bucket}
	if dimension != "" {
		head = append(head, &name)
	}
	var tail []interface{}
	switch metric {
	case ireport.MetricQPS:
		tail = []interface{}{&total}
	case ireport.MetricTokens:
		tail = []interface{}{&input, &output, &total}
	case ireport.MetricLatency:
		tail = []interface{}{&allTimeSum, &requestCount, &latencyMax}
	case ireport.MetricTTFT, ireport.MetricTPOT:
		tail = []interface{}{&ttftUsSum, &tpotUsSum, &streamRequests}
	case ireport.MetricCost:
		tail = []interface{}{&currency, &value}
	case ireport.MetricCacheTokens:
		tail = []interface{}{&kind, &value}
	default:
		return nil, xerror.WrapParamErrorWithMsg("invalid metric: %s", metric)
	}
	if err := scanner.Scan(append(head, tail...)...); err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	return rowToMetricPoint(metric, bucket.Int64, bucketSec, metricRowValues{
		name:           name.String,
		kind:           kind.String,
		total:          total.Int64,
		input:          input.Int64,
		output:         output.Int64,
		allTimeSum:     allTimeSum.Int64,
		requestCount:   requestCount.Int64,
		latencyMax:     latencyMax.Float64,
		ttftUsSum:      ttftUsSum.Int64,
		tpotUsSum:      tpotUsSum.Int64,
		streamRequests: streamRequests.Int64,
		value:          value.Int64,
		currency:       currency.String,
	}), nil
}

// rowToMetricPoint converts raw per-bucket sums into a point, applying the
// per-second rates and per-request calibers (pure function, identical
// semantics to the MySQL backend).
func rowToMetricPoint(metric string, bucket int64, bucketSec int, v metricRowValues) *ireport.MetricPoint {
	point := &ireport.MetricPoint{Time: bucket, Name: v.name, Kind: v.kind}
	float64Ptr := func(f float64) *float64 { return &f }

	switch metric {
	case ireport.MetricQPS:
		point.Value = float64Ptr(float64(v.total) / float64(bucketSec))
	case ireport.MetricTokens:
		point.Input = float64Ptr(float64(v.input) / float64(bucketSec))
		point.Output = float64Ptr(float64(v.output) / float64(bucketSec))
		point.Total = float64Ptr(float64(v.total) / float64(bucketSec))
	case ireport.MetricLatency:
		if v.requestCount > 0 {
			point.Avg = float64Ptr(float64(v.allTimeSum) / float64(v.requestCount))
		} else {
			point.Avg = float64Ptr(0)
		}
		point.Max = float64Ptr(v.latencyMax)
	case ireport.MetricTTFT:
		point.Value = float64Ptr(avgLatencyMs(v.ttftUsSum, v.streamRequests))
	case ireport.MetricTPOT:
		point.Value = float64Ptr(avgLatencyMs(v.tpotUsSum, v.streamRequests))
	case ireport.MetricCost:
		point.Value = float64Ptr(float64(v.value) / float64(bucketSec) / ireport.CostFixedPointScale)
		point.Currency = v.currency
	case ireport.MetricCacheTokens:
		point.Value = float64Ptr(float64(v.value) / float64(bucketSec))
	}
	return point
}

func avgLatencyMs(usSum, streamRequests int64) float64 {
	if streamRequests <= 0 {
		return 0
	}
	return float64(usSum) / float64(streamRequests) / 1000
}

// mergeLatencyPercentiles attaches per-bucket p50/p90/p99 to the aggregate
// latency points, keyed by bucket time (pure merge over the scanned rows).
func mergeLatencyPercentiles(rows *sql.Rows, points []*ireport.MetricPoint) error {
	byTime := make(map[int64]*ireport.MetricPoint, len(points))
	for _, point := range points {
		byTime[point.Time] = point
	}

	for rows.Next() {
		var (
			bucket sql.NullInt64
			p50    sql.NullFloat64
			p90    sql.NullFloat64
			p99    sql.NullFloat64
		)
		if err := rows.Scan(&bucket, &p50, &p90, &p99); err != nil {
			return xerror.WrapDaoError(err)
		}
		point, ok := byTime[bucket.Int64]
		if !ok {
			continue
		}
		if p50.Valid {
			v := p50.Float64
			point.P50 = &v
		}
		if p90.Valid {
			v := p90.Float64
			point.P90 = &v
		}
		if p99.Valid {
			v := p99.Float64
			point.P99 = &v
		}
	}
	return xerror.WrapDaoError(rows.Err())
}

// Rankings implements ireport.ReportStorager.
func (s *ReportStorager) Rankings(ctx context.Context, dimension string, f *ireport.Filter, limit int) ([]*ireport.RankingItem, error) {
	query, args, err := buildRankingsSQL(s.table(tableMetrics), dimension, f, limit)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	items := make([]*ireport.RankingItem, 0, limit)
	for rows.Next() {
		item := &ireport.RankingItem{}
		if err := rows.Scan(&item.Name, &item.RequestCount, &item.ErrorCount, &item.InputTokens, &item.OutputTokens); err != nil {
			return nil, xerror.WrapDaoError(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	return items, nil
}

// Distribution implements ireport.ReportStorager.
func (s *ReportStorager) Distribution(ctx context.Context, dimension string, f *ireport.Filter) ([]*ireport.DistItem, error) {
	query, args, err := buildDistributionSQL(s.table(tableMetrics), dimension, f)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	items := make([]*ireport.DistItem, 0, 16)
	for rows.Next() {
		item := &ireport.DistItem{}
		if err := rows.Scan(&item.Name, &item.RequestCount); err != nil {
			return nil, xerror.WrapDaoError(err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); nil != err {
		return nil, xerror.WrapDaoError(err)
	}

	distributionRatios(items)
	return items, nil
}

// distributionRatios fills the ratio of each bucket in place (pure
// function): bucket requests / window total requests.
func distributionRatios(items []*ireport.DistItem) {
	var total int64
	for _, item := range items {
		total += item.RequestCount
	}
	if total == 0 {
		return
	}
	for _, item := range items {
		item.Ratio = float64(item.RequestCount) / float64(total)
	}
}

// Logs implements ireport.ReportStorager.
func (s *ReportStorager) Logs(ctx context.Context, f *ireport.LogFilter) (*ireport.LogQueryResult, error) {
	detailTable := s.table(tableDetail)

	countSQL, countArgs, err := buildLogsCountSQL(detailTable, f)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	listSQL, listArgs, err := buildLogsSQL(detailTable, f)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	items := make([]*ireport.LogRow, 0, f.PageSize)
	for rows.Next() {
		item, err := scanLogRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	return &ireport.LogQueryResult{
		Total:    total,
		Page:     f.Page,
		PageSize: f.PageSize,
		Items:    items,
	}, nil
}

func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// nullBoolPtr converts a nullable TINYINT column into *bool (0 -> false,
// 1 -> true); NULL stays nil.
func nullBoolPtr(n sql.NullInt64) *bool {
	if !n.Valid {
		return nil
	}
	v := n.Int64 != 0
	return &v
}

// nullFloat64Ptr converts a nullable DOUBLE column into *float64; NULL
// stays nil.
func nullFloat64Ptr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func nullInt16Ptr(n sql.NullInt64) *int16 {
	if !n.Valid {
		return nil
	}
	v := int16(n.Int64)
	return &v
}

func nullStringPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	v := n.String
	return &v
}

// nullCostAmountPtr converts a nullable fixed-point cost column into the
// currency amount carried by ireport.LogRow; NULL stays nil.
func nullCostAmountPtr(n sql.NullInt64) *float64 {
	if !n.Valid {
		return nil
	}
	v := ireport.CostFixedPointToAmount(n.Int64)
	return &v
}

// scanLogRow scans one detail projection row (same shape as the MySQL
// backend; the two tables share column names by design).
func scanLogRow(scanner rowScanner) (*ireport.LogRow, error) {
	var (
		logid               sql.NullInt64
		logTime             sql.NullInt64
		hostid              sql.NullString
		product             sql.NullString
		apiKeyID            sql.NullString
		requestedModel      sql.NullString
		targetModel         sql.NullString
		provider            sql.NullString
		protocol            sql.NullString
		mode                sql.NullString
		stream              sql.NullInt64
		statusCode          sql.NullInt64
		errCode             sql.NullString
		errMsg              sql.NullString
		inputTokens         sql.NullInt64
		outputTokens        sql.NullInt64
		totalTokens         sql.NullInt64
		allTime             sql.NullInt64
		ttftUs              sql.NullInt64
		tpotUs              sql.NullInt64
		costValue           sql.NullInt64
		costCurrency        sql.NullString
		rateLimitHits       sql.NullString
		authRejectQuotaPlan sql.NullString
		level1Name          sql.NullString
		level1              sql.NullString
		level2Name          sql.NullString
		level2              sql.NullString
		level3Name          sql.NullString
		level3              sql.NullString
		level4Name          sql.NullString
		level4              sql.NullString
		level5Name          sql.NullString
		level5              sql.NullString
		clientIP            sql.NullString
		headerHost          sql.NullString
		originURI           sql.NullString
		reqHeaders          sql.NullString
		resHeaders          sql.NullString
		aiCacheStatus       sql.NullString
		mirrorHit           sql.NullInt64
		mirrorCluster       sql.NullString
		intentQuestion      sql.NullString
		intentAnswer        sql.NullString
		intentConfidence    sql.NullFloat64
		intentSource        sql.NullString
		intentLatencyUs     sql.NullInt64
		intentCacheHit      sql.NullInt64
		intentQuestionsVer  sql.NullString
	)

	err := scanner.Scan(
		&logid,
		&logTime,
		&hostid,
		&product,
		&apiKeyID,
		&requestedModel,
		&targetModel,
		&provider,
		&protocol,
		&mode,
		&stream,
		&statusCode,
		&errCode,
		&errMsg,
		&inputTokens,
		&outputTokens,
		&totalTokens,
		&allTime,
		&ttftUs,
		&tpotUs,
		&costValue,
		&costCurrency,
		&rateLimitHits,
		&authRejectQuotaPlan,
		&level1Name,
		&level1,
		&level2Name,
		&level2,
		&level3Name,
		&level3,
		&level4Name,
		&level4,
		&level5Name,
		&level5,
		&clientIP,
		&headerHost,
		&originURI,
		&reqHeaders,
		&resHeaders,
		&aiCacheStatus,
		&mirrorHit,
		&mirrorCluster,
		&intentQuestion,
		&intentAnswer,
		&intentConfidence,
		&intentSource,
		&intentLatencyUs,
		&intentCacheHit,
		&intentQuestionsVer,
	)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	row := &ireport.LogRow{
		LogID:               nullInt64Ptr(logid),
		LogTime:             logTime.Int64,
		Hostid:              nullStringPtr(hostid),
		Product:             nullStringPtr(product),
		APIKeyID:            nullStringPtr(apiKeyID),
		RequestedModel:      nullStringPtr(requestedModel),
		TargetModel:         nullStringPtr(targetModel),
		Provider:            nullStringPtr(provider),
		Protocol:            nullStringPtr(protocol),
		Mode:                nullStringPtr(mode),
		Stream:              nullInt16Ptr(stream),
		StatusCode:          nullInt16Ptr(statusCode),
		ErrCode:             nullStringPtr(errCode),
		ErrMsg:              nullStringPtr(errMsg),
		InputTokens:         nullInt64Ptr(inputTokens),
		OutputTokens:        nullInt64Ptr(outputTokens),
		TotalTokens:         nullInt64Ptr(totalTokens),
		AllTime:             nullInt64Ptr(allTime),
		TTFTUs:              nullInt64Ptr(ttftUs),
		TPOTUs:              nullInt64Ptr(tpotUs),
		CostValue:           nullCostAmountPtr(costValue),
		CostCurrency:        nullStringPtr(costCurrency),
		RateLimitHits:       nullStringPtr(rateLimitHits),
		AuthRejectQuotaPlan: nullStringPtr(authRejectQuotaPlan),
		Level1Name:          nullStringPtr(level1Name),
		Level1:              nullStringPtr(level1),
		Level2Name:          nullStringPtr(level2Name),
		Level2:              nullStringPtr(level2),
		Level3Name:          nullStringPtr(level3Name),
		Level3:              nullStringPtr(level3),
		Level4Name:          nullStringPtr(level4Name),
		Level4:              nullStringPtr(level4),
		Level5Name:          nullStringPtr(level5Name),
		Level5:              nullStringPtr(level5),
		ClientIP:            nullStringPtr(clientIP),
		HeaderHost:          nullStringPtr(headerHost),
		OriginURI:           nullStringPtr(originURI),
		ReqHeaders:          nullStringPtr(reqHeaders),
		ResHeaders:          nullStringPtr(resHeaders),

		AICacheStatus:        nullStringPtr(aiCacheStatus),
		MirrorHit:            nullBoolPtr(mirrorHit),
		MirrorCluster:        nullStringPtr(mirrorCluster),
		AIIntentQuestion:     nullStringPtr(intentQuestion),
		AIIntentAnswer:       nullStringPtr(intentAnswer),
		AIIntentConfidence:   nullFloat64Ptr(intentConfidence),
		AIIntentSource:       nullStringPtr(intentSource),
		AIIntentLatencyUs:    nullInt64Ptr(intentLatencyUs),
		AIIntentCacheHit:     nullBoolPtr(intentCacheHit),
		AIIntentQuestionsVer: nullStringPtr(intentQuestionsVer),
	}
	return row, nil
}
