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
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockDB(t *testing.T) (*sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	return &mock, func() { db.Close() }
}

var selectGetLock = "SELECT GET_LOCK(?, 0)"

func TestBuildAggregateInsertSQL_Structure(t *testing.T) {
	query := buildAggregateInsertSQL("bfe_ai_request_log", "bfe_ai_metrics_1m")

	// The explicit column list must match the DDL column order
	// (db_ddl_report_mysql.sql), and every SELECT alias must appear in the
	// same order.
	require.True(t, strings.HasPrefix(query, "INSERT INTO bfe_ai_metrics_1m ("))
	cols := strings.Split(aggregateInsertColumns[2:len(aggregateInsertColumns)-1], ",")
	require.Len(t, cols, 64) // 40 dimensions + 24 metrics

	selectPart := query[strings.Index(query, " SELECT"):]
	aliasRe := regexp.MustCompile(` AS ([A-Za-z0-9_]+)[, ]`)
	aliases := aliasRe.FindAllStringSubmatch(selectPart, -1)
	require.Len(t, aliases, len(cols))
	for i, col := range cols {
		assert.Equal(t, col, aliases[i][1], "column %d (%s)", i, col)
	}

	assert.Contains(t, query, "FROM bfe_ai_request_log WHERE log_time >= ? AND log_time < ?")
	assert.True(t, strings.HasSuffix(query, aggregateGroupBy))
	// JSON flattening of the rate-limit triple and quota-plan slots.
	assert.Contains(t, query, "JSON_EXTRACT(ai_rate_limit_hits,'$[0].rate_limit_policy_id')")
	assert.Contains(t, query, "JSON_EXTRACT(ai_auth_reject_quota_plans,'$[4]')")
	// 缓存/镜像/意图维度归一与 GROUP BY（2026-09-27 加列）。
	assert.Contains(t, query, "IFNULL(ai_cache_status,'') AS ai_cache_status")
	assert.Contains(t, query, "IFNULL(mirror_hit,0) AS mirror_hit")
	assert.Contains(t, query, "IFNULL(ai_intent_answer,'') AS ai_intent_answer")
	assert.True(t, strings.HasSuffix(aggregateGroupBy, "ai_cache_status,mirror_hit,ai_intent_answer"))
}

// TestBuildAggregateInsertSQL_CacheMirrorIntentDimensions 锁定新维度的聚合
// 口径：列清单、SELECT 归一表达式与 GROUP BY 三处同序对齐（与 DDL 列序一致）。
func TestBuildAggregateInsertSQL_CacheMirrorIntentDimensions(t *testing.T) {
	cols := strings.Split(aggregateInsertColumns[2:len(aggregateInsertColumns)-1], ",")
	detailCount := len(cols) - 24 // 40
	groupCols := strings.Split(strings.TrimPrefix(aggregateGroupBy, " GROUP BY "), ",")
	require.Len(t, groupCols, detailCount)
	assert.Equal(t, cols[:detailCount], groupCols, "GROUP BY 必须与维度列清单同序一致")

	// 三个新维度位于维度段末尾（metrics 段之前）。
	assert.Equal(t, []string{"ai_cache_status", "mirror_hit", "ai_intent_answer"}, cols[detailCount-3:detailCount])
}

func TestBuildAggregateDeleteSQL(t *testing.T) {
	assert.Equal(t, "DELETE FROM bfe_ai_metrics_1m WHERE ts_min = ?", buildAggregateDeleteSQL("bfe_ai_metrics_1m"))
	assert.Equal(t, "DELETE FROM r.bfe_ai_metrics_1m WHERE ts_min = ?", buildAggregateDeleteSQL("r.bfe_ai_metrics_1m"))
}

func TestRunAggregateOnce_WindowBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		now    time.Time
		window [2]time.Time
	}{
		{
			"mid minute",
			time.Date(2026, 9, 15, 10, 23, 45, 0, time.UTC),
			[2]time.Time{
				time.Date(2026, 9, 15, 10, 22, 0, 0, time.UTC),
				time.Date(2026, 9, 15, 10, 23, 0, 0, time.UTC),
			},
		},
		{
			"cross day",
			time.Date(2026, 9, 15, 0, 0, 30, 0, time.UTC),
			[2]time.Time{
				time.Date(2026, 9, 14, 23, 59, 0, 0, time.UTC),
				time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			"exact minute start",
			time.Date(2026, 9, 15, 10, 24, 0, 0, time.UTC),
			[2]time.Time{
				time.Date(2026, 9, 15, 10, 23, 0, 0, time.UTC),
				time.Date(2026, 9, 15, 10, 24, 0, 0, time.UTC),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectQuery(selectGetLock).WithArgs(aggLockName).
				WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK(?, 0)"}).AddRow(1))
			mock.ExpectBegin()
			mock.ExpectExec(buildAggregateDeleteSQL("bfe_ai_metrics_1m")).
				WithArgs(tc.window[0]).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(buildAggregateInsertSQL("bfe_ai_request_log", "bfe_ai_metrics_1m")).
				WithArgs(tc.window[0], tc.window[1]).WillReturnResult(sqlmock.NewResult(0, 5))
			mock.ExpectCommit()
			mock.ExpectExec("SELECT RELEASE_LOCK(?)").WithArgs(aggLockName).
				WillReturnResult(sqlmock.NewResult(0, 0))

			job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time { return tc.now }))
			require.NoError(t, job.RunAggregateOnce(context.Background()))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRunAggregateOnce_LockNotAcquired(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(selectGetLock).WithArgs(aggLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time {
		return time.Date(2026, 9, 15, 10, 23, 45, 0, time.UTC)
	}))
	require.NoError(t, job.RunAggregateOnce(context.Background()))
	// No DELETE/INSERT is executed: the expectations above are exhaustive.
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunAggregateOnce_LockNull(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(selectGetLock).WithArgs(aggLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(nil))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time {
		return time.Date(2026, 9, 15, 10, 23, 45, 0, time.UTC)
	}))
	require.NoError(t, job.RunAggregateOnce(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunAggregateOnce_DeleteErrorRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	windowStart := time.Date(2026, 9, 15, 10, 22, 0, 0, time.UTC)
	mock.ExpectQuery(selectGetLock).WithArgs(aggLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec(buildAggregateDeleteSQL("bfe_ai_metrics_1m")).
		WithArgs(windowStart).WillReturnError(context.DeadlineExceeded)
	mock.ExpectRollback()
	mock.ExpectExec("SELECT RELEASE_LOCK(?)").WithArgs(aggLockName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time {
		return time.Date(2026, 9, 15, 10, 23, 45, 0, time.UTC)
	}))
	require.Error(t, job.RunAggregateOnce(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func partitionRows(cols ...[]interface{}) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"PARTITION_NAME", "PARTITION_DESCRIPTION"})
	for _, r := range cols {
		rows.AddRow(r[0], r[1])
	}
	return rows
}

func TestRunPartitionMgmtOnce_AddAndDrop(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 9, 18, 3, 30, 0, 0, time.UTC)
	// detail table: init covers up to 09-18, named partitions up to 09-19;
	// metrics table: same layout.
	mock.ExpectQuery(selectGetLock).WithArgs(partitionLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	for _, table := range []string{"bfe_ai_request_log", "bfe_ai_metrics_1m"} {
		mock.ExpectQuery(listPartitionsSQL).WithArgs(table).WillReturnRows(partitionRows(
			[]interface{}{"p_init", "TO_DAYS('2026-09-18')"},
			[]interface{}{"p20260917", "TO_DAYS('2026-09-18')"},
			[]interface{}{"p20260918", "TO_DAYS('2026-09-19')"},
		))
		// 09-19 and 09-20 partitions are missing (boundary must exceed 09-19).
		mock.ExpectExec("ALTER TABLE " + table + " ADD PARTITION (PARTITION p20260919 VALUES LESS THAN (TO_DAYS('2026-09-20')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE " + table + " ADD PARTITION (PARTITION p20260920 VALUES LESS THAN (TO_DAYS('2026-09-21')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// retention 7: cutoff = 09-12; nothing here is expired.
	}
	mock.ExpectExec("SELECT RELEASE_LOCK(?)").WithArgs(partitionLockName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time { return now }))
	require.NoError(t, job.RunPartitionMgmtOnce(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunPartitionMgmtOnce_DropsExpired(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(selectGetLock).WithArgs(partitionLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	for _, table := range []string{"bfe_ai_request_log", "bfe_ai_metrics_1m"} {
		mock.ExpectQuery(listPartitionsSQL).WithArgs(table).WillReturnRows(partitionRows(
			[]interface{}{"p_init", "TO_DAYS('2026-09-13')"},
			[]interface{}{"p20260913", "TO_DAYS('2026-09-14')"},
			[]interface{}{"p20260919", "TO_DAYS('2026-09-20')"},
		))
		// boundaries 09-20..: adds for 09-20, 09-21, 09-22.
		mock.ExpectExec("ALTER TABLE " + table + " ADD PARTITION (PARTITION p20260920 VALUES LESS THAN (TO_DAYS('2026-09-21')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE " + table + " ADD PARTITION (PARTITION p20260921 VALUES LESS THAN (TO_DAYS('2026-09-22')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE " + table + " ADD PARTITION (PARTITION p20260922 VALUES LESS THAN (TO_DAYS('2026-09-23')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// retention 7: cutoff 09-14; p_init (boundary 09-13) and p20260913
		// (boundary 09-14) are expired; p20260919 stays.
		mock.ExpectExec("ALTER TABLE " + table + " DROP PARTITION p_init").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE " + table + " DROP PARTITION p20260913").
			WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("SELECT RELEASE_LOCK(?)").WithArgs(partitionLockName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time { return now }))
	require.NoError(t, job.RunPartitionMgmtOnce(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunPartitionMgmtOnce_NonPartitionedPurgesInBatches(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	cutoff := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(selectGetLock).WithArgs(partitionLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	for _, tc := range []struct {
		table   string
		timeCol string
	}{
		{"bfe_ai_request_log", "log_time"},
		{"bfe_ai_metrics_1m", "ts_min"},
	} {
		mock.ExpectQuery(listPartitionsSQL).WithArgs(tc.table).
			WillReturnRows(sqlmock.NewRows([]string{"PARTITION_NAME", "PARTITION_DESCRIPTION"}))
		// one full batch, then an empty batch stops the loop
		mock.ExpectExec(buildPurgeSQL(tc.table, tc.timeCol, defaultDeleteBatchSize)).
			WithArgs(cutoff).WillReturnResult(sqlmock.NewResult(0, int64(defaultDeleteBatchSize)))
		mock.ExpectExec(buildPurgeSQL(tc.table, tc.timeCol, defaultDeleteBatchSize)).
			WithArgs(cutoff).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	mock.ExpectExec("SELECT RELEASE_LOCK(?)").WithArgs(partitionLockName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time { return now }))
	require.NoError(t, job.RunPartitionMgmtOnce(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRunPartitionMgmtOnce_QualifiedDatabase 回归 issue #191：配置
// [Report].Database 后 j.table() 生成 schema 限定名，而 information_schema
// 的 TABLE_NAME 只存裸表名，分区查询恒返回 0 行，JOB 误判表未分区、静默
// 走 DELETE 兜底，永不 ADD PARTITION（log-reader 写入 Error 1526）。
// 该用例锁定：元数据查询以 (schema, 裸表名) 过滤、DDL 使用限定名、且
// 不出现 DELETE 兜底语句。
func TestRunPartitionMgmtOnce_QualifiedDatabase(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, 9, 21, 3, 30, 0, 0, time.UTC)
	const schema = "open_bfe_222"

	mock.ExpectQuery(selectGetLock).WithArgs(partitionLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(1))
	for _, table := range []string{"bfe_ai_request_log", "bfe_ai_metrics_1m"} {
		// information_schema 过滤参数为 (schema, 裸表名)；p_init 边界
		// 740244 = TO_DAYS('2026-09-20')，前瞻窗口 09-21..09-23 缺失。
		mock.ExpectQuery(listPartitionsInSchemaSQL).WithArgs(schema, table).WillReturnRows(partitionRows(
			[]interface{}{"p_init", "740244"},
		))
		mock.ExpectExec("ALTER TABLE " + schema + "." + table + " ADD PARTITION (PARTITION p20260921 VALUES LESS THAN (TO_DAYS('2026-09-22')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE " + schema + "." + table + " ADD PARTITION (PARTITION p20260922 VALUES LESS THAN (TO_DAYS('2026-09-23')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("ALTER TABLE " + schema + "." + table + " ADD PARTITION (PARTITION p20260923 VALUES LESS THAN (TO_DAYS('2026-09-24')))").
			WillReturnResult(sqlmock.NewResult(0, 0))
		// retention 7：cutoff 09-15，p_init（边界 09-20）保留。
	}
	mock.ExpectExec("SELECT RELEASE_LOCK(?)").WithArgs(partitionLockName).
		WillReturnResult(sqlmock.NewResult(0, 0))

	job := NewJob(db, schema, time.Minute, 7, WithNowFunc(func() time.Time { return now }))
	require.NoError(t, job.RunPartitionMgmtOnce(context.Background()))
	// 无 DELETE 兜底：上述期望是穷尽的。
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunPartitionMgmtOnce_LockNotAcquired(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(selectGetLock).WithArgs(partitionLockName).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time {
		return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	}))
	require.NoError(t, job.RunPartitionMgmtOnce(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestJobStartStop(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer db.Close()

	// Both startup cycles skip because the lock is never acquired. AnyArgs
	// keeps the test insensitive to which goroutine queries first.
	mock.ExpectQuery(selectGetLock).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))
	mock.ExpectQuery(selectGetLock).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"lock"}).AddRow(0))

	job := NewJob(db, "", time.Minute, 7, WithNowFunc(func() time.Time {
		return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	}))
	job.Start()
	job.Stop()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPlanPartitionAdds(t *testing.T) {
	now := time.Date(2026, 9, 18, 15, 4, 5, 0, time.UTC)
	today := time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)

	// no partitions at all: create the full ahead window.
	days := planPartitionAdds(now, 3, nil)
	require.Len(t, days, 3)
	assert.Equal(t, today, days[0])
	assert.Equal(t, today.AddDate(0, 0, 1), days[1])
	assert.Equal(t, today.AddDate(0, 0, 2), days[2])

	// boundary far ahead covers everything.
	days = planPartitionAdds(now, 3, []time.Time{today.AddDate(0, 0, 10)})
	assert.Empty(t, days)

	// boundary at today+1 covers today only.
	days = planPartitionAdds(now, 3, []time.Time{today.AddDate(0, 0, 1)})
	require.Len(t, days, 2)
	assert.Equal(t, today.AddDate(0, 0, 1), days[0])
	assert.Equal(t, today.AddDate(0, 0, 2), days[1])

	// cross-month window stays ascending.
	endOfMonth := time.Date(2026, 9, 30, 23, 59, 0, 0, time.UTC)
	days = planPartitionAdds(endOfMonth, 3, nil)
	require.Len(t, days, 3)
	assert.Equal(t, "p20260930", "p"+days[0].Format("20060102"))
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), days[1])
}

func TestPlanPartitionDrops(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	parts := []partitionInfo{
		{Name: "p_init", Description: "TO_DAYS('2026-09-13')"},
		{Name: "p20260913", Description: "TO_DAYS('2026-09-14')"},
		{Name: "p20260914", Description: "TO_DAYS('2026-09-15')"},
		{Name: "p20260919", Description: "TO_DAYS('2026-09-20')"},
		{Name: "p20260920", Description: "TO_DAYS('2026-09-21')"},
	}
	names := planPartitionDrops(now, 7, parts)
	// cutoff = 09-14: boundary 09-13 and 09-14 expire; 09-15+ stays.
	assert.Equal(t, []string{"p_init", "p20260913"}, names)

	// single partition is never dropped.
	single := []partitionInfo{{Name: "p_init", Description: "TO_DAYS('2026-09-13')"}}
	assert.Empty(t, planPartitionDrops(now, 7, single))

	// unparseable description is kept.
	weird := []partitionInfo{
		{Name: "p1", Description: "MAXVALUE"},
		{Name: "p2", Description: "TO_DAYS('2026-09-13')"},
	}
	assert.Equal(t, []string{"p2"}, planPartitionDrops(now, 7, weird))

	// disabled retention drops nothing.
	assert.Empty(t, planPartitionDrops(now, 0, parts))
}

func TestParseBoundaryDate(t *testing.T) {
	// 字面文本形态（部分形态/版本回显 TO_DAYS('...') 文本）。
	day, ok := parseBoundaryDate("TO_DAYS('2026-09-18')")
	require.True(t, ok)
	assert.Equal(t, "2026-09-18", day.Format("2006-01-02"))

	day, ok = parseBoundaryDate("'2026-09-18'")
	require.True(t, ok)
	assert.Equal(t, "2026-09-18", day.Format("2006-01-02"))

	// 求值整数形态：MySQL 8.x 对 VALUES LESS THAN (TO_DAYS('...')) 回显
	// TO_DAYS 值（如 738886 = 2023-01-01），需 FROM_DAYS 反解。
	day, ok = parseBoundaryDate("738886")
	require.True(t, ok)
	assert.Equal(t, "2023-01-01", day.Format("2006-01-02"))

	day, ok = parseBoundaryDate("740242")
	require.True(t, ok)
	assert.Equal(t, "2026-09-18", day.Format("2006-01-02"))

	day, ok = parseBoundaryDate("740243")
	require.True(t, ok)
	assert.Equal(t, "2026-09-19", day.Format("2006-01-02"))

	// MAXVALUE：无边界的尾分区，视为不可解析（保守跳过 DROP 决策）。
	_, ok = parseBoundaryDate("MAXVALUE")
	assert.False(t, ok)

	// 非法/过小整数/空：不可解析。
	_, ok = parseBoundaryDate("not_a_number")
	assert.False(t, ok)
	_, ok = parseBoundaryDate("5")
	assert.False(t, ok)
	_, ok = parseBoundaryDate("")
	assert.False(t, ok)
}

// TestPlanPartitionAdds_IntegerDescriptions 回归 SC31 的 Error 1493 噪音：
// information_schema 对 TO_DAYS 分区回显求值整数，若解析失败 JOB 会把现有
// 分区当作缺失并重复 ADD PARTITION（Error 1493）。整数反解后规划必须识别
// 现有分区、不再追加。
func TestPlanPartitionAdds_IntegerDescriptions(t *testing.T) {
	now := time.Date(2026, 9, 18, 3, 30, 0, 0, time.UTC)

	//  ahead 窗口 [09-18, 09-21) 需要边界覆盖到 09-21（740245）。分区全部
	// 以整数形态回显（MySQL 8.x 实测形态）：740242=09-18 … 740245=09-21。
	// 反解正确 → 规划为空，不再重复 ADD（修复前逐周期 Error 1493）。
	parts := []partitionInfo{
		{Name: "p_init", Description: "740242"},
		{Name: "p20260918", Description: "740243"},
		{Name: "p20260919", Description: "740244"},
		{Name: "p20260920", Description: "740245"},
	}
	assert.Empty(t, planPartitionAdds(now, 3, partitionBoundaries(parts)),
		"partitions already cover the ahead window, must not re-add")

	// 混合形态（文本 + 整数 + MAXVALUE 尾分区）同样不误加。
	mixed := []partitionInfo{
		{Name: "p20260918", Description: "TO_DAYS('2026-09-19')"},
		{Name: "p20260919", Description: "740244"},
		{Name: "p20260920", Description: "740245"},
		{Name: "pmax", Description: "MAXVALUE"},
	}
	assert.Empty(t, planPartitionAdds(now, 3, partitionBoundaries(mixed)))

	// 整数反解后 DROP 决策同样生效：740236 = TO_DAYS('2026-09-12') ≤ 保留
	// 下限（cutoff 2026-09-12），p_init 被清理；740243(09-19) 保留。
	dropParts := []partitionInfo{
		{Name: "p_init", Description: "740236"},
		{Name: "p20260919", Description: "740243"},
	}
	assert.Equal(t, []string{"p_init"}, planPartitionDrops(now, 7, dropParts))
}

func TestBuildPartitionSQLs(t *testing.T) {
	day := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	assert.Equal(t,
		"ALTER TABLE bfe_ai_request_log ADD PARTITION (PARTITION p20260919 VALUES LESS THAN (TO_DAYS('2026-09-20')))",
		buildAddPartitionSQL("bfe_ai_request_log", day))
	assert.Equal(t, "ALTER TABLE t DROP PARTITION p20260919", buildDropPartitionSQL("t", "p20260919"))
	assert.Equal(t, "DELETE FROM t WHERE ts_min < ? LIMIT 1000", buildPurgeSQL("t", "ts_min", 1000))
}

func TestRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), retentionCutoff(now, 7))
	assert.Equal(t, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), retentionCutoff(now, 1))
}

func TestJobTablePrefix(t *testing.T) {
	j := NewJob(nil, "r", time.Minute, 7)
	assert.Equal(t, "r.bfe_ai_request_log", j.table(tableDetail))

	j = NewJob(nil, "", time.Minute, 7)
	assert.Equal(t, "bfe_ai_request_log", j.table(tableDetail))
}

// TestBuildAggregateInsertSQL_TTFTStreamCaliber pins the stream-only caliber
// of ttft_us_sum/tpot_us_sum at aggregation time: BFE records first-byte time
// for non-stream responses as well (mod_body_process), so non-stream rows must
// not enter these sums (ai-gateway-api#315).
func TestBuildAggregateInsertSQL_TTFTStreamCaliber(t *testing.T) {
	query := buildAggregateInsertSQL("bfe_ai_request_log", "bfe_ai_metrics_1m")
	assert.Contains(t, query,
		"SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum")
	assert.Contains(t, query,
		"SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_tpot_us,0) ELSE 0 END) AS tpot_us_sum")
	assert.NotContains(t, query, "SUM(IFNULL(ai_ttft_us,0))")
	assert.NotContains(t, query, "SUM(IFNULL(ai_tpot_us,0))")
}
