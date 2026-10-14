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

package epp_assignments_test

import (
	"encoding/json"
	"os"
	"testing"

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

func patchEppPool(t *testing.T, body map[string]interface{}) *testutil.APIResponse {
	resp, err := testutil.GetClient().Patch("/open-api/v1/epp-pool", body)
	if err != nil {
		t.Fatalf("patch epp pool failed: %v", err)
	}
	return resp
}

func twoGroupsBody() map[string]interface{} {
	return map[string]interface{}{
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
					map[string]interface{}{"id": "epp-c", "host": "10.0.0.3", "port": 9002},
				},
			},
		},
	}
}

// createEPPCluster creates an EPP-mode cluster (provider auto-created) and
// returns its name.
func createEPPCluster(t *testing.T, name string) string {
	t.Helper()
	providerName := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(providerName); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}
	resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name":         name,
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
	if resp.ErrNum != 200 {
		t.Fatalf("create epp cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
	}
	return name
}

func getAssignments(t *testing.T, query ...map[string]string) map[string]interface{} {
	t.Helper()
	var resp *testutil.APIResponse
	var err error
	if len(query) > 0 {
		resp, err = testutil.GetClient().Get("/open-api/v1/epp-assignments", query[0])
	} else {
		resp, err = testutil.GetClient().Get("/open-api/v1/epp-assignments")
	}
	if err != nil {
		t.Fatalf("get assignments failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal assignments failed: %v", err)
	}
	return data
}

func findClusterEntry(t *testing.T, data map[string]interface{}, cluster string) map[string]interface{} {
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

// assertNoLegacyKeys asserts the assignment view no longer carries any
// primary/standby-era field.
func assertNoLegacyKeys(t *testing.T, raw []byte) {
	t.Helper()
	for _, legacy := range []string{"primary", "standby", "instances", "degraded"} {
		assert.NotContains(t, string(raw), "\""+legacy+"\"")
	}
}

// TestEppAssignments_UnassignedFilter 验证未分配 cluster 的过滤查询形态。
// 必须在任何 PATCH /epp-pool 之前运行（空池上创建的 EPP cluster 保持未分配态，
// 池中无可分配组时 reconciler 不会自动分配）。
func TestEppAssignments_UnassignedFilter(t *testing.T) {
	t.Run("EA-1-005 过滤未分配的 EPP cluster 返回空 clusters 并列入 unassigned", func(t *testing.T) {
		unassigned := testutil.UniqueClusterName()
		createEPPCluster(t, unassigned)
		defer testutil.DeleteCluster(unassigned)

		data := getAssignments(t, map[string]string{"cluster": unassigned})
		clusters := data["clusters"].([]interface{})
		assert.Empty(t, clusters)
		assert.Contains(t, data["unassigned_clusters"], unassigned)
	})
}

func TestEppAssignments_Get(t *testing.T) {
	clusterName := testutil.UniqueClusterName()

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
	})

	t.Run("EA-1-001 配置实例池后创建 EPP 集群自动分配", func(t *testing.T) {
		resp := patchEppPool(t, twoGroupsBody())
		testutil.AssertSuccess(t, resp)

		createEPPCluster(t, clusterName)

		data := getAssignments(t)
		raw, err := json.Marshal(data)
		require.NoError(t, err)
		assertNoLegacyKeys(t, raw)

		entry := findClusterEntry(t, data, clusterName)
		require.NotNil(t, entry, "cluster should appear in assignment view")
		assert.Equal(t, "g1", entry["group"], "greedy allocator picks least-loaded group, tie-break by name")
		assert.NotContains(t, data["idle_groups"], "g1")
		assert.Contains(t, data["idle_groups"], "g2")
		assert.NotContains(t, data["unassigned_clusters"], clusterName)
	})

	t.Run("EA-1-002 按 cluster 过滤查询", func(t *testing.T) {
		data := getAssignments(t, map[string]string{"cluster": clusterName})
		clusters := data["clusters"].([]interface{})
		require.Len(t, clusters, 1)
		entry := clusters[0].(map[string]interface{})
		assert.Equal(t, clusterName, entry["cluster"])
		assert.NotEmpty(t, entry["group"])
	})

	t.Run("EA-1-003 过滤不存在的 cluster 返回 404", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/epp-assignments", map[string]string{
			"cluster": "not_exist_cluster",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("EA-1-004 过滤 WRR 集群返回 404", func(t *testing.T) {
		wrrCluster := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(wrrCluster); err != nil {
			t.Fatalf("setup wrr cluster failed: %v", err)
		}
		defer testutil.DeleteCluster(wrrCluster)

		resp, err := testutil.GetClient().Get("/open-api/v1/epp-assignments", map[string]string{
			"cluster": wrrCluster,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})
}

// TestEppAssignments_GroupRepair 验证单实例组规则下的悬空修复语义：
// PATCH /epp-pool 同步触发修复，分配粒度为组（组仍在则分配保留）。
func TestEppAssignments_GroupRepair(t *testing.T) {
	t.Run("EA-1-006 组被移除后跨组重分配", func(t *testing.T) {
		resp := patchEppPool(t, twoGroupsBody())
		testutil.AssertSuccess(t, resp)

		cluster := testutil.UniqueClusterName()
		createEPPCluster(t, cluster)
		defer testutil.DeleteCluster(cluster)

		entry := findClusterEntry(t, getAssignments(t), cluster)
		require.NotNil(t, entry)

		// 全量替换为另一个全新组（原组被移除）→ 悬空修复跨组重分配。
		resp = patchEppPool(t, map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name":      "g-new",
					"instances": []interface{}{map[string]interface{}{"id": "epp-new", "host": "10.0.9.9", "port": 9002}},
				},
			},
		})
		testutil.AssertSuccess(t, resp)

		entry = findClusterEntry(t, getAssignments(t), cluster)
		require.NotNil(t, entry)
		assert.Equal(t, "g-new", entry["group"], "原组被移除：跨组重分配")
	})

	t.Run("EA-1-007 同组换实例后分配保留", func(t *testing.T) {
		resp := patchEppPool(t, twoGroupsBody())
		testutil.AssertSuccess(t, resp)

		cluster := testutil.UniqueClusterName()
		createEPPCluster(t, cluster)
		defer testutil.DeleteCluster(cluster)

		entry := findClusterEntry(t, getAssignments(t), cluster)
		require.NotNil(t, entry)
		group := entry["group"].(string)

		// 同组名替换为全新实例（组仍在）→ 分配保留在组上。
		replacement := map[string]interface{}{"id": "epp-replacement", "host": "10.0.9.9", "port": 9002}
		resp = patchEppPool(t, map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name":      group,
					"instances": []interface{}{replacement},
				},
			},
		})
		testutil.AssertSuccess(t, resp)

		entry = findClusterEntry(t, getAssignments(t), cluster)
		require.NotNil(t, entry)
		assert.Equal(t, group, entry["group"], "组仍存在：分配不换组")
	})
}
