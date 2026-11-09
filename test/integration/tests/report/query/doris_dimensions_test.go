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

// 本文件覆盖 Backend=doris 装配下的三个缓存/镜像/意图新维度
// （ai_intent_answer / ai_cache_status / mirror_hit）端到端可用性
// （二期，design-docs modifications/2026-10-01-report-doris-cache-mirror-intent-alignment）：
// rankings / distribution / timeseries 正常返回数据且口径与 MySQL 后端一致；
// 一期的能力门控（422）已随 dorisreport Capabilities 拉平移除。
//
// 装配说明：数据源仍是 REPORT_MYSQL_DSN 指向的 MySQL 实例（种子表含三个新
// 维度列；查询层的 Doris 方言差异函数如 PERCENTILE_APPROX 不在本文件覆盖
// 范围内，由两侧 storager 单测锁定），仅 [Report].Backend 装配为 "doris"
// （与 testutil.StartReportServerWithBackend 的约定一致）。真实 Doris FE 的
// 端到端验证见集成测试 Doris 组（排期见修改说明 §7 测试计划）。

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
)

// startDorisBackendServer 以 Backend="doris" 装配一个独立 api 进程（独立
// 随机库与端口），REPORT_MYSQL_DSN 未设置时 Skip。
func startDorisBackendServer(t *testing.T) *testutil.ReportServer {
	t.Helper()
	if os.Getenv("REPORT_MYSQL_DSN") == "" {
		t.Skip("REPORT_MYSQL_DSN not set; skipping doris backend integration tests.")
	}
	rs, err := testutil.StartReportServerWithBackend("doris", aggregateSeedSQL, detailSeedSQL)
	if err != nil {
		if errors.Is(err, testutil.ErrReportMySQLDSNNotSet) {
			t.Skip("REPORT_MYSQL_DSN not set; skipping doris backend integration tests.")
		}
		t.Fatalf("setup doris-backend report server failed: %v", err)
	}
	t.Cleanup(rs.Close)
	return rs
}

// TestDorisBackend_NewDimensionsSupported 验证二期 Doris 后端三个新维度
// 端到端可用，口径与 MySQL 后端一致（同种子数据，期望值与
// cases_test.go 的 MySQL 后端断言相同）。
func TestDorisBackend_NewDimensionsSupported(t *testing.T) {
	startDorisBackendServer(t)
	window := windowQuery()

	t.Run("rankings_cache_status", func(t *testing.T) {
		// 空值（''）不进排行。
		var data rankingsData
		getJSON(t, reportPrefix+"/rankings", with(window, "dimension", "ai_cache_status"), &data)
		require.Len(t, data.Items, 2)
		assert.Equal(t, "hit", data.Items[0].Name)
		assert.Equal(t, int64(450), data.Items[0].RequestCount)
		assert.Equal(t, "miss", data.Items[1].Name)
		assert.Equal(t, int64(200), data.Items[1].RequestCount)
	})

	t.Run("rankings_mirror_hit", func(t *testing.T) {
		// 数值维度 0/1 两桶都进排行。
		var data rankingsData
		getJSON(t, reportPrefix+"/rankings", with(window, "dimension", "mirror_hit"), &data)
		require.Len(t, data.Items, 2)
		assert.Equal(t, "0", data.Items[0].Name)
		assert.Equal(t, int64(530), data.Items[0].RequestCount)
		assert.Equal(t, "1", data.Items[1].Name)
		assert.Equal(t, int64(150), data.Items[1].RequestCount)
	})

	t.Run("rankings_intent_answer", func(t *testing.T) {
		var data rankingsData
		getJSON(t, reportPrefix+"/rankings", with(window, "dimension", "ai_intent_answer"), &data)
		require.Len(t, data.Items, 3)
		byName := map[string]int64{}
		for _, one := range data.Items {
			byName[one.Name] = one.RequestCount
		}
		assert.Equal(t, int64(200), byName["unknown"])
		assert.Equal(t, int64(300), byName["writing"])
		assert.Equal(t, int64(150), byName["coding"])
		assert.NotContains(t, byName, "")
	})

	t.Run("distribution_cache_status", func(t *testing.T) {
		// 空值归一为 unknown 桶。
		var data distributionData
		getJSON(t, reportPrefix+"/distribution", with(window, "dimension", "ai_cache_status"), &data)
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

	t.Run("distribution_mirror_hit", func(t *testing.T) {
		var data distributionData
		getJSON(t, reportPrefix+"/distribution", with(window, "dimension", "mirror_hit"), &data)
		require.Len(t, data.Items, 2)
		byName := map[string]distItem{}
		for _, one := range data.Items {
			byName[one.Name] = one
		}
		assert.Equal(t, int64(530), byName["0"].RequestCount)
		assert.Equal(t, int64(150), byName["1"].RequestCount)
	})

	t.Run("timeseries_qps_by_cache_status", func(t *testing.T) {
		var data timeseriesData
		getJSON(t, reportPrefix+"/timeseries", with(window, "metric", "qps", "dimension", "ai_cache_status"), &data)
		assert.Equal(t, 60, data.BucketSec)
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

	t.Run("legacy_dimension_still_supported", func(t *testing.T) {
		// 既有维度在 doris 后端行为不变。
		var data rankingsData
		getJSON(t, reportPrefix+"/rankings", with(window, "dimension", "model"), &data)
		require.NotEmpty(t, data.Items)
	})
}

// TestDorisBackend_DetailCapabilitiesUngated 验证 Doris 装配下明细能力不受
// 维度能力影响：logs 新过滤（两后端方言一致的明细列过滤）正常返回。
// overview 的 PERCENTILE_APPROX 与 timeseries 的 Doris 桶表达式属 Doris
// 方言，本 harness 数据源为 MySQL 实例无法执行（需真实 Doris FE），不在此
// 覆盖（由两侧 storager 单测锁定）。
func TestDorisBackend_DetailCapabilitiesUngated(t *testing.T) {
	rs := startDorisBackendServer(t)
	client := &testutil.Client{
		BaseURL:    rs.Server.ServerURL,
		HTTPClient: testutil.GetClient().HTTPClient,
		Token:      testutil.GetClient().Token,
	}

	window := windowQuery()

	t.Run("logs_filter", func(t *testing.T) {
		resp, err := client.Get(reportPrefix+"/logs", with(window, "cache_status", "hit"))
		require.NoError(t, err)
		require.NotNil(t, resp)
		testutil.AssertSuccess(t, resp)
	})

	t.Run("logs_new_columns_projected", func(t *testing.T) {
		resp, err := client.Get(reportPrefix+"/logs", with(window, "intent_answer", "coding"))
		require.NoError(t, err)
		require.NotNil(t, resp)
		testutil.AssertSuccess(t, resp)
	})
}
