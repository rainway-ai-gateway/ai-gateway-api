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

// 本文件覆盖报表查询 Backend=clickhouse 装配（三期，
// design-docs modifications/2026-10-02-report-clickhouse-backend），分两组：
//
// 组 1（TestClickHouseBackend_ParamValidation / _EndpointsRegistered）：
// 装配与参数校验门控，借 REPORT_MYSQL_DSN 指向的 MySQL 实例启动
// [Report].Backend="clickhouse" 的 api 进程（数据源仍是 MySQL，仅 backend
// 标识不同，与 testutil.StartReportServerWithBackend 的约定及 doris 组先例
// 一致）。非法 dimension/metric、时间窗 >7 天等参数错误在 manager 层拦截
// （422 Param Illegal，不触达 SQL），端点注册性由"有效请求不再 404"锁定
//（有效请求会穿过 manager 触达 storager，CH 方言 SQL 在 MySQL 上必失败，
// 返回 500 Database Exception——这正是本组与 doris 组的分界：dorisreport
// 的 SQL 恰好是 MySQL 兼容子集，doris 组因此可断言查询数据）。本组不灌种子、
// 不断言任何查询数据。
//
// 组 2（TestClickHouseBackend_* 其余用例）：真实 ClickHouse 端到端。
// 数据源 REPORT_CLICKHOUSE_DSN（clickhouse-go/v2 stdlib DSN，形如
// clickhouse://report_read:pass@127.0.0.1:9000/bfe_observability，库名段
// 会被忽略，管理连接落 default 库）；DDL 目录 REPORT_CLICKHOUSE_DDL_DIR
//（指向 ai-gateway-observability/clickhouse/sqls）。testutil 建临时库
// report_ch_it_<ns>（DROP IF EXISTS + CREATE），顺序套用 bfe_observability.sql、
// bfe_ai_request_log.sql、bfe_ai_metrics_1m.sql（只建两表，无 Kafka 引擎表与
// MV），灌 chAggregateSeedSQL / chDetailSeedSQL（clickhouse_seed_test.go，
// 与 MySQL 组同源的确定性种子），再以 Backend="clickhouse" 起 api 进程，
// 用毕由 t.Cleanup(rs.Close) 停进程并 DROP 临时库。
//
// Skip 条件（仿现有 skip 风格）：
//   - 组 1：REPORT_MYSQL_DSN 未设置（t.Skip，同 doris 组）。
//   - 组 2：REPORT_CLICKHOUSE_DSN / REPORT_CLICKHOUSE_DDL_DIR 未设置、DDL
//     文件缺失、或 ClickHouse 不可达（PingClickHouse 5s 超时）时 t.Skip；
//     另因包级 TestMain 以 REPORT_MYSQL_DSN 为门控，单独运行本文件用例仍需
//     带上 REPORT_MYSQL_DSN（startClickHouseServer 内有兜底提示）。
//
// 组 2 断言口径：overview / timeseries / rankings / distribution 的聚合表
// 驱动指标与 cases_test.go 中 MySQL/doris 组的手算值逐项一致（同一种子，
// 仅时间平移到 2030-09-15 以规避 CH 两表 TTL 7 天，见
// clickhouse_seed_test.go 文件头）；latency 分位数为 t-digest 近似，只断言
// p50/p90/p99 字段存在、不做数值断言，空窗口验证 NaN 守卫（分位数字段省略）；
// logs 端点断言 count / 过滤 / 行形状，跳过 req_headers / ai_rate_limit_hits /
// ai_auth_reject_quota_plans 等复杂类型列的内容断言（CH 列类型为
// Nested / Array(Tuple) / Array(String)，跨引擎序列化形态未锁定）。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
)

// ---------------------------------------------------------------------------
// 组 1：装配与参数校验（借 MySQL 实例，Backend 标识为 clickhouse）
// ---------------------------------------------------------------------------

// startClickHouseGateServer 以 Backend="clickhouse" 装配一个独立 api 进程
// （独立随机库与端口；数据源是 REPORT_MYSQL_DSN 指向的 MySQL 实例），
// REPORT_MYSQL_DSN 未设置时 Skip。门控用例不触达查询数据，故不灌种子。
func startClickHouseGateServer(t *testing.T) *testutil.ReportServer {
	t.Helper()
	if os.Getenv("REPORT_MYSQL_DSN") == "" {
		t.Skip("REPORT_MYSQL_DSN not set; skipping clickhouse backend gate tests.")
	}
	rs, err := testutil.StartReportServerWithBackend("clickhouse")
	if err != nil {
		if errors.Is(err, testutil.ErrReportMySQLDSNNotSet) {
			t.Skip("REPORT_MYSQL_DSN not set; skipping clickhouse backend gate tests.")
		}
		t.Fatalf("setup clickhouse-backend report server failed: %v", err)
	}
	t.Cleanup(rs.Close)
	return rs
}

// clickHouseClient 返回指向给定报表服务进程的专用客户端：本文件内多个
// 用例各自启动独立 api 进程，不能复用指向其他进程的全局客户端。
func clickHouseClient(rs *testutil.ReportServer) *testutil.Client {
	return &testutil.Client{
		BaseURL:    rs.Server.ServerURL,
		HTTPClient: testutil.GetClient().HTTPClient,
		Token:      testutil.GetClient().Token,
	}
}

// getCHJSON 以指定客户端请求报表端点并反序列化 Data，语义同 getJSON。
func getCHJSON(t *testing.T, client *testutil.Client, path string, query map[string]string, out interface{}) *testutil.APIResponse {
	t.Helper()
	resp, err := client.Get(path, query)
	require.NoError(t, err)
	require.NotNil(t, resp)
	testutil.AssertSuccess(t, resp)
	if out != nil {
		require.NoError(t, json.Unmarshal(resp.Data, out), "data: %s", string(resp.Data))
	}
	return resp
}

// TestClickHouseBackend_ParamValidation 验证 clickhouse 装配下 manager 层
// 参数校验与 MySQL/doris 装配一致（查询层拦截、不触达 SQL）：
// 非法 metric/dimension、缺参、时间窗 >7 天、keyword 超长均为
// 422 Param Illegal（与 cases_test.go TestParamValidation 同口径）。
func TestClickHouseBackend_ParamValidation(t *testing.T) {
	rs := startClickHouseGateServer(t)
	client := clickHouseClient(rs)

	longKeyword := strings.Repeat("a", 129)

	cases := []struct {
		name  string
		path  string
		query map[string]string
	}{
		{"window_over_7_days", "/overview", map[string]string{
			"start": fmt.Sprint(epoch("2026-09-15 10:00:00")),
			"end":   fmt.Sprint(epoch("2026-09-22 10:00:01")),
		}},
		{"invalid_metric", "/timeseries", with(windowQuery(), "metric", "bogus")},
		{"missing_metric", "/timeseries", windowQuery()},
		{"invalid_timeseries_dimension", "/timeseries", with(windowQuery(), "metric", "qps", "dimension", "model")},
		{"invalid_ranking_dimension", "/rankings", with(windowQuery(), "dimension", "stream")},
		{"invalid_distribution_dimension", "/distribution", with(windowQuery(), "dimension", "model")},
		{"keyword_too_long", "/logs", with(windowQuery(), "keyword", longKeyword)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Get(reportPrefix+tc.path, tc.query)
			require.NoError(t, err)
			require.NotNil(t, resp)
			testutil.AssertErrCode(t, resp, 422)
			assert.Contains(t, resp.ErrMsg, "Param Illegal")
		})
	}
}

// TestClickHouseBackend_EndpointsRegistered 验证 Backend="clickhouse" 装配后
// 五个报表端点均已注册：有效窗口请求不再落入 mux 的 404（未装配形态）。
// 请求会穿过 manager 到达 storager，CH 方言 SQL（fromUnixTimestamp/intDiv/
// toString 等）在 MySQL 上无法执行，返回 500 Database Exception——该值被
// 精确锁定，作为"已注册且到达 storager 层"的判据；这正是本组与 doris 组
// 的分界（dorisreport SQL 是 MySQL 兼容子集，doris 组可断言数据）。
func TestClickHouseBackend_EndpointsRegistered(t *testing.T) {
	rs := startClickHouseGateServer(t)
	client := clickHouseClient(rs)

	endpoints := []struct {
		name  string
		path  string
		query map[string]string
	}{
		{"overview", "/overview", windowQuery()},
		{"timeseries", "/timeseries", with(windowQuery(), "metric", "qps")},
		{"rankings", "/rankings", with(windowQuery(), "dimension", "model")},
		{"distribution", "/distribution", with(windowQuery(), "dimension", "status")},
		{"logs", "/logs", windowQuery()},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			resp, err := client.Get(reportPrefix+ep.path, ep.query)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, 500, resp.ErrNum,
				"%s must be assembled (not 404); CH dialect SQL cannot run on MySQL, got ErrMsg=%s",
				ep.path, resp.ErrMsg)
			assert.Contains(t, resp.ErrMsg, "Database Exception")
		})
	}
}

// ---------------------------------------------------------------------------
// 组 2：真实 ClickHouse 端到端
// ---------------------------------------------------------------------------

// chWindowQuery 是组 2 的查询窗口 [2030-09-15 09:59, 10:05)（≤6h → 60s 桶），
// 与 windowQuery 同结构；种子时间平移到 2030-09-15 以规避 CH 两表 TTL 7 天
// （见 clickhouse_seed_test.go 文件头）。
func chWindowQuery() map[string]string {
	return map[string]string{
		"start": fmt.Sprintf("%d", epoch("2030-09-15 09:59:00")),
		"end":   fmt.Sprintf("%d", epoch("2030-09-15 10:05:00")),
	}
}

// startClickHouseServer 装配一个 Backend="clickhouse"、数据源为真实
// ClickHouse 实例（临时库 report_ch_it_<ns>，用毕 DROP）的报表服务，
// 返回服务与专用客户端。REPORT_CLICKHOUSE_DSN / REPORT_CLICKHOUSE_DDL_DIR
// 未设置、DDL 文件缺失或 ClickHouse 不可达时 Skip（仿现有 skip 风格）。
func startClickHouseServer(t *testing.T) (*testutil.ReportServer, *testutil.Client) {
	t.Helper()
	dsn := os.Getenv("REPORT_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip(`REPORT_CLICKHOUSE_DSN not set; skipping clickhouse end-to-end integration tests.`)
	}
	ddlDir := os.Getenv("REPORT_CLICKHOUSE_DDL_DIR")
	if ddlDir == "" {
		t.Skip(`REPORT_CLICKHOUSE_DDL_DIR not set (指向 ai-gateway-observability/clickhouse/sqls); ` +
			`skipping clickhouse end-to-end integration tests.`)
	}
	if os.Getenv("REPORT_MYSQL_DSN") == "" {
		// 包级 TestMain 以 REPORT_MYSQL_DSN 为门控，缺省时整个包在 TestMain
		// 阶段即退出；此处兜底仅为将来调整 TestMain 门控后给出明确提示。
		t.Skip("REPORT_MYSQL_DSN not set; 包级 TestMain 门控要求同时设置 REPORT_MYSQL_DSN。")
	}
	for _, file := range []string{"bfe_observability.sql", "bfe_ai_request_log.sql", "bfe_ai_metrics_1m.sql"} {
		if _, err := os.Stat(filepath.Join(ddlDir, file)); err != nil {
			t.Skipf("ClickHouse DDL 文件缺失: %s (REPORT_CLICKHOUSE_DDL_DIR=%s); skipping clickhouse end-to-end integration tests.", file, ddlDir)
		}
	}
	if err := testutil.PingClickHouse(dsn); err != nil {
		t.Skipf("ClickHouse 不可达 (%v); 请先启动实例并检查 REPORT_CLICKHOUSE_DSN，skipping clickhouse end-to-end integration tests.", err)
	}

	rs, err := testutil.StartClickHouseReportServer(dsn, ddlDir, chAggregateSeedSQL, chDetailSeedSQL)
	if err != nil {
		t.Fatalf("setup clickhouse report server failed: %v", err)
	}
	// Close 负责停止 api 进程并 DROP 临时库（级联删除两表）。
	t.Cleanup(rs.Close)
	return rs, clickHouseClient(rs)
}

// chOverviewData 在 overviewData（聚合字段）上扩展 CH 后端的
// latency_p90_ms / latency_p99_ms（p50 字段 overviewData 已含）：
// CH 后端由明细表 quantile t-digest 提供三个分位数（MySQL 后端不返回）。
type chOverviewData struct {
	overviewData
	LatencyP90Ms *float64 `json:"latency_p90_ms"`
	LatencyP99Ms *float64 `json:"latency_p99_ms"`
}

// TestClickHouseBackend_Overview 验证 CH 后端总览指标卡：聚合表合计、成本
// 分组、明细 COUNT 与缓存/镜像/意图组全部精确等于手算值（与 cases_test.go
// 的 MySQL 组逐项一致，同一种子）；latency_p50/p90/p99 只断言字段存在
// （t-digest 近似，不做数值断言）。
func TestClickHouseBackend_Overview(t *testing.T) {
	_, client := startClickHouseServer(t)

	t.Run("seeded_window", func(t *testing.T) {
		var data chOverviewData
		getCHJSON(t, client, reportPrefix+"/overview", chWindowQuery(), &data)

		assert.Equal(t, int64(680), data.RequestTotal)
		assert.Equal(t, int64(200), data.ErrorTotal)
		assert.InDelta(t, 200.0/680.0, data.ErrorRate, 1e-9)
		assert.Equal(t, int64(6600), data.InputTokens)
		assert.Equal(t, int64(1320), data.OutputTokens)
		assert.Equal(t, int64(7920), data.TotalTokens)

		// latency_avg = all_time_sum/request_total = 68000/680 = 100ms；
		// latency_max = MAX(all_time_sum/request_count)（各组均 100ms）。
		assert.InDelta(t, 100, data.LatencyAvgMs, 1e-9)
		assert.InDelta(t, 100, data.LatencyMaxMs, 1e-9)
		// CH 后端返回三个 t-digest 分位数；微数据集体下不做数值断言。
		require.NotNil(t, data.LatencyP50Ms)
		require.NotNil(t, data.LatencyP90Ms)
		require.NotNil(t, data.LatencyP99Ms)

		// ttft/tpot 聚合于 stream 请求（450 个），微秒→毫秒；行 F（非流式
		// ttft_us_sum=40000，issue #315 污染复现行）不得计入分子。
		assert.InDelta(t, 225000.0/450/1000, data.TtftAvgMs, 1e-9) // 0.5ms
		assert.InDelta(t, 22500.0/450/1000, data.TpotAvgMs, 1e-9)  // 0.05ms

		require.Len(t, data.Cost, 2)
		costByCurrency := map[string]float64{}
		for _, one := range data.Cost {
			costByCurrency[one.Currency] = one.Value
		}
		assert.InDelta(t, 3.5e-6, costByCurrency["USD"], 1e-12)
		assert.InDelta(t, 3e-6, costByCurrency["RMB"], 1e-12)

		assert.Equal(t, int64(2), data.RateLimitHits)
		assert.Equal(t, int64(1), data.AuthRejects)
		assert.Equal(t, int64(6), data.LogsTotal) // 明细表 count()

		assert.Equal(t, int64(2), data.Cache.HitCount)
		assert.Equal(t, int64(2), data.Cache.MissCount)
		assert.Equal(t, int64(1), data.Cache.SkipCount)
		assert.InDelta(t, 0.5, data.Cache.HitRate, 1e-9)
		assert.Equal(t, int64(4500), data.Cache.ReadTokens)
		assert.Equal(t, int64(700), data.Cache.WriteTokens)
		assert.Equal(t, int64(2), data.Mirror.HitCount)
		assert.Equal(t, int64(3), data.Intent.ClassifiedCount)
		assert.Equal(t, int64(1), data.Intent.UnknownCount)
		assert.InDelta(t, 0.25, data.Intent.UnknownRate, 1e-9)
	})

	t.Run("filtered", func(t *testing.T) {
		// models + status_codes 过滤项参与聚合口径（gpt-4o + 200：
		// 聚合行 A(100) + D(50)；明细 1001、1006 两行）。
		query := with(chWindowQuery(), "models", "gpt-4o", "status_codes", "200")
		var data chOverviewData
		getCHJSON(t, client, reportPrefix+"/overview", query, &data)

		assert.Equal(t, int64(150), data.RequestTotal)
		assert.Equal(t, int64(0), data.ErrorTotal)
		assert.Equal(t, int64(1500), data.InputTokens)
		assert.Equal(t, int64(2), data.LogsTotal)
	})
}

// TestClickHouseBackend_TimeSeries 验证 CH 后端时序：qps/tokens/cost/
// cache_tokens 与 dimension 拆序列的桶值精确等于手算值（与 MySQL/doris
// 组一致）；latency 时序见 TestClickHouseBackend_LatencyPercentiles。
func TestClickHouseBackend_TimeSeries(t *testing.T) {
	_, client := startClickHouseServer(t)

	t.Run("qps", func(t *testing.T) {
		var data timeseriesData
		getCHJSON(t, client, reportPrefix+"/timeseries", with(chWindowQuery(), "metric", "qps"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 2)
		assert.Equal(t, epoch("2030-09-15 10:00:00"), data.Series[0].Time)
		require.NotNil(t, data.Series[0].Value)
		assert.InDelta(t, 10, *data.Series[0].Value, 1e-9) // 600/60
		assert.Equal(t, epoch("2030-09-15 10:01:00"), data.Series[1].Time)
		require.NotNil(t, data.Series[1].Value)
		assert.InDelta(t, 80.0/60, *data.Series[1].Value, 1e-9) // (50+10+20)/60，含行 F
	})

	t.Run("tokens", func(t *testing.T) {
		var data timeseriesData
		getCHJSON(t, client, reportPrefix+"/timeseries", with(chWindowQuery(), "metric", "tokens"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 2)
		assert.InDelta(t, 100, *data.Series[0].Input, 1e-9) // 6000/60
		assert.InDelta(t, 20, *data.Series[0].Output, 1e-9)
		assert.InDelta(t, 120, *data.Series[0].Total, 1e-9)
		assert.InDelta(t, 10, *data.Series[1].Input, 1e-9) // 600/60
		assert.InDelta(t, 2, *data.Series[1].Output, 1e-9)
		assert.InDelta(t, 12, *data.Series[1].Total, 1e-9)
	})

	t.Run("ttft_tpot", func(t *testing.T) {
		// 流式口径（issue #315）：每桶 = 桶内 ai_stream=1 行合计 ÷ 流式请求数
		// ÷ 1000；行 F（10:01 桶内非流式、ttft_us_sum=40000）不得计入分子。
		for _, tc := range []struct {
			metric   string
			bucket00 float64 // 10:00：(50000+150000)/(100+300)/1000
			bucket01 float64 // 10:01：25000/50/1000
		}{
			{"ttft", 0.5, 0.5},
			{"tpot", 0.05, 0.05},
		} {
			var data timeseriesData
			getCHJSON(t, client, reportPrefix+"/timeseries", with(chWindowQuery(), "metric", tc.metric), &data)

			assert.Equal(t, 60, data.BucketSec)
			require.Len(t, data.Series, 2)
			assert.Equal(t, epoch("2030-09-15 10:00:00"), data.Series[0].Time)
			require.NotNil(t, data.Series[0].Value)
			assert.InDelta(t, tc.bucket00, *data.Series[0].Value, 1e-9)
			assert.Equal(t, epoch("2030-09-15 10:01:00"), data.Series[1].Time)
			require.NotNil(t, data.Series[1].Value)
			assert.InDelta(t, tc.bucket01, *data.Series[1].Value, 1e-9)
		}
	})

	t.Run("cost", func(t *testing.T) {
		var data timeseriesData
		getCHJSON(t, client, reportPrefix+"/timeseries", with(chWindowQuery(), "metric", "cost"), &data)

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
		assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2030-09-15 10:00:00"), "USD"}], 1e-15)
		assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2030-09-15 10:00:00"), "RMB"}], 1e-15)
		assert.InDelta(t, 50.0/60/1e8, byKey[pointKey{epoch("2030-09-15 10:01:00"), "USD"}], 1e-15)
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:01:00"), ""}])
	})

	t.Run("cache_tokens", func(t *testing.T) {
		var data timeseriesData
		getCHJSON(t, client, reportPrefix+"/timeseries", with(chWindowQuery(), "metric", "cache_tokens"), &data)

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
		assert.InDelta(t, 4000.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), "cache_read"}], 1e-9)
		assert.InDelta(t, 600.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), "cache_write"}], 1e-9)
		assert.InDelta(t, 500.0/60, byKey[pointKey{epoch("2030-09-15 10:01:00"), "cache_read"}], 1e-9)
		assert.InDelta(t, 100.0/60, byKey[pointKey{epoch("2030-09-15 10:01:00"), "cache_write"}], 1e-9)
	})

	t.Run("qps_by_cache_status", func(t *testing.T) {
		var data timeseriesData
		getCHJSON(t, client, reportPrefix+"/timeseries",
			with(chWindowQuery(), "metric", "qps", "dimension", "ai_cache_status"), &data)

		assert.Equal(t, 60, data.BucketSec)
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
		assert.InDelta(t, 400.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), "hit"}], 1e-9)
		assert.InDelta(t, 200.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), "miss"}], 1e-9)
		assert.InDelta(t, 50.0/60, byKey[pointKey{epoch("2030-09-15 10:01:00"), "hit"}], 1e-9)
		assert.InDelta(t, 30.0/60, byKey[pointKey{epoch("2030-09-15 10:01:00"), ""}], 1e-9)
	})
}

// TestClickHouseBackend_Rankings 验证 CH 后端排行：聚合表 GROUP BY 维度 +
// SUM(指标) 口径与 MySQL/doris 组手算值一致（数值维度 toString 转名、
// 空值不进排行、mirror_hit 0/1 双桶都进排行的既有口径在 CH 模板下不变）。
func TestClickHouseBackend_Rankings(t *testing.T) {
	_, client := startClickHouseServer(t)

	t.Run("model", func(t *testing.T) {
		var data rankingsData
		getCHJSON(t, client, reportPrefix+"/rankings", with(chWindowQuery(), "dimension", "model"), &data)

		require.Len(t, data.Items, 4)
		assert.Equal(t, "gpt-4o", data.Items[0].Name)
		assert.Equal(t, int64(350), data.Items[0].RequestCount)
		assert.Equal(t, int64(200), data.Items[0].ErrorCount)
		assert.Equal(t, int64(3500), data.Items[0].InputTokens)
		assert.Equal(t, int64(700), data.Items[0].OutputTokens)
		assert.Equal(t, "gpt-4", data.Items[1].Name)
		assert.Equal(t, int64(300), data.Items[1].RequestCount)
		assert.Equal(t, "llama-3", data.Items[2].Name) // 行 F
		assert.Equal(t, int64(20), data.Items[2].RequestCount)
		assert.Equal(t, "claude", data.Items[3].Name)
		assert.Equal(t, int64(10), data.Items[3].RequestCount)
	})

	t.Run("status", func(t *testing.T) {
		var data rankingsData
		getCHJSON(t, client, reportPrefix+"/rankings", with(chWindowQuery(), "dimension", "status"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "200", data.Items[0].Name)
		assert.Equal(t, int64(480), data.Items[0].RequestCount)
		assert.Equal(t, "500", data.Items[1].Name)
		assert.Equal(t, int64(200), data.Items[1].RequestCount)
	})

	t.Run("cache_status", func(t *testing.T) {
		// 空值（''）不进排行。
		var data rankingsData
		getCHJSON(t, client, reportPrefix+"/rankings", with(chWindowQuery(), "dimension", "ai_cache_status"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "hit", data.Items[0].Name)
		assert.Equal(t, int64(450), data.Items[0].RequestCount)
		assert.Equal(t, "miss", data.Items[1].Name)
		assert.Equal(t, int64(200), data.Items[1].RequestCount)
	})

	t.Run("mirror_hit", func(t *testing.T) {
		var data rankingsData
		getCHJSON(t, client, reportPrefix+"/rankings", with(chWindowQuery(), "dimension", "mirror_hit"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "0", data.Items[0].Name)
		assert.Equal(t, int64(530), data.Items[0].RequestCount) // B+C+E+F
		assert.Equal(t, "1", data.Items[1].Name)
		assert.Equal(t, int64(150), data.Items[1].RequestCount) // A+D
	})

	t.Run("intent_answer", func(t *testing.T) {
		var data rankingsData
		getCHJSON(t, client, reportPrefix+"/rankings", with(chWindowQuery(), "dimension", "ai_intent_answer"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]int64{}
		for _, one := range data.Items {
			byName[one.Name] = one.RequestCount
		}
		assert.Equal(t, int64(200), byName["unknown"])
		assert.Equal(t, int64(300), byName["writing"])
		assert.Equal(t, int64(150), byName["coding"])
		assert.NotContains(t, byName, "") // 行 E 的空值不进排行
	})
}

// TestClickHouseBackend_Distribution 验证 CH 后端分布：CASE 归一空值桶 +
// ratio 口径与 MySQL/doris 组手算值一致。
func TestClickHouseBackend_Distribution(t *testing.T) {
	_, client := startClickHouseServer(t)

	t.Run("status", func(t *testing.T) {
		var data distributionData
		getCHJSON(t, client, reportPrefix+"/distribution", with(chWindowQuery(), "dimension", "status"), &data)

		require.Len(t, data.Items, 2)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(480), byName["200"].RequestCount)
		assert.InDelta(t, 480.0/680.0, byName["200"].Ratio, 1e-9)
		assert.Equal(t, int64(200), byName["500"].RequestCount)
		assert.InDelta(t, 200.0/680.0, byName["500"].Ratio, 1e-9)
		assert.InDelta(t, 1, byName["200"].Ratio+byName["500"].Ratio, 1e-9)
	})

	t.Run("protocol_unknown_bucket", func(t *testing.T) {
		var data distributionData
		getCHJSON(t, client, reportPrefix+"/distribution", with(chWindowQuery(), "dimension", "protocol"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(650), byName["openai"].RequestCount)
		assert.Equal(t, int64(20), byName["anthropic"].RequestCount) // 行 F
		assert.Equal(t, int64(10), byName["unknown"].RequestCount)
		assert.InDelta(t, 10.0/680.0, byName["unknown"].Ratio, 1e-9)
	})

	t.Run("cache_status", func(t *testing.T) {
		var data distributionData
		getCHJSON(t, client, reportPrefix+"/distribution", with(chWindowQuery(), "dimension", "ai_cache_status"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(450), byName["hit"].RequestCount)
		assert.InDelta(t, 450.0/680.0, byName["hit"].Ratio, 1e-9)
		assert.Equal(t, int64(200), byName["miss"].RequestCount)
		assert.Equal(t, int64(30), byName["unknown"].RequestCount)
	})

	t.Run("mirror_hit", func(t *testing.T) {
		var data distributionData
		getCHJSON(t, client, reportPrefix+"/distribution", with(chWindowQuery(), "dimension", "mirror_hit"), &data)

		require.Len(t, data.Items, 2)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(530), byName["0"].RequestCount)
		assert.Equal(t, int64(150), byName["1"].RequestCount)
		assert.InDelta(t, 150.0/680.0, byName["1"].Ratio, 1e-9)
	})
}

// TestClickHouseBackend_Logs 验证 CH 后端明细：count / 分页 / 过滤与行形状。
// 复杂类型列（req_headers/res_headers 为 Nested，ai_rate_limit_hits 为
// Array(Tuple)，ai_auth_reject_quota_plans 为 Array(String)）只做种子写入
// （见 clickhouse_seed_test.go 偏差 2-4），响应序列化形态未锁定，跳过其
// 内容断言；NULL 语义的 String 列（1004 的 ai_apikey_id/ai_cost_currency）
// 在 CH 为 ”（非 NULL 列），按 CH 语义断言。
func TestClickHouseBackend_Logs(t *testing.T) {
	_, client := startClickHouseServer(t)

	t.Run("paging", func(t *testing.T) {
		var page1 logsData
		getCHJSON(t, client, reportPrefix+"/logs", with(chWindowQuery(), "page", "1", "page_size", "2"), &page1)

		assert.Equal(t, int64(6), page1.Total)
		assert.Equal(t, 1, page1.Page)
		assert.Equal(t, 2, page1.PageSize)
		require.Len(t, page1.Items, 2)
		// log_time 倒序：10:02:00(1006) > 10:01:10(1005)。
		assert.Equal(t, int64(1006), *page1.Items[0].LogID)
		assert.Equal(t, int64(1005), *page1.Items[1].LogID)

		var page2 logsData
		getCHJSON(t, client, reportPrefix+"/logs", with(chWindowQuery(), "page", "2", "page_size", "2"), &page2)
		require.Len(t, page2.Items, 2)
		assert.Equal(t, int64(1004), *page2.Items[0].LogID)
		assert.Equal(t, int64(1003), *page2.Items[1].LogID)
	})

	t.Run("filters_err_keyword", func(t *testing.T) {
		var errOnly logsData
		getCHJSON(t, client, reportPrefix+"/logs", with(chWindowQuery(), "err_only", "true"), &errOnly)
		assert.Equal(t, int64(2), errOnly.Total)
		require.Len(t, errOnly.Items, 2)
		assert.Equal(t, int64(1004), *errOnly.Items[0].LogID)
		assert.Equal(t, int64(1002), *errOnly.Items[1].LogID)

		var keyword logsData
		getCHJSON(t, client, reportPrefix+"/logs", with(chWindowQuery(), "keyword", "timeout"), &keyword)
		assert.Equal(t, int64(1), keyword.Total)
		require.Len(t, keyword.Items, 1)
		assert.Equal(t, int64(1002), *keyword.Items[0].LogID)
		require.NotNil(t, keyword.Items[0].ErrMsg)
		assert.Contains(t, *keyword.Items[0].ErrMsg, "timeout")
	})

	t.Run("requested_models", func(t *testing.T) {
		var data logsData
		getCHJSON(t, client, reportPrefix+"/logs", with(chWindowQuery(), "requested_models", "claude-3"), &data)
		assert.Equal(t, int64(1), data.Total)
		require.Len(t, data.Items, 1)
		assert.Equal(t, int64(1005), *data.Items[0].LogID)
	})

	t.Run("cache_mirror_intent_filters", func(t *testing.T) {
		// 与 cases_test.go 的过滤口径表一致（明细驱动，与后端方言无关）。
		cases := []struct {
			name     string
			query    map[string]string
			expectID []int64
		}{
			{"cache_status_hit", with(chWindowQuery(), "cache_status", "hit"), []int64{1003, 1001}},
			{"cache_status_skip", with(chWindowQuery(), "cache_status", "skip"), []int64{1005}},
			{"mirror_hit_true", with(chWindowQuery(), "mirror_hit", "true"), []int64{1005, 1001}},
			{"mirror_hit_false", with(chWindowQuery(), "mirror_hit", "false"), []int64{1006, 1004, 1003, 1002}},
			{"intent_answer_coding", with(chWindowQuery(), "intent_answer", "coding"), []int64{1005, 1001}},
			{"intent_answer_unknown", with(chWindowQuery(), "intent_answer", "unknown"), []int64{1002}},
			{"intent_source_cache", with(chWindowQuery(), "intent_source", "cache"), []int64{1005}},
			{"combined", with(chWindowQuery(), "cache_status", "hit", "intent_answer", "coding"), []int64{1001}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var data logsData
				getCHJSON(t, client, reportPrefix+"/logs", tc.query, &data)

				assert.Equal(t, int64(len(tc.expectID)), data.Total)
				require.Len(t, data.Items, len(tc.expectID))
				for i, id := range tc.expectID {
					assert.Equal(t, id, *data.Items[i].LogID, "position %d", i)
				}
			})
		}
	})

	t.Run("row_shape", func(t *testing.T) {
		var data logsData
		getCHJSON(t, client, reportPrefix+"/logs", with(chWindowQuery(), "page_size", "6"), &data)

		require.Len(t, data.Items, 6)
		byID := map[int64]logItem{}
		for _, one := range data.Items {
			byID[*one.LogID] = one
		}

		// log_time 为 Unix 秒（toUnixTimestamp，UTC epoch）。
		newest := byID[1006]
		assert.Equal(t, epoch("2030-09-15 10:02:00"), newest.LogTime)
		require.NotNil(t, newest.Hostid)
		assert.Equal(t, "gw-01", *newest.Hostid)
		require.NotNil(t, newest.Product)
		assert.Equal(t, "BFE", *newest.Product)

		// 1004（未认证行）：CH 明细表 ai_apikey_id 为 String DEFAULT ''
		//（非 NULL 列），NULL 种子落入 '' → 响应为 ""（MySQL 组为 null，偏差）。
		unauthenticated := byID[1004]
		require.NotNil(t, unauthenticated.APIKeyID)
		assert.Equal(t, "", *unauthenticated.APIKeyID)
		require.NotNil(t, unauthenticated.ErrCode)
		assert.Equal(t, "E401", *unauthenticated.ErrCode)

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

		// 零成本行：1004 的 currency 为 NULL → CH 落入 ''（响应 ""，值为 0）；
		// 1005 的空串语义两引擎一致。
		noCurrency := byID[1004]
		require.NotNil(t, noCurrency.CostValue)
		assert.Equal(t, 0.0, *noCurrency.CostValue)
		require.NotNil(t, noCurrency.CostCurrency)
		assert.Equal(t, "", *noCurrency.CostCurrency)
		emptyCurrency := byID[1005]
		require.NotNil(t, emptyCurrency.CostValue)
		assert.Equal(t, 0.0, *emptyCurrency.CostValue)
		require.NotNil(t, emptyCurrency.CostCurrency)
		assert.Equal(t, "", *emptyCurrency.CostCurrency)

		// 缓存/镜像/意图新 10 列：1001 classifier 源全量字段。
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

		// 1005 cache 源：latency_us 为 NULL -> nil；cache_hit=true。
		fromCache := byID[1005]
		require.NotNil(t, fromCache.AIIntentSource)
		assert.Equal(t, "cache", *fromCache.AIIntentSource)
		assert.Nil(t, fromCache.AIIntentLatencyUs)
		require.NotNil(t, fromCache.AIIntentCacheHit)
		assert.True(t, *fromCache.AIIntentCacheHit)

		// 1004 意图未求值行：可空列为 NULL/空串。
		none := byID[1004]
		require.NotNil(t, none.AICacheStatus)
		assert.Equal(t, "", *none.AICacheStatus)
		require.NotNil(t, none.MirrorHit)
		assert.False(t, *none.MirrorHit)
		assert.Nil(t, none.AIIntentConfidence)
		require.NotNil(t, none.AIIntentSource)
		assert.Equal(t, "", *none.AIIntentSource)
		assert.Nil(t, none.AIIntentLatencyUs)
		assert.Nil(t, none.AIIntentCacheHit)

		// 复杂列 JSON 文本：1002 的 req_headers 由 flatten_nested 物理子列
		//（req_headers.key/value）经 arrayMap 重组为具名 Tuple 后 toJSONString，
		// 形态与 MySQL JSON 文本一致；限流命中为 Array(Tuple) 直出 JSON；
		// 1004 的配额计划为 JSON 字符串数组；空数组行渲染 "[]"（MySQL 侧
		// 为 null，语义等价）。
		withHeaders := byID[1002]
		require.NotNil(t, withHeaders.ReqHeaders)
		assert.JSONEq(t, `[{"key":"X-Test","value":"1"}]`, *withHeaders.ReqHeaders)
		require.NotNil(t, withHeaders.RateLimitHits)
		assert.Contains(t, *withHeaders.RateLimitHits, "rate_limit_policy_id")
		assert.Contains(t, *withHeaders.RateLimitHits, "p1")
		withPlans := byID[1004]
		require.NotNil(t, withPlans.AuthRejectQuotaPlans)
		assert.Contains(t, *withPlans.AuthRejectQuotaPlans, "plan-a")
		emptyHeaders := byID[1001]
		require.NotNil(t, emptyHeaders.ReqHeaders)
		assert.Equal(t, "[]", *emptyHeaders.ReqHeaders)
	})
}

// chMetricPoint / chTimeseriesData 与 metricPoint / timeseriesData 同构，扩展
// CH 后端 latency 时序的 avg/max/p50/p90/p99 字段（MySQL 组无分位数，
// query_test.go 的共享结构未含这些字段）。
type chMetricPoint struct {
	Time     int64    `json:"time"`
	Value    *float64 `json:"value"`
	Input    *float64 `json:"input"`
	Output   *float64 `json:"output"`
	Total    *float64 `json:"total"`
	Avg      *float64 `json:"avg"`
	Max      *float64 `json:"max"`
	P50      *float64 `json:"p50"`
	P90      *float64 `json:"p90"`
	P99      *float64 `json:"p99"`
	Currency string   `json:"currency"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
}

type chTimeseriesData struct {
	BucketSec int             `json:"bucket_sec"`
	Series    []chMetricPoint `json:"series"`
}

// TestClickHouseBackend_LatencyPercentiles 验证 CH 后端延迟分位数：
//   - timeseries?metric=latency 的桶点由聚合表提供 avg/max（精确值断言），
//     并按桶从明细表合并 p50/p90/p99（t-digest）——微数据集下近似算法不保证
//     精确值，只断言三个分位数字段存在；
//   - 空窗口（超出种子范围）overview 不返回分位数字段，验证 NaN 守卫
//     （CH quantile 对空输入返回 NaN，扫描层丢弃后与 Doris 空窗口行为对齐）。
func TestClickHouseBackend_LatencyPercentiles(t *testing.T) {
	_, client := startClickHouseServer(t)

	t.Run("timeseries_latency_percentile_fields_present", func(t *testing.T) {
		var data chTimeseriesData
		getCHJSON(t, client, reportPrefix+"/timeseries", with(chWindowQuery(), "metric", "latency"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 2)

		// 聚合 avg/max：10:00 桶 all_time_sum=60000/request_count=600；
		// 10:01 桶 6000/60；各组 latency_max 均 100ms。
		assert.Equal(t, epoch("2030-09-15 10:00:00"), data.Series[0].Time)
		require.NotNil(t, data.Series[0].Avg)
		assert.InDelta(t, 100, *data.Series[0].Avg, 1e-9)
		require.NotNil(t, data.Series[0].Max)
		assert.InDelta(t, 100, *data.Series[0].Max, 1e-9)
		assert.Equal(t, epoch("2030-09-15 10:01:00"), data.Series[1].Time)
		require.NotNil(t, data.Series[1].Avg)
		assert.InDelta(t, 100, *data.Series[1].Avg, 1e-9)

		// t-digest 分位数只断言字段存在，不做数值断言。
		for i, point := range data.Series {
			require.NotNil(t, point.P50, "series %d", i)
			require.NotNil(t, point.P90, "series %d", i)
			require.NotNil(t, point.P99, "series %d", i)
		}
	})

	t.Run("overview_empty_window_nan_guard", func(t *testing.T) {
		// 窗口完全超出种子范围：CH quantile 返回 NaN，扫描层守卫后字段省略。
		query := map[string]string{
			"start": fmt.Sprint(epoch("2030-09-15 12:00:00")),
			"end":   fmt.Sprint(epoch("2030-09-15 13:00:00")),
		}
		var data chOverviewData
		getCHJSON(t, client, reportPrefix+"/overview", query, &data)

		assert.Equal(t, int64(0), data.RequestTotal)
		assert.Nil(t, data.LatencyP50Ms)
		assert.Nil(t, data.LatencyP90Ms)
		assert.Nil(t, data.LatencyP99Ms)
	})
}
