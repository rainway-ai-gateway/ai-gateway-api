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

// 本文件覆盖报表查询 Backend=starrocks 装配（四期，design-docs
// modifications/2026-10-02-report-starrocks-backend），分两组：
//
// 组 1（TestStarRocksBackend_ParamValidation / *_OnMySQL）：借
// REPORT_MYSQL_DSN 指向的 MySQL 实例启动 [Report].Backend="starrocks"
// 的 api 进程（数据源与种子同 MySQL/doris 组，仅 backend 标识不同，与
// testutil.StartReportServerWithBackend 的约定一致）。starrocksreport
// 是 dorisreport 同源克隆、SQL 为 MySQL 兼容子集（CAST AS SIGNED 桶表达式
// 已实机验证被 MySQL 8.4 与 SR 3.5.21 同时接受），故本组可真实执行 SQL 并
// 断言五端点中不依赖 percentile_approx 的数据，口径与 cases_test.go 的
// MySQL 组手算值逐项一致（rankings / distribution / timeseries(qps/
// tokens/cost/cache_tokens/dimension) / logs）。排除说明：overview 与
// timeseries?metric=latency 的百分位路径调用 percentile_approx，MySQL 8.4
// 无该函数（endpoint 500），借实例组不覆盖，与 doris_dimensions_test.go 的
// 排除同理；percentile_approx 路径由组 2 真实 SR 覆盖。
//
// 组 2（TestStarRocksBackend_* 其余用例）：真实 StarRocks 端到端。
// 数据源 REPORT_STARROCKS_DSN（go-sql-driver DSN，形如
// root:pass@tcp(127.0.0.1:9030)/，SR FE 的 MySQL 协议端口，库名段允许为空）；
// DDL 目录 REPORT_STARROCKS_DDL_DIR（指向 ai-gateway-observability/
// starrocks/sqls）。testutil 建临时库 report_sr_it_<ns>（DROP IF EXISTS +
// CREATE），顺序套用 bfe_observability.sql / bfe_ai_request_log.sql /
// bfe_ai_metrics_1m.sql（占位符 ${STARROCKS_DATABASE} 与
// ${INIT_PARTITION_DATE} 替换），灌 srDetailSeedSQL（starrocks_seed_test.go，
// 只灌基表 bfe_ai_request_log），轮询异步物化视图 bfe_ai_metrics_1m 刷新到
// 6 行后再以 Backend="starrocks" 起 api 进程，用毕由 t.Cleanup(rs.Close)
// 停进程并 DROP 临时库（SR DROP DATABASE 级联删表与物化视图）。
//
// 物化视图种子路径的确认过程与最终选择（实机验证，2026-10-02，SR 3.5.21）：
// bfe_ai_metrics_1m 是异步物化视图（information_schema.tables 中
// TABLE_TYPE=VIEW；SHOW CREATE TABLE 显示 REFRESH ASYNC EVERY (INTERVAL
// 1 MINUTE) + PARTITION BY ts_day + partition_ttl "7 DAY"），直插被 FE
// 拒绝（"The data of 'bfe_ai_metrics_1m' cannot be inserted because ...
// is a materialized view"）。因此聚合种子不走 MV 直插：明细种子直插基表，
// 由 MV 按 40 维分钟桶聚合（端到端 1~2 分钟，实测约 34s），testutil 轮询
// MV 行数（COUNT(*)）达标后才启动 api，组 2 用例内不再有刷新时序问题。
// 由此带来的断言口径变化：聚合端点（overview/timeseries/rankings/
// distribution）的期望值是"6 行明细经 MV 的派生手算值"，而非 MySQL/doris
// 组的聚合表手算值（680 请求口径）——手算值表见 starrocks_seed_test.go
// 文件头。另：种子时间平移到 2030-09-15 以规避 MV partition_ttl 7 天，
// 基表动态分区仅维护 [今天-7, 今天+3]，2030 行靠 ${INIT_PARTITION_DATE}
// 替换为 2030-09-16 落入 p_init 分区。
//
// Skip 条件（仿现有 skip 风格）：
//   - 组 1：REPORT_MYSQL_DSN 未设置（t.Skip，同 doris 组）。
//   - 组 2：REPORT_STARROCKS_DSN / REPORT_STARROCKS_DDL_DIR 未设置、DDL
//     文件缺失、或 StarRocks 不可达（PingStarRocks 5s 超时）时 t.Skip；
//     另因包级 TestMain 以 REPORT_MYSQL_DSN 为门控，单独运行本文件用例仍需
//     带上 REPORT_MYSQL_DSN（startStarRocksServer 内有兜底提示）。
//
// 组 2 断言口径：overview / timeseries / rankings / distribution 的聚合
// 驱动指标与 srDetailSeedSQL 的 MV 派生手算值逐项一致（见
// starrocks_seed_test.go 头表）；latency 分位数 percentile_approx 已实机
// 验证对微数据集精确（窗口 6 值 p50=80；10:00 桶 4 值 p50=150——设计文档
// 所记"p50=150"即桶口径），p50 精确断言，p90/p99 只断言字段存在（p90 为素
// 描近似，实机窗口 p90≈289.99994）；空窗口 percentile_approx 返回 NULL，
// 扫描层省略分位数字段（实机验证）。logs 端点断言 count / 过滤 / 行形状，
// 并对复杂列做内容断言：SR 明细表的 req_headers/res_headers/
// ai_rate_limit_hits 为 VARCHAR(JSON 文本)原样回显，ai_auth_reject_quota_plans
// 为原生 ARRAY<VARCHAR>经 MySQL 协议返回 JSON 文本（实机：["plan-a"]），
// 与 Doris JSON 列线格式一致。
//
// 环境互斥：SR 与 Doris 共用 9030 端口（environment/starrocks-installation.md
// 声明），真实实例组与 Doris 组须错峰运行；借 MySQL 组不受影响。
// 与 CH 组的结构差异：CH 组聚合表是普通表可直插两种子、无刷新等待；
// SR 组单源明细种子 + MV 轮询等待，聚合期望值改为 MV 派生值；CH 的
// flatten_nested/别名遮蔽/toJSONString/NaN 守卫等陷阱均为 CH 特有，SR 组
// 无对应处理（NULL 语义同 MySQL 组，而非 CH 偏差 5 的 '' 语义）。

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
// 组 1：借 MySQL 实例的数据断言（Backend 标识为 starrocks）
// ---------------------------------------------------------------------------

// startStarRocksMySQLServer 以 Backend="starrocks" 装配一个独立 api 进程
// （独立随机库与端口；数据源是 REPORT_MYSQL_DSN 指向的 MySQL 实例，
// 种子表与 MySQL/doris 组相同），REPORT_MYSQL_DSN 未设置时 Skip。
func startStarRocksMySQLServer(t *testing.T) *testutil.ReportServer {
	t.Helper()
	if os.Getenv("REPORT_MYSQL_DSN") == "" {
		t.Skip("REPORT_MYSQL_DSN not set; skipping starrocks backend integration tests.")
	}
	rs, err := testutil.StartReportServerWithBackend("starrocks", aggregateSeedSQL, detailSeedSQL)
	if err != nil {
		if errors.Is(err, testutil.ErrReportMySQLDSNNotSet) {
			t.Skip("REPORT_MYSQL_DSN not set; skipping starrocks backend integration tests.")
		}
		t.Fatalf("setup starrocks-backend report server failed: %v", err)
	}
	t.Cleanup(rs.Close)
	return rs
}

// starRocksClient 返回指向给定报表服务进程的专用客户端：本文件内多个
// 用例各自启动独立 api 进程，不能复用指向其他进程的全局客户端。
func starRocksClient(rs *testutil.ReportServer) *testutil.Client {
	return &testutil.Client{
		BaseURL:    rs.Server.ServerURL,
		HTTPClient: testutil.GetClient().HTTPClient,
		Token:      testutil.GetClient().Token,
	}
}

// getSRJSON 以指定客户端请求报表端点并反序列化 Data，语义同 getJSON。
func getSRJSON(t *testing.T, client *testutil.Client, path string, query map[string]string, out interface{}) *testutil.APIResponse {
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

// TestStarRocksBackend_ParamValidation 验证 starrocks 装配下 manager 层
// 参数校验与 MySQL/doris/clickhouse 装配一致（查询层拦截、不触达 SQL）：
// 非法 metric/dimension、缺参、时间窗 >7 天、keyword 超长均为
// 422 Param Illegal（与 cases_test.go TestParamValidation 同口径）。
func TestStarRocksBackend_ParamValidation(t *testing.T) {
	rs := startStarRocksMySQLServer(t)
	client := starRocksClient(rs)

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

// TestStarRocksBackend_RankingsOnMySQL 验证 starrocksreport 的 rankings
// SQL（gendry builder + CAST AS CHAR 数值维度 + 空值排除 + LIMIT 内联，
// MySQL 兼容子集）在真实 MySQL 上执行并与 MySQL/doris 组手算值一致。
func TestStarRocksBackend_RankingsOnMySQL(t *testing.T) {
	rs := startStarRocksMySQLServer(t)
	client := starRocksClient(rs)
	window := windowQuery()

	t.Run("model", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(window, "dimension", "model"), &data)

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
		getSRJSON(t, client, reportPrefix+"/rankings", with(window, "dimension", "status"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "200", data.Items[0].Name)
		assert.Equal(t, int64(480), data.Items[0].RequestCount)
		assert.Equal(t, "500", data.Items[1].Name)
		assert.Equal(t, int64(200), data.Items[1].RequestCount)
	})

	t.Run("cache_status", func(t *testing.T) {
		// 空值（''）不进排行。
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(window, "dimension", "ai_cache_status"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "hit", data.Items[0].Name)
		assert.Equal(t, int64(450), data.Items[0].RequestCount)
		assert.Equal(t, "miss", data.Items[1].Name)
		assert.Equal(t, int64(200), data.Items[1].RequestCount)
	})

	t.Run("mirror_hit", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(window, "dimension", "mirror_hit"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "0", data.Items[0].Name)
		assert.Equal(t, int64(530), data.Items[0].RequestCount) // B+C+E+F
		assert.Equal(t, "1", data.Items[1].Name)
		assert.Equal(t, int64(150), data.Items[1].RequestCount) // A+D
	})

	t.Run("intent_answer", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(window, "dimension", "ai_intent_answer"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]int64{}
		for _, one := range data.Items {
			byName[one.Name] = one.RequestCount
		}
		assert.Equal(t, int64(200), byName["unknown"])
		assert.Equal(t, int64(300), byName["writing"])
		assert.Equal(t, int64(150), byName["coding"])
		assert.NotContains(t, byName, "") // 行 E/F 的空值不进排行
	})
}

// TestStarRocksBackend_DistributionOnMySQL 验证 starrocksreport 的
// distribution SQL（CASE 归一空值桶 + ratio 口径，MySQL 兼容子集）在真实
// MySQL 上执行并与 MySQL/doris 组手算值一致。
func TestStarRocksBackend_DistributionOnMySQL(t *testing.T) {
	rs := startStarRocksMySQLServer(t)
	client := starRocksClient(rs)
	window := windowQuery()

	t.Run("status", func(t *testing.T) {
		var data distributionData
		getSRJSON(t, client, reportPrefix+"/distribution", with(window, "dimension", "status"), &data)

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
		getSRJSON(t, client, reportPrefix+"/distribution", with(window, "dimension", "protocol"), &data)

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
		getSRJSON(t, client, reportPrefix+"/distribution", with(window, "dimension", "ai_cache_status"), &data)

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
		getSRJSON(t, client, reportPrefix+"/distribution", with(window, "dimension", "mirror_hit"), &data)

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

// TestStarRocksBackend_TimeSeriesOnMySQL 验证 starrocksreport 的时序 SQL
// （CAST AS SIGNED 桶表达式 + UNION ALL cache_tokens 双臂 + LIMIT/OFFSET
// 内联，均为 MySQL 兼容子集）在真实 MySQL 上执行并与 MySQL/doris 组手算
// 值一致。
func TestStarRocksBackend_TimeSeriesOnMySQL(t *testing.T) {
	rs := startStarRocksMySQLServer(t)
	client := starRocksClient(rs)
	window := windowQuery()

	t.Run("qps", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(window, "metric", "qps"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 2)
		assert.Equal(t, epoch("2026-09-15 10:00:00"), data.Series[0].Time)
		require.NotNil(t, data.Series[0].Value)
		assert.InDelta(t, 10, *data.Series[0].Value, 1e-9) // 600/60
		assert.Equal(t, epoch("2026-09-15 10:01:00"), data.Series[1].Time)
		require.NotNil(t, data.Series[1].Value)
		assert.InDelta(t, 80.0/60, *data.Series[1].Value, 1e-9) // (50+10+20)/60，含行 F
	})

	t.Run("tokens", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(window, "metric", "tokens"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 2)
		assert.InDelta(t, 100, *data.Series[0].Input, 1e-9) // 6000/60
		assert.InDelta(t, 20, *data.Series[0].Output, 1e-9)
		assert.InDelta(t, 120, *data.Series[0].Total, 1e-9)
		assert.InDelta(t, 10, *data.Series[1].Input, 1e-9) // 600/60
		assert.InDelta(t, 2, *data.Series[1].Output, 1e-9)
		assert.InDelta(t, 12, *data.Series[1].Total, 1e-9)
	})

	t.Run("cost", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(window, "metric", "cost"), &data)

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
		assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2026-09-15 10:00:00"), "USD"}], 1e-15)
		assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2026-09-15 10:00:00"), "RMB"}], 1e-15)
		assert.InDelta(t, 50.0/60/1e8, byKey[pointKey{epoch("2026-09-15 10:01:00"), "USD"}], 1e-15)
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2026-09-15 10:01:00"), ""}])
	})

	t.Run("cache_tokens", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(window, "metric", "cache_tokens"), &data)

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
		assert.InDelta(t, 4000.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "cache_read"}], 1e-9)
		assert.InDelta(t, 600.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "cache_write"}], 1e-9)
		assert.InDelta(t, 500.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), "cache_read"}], 1e-9)
		assert.InDelta(t, 100.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), "cache_write"}], 1e-9)
	})

	t.Run("qps_by_cache_status", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries",
			with(window, "metric", "qps", "dimension", "ai_cache_status"), &data)

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
		assert.InDelta(t, 400.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "hit"}], 1e-9)
		assert.InDelta(t, 200.0/60, byKey[pointKey{epoch("2026-09-15 10:00:00"), "miss"}], 1e-9)
		assert.InDelta(t, 50.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), "hit"}], 1e-9)
		assert.InDelta(t, 30.0/60, byKey[pointKey{epoch("2026-09-15 10:01:00"), ""}], 1e-9)
	})
}

// TestStarRocksBackend_LogsOnMySQL 验证 starrocksreport 的明细 SQL（47 列
// 投影 + log_time 的 TIMESTAMPDIFF epoch 渲染 + 新过滤项，均 MySQL 兼容）
// 在真实 MySQL 上执行并与 cases_test.go 的手算值一致（NULL 语义同 MySQL
// 组）。
func TestStarRocksBackend_LogsOnMySQL(t *testing.T) {
	rs := startStarRocksMySQLServer(t)
	client := starRocksClient(rs)
	window := windowQuery()

	t.Run("paging", func(t *testing.T) {
		var page1 logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(window, "page", "1", "page_size", "2"), &page1)

		assert.Equal(t, int64(6), page1.Total)
		assert.Equal(t, 1, page1.Page)
		assert.Equal(t, 2, page1.PageSize)
		require.Len(t, page1.Items, 2)
		// log_time 倒序：10:02:00(1006) > 10:01:10(1005)。
		assert.Equal(t, int64(1006), *page1.Items[0].LogID)
		assert.Equal(t, int64(1005), *page1.Items[1].LogID)

		var page2 logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(window, "page", "2", "page_size", "2"), &page2)
		require.Len(t, page2.Items, 2)
		assert.Equal(t, int64(1004), *page2.Items[0].LogID)
		assert.Equal(t, int64(1003), *page2.Items[1].LogID)
	})

	t.Run("filters_err_keyword", func(t *testing.T) {
		var errOnly logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(window, "err_only", "true"), &errOnly)
		assert.Equal(t, int64(2), errOnly.Total)
		require.Len(t, errOnly.Items, 2)
		assert.Equal(t, int64(1004), *errOnly.Items[0].LogID)
		assert.Equal(t, int64(1002), *errOnly.Items[1].LogID)

		var keyword logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(window, "keyword", "timeout"), &keyword)
		assert.Equal(t, int64(1), keyword.Total)
		require.Len(t, keyword.Items, 1)
		assert.Equal(t, int64(1002), *keyword.Items[0].LogID)
		require.NotNil(t, keyword.Items[0].ErrMsg)
		assert.Contains(t, *keyword.Items[0].ErrMsg, "timeout")
	})

	t.Run("requested_models", func(t *testing.T) {
		var data logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(window, "requested_models", "claude-3"), &data)
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
			{"cache_status_hit", with(window, "cache_status", "hit"), []int64{1003, 1001}},
			{"cache_status_skip", with(window, "cache_status", "skip"), []int64{1005}},
			{"mirror_hit_true", with(window, "mirror_hit", "true"), []int64{1005, 1001}},
			{"mirror_hit_false", with(window, "mirror_hit", "false"), []int64{1006, 1004, 1003, 1002}},
			{"intent_answer_coding", with(window, "intent_answer", "coding"), []int64{1005, 1001}},
			{"intent_answer_unknown", with(window, "intent_answer", "unknown"), []int64{1002}},
			{"intent_source_cache", with(window, "intent_source", "cache"), []int64{1005}},
			{"combined", with(window, "cache_status", "hit", "intent_answer", "coding"), []int64{1001}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var data logsData
				getSRJSON(t, client, reportPrefix+"/logs", tc.query, &data)

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
		getSRJSON(t, client, reportPrefix+"/logs", with(window, "page_size", "6"), &data)

		require.Len(t, data.Items, 6)
		byID := map[int64]logItem{}
		for _, one := range data.Items {
			byID[*one.LogID] = one
		}

		// log_time 为 Unix 秒（TIMESTAMPDIFF，UTC epoch）。
		newest := byID[1006]
		assert.Equal(t, epoch("2026-09-15 10:02:00"), newest.LogTime)
		require.NotNil(t, newest.Hostid)
		assert.Equal(t, "gw-01", *newest.Hostid)

		// 未认证行：ai_apikey_id 为 null（MySQL 语义，同 cases_test.go）。
		unauthenticated := byID[1004]
		assert.Nil(t, unauthenticated.APIKeyID)
		require.NotNil(t, unauthenticated.ErrCode)
		assert.Equal(t, "E401", *unauthenticated.ErrCode)

		// JSON 列原样字符串返回（键序不保证，键与值分别断言）。
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

		// 缓存/镜像/意图新 10 列：1001 classifier 源全量字段。
		full := byID[1001]
		require.NotNil(t, full.AICacheStatus)
		assert.Equal(t, "hit", *full.AICacheStatus)
		require.NotNil(t, full.MirrorHit)
		assert.True(t, *full.MirrorHit)
		require.NotNil(t, full.AIIntentAnswer)
		assert.Equal(t, "coding", *full.AIIntentAnswer)
		require.NotNil(t, full.AIIntentConfidence)
		assert.InDelta(t, 0.95, *full.AIIntentConfidence, 1e-12)
		require.NotNil(t, full.AIIntentLatencyUs)
		assert.Equal(t, int64(1200), *full.AIIntentLatencyUs)

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
	})
}

// ---------------------------------------------------------------------------
// 组 2：真实 StarRocks 端到端
// ---------------------------------------------------------------------------

// srWindowQuery 是组 2 的查询窗口 [2030-09-15 09:59, 10:05)（≤6h → 60s 桶），
// 与 windowQuery 同结构；种子时间平移到 2030-09-15 以规避物化视图
// partition_ttl 7 天（见 starrocks_seed_test.go 文件头）。
func srWindowQuery() map[string]string {
	return map[string]string{
		"start": fmt.Sprintf("%d", epoch("2030-09-15 09:59:00")),
		"end":   fmt.Sprintf("%d", epoch("2030-09-15 10:05:00")),
	}
}

// startStarRocksServer 装配一个 Backend="starrocks"、数据源为真实
// StarRocks 实例（临时库 report_sr_it_<ns>，用毕 DROP）的报表服务，
// 返回服务与专用客户端。REPORT_STARROCKS_DSN / REPORT_STARROCKS_DDL_DIR
// 未设置、DDL 文件缺失或 StarRocks 不可达时 Skip（仿现有 skip 风格）。
func startStarRocksServer(t *testing.T) (*testutil.ReportServer, *testutil.Client) {
	t.Helper()
	dsn := os.Getenv("REPORT_STARROCKS_DSN")
	if dsn == "" {
		t.Skip(`REPORT_STARROCKS_DSN not set; skipping starrocks end-to-end integration tests.`)
	}
	ddlDir := os.Getenv("REPORT_STARROCKS_DDL_DIR")
	if ddlDir == "" {
		t.Skip(`REPORT_STARROCKS_DDL_DIR not set (指向 ai-gateway-observability/starrocks/sqls); ` +
			`skipping starrocks end-to-end integration tests.`)
	}
	if os.Getenv("REPORT_MYSQL_DSN") == "" {
		// 包级 TestMain 以 REPORT_MYSQL_DSN 为门控，缺省时整个包在 TestMain
		// 阶段即退出；此处兜底仅为将来调整 TestMain 门控后给出明确提示。
		t.Skip("REPORT_MYSQL_DSN not set; 包级 TestMain 门控要求同时设置 REPORT_MYSQL_DSN。")
	}
	for _, file := range []string{"bfe_observability.sql", "bfe_ai_request_log.sql", "bfe_ai_metrics_1m.sql"} {
		if _, err := os.Stat(filepath.Join(ddlDir, file)); err != nil {
			t.Skipf("StarRocks DDL 文件缺失: %s (REPORT_STARROCKS_DDL_DIR=%s); skipping starrocks end-to-end integration tests.", file, ddlDir)
		}
	}
	if err := testutil.PingStarRocks(dsn); err != nil {
		t.Skipf("StarRocks 不可达 (%v); 请先启动实例并检查 REPORT_STARROCKS_DSN，skipping starrocks end-to-end integration tests.", err)
	}

	// 种子直插基表（物化视图不可直插）；6 行明细经异步 MV 聚出 6 行
	// （40 维分钟桶全键，每行明细自成一组），行数达标后才启动 api。
	rs, err := testutil.StartStarRocksReportServer(dsn, ddlDir, 6, srDetailSeedSQL)
	if err != nil {
		t.Fatalf("setup starrocks report server failed: %v", err)
	}
	// Close 负责停止 api 进程并 DROP 临时库（级联删除基表与物化视图）。
	t.Cleanup(rs.Close)
	return rs, starRocksClient(rs)
}

// srOverviewData 在 overviewData（聚合字段）上扩展 SR 后端的
// latency_p90_ms / latency_p99_ms（p50 字段 overviewData 已含）：
// SR 后端由明细表 percentile_approx 提供三个分位数（MySQL 后端不返回）。
type srOverviewData struct {
	overviewData
	LatencyP90Ms *float64 `json:"latency_p90_ms"`
	LatencyP99Ms *float64 `json:"latency_p99_ms"`
}

// TestStarRocksBackend_Overview 验证 SR 后端总览指标卡：聚合表（异步 MV）
// 合计、成本分组、明细 COUNT 与缓存/镜像/意图组全部精确等于 srDetailSeedSQL
// 的 MV 派生手算值（见 starrocks_seed_test.go 头表）；latency p50/p99 对
// 微数据集精确断言（实机验证：窗口 6 值 p50=80、p99=300），p90 为素描近似
// 只断存在。
func TestStarRocksBackend_Overview(t *testing.T) {
	_, client := startStarRocksServer(t)

	t.Run("seeded_window", func(t *testing.T) {
		var data srOverviewData
		getSRJSON(t, client, reportPrefix+"/overview", srWindowQuery(), &data)

		assert.Equal(t, int64(6), data.RequestTotal)
		assert.Equal(t, int64(2), data.ErrorTotal)
		assert.InDelta(t, 2.0/6.0, data.ErrorRate, 1e-9)
		assert.Equal(t, int64(720), data.InputTokens)
		assert.Equal(t, int64(144), data.OutputTokens)
		assert.Equal(t, int64(864), data.TotalTokens)

		// latency_avg = all_time_sum/request_total = 720/6 = 120ms；
		// latency_max = MAX(all_time_sum/request_count)（1003 行组 300ms）。
		assert.InDelta(t, 120, data.LatencyAvgMs, 1e-9)
		assert.InDelta(t, 300, data.LatencyMaxMs, 1e-9)
		// percentile_approx 微数据集精确值（实机验证）；p90 素描近似只断存在。
		require.NotNil(t, data.LatencyP50Ms)
		assert.InDelta(t, 80, *data.LatencyP50Ms, 1e-9)
		require.NotNil(t, data.LatencyP90Ms)
		require.NotNil(t, data.LatencyP99Ms)
		assert.InDelta(t, 300, *data.LatencyP99Ms, 1e-6)

		// ttft/tpot 聚合于 stream 请求（1001/1003 两个流式行），微秒→毫秒。
		assert.InDelta(t, 200000.0/2/1000, data.TtftAvgMs, 1e-9) // 100ms
		assert.InDelta(t, 20000.0/2/1000, data.TpotAvgMs, 1e-9)  // 10ms

		require.Len(t, data.Cost, 2)
		costByCurrency := map[string]float64{}
		for _, one := range data.Cost {
			costByCurrency[one.Currency] = one.Value
		}
		assert.InDelta(t, 3.6e-6, costByCurrency["USD"], 1e-12) // 定点 360
		assert.InDelta(t, 3e-6, costByCurrency["RMB"], 1e-12)   // 定点 300

		assert.Equal(t, int64(1), data.RateLimitHits) // 1002 行 JSON 非空计 1
		assert.Equal(t, int64(1), data.AuthRejects)   // 1004 行拒绝原因非空
		assert.Equal(t, int64(6), data.LogsTotal)     // 明细表 count()

		assert.Equal(t, int64(2), data.Cache.HitCount)
		assert.Equal(t, int64(2), data.Cache.MissCount)
		assert.Equal(t, int64(1), data.Cache.SkipCount)
		assert.InDelta(t, 0.5, data.Cache.HitRate, 1e-9)
		assert.Equal(t, int64(4000), data.Cache.ReadTokens)
		assert.Equal(t, int64(600), data.Cache.WriteTokens)
		assert.Equal(t, int64(2), data.Mirror.HitCount)
		assert.Equal(t, int64(3), data.Intent.ClassifiedCount)
		assert.Equal(t, int64(1), data.Intent.UnknownCount)
		assert.InDelta(t, 0.25, data.Intent.UnknownRate, 1e-9)
	})

	t.Run("filtered", func(t *testing.T) {
		// models + status_codes 过滤项参与聚合口径（gpt-4o + 200：
		// 明细 1001、1006 两行 → MV 两组）。
		query := with(srWindowQuery(), "models", "gpt-4o", "status_codes", "200")
		var data srOverviewData
		getSRJSON(t, client, reportPrefix+"/overview", query, &data)

		assert.Equal(t, int64(2), data.RequestTotal)
		assert.Equal(t, int64(0), data.ErrorTotal)
		assert.Equal(t, int64(160), data.InputTokens) // 100+60
		assert.Equal(t, int64(2), data.LogsTotal)
	})
}

// TestStarRocksBackend_TimeSeries 验证 SR 后端时序：qps/tokens/cost/
// cache_tokens 与 dimension 拆分的桶值精确等于 MV 派生手算值（3 个 60s
// 桶：10:00 四行、10:01/10:02 各一行）；latency 时序见
// TestStarRocksBackend_LatencyPercentiles。
func TestStarRocksBackend_TimeSeries(t *testing.T) {
	_, client := startStarRocksServer(t)

	t.Run("qps", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(srWindowQuery(), "metric", "qps"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 3)
		assert.Equal(t, epoch("2030-09-15 10:00:00"), data.Series[0].Time)
		require.NotNil(t, data.Series[0].Value)
		assert.InDelta(t, 4.0/60, *data.Series[0].Value, 1e-9)
		assert.Equal(t, epoch("2030-09-15 10:01:00"), data.Series[1].Time)
		require.NotNil(t, data.Series[1].Value)
		assert.InDelta(t, 1.0/60, *data.Series[1].Value, 1e-9)
		assert.Equal(t, epoch("2030-09-15 10:02:00"), data.Series[2].Time)
		require.NotNil(t, data.Series[2].Value)
		assert.InDelta(t, 1.0/60, *data.Series[2].Value, 1e-9)
	})

	t.Run("tokens", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(srWindowQuery(), "metric", "tokens"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 3)
		assert.InDelta(t, 650.0/60, *data.Series[0].Input, 1e-9) // 1001-1004
		assert.InDelta(t, 130.0/60, *data.Series[0].Output, 1e-9)
		assert.InDelta(t, 780.0/60, *data.Series[0].Total, 1e-9)
		assert.InDelta(t, 10.0/60, *data.Series[1].Input, 1e-9) // 1005
		assert.InDelta(t, 2.0/60, *data.Series[1].Output, 1e-9)
		assert.InDelta(t, 12.0/60, *data.Series[1].Total, 1e-9)
		assert.InDelta(t, 60.0/60, *data.Series[2].Input, 1e-9) // 1006
		assert.InDelta(t, 12.0/60, *data.Series[2].Output, 1e-9)
		assert.InDelta(t, 72.0/60, *data.Series[2].Total, 1e-9)
	})

	t.Run("cost", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(srWindowQuery(), "metric", "cost"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 5) // 含 1004(NULL 币种)与 1005('' 币种)的空币种桶

		type pointKey struct {
			time     int64
			currency string
		}
		byKey := map[pointKey]float64{}
		for _, one := range data.Series {
			require.NotNil(t, one.Value)
			byKey[pointKey{one.Time, one.Currency}] = *one.Value
		}
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:00:00"), ""}])                      // 1004
		assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2030-09-15 10:00:00"), "USD"}], 1e-15) // 1001+1002
		assert.InDelta(t, 300.0/60/1e8, byKey[pointKey{epoch("2030-09-15 10:00:00"), "RMB"}], 1e-15) // 1003
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:01:00"), ""}])                      // 1005
		assert.InDelta(t, 60.0/60/1e8, byKey[pointKey{epoch("2030-09-15 10:02:00"), "USD"}], 1e-15)  // 1006
	})

	t.Run("cache_tokens", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(srWindowQuery(), "metric", "cache_tokens"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 6) // 3 桶 × read/write，SUM 零点仍在

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
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:01:00"), "cache_read"}])
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:01:00"), "cache_write"}])
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:02:00"), "cache_read"}])
		assert.Equal(t, 0.0, byKey[pointKey{epoch("2030-09-15 10:02:00"), "cache_write"}])
	})

	t.Run("qps_by_cache_status", func(t *testing.T) {
		var data timeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries",
			with(srWindowQuery(), "metric", "qps", "dimension", "ai_cache_status"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 5) // 含 1004 的空值桶（维度拆分空值点 name 为 ""）

		type pointKey struct {
			time int64
			name string
		}
		byKey := map[pointKey]float64{}
		for _, one := range data.Series {
			require.NotNil(t, one.Value)
			byKey[pointKey{one.Time, one.Name}] = *one.Value
		}
		assert.InDelta(t, 2.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), "hit"}], 1e-9)  // 1001+1003
		assert.InDelta(t, 1.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), "miss"}], 1e-9) // 1002
		assert.InDelta(t, 1.0/60, byKey[pointKey{epoch("2030-09-15 10:00:00"), ""}], 1e-9)     // 1004
		assert.InDelta(t, 1.0/60, byKey[pointKey{epoch("2030-09-15 10:01:00"), "skip"}], 1e-9) // 1005
		assert.InDelta(t, 1.0/60, byKey[pointKey{epoch("2030-09-15 10:02:00"), "miss"}], 1e-9) // 1006
	})
}

// TestStarRocksBackend_Rankings 验证 SR 后端排行：MV GROUP BY 维度 +
// SUM(指标) 的派生口径（数值维度 CAST AS CHAR 转名、空值不进排行、
// mirror_hit 0/1 双桶都进排行），并列名次按 map 断言（排序键相同的两组
// 顺序由引擎自定）。
func TestStarRocksBackend_Rankings(t *testing.T) {
	_, client := startStarRocksServer(t)

	t.Run("model", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(srWindowQuery(), "dimension", "model"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]rankingItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(4), byName["gpt-4o"].RequestCount) // 1001+1002+1004+1006
		assert.Equal(t, int64(2), byName["gpt-4o"].ErrorCount)   // 1002+1004
		assert.Equal(t, int64(410), byName["gpt-4o"].InputTokens)
		assert.Equal(t, int64(82), byName["gpt-4o"].OutputTokens)
		assert.Equal(t, int64(1), byName["gpt-4"].RequestCount) // 1003
		assert.Equal(t, int64(300), byName["gpt-4"].InputTokens)
		assert.Equal(t, int64(1), byName["claude"].RequestCount) // 1005
		assert.Equal(t, int64(10), byName["claude"].InputTokens)
	})

	t.Run("status", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(srWindowQuery(), "dimension", "status"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]int64{}
		for _, one := range data.Items {
			byName[one.Name] = one.RequestCount
		}
		assert.Equal(t, int64(4), byName["200"])
		assert.Equal(t, int64(1), byName["500"])
		assert.Equal(t, int64(1), byName["401"])
	})

	t.Run("cache_status", func(t *testing.T) {
		// 空值（''）不进排行；明细种子含 skip 行，故为三桶。
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(srWindowQuery(), "dimension", "ai_cache_status"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]int64{}
		for _, one := range data.Items {
			byName[one.Name] = one.RequestCount
		}
		assert.Equal(t, int64(2), byName["hit"])  // 1001+1003
		assert.Equal(t, int64(2), byName["miss"]) // 1002+1004
		assert.Equal(t, int64(1), byName["skip"]) // 1005
		assert.NotContains(t, byName, "")         // 1004 的空值不进排行
	})

	t.Run("mirror_hit", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(srWindowQuery(), "dimension", "mirror_hit"), &data)

		require.Len(t, data.Items, 2)
		assert.Equal(t, "0", data.Items[0].Name)
		assert.Equal(t, int64(4), data.Items[0].RequestCount)
		assert.Equal(t, "1", data.Items[1].Name)
		assert.Equal(t, int64(2), data.Items[1].RequestCount)
	})

	t.Run("intent_answer", func(t *testing.T) {
		var data rankingsData
		getSRJSON(t, client, reportPrefix+"/rankings", with(srWindowQuery(), "dimension", "ai_intent_answer"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]int64{}
		for _, one := range data.Items {
			byName[one.Name] = one.RequestCount
		}
		assert.Equal(t, int64(2), byName["coding"])  // 1001+1005
		assert.Equal(t, int64(1), byName["unknown"]) // 1002
		assert.Equal(t, int64(1), byName["writing"]) // 1003
		assert.NotContains(t, byName, "")            // 1004/1006 的空值不进排行
	})
}

// TestStarRocksBackend_Distribution 验证 SR 后端分布：CASE 归一空值桶 +
// ratio 口径的 MV 派生值。protocol 维度明细种子无空值行，故仅 openai
// 单桶全占比（unknown 归一桶口径由 cache_status 维度的 1004 空值覆盖）。
func TestStarRocksBackend_Distribution(t *testing.T) {
	_, client := startStarRocksServer(t)

	t.Run("status", func(t *testing.T) {
		var data distributionData
		getSRJSON(t, client, reportPrefix+"/distribution", with(srWindowQuery(), "dimension", "status"), &data)

		require.Len(t, data.Items, 3)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(4), byName["200"].RequestCount)
		assert.InDelta(t, 4.0/6.0, byName["200"].Ratio, 1e-9)
		assert.Equal(t, int64(1), byName["401"].RequestCount)
		assert.Equal(t, int64(1), byName["500"].RequestCount)
		assert.InDelta(t, 1, byName["200"].Ratio+byName["401"].Ratio+byName["500"].Ratio, 1e-9)
	})

	t.Run("protocol", func(t *testing.T) {
		var data distributionData
		getSRJSON(t, client, reportPrefix+"/distribution", with(srWindowQuery(), "dimension", "protocol"), &data)

		require.Len(t, data.Items, 1)
		assert.Equal(t, "openai", data.Items[0].Name) // 1005 的协议也是 openai
		assert.Equal(t, int64(6), data.Items[0].RequestCount)
		assert.InDelta(t, 1, data.Items[0].Ratio, 1e-9)
	})

	t.Run("cache_status", func(t *testing.T) {
		// 空值归一为 unknown 桶（1004）。
		var data distributionData
		getSRJSON(t, client, reportPrefix+"/distribution", with(srWindowQuery(), "dimension", "ai_cache_status"), &data)

		require.Len(t, data.Items, 4)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(2), byName["hit"].RequestCount)
		assert.Equal(t, int64(2), byName["miss"].RequestCount)
		assert.Equal(t, int64(1), byName["skip"].RequestCount)
		assert.Equal(t, int64(1), byName["unknown"].RequestCount)
		assert.InDelta(t, 1,
			byName["hit"].Ratio+byName["miss"].Ratio+byName["skip"].Ratio+byName["unknown"].Ratio, 1e-9)
	})

	t.Run("mirror_hit", func(t *testing.T) {
		var data distributionData
		getSRJSON(t, client, reportPrefix+"/distribution", with(srWindowQuery(), "dimension", "mirror_hit"), &data)

		require.Len(t, data.Items, 2)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(4), byName["0"].RequestCount)
		assert.Equal(t, int64(2), byName["1"].RequestCount)
		assert.InDelta(t, 2.0/6.0, byName["1"].Ratio, 1e-9)
	})
}

// TestStarRocksBackend_Logs 验证 SR 后端明细：count / 分页 / 过滤与行形状。
// 复杂列内容断言（实机验证的线格式，见 starrocks_seed_test.go 偏差 3）：
// req_headers / ai_rate_limit_hits 为 VARCHAR(JSON 文本)原样回显，
// ai_auth_reject_quota_plans 为 ARRAY<VARCHAR>经 MySQL 协议返回 JSON 文本
// ["plan-a"]；NULL 语义同 MySQL 组（1004 的 ai_apikey_id/ai_cost_currency
// 响应 null，区别于 CH 组的 ""；1001 的空 req_headers 响应 null，区别于
// CH 组的 "[]"）。
func TestStarRocksBackend_Logs(t *testing.T) {
	_, client := startStarRocksServer(t)

	t.Run("paging", func(t *testing.T) {
		var page1 logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(srWindowQuery(), "page", "1", "page_size", "2"), &page1)

		assert.Equal(t, int64(6), page1.Total)
		assert.Equal(t, 1, page1.Page)
		assert.Equal(t, 2, page1.PageSize)
		require.Len(t, page1.Items, 2)
		// log_time 倒序：10:02:00(1006) > 10:01:10(1005)。
		assert.Equal(t, int64(1006), *page1.Items[0].LogID)
		assert.Equal(t, int64(1005), *page1.Items[1].LogID)

		var page2 logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(srWindowQuery(), "page", "2", "page_size", "2"), &page2)
		require.Len(t, page2.Items, 2)
		assert.Equal(t, int64(1004), *page2.Items[0].LogID)
		assert.Equal(t, int64(1003), *page2.Items[1].LogID)
	})

	t.Run("filters_err_keyword", func(t *testing.T) {
		var errOnly logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(srWindowQuery(), "err_only", "true"), &errOnly)
		assert.Equal(t, int64(2), errOnly.Total)
		require.Len(t, errOnly.Items, 2)
		assert.Equal(t, int64(1004), *errOnly.Items[0].LogID)
		assert.Equal(t, int64(1002), *errOnly.Items[1].LogID)

		var keyword logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(srWindowQuery(), "keyword", "timeout"), &keyword)
		assert.Equal(t, int64(1), keyword.Total)
		require.Len(t, keyword.Items, 1)
		assert.Equal(t, int64(1002), *keyword.Items[0].LogID)
		require.NotNil(t, keyword.Items[0].ErrMsg)
		assert.Contains(t, *keyword.Items[0].ErrMsg, "timeout")
	})

	t.Run("requested_models", func(t *testing.T) {
		var data logsData
		getSRJSON(t, client, reportPrefix+"/logs", with(srWindowQuery(), "requested_models", "claude-3"), &data)
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
			{"cache_status_hit", with(srWindowQuery(), "cache_status", "hit"), []int64{1003, 1001}},
			{"cache_status_skip", with(srWindowQuery(), "cache_status", "skip"), []int64{1005}},
			{"mirror_hit_true", with(srWindowQuery(), "mirror_hit", "true"), []int64{1005, 1001}},
			{"mirror_hit_false", with(srWindowQuery(), "mirror_hit", "false"), []int64{1006, 1004, 1003, 1002}},
			{"intent_answer_coding", with(srWindowQuery(), "intent_answer", "coding"), []int64{1005, 1001}},
			{"intent_answer_unknown", with(srWindowQuery(), "intent_answer", "unknown"), []int64{1002}},
			{"intent_source_cache", with(srWindowQuery(), "intent_source", "cache"), []int64{1005}},
			{"combined", with(srWindowQuery(), "cache_status", "hit", "intent_answer", "coding"), []int64{1001}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var data logsData
				getSRJSON(t, client, reportPrefix+"/logs", tc.query, &data)

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
		getSRJSON(t, client, reportPrefix+"/logs", with(srWindowQuery(), "page_size", "6"), &data)

		require.Len(t, data.Items, 6)
		byID := map[int64]logItem{}
		for _, one := range data.Items {
			byID[*one.LogID] = one
		}

		// log_time 为 Unix 秒（TIMESTAMPDIFF，UTC epoch）。
		newest := byID[1006]
		assert.Equal(t, epoch("2030-09-15 10:02:00"), newest.LogTime)
		require.NotNil(t, newest.Hostid)
		assert.Equal(t, "gw-01", *newest.Hostid)
		require.NotNil(t, newest.Product)
		assert.Equal(t, "BFE", *newest.Product)

		// 1004（未认证行）：NULL 语义同 MySQL 组（区别于 CH 的 ""）。
		unauthenticated := byID[1004]
		assert.Nil(t, unauthenticated.APIKeyID)
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

		// 零成本行：1004 的 currency 为 NULL -> nil；1005 的空串语义同 MySQL 组。
		noCurrency := byID[1004]
		require.NotNil(t, noCurrency.CostValue)
		assert.Equal(t, 0.0, *noCurrency.CostValue)
		assert.Nil(t, noCurrency.CostCurrency)
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
		require.NotNil(t, full.AIIntentQuestionsVer)
		assert.Equal(t, "v3", *full.AIIntentQuestionsVer)

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

		// 复杂列 JSON 文本：1002 的 req_headers 为 VARCHAR 原样回显的
		// 归一形态（插入的文本即响应文本，JSONEq 断言键值）；
		// ai_rate_limit_hits 同为 VARCHAR JSON 文本；1004 的配额计划为
		// ARRAY<VARCHAR> 的 JSON 文本表示 ["plan-a"]；1001 的空 req_headers
		// 为 NULL -> nil（区别于 CH 组的 "[]"）。
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
		assert.Nil(t, emptyHeaders.ReqHeaders)
	})
}

// srMetricPoint / srTimeseriesData 与 metricPoint / timeseriesData 同构，扩展
// SR 后端 latency 时序的 avg/max/p50/p90/p99 字段（MySQL 组无分位数，
// query_test.go 的共享结构未含这些字段）。
type srMetricPoint struct {
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

type srTimeseriesData struct {
	BucketSec int             `json:"bucket_sec"`
	Series    []srMetricPoint `json:"series"`
}

// TestStarRocksBackend_LatencyPercentiles 验证 SR 后端延迟分位数：
//   - timeseries?metric=latency 的桶点由聚合表（MV）提供 avg/max（精确值
//     断言：10:00 桶 650/4=162.5ms、max 300ms），并按桶从明细表合并
//     p50/p90/p99（percentile_approx）——微数据集下 p50 精确（实机验证：
//     10:00 桶 4 值 p50=150，单值桶 p50 即该值），p90/p99 只断言字段存在；
//   - 空窗口（超出种子范围）overview 不返回分位数字段（实机验证 SR
//     percentile_approx 对空输入返回 NULL，扫描层省略字段，与 Doris 空
//     窗口行为对齐）。
func TestStarRocksBackend_LatencyPercentiles(t *testing.T) {
	_, client := startStarRocksServer(t)

	t.Run("timeseries_latency_percentiles", func(t *testing.T) {
		var data srTimeseriesData
		getSRJSON(t, client, reportPrefix+"/timeseries", with(srWindowQuery(), "metric", "latency"), &data)

		assert.Equal(t, 60, data.BucketSec)
		require.Len(t, data.Series, 3)

		// 聚合 avg/max：10:00 桶 all_time_sum=650/request_count=4；
		// 10:01 桶 10/1；10:02 桶 60/1；各桶 latency_max 即行 all_time。
		assert.Equal(t, epoch("2030-09-15 10:00:00"), data.Series[0].Time)
		require.NotNil(t, data.Series[0].Avg)
		assert.InDelta(t, 162.5, *data.Series[0].Avg, 1e-9)
		require.NotNil(t, data.Series[0].Max)
		assert.InDelta(t, 300, *data.Series[0].Max, 1e-9)
		assert.Equal(t, epoch("2030-09-15 10:01:00"), data.Series[1].Time)
		require.NotNil(t, data.Series[1].Avg)
		assert.InDelta(t, 10, *data.Series[1].Avg, 1e-9)
		assert.Equal(t, epoch("2030-09-15 10:02:00"), data.Series[2].Time)
		require.NotNil(t, data.Series[2].Avg)
		assert.InDelta(t, 60, *data.Series[2].Avg, 1e-9)

		// 分位数：p50 微数据集精确（实机验证）；p90/p99 只断言字段存在
		// （10:00 桶实机 p90=p99=300，但素描算法随版本可能漂移）。
		assert.InDelta(t, 150, *data.Series[0].P50, 1e-9) // [100,200,300,50]
		assert.InDelta(t, 10, *data.Series[1].P50, 1e-9)
		assert.InDelta(t, 60, *data.Series[2].P50, 1e-9)
		for i, point := range data.Series {
			require.NotNil(t, point.P90, "series %d", i)
			require.NotNil(t, point.P99, "series %d", i)
		}
	})

	t.Run("overview_empty_window_percentile_omitted", func(t *testing.T) {
		// 窗口完全超出种子范围：SR percentile_approx 返回 NULL，扫描层省略字段。
		query := map[string]string{
			"start": fmt.Sprint(epoch("2030-09-15 12:00:00")),
			"end":   fmt.Sprint(epoch("2030-09-15 13:00:00")),
		}
		var data srOverviewData
		getSRJSON(t, client, reportPrefix+"/overview", query, &data)

		assert.Equal(t, int64(0), data.RequestTotal)
		assert.Nil(t, data.LatencyP50Ms)
		assert.Nil(t, data.LatencyP90Ms)
		assert.Nil(t, data.LatencyP99Ms)
	})
}
