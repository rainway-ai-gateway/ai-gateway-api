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

package clickhousereport

import (
	"database/sql"
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

func assertSnapshot(t *testing.T, query string, args []interface{}, err error,
	expectSQL string, expectArgs []interface{}) {
	t.Helper()
	require.NoError(t, err)
	assert.Equal(t, expectSQL, query)
	assert.Equal(t, expectArgs, args)
}

// fullFilterArgs is the fixed-order argument chain of the hand-assembled
// WHERE fragments: the time bounds bind Unix seconds (fromUnixTimestamp),
// followed by models / apikeys / providers / hosts / stream / status codes
// in the declaration order of metricsWhere (unlike the gendry map of the
// Doris backend, whose keys render alphabetically).
func fullFilterArgs() []interface{} {
	return []interface{}{
		testStart.Unix(), testEnd.Unix(),
		"gpt-4o", "gpt-4", "key-1", "openai", "gw-01", int8(1), 200, 500,
	}
}

func TestBuildOverviewMetricsSQL(t *testing.T) {
	query, args, err := buildOverviewMetricsSQL("bfe_ai_metrics_1m", fullFilter())

	require.NoError(t, err)
	assert.Contains(t, query, "ifNull(MAX(all_time_sum/request_count),0) AS latency_max")
	assert.Equal(t, fullFilterArgs(), args)
}

func TestBuildOverviewCostSQL(t *testing.T) {
	query, args, err := buildOverviewCostSQL("bfe_ai_metrics_1m", fullFilter())

	require.NoError(t, err)
	// The empty currency is excluded with a bound parameter, appended after
	// the shared filter chain in the fixed fragment order.
	assert.Contains(t, query, "ai_cost_currency != ?")
	assert.Contains(t, query, " GROUP BY ai_cost_currency")
	assert.Equal(t, append(fullFilterArgs(), ""), args)
}

func TestBuildOverviewPercentileSQL(t *testing.T) {
	query, args, err := buildOverviewPercentileSQL("bfe_ai_request_log", fullFilter())

	assertSnapshot(t, query, args, err,
		"SELECT quantile(0.5)(all_time) AS p50,"+
			"quantile(0.9)(all_time) AS p90,"+
			"quantile(0.99)(all_time) AS p99"+
			" FROM bfe_ai_request_log"+
			" WHERE log_time >= fromUnixTimestamp(?) AND log_time < fromUnixTimestamp(?)"+
			" AND ai_target_model IN (?,?) AND ai_apikey_id IN (?)"+
			" AND ai_provider IN (?) AND hostid IN (?)"+
			" AND ai_stream = ? AND res_status_code IN (?,?) AND all_time IS NOT NULL",
		fullFilterArgs())
}

func TestBuildTimeSeriesSQL_Dialect(t *testing.T) {
	f := &ireport.Filter{Start: testStart, End: testEnd}

	query, args, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricQPS, "", f, 300)
	assertSnapshot(t, query, args, err,
		"SELECT intDiv(toUnixTimestamp(ts_min), 300) * 300 AS time,"+
			"SUM(request_count) AS total"+
			" FROM bfe_ai_metrics_1m"+
			" WHERE ts_min >= fromUnixTimestamp(?) AND ts_min < fromUnixTimestamp(?)"+
			" GROUP BY time ORDER BY time ASC",
		[]interface{}{testStart.Unix(), testEnd.Unix()})

	query, _, err = buildLatencyPercentileSQL("bfe_ai_request_log", f, 60)
	require.NoError(t, err)
	assert.Equal(t, "SELECT intDiv(toUnixTimestamp(log_time), 60) * 60 AS time,"+
		"quantile(0.5)(all_time) AS p50,"+
		"quantile(0.9)(all_time) AS p90,"+
		"quantile(0.99)(all_time) AS p99"+
		" FROM bfe_ai_request_log"+
		" WHERE log_time >= fromUnixTimestamp(?) AND log_time < fromUnixTimestamp(?)"+
		" AND all_time IS NOT NULL GROUP BY time ORDER BY time ASC", query)
}

// TestBuildTimeSeriesSQL_CacheTokens 验证 ClickHouse 侧 cache_tokens 的
// UNION ALL 形态（与 MySQL / Doris 同口径，kind 字面量内联）。
func TestBuildTimeSeriesSQL_CacheTokens(t *testing.T) {
	f := &ireport.Filter{Start: testStart, End: testEnd}

	query, args, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricCacheTokens, "", f, 300)
	require.NoError(t, err)

	bucket := "intDiv(toUnixTimestamp(ts_min), 300) * 300 AS time"
	expect := "SELECT " + bucket + ",'cache_read' AS kind,SUM(cache_read_tokens) AS value" +
		" FROM bfe_ai_metrics_1m" +
		" WHERE ts_min >= fromUnixTimestamp(?) AND ts_min < fromUnixTimestamp(?) GROUP BY time" +
		" UNION ALL " +
		"SELECT " + bucket + ",'cache_write' AS kind,SUM(cache_write_tokens) AS value" +
		" FROM bfe_ai_metrics_1m" +
		" WHERE ts_min >= fromUnixTimestamp(?) AND ts_min < fromUnixTimestamp(?) GROUP BY time" +
		" ORDER BY time ASC,kind ASC"
	assertSnapshot(t, query, args, err, expect,
		[]interface{}{testStart.Unix(), testEnd.Unix(), testStart.Unix(), testEnd.Unix()})
}

// TestBuildTimeSeriesSQL_Dimension 验证维度分支：dimension 列渲染为 name
// 并加宽 GROUP BY（与 MySQL / Doris 后端同构，ClickHouse 方言）。
func TestBuildTimeSeriesSQL_Dimension(t *testing.T) {
	f := &ireport.Filter{Start: testStart, End: testEnd}

	// qps × ai_cache_status
	query, args, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricQPS, ireport.DimensionCacheStatus, f, 300)
	assertSnapshot(t, query, args, err,
		"SELECT intDiv(toUnixTimestamp(ts_min), 300) * 300 AS time,"+
			"ai_cache_status AS name,SUM(request_count) AS total"+
			" FROM bfe_ai_metrics_1m"+
			" WHERE ts_min >= fromUnixTimestamp(?) AND ts_min < fromUnixTimestamp(?)"+
			" GROUP BY time,ai_cache_status ORDER BY time ASC",
		[]interface{}{testStart.Unix(), testEnd.Unix()})

	// cost × mirror_hit：维度列插在 bucket 与 currency 之间，GROUP BY 含币种
	query, _, err = buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricCost, ireport.DimensionMirrorHit, f, 300)
	require.NoError(t, err)
	assert.Contains(t, query, "AS time,mirror_hit AS name,ai_cost_currency AS currency,SUM(ai_cost_value_sum) AS value")
	assert.Contains(t, query, "GROUP BY time,ai_cost_currency,mirror_hit")

	// cache_tokens × ai_intent_answer：UNION ALL 双臂携带 name，ORDER BY 含维度列
	query, args, err = buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricCacheTokens, ireport.DimensionIntentAnswer, f, 300)
	require.NoError(t, err)
	bucket := "intDiv(toUnixTimestamp(ts_min), 300) * 300 AS time"
	expect := "SELECT " + bucket + ",'cache_read' AS kind,ai_intent_answer AS name,SUM(cache_read_tokens) AS value" +
		" FROM bfe_ai_metrics_1m" +
		" WHERE ts_min >= fromUnixTimestamp(?) AND ts_min < fromUnixTimestamp(?) GROUP BY time,ai_intent_answer" +
		" UNION ALL " +
		"SELECT " + bucket + ",'cache_write' AS kind,ai_intent_answer AS name,SUM(cache_write_tokens) AS value" +
		" FROM bfe_ai_metrics_1m" +
		" WHERE ts_min >= fromUnixTimestamp(?) AND ts_min < fromUnixTimestamp(?) GROUP BY time,ai_intent_answer" +
		" ORDER BY time ASC,ai_intent_answer ASC,kind ASC"
	assertSnapshot(t, query, args, err, expect,
		[]interface{}{testStart.Unix(), testEnd.Unix(), testStart.Unix(), testEnd.Unix()})

	// 非法维度仍然报错
	_, _, err = buildTimeSeriesSQL("bfe_ai_metrics_1m", ireport.MetricQPS, "bogus", f, 60)
	require.Error(t, err)
}

// TestCapabilities 验证能力声明：聚合表 40 维齐全，声明 model/ireport 全部
// Dimension* 常量（12 个），manager 维度门控不再拦截。
func TestCapabilities(t *testing.T) {
	caps := New(nil, "", "clickhouse").Capabilities()
	require.NotNil(t, caps)
	assert.Equal(t, "clickhouse", caps.Backend)
	assert.Len(t, caps.SupportedDimensions, 12)
	for _, dim := range []string{
		ireport.DimensionCacheStatus, ireport.DimensionMirrorHit, ireport.DimensionIntentAnswer,
	} {
		assert.Contains(t, caps.SupportedDimensions, dim)
	}
}

func TestBuildRankingsSQL_Dialect(t *testing.T) {
	// string dimension: plain != '' predicate with a bound parameter.
	query, args, err := buildRankingsSQL("bfe_ai_metrics_1m", ireport.DimensionProvider, &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.NoError(t, err)
	assert.Contains(t, query, "SELECT ai_provider AS name,")
	assert.Contains(t, query, "ai_provider != ?")
	// LIMIT is inlined as a literal (server-derived integer).
	assert.True(t, strings.HasSuffix(query, " LIMIT 10"))
	assert.Equal(t, []interface{}{testStart.Unix(), testEnd.Unix(), ""}, args)

	// status dimension: numeric empty marker 0, name via toString.
	query, _, err = buildRankingsSQL("bfe_ai_metrics_1m", ireport.DimensionStatus, &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.NoError(t, err)
	assert.Contains(t, query, "SELECT toString(res_status_code) AS name,")
	assert.Contains(t, query, "res_status_code != ?")

	// cache_status: string dimension with the empty-marker exclusion.
	query, args, err = buildRankingsSQL("bfe_ai_metrics_1m", ireport.DimensionCacheStatus, &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.NoError(t, err)
	assert.Contains(t, query, "SELECT ai_cache_status AS name,")
	assert.Contains(t, query, "ai_cache_status != ?")
	assert.Equal(t, []interface{}{testStart.Unix(), testEnd.Unix(), ""}, args)

	// intent_answer: same string caliber.
	query, _, err = buildRankingsSQL("bfe_ai_metrics_1m", ireport.DimensionIntentAnswer, &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.NoError(t, err)
	assert.Contains(t, query, "SELECT ai_intent_answer AS name,")
	assert.Contains(t, query, "ai_intent_answer != ?")

	// mirror_hit: numeric 0/1, toString name, both buckets rank (no empty predicate).
	query, args, err = buildRankingsSQL("bfe_ai_metrics_1m", ireport.DimensionMirrorHit, &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.NoError(t, err)
	assert.Contains(t, query, "SELECT toString(mirror_hit) AS name,")
	assert.NotContains(t, query, "mirror_hit !=")
	assert.True(t, strings.HasSuffix(query, " LIMIT 10"))
	assert.Equal(t, []interface{}{testStart.Unix(), testEnd.Unix()}, args)

	_, _, err = buildRankingsSQL("bfe_ai_metrics_1m", "bogus", &ireport.Filter{Start: testStart, End: testEnd}, 10)
	require.Error(t, err)
}

func TestBuildDistributionSQL_Dialect(t *testing.T) {
	// status dimension: the 0 marker normalizes to the unknown bucket.
	query, _, err := buildDistributionSQL("bfe_ai_metrics_1m", ireport.DimensionStatus, &ireport.Filter{Start: testStart, End: testEnd})
	require.NoError(t, err)
	assert.Contains(t, query, "if(res_status_code = 0, 'unknown', toString(res_status_code)) AS name")

	query, _, err = buildDistributionSQL("bfe_ai_metrics_1m", ireport.DimensionMode, &ireport.Filter{Start: testStart, End: testEnd})
	require.NoError(t, err)
	assert.Contains(t, query, "if(ai_mode = '', 'unknown', toString(ai_mode)) AS name")

	// cache_status / intent_answer: '' normalizes to the unknown bucket.
	query, _, err = buildDistributionSQL("bfe_ai_metrics_1m", ireport.DimensionCacheStatus, &ireport.Filter{Start: testStart, End: testEnd})
	require.NoError(t, err)
	assert.Contains(t, query, "if(ai_cache_status = '', 'unknown', toString(ai_cache_status)) AS name")

	query, _, err = buildDistributionSQL("bfe_ai_metrics_1m", ireport.DimensionIntentAnswer, &ireport.Filter{Start: testStart, End: testEnd})
	require.NoError(t, err)
	assert.Contains(t, query, "if(ai_intent_answer = '', 'unknown', toString(ai_intent_answer)) AS name")

	// mirror_hit: numeric dimension as string buckets.
	query, _, err = buildDistributionSQL("bfe_ai_metrics_1m", ireport.DimensionMirrorHit, &ireport.Filter{Start: testStart, End: testEnd})
	require.NoError(t, err)
	assert.Contains(t, query, "toString(mirror_hit) AS name")

	_, _, err = buildDistributionSQL("bfe_ai_metrics_1m", "model", &ireport.Filter{Start: testStart, End: testEnd})
	require.Error(t, err)
}

func TestBuildLogsSQL(t *testing.T) {
	f := &ireport.LogFilter{
		Filter:   *fullFilter(),
		ErrOnly:  true,
		Keyword:  "timeout",
		Page:     2,
		PageSize: 20,
	}
	query, args, err := buildLogsSQL("bfe_ai_request_log", f)
	require.NoError(t, err)
	// pageSize/offset are inlined as literals (server-derived integers).
	assert.Contains(t, query, " ORDER BY log_time DESC LIMIT 20 OFFSET 20")
	assert.Contains(t, query, "err_msg LIKE ?")
	assert.Equal(t, append(fullFilterArgs(), "", "%timeout%"), args)
	// Complex columns render as JSON text: clickhouse-go stdlib returns
	// Nested/Array values as Go slices which database/sql cannot scan
	// into a string (see logRowFields). Nested columns are flattened by
	// the server default flatten_nested=1, so req_headers/res_headers
	// must recombine the physical subcolumns via arrayMap.
	assert.Contains(t, query, "toJSONString(CAST(arrayMap((k, v) -> (k, v), req_headers.key, req_headers.value) AS Array(Tuple(key String, value String)))) AS req_headers")
	assert.Contains(t, query, "toJSONString(CAST(arrayMap((k, v) -> (k, v), res_headers.key, res_headers.value) AS Array(Tuple(key String, value String)))) AS res_headers")
	assert.Contains(t, query, "toJSONString(ai_rate_limit_hits) AS ai_rate_limit_hits")
	assert.Contains(t, query, "toJSONString(ai_auth_reject_quota_plans) AS ai_auth_reject_quota_plans")

	countSQL, countArgs, err := buildLogsCountSQL("bfe_ai_request_log", f)
	require.NoError(t, err)
	assert.Equal(t, "SELECT count() FROM bfe_ai_request_log", countSQL[:strings.Index(countSQL, " WHERE")])
	assert.Contains(t, countSQL, "ifNull(err_code, '') != ?")
	assert.Equal(t, countArgs, args)
}

// TestBuildLogsSQL_CacheMirrorIntentFilters 验证五个新过滤参数渲染
// （与 MySQL / Doris 同口径）。
func TestBuildLogsSQL_CacheMirrorIntentFilters(t *testing.T) {
	cacheStatus := "hit"
	mirrorHit := true
	intentAnswer := "unknown"
	f := &ireport.LogFilter{
		Filter:       ireport.Filter{Start: testStart, End: testEnd},
		CacheStatus:  &cacheStatus,
		MirrorHit:    &mirrorHit,
		IntentAnswer: &intentAnswer,
	}
	query, args, err := buildLogsCountSQL("bfe_ai_request_log", f)
	require.NoError(t, err)
	assert.Contains(t, query, "ai_cache_status = ?")
	assert.Contains(t, query, "mirror_hit = ?")
	assert.Contains(t, query, "ai_intent_answer = ?")
	assert.Equal(t, []interface{}{testStart.Unix(), testEnd.Unix(), "hit", int8(1), "unknown"}, args)
}

func TestTablePrefix(t *testing.T) {
	s := New(nil, "bfe_observability", "clickhouse")
	assert.Equal(t, "bfe_observability.bfe_ai_request_log", s.table(tableDetail))

	s = New(nil, "", "clickhouse")
	assert.Equal(t, "bfe_ai_metrics_1m", s.table(tableMetrics))
}

func TestBuildOverviewDetailCountsSQL(t *testing.T) {
	query, args, err := buildOverviewDetailCountsSQL("bfe_ai_request_log", fullFilter())

	require.NoError(t, err)
	assert.Contains(t, query, "SUM(CASE WHEN ai_cache_status='hit' THEN 1 ELSE 0 END)")
	assert.Contains(t, query, "SUM(CASE WHEN ifNull(mirror_hit,0)=1 THEN 1 ELSE 0 END)")
	assert.Contains(t, query, "SUM(CASE WHEN ai_intent_answer='unknown' THEN 1 ELSE 0 END)")
	assert.Equal(t, fullFilterArgs(), args)
}

func TestRowToMetricPoint(t *testing.T) {
	qps := rowToMetricPoint(ireport.MetricQPS, 1000, 60, metricRowValues{total: 300})
	require.NotNil(t, qps.Value)
	assert.InDelta(t, 5, *qps.Value, 1e-9)

	latency := rowToMetricPoint(ireport.MetricLatency, 1000, 60, metricRowValues{allTimeSum: 900, requestCount: 3, latencyMax: 450})
	assert.InDelta(t, 300, *latency.Avg, 1e-9)
	assert.InDelta(t, 450, *latency.Max, 1e-9)

	ttft := rowToMetricPoint(ireport.MetricTTFT, 1000, 60, metricRowValues{ttftUsSum: 2_000_000, streamRequests: 4})
	assert.InDelta(t, 500, *ttft.Value, 1e-9)

	cost := rowToMetricPoint(ireport.MetricCost, 1000, 60, metricRowValues{value: 600_000_000, currency: "USD"})
	assert.InDelta(t, 0.1, *cost.Value, 1e-9) // 6e8 定点 / 60s / 1e8 = 0.1 元/秒
	assert.Equal(t, "USD", cost.Currency)

	cacheRead := rowToMetricPoint(ireport.MetricCacheTokens, 1000, 60, metricRowValues{value: 4000, kind: "cache_read"})
	require.NotNil(t, cacheRead.Value)
	assert.InDelta(t, 4000.0/60, *cacheRead.Value, 1e-9)
	assert.Equal(t, "cache_read", cacheRead.Kind)

	_, err := scanMetricPoint(nil, "bogus", "", 60)
	require.Error(t, err)
}

func TestOverviewResultFromRow(t *testing.T) {
	row := &overviewMetricsRow{
		requestTotal:     sql.NullInt64{Int64: 200, Valid: true},
		errorTotal:       sql.NullInt64{Int64: 10, Valid: true},
		allTimeSum:       sql.NullInt64{Int64: 400000, Valid: true},
		latencyMax:       sql.NullFloat64{Float64: 5000, Valid: true},
		ttftUsSum:        sql.NullInt64{Int64: 10_000_000, Valid: true},
		streamRequests:   sql.NullInt64{Int64: 40, Valid: true},
		tpotUsSum:        sql.NullInt64{Int64: 1_000_000, Valid: true},
		cacheReadTokens:  sql.NullInt64{Int64: 4500, Valid: true},
		cacheWriteTokens: sql.NullInt64{Int64: 700, Valid: true},
	}
	detailCounts := &overviewDetailCounts{
		cacheHitCount:         sql.NullInt64{Int64: 2, Valid: true},
		cacheMissCount:        sql.NullInt64{Int64: 2, Valid: true},
		cacheSkipCount:        sql.NullInt64{Int64: 1, Valid: true},
		mirrorHitCount:        sql.NullInt64{Int64: 2, Valid: true},
		intentClassifiedCount: sql.NullInt64{Int64: 3, Valid: true},
		intentUnknownCount:    sql.NullInt64{Int64: 1, Valid: true},
	}
	result := overviewResultFromRow(row, nil, 5, detailCounts)
	assert.Equal(t, int64(200), result.RequestTotal)
	assert.InDelta(t, 0.05, result.ErrorRate, 1e-9)
	assert.InDelta(t, 2000, result.LatencyAvgMs, 1e-9)
	assert.InDelta(t, 250, result.TtftAvgMs, 1e-9)
	assert.Nil(t, result.LatencyP50Ms)
	// 与 MySQL / Doris 后端同口径：hit_rate=hit/(hit+miss)，skip 不计分母。
	assert.InDelta(t, 0.5, result.Cache.HitRate, 1e-9)
	assert.Equal(t, int64(4500), result.Cache.ReadTokens)
	assert.Equal(t, int64(700), result.Cache.WriteTokens)
	assert.Equal(t, int64(2), result.Mirror.HitCount)
	assert.Equal(t, int64(3), result.Intent.ClassifiedCount)
	assert.Equal(t, int64(1), result.Intent.UnknownCount)
	assert.InDelta(t, 0.25, result.Intent.UnknownRate, 1e-9)
}

func TestDistributionRatios(t *testing.T) {
	items := []*ireport.DistItem{{Name: "chat", RequestCount: 90}, {Name: "unknown", RequestCount: 10}}
	distributionRatios(items)
	assert.InDelta(t, 0.9, items[0].Ratio, 1e-9)
	assert.InDelta(t, 0.1, items[1].Ratio, 1e-9)
}

func TestNullPtrHelpers(t *testing.T) {
	assert.Nil(t, nullInt64Ptr(sql.NullInt64{}))
	assert.Nil(t, nullInt16Ptr(sql.NullInt64{}))
	assert.Nil(t, nullStringPtr(sql.NullString{}))
	assert.Nil(t, nullCostAmountPtr(sql.NullInt64{}))
	assert.Nil(t, nullBoolPtr(sql.NullInt64{}))
	assert.Nil(t, nullFloat64Ptr(sql.NullFloat64{}))

	v := sql.NullInt64{Int64: 7, Valid: true}
	require.NotNil(t, nullInt64Ptr(v))
	assert.Equal(t, int64(7), *nullInt64Ptr(v))
	require.NotNil(t, nullInt16Ptr(v))
	assert.Equal(t, int16(7), *nullInt16Ptr(v))

	one := sql.NullInt64{Int64: 1, Valid: true}
	require.NotNil(t, nullBoolPtr(one))
	assert.True(t, *nullBoolPtr(one))

	f := sql.NullFloat64{Float64: 0.95, Valid: true}
	require.NotNil(t, nullFloat64Ptr(f))
	assert.InDelta(t, 0.95, *nullFloat64Ptr(f), 1e-12)

	c := sql.NullInt64{Int64: 66900, Valid: true}
	require.NotNil(t, nullCostAmountPtr(c))
	assert.InDelta(t, 0.000669, *nullCostAmountPtr(c), 1e-12)
}

// TestTTFTTPOTStreamCaliber pins the stream-only caliber of ttft/tpot on the
// query side: only rows with ai_stream=1 enter the numerator, so non-stream
// rows carrying BFE-measured first-byte time no longer inflate the average
// (ai-gateway-api#315; the MV side excludes them at aggregation time).
func TestTTFTTPOTStreamCaliber(t *testing.T) {
	overview, _, err := buildOverviewMetricsSQL("bfe_ai_metrics_1m", fullFilter())
	require.NoError(t, err)
	assert.Contains(t, overview,
		"ifNull(SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END),0) AS ttft_us_sum_total")
	assert.Contains(t, overview,
		"ifNull(SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END),0) AS tpot_us_sum_total")
	// Denominator caliber is unchanged and numerator has no bare SUM left.
	assert.Contains(t, overview, "SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END),0) AS stream_requests")
	assert.NotContains(t, overview, "SUM(ttft_us_sum)")
	assert.NotContains(t, overview, "SUM(tpot_us_sum)")

	for _, metric := range []string{ireport.MetricTTFT, ireport.MetricTPOT} {
		ts, _, err := buildTimeSeriesSQL("bfe_ai_metrics_1m", metric, "", &ireport.Filter{Start: testStart, End: testEnd}, 60)
		require.NoError(t, err)
		assert.Contains(t, ts, "SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END) AS ttft_us_sum_total")
		assert.Contains(t, ts, "SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END) AS tpot_us_sum_total")
		assert.Contains(t, ts, "SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END) AS stream_requests")
		assert.NotContains(t, ts, "SUM(ttft_us_sum)")
		assert.NotContains(t, ts, "SUM(tpot_us_sum)")
	}
}
