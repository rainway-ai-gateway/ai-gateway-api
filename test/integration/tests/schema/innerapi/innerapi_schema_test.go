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

package innerapi

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
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

func TestInnerAPI_Schema(t *testing.T) {
	t.Run("server_data_conf", testServerDataConfSchema)
	t.Run("server_data_conf_tiered_pricing", testServerDataConfTieredPricingSchema)
	t.Run("gslb", testGSLBSchema)
	t.Run("cluster_table", testClusterTableSchema)
	t.Run("server_cert_conf", testServerCertConfSchema)
	t.Run("mod_api_key", testModAPIKeySchema)
	t.Run("mod_body_process", testModBodyProcessSchema)
	t.Run("rate_limit_policy", testRateLimitPolicySchema)
	t.Run("ai_route", testAIRouteSchema)
	t.Run("ai_cache_rule", testAICacheRuleSchema)
	t.Run("ai_context_rule", testAIContextRuleSchema)
	t.Run("traffic_mirror_rule", testTrafficMirrorRuleSchema)
	t.Run("mod_ai_intent", testModAIIntentSchema)
	t.Run("epp_data", testEppDataSchema)
	t.Run("server_data_conf_epp", testServerDataConfEppSchema)
}

// setupCluster 创建一个测试集群并返回名称
func setupCluster(t *testing.T) string {
	clusterName := testutil.UniqueClusterName()
	_, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": clusterName,
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": "deepseek",
		},
	})
	require.NoError(t, err)
	return clusterName
}

// setupAPIKeyWithRoute 创建一个启用 route_rules 的 API-Key
func setupAPIKeyWithRoute(t *testing.T, clusterName string) string {
	resp, err := testutil.GetClient().Post("/open-api/v1/api-keys", map[string]interface{}{
		"description": "inner-schema-route",
		"quota_plan": map[string]interface{}{
			"unlimited": false,
			"quota":     1000,
			"unit":      "total_token",
		},
		"route_rules": map[string]interface{}{
			"enabled": true,
			"rules": []interface{}{
				map[string]interface{}{
					"name":      "default",
					"cond":      "default_t()",
					"targets":   []interface{}{map[string]interface{}{"cluster_name": clusterName, "model": "", "weight": 100}},
					"fallbacks": []interface{}{},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
	id, err := testutil.GetDataField(resp, "id")
	require.NoError(t, err)
	return id.(string)
}

func testServerDataConfSchema(t *testing.T) {
	clusterName := setupCluster(t)

	// Create an anthropic provider and cluster to verify AIConf.ModelProtocols export.
	anthropicProviderName := testutil.UniqueProviderName()
	_, err := testutil.CreateProvider(anthropicProviderName, map[string]interface{}{
		"model_protocols": []string{"anthropic"},
		"models":          []string{"claude-3-5-sonnet-20241022"},
	})
	require.NoError(t, err)

	anthropicClusterName := testutil.UniqueClusterName()
	_, err = testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": anthropicClusterName,
		"llm_config": map[string]interface{}{
			"models":   []string{"claude-3-5-sonnet-20241022"},
			"provider": anthropicProviderName,
		},
	})
	require.NoError(t, err)

	// Create a cluster with key_affinity to verify AIConf.KeyPolicy.SessionAffinity* export.
	affinityProviderName := testutil.UniqueProviderName()
	_, err = testutil.CreateProvider(affinityProviderName, map[string]interface{}{
		"models": []string{"deepseek-chat"},
	})
	require.NoError(t, err)

	affinityClusterName := testutil.UniqueClusterName()
	_, err = testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": affinityClusterName,
		"llm_config": map[string]interface{}{
			"models": []string{"deepseek-chat"},
			"key_affinity": map[string]interface{}{
				"enabled":        true,
				"ttl":            600,
				"redis_prefix":   "bfe:ai:key_affinity",
				"penalty_enable": true,
			},
			"provider": affinityProviderName,
		},
	})
	require.NoError(t, err)

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, ServerDataConfSchema)
		assertServerDataConfModelProtocols(t, resp.Data, anthropicClusterName)
		assertServerDataConfKeyAffinity(t, resp.Data, affinityClusterName)
	}

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteCluster(anthropicClusterName)
		testutil.DeleteCluster(affinityClusterName)
		testutil.DeleteProvider(anthropicProviderName)
		testutil.DeleteProvider(affinityProviderName)
	})
}

// assertServerDataConfKeyAffinity 校验导出结果中指定 cluster 的 AIConf.KeyPolicy.SessionAffinity*。
func assertServerDataConfKeyAffinity(t *testing.T, data []byte, clusterName string) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal server_data_conf data: %v", err)
	}
	clusterConf, ok := payload["ClusterConf"].(map[string]interface{})
	if !ok {
		t.Fatal("ClusterConf is not an object")
	}
	config, ok := clusterConf["Config"].(map[string]interface{})
	if !ok {
		t.Fatal("ClusterConf.Config is not an object")
	}
	cluster, ok := config[clusterName].(map[string]interface{})
	if !ok {
		t.Fatalf("cluster %s not found in ClusterConf.Config", clusterName)
	}
	aiconf, ok := cluster["AIConf"].(map[string]interface{})
	if !ok {
		t.Fatalf("AIConf not found for cluster %s", clusterName)
	}
	keyPolicy, ok := aiconf["KeyPolicy"].(map[string]interface{})
	if !ok {
		t.Fatalf("AIConf.KeyPolicy is not an object for cluster %s", clusterName)
	}
	assert.Equal(t, true, keyPolicy["SessionAffinity"])
	assert.Equal(t, float64(600), keyPolicy["SessionAffinityTTL"])
	assert.Equal(t, "bfe:ai:key_affinity", keyPolicy["SessionAffinityRedisPrefix"])
	assert.Equal(t, true, keyPolicy["SessionAffinityPenaltyEnable"])
}

// assertServerDataConfModelProtocols 校验导出结果中指定 cluster 的 AIConf.ModelProtocols。
func assertServerDataConfModelProtocols(t *testing.T, data []byte, clusterName string) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal server_data_conf data: %v", err)
	}
	clusterConf, ok := payload["ClusterConf"].(map[string]interface{})
	if !ok {
		t.Fatal("ClusterConf is not an object")
	}
	config, ok := clusterConf["Config"].(map[string]interface{})
	if !ok {
		t.Fatal("ClusterConf.Config is not an object")
	}
	cluster, ok := config[clusterName].(map[string]interface{})
	if !ok {
		t.Fatalf("cluster %s not found in ClusterConf.Config", clusterName)
	}
	aiconf, ok := cluster["AIConf"].(map[string]interface{})
	if !ok {
		t.Fatalf("AIConf not found for cluster %s", clusterName)
	}
	modelProtocols, ok := aiconf["ModelProtocols"].([]interface{})
	if !ok || len(modelProtocols) != 1 {
		t.Fatalf("expected AIConf.ModelProtocols=[anthropic] for cluster %s, got %v", clusterName, aiconf["ModelProtocols"])
	}
	if modelProtocols[0] != "anthropic" {
		t.Fatalf("expected ModelProtocols[0]=anthropic for cluster %s, got %v", clusterName, modelProtocols[0])
	}
}

func testServerDataConfTieredPricingSchema(t *testing.T) {
	providerName := testutil.UniqueProviderName()
	_, err := testutil.CreateProvider(providerName)
	require.NoError(t, err)

	tierResp, err := testutil.UpdatePricingTiers(providerName, map[string]interface{}{
		"time_zone": "Asia/Shanghai",
		"tiers": []interface{}{
			map[string]interface{}{
				"name": "peak",
				"time_ranges": []interface{}{
					map[string]interface{}{"weekdays": []int{1, 2, 3, 4, 5}, "start": "09:00", "end": "12:00"},
					map[string]interface{}{"weekdays": []int{1, 2, 3, 4, 5}, "start": "14:00", "end": "18:00"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, tierResp.ErrNum, tierResp.ErrMsg)

	yamlContent := []byte(`version: v1.0
default_currency: RMB
models:
  - provider: ` + providerName + `
    model: deepseek-chat
    base_model: deepseek-chat
    mode: chat
    prices:
      input_cost_per_token: 0.000002
      output_cost_per_token: 0.000008
      cache_read_input_token_cost: 0.0000005
    tier_prices:
      peak:
        input_cost_per_token: 0.000004
        output_cost_per_token: 0.000016
        cache_read_input_token_cost: 0.000001
`)
	err = testutil.ImportModelPrices(yamlContent, "replace")
	require.NoError(t, err)

	clusterName := testutil.UniqueClusterName()
	_, err = testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": clusterName,
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": providerName,
		},
	})
	require.NoError(t, err)

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, ServerDataConfSchema)
		assertServerDataConfTieredPricing(t, resp.Data, clusterName)
	}

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteModelPriceByQuery(providerName, "deepseek-chat", "chat")
		testutil.DeleteProvider(providerName)
	})
}

// assertServerDataConfTieredPricing 校验导出结果中指定 cluster 的 AIConf.ModelTable 携带分时段定价。
func assertServerDataConfTieredPricing(t *testing.T, data []byte, clusterName string) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal server_data_conf data: %v", err)
	}
	clusterConf, ok := payload["ClusterConf"].(map[string]interface{})
	if !ok {
		t.Fatal("ClusterConf is not an object")
	}
	config, ok := clusterConf["Config"].(map[string]interface{})
	if !ok {
		t.Fatal("ClusterConf.Config is not an object")
	}
	cluster, ok := config[clusterName].(map[string]interface{})
	if !ok {
		t.Fatalf("cluster %s not found in ClusterConf.Config", clusterName)
	}
	aiconf, ok := cluster["AIConf"].(map[string]interface{})
	if !ok {
		t.Fatalf("AIConf not found for cluster %s", clusterName)
	}
	modelTable, ok := aiconf["ModelTable"].(map[string]interface{})
	if !ok {
		t.Fatalf("AIConf.ModelTable is not an object for cluster %s", clusterName)
	}

	modelTableJSON, err := json.Marshal(modelTable)
	if err != nil {
		t.Fatalf("marshal model table: %v", err)
	}
	testutil.AssertSchema(t, &testutil.APIResponse{Data: modelTableJSON}, ModelTableSchema)

	assert.Equal(t, "RMB", modelTable["Currency"])
	assert.Equal(t, "Asia/Shanghai", modelTable["TimeZone"])

	tiers, ok := modelTable["Tiers"].([]interface{})
	require.True(t, ok, "ModelTable.Tiers should be an array")
	require.Len(t, tiers, 1)
	tier0, _ := tiers[0].(map[string]interface{})
	assert.Equal(t, "peak", tier0["Name"])

	models, ok := modelTable["Models"].([]interface{})
	require.True(t, ok, "ModelTable.Models should be an array")
	require.Len(t, models, 1)
	model0, _ := models[0].(map[string]interface{})
	assert.Equal(t, "deepseek-chat", model0["Model"])

	prices, ok := model0["Prices"].(map[string]interface{})
	require.True(t, ok, "ModelPrice.Prices should be an object")
	assert.Equal(t, 0.000002, prices["input_cost_per_token"])

	tierPrices, ok := model0["TierPrices"].(map[string]interface{})
	require.True(t, ok, "ModelPrice.TierPrices should be an object")
	peak, ok := tierPrices["peak"].(map[string]interface{})
	require.True(t, ok, "TierPrices.peak should be an object")
	assert.Equal(t, 0.000004, peak["input_cost_per_token"])
	assert.Equal(t, 0.000016, peak["output_cost_per_token"])
	assert.Equal(t, 0.000001, peak["cache_read_input_token_cost"])
}

func testGSLBSchema(t *testing.T) {
	clusterName := setupCluster(t)

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/gslb_data/gslb", map[string]string{
		"bfe_cluster": "BFE-AI_product.szyf",
	})
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, GSLBSchema)
	}

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
	})
}

func testClusterTableSchema(t *testing.T) {
	clusterName := setupCluster(t)

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/gslb_data/cluster_table")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, ClusterTableSchema)
	}

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
	})
}

func testServerCertConfSchema(t *testing.T) {
	certName := testutil.UniqueCertName()
	certPEM, keyPEM, err := testutil.GenerateTestCert(certName)
	require.NoError(t, err)

	_, err = testutil.GetClient().Post("/open-api/v1/certificates", map[string]interface{}{
		"cert_name":         certName,
		"description":       "schema test",
		"is_default":        true,
		"cert_file_content": certPEM,
		"key_file_content":  keyPEM,
	})
	require.NoError(t, err)

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/protocol/server_cert_conf")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, ServerCertConfSchema)
	}

	t.Cleanup(func() {
		_ = testutil.DeleteCertificate(certName)
	})
}

func testModAPIKeySchema(t *testing.T) {
	resp, err := testutil.GetClient().Post("/open-api/v1/api-keys", map[string]interface{}{
		"description": "inner-schema-mod-api-key",
		"quota_plan": map[string]interface{}{
			"unlimited": false,
			"quota":     1000,
			"unit":      "total_token",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
	id, err := testutil.GetDataField(resp, "id")
	require.NoError(t, err)
	apiKeyID := id.(string)

	innerResp, err := testutil.GetClient().Get("/inner-api/v1/configs/mod-api-key")
	require.NoError(t, err)
	testutil.AssertSuccess(t, innerResp)
	if innerResp.Data != nil && string(innerResp.Data) != "null" {
		testutil.AssertSchema(t, innerResp, ModAPIKeySchema)
		assertModAPIKeyFieldDetails(t, innerResp.Data)
	}

	t.Cleanup(func() {
		testutil.DeleteAPIKey(apiKeyID)
	})
}

// assertModAPIKeyFieldDetails 校验 /configs/mod-api-key 响应中 tokens 和 QuotaPlans 的字段细节：
// token 的 enabled 为 bool，不包含 status/update_time；QuotaPlan 不包含 CreateTime/ResetMode；Tags 包含 TagLevel。
func assertModAPIKeyFieldDetails(t *testing.T, data []byte) {
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal mod-api-key data: %v", err)
	}

	tokens, ok := payload["tokens"].(map[string]interface{})
	if !ok {
		t.Fatal("tokens is not an object")
	}
	for product, productTokens := range tokens {
		productTokenMap, ok := productTokens.(map[string]interface{})
		if !ok {
			t.Fatalf("tokens.%s is not an object", product)
		}
		for key, token := range productTokenMap {
			tokenMap, ok := token.(map[string]interface{})
			if !ok {
				t.Fatalf("tokens.%s.%s is not an object", product, key)
			}
			if _, ok := tokenMap["status"]; ok {
				t.Errorf("tokens.%s.%s should not contain status", product, key)
			}
			if _, ok := tokenMap["update_time"]; ok {
				t.Errorf("tokens.%s.%s should not contain update_time", product, key)
			}
			if enabled, ok := tokenMap["enabled"].(bool); !ok {
				t.Errorf("tokens.%s.%s.enabled should be bool", product, key)
			} else if !enabled {
				t.Errorf("tokens.%s.%s.enabled should be true", product, key)
			}
			if keyID, ok := tokenMap["key_id"].(string); !ok || keyID == "" {
				t.Errorf("tokens.%s.%s.key_id should be non-empty string", product, key)
			}
			if expiredTime, ok := tokenMap["expired_time"].(float64); !ok {
				t.Errorf("tokens.%s.%s.expired_time should be number", product, key)
			} else if expiredTime != -1 {
				// ok, can be -1 or a timestamp
			}
			if _, ok := tokenMap["unlimited_quota"].(bool); !ok {
				t.Errorf("tokens.%s.%s.unlimited_quota should be bool", product, key)
			}
			if models, ok := tokenMap["allow_models"].(string); ok && models == "*" {
				t.Errorf("tokens.%s.%s.allow_models should not be '*', got %q", product, key, models)
			}
			if quotaPlans, ok := tokenMap["quota_plans"].([]interface{}); ok {
				for i, qp := range quotaPlans {
					if _, ok := qp.(string); !ok {
						t.Errorf("tokens.%s.%s.quota_plans[%d] should be string", product, key, i)
					}
				}
			}
			if tags, ok := tokenMap["Tags"].([]interface{}); ok {
				for i, tag := range tags {
					tagMap, ok := tag.(map[string]interface{})
					if !ok {
						t.Fatalf("tokens.%s.%s.Tags[%d] is not an object", product, key, i)
					}
					if _, ok := tagMap["TagName"].(string); !ok {
						t.Errorf("tokens.%s.%s.Tags[%d].TagName should be string", product, key, i)
					}
					if _, ok := tagMap["TagValue"].(string); !ok {
						t.Errorf("tokens.%s.%s.Tags[%d].TagValue should be string", product, key, i)
					}
					if _, ok := tagMap["TagLevel"].(float64); !ok {
						t.Errorf("tokens.%s.%s.Tags[%d].TagLevel should be int", product, key, i)
					}
				}
			}
		}
	}

	quotaPlans, ok := payload["QuotaPlans"].(map[string]interface{})
	if !ok {
		t.Fatal("QuotaPlans is not an object")
	}
	for product, plans := range quotaPlans {
		planList, ok := plans.([]interface{})
		if !ok {
			t.Fatalf("QuotaPlans.%s is not an array", product)
		}
		for i, plan := range planList {
			planMap, ok := plan.(map[string]interface{})
			if !ok {
				t.Fatalf("QuotaPlans.%s[%d] is not an object", product, i)
			}
			if _, ok := planMap["CreateTime"]; ok {
				t.Errorf("QuotaPlans.%s[%d] should not contain CreateTime", product, i)
			}
			if _, ok := planMap["ResetMode"]; ok {
				t.Errorf("QuotaPlans.%s[%d] should not contain ResetMode", product, i)
			}
			if id, ok := planMap["Id"].(string); !ok || id == "" {
				t.Errorf("QuotaPlans.%s[%d].Id should be non-empty string", product, i)
			}
			if _, ok := planMap["Unlimited"].(bool); !ok {
				t.Errorf("QuotaPlans.%s[%d].Unlimited should be bool", product, i)
			}
			if _, ok := planMap["PassNoQuota"].(bool); !ok {
				t.Errorf("QuotaPlans.%s[%d].PassNoQuota should be bool", product, i)
			}
			if redisKey, ok := planMap["RedisKey"].(string); !ok || redisKey == "" {
				t.Errorf("QuotaPlans.%s[%d].RedisKey should be non-empty string", product, i)
			}
			if _, ok := planMap["ExpiredTime"].(float64); !ok {
				t.Errorf("QuotaPlans.%s[%d].ExpiredTime should be number", product, i)
			}
			if _, ok := planMap["Quota"].(float64); !ok {
				t.Errorf("QuotaPlans.%s[%d].Quota should be number", product, i)
			}
		}
	}
}

func testModBodyProcessSchema(t *testing.T) {
	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/mod-body-process")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, ModBodyProcessSchema)
	}
}

func testRateLimitPolicySchema(t *testing.T) {
	resp, err := testutil.GetClient().Post("/open-api/v1/api-keys", map[string]interface{}{
		"description": "inner-schema-rate-limit",
		"rate_limit_policy": map[string]interface{}{
			"enabled": true,
			"rules": map[string]interface{}{
				"tpm": []interface{}{
					map[string]interface{}{"name": "tpm-1m", "model": "*", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1},
				},
				"rpm": []interface{}{
					map[string]interface{}{"name": "rpm-1m", "model": "*", "window_minutes": 1, "max_requests": 10},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
	id, err := testutil.GetDataField(resp, "id")
	require.NoError(t, err)
	apiKeyID := id.(string)

	innerResp, err := testutil.GetClient().Get("/inner-api/v1/configs/rate-limit-policy")
	require.NoError(t, err)
	testutil.AssertSuccess(t, innerResp)
	if innerResp.Data != nil && string(innerResp.Data) != "null" {
		testutil.AssertSchema(t, innerResp, RateLimitPolicySchema)
		assertRateLimitPolicyFieldDetails(t, innerResp.Data)
	}

	t.Cleanup(func() {
		testutil.DeleteAPIKey(apiKeyID)
	})
}

// assertRateLimitPolicyFieldDetails 校验 /configs/rate-limit-policy 中每条 TPM/RPM 规则都包含非空的 redis_key。
func assertRateLimitPolicyFieldDetails(t *testing.T, data []byte) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal rate-limit-policy data: %v", err)
	}

	policies, ok := payload["RateLimitPolicies"].(map[string]interface{})
	if !ok {
		t.Fatal("RateLimitPolicies is not an object")
	}
	if len(policies) == 0 {
		t.Fatal("RateLimitPolicies is empty")
	}

	for policyKey, policy := range policies {
		policyMap, ok := policy.(map[string]interface{})
		if !ok {
			t.Fatalf("RateLimitPolicies.%s is not an object", policyKey)
		}
		if name, ok := policyMap["name"].(string); !ok || name == "" {
			t.Errorf("RateLimitPolicies.%s.name should be non-empty string", policyKey)
		}
		if enabled, ok := policyMap["enabled"].(bool); !ok {
			t.Errorf("RateLimitPolicies.%s.enabled should be bool", policyKey)
		} else if !enabled {
			t.Errorf("RateLimitPolicies.%s.enabled should be true", policyKey)
		}

		rules, ok := policyMap["rules"].(map[string]interface{})
		if !ok {
			t.Fatalf("RateLimitPolicies.%s.rules is not an object", policyKey)
		}

		assertRuleRedisKey(t, policyKey, "tpm", rules)
		assertRuleRedisKey(t, policyKey, "rpm", rules)
	}
}

func assertRuleRedisKey(t *testing.T, policyKey, ruleType string, rules map[string]interface{}) {
	ruleList, ok := rules[ruleType].([]interface{})
	if !ok {
		t.Fatalf("RateLimitPolicies.%s.rules.%s is not an array", policyKey, ruleType)
	}
	for i, item := range ruleList {
		rule, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("RateLimitPolicies.%s.rules.%s[%d] is not an object", policyKey, ruleType, i)
		}
		redisKey, ok := rule["redis_key"].(string)
		if !ok || redisKey == "" {
			t.Errorf("RateLimitPolicies.%s.rules.%s[%d].redis_key should be non-empty string", policyKey, ruleType, i)
			continue
		}
		// redis_key 可能包含产品线/集群前缀（如 default_bfe_<policy>_RL_...），
		// 因此校验关键片段而非严格前缀。
		wantInfix := "RL_" + strings.ToUpper(ruleType) + "_" + policyKey + "_"
		if !strings.Contains(redisKey, wantInfix) {
			t.Errorf("RateLimitPolicies.%s.rules.%s[%d].redis_key format mismatch: got %s, want contain %s",
				policyKey, ruleType, i, redisKey, wantInfix)
		}
	}
}

func testAIRouteSchema(t *testing.T) {
	// 设置 global route
	_, err := testutil.GetClient().Put("/open-api/v1/global-route-rules", map[string]interface{}{
		"enabled": true,
		"rules": []interface{}{
			map[string]interface{}{
				"name":      "global-default",
				"cond":      "default_t()",
				"targets":   []interface{}{map[string]interface{}{"cluster_name": "cluster_global", "model": "", "weight": 100}},
				"fallbacks": []interface{}{},
			},
		},
	})
	require.NoError(t, err)

	clusterName := setupCluster(t)
	apiKeyID := setupAPIKeyWithRoute(t, clusterName)

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/ai-route")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data != nil && string(resp.Data) != "null" {
		testutil.AssertSchema(t, resp, AIRouteSchema)
	}

	t.Cleanup(func() {
		testutil.DeleteAPIKey(apiKeyID)
		testutil.DeleteCluster(clusterName)
	})
}

// ---------- ai_cache_rule ----------

// phaseTwoAICacheFields 一期不得导出的二期/预留字段（ai-cache-rule.md §3.2 合同锁定）。
var phaseTwoAICacheFields = []string{
	"cacheKeyFrom", "cacheValueFrom", "cacheStreamValueFrom",
	"cacheToolCallsFrom", "responseTemplate", "streamResponseTemplate",
}

func testAICacheRuleSchema(t *testing.T) {
	// 自建集合：1 条仅 name+cond（默认值回填），1 条显式非默认。
	validCond := `req_path_in("/v1/chat/completions", false)`
	putResp, err := testutil.GetClient().Put("/open-api/v1/ai-cache-rules", map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"name": testutil.UniqueName("inner-schema-ac-1"), "cond": validCond},
			map[string]interface{}{
				"name": testutil.UniqueName("inner-schema-ac-2"), "cond": validCond,
				"cache_key_strategy": "allQuestions", "cache_ttl": 3600,
				"max_body_bytes": 2097152, "max_value_bytes": 2097152,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, putResp.ErrNum, putResp.ErrMsg)
	t.Cleanup(func() {
		_, _ = testutil.GetClient().Put("/open-api/v1/ai-cache-rules", map[string]interface{}{
			"rules": []interface{}{},
		})
	})

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/ai-cache-rule")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data == nil || string(resp.Data) == "null" {
		t.Fatal("first pull must return data")
	}
	testutil.AssertSchema(t, resp, AICacheRuleExportSchema)

	// 定向断言 1：Version/Semantic/Config 存在，Config.AI_product 长度=2。
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &payload))
	version, ok := payload["Version"].(string)
	require.True(t, ok, "Version should be string")
	require.NotEmpty(t, version)

	// Semantic 块恒导出（二期）：空设置表时以默认值 1/0.15/lt 填充。
	semantic, ok := payload["Semantic"].(map[string]interface{})
	require.True(t, ok, "Semantic should be object")
	assert.Equal(t, float64(1), semantic["topK"])
	assert.InDelta(t, 0.15, semantic["threshold"], 1e-9)
	assert.Equal(t, "lt", semantic["thresholdRelation"])

	config, ok := payload["Config"].(map[string]interface{})
	require.True(t, ok, "Config should be object")
	productRules, ok := config["AI_product"].([]interface{})
	require.True(t, ok, "Config.AI_product should be array")
	require.Len(t, productRules, 2)

	// 定向断言 2：每条规则恰含 6 个导出 tag（精确集合，防幻影键），值正确。
	wantValues := []map[string]interface{}{
		{
			"cond": validCond, "cacheKeyStrategy": "lastQuestion",
			"cacheTTL": float64(0), "maxBodyBytes": float64(1048576), "maxValueBytes": float64(1048576),
			"enableSemanticCache": false,
		},
		{
			"cond": validCond, "cacheKeyStrategy": "allQuestions",
			"cacheTTL": float64(3600), "maxBodyBytes": float64(2097152), "maxValueBytes": float64(2097152),
			"enableSemanticCache": false,
		},
	}
	wantKeys := []string{
		"cond", "cacheKeyStrategy", "cacheTTL", "maxBodyBytes", "maxValueBytes",
		"enableSemanticCache",
	}
	for i, item := range productRules {
		rule, ok := item.(map[string]interface{})
		require.True(t, ok, "AI_product[%d] should be object", i)
		keys := make([]string, 0, len(rule))
		for k := range rule {
			keys = append(keys, k)
		}
		assert.ElementsMatch(t, wantKeys, keys, "AI_product[%d] must carry exactly the 6 contract tags", i)
		for k, v := range wantValues[i] {
			assert.Equal(t, v, rule[k], "AI_product[%d].%s", i, k)
		}
	}

	// 定向断言 3：二期 6 字段在整个导出 body 中缺席（合同锁定）。
	body := string(resp.RawBody)
	for _, field := range phaseTwoAICacheFields {
		assert.NotContains(t, body, field, "phase-2 field %s must not appear in export", field)
	}
}

// ---------- ai_context_rule ----------

// phaseTwoAIContextFields 一期不得导出的二期字段（ai-context-rule.md §3.1/§3.3 合同锁定：
// 规则级 override、Defaults 内 summary；BFE 未知字段忽略并计数告警，升级平滑）。
var phaseTwoAIContextFields = []string{"override", "summary"}

// testAIContextRuleSchema 覆盖 /configs/ai-context-rule 导出 schema。
// Defaults 块恒导出（空设置表时以文档默认值填充，本包内无其他用例写 ai_context 设置），
// 与 Version/Config 并列顶层键；规则恰含 4 个 camelCase 冻结 tag。
func testAIContextRuleSchema(t *testing.T) {
	// 自建集合：1 条 balanced 带显式预算 + 1 条 default_t() 兜底 off。
	validCond := `req_path_in("/v1/chat/completions", false)`
	putResp, err := testutil.GetClient().Put("/open-api/v1/ai-context-rules", map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"cond": validCond, "mode": "balanced",
				"max_context_tokens": 64000, "reserve_tokens": 8192,
			},
			map[string]interface{}{"cond": "default_t()", "mode": "off"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, putResp.ErrNum, putResp.ErrMsg)
	t.Cleanup(func() {
		_, _ = testutil.GetClient().Put("/open-api/v1/ai-context-rules", map[string]interface{}{
			"rules": []interface{}{},
		})
	})

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/ai-context-rule")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data == nil || string(resp.Data) == "null" {
		t.Fatal("first pull must return data")
	}
	testutil.AssertSchema(t, resp, AIContextRuleExportSchema)

	// 定向断言 1：Version 非空；Defaults 块恒导出且空表时恰为文档默认值（8 项）。
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &payload))
	version, ok := payload["Version"].(string)
	require.True(t, ok, "Version should be string")
	require.NotEmpty(t, version)

	defaults, ok := payload["Defaults"].(map[string]interface{})
	require.True(t, ok, "Defaults should always be exported")
	assert.ElementsMatch(t,
		[]string{
			"triggerRatio", "keepLatestImages", "toolResultMaxChars",
			"thinkingPolicy", "charsPerToken", "imageTokenEstimate", "rewrite",
		}, keysOfInnerMap(defaults), "Defaults must carry exactly the 7 frozen top-level tags")
	assert.InDelta(t, 0.7, defaults["triggerRatio"], 1e-9)
	assert.Equal(t, float64(2), defaults["keepLatestImages"])
	assert.Equal(t, float64(2000), defaults["toolResultMaxChars"])
	assert.Equal(t, "trim-all-but-last", defaults["thinkingPolicy"])
	assert.Equal(t, float64(4), defaults["charsPerToken"])
	assert.Equal(t, float64(1200), defaults["imageTokenEstimate"])
	defaultsRewrite, ok := defaults["rewrite"].(map[string]interface{})
	require.True(t, ok, "rewrite should be object")
	assert.Equal(t, "lite", defaultsRewrite["strength"])
	assert.InDelta(t, 0.95, defaultsRewrite["protectedSurvivalRate"], 1e-9)

	// 定向断言 2：Config.AI_product 长度=2，每条规则恰含 4 个导出 tag，值正确。
	config, ok := payload["Config"].(map[string]interface{})
	require.True(t, ok, "Config should be object")
	productRules, ok := config["AI_product"].([]interface{})
	require.True(t, ok, "Config.AI_product should be array")
	require.Len(t, productRules, 2)

	wantKeys := []string{"cond", "mode", "maxContextTokens", "reserveTokens"}
	wantValues := []map[string]interface{}{
		{
			"cond": validCond, "mode": "balanced",
			"maxContextTokens": float64(64000), "reserveTokens": float64(8192),
		},
		{
			"cond": "default_t()", "mode": "off",
			"maxContextTokens": float64(0), "reserveTokens": float64(0),
		},
	}
	for i, item := range productRules {
		rule, ok := item.(map[string]interface{})
		require.True(t, ok, "AI_product[%d] should be object", i)
		assert.ElementsMatch(t, wantKeys, keysOfInnerMap(rule),
			"AI_product[%d] must carry exactly the 4 contract tags", i)
		for k, v := range wantValues[i] {
			assert.Equal(t, v, rule[k], "AI_product[%d].%s", i, k)
		}
	}

	// 定向断言 3：二期字段（override/summary）在整个导出 body 中缺席（合同锁定）。
	body := string(resp.RawBody)
	for _, field := range phaseTwoAIContextFields {
		assert.NotContains(t, body, field, "phase-2 field %s must not appear in export", field)
	}
}

// ---------- traffic_mirror_rule ----------

// testTrafficMirrorRuleSchema 覆盖 /configs/traffic-mirror-rule 导出 schema。
// 合同锁定：每条规则恰含 7 个导出 tag（全字段恒输出）；removeHeaders 两层默认语义
// （缺省填默认黑名单、显式 [] 不剔除）以取值断言锁死。
func testTrafficMirrorRuleSchema(t *testing.T) {
	clusterName, err := testutil.CreateCluster(testutil.UniqueClusterName())
	require.NoError(t, err)

	validCond := `req_path_prefix_in("/v1/chat/completions", true)`
	validCondB := `default_t()`
	validCondC := `req_path_prefix_in("/v1/completions", true)`
	defaultBlacklist := []string{"Authorization", "Cookie", "X-Api-Key"}

	// 自建集合：1 条仅必填（缺省 remove_headers），1 条显式空黑名单，1 条全字段。
	putResp, err := testutil.GetClient().Put("/open-api/v1/traffic-mirror-rules", map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{"name": testutil.UniqueName("inner-schema-tm-1"), "cond": validCond, "mirror_cluster": clusterName},
			map[string]interface{}{
				"name": testutil.UniqueName("inner-schema-tm-2"), "cond": validCondB, "mirror_cluster": clusterName,
				"remove_headers": []interface{}{},
			},
			map[string]interface{}{
				"name": testutil.UniqueName("inner-schema-tm-3"), "cond": validCondC, "mirror_cluster": clusterName,
				"percentage":     10,
				"remove_headers": []interface{}{"X-Custom-Secret"},
				"set_headers":    map[string]interface{}{"X-Shadow-Env": "pre-release"},
				"body_rewrites":  []interface{}{map[string]interface{}{"path": "model", "value": "deepseek-v3"}},
				"path_rewrite":   "/v1/internal/chat/completions",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, putResp.ErrNum, putResp.ErrMsg)
	t.Cleanup(func() {
		_, _ = testutil.GetClient().Put("/open-api/v1/traffic-mirror-rules", map[string]interface{}{
			"rules": []interface{}{},
		})
	})

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/traffic-mirror-rule")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data == nil || string(resp.Data) == "null" {
		t.Fatal("first pull must return data")
	}
	testutil.AssertSchema(t, resp, TrafficMirrorRuleExportSchema)

	// 定向断言 1：Version/Config 存在，Config.AI_product 长度=3、顺序=提交顺序。
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &payload))
	version, ok := payload["Version"].(string)
	require.True(t, ok, "Version should be string")
	require.NotEmpty(t, version)
	config, ok := payload["Config"].(map[string]interface{})
	require.True(t, ok, "Config should be object")
	productRules, ok := config["AI_product"].([]interface{})
	require.True(t, ok, "Config.AI_product should be array")
	require.Len(t, productRules, 3)

	// 定向断言 2：每条规则恰含 7 个导出 tag（精确集合，防幻影键），值正确。
	wantKeys := []string{
		"cond", "mirrorCluster", "percentage", "removeHeaders", "setHeaders", "bodyRewrites", "pathRewrite",
	}
	type wantRule struct {
		cond          string
		percentage    float64
		removeHeaders []string
	}
	wants := []wantRule{
		{cond: validCond, percentage: 100, removeHeaders: defaultBlacklist}, // 缺省 → 默认黑名单
		{cond: validCondB, percentage: 100, removeHeaders: []string{}},      // 显式 [] → 不剔除
		{cond: validCondC, percentage: 10, removeHeaders: []string{"X-Custom-Secret"}},
	}
	for i, item := range productRules {
		rule, ok := item.(map[string]interface{})
		require.True(t, ok, "AI_product[%d] should be object", i)
		keys := make([]string, 0, len(rule))
		for k := range rule {
			keys = append(keys, k)
		}
		assert.ElementsMatch(t, wantKeys, keys, "AI_product[%d] must carry exactly the 7 export tags", i)
		assert.Equal(t, wants[i].cond, rule["cond"], "AI_product[%d].cond", i)
		assert.Equal(t, clusterName, rule["mirrorCluster"], "AI_product[%d].mirrorCluster", i)
		assert.Equal(t, wants[i].percentage, rule["percentage"], "AI_product[%d].percentage", i)
		gotRH, ok := rule["removeHeaders"].([]interface{})
		require.True(t, ok, "AI_product[%d].removeHeaders should be array", i)
		gotRHStr := make([]string, 0, len(gotRH))
		for _, h := range gotRH {
			gotRHStr = append(gotRHStr, h.(string))
		}
		assert.ElementsMatch(t, wants[i].removeHeaders, gotRHStr,
			"AI_product[%d].removeHeaders 两层默认语义", i)
	}

	// 定向断言 3：两层默认语义的原始 body 文本形态（家族8/#102）。
	body := string(resp.RawBody)
	assert.Contains(t, body, `"removeHeaders":["Authorization","Cookie","X-Api-Key"]`)
	assert.Contains(t, body, `"removeHeaders":[]`)
	assert.Contains(t, body, `"removeHeaders":["X-Custom-Secret"]`)
	assert.Contains(t, body, `"setHeaders":{}`)
	assert.Contains(t, body, `"pathRewrite":""`)
	assert.Contains(t, body, `"bodyRewrites":[{"path":"model","value":"deepseek-v3"}]`)
}

// ---------- mod_ai_intent ----------

// intentConfigVersionPattern 锁定 Version 的时间戳格式（yyyyMMddHHmmss）。
var intentConfigVersionPattern = regexp.MustCompile(`^\d{14}$`)

// testModAIIntentSchema 覆盖 /configs/mod-ai-intent 导出 schema。
// 合同锁定：Data 即 intent_questions.data 文件内容原样——Version 内嵌文件
// （时间戳格式）、PascalCase 字段，无 Config 包装层（ai-route 形态，非 ai-cache
// 两段结构）；choice/score 互斥键集合精确；version 增量参数（传当前 version）
// 返回 Data:null（mod-ai-intent.md §3）。
func testModAIIntentSchema(t *testing.T) {
	const exportPath = "/inner-api/v1/configs/mod-ai-intent"

	// 未发布首拉：Data 为 null（conf-agent 不落盘、不触发 reload）。
	firstResp, err := testutil.GetClient().Get(exportPath)
	require.NoError(t, err)
	testutil.AssertSuccess(t, firstResp)
	testutil.AssertDataNull(t, firstResp)

	// 发布配置（1 choice + 1 score，含逐问题阈值覆盖）。
	putResp, err := testutil.GetClient().Put("/open-api/v1/intent-config", map[string]interface{}{
		"min_confidence": 0.65,
		"questions": []interface{}{
			map[string]interface{}{
				"name":         testutil.UniqueName("inner-schema-ic-choice"),
				"type":         "choice",
				"instructions": "这条请求属于哪类研发任务？",
				"criteria": map[string]interface{}{
					"coding":       "编写或修改代码、调试、重构、代码审查",
					"test_writing": "编写测试用例、单元测试、集成测试、补充断言",
				},
			},
			map[string]interface{}{
				"name":           testutil.UniqueName("inner-schema-ic-score"),
				"type":           "score",
				"instructions":   "这个任务的复杂度如何？",
				"min_confidence": 0.7,
				"levels": []interface{}{
					map[string]interface{}{"name": "simple", "description": "单步即可完成"},
					map[string]interface{}{"name": "complex", "description": "需要深入推理或跨模块设计"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, putResp.ErrNum, putResp.ErrMsg)

	// 发布后首拉：Data 非 null，符合 PascalCase 文件合同。
	resp, err := testutil.GetClient().Get(exportPath)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	if resp.Data == nil || string(resp.Data) == "null" {
		t.Fatal("first pull after publish must return data")
	}
	testutil.AssertSchema(t, resp, IntentConfigExportSchema)

	// 定向断言 1：Version 时间戳格式；无 Config 包装层（顶层键精确 3 字段）。
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &payload))
	version, ok := payload["Version"].(string)
	require.True(t, ok, "Version should be string")
	require.True(t, intentConfigVersionPattern.MatchString(version),
		"Version must be a yyyyMMddHHmmss timestamp, got %q", version)
	assert.ElementsMatch(t, []string{"Version", "MinConfidence", "Questions"}, keysOfInnerMap(payload),
		"export must be the file content verbatim (Version embedded, no Config wrapper)")
	assert.InDelta(t, 0.65, payload["MinConfidence"], 1e-9)

	// 定向断言 2：choice/score 互斥键集合精确（PascalCase，防幻影键）。
	questions, ok := payload["Questions"].([]interface{})
	require.True(t, ok, "Questions should be array")
	require.Len(t, questions, 2)

	choice, ok := questions[0].(map[string]interface{})
	require.True(t, ok, "Questions[0] should be object")
	assert.ElementsMatch(t,
		[]string{"Name", "Type", "Instructions", "Criteria"}, keysOfInnerMap(choice),
		"choice question must carry exactly the PascalCase contract keys (no Levels)")
	criteria, ok := choice["Criteria"].(map[string]interface{})
	require.True(t, ok, "Criteria should be object")
	assert.Contains(t, criteria, "coding")

	score, ok := questions[1].(map[string]interface{})
	require.True(t, ok, "Questions[1] should be object")
	assert.ElementsMatch(t,
		[]string{"Name", "Type", "Instructions", "MinConfidence", "Levels"}, keysOfInnerMap(score),
		"score question must carry exactly the PascalCase contract keys (no Criteria)")
	assert.InDelta(t, 0.7, score["MinConfidence"], 1e-9)
	levels, ok := score["Levels"].([]interface{})
	require.True(t, ok, "Levels should be array")
	require.Len(t, levels, 2)
	for i, item := range levels {
		level, ok := item.(map[string]interface{})
		require.True(t, ok, "Levels[%d] should be object", i)
		assert.ElementsMatch(t, []string{"Name", "Description"}, keysOfInnerMap(level),
			"Levels[%d] must not leak internal fields", i)
	}

	// 定向断言 3：原始 body 无 Config 包装层、无小写词汇泄漏。
	body := string(resp.RawBody)
	assert.NotContains(t, body, `"Config"`)
	assert.NotContains(t, body, `"questions"`)
	assert.NotContains(t, body, `"min_confidence"`)

	// version 增量参数：传当前 version → Data:null（conf-agent 不落盘、不 reload）。
	unchangedResp, err := testutil.GetClient().Get(exportPath, map[string]string{"version": version})
	require.NoError(t, err)
	testutil.AssertSuccess(t, unchangedResp)
	testutil.AssertDataNull(t, unchangedResp)
}

func keysOfInnerMap(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ---------- epp_data ----------

// patchEppPoolForSchema 为 EPP schema 用例配置独立的实例池布局（EPP 池为单例）。
func patchEppPoolForSchema(t *testing.T) {
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
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
}

// createEPPClusterForSchema 创建一个 EPP 模式集群（自带 provider），返回名称。
func createEPPClusterForSchema(t *testing.T, eppConfig map[string]interface{}) string {
	t.Helper()
	providerName := testutil.UniqueProviderName()
	_, err := testutil.CreateProvider(providerName)
	require.NoError(t, err)

	clusterName := testutil.UniqueClusterName()
	resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name":         clusterName,
		"balance_mode": "EPP",
		"epp_config":   eppConfig,
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": providerName,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
	return clusterName
}

func testEppDataSchema(t *testing.T) {
	patchEppPoolForSchema(t)

	clusterName := createEPPClusterForSchema(t, map[string]interface{}{
		"load_profile": "kv-first",
		"flow_control": map[string]interface{}{
			"max_requests": 100,
			"queue_ttl":    30,
		},
	})
	t.Cleanup(func() { testutil.DeleteCluster(clusterName) })

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/epp_data/config")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	testutil.AssertSchema(t, resp, EppDataSchema)

	// 定向断言（epp-data.md §3.2/§3.3）：
	// assignment 含该 cluster 条目且 primary 非空；epp_config 编译产物关键字段存在。
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &payload))
	config := payload["Config"].(map[string]interface{})

	assignment := config["assignment"].(map[string]interface{})
	entry, ok := assignment[clusterName].(map[string]interface{})
	require.True(t, ok, "assignment should contain cluster %s", clusterName)
	primary, ok := entry["primary"].(string)
	require.True(t, ok, "assignment.primary should be a string")
	require.NotEmpty(t, primary)

	eppConfig := config["epp_config"].(map[string]interface{})
	compiled, ok := eppConfig[clusterName].(map[string]interface{})
	require.True(t, ok, "epp_config should contain cluster %s", clusterName)
	plugins, ok := compiled["plugins"].([]interface{})
	require.True(t, ok, "compiled epp_config.plugins should be an array")
	require.NotEmpty(t, plugins)
	require.Contains(t, compiled, "featureGates")
	require.Contains(t, compiled, "schedulingProfiles")
	require.Contains(t, compiled, "dataLayer")

	pluginNames := map[string]bool{}
	for _, p := range plugins {
		pluginNames[p.(map[string]interface{})["name"].(string)] = true
	}
	require.True(t, pluginNames["saturation-detector"], "compiled plugins should include saturation-detector")
	flowControl, ok := compiled["flowControl"].(map[string]interface{})
	require.True(t, ok, "compiled epp_config.flowControl should be present")
	saturationDetector, ok := flowControl["saturationDetector"].(map[string]interface{})
	require.True(t, ok, "flowControl.saturationDetector should be present")
	require.Equal(t, "saturation-detector", saturationDetector["pluginRef"])
}

// ---------- server_data_conf EPP 导出 ----------

func testServerDataConfEppSchema(t *testing.T) {
	patchEppPoolForSchema(t)

	eppClusterName := createEPPClusterForSchema(t, map[string]interface{}{
		"load_profile": "balanced",
	})
	wrrClusterName := testutil.UniqueClusterName()
	{
		providerName := testutil.UniqueProviderName()
		_, err := testutil.CreateProvider(providerName)
		require.NoError(t, err)
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": wrrClusterName,
			"llm_config": map[string]interface{}{
				"models":   []string{"deepseek-chat"},
				"provider": providerName,
			},
		})
		require.NoError(t, err)
		require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })
	}
	t.Cleanup(func() {
		testutil.DeleteCluster(eppClusterName)
		testutil.DeleteCluster(wrrClusterName)
	})

	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	testutil.AssertSchema(t, resp, ServerDataConfSchema)

	// EPP cluster：BalanceMode=EPP 且 EPPAddr 为单元素 [主]（server-data-conf.md §3.3；
	// 自 2026-10-14-epp-openapi-single-instance 起实例池为单实例组，导出恒为 [主]）。
	assignResp, err := testutil.GetClient().Get("/open-api/v1/epp-assignments", map[string]string{
		"cluster": eppClusterName,
	})
	require.NoError(t, err)
	require.Equal(t, 200, assignResp.ErrNum, assignResp.ErrMsg)
	var assignView map[string]interface{}
	require.NoError(t, json.Unmarshal(assignResp.Data, &assignView))
	entries := assignView["clusters"].([]interface{})
	entry := entries[0].(map[string]interface{})
	primary := entry["primary"].(map[string]interface{})
	expectedAddrs := []interface{}{
		fmt.Sprintf("%s:%v", primary["host"], int(primary["port"].(float64))),
	}
	assertServerDataConfEpp(t, resp.Data, eppClusterName, "EPP", expectedAddrs)

	// WRR cluster：BalanceMode=WRR 且无 EPPAddr。
	assertServerDataConfEpp(t, resp.Data, wrrClusterName, "WRR", nil)
}

// assertServerDataConfEpp 校验导出结果中指定 cluster 的 GslbBasic.BalanceMode / EPPAddr
// （server-data-conf.md §3.3：单实例组下 EPPAddr 恒为单元素 [主]，WRR 时为 null）。
func assertServerDataConfEpp(t *testing.T, data []byte, clusterName, wantMode string, wantEPPAddr []interface{}) {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal server_data_conf data: %v", err)
	}
	clusterConf := payload["ClusterConf"].(map[string]interface{})
	config := clusterConf["Config"].(map[string]interface{})
	cluster, ok := config[clusterName].(map[string]interface{})
	if !ok {
		t.Fatalf("cluster %s not found in ClusterConf.Config", clusterName)
	}
	gslbBasic, ok := cluster["GslbBasic"].(map[string]interface{})
	if !ok {
		t.Fatalf("GslbBasic not found for cluster %s", clusterName)
	}
	assert.Equal(t, wantMode, gslbBasic["BalanceMode"])
	if wantEPPAddr == nil {
		assert.Nil(t, gslbBasic["EPPAddr"], "WRR cluster should not export EPPAddr")
		return
	}
	eppAddr, ok := gslbBasic["EPPAddr"].([]interface{})
	if !assert.True(t, ok, "EPPAddr should be an array for cluster %s", clusterName) {
		return
	}
	assert.Equal(t, wantEPPAddr, eppAddr, "EPPAddr should be a single-element [primary] list")
}
