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

func putOverride(t *testing.T, cluster string, body map[string]interface{}) *testutil.APIResponse {
	resp, err := testutil.GetClient().Put("/open-api/v1/epp-assignments/"+cluster, body)
	if err != nil {
		t.Fatalf("put override failed: %v", err)
	}
	return resp
}

func TestEppAssignments_Put(t *testing.T) {
	clusterName := testutil.UniqueClusterName()

	patchResp := patchEppPool(t, twoGroupsBody())
	testutil.AssertSuccess(t, patchResp)
	createEPPCluster(t, clusterName)

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
	})

	t.Run("EA-2-001 手工覆写切换 primary 并回读验证", func(t *testing.T) {
		entry := findClusterEntry(t, getAssignments(t), clusterName)
		require.NotNil(t, entry)

		// 覆写到另一组。
		resp := putOverride(t, clusterName, map[string]interface{}{
			"group_name":          "g2",
			"primary_instance_id": "epp-c",
		})
		testutil.AssertSuccess(t, resp)

		var respData map[string]interface{}
		if err := json.Unmarshal(resp.Data, &respData); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		assert.Equal(t, clusterName, respData["cluster"])
		assert.Equal(t, "g2", respData["group"])
		respPrimary := respData["primary"].(map[string]interface{})
		assert.Equal(t, "epp-c", respPrimary["id"])
		// OpenAPI 覆写响应不再暴露 standby（key 物理删除）。
		_, hasStandby := respData["standby"]
		assert.False(t, hasStandby, "standby key must be absent from the OpenAPI response")

		// GET 视图验证覆写生效
		view := getAssignments(t, map[string]string{"cluster": clusterName})
		viewEntry := findClusterEntry(t, view, clusterName)
		require.NotNil(t, viewEntry)
		assert.Equal(t, "epp-c", viewEntry["primary"].(map[string]interface{})["id"])
		_, hasStandby = viewEntry["standby"]
		assert.False(t, hasStandby, "standby key must be absent from the OpenAPI view")
	})

	t.Run("EA-2-002 覆写回原组", func(t *testing.T) {
		resp := putOverride(t, clusterName, map[string]interface{}{
			"group_name":          "g1",
			"primary_instance_id": "epp-a",
		})
		testutil.AssertSuccess(t, resp)

		var respData map[string]interface{}
		json.Unmarshal(resp.Data, &respData)
		assert.Equal(t, "g1", respData["group"])
		assert.Equal(t, "epp-a", respData["primary"].(map[string]interface{})["id"])
		_, hasStandby := respData["standby"]
		assert.False(t, hasStandby, "standby key must be absent from the OpenAPI response")

		view := getAssignments(t, map[string]string{"cluster": clusterName})
		viewEntry := findClusterEntry(t, view, clusterName)
		require.NotNil(t, viewEntry)
		_, hasStandby = viewEntry["standby"]
		assert.False(t, hasStandby, "standby key must be absent from the OpenAPI view")
	})

	t.Run("EA-2-003 覆写不存在的 cluster 返回 404", func(t *testing.T) {
		resp := putOverride(t, "not_exist_cluster", map[string]interface{}{
			"group_name":          "g1",
			"primary_instance_id": "epp-a",
		})
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("EA-2-004 覆写 WRR 集群返回 404", func(t *testing.T) {
		wrrCluster := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(wrrCluster); err != nil {
			t.Fatalf("setup wrr cluster failed: %v", err)
		}
		defer testutil.DeleteCluster(wrrCluster)

		resp := putOverride(t, wrrCluster, map[string]interface{}{
			"group_name":          "g1",
			"primary_instance_id": "epp-a",
		})
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("EA-2-005 覆写到不存在的组返回 422", func(t *testing.T) {
		resp := putOverride(t, clusterName, map[string]interface{}{
			"group_name":          "not_exist_group",
			"primary_instance_id": "epp-a",
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("EA-2-006 primary 不在目标组返回 422", func(t *testing.T) {
		resp := putOverride(t, clusterName, map[string]interface{}{
			"group_name":          "g1",
			"primary_instance_id": "epp-c",
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("EA-2-007 缺少必填参数返回 422", func(t *testing.T) {
		resp := putOverride(t, clusterName, map[string]interface{}{
			"group_name": "g1",
		})
		testutil.AssertErrCode(t, resp, 422)

		resp = putOverride(t, clusterName, map[string]interface{}{
			"primary_instance_id": "epp-a",
		})
		testutil.AssertErrCode(t, resp, 422)
	})
}
