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

package clusters_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
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

func resetGlobalRouteRules(t *testing.T) {
	t.Helper()
	resp, err := testutil.GetClient().Put("/open-api/v1/global-route-rules", map[string]interface{}{
		"rules": []interface{}{},
	})
	if err != nil {
		t.Fatalf("reset global route rules failed: %v", err)
	}
	if resp.ErrNum != 200 {
		t.Fatalf("reset global route rules failed: %d %s", resp.ErrNum, resp.ErrMsg)
	}
}

func setGlobalRouteRules(t *testing.T, rules []interface{}) {
	t.Helper()
	resp, err := testutil.GetClient().Put("/open-api/v1/global-route-rules", map[string]interface{}{
		"rules": rules,
	})
	if err != nil {
		t.Fatalf("set global route rules failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
}

func updateEntityRouteRules(t *testing.T, entityID string, rules []interface{}) {
	t.Helper()
	resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+entityID, map[string]interface{}{
		"route_rules": map[string]interface{}{
			"enabled": true,
			"rules":   rules,
		},
	})
	if err != nil {
		t.Fatalf("update entity route rules failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
}

func updateAPIKeyRouteRules(t *testing.T, apiKeyID string, rules []interface{}) {
	t.Helper()
	resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+apiKeyID, map[string]interface{}{
		"route_rules": map[string]interface{}{
			"enabled": true,
			"rules":   rules,
		},
	})
	if err != nil {
		t.Fatalf("update api-key route rules failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
}

func routeRule(name, clusterName string) map[string]interface{} {
	return map[string]interface{}{
		"name":    name,
		"cond":    "default_t()",
		"targets": []interface{}{map[string]interface{}{"cluster_name": clusterName, "model": "", "weight": 100}},
		"fallbacks": []interface{}{},
	}
}

func routeRuleWithFallback(name, targetCluster, fallbackCluster string) map[string]interface{} {
	return map[string]interface{}{
		"name":    name,
		"cond":    "default_t()",
		"targets": []interface{}{map[string]interface{}{"cluster_name": targetCluster, "model": "", "weight": 100}},
		"fallbacks": []interface{}{map[string]interface{}{"cluster_name": fallbackCluster, "model": ""}},
	}
}

func assertDeleteBlocked(t *testing.T, clusterName string) {
	t.Helper()
	resp, err := testutil.GetClient().Delete("/open-api/v1/clusters/" + clusterName)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.ErrNum != 409 {
		t.Fatalf("expected ErrNum=409, got ErrNum=%d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
	}
	if resp.ErrMsg != "" && !strings.Contains(resp.ErrMsg, "Refer To This Cluster") && !strings.Contains(resp.ErrMsg, "集群被转发规则") {
		t.Errorf("expected error message to contain reference hint, got: %s", resp.ErrMsg)
	}
}

func assertDeleteSuccess(t *testing.T, clusterName string) {
	t.Helper()
	resp, err := testutil.GetClient().Delete("/open-api/v1/clusters/" + clusterName)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	resp, _ = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
	testutil.AssertErrCode(t, resp, 404)
}

func TestClusters_Delete(t *testing.T) {
	t.Run("CL-5-001 删除集群", func(t *testing.T) {
		clusterName := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		assertDeleteSuccess(t, clusterName)
	})

	t.Run("CL-5-002 删除不存在的集群", func(t *testing.T) {
		resp, err := testutil.GetClient().Delete("/open-api/v1/clusters/non_existent_cluster")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("CL-5-003 删除被 global 路由规则 target 引用的集群", func(t *testing.T) {
		resetGlobalRouteRules(t)
		clusterName := testutil.UniqueClusterName()
		otherCluster := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if _, err := testutil.CreateCluster(otherCluster); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		setGlobalRouteRules(t, []interface{}{routeRule("global-ref", clusterName)})
		assertDeleteBlocked(t, clusterName)

		t.Cleanup(func() {
			resetGlobalRouteRules(t)
			testutil.DeleteCluster(clusterName)
			testutil.DeleteCluster(otherCluster)
		})
	})

	t.Run("CL-5-004 删除被 global 路由规则 fallback 引用的集群", func(t *testing.T) {
		resetGlobalRouteRules(t)
		clusterName := testutil.UniqueClusterName()
		targetCluster := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if _, err := testutil.CreateCluster(targetCluster); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		setGlobalRouteRules(t, []interface{}{routeRuleWithFallback("global-fb-ref", targetCluster, clusterName)})
		assertDeleteBlocked(t, clusterName)

		t.Cleanup(func() {
			resetGlobalRouteRules(t)
			testutil.DeleteCluster(clusterName)
			testutil.DeleteCluster(targetCluster)
		})
	})

	t.Run("CL-5-005 删除被 entity 路由规则 target 引用的集群", func(t *testing.T) {
		resetGlobalRouteRules(t)
		typeName := testutil.UniqueEntityTypeName()
		if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		entityName := testutil.UniqueEntityName()
		entityID, err := testutil.CreateEntity(entityName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		clusterName := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		updateEntityRouteRules(t, entityID, []interface{}{routeRule("entity-ref", clusterName)})
		assertDeleteBlocked(t, clusterName)

		t.Cleanup(func() {
			testutil.DeleteEntity(entityID)
			testutil.DeleteCluster(clusterName)
			testutil.DeleteEntityType(typeName)
		})
	})

	t.Run("CL-5-006 删除被 apikey 路由规则 target 引用的集群", func(t *testing.T) {
		resetGlobalRouteRules(t)
		apiKeyID, err := testutil.CreateAPIKey(testutil.UniqueAPIKeyDesc(), "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		clusterName := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		updateAPIKeyRouteRules(t, apiKeyID, []interface{}{routeRule("apikey-ref", clusterName)})
		assertDeleteBlocked(t, clusterName)

		t.Cleanup(func() {
			testutil.DeleteAPIKey(apiKeyID)
			testutil.DeleteCluster(clusterName)
		})
	})

	t.Run("CL-5-007 解除引用后可删除集群", func(t *testing.T) {
		resetGlobalRouteRules(t)
		clusterName := testutil.UniqueClusterName()
		otherCluster := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if _, err := testutil.CreateCluster(otherCluster); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		setGlobalRouteRules(t, []interface{}{routeRule("global-ref", clusterName)})
		assertDeleteBlocked(t, clusterName)

		setGlobalRouteRules(t, []interface{}{routeRule("global-ref", otherCluster)})
		assertDeleteSuccess(t, clusterName)

		t.Cleanup(func() {
			resetGlobalRouteRules(t)
			testutil.DeleteCluster(otherCluster)
		})
	})

	t.Run("CL-5-008 路由规则引用其他集群时可删除", func(t *testing.T) {
		resetGlobalRouteRules(t)
		clusterName := testutil.UniqueClusterName()
		referredCluster := testutil.UniqueClusterName()
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if _, err := testutil.CreateCluster(referredCluster); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		setGlobalRouteRules(t, []interface{}{routeRule("global-other-ref", referredCluster)})
		assertDeleteSuccess(t, clusterName)

		t.Cleanup(func() {
			resetGlobalRouteRules(t)
			testutil.DeleteCluster(referredCluster)
		})
	})

	t.Run("CL-5-009 删除单字符名称的集群", func(t *testing.T) {
		clusterName := "c"
		if _, err := testutil.CreateCluster(clusterName); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		assertDeleteSuccess(t, clusterName)
	})

	t.Run("CL-5-010 删除非法名称集群必须 422", func(t *testing.T) {
		// issue #311：DELETE 此前对非法名返回 404，与 PATCH/POST 不一致。
		resp, err := testutil.GetClient().Delete("/open-api/v1/clusters/" + strings.Repeat("a", 257))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("CL-5-011 删除 65 字符名称集群必须 422", func(t *testing.T) {
		resp, err := testutil.GetClient().Delete("/open-api/v1/clusters/" + strings.Repeat("a", 65))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})
}
