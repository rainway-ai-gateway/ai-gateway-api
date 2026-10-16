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

package innerapi_test

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sm *testutil.ServerManager

func TestMain(m *testing.M) {
	var err error
	sm, err = testutil.StartServer()
	if err != nil {
		panic("failed to start server: " + err.Error())
	}
	code := m.Run()
	sm.Shutdown()
	os.Exit(code)
}

// getServerDataConfCluster 导出 server_data_conf 并返回指定 cluster 的导出配置。
func getServerDataConfCluster(t *testing.T, clusterName string) map[string]interface{} {
	t.Helper()
	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
	if err != nil {
		t.Fatalf("export server_data_conf failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal server_data_conf failed: %v", err)
	}
	clusterConf := data["ClusterConf"].(map[string]interface{})
	config := clusterConf["Config"].(map[string]interface{})
	cluster, ok := config[clusterName].(map[string]interface{})
	require.True(t, ok, "cluster %s should exist in server_data_conf export", clusterName)
	return cluster
}

// getEppData 拉取 epp_data 导出，version 为空表示首次拉取。
func getEppData(t *testing.T, version string) *testutil.APIResponse {
	t.Helper()
	var resp *testutil.APIResponse
	var err error
	if version == "" {
		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/epp_data/config")
	} else {
		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/epp_data/config", map[string]string{
			"version": version,
		})
	}
	if err != nil {
		t.Fatalf("export epp_data failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	return resp
}

func findAssignmentEntry(t *testing.T, data map[string]interface{}, cluster string) map[string]interface{} {
	t.Helper()
	clusters := data["clusters"].([]interface{})
	for _, item := range clusters {
		entry := item.(map[string]interface{})
		if entry["cluster"] == cluster {
			return entry
		}
	}
	return nil
}

func waitAssignmentPrimary(t *testing.T, cluster string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := testutil.GetClient().Get("/open-api/v1/epp-assignments", map[string]string{"cluster": cluster})
		if err != nil {
			t.Fatalf("get assignments failed: %v", err)
		}
		if resp.ErrNum == 200 {
			var data map[string]interface{}
			if err := json.Unmarshal(resp.Data, &data); err == nil {
				entry := findAssignmentEntry(t, data, cluster)
				if entry != nil {
					if primary, ok := entry["primary"].(map[string]interface{}); ok {
						return primary
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("cluster %s not assigned within %v", cluster, timeout)
	return nil
}

func TestInnerAPI_EppData(t *testing.T) {
	pt := t
	// degradeCluster 在空池上创建，进入未分配态；degradePrimary 在恢复后赋值。
	var degradeCluster string
	var degradePrimary map[string]interface{}

	t.Run("IN-EPP-001 空池创建 EPP 集群进入未分配态并降级导出", func(t *testing.T) {
		degradeCluster = testutil.UniqueClusterName()
		providerName := testutil.UniqueProviderName()
		if _, err := testutil.CreateProvider(providerName); err != nil {
			t.Fatalf("setup provider failed: %v", err)
		}
		// 清理注册到父测试，避免子测试结束即删除（cleanup 按 LIFO，
		// 先注册 provider 后注册 cluster，保证先删 cluster 再删 provider）。
		pt.Cleanup(func() { testutil.DeleteProvider(providerName) })

		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name":         degradeCluster,
			"balance_mode": "EPP",
			"epp_config":   map[string]interface{}{"load_profile": "balanced"},
			"llm_config": map[string]interface{}{
				"models":   []string{"deepseek-chat"},
				"provider": providerName,
			},
		})
		if err != nil {
			t.Fatalf("create epp cluster failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		pt.Cleanup(func() { testutil.DeleteCluster(degradeCluster) })

		// 分配视图：未分配，primary 为 null
		view, err := testutil.GetClient().Get("/open-api/v1/epp-assignments", map[string]string{"cluster": degradeCluster})
		if err != nil {
			t.Fatalf("get assignments failed: %v", err)
		}
		testutil.AssertSuccess(t, view)
		var viewData map[string]interface{}
		json.Unmarshal(view.Data, &viewData)
		entry := findAssignmentEntry(t, viewData, degradeCluster)
		require.NotNil(t, entry)
		assert.Nil(t, entry["primary"])
		assert.Contains(t, viewData["unassigned_clusters"], degradeCluster)

		// server_data_conf 降级导出：BalanceMode=WRR，无 EPPAddr
		cluster := getServerDataConfCluster(t, degradeCluster)
		gslbBasic, ok := cluster["GslbBasic"].(map[string]interface{})
		require.True(t, ok, "GslbBasic should exist")
		assert.Equal(t, "WRR", gslbBasic["BalanceMode"], "unassigned EPP cluster degrades to WRR")
		assert.Nil(t, gslbBasic["EPPAddr"], "degraded cluster should not export EPPAddr")
	})

	t.Run("IN-EPP-002 恢复容量后 reconciler 自动分配并回到 EPP", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/epp-pool", map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": "g1",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("patch epp pool failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// reconciler 周期 1s（测试配置），轮询等待自动分配
		degradePrimary = waitAssignmentPrimary(t, degradeCluster, 15*time.Second)
		assert.Equal(t, "epp-a", degradePrimary["id"])

		cluster := getServerDataConfCluster(t, degradeCluster)
		gslbBasic := cluster["GslbBasic"].(map[string]interface{})
		assert.Equal(t, "EPP", gslbBasic["BalanceMode"])
		eppAddr, ok := gslbBasic["EPPAddr"].([]interface{})
		require.True(t, ok, "EPPAddr should be exported after assignment")
		require.Len(t, eppAddr, 1, "single-instance group exports [primary]")
		expectedPrimary := fmt.Sprintf("%s:%v", degradePrimary["host"], int(degradePrimary["port"].(float64)))
		assert.Equal(t, expectedPrimary, eppAddr[0], "EPPAddr[0] should be primary")
	})

	var fullCluster string
	var fullPrimary map[string]interface{}
	var firstVersion string

	t.Run("IN-EPP-003 epp_data 导出含编译后 epp_config 与 assignment 两段", func(t *testing.T) {
		fullCluster = testutil.UniqueClusterName()
		providerName := testutil.UniqueProviderName()
		if _, err := testutil.CreateProvider(providerName); err != nil {
			t.Fatalf("setup provider failed: %v", err)
		}
		// 注册到父测试且先于 cluster 注册，保证清理时先删 cluster 再删 provider。
		pt.Cleanup(func() { testutil.DeleteProvider(providerName) })

		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name":         fullCluster,
			"balance_mode": "EPP",
			"epp_config": map[string]interface{}{
				"load_profile":                   "queue-first",
				"affinity":                       "high",
				"kv_cache_utilization_max":       0.85,
				"waiting_queue_max":              5,
				"running_requests_max":           16,
				"fallback_on_empty":              true,
				"metrics_staleness_threshold_ms": 300,
				"session_affinity_enabled":       true,
				"session_affinity_header":        "x-session-id",
				"flow_control": map[string]interface{}{
					"max_requests":          200,
					"queue_ttl":             45,
					"no_endpoint_queue_ttl": 120,
					"enable_eviction":       false,
				},
			},
			"llm_config": map[string]interface{}{
				"models":   []string{"deepseek-chat"},
				"provider": providerName,
			},
		})
		if err != nil {
			t.Fatalf("create epp cluster failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		pt.Cleanup(func() { testutil.DeleteCluster(fullCluster) })

		// 自动分配：与 degradeCluster 共享 g1（单实例组，同一实例可服务
		// 多个 cluster）。
		fullPrimary = waitAssignmentPrimary(t, fullCluster, 15*time.Second)

		viewResp, err := testutil.GetClient().Get("/open-api/v1/epp-assignments", map[string]string{"cluster": fullCluster})
		if err != nil {
			t.Fatalf("get assignments failed: %v", err)
		}
		var viewData map[string]interface{}
		json.Unmarshal(viewResp.Data, &viewData)
		entry := findAssignmentEntry(t, viewData, fullCluster)
		require.NotNil(t, entry)
		// OpenAPI 视图不再暴露 standby（2026-10-14-epp-openapi-single-instance），
		// standby 仅保留在 InnerAPI epp_data 导出中（单实例组下为 null）。
		_, hasStandby := entry["standby"]
		assert.False(t, hasStandby, "standby key must be absent from the OpenAPI view")

		exportResp := getEppData(t, "")
		testutil.AssertDataNotEmpty(t, exportResp)
		firstVersionBytes, err := testutil.GetDataField(exportResp, "Version")
		require.NoError(t, err)
		firstVersion = firstVersionBytes.(string)
		require.NotEmpty(t, firstVersion)

		configVal, err := testutil.GetDataField(exportResp, "Config")
		if err != nil {
			t.Fatalf("Config missing: %v", err)
		}
		config := configVal.(map[string]interface{})

		// epp_config 段：编译后的 EndpointPickerConfig
		eppConfig := config["epp_config"].(map[string]interface{})
		compiled, ok := eppConfig[fullCluster].(map[string]interface{})
		require.True(t, ok, "compiled epp_config should contain the cluster")
		assert.Equal(t, []interface{}{"flowControl"}, compiled["featureGates"])

		flowControl := compiled["flowControl"].(map[string]interface{})
		assert.Equal(t, "200", flowControl["maxRequests"])
		assert.Equal(t, "45s", flowControl["defaultRequestTTL"])
		assert.Equal(t, "2m0s", flowControl["noEndpointRequestTTL"])
		assert.NotContains(t, flowControl, "enableEviction")
		saturationDetector := flowControl["saturationDetector"].(map[string]interface{})
		assert.Equal(t, "saturation-detector", saturationDetector["pluginRef"])

		// band 0 显式下发：ai-gateway-epp 所有请求均为 priority 0，
		// 不显式下发会静默落入 llm-d 隐藏默认（5000/1GB）截断全局配置。
		bands, ok := flowControl["priorityBands"].([]interface{})
		require.True(t, ok, "flowControl must explicitly emit priorityBands (band 0)")
		require.Len(t, bands, 1)
		band := bands[0].(map[string]interface{})
		assert.Equal(t, float64(0), band["priority"])
		assert.Equal(t, "200", band["maxRequests"], "band0 mirrors global, no silent truncation")
		assert.Equal(t, "5Gi", band["maxBytes"])

		plugins := compiled["plugins"].([]interface{})
		pluginTypes := map[string]bool{}
		for _, p := range plugins {
			plugin := p.(map[string]interface{})
			pluginTypes[plugin["name"].(string)] = true
		}
		for _, name := range []string{"ep-discover", "util-filter", "saturation-detector", "kv-scorer", "queue-scorer", "prefix-scorer", "session-scorer", "max-score", "openai-parser"} {
			assert.True(t, pluginTypes[name], "plugin %s should exist", name)
		}

		// session scorer 携带 header 配置
		for _, p := range plugins {
			plugin := p.(map[string]interface{})
			if plugin["name"] == "session-scorer" {
				params := plugin["parameters"].(map[string]interface{})
				// strategy 必须显式下发 session_id：缺失时落到 llm-d 插件默认值
				// （无状态 header 亲和），会话亲和静默失效（issue #181 回归锚点）。
				assert.Equal(t, "session_id", params["strategy"], "session-scorer strategy must be session_id")
				sessionCfg := params["sessionIdConfig"].(map[string]interface{})
				sources := sessionCfg["sources"].([]interface{})
				source := sources[0].(map[string]interface{})
				assert.Equal(t, "x-session-id", source["header"])
			}
			if plugin["name"] == "util-filter" {
				params := plugin["parameters"].(map[string]interface{})
				assert.Equal(t, true, params["fallbackOnEmpty"])
				conditions := params["conditions"].([]interface{})
				require.Len(t, conditions, 3)
				assert.Equal(t, "kv-cache-utilization", conditions[0].(map[string]interface{})["metric"])
				assert.Equal(t, 0.85, conditions[0].(map[string]interface{})["maxValue"])
				assert.Equal(t, "waiting-queue", conditions[1].(map[string]interface{})["metric"])
				assert.Equal(t, float64(5), conditions[1].(map[string]interface{})["maxValue"])
				assert.Equal(t, "running-requests", conditions[2].(map[string]interface{})["metric"])
				assert.Equal(t, float64(16), conditions[2].(map[string]interface{})["maxValue"])
			}
			if plugin["name"] == "saturation-detector" {
				params := plugin["parameters"].(map[string]interface{})
				assert.Equal(t, 0.85, params["kvCacheUtilThreshold"])
				assert.Equal(t, float64(5), params["queueDepthThreshold"])
				assert.Equal(t, "300ms", params["metricsStalenessThreshold"])
				assert.Equal(t, "ignore", params["stalenessPolicy"])
			}
		}

		// queue-first 档位权重：kv=0.2，queue=1.0；affinity=high → 亲和 scorer 权重 1.0
		profiles := compiled["schedulingProfiles"].([]interface{})
		profile := profiles[0].(map[string]interface{})
		weights := map[string]float64{}
		for _, pp := range profile["plugins"].([]interface{}) {
			profilePlugin := pp.(map[string]interface{})
			if w, ok := profilePlugin["weight"].(float64); ok {
				weights[profilePlugin["pluginRef"].(string)] = w
			}
		}
		assert.Equal(t, 0.2, weights["kv-scorer"])
		assert.Equal(t, 1.0, weights["queue-scorer"])
		assert.Equal(t, 1.0, weights["prefix-scorer"])
		assert.Equal(t, 1.0, weights["session-scorer"])

		// assignment 段：全量视图，primary 与分配视图一致；standby 保留在
		// InnerAPI 导出中，单实例组下为 null（契约未变）。
		assignment := config["assignment"].(map[string]interface{})
		entry1, ok := assignment[fullCluster].(map[string]interface{})
		require.True(t, ok, "assignment should contain the cluster")
		assert.Equal(t, fullPrimary["id"], entry1["primary"])
		assert.Nil(t, entry1["standby"], "single-instance group exports standby null in epp_data")

		entry2, ok := assignment[degradeCluster].(map[string]interface{})
		require.True(t, ok, "assignment should contain all EPP clusters")
		assert.NotEmpty(t, entry2["primary"])
	})

	t.Run("IN-EPP-004 epp_data 增量拉取同版本返回 Data null", func(t *testing.T) {
		resp := getEppData(t, firstVersion)
		assert.Equal(t, "null", string(resp.Data), "same version pull should return Data null")
	})

	t.Run("IN-EPP-005 手工覆写后版本推进且 assignment 更新", func(t *testing.T) {
		// 扩充池为两组，然后覆写到新组。
		patchResp, err := testutil.GetClient().Patch("/open-api/v1/epp-pool", map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": "g1",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
					},
				},
				map[string]interface{}{
					"name": "g2",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-b", "host": "10.0.0.2", "port": 9002},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("patch epp pool failed: %v", err)
		}
		testutil.AssertSuccess(t, patchResp)

		resp, err := testutil.GetClient().Put("/open-api/v1/epp-assignments/"+fullCluster, map[string]interface{}{
			"group_name":          "g2",
			"primary_instance_id": "epp-b",
		})
		if err != nil {
			t.Fatalf("override failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		exportResp := getEppData(t, firstVersion)
		require.NotEqual(t, "null", string(exportResp.Data), "override should bump epp_data version")
		configVal, _ := testutil.GetDataField(exportResp, "Config")
		assignment := configVal.(map[string]interface{})["assignment"].(map[string]interface{})
		entry := assignment[fullCluster].(map[string]interface{})
		assert.Equal(t, "epp-b", entry["primary"])
		assert.Nil(t, entry["standby"], "single-instance group exports standby null in epp_data")

		// 新版本再次拉取返回 null
		newVersionVal, _ := testutil.GetDataField(exportResp, "Version")
		reResp := getEppData(t, newVersionVal.(string))
		assert.Equal(t, "null", string(reResp.Data))
	})
}
