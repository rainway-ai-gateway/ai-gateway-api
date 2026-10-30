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
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ireport"
)

var (
	testStart = time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	testEnd   = time.Date(2026, 9, 15, 11, 0, 0, 0, time.UTC)
)

func fullFilter() *ireport.Filter {
	stream := true
	return &ireport.Filter{
		Start:       testStart,
		End:         testEnd,
		Models:      []string{"gpt-4o", "gpt-4"},
		ApikeyIDs:   []string{"key-1"},
		Providers:   []string{"openai"},
		Hosts:       []string{"gw-01"},
		Stream:      &stream,
		StatusCodes: []int{200, 500},
	}
}

// assertSnapshot compares the builder output against the expected SQL and
// args. The expected SQL is the verbatim gendry rendering: conditions are
// grouped by operator (=, IN, !=, >=, <, LIKE) and lexicographically sorted
// within a group, and the whole WHERE clause is wrapped in parentheses.
func assertSnapshot(t *testing.T, query string, args []interface{}, err error,
	expectSQL string, expectArgs []interface{}) {
	t.Helper()
	require.NoError(t, err)
	assert.Equal(t, expectSQL, query)
	assert.Equal(t, expectArgs, args)
}

func fullFilterArgs() []interface{} {
	return []interface{}{
		int8(1), "key-1", "openai", "gpt-4o", "gpt-4", "gw-01", 200, 500,
		testStart, testEnd,
	}
}

const fullWhere = " WHERE (ai_stream=? AND ai_apikey_id IN (?) AND ai_provider IN (?)" +
	" AND ai_target_model IN (?,?) AND hostid IN (?) AND res_status_code IN (?,?)" +
	" AND ts_min>=? AND ts_min<?)"

const overviewFields = "SELECT IFNULL(SUM(request_count),0) AS request_total," +
	"IFNULL(SUM(error_count),0) AS error_total," +
	"IFNULL(SUM(input_tokens),0) AS input_tokens," +
	"IFNULL(SUM(output_tokens),0) AS output_tokens," +
	"IFNULL(SUM(total_tokens),0) AS total_tokens," +
	"IFNULL(SUM(all_time_sum),0) AS all_time_sum," +
	"IFNULL(MAX(all_time_sum/request_count),0) AS latency_max," +
	"IFNULL(SUM(ttft_us_sum),0) AS ttft_us_sum," +
	"IFNULL(SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END),0) AS stream_requests," +
	"IFNULL(SUM(tpot_us_sum),0) AS tpot_us_sum," +
	"IFNULL(SUM(rate_limit_hits),0) AS rate_limit_hits," +
	"IFNULL(SUM(auth_reject_count),0) AS auth_rejects," +
	"IFNULL(SUM(cache_read_tokens),0) AS cache_read_tokens," +
	"IFNULL(SUM(cache_write_tokens),0) AS cache_write_tokens"

const overviewDetailCountFieldsSQL = "SELECT IFNULL(SUM(CASE WHEN ai_cache_status='hit' THEN 1 ELSE 0 END),0) AS cache_hit_count," +
	"IFNULL(SUM(CASE WHEN ai_cache_status='miss' THEN 1 ELSE 0 END),0) AS cache_miss_count," +
	"IFNULL(SUM(CASE WHEN ai_cache_status='skip' THEN 1 ELSE 0 END),0) AS cache_skip_count," +
	"IFNULL(SUM(CASE WHEN IFNULL(mirror_hit,0)=1 THEN 1 ELSE 0 END),0) AS mirror_hit_count," +
	"IFNULL(SUM(CASE WHEN IFNULL(ai_intent_answer,'')!='' AND ai_intent_answer!='unknown' THEN 1 ELSE 0 END),0) AS intent_classified_count," +
	"IFNULL(SUM(CASE WHEN ai_intent_answer='unknown' THEN 1 ELSE 0 END),0) AS intent_unknown_count"

func TestBuildOverviewMetricsSQL_FullFilter(t *testing.T) {
	query, args, err := buildOverviewMetricsSQL("bfe_ai_metrics_1m", fullFilter())

	assertSnapshot(t, query, args, err,
		overviewFields+" FROM bfe_ai_metrics_1m"+fullWhere, fullFilterArgs())
}

func TestBuildOverviewMetricsSQL_TimeOnly(t *testing.T) {
	query, args, err := buildOverviewMetricsSQL("bfe_ai_metrics_1m", &ireport.Filter{Start: testStart, End: testEnd})

	require.NoError(t, err)
	assert.Equal(t,
		overviewFields+" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?)",
		query)
	assert.Equal(t, []interface{}{testStart, testEnd}, args)
}

func TestBuildOverviewCostSQL(t *testing.T) {
	query, args, err := buildOverviewCostSQL("bfe_ai_metrics_1m", fullFilter())

	assertSnapshot(t, query, args, err,
		"SELECT ai_cost_currency AS currency,IFNULL(SUM(ai_cost_value_sum),0) AS value"+
			" FROM bfe_ai_metrics_1m"+
			" WHERE (ai_stream=? AND ai_apikey_id IN (?) AND ai_provider IN (?)"+
			" AND ai_target_model IN (?,?) AND hostid IN (?) AND res_status_code IN (?,?)"+
			" AND IFNULL(ai_cost_currency,'')!=? AND ts_min>=? AND ts_min<?)"+
			" GROUP BY ai_cost_currency",
		append(fullFilterArgs()[:8], append([]interface{}{""}, fullFilterArgs()[8:]...)...))
}

func TestBuildLogsTotalSQL(t *testing.T) {
	query, args, err := buildLogsTotalSQL("bfe_ai_request_log", fullFilter())

	assertSnapshot(t, query, args, err,
		"SELECT COUNT(*) FROM bfe_ai_request_log"+
			" WHERE (ai_stream=? AND ai_apikey_id IN (?) AND ai_provider IN (?)"+
			" AND ai_target_model IN (?,?) AND hostid IN (?) AND res_status_code IN (?,?)"+
			" AND log_time>=? AND log_time<?)",
		fullFilterArgs())
}

func TestBuildOverviewDetailCountsSQL(t *testing.T) {
	query, args, err := buildOverviewDetailCountsSQL("bfe_ai_request_log", fullFilter())

	assertSnapshot(t, query, args, err,
		overviewDetailCountFieldsSQL+" FROM bfe_ai_request_log"+
			" WHERE (ai_stream=? AND ai_apikey_id IN (?) AND ai_provider IN (?)"+
			" AND ai_target_model IN (?,?) AND hostid IN (?) AND res_status_code IN (?,?)"+
			" AND log_time>=? AND log_time<?)",
		fullFilterArgs())
}

func TestBuildTimeSeriesSQL_AllMetrics(t *testing.T) {
	f := &ireport.Filter{Start: testStart, End: testEnd}

	cases := []struct {
		metric    string
		bucket    int
		expectSQL string
	}{
		{
			ireport.MetricQPS, 60,
			"SELECT CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/60)*60 AS SIGNED) AS time,SUM(request_count) AS total" +
				" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time ORDER BY time ASC",
		},
		{
			ireport.MetricTokens, 300,
			"SELECT CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/300)*300 AS SIGNED) AS time,SUM(input_tokens) AS input," +
				"SUM(output_tokens) AS output,SUM(total_tokens) AS total" +
				" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time ORDER BY time ASC",
		},
		{
			ireport.MetricLatency, 1800,
			"SELECT CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/1800)*1800 AS SIGNED) AS time,SUM(all_time_sum) AS all_time_sum," +
				"SUM(request_count) AS request_count,MAX(all_time_sum/request_count) AS latency_max" +
				" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time ORDER BY time ASC",
		},
		{
			ireport.MetricTTFT, 60,
			"SELECT CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/60)*60 AS SIGNED) AS time,SUM(ttft_us_sum) AS ttft_us_sum," +
				"SUM(tpot_us_sum) AS tpot_us_sum,SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END) AS stream_requests" +
				" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time ORDER BY time ASC",
		},
		{
			ireport.MetricCost, 60,
			"SELECT CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/60)*60 AS SIGNED) AS time,ai_cost_currency AS currency," +
				"SUM(ai_cost_value_sum) AS value" +
				" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time,ai_cost_currency ORDER BY time ASC",
		},
	}
	for _, tc := range cases {
		t.Run(tc.metric, func(t *testing.T) {
			query, args, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", tc.metric, "", f, tc.bucket)
			assertSnapshot(t, query, args, err, tc.expectSQL, []interface{}{testStart, testEnd})
		})
	}
}

func TestBuildTimeSeriesSQL_InvalidMetric(t *testing.T) {
	_, _, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", "bogus", "", &ireport.Filter{Start: testStart, End: testEnd}, 60)
	require.Error(t, err)

	_, _, err = buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricQPS, "bogus", &ireport.Filter{Start: testStart, End: testEnd}, 60)
	require.Error(t, err)
}

// TestBuildTimeSeriesSQL_CacheTokens 验证 cache_tokens 的 UNION ALL 形态：
// cache_read/cache_write 两臂各带字面 kind，按 time,kind 排序。
func TestBuildTimeSeriesSQL_CacheTokens(t *testing.T) {
	f := &ireport.Filter{Start: testStart, End: testEnd}

	query, args, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricCacheTokens, "", f, 60)
	require.NoError(t, err)

	bucket := "CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/60)*60 AS SIGNED) AS time"
	expect := "SELECT " + bucket + ",'cache_read' AS kind,SUM(cache_read_tokens) AS value" +
		" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time" +
		" UNION ALL " +
		"SELECT " + bucket + ",'cache_write' AS kind,SUM(cache_write_tokens) AS value" +
		" FROM bfe_ai_metrics_1m WHERE (ts_min>=? AND ts_min<?) GROUP BY time" +
		" ORDER BY time ASC,kind ASC"
	assert.Equal(t, expect, query)
	assert.Equal(t, []interface{}{testStart, testEnd, testStart, testEnd}, args)
}

// TestBuildTimeSeriesSQL_Dimension 验证可选 dimension 参数：name 字段 +
// GROUP BY time,<维度列>。
func TestBuildTimeSeriesSQL_Dimension(t *testing.T) {
	f := &ireport.Filter{Start: testStart, End: testEnd}

	cases := []struct {
		dimension string
		column    string
	}{
		{ireport.DimensionCacheStatus, "ai_cache_status"},
		{ireport.DimensionMirrorHit, "mirror_hit"},
		{ireport.DimensionIntentAnswer, "ai_intent_answer"},
	}
	for _, tc := range cases {
		t.Run(tc.dimension, func(t *testing.T) {
			query, args, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricQPS, tc.dimension, f, 60)
			require.NoError(t, err)
			assert.Contains(t, query, "CAST(FLOOR(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)/60)*60 AS SIGNED) AS time,")
			assert.Contains(t, query, tc.column+" AS name,SUM(request_count) AS total")
			assert.Contains(t, query, " GROUP BY time,"+tc.column+" ORDER BY time ASC")
			assert.Equal(t, []interface{}{testStart, testEnd}, args)
		})
	}

	// cache_tokens × dimension：UNION ALL 两臂各带 name 并按维度列、kind 排序。
	query, _, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricCacheTokens, ireport.DimensionCacheStatus, f, 60)
	require.NoError(t, err)
	assert.Contains(t, query, "'cache_read' AS kind,ai_cache_status AS name,SUM(cache_read_tokens) AS value")
	assert.Contains(t, query, "'cache_write' AS kind,ai_cache_status AS name,SUM(cache_write_tokens) AS value")
	assert.Contains(t, query, " GROUP BY time,ai_cache_status UNION ALL ")
	assert.Contains(t, query, " ORDER BY time ASC,ai_cache_status ASC,kind ASC")

	// cost × dimension：GROUP BY time,ai_cost_currency,<维度列>。
	query, _, err = buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricCost, ireport.DimensionIntentAnswer, f, 60)
	require.NoError(t, err)
	assert.Contains(t, query, "ai_intent_answer AS name,ai_cost_currency AS currency,SUM(ai_cost_value_sum) AS value")
	assert.Contains(t, query, " GROUP BY time,ai_cost_currency,ai_intent_answer ORDER BY time ASC")
}

func TestBuildRankingsSQL(t *testing.T) {
	cases := []struct {
		dimension string
		expectSel string
		expectGrp string
	}{
		{ireport.DimensionModel, "SELECT ai_target_model AS name,", " GROUP BY ai_target_model "},
		{ireport.DimensionRequestedModel, "SELECT ai_requested_model AS name,", " GROUP BY ai_requested_model "},
		{ireport.DimensionProvider, "SELECT ai_provider AS name,", " GROUP BY ai_provider "},
		{ireport.DimensionAPIKey, "SELECT ai_apikey_id AS name,", " GROUP BY ai_apikey_id "},
		{ireport.DimensionHost, "SELECT hostid AS name,", " GROUP BY hostid "},
		{ireport.DimensionStatus, "SELECT CAST(res_status_code AS CHAR) AS name,", " GROUP BY res_status_code "},
		{ireport.DimensionProtocol, "SELECT ai_protocol AS name,", " GROUP BY ai_protocol "},
		{ireport.DimensionMode, "SELECT ai_mode AS name,", " GROUP BY ai_mode "},
		{ireport.DimensionCacheStatus, "SELECT ai_cache_status AS name,", " GROUP BY ai_cache_status "},
		{ireport.DimensionIntentAnswer, "SELECT ai_intent_answer AS name,", " GROUP BY ai_intent_answer "},
		{ireport.DimensionMirrorHit, "SELECT CAST(mirror_hit AS CHAR) AS name,", " GROUP BY mirror_hit "},
	}
	for _, tc := range cases {
		t.Run(tc.dimension, func(t *testing.T) {
			query, args, err := buildRankingsSQL("bfe_ai_metrics_1m", tc.dimension, &ireport.Filter{Start: testStart, End: testEnd}, 10)
			require.NoError(t, err)
			assert.Contains(t, query, tc.expectSel)
			if tc.dimension == ireport.DimensionMirrorHit {
				// mirror_hit is numeric 0/1 without an empty marker: both
				// buckets must rank, so no IFNULL exclusion predicate/arg.
				assert.NotContains(t, query, "IFNULL(mirror_hit")
				assert.Equal(t, []interface{}{testStart, testEnd, 0, 10}, args)
			} else {
				assert.Contains(t, query, "IFNULL(")
				assert.Equal(t, []interface{}{"", testStart, testEnd, 0, 10}, args)
			}
			assert.Contains(t, query, tc.expectGrp)
			assert.Contains(t, query, " ORDER BY request_count DESC LIMIT ?,?")
		})
	}
}

func TestBuildRankingsSQL_InvalidDimension(t *testing.T) {
	_, _, err := buildRankingsSQL("bfe_ai_metrics_1m", "bogus", &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.Error(t, err)
}

func TestBuildDistributionSQL(t *testing.T) {
	cases := []struct {
		dimension string
		expectSel string
	}{
		{ireport.DimensionStatus, "SELECT CASE WHEN res_status_code=0 THEN 'unknown' ELSE CAST(res_status_code AS CHAR) END AS name,"},
		{ireport.DimensionProtocol, "SELECT CASE WHEN IFNULL(ai_protocol,'')='' THEN 'unknown' ELSE ai_protocol END AS name,"},
		{ireport.DimensionMode, "SELECT CASE WHEN IFNULL(ai_mode,'')='' THEN 'unknown' ELSE ai_mode END AS name,"},
		{ireport.DimensionStream, "SELECT CAST(ai_stream AS CHAR) AS name,"},
		{ireport.DimensionCacheStatus, "SELECT CASE WHEN IFNULL(ai_cache_status,'')='' THEN 'unknown' ELSE ai_cache_status END AS name,"},
		{ireport.DimensionIntentAnswer, "SELECT CASE WHEN IFNULL(ai_intent_answer,'')='' THEN 'unknown' ELSE ai_intent_answer END AS name,"},
		{ireport.DimensionMirrorHit, "SELECT CAST(mirror_hit AS CHAR) AS name,"},
	}
	for _, tc := range cases {
		t.Run(tc.dimension, func(t *testing.T) {
			query, args, err := buildDistributionSQL("bfe_ai_metrics_1m", tc.dimension, &ireport.Filter{Start: testStart, End: testEnd})
			require.NoError(t, err)
			assert.Contains(t, query, tc.expectSel)
			assert.Contains(t, query, " GROUP BY name ORDER BY request_count DESC")
			assert.Equal(t, []interface{}{testStart, testEnd}, args)
		})
	}
}

func TestBuildDistributionSQL_InvalidDimension(t *testing.T) {
	_, _, err := buildDistributionSQL("bfe_ai_metrics_1m", "model", &ireport.Filter{Start: testStart, End: testEnd})
	require.Error(t, err)
}

func TestBuildLogsCountSQL_WithLogFilters(t *testing.T) {
	f := &ireport.LogFilter{
		Filter:          *fullFilter(),
		RequestedModels: []string{"gpt-4"},
		ErrOnly:         true,
		Keyword:         "timeout",
		Page:            2,
		PageSize:        20,
	}
	query, args, err := buildLogsCountSQL("bfe_ai_request_log", f)

	require.NoError(t, err)
	assert.Equal(t, "SELECT COUNT(*) FROM bfe_ai_request_log"+
		" WHERE (ai_stream=? AND ai_apikey_id IN (?) AND ai_provider IN (?) AND ai_requested_model IN (?)"+
		" AND ai_target_model IN (?,?) AND hostid IN (?) AND res_status_code IN (?,?)"+
		" AND IFNULL(err_code,'')!=? AND log_time>=? AND log_time<? AND err_msg LIKE ?)", query)
	assert.Equal(t, []interface{}{
		int8(1), "key-1", "openai", "gpt-4", "gpt-4o", "gpt-4", "gw-01", 200, 500,
		"", testStart, testEnd, "%timeout%",
	}, args)
}

// TestBuildLogsCountSQL_CacheMirrorIntentFilters 验证五个新过滤参数的
// WHERE 渲染（精确匹配，全填充时按运算符分组字典序）。
func TestBuildLogsCountSQL_CacheMirrorIntentFilters(t *testing.T) {
	cacheStatus := "hit"
	mirrorHit := true
	intentQuestion := "task_type"
	intentAnswer := "unknown"
	intentSource := "classifier"
	f := &ireport.LogFilter{
		Filter:         ireport.Filter{Start: testStart, End: testEnd},
		CacheStatus:    &cacheStatus,
		MirrorHit:      &mirrorHit,
		IntentQuestion: &intentQuestion,
		IntentAnswer:   &intentAnswer,
		IntentSource:   &intentSource,
	}
	query, args, err := buildLogsCountSQL("bfe_ai_request_log", f)

	require.NoError(t, err)
	assert.Equal(t, "SELECT COUNT(*) FROM bfe_ai_request_log"+
		" WHERE (ai_cache_status=? AND ai_intent_answer=? AND ai_intent_question=? AND ai_intent_source=?"+
		" AND mirror_hit=? AND log_time>=? AND log_time<?)", query)
	assert.Equal(t, []interface{}{"hit", "unknown", "task_type", "classifier", int8(1), testStart, testEnd}, args)

	// mirror_hit=false 渲染 0；未填充的参数不进 WHERE。
	mirrorHit = false
	f.CacheStatus = nil
	f.IntentQuestion = nil
	f.IntentAnswer = nil
	f.IntentSource = nil
	query, args, err = buildLogsCountSQL("bfe_ai_request_log", f)
	require.NoError(t, err)
	assert.Contains(t, query, "mirror_hit=?")
	assert.NotContains(t, query, "ai_cache_status")
	assert.NotContains(t, query, "ai_intent_answer")
	assert.Equal(t, []interface{}{int8(0), testStart, testEnd}, args)
}

func TestBuildLogsSQL_Pagination(t *testing.T) {
	cases := []struct {
		name   string
		page   int
		size   int
		offset int
	}{
		{"first page", 1, 20, 0},
		{"third page", 3, 50, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &ireport.LogFilter{
				Filter:   ireport.Filter{Start: testStart, End: testEnd},
				Page:     tc.page,
				PageSize: tc.size,
			}
			query, args, err := buildLogsSQL("bfe_ai_request_log", f)

			require.NoError(t, err)
			assert.Contains(t, query, " ORDER BY log_time DESC LIMIT ?,?")
			assert.Equal(t, []interface{}{testStart, testEnd, tc.offset, tc.size}, args)
		})
	}
}

func TestBuildLogsSQL_Projection(t *testing.T) {
	f := &ireport.LogFilter{
		Filter:   ireport.Filter{Start: testStart, End: testEnd},
		Page:     1,
		PageSize: 20,
	}
	query, _, err := buildLogsSQL("bfe_ai_request_log", f)
	require.NoError(t, err)
	assert.Contains(t, query, "TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', log_time) AS log_time")
	assert.Contains(t, query, "ai_auth_reject_quota_plans")
	assert.Contains(t, query, "req_headers")
	assert.Contains(t, query, "res_headers")
	// 缓存/镜像/意图 10 列投影，列序与 db_ddl_report_mysql.sql 一致。
	assert.True(t, strings.HasSuffix(query[strings.Index(query, "res_headers"):],
		"res_headers,ai_cache_status,mirror_hit,mirror_cluster,ai_intent_question,ai_intent_answer,"+
			"ai_intent_confidence,ai_intent_source,ai_intent_latency_us,ai_intent_cache_hit,ai_intent_questions_version "+
			"FROM bfe_ai_request_log WHERE (log_time>=? AND log_time<?) ORDER BY log_time DESC LIMIT ?,?"))
}

func TestTablePrefix(t *testing.T) {
	s := New(nil, "bfe_report", "mysql")
	assert.Equal(t, "bfe_report.bfe_ai_request_log", s.table(tableDetail))
	assert.Equal(t, "bfe_report.bfe_ai_metrics_1m", s.table(tableMetrics))

	s = New(nil, "", "mysql")
	assert.Equal(t, "bfe_ai_request_log", s.table(tableDetail))
}

// TestCapabilities 验证 MySQL 后端声明全部 12 个维度（含三个新维度）。
func TestCapabilities(t *testing.T) {
	caps := New(nil, "", "mysql").Capabilities()
	require.NotNil(t, caps)
	assert.Equal(t, "mysql", caps.Backend)
	assert.Len(t, caps.SupportedDimensions, 12)
	for _, dim := range []string{
		ireport.DimensionCacheStatus, ireport.DimensionMirrorHit, ireport.DimensionIntentAnswer,
	} {
		assert.Contains(t, caps.SupportedDimensions, dim)
	}
}

func TestOverviewResultFromRow(t *testing.T) {
	row := &overviewMetricsRow{
		requestTotal:     sql.NullInt64{Int64: 100, Valid: true},
		errorTotal:       sql.NullInt64{Int64: 5, Valid: true},
		inputTokens:      sql.NullInt64{Int64: 1000, Valid: true},
		outputTokens:     sql.NullInt64{Int64: 200, Valid: true},
		totalTokens:      sql.NullInt64{Int64: 1200, Valid: true},
		allTimeSum:       sql.NullInt64{Int64: 120000, Valid: true},
		latencyMax:       sql.NullFloat64{Float64: 3000, Valid: true},
		ttftUsSum:        sql.NullInt64{Int64: 40_000_000, Valid: true},
		streamRequests:   sql.NullInt64{Int64: 50, Valid: true},
		tpotUsSum:        sql.NullInt64{Int64: 2_000_000, Valid: true},
		rateLimitHits:    sql.NullInt64{Int64: 7, Valid: true},
		authRejects:      sql.NullInt64{Int64: 3, Valid: true},
		cacheReadTokens:  sql.NullInt64{Int64: 123456, Valid: true},
		cacheWriteTokens: sql.NullInt64{Int64: 7890, Valid: true},
	}
	detailCounts := &overviewDetailCounts{
		cacheHitCount:         sql.NullInt64{Int64: 120, Valid: true},
		cacheMissCount:        sql.NullInt64{Int64: 30, Valid: true},
		cacheSkipCount:        sql.NullInt64{Int64: 5, Valid: true},
		mirrorHitCount:        sql.NullInt64{Int64: 12, Valid: true},
		intentClassifiedCount: sql.NullInt64{Int64: 145, Valid: true},
		intentUnknownCount:    sql.NullInt64{Int64: 15, Valid: true},
	}
	cost := []*ireport.CostItem{{Currency: "USD", Value: 0.00005}}

	result := overviewResultFromRow(row, cost, 123, detailCounts)

	assert.Equal(t, int64(100), result.RequestTotal)
	assert.InDelta(t, 0.05, result.ErrorRate, 1e-9)
	assert.InDelta(t, 1200, result.LatencyAvgMs, 1e-9)
	assert.InDelta(t, 3000, result.LatencyMaxMs, 1e-9)
	assert.InDelta(t, 800, result.TtftAvgMs, 1e-9) // 40s/50 / 1000
	assert.InDelta(t, 40, result.TpotAvgMs, 1e-9)  // 2s/50 / 1000
	assert.Equal(t, int64(7), result.RateLimitHits)
	assert.Equal(t, int64(3), result.AuthRejects)
	assert.Equal(t, int64(123), result.LogsTotal)
	require.Len(t, result.Cost, 1)
	assert.Nil(t, result.LatencyP50Ms)

	// cache/mirror/intent 组：hit_rate=hit/(hit+miss)（skip 不计分母）；
	// unknown_rate=unknown/(classified+unknown)。
	assert.Equal(t, int64(120), result.Cache.HitCount)
	assert.Equal(t, int64(30), result.Cache.MissCount)
	assert.Equal(t, int64(5), result.Cache.SkipCount)
	assert.InDelta(t, 0.8, result.Cache.HitRate, 1e-9)
	assert.Equal(t, int64(123456), result.Cache.ReadTokens)
	assert.Equal(t, int64(7890), result.Cache.WriteTokens)
	assert.Equal(t, int64(12), result.Mirror.HitCount)
	assert.Equal(t, int64(145), result.Intent.ClassifiedCount)
	assert.Equal(t, int64(15), result.Intent.UnknownCount)
	assert.InDelta(t, 0.09375, result.Intent.UnknownRate, 1e-9)

	// zero requests: rates stay zero without division by zero.
	empty := overviewResultFromRow(&overviewMetricsRow{}, nil, 0, &overviewDetailCounts{})
	assert.Equal(t, float64(0), empty.ErrorRate)
	assert.Equal(t, float64(0), empty.LatencyAvgMs)
	assert.Equal(t, float64(0), empty.TtftAvgMs)
	assert.Equal(t, float64(0), empty.Cache.HitRate)
	assert.Equal(t, float64(0), empty.Intent.UnknownRate)
	assert.Equal(t, int64(0), empty.Cache.ReadTokens)
}

// TestOverviewResultFromRow_CacheHitRateSkipExcluded 锁定口径：skip 不计入
// 命中率分母（hit/(hit+miss)）。
func TestOverviewResultFromRow_CacheHitRateSkipExcluded(t *testing.T) {
	row := &overviewMetricsRow{}
	detailCounts := &overviewDetailCounts{
		cacheHitCount: sql.NullInt64{Int64: 1, Valid: true},
		// miss=0：hit_rate=1（skip 不参与）。
		cacheSkipCount: sql.NullInt64{Int64: 9, Valid: true},
	}
	result := overviewResultFromRow(row, nil, 0, detailCounts)
	assert.InDelta(t, 1.0, result.Cache.HitRate, 1e-9)

	// hit=miss=0（只有 skip）：分母为 0，命中率返回 0 而非 NaN。
	onlySkip := &overviewDetailCounts{cacheSkipCount: sql.NullInt64{Int64: 9, Valid: true}}
	result = overviewResultFromRow(row, nil, 0, onlySkip)
	assert.Equal(t, float64(0), result.Cache.HitRate)
}

func TestRowToMetricPoint(t *testing.T) {
	qps := rowToMetricPoint(ireport.MetricQPS, 1000, 60, metricRowValues{total: 300})
	require.NotNil(t, qps.Value)
	assert.InDelta(t, 5, *qps.Value, 1e-9)
	assert.Nil(t, qps.P50)

	tokens := rowToMetricPoint(ireport.MetricTokens, 1000, 300, metricRowValues{input: 600, output: 300, total: 900})
	assert.InDelta(t, 2, *tokens.Input, 1e-9)
	assert.InDelta(t, 1, *tokens.Output, 1e-9)
	assert.InDelta(t, 3, *tokens.Total, 1e-9)

	latency := rowToMetricPoint(ireport.MetricLatency, 1000, 60, metricRowValues{allTimeSum: 900, requestCount: 3, latencyMax: 450})
	assert.InDelta(t, 300, *latency.Avg, 1e-9)
	assert.InDelta(t, 450, *latency.Max, 1e-9)

	ttft := rowToMetricPoint(ireport.MetricTTFT, 1000, 60, metricRowValues{ttftUsSum: 2_000_000, streamRequests: 4})
	assert.InDelta(t, 500, *ttft.Value, 1e-9)

	noStream := rowToMetricPoint(ireport.MetricTPOT, 1000, 60, metricRowValues{})
	require.NotNil(t, noStream.Value)
	assert.Equal(t, float64(0), *noStream.Value)

	cost := rowToMetricPoint(ireport.MetricCost, 1000, 60, metricRowValues{value: 600_000_000, currency: "USD"})
	assert.InDelta(t, 0.1, *cost.Value, 1e-9) // 6e8 定点 / 60s / 1e8 = 0.1 元/秒
	assert.Equal(t, "USD", cost.Currency)

	// cache_tokens：原始 token 计数 ÷ 桶宽（不做 ÷1e8 换算），kind 区分序列。
	cacheRead := rowToMetricPoint(ireport.MetricCacheTokens, 1000, 60, metricRowValues{value: 4000, kind: "cache_read"})
	require.NotNil(t, cacheRead.Value)
	assert.InDelta(t, 4000.0/60, *cacheRead.Value, 1e-9)
	assert.Equal(t, "cache_read", cacheRead.Kind)

	// 维度拆分序列：name 透传。
	dim := rowToMetricPoint(ireport.MetricQPS, 1000, 60, metricRowValues{total: 300, name: "hit"})
	assert.Equal(t, "hit", dim.Name)
	assert.Equal(t, "", dim.Kind)
}

func TestDistributionRatios(t *testing.T) {
	items := []*ireport.DistItem{
		{Name: "200", RequestCount: 80},
		{Name: "500", RequestCount: 20},
	}
	distributionRatios(items)
	assert.InDelta(t, 0.8, items[0].Ratio, 1e-9)
	assert.InDelta(t, 0.2, items[1].Ratio, 1e-9)

	empty := []*ireport.DistItem{{Name: "unknown", RequestCount: 0}}
	distributionRatios(empty)
	assert.Equal(t, float64(0), empty[0].Ratio)
}

func TestNullPtrHelpers(t *testing.T) {
	assert.Nil(t, nullInt64Ptr(sql.NullInt64{}))
	assert.Nil(t, nullInt16Ptr(sql.NullInt64{}))
	assert.Nil(t, nullStringPtr(sql.NullString{}))
	assert.Nil(t, nullBoolPtr(sql.NullInt64{}))
	assert.Nil(t, nullFloat64Ptr(sql.NullFloat64{}))

	v := sql.NullInt64{Int64: 42, Valid: true}
	require.NotNil(t, nullInt64Ptr(v))
	assert.Equal(t, int64(42), *nullInt64Ptr(v))
	require.NotNil(t, nullInt16Ptr(v))
	assert.Equal(t, int16(42), *nullInt16Ptr(v))

	one := sql.NullInt64{Int64: 1, Valid: true}
	require.NotNil(t, nullBoolPtr(one))
	assert.True(t, *nullBoolPtr(one))
	zero := sql.NullInt64{Int64: 0, Valid: true}
	require.NotNil(t, nullBoolPtr(zero))
	assert.False(t, *nullBoolPtr(zero))

	f := sql.NullFloat64{Float64: 0.95, Valid: true}
	require.NotNil(t, nullFloat64Ptr(f))
	assert.InDelta(t, 0.95, *nullFloat64Ptr(f), 1e-12)

	c := sql.NullInt64{Int64: 66900, Valid: true}
	require.NotNil(t, nullCostAmountPtr(c))
	assert.InDelta(t, 0.000669, *nullCostAmountPtr(c), 1e-12)

	s := sql.NullString{String: "x", Valid: true}
	require.NotNil(t, nullStringPtr(s))
	assert.Equal(t, "x", *nullStringPtr(s))
}

// TestUnixSecondRendering_TimezoneNeutral 是时区口径问题的说明性测试：
// 所有「DATETIME → Unix 秒」的渲染必须走 TIMESTAMPDIFF（日历算术，不依赖
// MySQL 会话时区），不得使用按会话时区解读墙钟的 UNIX_TIMESTAMP。
// log-reader 以 UTC 墙钟写入 log_time，存储值即 UTC 墙钟。
func TestUnixSecondRendering_TimezoneNeutral(t *testing.T) {
	for _, bucket := range []int{60, 300, 1800} {
		expr := bucketExpr(bucket)
		assert.Contains(t, expr, "TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', ts_min)")
		assert.NotContains(t, expr, "UNIX_TIMESTAMP")
	}
	for _, field := range logRowFields {
		assert.NotContains(t, field, "UNIX_TIMESTAMP", "field: %s", field)
	}
	assert.Contains(t, logRowFields[1], "TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', log_time)")

	// 口径自洽性：TIMESTAMPDIFF 对 UTC 墙钟做日历算术，结果即真实 Unix 秒
	//（该值与任何会话时区无关；UNIX_TIMESTAMP 在 CST 会话下会把同一墙钟
	// 解读为 1789437600 = 2026-09-15 02:00:00 UTC，正是本修复消除的 8h 偏移）。
	want := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC).Unix()
	assert.Equal(t, int64(1789466400), want)
}

// fakeRowScanner 按预置值顺序填充 Scan 目标，模拟一行业务数据。
type fakeRowScanner struct {
	values []interface{}
	err    error
}

func (s *fakeRowScanner) Scan(dest ...interface{}) error {
	if s.err != nil {
		return s.err
	}
	if len(dest) != len(s.values) {
		return fmt.Errorf("dest len %d != values len %d", len(dest), len(s.values))
	}
	for i, d := range dest {
		switch t := d.(type) {
		case *sql.NullInt64:
			if v, ok := s.values[i].(sql.NullInt64); ok {
				*t = v
			}
		case *sql.NullString:
			if v, ok := s.values[i].(sql.NullString); ok {
				*t = v
			}
		case *sql.NullFloat64:
			if v, ok := s.values[i].(sql.NullFloat64); ok {
				*t = v
			}
		default:
			return fmt.Errorf("unexpected dest type %T", d)
		}
	}
	return nil
}

// TestLogWhere_PriceWarn 验证缺价形态过滤进 WHERE（issue #1479）。
func TestLogWhere_PriceWarn(t *testing.T) {
	shape := "reject"
	f := &ireport.LogFilter{
		Filter:    ireport.Filter{Start: testStart, End: testEnd},
		PriceWarn: &shape,
	}
	query, args, err := buildLogsCountSQL("bfe_ai_request_log", f)
	require.NoError(t, err)
	assert.Contains(t, query, "ai_price_warn=?")
	assert.Equal(t, []interface{}{"reject", testStart, testEnd}, args)
}

// TestScanLogRow_PriceMissColumns 验证两列缺价字段按投影列序映射到 LogRow：
// ai_price_miss 的 TINYINT 转 bool，ai_price_warn 原样透传（issue #1479）。
func TestScanLogRow_PriceMissColumns(t *testing.T) {
	values := make([]interface{}, len(logRowFields))
	for i := range values {
		values[i] = sql.NullString{}
	}
	values[0] = sql.NullInt64{Int64: 1, Valid: true}          // logid
	values[1] = sql.NullInt64{Int64: 1789466410, Valid: true} // log_time
	// 列序与 logRowFields 一致：cost_value / cost_currency 之后两列。
	idx := 0
	for i, field := range logRowFields {
		if field == "ai_price_miss" {
			idx = i
			break
		}
	}
	require.NotZero(t, idx, "ai_price_miss must be in logRowFields")
	values[idx] = sql.NullInt64{Int64: 1, Valid: true}
	values[idx+1] = sql.NullString{String: "reject", Valid: true}

	row, err := scanLogRow(&fakeRowScanner{values: values})
	require.NoError(t, err)
	require.NotNil(t, row.PriceMiss)
	assert.True(t, *row.PriceMiss)
	require.NotNil(t, row.PriceWarn)
	assert.Equal(t, "reject", *row.PriceWarn)

	// NULL 保持 nil（未发生缺价的行）
	values[idx] = sql.NullInt64{}
	values[idx+1] = sql.NullString{}
	row, err = scanLogRow(&fakeRowScanner{values: values})
	require.NoError(t, err)
	assert.Nil(t, row.PriceMiss)
	assert.Nil(t, row.PriceWarn)
}

// TestScanLogRow_CacheMirrorIntentColumns 验证明细行扫描：10 个新列按
// 投影列序映射到 LogRow，TINYINT 列转 bool，NULL 保持 nil。
func TestScanLogRow_CacheMirrorIntentColumns(t *testing.T) {
	// 列序与 logRowFields 一致：前 40 列任意填充，断言只看后 10 列。
	values := make([]interface{}, 0, len(logRowFields))
	values = append(values,
		sql.NullInt64{Int64: 1, Valid: true},          // logid
		sql.NullInt64{Int64: 1789466410, Valid: true}, // log_time
	)
	for len(values) < len(logRowFields)-10 {
		values = append(values, sql.NullString{})
	}
	values = append(values,
		sql.NullString{String: "hit", Valid: true},        // ai_cache_status
		sql.NullInt64{Int64: 1, Valid: true},              // mirror_hit
		sql.NullString{String: "mirror-bj", Valid: true},  // mirror_cluster
		sql.NullString{String: "task_type", Valid: true},  // ai_intent_question
		sql.NullString{String: "coding", Valid: true},     // ai_intent_answer
		sql.NullFloat64{Float64: 0.95, Valid: true},       // ai_intent_confidence
		sql.NullString{String: "classifier", Valid: true}, // ai_intent_source
		sql.NullInt64{Int64: 1200, Valid: true},           // ai_intent_latency_us
		sql.NullInt64{Int64: 0, Valid: true},              // ai_intent_cache_hit
		sql.NullString{String: "v3", Valid: true},         // ai_intent_questions_version
	)
	require.Len(t, values, len(logRowFields))

	row, err := scanLogRow(&fakeRowScanner{values: values})
	require.NoError(t, err)

	require.NotNil(t, row.AICacheStatus)
	assert.Equal(t, "hit", *row.AICacheStatus)
	require.NotNil(t, row.MirrorHit)
	assert.True(t, *row.MirrorHit)
	require.NotNil(t, row.MirrorCluster)
	assert.Equal(t, "mirror-bj", *row.MirrorCluster)
	require.NotNil(t, row.AIIntentQuestion)
	assert.Equal(t, "task_type", *row.AIIntentQuestion)
	require.NotNil(t, row.AIIntentAnswer)
	assert.Equal(t, "coding", *row.AIIntentAnswer)
	require.NotNil(t, row.AIIntentConfidence)
	assert.InDelta(t, 0.95, *row.AIIntentConfidence, 1e-12)
	require.NotNil(t, row.AIIntentSource)
	assert.Equal(t, "classifier", *row.AIIntentSource)
	require.NotNil(t, row.AIIntentLatencyUs)
	assert.Equal(t, int64(1200), *row.AIIntentLatencyUs)
	require.NotNil(t, row.AIIntentCacheHit)
	assert.False(t, *row.AIIntentCacheHit)
	require.NotNil(t, row.AIIntentQuestionsVer)
	assert.Equal(t, "v3", *row.AIIntentQuestionsVer)

	// 意图未求值行：可空列为 NULL -> nil。
	values[len(values)-10] = sql.NullString{String: "", Valid: true} // ai_cache_status=''
	values[len(values)-9] = sql.NullInt64{Int64: 0, Valid: true}     // mirror_hit=0
	values[len(values)-5] = sql.NullFloat64{}                        // confidence NULL
	values[len(values)-4] = sql.NullString{String: "", Valid: true}  // source=''
	values[len(values)-3] = sql.NullInt64{}                          // latency NULL
	values[len(values)-2] = sql.NullInt64{}                          // cache_hit NULL

	row, err = scanLogRow(&fakeRowScanner{values: values})
	require.NoError(t, err)
	require.NotNil(t, row.AICacheStatus)
	assert.Equal(t, "", *row.AICacheStatus)
	require.NotNil(t, row.MirrorHit)
	assert.False(t, *row.MirrorHit)
	assert.Nil(t, row.AIIntentConfidence)
	assert.Nil(t, row.AIIntentLatencyUs)
	assert.Nil(t, row.AIIntentCacheHit)
}
