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

package query_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reportPrefix = "/open-api/v1/report"

func getJSON(t *testing.T, path string, query map[string]string, out interface{}) *testutil.APIResponse {
	t.Helper()
	resp, err := testutil.GetClient().Get(path, query)
	require.NoError(t, err)
	require.NotNil(t, resp)
	testutil.AssertSuccess(t, resp)
	if out != nil {
		require.NoError(t, json.Unmarshal(resp.Data, out), "data: %s", string(resp.Data))
	}
	return resp
}

// TestOverview 验证总览指标卡：聚合表合计 + 成本分组 + 明细 COUNT 全部精确等于手算值。
func TestOverview(t *testing.T) {
	var data overviewData
	getJSON(t, reportPrefix+"/overview", windowQuery(), &data)

	assert.Equal(t, int64(660), data.RequestTotal)
	assert.Equal(t, int64(200), data.ErrorTotal)
	assert.InDelta(t, 200.0/660.0, data.ErrorRate, 1e-9)
	assert.Equal(t, int64(6600), data.InputTokens)
	assert.Equal(t, int64(1320), data.OutputTokens)
	assert.Equal(t, int64(7920), data.TotalTokens)

	// latency_avg = all_time_sum/request_total = 66000/660 = 100ms；
	// latency_max = MAX(all_time_sum/request_count)（各组均 100ms）。
	assert.InDelta(t, 100, data.LatencyAvgMs, 1e-9)
	assert.InDelta(t, 100, data.LatencyMaxMs, 1e-9)
	// MySQL 后端不返回分位数，字段恒不存在。
	assert.Nil(t, data.LatencyP50Ms)

	// ttft/tpot 聚合于 stream 请求（450 个），微秒→毫秒。
	assert.InDelta(t, 225000.0/450/1000, data.TtftAvgMs, 1e-9) // 0.5ms
	assert.InDelta(t, 22500.0/450/1000, data.TpotAvgMs, 1e-9)  // 0.05ms

	require.Len(t, data.Cost, 2)
	costByCurrency := map[string]float64{}
	for _, one := range data.Cost {
		costByCurrency[one.Currency] = one.Value
	}
	// 定点种子 USD=350 / RMB=300，接口出口换算为金额（÷1e8）。
	assert.InDelta(t, 3.5e-6, costByCurrency["USD"], 1e-12)
	assert.InDelta(t, 3e-6, costByCurrency["RMB"], 1e-12)

	assert.Equal(t, int64(2), data.RateLimitHits)
	assert.Equal(t, int64(1), data.AuthRejects)
	assert.Equal(t, int64(6), data.LogsTotal) // 明细表 COUNT(*)

	// 缓存/镜像/意图组：明细 count 口径（见种子注释）+ 聚合表 cache token SUM。
	assert.Equal(t, int64(2), data.Cache.HitCount)
	assert.Equal(t, int64(2), data.Cache.MissCount)
	assert.Equal(t, int64(1), data.Cache.SkipCount)
	assert.InDelta(t, 0.5, data.Cache.HitRate, 1e-9) // hit/(hit+miss)，skip 不计分母
	assert.Equal(t, int64(4500), data.Cache.ReadTokens)
	assert.Equal(t, int64(700), data.Cache.WriteTokens)
	assert.Equal(t, int64(2), data.Mirror.HitCount)
	assert.Equal(t, int64(3), data.Intent.ClassifiedCount)
	assert.Equal(t, int64(1), data.Intent.UnknownCount)
	assert.InDelta(t, 0.25, data.Intent.UnknownRate, 1e-9)
}

// TestOverview_Filtered 验证过滤项（models + status_codes）参与聚合口径。
func TestOverview_Filtered(t *testing.T) {
	query := with(windowQuery(),
		"models", "gpt-4o",
		"status_codes", "200")
	var data overviewData
	getJSON(t, reportPrefix+"/overview", query, &data)

	// gpt-4o + 200：聚合行 A(100) + D(50)。
	assert.Equal(t, int64(150), data.RequestTotal)
	assert.Equal(t, int64(0), data.ErrorTotal)
	assert.Equal(t, int64(1500), data.InputTokens)
	// 明细中 ai_target_model=gpt-4o 且 res_status_code=200：1001、1006 两行。
	assert.Equal(t, int64(2), data.LogsTotal)
}

// TestTimeSeries_QPS 验证 qps 时序：≤6h 窗口 → 60s 桶，值为每分钟请求数/60。
func TestTimeSeries_QPS(t *testing.T) {
	var data timeseriesData
	getJSON(t, reportPrefix+"/timeseries", with(windowQuery(), "metric", "qps"), &data)

	assert.Equal(t, 60, data.BucketSec)
	require.Len(t, data.Series, 2)

	assert.Equal(t, epoch("2026-09-15 10:00:00"), data.Series[0].Time)
	require.NotNil(t, data.Series[0].Value)
	assert.InDelta(t, 10, *data.Series[0].Value, 1e-9) // 600/60

	assert.Equal(t, epoch("2026-09-15 10:01:00"), data.Series[1].Time)
	require.NotNil(t, data.Series[1].Value)
	assert.InDelta(t, 1, *data.Series[1].Value, 1e-9) // 60/60
}

// TestTimeSeries_Tokens 验证 tokens 时序的多值字段（个/秒）。
func TestTimeSeries_Tokens(t *testing.T) {
	var data timeseriesData
	getJSON(t, reportPrefix+"/timeseries", with(windowQuery(), "metric", "tokens"), &data)

	assert.Equal(t, 60, data.BucketSec)
	require.Len(t, data.Series, 2)

	assert.InDelta(t, 100, *data.Series[0].Input, 1e-9) // 6000/60
	assert.InDelta(t, 20, *data.Series[0].Output, 1e-9) // 1200/60
	assert.InDelta(t, 120, *data.Series[0].Total, 1e-9) // 7200/60
	assert.InDelta(t, 10, *data.Series[1].Input, 1e-9)  // 600/60
	assert.InDelta(t, 2, *data.Series[1].Output, 1e-9)  // 120/60
	assert.InDelta(t, 12, *data.Series[1].Total, 1e-9)  // 720/60
}

// TestTimeSeries_Cost 验证成本时序：金额/秒口径（定点值 ÷ 桶宽 ÷ 1e8），
// 按 (time, currency) 分序列。注意：时序成本不像 overview 成本那样过滤空币种，
// 种子行 E 会产生 (10:01, "") 的 0 值点，此处一并锁定该现状行为。
func TestTimeSeries_Cost(t *testing.T) {
	var data timeseriesData
	getJSON(t, reportPrefix+"/timeseries", with(windowQuery(), "metric", "cost"), &data)

	assert.Equal(t, 60, data.BucketSec)
	require.Len(t, data.Series, 4)

	type pointKey struct {
		time     int64
		currency string
	}
	byKey := map[pointKey]float64{}
	for _, one := range data.Series {
		require.NotNil(t, one.Value)
		byKey[pointKey{one.Time, one.Currency}] = *one.Value
	}

	// 10:00 桶：USD 定点 300（行 A+B）、RMB 定点 300（行 C）；
	// 10:01 桶：USD 定点 50（行 D）、空币种定点 0（行 E）。
	assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2026-09-15 10:00:00"), "USD"}], 1e-15)
	assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2026-09-15 10:00:00"), "RMB"}], 1e-15)
	assert.InDelta(t, 50.0/60/1e8, byKey[pointKey{epoch("2026-09-15 10:01:00"), "USD"}], 1e-15)
	assert.Equal(t, 0.0, byKey[pointKey{epoch("2026-09-15 10:01:00"), ""}])
}

// TestRankings_Model 验证模型维度排行：按 request_count 降序，指标列精确。
func TestRankings_Model(t *testing.T) {
	var data rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "model"), &data)

	require.Len(t, data.Items, 3)
	assert.Equal(t, "gpt-4o", data.Items[0].Name)
	assert.Equal(t, int64(350), data.Items[0].RequestCount)
	assert.Equal(t, int64(200), data.Items[0].ErrorCount)
	assert.Equal(t, int64(3500), data.Items[0].InputTokens)
	assert.Equal(t, int64(700), data.Items[0].OutputTokens)

	assert.Equal(t, "gpt-4", data.Items[1].Name)
	assert.Equal(t, int64(300), data.Items[1].RequestCount)

	assert.Equal(t, "claude", data.Items[2].Name)
	assert.Equal(t, int64(10), data.Items[2].RequestCount)
}

// TestRankings_Status 验证状态码维度排行：数字维度以字符串名返回。
func TestRankings_Status(t *testing.T) {
	var data rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "status"), &data)

	require.Len(t, data.Items, 2)
	assert.Equal(t, "200", data.Items[0].Name)
	assert.Equal(t, int64(460), data.Items[0].RequestCount)
	assert.Equal(t, "500", data.Items[1].Name)
	assert.Equal(t, int64(200), data.Items[1].RequestCount)
}

// TestRankings_Limit 验证 limit 默认 10；显式 limit 生效。
func TestRankings_Limit(t *testing.T) {
	var data rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "model", "limit", "2"), &data)
	require.Len(t, data.Items, 2)

	var defaultLimit rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "model"), &defaultLimit)
	require.Len(t, defaultLimit.Items, 3) // 种子只有 3 组，默认 limit=10 全量返回
}

// TestDistribution_Status 验证状态码分布：ratio 合计≈1。
func TestDistribution_Status(t *testing.T) {
	var data distributionData
	getJSON(t, reportPrefix+"/distribution", with(windowQuery(), "dimension", "status"), &data)

	require.Len(t, data.Items, 2)
	byName := map[string]distItem{}
	for _, one := range data.Items {
		byName[one.Name] = one
	}

	require.Contains(t, byName, "200")
	require.Contains(t, byName, "500")
	assert.Equal(t, int64(460), byName["200"].RequestCount)
	assert.InDelta(t, 460.0/660.0, byName["200"].Ratio, 1e-9)
	assert.Equal(t, int64(200), byName["500"].RequestCount)
	assert.InDelta(t, 200.0/660.0, byName["500"].Ratio, 1e-9)
	assert.InDelta(t, 1, byName["200"].Ratio+byName["500"].Ratio, 1e-9)
}

// TestDistribution_Protocol_UnknownBucket 验证空维度归一为 "unknown" 桶。
func TestDistribution_Protocol_UnknownBucket(t *testing.T) {
	var data distributionData
	getJSON(t, reportPrefix+"/distribution", with(windowQuery(), "dimension", "protocol"), &data)

	require.Len(t, data.Items, 2)
	byName := map[string]distItem{}
	for _, one := range data.Items {
		byName[one.Name] = one
	}

	require.Contains(t, byName, "openai")
	require.Contains(t, byName, "unknown")
	assert.Equal(t, int64(650), byName["openai"].RequestCount)
	assert.Equal(t, int64(10), byName["unknown"].RequestCount)
	assert.InDelta(t, 10.0/660.0, byName["unknown"].Ratio, 1e-9)
}

// TestLogs_Paging 验证明细分页：total、page_size 生效、log_time 倒序。
func TestLogs_Paging(t *testing.T) {
	var page1 logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "page", "1", "page_size", "2"), &page1)

	assert.Equal(t, int64(6), page1.Total)
	assert.Equal(t, 1, page1.Page)
	assert.Equal(t, 2, page1.PageSize)
	require.Len(t, page1.Items, 2)
	// log_time 倒序：10:02:00(1006) > 10:01:10(1005)。
	assert.Equal(t, int64(1006), *page1.Items[0].LogID)
	assert.Equal(t, int64(1005), *page1.Items[1].LogID)

	var page2 logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "page", "2", "page_size", "2"), &page2)
	require.Len(t, page2.Items, 2)
	assert.Equal(t, int64(1004), *page2.Items[0].LogID)
	assert.Equal(t, int64(1003), *page2.Items[1].LogID)
}

// TestLogs_Filters 验证 err_only 与 keyword（err_msg LIKE）过滤。
func TestLogs_Filters(t *testing.T) {
	var errOnly logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "err_only", "true"), &errOnly)

	assert.Equal(t, int64(2), errOnly.Total)
	require.Len(t, errOnly.Items, 2)
	assert.Equal(t, int64(1004), *errOnly.Items[0].LogID) // 10:00:40 先于 10:00:20
	assert.Equal(t, int64(1002), *errOnly.Items[1].LogID)

	var keyword logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "keyword", "timeout"), &keyword)

	assert.Equal(t, int64(1), keyword.Total)
	require.Len(t, keyword.Items, 1)
	assert.Equal(t, int64(1002), *keyword.Items[0].LogID)
	require.NotNil(t, keyword.Items[0].ErrMsg)
	assert.Contains(t, *keyword.Items[0].ErrMsg, "timeout")
}

// TestLogs_RowShape 验证明细行字段形状：Unix 秒时间、NULL 列、JSON 列原样字符串、标签打平。
func TestLogs_RowShape(t *testing.T) {
	var data logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "page_size", "6"), &data)

	require.Len(t, data.Items, 6)
	byID := map[int64]logItem{}
	for _, one := range data.Items {
		byID[*one.LogID] = one
	}

	// log_time 为 Unix 秒（UNIX_TIMESTAMP 语义，与种子时刻一致）。
	newest := byID[1006]
	assert.Equal(t, epoch("2026-09-15 10:02:00"), newest.LogTime)
	require.NotNil(t, newest.Hostid)
	assert.Equal(t, "gw-01", *newest.Hostid)
	require.NotNil(t, newest.Product)
	assert.Equal(t, "BFE", *newest.Product)

	// 未认证行：ai_apikey_id 为 null。
	unauthenticated := byID[1004]
	assert.Nil(t, unauthenticated.APIKeyID)
	require.NotNil(t, unauthenticated.ErrCode)
	assert.Equal(t, "E401", *unauthenticated.ErrCode)

	// JSON 列原样字符串返回（MySQL JSON 列按规范形式回显：键序不保证、冒号后带空格，
	// 因此键与值分别断言）。
	withHits := byID[1002]
	require.NotNil(t, withHits.RateLimitHits)
	assert.Contains(t, *withHits.RateLimitHits, "rate_limit_policy_id")
	assert.Contains(t, *withHits.RateLimitHits, "p1")
	require.NotNil(t, withHits.ReqHeaders)
	assert.Contains(t, *withHits.ReqHeaders, "X-Test")

	withPlans := byID[1004]
	require.NotNil(t, withPlans.AuthRejectQuotaPlans)
	assert.Contains(t, *withPlans.AuthRejectQuotaPlans, "plan-a")

	// API Key 标签打平字段。
	tagged := byID[1001]
	require.NotNil(t, tagged.Level1Name)
	assert.Equal(t, "dep", *tagged.Level1Name)
	require.NotNil(t, tagged.Level1)
	assert.Equal(t, "ops", *tagged.Level1)
	require.NotNil(t, tagged.OriginURI)
	assert.Equal(t, "/v1/chat", *tagged.OriginURI)

	// 成本列：定点值换算为金额（÷1e8），币种随列返回。
	withCost := byID[1001]
	require.NotNil(t, withCost.CostValue)
	assert.InDelta(t, 100.0/1e8, *withCost.CostValue, 1e-15)
	require.NotNil(t, withCost.CostCurrency)
	assert.Equal(t, "USD", *withCost.CostCurrency)

	// 零成本行：currency 为 NULL（1004）/ 空串（1005）时 ai_cost_value 为 0。
	noCurrency := byID[1004]
	require.NotNil(t, noCurrency.CostValue)
	assert.Equal(t, 0.0, *noCurrency.CostValue)
	assert.Nil(t, noCurrency.CostCurrency)
	emptyCurrency := byID[1005]
	require.NotNil(t, emptyCurrency.CostValue)
	assert.Equal(t, 0.0, *emptyCurrency.CostValue)
	require.NotNil(t, emptyCurrency.CostCurrency)
	assert.Equal(t, "", *emptyCurrency.CostCurrency)
}

// TestLogs_RequestedModels 验证 requested_models 过滤（仅明细端点支持）。
func TestLogs_RequestedModels(t *testing.T) {
	var data logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "requested_models", "claude-3"), &data)

	assert.Equal(t, int64(1), data.Total)
	require.Len(t, data.Items, 1)
	assert.Equal(t, int64(1005), *data.Items[0].LogID)
}

// TestOverview_CacheMirrorIntentFiltered 验证新过滤项（models 之外的维度）不干扰
// 缓存/镜像/意图组的明细 count 口径：过滤后窗口内 1001 单行。
func TestOverview_CacheMirrorIntentFiltered(t *testing.T) {
	var data overviewData
	getJSON(t, reportPrefix+"/overview", with(windowQuery(),
		"models", "gpt-4o", "status_codes", "200"), &data)

	// 明细中 gpt-4o 且 200：1001、1006；其中 1006 intent 未求值、1001 全量字段。
	assert.Equal(t, int64(2), data.LogsTotal)
	assert.Equal(t, int64(1), data.Cache.HitCount)  // 1001
	assert.Equal(t, int64(1), data.Cache.MissCount) // 1006
	assert.Equal(t, int64(1), data.Mirror.HitCount) // 1001
	assert.Equal(t, int64(1), data.Intent.ClassifiedCount)
	assert.Equal(t, int64(0), data.Intent.UnknownCount)
	assert.Equal(t, float64(0), data.Intent.UnknownRate) // 分母为 0
}

// TestTimeSeries_CacheTokens 验证 cache_tokens 时序：cache_read/cache_write
// 两条序列（kind 区分），值 = 聚合表列 SUM ÷ 60s（不做 ÷1e8）。
func TestTimeSeries_CacheTokens(t *testing.T) {
	var data timeseriesData
	getJSON(t, reportPrefix+"/timeseries", with(windowQuery(), "metric", "cache_tokens"), &data)

	assert.Equal(t, 60, data.BucketSec)
	require.Len(t, data.Series, 4)

	type pointKey struct {
		time int64
		kind string
	}
	byKey := map[pointKey]float64{}
	for _, one := range data.Series {
		require.NotNil(t, one.Value)
		byKey[pointKey{one.Time, one.Kind}] = *one.Value
	}
	// 10:00 桶：read=A(1000)+B(0)+C(3000)=4000、write=A(200)+B(400)+C(0)=600；
	// 10:01 桶：read=D(500)+E(0)=500、write=D(100)+E(0)=100。
	assert.InDelta(t, 4000.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "cache_read"}], 1e-9)
	assert.InDelta(t, 600.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "cache_write"}], 1e-9)
	assert.InDelta(t, 500.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), "cache_read"}], 1e-9)
	assert.InDelta(t, 100.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), "cache_write"}], 1e-9)
}

// TestTimeSeries_Dimension 验证 timeseries 可选 dimension 参数：按
// (时间桶, 维度值) 拆序列，点携带 name（空值为 ""）。
func TestTimeSeries_Dimension(t *testing.T) {
	var data timeseriesData
	getJSON(t, reportPrefix+"/timeseries", with(windowQuery(),
		"metric", "qps", "dimension", "ai_cache_status"), &data)

	assert.Equal(t, 60, data.BucketSec)
	// 10:00：hit(A+C=400)、miss(B=200)；10:01：hit(D=50)、''(E=10)。
	require.Len(t, data.Series, 4)

	type pointKey struct {
		time int64
		name string
	}
	byKey := map[pointKey]float64{}
	for _, one := range data.Series {
		require.NotNil(t, one.Value)
		byKey[pointKey{one.Time, one.Name}] = *one.Value
	}
	assert.InDelta(t, 400.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "hit"}], 1e-9)
	assert.InDelta(t, 200.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "miss"}], 1e-9)
	assert.InDelta(t, 50.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), "hit"}], 1e-9)
	assert.InDelta(t, 10.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), ""}], 1e-9)
}

// TestTimeSeries_InvalidDimension 验证 timeseries dimension 白名单：既有维度
// （model）不被接受。
func TestTimeSeries_InvalidDimension(t *testing.T) {
	client := testutil.GetClient()

	resp, err := client.Get(reportPrefix+"/timeseries", with(windowQuery(),
		"metric", "qps", "dimension", "model"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	testutil.AssertErrCode(t, resp, 422)
	assert.Contains(t, resp.ErrMsg, "invalid dimension")
}

// TestRankings_CacheStatus 验证 ai_cache_status 维度排行：空值（''）不进排行。
func TestRankings_CacheStatus(t *testing.T) {
	var data rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "ai_cache_status"), &data)

	require.Len(t, data.Items, 2)
	assert.Equal(t, "hit", data.Items[0].Name)
	assert.Equal(t, int64(450), data.Items[0].RequestCount) // A+C+D
	assert.Equal(t, int64(0), data.Items[0].ErrorCount)
	assert.Equal(t, "miss", data.Items[1].Name)
	assert.Equal(t, int64(200), data.Items[1].RequestCount) // B
}

// TestRankings_MirrorHit 验证 mirror_hit 维度排行：数值维度 0/1 两桶都进排行。
func TestRankings_MirrorHit(t *testing.T) {
	var data rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "mirror_hit"), &data)

	require.Len(t, data.Items, 2)
	assert.Equal(t, "0", data.Items[0].Name)
	assert.Equal(t, int64(510), data.Items[0].RequestCount) // B+C+E
	assert.Equal(t, "1", data.Items[1].Name)
	assert.Equal(t, int64(150), data.Items[1].RequestCount) // A+D
}

// TestRankings_IntentAnswer 验证 ai_intent_answer 维度排行。
func TestRankings_IntentAnswer(t *testing.T) {
	var data rankingsData
	getJSON(t, reportPrefix+"/rankings", with(windowQuery(), "dimension", "ai_intent_answer"), &data)

	require.Len(t, data.Items, 3)
	byName := map[string]int64{}
	for _, one := range data.Items {
		byName[one.Name] = one.RequestCount
	}
	assert.Equal(t, int64(200), byName["unknown"]) // B
	assert.Equal(t, int64(300), byName["writing"]) // C
	assert.Equal(t, int64(150), byName["coding"])  // A+D
	// 空值行 E 不进排行。
	assert.NotContains(t, byName, "")
}

// TestDistribution_CacheStatus 验证 ai_cache_status 分布：空值归一为 unknown 桶。
func TestDistribution_CacheStatus(t *testing.T) {
	var data distributionData
	getJSON(t, reportPrefix+"/distribution", with(windowQuery(), "dimension", "ai_cache_status"), &data)

	require.Len(t, data.Items, 3)
	byName := map[string]distItem{}
	for _, one := range data.Items {
		byName[one.Name] = one
	}
	assert.Equal(t, int64(450), byName["hit"].RequestCount)
	assert.InDelta(t, 450.0/660.0, byName["hit"].Ratio, 1e-9)
	assert.Equal(t, int64(200), byName["miss"].RequestCount)
	assert.Equal(t, int64(10), byName["unknown"].RequestCount) // 行 E 的空值
	assert.InDelta(t, 1, byName["hit"].Ratio+byName["miss"].Ratio+byName["unknown"].Ratio, 1e-9)
}

// TestDistribution_MirrorHit 验证 mirror_hit 分布：数值维度以字符串名返回。
func TestDistribution_MirrorHit(t *testing.T) {
	var data distributionData
	getJSON(t, reportPrefix+"/distribution", with(windowQuery(), "dimension", "mirror_hit"), &data)

	require.Len(t, data.Items, 2)
	byName := map[string]distItem{}
	for _, one := range data.Items {
		byName[one.Name] = one
	}
	assert.Equal(t, int64(510), byName["0"].RequestCount)
	assert.Equal(t, int64(150), byName["1"].RequestCount)
	assert.InDelta(t, 150.0/660.0, byName["1"].Ratio, 1e-9)
}

// TestLogs_CacheMirrorIntentFilters 验证五个新过滤参数的端到端过滤口径。
func TestLogs_CacheMirrorIntentFilters(t *testing.T) {
	cases := []struct {
		name     string
		query    map[string]string
		expectID []int64
	}{
		{"cache_status_hit", with(windowQuery(), "cache_status", "hit"), []int64{1003, 1001}},
		{"cache_status_skip", with(windowQuery(), "cache_status", "skip"), []int64{1005}},
		{"mirror_hit_true", with(windowQuery(), "mirror_hit", "true"), []int64{1005, 1001}},
		{"mirror_hit_false", with(windowQuery(), "mirror_hit", "false"), []int64{1006, 1004, 1003, 1002}},
		{"intent_question", with(windowQuery(), "intent_question", "task_type"), []int64{1005, 1003, 1002, 1001}},
		{"intent_answer_coding", with(windowQuery(), "intent_answer", "coding"), []int64{1005, 1001}},
		{"intent_answer_unknown", with(windowQuery(), "intent_answer", "unknown"), []int64{1002}},
		{"intent_source_classifier", with(windowQuery(), "intent_source", "classifier"), []int64{1002, 1001}},
		{"intent_source_cache", with(windowQuery(), "intent_source", "cache"), []int64{1005}},
		{"combined", with(windowQuery(), "cache_status", "hit", "intent_answer", "coding"), []int64{1001}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data logsData
			getJSON(t, reportPrefix+"/logs", tc.query, &data)

			assert.Equal(t, int64(len(tc.expectID)), data.Total)
			require.Len(t, data.Items, len(tc.expectID))
			for i, id := range tc.expectID {
				assert.Equal(t, id, *data.Items[i].LogID, "position %d", i)
			}
		})
	}
}

// TestLogs_CacheMirrorIntentRowShape 验证明细行新 10 列的序列化与空值语义。
func TestLogs_CacheMirrorIntentRowShape(t *testing.T) {
	var data logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "page_size", "6"), &data)

	require.Len(t, data.Items, 6)
	byID := map[int64]logItem{}
	for _, one := range data.Items {
		byID[*one.LogID] = one
	}

	full := byID[1001]
	require.NotNil(t, full.AICacheStatus)
	assert.Equal(t, "hit", *full.AICacheStatus)
	require.NotNil(t, full.MirrorHit)
	assert.True(t, *full.MirrorHit)
	require.NotNil(t, full.MirrorCluster)
	assert.Equal(t, "mirror-bj", *full.MirrorCluster)
	require.NotNil(t, full.AIIntentQuestion)
	assert.Equal(t, "task_type", *full.AIIntentQuestion)
	require.NotNil(t, full.AIIntentAnswer)
	assert.Equal(t, "coding", *full.AIIntentAnswer)
	require.NotNil(t, full.AIIntentConfidence)
	assert.InDelta(t, 0.95, *full.AIIntentConfidence, 1e-12)
	require.NotNil(t, full.AIIntentSource)
	assert.Equal(t, "classifier", *full.AIIntentSource)
	require.NotNil(t, full.AIIntentLatencyUs)
	assert.Equal(t, int64(1200), *full.AIIntentLatencyUs)
	require.NotNil(t, full.AIIntentCacheHit)
	assert.False(t, *full.AIIntentCacheHit)
	require.NotNil(t, full.AIIntentQuestionsVer)
	assert.Equal(t, "v3", *full.AIIntentQuestionsVer)

	// cache 源：latency_us / cache_hit 为 NULL -> nil。
	fromCache := byID[1005]
	require.NotNil(t, fromCache.AIIntentSource)
	assert.Equal(t, "cache", *fromCache.AIIntentSource)
	assert.Nil(t, fromCache.AIIntentLatencyUs)
	require.NotNil(t, fromCache.AIIntentCacheHit)
	assert.True(t, *fromCache.AIIntentCacheHit)

	// 意图未求值行（1004）：可空列为 NULL/空串。
	none := byID[1004]
	require.NotNil(t, none.AICacheStatus)
	assert.Equal(t, "", *none.AICacheStatus)
	require.NotNil(t, none.MirrorHit)
	assert.False(t, *none.MirrorHit)
	require.NotNil(t, none.MirrorCluster)
	assert.Equal(t, "", *none.MirrorCluster)
	require.NotNil(t, none.AIIntentQuestion)
	assert.Equal(t, "", *none.AIIntentQuestion)
	require.NotNil(t, none.AIIntentAnswer)
	assert.Equal(t, "", *none.AIIntentAnswer)
	assert.Nil(t, none.AIIntentConfidence)
	require.NotNil(t, none.AIIntentSource)
	assert.Equal(t, "", *none.AIIntentSource)
	assert.Nil(t, none.AIIntentLatencyUs)
	assert.Nil(t, none.AIIntentCacheHit)
	require.NotNil(t, none.AIIntentQuestionsVer)
	assert.Equal(t, "", *none.AIIntentQuestionsVer)
}

// TestLogs_PageSizeCap 验证 page_size 超上限按实现语义封顶 100（对齐
// operation-logs 的分页惯例：超限截断而非报错）。
func TestLogs_PageSizeCap(t *testing.T) {
	var data logsData
	getJSON(t, reportPrefix+"/logs", with(windowQuery(), "page_size", "101"), &data)

	assert.Equal(t, 100, data.PageSize)
	assert.Equal(t, int64(6), data.Total)
	require.Len(t, data.Items, 6)
}

// TestParamValidation 验证参数校验错误（xerror PARAM → ErrNum=422）。
func TestParamValidation(t *testing.T) {
	client := testutil.GetClient()

	longKeyword := strings.Repeat("a", 129)

	cases := []struct {
		name  string
		path  string
		query map[string]string
	}{
		{"start_equals_end", "/overview", with(windowQuery(), "start", fmt.Sprint(epoch("2026-09-15 10:05:00")))},
		{"window_over_7_days", "/overview", map[string]string{
			"start": fmt.Sprint(epoch("2026-09-15 10:00:00")),
			"end":   fmt.Sprint(epoch("2026-09-22 10:00:01")),
		}},
		{"missing_end", "/overview", map[string]string{"start": "1782345600"}},
		{"invalid_metric", "/timeseries", with(windowQuery(), "metric", "bogus")},
		{"missing_metric", "/timeseries", windowQuery()},
		{"invalid_ranking_dimension", "/rankings", with(windowQuery(), "dimension", "stream")},
		{"invalid_distribution_dimension", "/distribution", with(windowQuery(), "dimension", "model")},
		{"keyword_too_long", "/logs", with(windowQuery(), "keyword", longKeyword)},
		{"invalid_mirror_hit", "/logs", with(windowQuery(), "mirror_hit", "not-bool")},
		{"invalid_intent_source", "/logs", with(windowQuery(), "intent_source", "bogus")},
		{"invalid_price_warn", "/logs", with(windowQuery(), "price_warn", "bogus")},
		{"invalid_timeseries_dimension", "/timeseries", with(windowQuery(), "metric", "qps", "dimension", "model")},
		// issue #233：stream 非法布尔值（共有过滤项 tag 绑定路径）五个端点全部 422
		{"invalid_stream_overview", "/overview", with(windowQuery(), "stream", "not-bool")},
		{"invalid_stream_logs", "/logs", with(windowQuery(), "stream", "not-bool")},
		{"invalid_stream_timeseries", "/timeseries", with(windowQuery(), "metric", "qps", "stream", "not-bool")},
		{"invalid_stream_rankings", "/rankings", with(windowQuery(), "dimension", "model", "stream", "not-bool")},
		{"invalid_stream_distribution", "/distribution", with(windowQuery(), "dimension", "status", "stream", "not-bool")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Get(reportPrefix+tc.path, tc.query)
			require.NoError(t, err)
			require.NotNil(t, resp)
			testutil.AssertErrCode(t, resp, 422)
			assert.Contains(t, resp.ErrMsg, "Param Illegal")
			if tc.name == "invalid_stream_overview" {
				// 字段感知文案，且不回显 strconv 解析错误原文
				assert.Contains(t, resp.ErrMsg, "invalid stream: not-bool")
				assert.NotContains(t, resp.ErrMsg, "strconv")
			}
		})
	}
}

// TestStreamFilterValidValues 验证 stream 合法取值与缺省行为不回退（issue #233 回归）。
func TestStreamFilterValidValues(t *testing.T) {
	var data overviewData
	// 显式 true/false 与省略 stream 均应 200
	getJSON(t, reportPrefix+"/overview", with(windowQuery(), "stream", "true"), &data)
	getJSON(t, reportPrefix+"/overview", with(windowQuery(), "stream", "false"), &data)
	getJSON(t, reportPrefix+"/overview", windowQuery(), &data)
}
