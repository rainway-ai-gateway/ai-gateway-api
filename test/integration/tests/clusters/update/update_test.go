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
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
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

func TestClusters_Update(t *testing.T) {
	clusterName := testutil.UniqueClusterName()
	if _, err := testutil.CreateCluster(clusterName); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	providerUpdate := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(providerUpdate, map[string]interface{}{
		"models": []string{"qwen-turbo"},
	}); err != nil {
		t.Fatalf("setup providerUpdate failed: %v", err)
	}

	providerKeys := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(providerKeys, map[string]interface{}{
		"keys": []interface{}{
			map[string]interface{}{"name": "new-primary", "key": "sk-new-primary"},
			map[string]interface{}{"name": "new-secondary", "key": "sk-new-secondary"},
		},
	}); err != nil {
		t.Fatalf("setup providerKeys failed: %v", err)
	}

	providerPrefix := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(providerPrefix, map[string]interface{}{
		"models": []string{"openrouter/anthropic/claude-sonnet-4", "deepseek-chat"},
	}); err != nil {
		t.Fatalf("setup providerPrefix failed: %v", err)
	}

	t.Run("CL-4-001 更新 description", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": "更新后的集群描述",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "name", clusterName)
	})

	t.Run("CL-4-002 更新 llm_config", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models":   []string{"qwen-turbo"},
				"provider": providerUpdate,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "name", clusterName)
	})

	t.Run("CL-4-003 更新后查询一致性", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", "更新后的集群描述")
	})

	t.Run("CL-4-004 更新不存在的集群", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/non_existent_cluster", map[string]interface{}{
			"description": "x",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("CL-4-024 更新 description 恰好 256 字节（合法边界）", func(t *testing.T) {
		// clusters.md §1/§2.1：0-256 字节，256 是合法上界
		desc := strings.Repeat("b", 256)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": desc,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", desc)

		got, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "description", desc)
	})

	t.Run("CL-4-025 更新 description 257 字节被拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": strings.Repeat("c", 257),
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("CL-4-026 更新 description 含控制字符被拒绝且旧值保留", func(t *testing.T) {
		baseline := "控制字符基线"
		if _, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": baseline,
		}); err != nil {
			t.Fatalf("request failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": "abc\x01def",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		got, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "description", baseline)
	})

	t.Run("CL-4-027 非法 description 不污染存量值（原子性）", func(t *testing.T) {
		baseline := "原子性基线"
		if _, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": baseline,
		}); err != nil {
			t.Fatalf("request failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": strings.Repeat("d", 257),
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		got, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "description", baseline)
	})

	t.Run("CL-4-006 更新 keys（全量替换）", func(t *testing.T) {
		clusterUpdateKeys := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterUpdateKeys,
			"llm_config": map[string]interface{}{
				"models":   []string{"deepseek-chat"},
				"provider": providerKeys,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterUpdateKeys)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterUpdateKeys, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models": []string{"deepseek-chat"},
				"keys": []interface{}{
					map[string]interface{}{
						"name":   "new-primary",
						"weight": 60,
					},
					map[string]interface{}{
						"name":   "new-secondary",
						"weight": 40,
					},
				},
				"key_policy": map[string]interface{}{
					"strategy":    "weighted_random",
					"max_retries": 5,
				},
				"provider": providerKeys,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterUpdateKeys)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "name", clusterUpdateKeys)
	})

	t.Run("CL-4-007 更新 key_policy", func(t *testing.T) {
		clusterUpdatePolicy := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterUpdatePolicy,
			"llm_config": map[string]interface{}{
				"models": []string{"deepseek-chat"},
				"keys": []interface{}{
					map[string]interface{}{"name": "new-primary", "weight": 60},
					map[string]interface{}{"name": "new-secondary", "weight": 40},
				},
				"provider": providerKeys,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterUpdatePolicy)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterUpdatePolicy, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models": []string{"deepseek-chat"},
				"key_policy": map[string]interface{}{
					"strategy":              "weighted_random",
					"retry_backoff_initial": 200,
					"retry_backoff_max":     2000,
				},
				"provider": providerKeys,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterUpdatePolicy)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "name", clusterUpdatePolicy)
	})

	t.Run("CL-4-008 更新 match_prefix / strip_prefix", func(t *testing.T) {
		clusterUpdatePrefix := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterUpdatePrefix,
			"llm_config": map[string]interface{}{
				"models":       []string{"openrouter/anthropic/claude-sonnet-4"},
				"match_prefix": "openrouter/",
				"strip_prefix": true,
				"provider":     providerPrefix,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterUpdatePrefix)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterUpdatePrefix, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models":       []string{"deepseek-chat"},
				"match_prefix": "deepseek/",
				"strip_prefix": false,
				"provider":     providerPrefix,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterUpdatePrefix)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		llm, _ := data["llm_config"].(map[string]interface{})
		assert.Equal(t, "deepseek/", llm["match_prefix"])
		assert.Equal(t, false, llm["strip_prefix"])

		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
		if err != nil {
			t.Fatalf("inner api request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var innerData map[string]interface{}
		if err := json.Unmarshal(resp.Data, &innerData); err != nil {
			t.Fatalf("unmarshal inner data failed: %v", err)
		}
		clusterConf, _ := innerData["ClusterConf"].(map[string]interface{})
		config, _ := clusterConf["Config"].(map[string]interface{})
		cluster, _ := config[clusterUpdatePrefix].(map[string]interface{})
		aiconf, _ := cluster["AIConf"].(map[string]interface{})
		assert.Equal(t, "deepseek/", aiconf["MatchPrefix"])
		assert.Equal(t, false, aiconf["StripPrefix"])
	})

	t.Run("CL-4-011 更新 key_affinity", func(t *testing.T) {
		clusterUpdateAffinity := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterUpdateAffinity,
			"llm_config": map[string]interface{}{
				"models": []string{"deepseek-chat"},
				"key_affinity": map[string]interface{}{
					"enabled":        false,
					"ttl":            600,
					"redis_prefix":   "bfe:ai:key_affinity",
					"penalty_enable": true,
				},
				"provider": providerPrefix,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterUpdateAffinity)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterUpdateAffinity, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models": []string{"deepseek-chat"},
				"key_affinity": map[string]interface{}{
					"enabled":        true,
					"ttl":            1200,
					"redis_prefix":   "bfe:ai:key_affinity:v2",
					"penalty_enable": false,
				},
				"provider": providerPrefix,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterUpdateAffinity)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		llm, _ := data["llm_config"].(map[string]interface{})
		affinity, _ := llm["key_affinity"].(map[string]interface{})
		assert.Equal(t, true, affinity["enabled"])
		assert.Equal(t, float64(1200), affinity["ttl"])
		assert.Equal(t, "bfe:ai:key_affinity:v2", affinity["redis_prefix"])
		assert.Equal(t, false, affinity["penalty_enable"])

		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
		if err != nil {
			t.Fatalf("inner api request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var innerData map[string]interface{}
		if err := json.Unmarshal(resp.Data, &innerData); err != nil {
			t.Fatalf("unmarshal inner data failed: %v", err)
		}
		clusterConf, _ := innerData["ClusterConf"].(map[string]interface{})
		config, _ := clusterConf["Config"].(map[string]interface{})
		cluster, _ := config[clusterUpdateAffinity].(map[string]interface{})
		aiconf, _ := cluster["AIConf"].(map[string]interface{})
		keyPolicy, _ := aiconf["KeyPolicy"].(map[string]interface{})
		assert.Equal(t, true, keyPolicy["SessionAffinity"])
		assert.Equal(t, float64(1200), keyPolicy["SessionAffinityTTL"])
		assert.Equal(t, "bfe:ai:key_affinity:v2", keyPolicy["SessionAffinityRedisPrefix"])
		assert.Equal(t, false, keyPolicy["SessionAffinityPenaltyEnable"])
	})

	t.Run("CL-4-012 请求体不包含 name", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"description": "请求体未传 name",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "name", clusterName)
	})

	t.Run("CL-4-013 请求体包含 name", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"name":        clusterName,
			"description": "请求体传了 name",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("CL-4-014 更新 passive_health_check 合法值", func(t *testing.T) {
		clusterPHC := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterPHC,
			"llm_config": map[string]interface{}{
				"models":   []string{"qwen-turbo"},
				"provider": providerUpdate,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterPHC)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterPHC, map[string]interface{}{
			"passive_health_check": map[string]interface{}{
				"interval":   2000,
				"failnum":    5,
				"statuscode": 200,
				"uri":        "/healthz",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterPHC)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		phc, _ := data["passive_health_check"].(map[string]interface{})
		assert.Equal(t, float64(2000), phc["interval"])
		assert.Equal(t, float64(5), phc["failnum"])
		assert.Equal(t, float64(200), phc["statuscode"])
		assert.Equal(t, "/healthz", phc["uri"])

		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
		if err != nil {
			t.Fatalf("inner api request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var innerData map[string]interface{}
		if err := json.Unmarshal(resp.Data, &innerData); err != nil {
			t.Fatalf("unmarshal inner data failed: %v", err)
		}
		clusterConf, _ := innerData["ClusterConf"].(map[string]interface{})
		config, _ := clusterConf["Config"].(map[string]interface{})
		cluster, _ := config[clusterPHC].(map[string]interface{})
		checkConf, _ := cluster["CheckConf"].(map[string]interface{})
		assert.Equal(t, "/healthz", checkConf["Uri"])
		assert.Equal(t, float64(200), checkConf["StatusCode"])
		assert.Equal(t, float64(5), checkConf["FailNum"])
		assert.Equal(t, float64(2000), checkConf["CheckInterval"])
	})

	t.Run("CL-4-015 PATCH 被动健康检查 failnum 负值拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"passive_health_check": map[string]interface{}{"failnum": -1},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "passive_health_check.failnum must be >= 0")
	})

	t.Run("CL-4-016 PATCH 被动健康检查 statuscode 超范围拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"passive_health_check": map[string]interface{}{"statuscode": 999},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "passive_health_check.statuscode must be 0 or in [100, 599]")
	})

	t.Run("CL-4-017 PATCH 被动健康检查 uri 非 / 开头拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"passive_health_check": map[string]interface{}{"uri": "healthz"},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "passive_health_check.uri must be non-empty and start with '/'")
	})

	t.Run("CL-4-018 PATCH 非法被动健康检查不污染存量配置", func(t *testing.T) {
		clusterPHCReject := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterPHCReject,
			"passive_health_check": map[string]interface{}{
				"interval":   3000,
				"failnum":    4,
				"statuscode": 200,
				"uri":        "/live",
			},
			"llm_config": map[string]interface{}{
				"models":   []string{"qwen-turbo"},
				"provider": providerUpdate,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterPHCReject)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterPHCReject, map[string]interface{}{
			"passive_health_check": map[string]interface{}{"failnum": -1, "uri": "no-slash"},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterPHCReject)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		phc, _ := data["passive_health_check"].(map[string]interface{})
		assert.Equal(t, float64(3000), phc["interval"])
		assert.Equal(t, float64(4), phc["failnum"])
		assert.Equal(t, float64(200), phc["statuscode"])
		assert.Equal(t, "/live", phc["uri"])
	})

	t.Run("CL-4-019 PATCH basic.connection.max_idle_conn_per_rs 负值拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"basic": map[string]interface{}{
				"connection": map[string]interface{}{"max_idle_conn_per_rs": -1},
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "basic.connection.max_idle_conn_per_rs must be >= 0")
	})

	t.Run("CL-4-020 PATCH basic.retries.max_retry_in_cluster 负值拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"basic": map[string]interface{}{
				"retries": map[string]interface{}{"max_retry_in_cluster": -1},
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "basic.retries.max_retry_in_cluster must be >= 0")
	})

	t.Run("CL-4-021 PATCH basic.buffers.req_write_buffer_size 零值拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"basic": map[string]interface{}{
				"buffers": map[string]interface{}{"req_write_buffer_size": 0},
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "basic.buffers.req_write_buffer_size must be > 0")
	})

	t.Run("CL-4-022 PATCH basic.timeouts.timeout_conn_serv 零值拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"basic": map[string]interface{}{
				"timeouts": map[string]interface{}{"timeout_conn_serv": 0},
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "basic.timeouts.timeout_conn_serv must be > 0")
	})

	t.Run("CL-4-023 PATCH 非法 basic 不污染存量配置", func(t *testing.T) {
		clusterBasicReject := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": clusterBasicReject,
			"basic": map[string]interface{}{
				"connection": map[string]interface{}{"max_idle_conn_per_rs": 7},
				"retries":    map[string]interface{}{"max_retry_in_cluster": 3},
				"buffers":    map[string]interface{}{"req_write_buffer_size": 1024},
				"timeouts":   map[string]interface{}{"timeout_conn_serv": 40000},
			},
			"llm_config": map[string]interface{}{
				"models":   []string{"qwen-turbo"},
				"provider": providerUpdate,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
		}
		defer testutil.DeleteCluster(clusterBasicReject)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterBasicReject, map[string]interface{}{
			"basic": map[string]interface{}{
				"connection": map[string]interface{}{"max_idle_conn_per_rs": -1},
				"buffers":    map[string]interface{}{"req_write_buffer_size": 0},
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		resp, err = testutil.GetClient().Get("/open-api/v1/clusters/" + clusterBasicReject)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		basic, _ := data["basic"].(map[string]interface{})
		conn, _ := basic["connection"].(map[string]interface{})
		assert.Equal(t, float64(7), conn["max_idle_conn_per_rs"])
		retries, _ := basic["retries"].(map[string]interface{})
		assert.Equal(t, float64(3), retries["max_retry_in_cluster"])
		buffers, _ := basic["buffers"].(map[string]interface{})
		assert.Equal(t, float64(1024), buffers["req_write_buffer_size"])
		timeouts, _ := basic["timeouts"].(map[string]interface{})
		assert.Equal(t, float64(40000), timeouts["timeout_conn_serv"])
	})

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerUpdate)
		testutil.DeleteProvider(providerKeys)
		testutil.DeleteProvider(providerPrefix)
	})
}

func TestClusters_Update_PreventDeleteReferencedModel(t *testing.T) {
	providerName := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"models": []string{"test-model", "other-model"},
	}); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}

	clusterName := testutil.UniqueClusterName()
	resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": clusterName,
		"llm_config": map[string]interface{}{
			"models":   []string{"test-model"},
			"provider": providerName,
		},
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if resp.ErrNum != 200 {
		t.Fatalf("setup create cluster failed: %d %s", resp.ErrNum, resp.ErrMsg)
	}

	// Create API-Key with route rule referencing the cluster model.
	apiKeyResp, err := testutil.GetClient().Post("/open-api/v1/api-keys", map[string]interface{}{
		"description": "model-ref-key",
		"route_rules": map[string]interface{}{
			"enabled": true,
			"rules": []interface{}{
				map[string]interface{}{
					"name": "rule-ref-model",
					"cond": "default_t()",
					"targets": []interface{}{
						map[string]interface{}{
							"cluster_name": clusterName,
							"model":        "test-model",
							"weight":       100,
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create api-key failed: %v", err)
	}
	if apiKeyResp.ErrNum != 200 {
		t.Fatalf("create api-key failed: %d %s", apiKeyResp.ErrNum, apiKeyResp.ErrMsg)
	}
	apiKeyID, _ := testutil.GetDataField(apiKeyResp, "id")

	t.Run("CL-4-009 删除被路由引用的模型应被拦截", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models":   []string{"other-model"},
				"provider": providerName,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 409)
		assert.Contains(t, resp.ErrMsg, "Rule rule-ref-model Refer To Model test-model In Cluster")
	})

	t.Run("CL-4-010 清理路由引用后可删除模型", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+apiKeyID.(string), map[string]interface{}{
			"route_rules": map[string]interface{}{
				"enabled": false,
				"rules":   []interface{}{},
			},
		})
		if err != nil {
			t.Fatalf("update api-key failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models":   []string{"other-model"},
				"provider": providerName,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
	})

	t.Cleanup(func() {
		testutil.GetClient().Delete("/open-api/v1/api-keys/" + apiKeyID.(string))
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
	})
}

// TestClusters_Update_NormalizeUpstreamError covers PATCH semantics of
// llm_config.normalize_upstream_error (2026-10-06 upstream error
// normalization): omitted-field preservation, whole-object replacement
// (same pattern as CL-4-011 key_affinity), explicit-null equivalence,
// PUT/PATCH symmetry on invalid values, and read-back-zero-change after
// 4xx. Design: tests/integration/tests/clusters/design.md §12.
func TestClusters_Update_NormalizeUpstreamError(t *testing.T) {
	provider := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(provider, map[string]interface{}{
		"models": []string{"deepseek-chat"},
	}); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}
	defer testutil.DeleteProvider(provider)

	fullNue := func() map[string]interface{} {
		return map[string]interface{}{
			"enabled":             true,
			"stream_enabled":      true,
			"unrecognized_action": "rewrite_generic",
			"max_body_bytes":      65536,
			"redact_secrets":      false,
		}
	}
	llmWithNue := func(nue map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"models":                   []string{"deepseek-chat"},
			"provider":                 provider,
			"normalize_upstream_error": nue,
		}
	}
	newClusterWithNue := func(t *testing.T) string {
		t.Helper()
		name := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name":       name,
			"llm_config": llmWithNue(fullNue()),
		})
		if err != nil {
			t.Fatalf("setup create cluster failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster: expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		return name
	}
	readBackNue := func(t *testing.T, cluster string) map[string]interface{} {
		t.Helper()
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + cluster)
		if err != nil {
			t.Fatalf("get cluster failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		llm, _ := data["llm_config"].(map[string]interface{})
		nue, _ := llm["normalize_upstream_error"].(map[string]interface{})
		return nue
	}
	assertFullNueUnchanged := func(t *testing.T, cluster string) {
		t.Helper()
		nue := readBackNue(t, cluster)
		if assert.NotNil(t, nue, "nue must be preserved") {
			assert.Equal(t, true, nue["enabled"])
			assert.Equal(t, true, nue["stream_enabled"])
			assert.Equal(t, "rewrite_generic", nue["unrecognized_action"])
			assert.Equal(t, float64(65536), nue["max_body_bytes"])
			assert.Equal(t, false, nue["redact_secrets"])
		}
	}

	t.Run("CL-4-101 PATCH 省略字段保留原值", func(t *testing.T) {
		name := newClusterWithNue(t)
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"description": "patch unrelated field only",
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		assertFullNueUnchanged(t, name)
	})

	t.Run("CL-4-102 PATCH 整体替换 nue 对象", func(t *testing.T) {
		name := newClusterWithNue(t)
		defer testutil.DeleteCluster(name)
		replaced := map[string]interface{}{
			"enabled":             false,
			"stream_enabled":      false,
			"unrecognized_action": "passthrough",
			"max_body_bytes":      131072,
			"redact_secrets":      true,
		}
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWithNue(replaced),
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		nue := readBackNue(t, name)
		if assert.NotNil(t, nue) {
			assert.Equal(t, false, nue["enabled"])
			assert.Equal(t, false, nue["stream_enabled"])
			assert.Equal(t, "passthrough", nue["unrecognized_action"])
			assert.Equal(t, float64(131072), nue["max_body_bytes"])
			assert.Equal(t, true, nue["redact_secrets"])
		}
	})

	t.Run("CL-4-103 PATCH llm_config 省略 nue = 清空（全量替换语义）", func(t *testing.T) {
		name := newClusterWithNue(t)
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models":   []string{"deepseek-chat"},
				"provider": provider,
			},
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		// llm_config is wholesale-replaced on PATCH (same semantics as
		// llm_config.keys per the clusters contract): an omitted
		// normalize_upstream_error is cleared, not preserved.
		nue := readBackNue(t, name)
		assert.Nil(t, nue, "nue must be cleared when llm_config is patched without it")
	})

	t.Run("CL-4-104 PATCH 非法值拒绝 + 回读零变更", func(t *testing.T) {
		name := newClusterWithNue(t)
		defer testutil.DeleteCluster(name)

		badNue := fullNue()
		badNue["unrecognized_action"] = "replace"
		patchResp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWithNue(badNue),
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}

		if patchResp.ErrNum != 422 {
			t.Fatalf("expected 422, got %d, ErrMsg=%s", patchResp.ErrNum, patchResp.ErrMsg)
		}
		assert.Contains(t, patchResp.ErrMsg, "unrecognized_action")

		// read-back zero change after the rejection
		assertFullNueUnchanged(t, name)
	})
}

// TestClusters_Update_PriceMissAction covers PATCH semantics of
// llm_config.price_miss_action (issue #1479): omitted-field preservation,
// value change, invalid-value rejection with read-back zero-change, and
// PUT/PATCH symmetry. Design: tests/integration/tests/clusters/design.md §13.
func TestClusters_Update_PriceMissAction(t *testing.T) {
	provider := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(provider, map[string]interface{}{
		"models": []string{"deepseek-chat"},
	}); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}
	defer testutil.DeleteProvider(provider)

	llmWithAction := func(action string) map[string]interface{} {
		return map[string]interface{}{
			"models":            []string{"deepseek-chat"},
			"provider":          provider,
			"price_miss_action": action,
		}
	}
	newClusterWithAction := func(t *testing.T) string {
		t.Helper()
		name := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name":       name,
			"llm_config": llmWithAction("reject"),
		})
		if err != nil {
			t.Fatalf("setup create cluster failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("setup create cluster: expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		return name
	}
	readBackAction := func(t *testing.T, cluster string) interface{} {
		t.Helper()
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + cluster)
		if err != nil {
			t.Fatalf("get cluster failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		llm, _ := data["llm_config"].(map[string]interface{})
		return llm["price_miss_action"]
	}

	t.Run("CL-4-121 PATCH 省略字段保留原值", func(t *testing.T) {
		name := newClusterWithAction(t)
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"description": "patch unrelated field only",
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		if got := readBackAction(t, name); got != "reject" {
			t.Fatalf("price_miss_action = %v, want reject preserved", got)
		}
	})

	t.Run("CL-4-122 PATCH 修改为 bill_zero", func(t *testing.T) {
		name := newClusterWithAction(t)
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWithAction("bill_zero"),
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		if got := readBackAction(t, name); got != "bill_zero" {
			t.Fatalf("price_miss_action = %v, want bill_zero", got)
		}
	})

	t.Run("CL-4-123 非法值 4xx 且回读零变更", func(t *testing.T) {
		name := newClusterWithAction(t)
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWithAction("bogus"),
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		if resp.ErrNum == 200 {
			t.Fatalf("expected a 4xx for an invalid price_miss_action, got 200")
		}
		if got := readBackAction(t, name); got != "reject" {
			t.Fatalf("price_miss_action = %v, want reject unchanged after 4xx", got)
		}
	})
}

// TestClusters_Update_EnsureReasoningContent 覆盖 llm_config.ensure_reasoning_content
// 的 PATCH 语义。每个用例都按集成测试检查清单执行：
//   - 省略字段保留原值（且断言其余未提交字段同样不变）
//   - 显式空数组清空（与省略形成对照）
//   - 4xx 后回读零变更
//   - 反向引用完整性：provider 收窄协议后既有 cluster 配置的完整性
func TestClusters_Update_EnsureReasoningContent(t *testing.T) {
	provider := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(provider, map[string]interface{}{
		"models":          []string{"deepseek-chat"},
		"model_protocols": []string{"openai", "anthropic"},
	}); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}
	defer testutil.DeleteProvider(provider)

	llmWith := func(ensure []string, maxRetries int) map[string]interface{} {
		llm := map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": provider,
			"key_policy": map[string]interface{}{
				"strategy":    "weighted_random",
				"max_retries": maxRetries,
			},
		}
		if ensure != nil {
			llm["ensure_reasoning_content"] = ensure
		}
		return llm
	}

	newCluster := func(t *testing.T, ensure []string) string {
		t.Helper()
		name := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
			"name": name, "llm_config": llmWith(ensure, 3),
		})
		if err != nil || resp.ErrNum != 200 {
			t.Fatalf("setup create cluster failed: err=%v ErrNum=%d ErrMsg=%s", err, resp.ErrNum, resp.ErrMsg)
		}
		return name
	}

	readLLM := func(t *testing.T, cluster string) map[string]interface{} {
		t.Helper()
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + cluster)
		if err != nil {
			t.Fatalf("get cluster failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		llm, _ := data["llm_config"].(map[string]interface{})
		return llm
	}

	readEnsure := func(t *testing.T, cluster string) []interface{} {
		t.Helper()
		v, _ := readLLM(t, cluster)["ensure_reasoning_content"].([]interface{})
		return v
	}

	exported := func(t *testing.T, cluster string) []interface{} {
		t.Helper()
		resp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
		if err != nil {
			t.Fatalf("export request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		clusterConf, _ := data["ClusterConf"].(map[string]interface{})
		config, _ := clusterConf["Config"].(map[string]interface{})
		entry, _ := config[cluster].(map[string]interface{})
		aiConf, _ := entry["AIConf"].(map[string]interface{})
		v, _ := aiConf["EnsureReasoningContent"].([]interface{})
		return v
	}

	t.Run("CL-4-201 省略字段保留原值且其余 llm_config 不变", func(t *testing.T) {
		name := newCluster(t, []string{"openai"})
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"description": "patch unrelated field only",
		})
		if err != nil || resp.ErrNum != 200 {
			t.Fatalf("patch failed: err=%v ErrNum=%d ErrMsg=%s", err, resp.ErrNum, resp.ErrMsg)
		}
		assert.Equal(t, []interface{}{"openai"}, readEnsure(t, name))
		// 未提交字段同样不得被重置：key_policy.max_retries 仍为 3
		assert.Equal(t, true, readLLM(t, name)["provider"] == provider)
		kp, _ := readLLM(t, name)["key_policy"].(map[string]interface{})
		assert.Equal(t, float64(3), kp["max_retries"])
	})

	t.Run("CL-4-202 显式设置后回读与导出均生效", func(t *testing.T) {
		name := newCluster(t, nil)
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWith([]string{"openai", "anthropic"}, 3),
		})
		if err != nil || resp.ErrNum != 200 {
			t.Fatalf("patch failed: err=%v ErrNum=%d ErrMsg=%s", err, resp.ErrNum, resp.ErrMsg)
		}
		assert.Equal(t, []interface{}{"openai", "anthropic"}, readEnsure(t, name))
		assert.Equal(t, []interface{}{"openai", "anthropic"}, exported(t, name))
	})

	t.Run("CL-4-203 显式空数组清空（与省略对照）", func(t *testing.T) {
		name := newCluster(t, []string{"openai"})
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWith([]string{}, 3),
		})
		if err != nil || resp.ErrNum != 200 {
			t.Fatalf("patch failed: err=%v ErrNum=%d ErrMsg=%s", err, resp.ErrNum, resp.ErrMsg)
		}
		if v := readEnsure(t, name); len(v) != 0 {
			t.Fatalf("ensure_reasoning_content = %v, want empty after clearing", v)
		}
		if v := exported(t, name); len(v) != 0 {
			t.Fatalf("exported EnsureReasoningContent = %v, want null/empty after clearing", v)
		}
	})

	t.Run("CL-4-204 非法值 4xx 后回读零变更", func(t *testing.T) {
		name := newCluster(t, []string{"openai"})
		defer testutil.DeleteCluster(name)
		before := readLLM(t, name)
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+name, map[string]interface{}{
			"llm_config": llmWith([]string{"gemini"}, 9),
		})
		if err != nil {
			t.Fatalf("patch failed: %v", err)
		}
		if resp.ErrNum == 200 {
			t.Fatalf("expected a 4xx for gemini, got 200")
		}
		assert.Contains(t, resp.ErrMsg, "ensure_reasoning_content")
		assert.Contains(t, resp.ErrMsg, "unsupported protocol")
		// 回读零变更：本字段与同 body 里其它被尝试修改的字段都必须保持原值
		after := readLLM(t, name)
		assert.Equal(t, before["ensure_reasoning_content"], after["ensure_reasoning_content"])
		beforeKP, _ := before["key_policy"].(map[string]interface{})
		afterKP, _ := after["key_policy"].(map[string]interface{})
		assert.Equal(t, beforeKP["max_retries"], afterKP["max_retries"])
	})

	// CL-4-205 / CL-4-206：反向引用完整性。provider 收窄 model_protocols 时，
	// 若某个引用它的 cluster 的 ensure_reasoning_content 含被移除的协议，必须
	// 409 拒绝（合同见 providers.md 的 PATCH 行为说明 + 本次 guard 变更）。
	patchProvider := func(t *testing.T, name string, protocols []string) *testutil.APIResponse {
		t.Helper()
		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+name, map[string]interface{}{
			"instance_pool":   []interface{}{map[string]interface{}{"addr": "10.0.0.1", "weight": 100, "port": 8080}},
			"models":          []string{"deepseek-chat"},
			"model_protocols": protocols,
		})
		if err != nil {
			t.Fatalf("patch provider failed: %v", err)
		}
		return resp
	}
	readProviderProtocols := func(t *testing.T, name string) []interface{} {
		t.Helper()
		resp, err := testutil.GetClient().Get("/open-api/v1/providers/" + name)
		if err != nil {
			t.Fatalf("get provider failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		v, _ := data["model_protocols"].([]interface{})
		return v
	}

	t.Run("CL-4-205 收窄被引用协议：409 且 provider 与 cluster 均零变更", func(t *testing.T) {
		name := newCluster(t, []string{"anthropic"})
		defer testutil.DeleteCluster(name)

		resp := patchProvider(t, provider, []string{"openai"})
		if resp.ErrNum != 409 {
			t.Fatalf("expected 409 for a narrowing that invalidates cluster %s, got %d %s",
				name, resp.ErrNum, resp.ErrMsg)
		}
		// 错误可归因：provider、协议、引用它的 cluster 都要出现
		assert.Contains(t, resp.ErrMsg, provider)
		assert.Contains(t, resp.ErrMsg, "anthropic")
		assert.Contains(t, resp.ErrMsg, name)

		// 回读零变更：provider 仍保留双协议
		assert.Equal(t, []interface{}{"openai", "anthropic"}, readProviderProtocols(t, provider))
		// cluster 配置不变
		assert.Equal(t, []interface{}{"anthropic"}, readEnsure(t, name))
		// 导出仍然合法（ModelProtocols 仍含 anthropic）
		exp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
		if err != nil {
			t.Fatalf("export failed: %v", err)
		}
		var edata map[string]interface{}
		json.Unmarshal(exp.Data, &edata)
		cc, _ := edata["ClusterConf"].(map[string]interface{})
		cfg, _ := cc["Config"].(map[string]interface{})
		entry, _ := cfg[name].(map[string]interface{})
		aiConf, _ := entry["AIConf"].(map[string]interface{})
		protocols, _ := aiConf["ModelProtocols"].([]interface{})
		assert.Contains(t, protocols, "anthropic")
	})

	t.Run("CL-4-206 收窄但无引用：允许", func(t *testing.T) {
		// 该 cluster 只用 openai，收窄到 ["openai"] 不影响它
		name := newCluster(t, []string{"openai"})
		defer testutil.DeleteCluster(name)

		resp := patchProvider(t, provider, []string{"openai"})
		if resp.ErrNum != 200 {
			t.Fatalf("expected 200 for an unreferenced narrowing, got %d %s", resp.ErrNum, resp.ErrMsg)
		}
		assert.Equal(t, []interface{}{"openai"}, readProviderProtocols(t, provider))
		// 清理：恢复双协议，避免影响同包其它用例（TestMain 共享一个服务）
		restore := patchProvider(t, provider, []string{"openai", "anthropic"})
		if restore.ErrNum != 200 {
			t.Fatalf("restore provider failed: %d %s", restore.ErrNum, restore.ErrMsg)
		}
	})

}

// TestClusters_Update_KeyPenaltyTTL covers the update path of issue #1448:
// explicit values are stored and exported to the InnerAPI, out-of-range values
// are rejected without changing the stored config, and the key_affinity object
// is replaced wholesale (an omitted penalty field falls back to its default).
func TestClusters_Update_KeyPenaltyTTL(t *testing.T) {
	provider := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(provider, map[string]interface{}{
		"models": []string{"deepseek-chat"},
	}); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}

	clusterName := testutil.UniqueClusterName()
	resp, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": clusterName,
		"llm_config": map[string]interface{}{
			"models": []string{"deepseek-chat"},
			"key_affinity": map[string]interface{}{
				"enabled": true,
			},
			"provider": provider,
		},
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	defer testutil.DeleteCluster(clusterName)

	patchBody := func(affinity map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{
			"llm_config": map[string]interface{}{
				"models":       []string{"deepseek-chat"},
				"key_affinity": affinity,
				"provider":     provider,
			},
		}
	}

	t.Run("更新后回读与 InnerAPI 导出一致", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, patchBody(map[string]interface{}{
			"enabled":          true,
			"penalty_ttl_429":  30,
			"penalty_ttl_auth": 120,
		}))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		assertUpdatedClusterPenaltyTTL(t, clusterName, 30, 120)

		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
		if err != nil {
			t.Fatalf("inner api request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var innerData map[string]interface{}
		if err := json.Unmarshal(resp.Data, &innerData); err != nil {
			t.Fatalf("unmarshal inner data failed: %v", err)
		}
		clusterConf, _ := innerData["ClusterConf"].(map[string]interface{})
		config, _ := clusterConf["Config"].(map[string]interface{})
		cluster, _ := config[clusterName].(map[string]interface{})
		aiconf, _ := cluster["AIConf"].(map[string]interface{})
		keyPolicy, _ := aiconf["KeyPolicy"].(map[string]interface{})
		assert.Equal(t, float64(30), keyPolicy["SessionAffinityPenaltyTTL429"])
		assert.Equal(t, float64(120), keyPolicy["SessionAffinityPenaltyTTLAuth"])
	})

	t.Run("非法值 422 且回读零变更", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, patchBody(map[string]interface{}{
			"enabled":          true,
			"penalty_ttl_429":  30,
			"penalty_ttl_auth": 1801,
		}))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		assert.Equal(t, 422, resp.ErrNum, "penalty_ttl_auth=1801 should be rejected")

		assertUpdatedClusterPenaltyTTL(t, clusterName, 30, 120)
	})

	t.Run("key_affinity 整体替换：省略的 429 档回落默认 60", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, patchBody(map[string]interface{}{
			"enabled":          true,
			"penalty_ttl_auth": 120,
		}))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		assertUpdatedClusterPenaltyTTL(t, clusterName, 60, 120)
	})
}

// assertUpdatedClusterPenaltyTTL reads the cluster back and checks both penalty
// TTLs against the stored llm_config.
func assertUpdatedClusterPenaltyTTL(t *testing.T, name string, want429, wantAuth int) {
	t.Helper()

	resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + name)
	if err != nil {
		t.Fatalf("read back failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	llm, _ := data["llm_config"].(map[string]interface{})
	affinity, _ := llm["key_affinity"].(map[string]interface{})
	assert.Equal(t, float64(want429), affinity["penalty_ttl_429"])
	assert.Equal(t, float64(wantAuth), affinity["penalty_ttl_auth"])
}
