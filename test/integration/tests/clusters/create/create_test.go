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

func minClusterBody(name, provider string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": provider,
		},
	}
}

// clusterBodyWithPHC builds a minimal cluster body carrying the given
// passive_health_check payload (issue #172 validation cases).
func clusterBodyWithPHC(name, provider string, phc map[string]interface{}) map[string]interface{} {
	body := minClusterBody(name, provider)
	body["passive_health_check"] = phc
	return body
}

// clusterBodyWithBasic builds a minimal cluster body carrying the given
// basic payload (issue #173 validation cases).
func clusterBodyWithBasic(name, provider string, basic map[string]interface{}) map[string]interface{} {
	body := minClusterBody(name, provider)
	body["basic"] = basic
	return body
}

func assertNoInternalFields(t *testing.T, data map[string]interface{}) {
	assert.NotContains(t, data, "ready")
	assert.NotContains(t, data, "sub_clusters")
	assert.NotContains(t, data, "scheduler")
	assert.NotContains(t, data, "instance_pool")
}

func TestClusters_Create(t *testing.T) {
	clusterMin := testutil.UniqueClusterName()
	clusterFull := testutil.UniqueClusterName()
	clusterDup := testutil.UniqueClusterName()
	clusterDescBoundary := testutil.UniqueClusterName()

	providerFull := testutil.UniqueProviderName()
	providerKeys := testutil.UniqueProviderName()
	providerPrefix := testutil.UniqueProviderName()
	providerAffinity := testutil.UniqueProviderName()
	providerNotExist := testutil.UniqueProviderName()

	if _, err := testutil.CreateProvider(providerFull, map[string]interface{}{
		"models": []string{"deepseek-chat", "deepseek-coder"},
		"keys": []interface{}{
			map[string]interface{}{"name": "primary", "key": "sk-aaaaaaaaaaaa"},
			map[string]interface{}{"name": "secondary", "key": "sk-bbbbbbbbbbbb"},
		},
	}); err != nil {
		t.Fatalf("setup providerFull failed: %v", err)
	}
	if _, err := testutil.CreateProvider(providerKeys, map[string]interface{}{
		"keys": []interface{}{
			map[string]interface{}{"name": "primary", "key": "sk-aaaaaaaaaaaa"},
			map[string]interface{}{"name": "secondary", "key": "sk-bbbbbbbbbbbb"},
		},
	}); err != nil {
		t.Fatalf("setup providerKeys failed: %v", err)
	}
	if _, err := testutil.CreateProvider(providerPrefix, map[string]interface{}{
		"models": []string{"openrouter/anthropic/claude-sonnet-4"},
	}); err != nil {
		t.Fatalf("setup providerPrefix failed: %v", err)
	}
	if _, err := testutil.CreateProvider(providerAffinity, map[string]interface{}{
		"models": []string{"deepseek-chat"},
	}); err != nil {
		t.Fatalf("setup providerAffinity failed: %v", err)
	}

	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode int
		wantMsg  string
		skip     string
		check    func(t *testing.T, resp *testutil.APIResponse)
	}{
		{
			name:     "CL-1-001 最小参数创建集群",
			body:     minClusterBody(clusterMin, providerFull),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				assertNoInternalFields(t, data)
				testutil.AssertDataFieldEquals(t, resp, "name", clusterMin)

				sticky, ok := data["sticky_sessions"].(map[string]interface{})
				if assert.True(t, ok, "sticky_sessions should be an object") {
					assert.Equal(t, false, sticky["enabled"])
					assert.Equal(t, "CLIENT_IP_ONLY", sticky["hash_strategy"])
					assert.Equal(t, "", sticky["hash_header"])
				}
			},
		},
		{
			name: "CL-1-002 完整参数创建集群",
			body: map[string]interface{}{
				"name":        clusterFull,
				"description": "完整集群",
				"basic": map[string]interface{}{
					"protocol": "http",
					"connection": map[string]interface{}{
						"max_idle_conn_per_rs":   0,
						"cancel_on_client_close": false,
					},
					"retries": map[string]interface{}{
						"max_retry_in_cluster": 2,
					},
					"buffers": map[string]interface{}{
						"req_write_buffer_size": 512,
					},
					"timeouts": map[string]interface{}{
						"timeout_conn_serv":         50000,
						"timeout_response_header":   50000,
						"timeout_readbody_client":   30000,
						"timeout_read_client_again": 30000,
						"timeout_write_client":      60000,
					},
				},
				"sticky_sessions": map[string]interface{}{
					"enabled":       false,
					"hash_strategy": "CLIENT_IP_ONLY",
					"hash_header":   "",
				},
				"passive_health_check": map[string]interface{}{
					"interval":   1000,
					"failnum":    3,
					"host":       "",
					"uri":        "/",
					"statuscode": 0,
				},
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat", "deepseek-coder"},
					"keys": []interface{}{
						map[string]interface{}{
							"name":   "primary",
							"weight": 70,
						},
						map[string]interface{}{
							"name":   "secondary",
							"weight": 30,
						},
					},
					"key_policy": map[string]interface{}{
						"strategy":              "weighted_random",
						"max_retries":           3,
						"retry_backoff_initial": 100,
						"retry_backoff_max":     5000,
					},
					"provider": providerFull,
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				assertNoInternalFields(t, data)
				testutil.AssertDataFieldEquals(t, resp, "name", clusterFull)
				llm, _ := data["llm_config"].(map[string]interface{})
				keys, _ := llm["keys"].([]interface{})
				assert.Len(t, keys, 2)
				policy, _ := llm["key_policy"].(map[string]interface{})
				assert.Equal(t, "weighted_random", policy["strategy"])
			},
		},
		{
			name: "CL-1-003 缺少 llm_config",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
			},
			wantCode: 422,
		},
		{
			name:     "CL-1-005 重复集群名",
			body:     minClusterBody(clusterDup, providerFull),
			wantCode: 555,
		},
		{
			name: "CL-1-008 非法 name",
			body: map[string]interface{}{
				"name": "-bad-name-",
				"llm_config": map[string]interface{}{
					"models":   []string{"m"},
					"provider": providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-012 llm_config 模型重复",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":   []string{"m", "m"},
					"provider": providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-013 使用多 Key 创建集群",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{
							"name":   "primary",
							"weight": 70,
						},
						map[string]interface{}{
							"name":   "secondary",
							"weight": 30,
						},
					},
					"key_policy": map[string]interface{}{
						"strategy":              "weighted_random",
						"max_retries":           3,
						"retry_backoff_initial": 100,
						"retry_backoff_max":     5000,
					},
					"provider": providerKeys,
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				llm, _ := data["llm_config"].(map[string]interface{})
				keys, _ := llm["keys"].([]interface{})
				assert.Len(t, keys, 2)
				policy, _ := llm["key_policy"].(map[string]interface{})
				assert.Equal(t, "weighted_random", policy["strategy"])
			},
		},
		{
			name: "CL-1-014 keys 权重和不为 100",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{"name": "primary", "weight": 60},
						map[string]interface{}{"name": "secondary", "weight": 30},
					},
					"provider": providerKeys,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-015 keys 中存在重复 name",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{"name": "same", "weight": 50},
						map[string]interface{}{"name": "same", "weight": 50},
					},
					"provider": providerKeys,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-016 keys 元素缺少必填字段",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{"name": "primary"},
					},
					"provider": providerKeys,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-018 key_policy 非法 strategy",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{"name": "primary", "weight": 100},
					},
					"key_policy": map[string]interface{}{
						"strategy": "round_robin",
					},
					"provider": providerKeys,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-019 key_policy 退避参数非法",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{"name": "primary", "weight": 100},
					},
					"key_policy": map[string]interface{}{
						"strategy":              "weighted_random",
						"retry_backoff_initial": 1000,
						"retry_backoff_max":     500,
					},
					"provider": providerKeys,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-020 合法前缀配置（strip_prefix=true）",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":       []string{"openrouter/anthropic/claude-sonnet-4"},
					"match_prefix": "openrouter/",
					"strip_prefix": true,
					"provider":     providerPrefix,
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				llm, _ := data["llm_config"].(map[string]interface{})
				assert.Equal(t, "openrouter/", llm["match_prefix"])
				assert.Equal(t, true, llm["strip_prefix"])
			},
		},
		{
			name: "CL-1-021 strip_prefix=true 但 match_prefix 为空",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":       []string{"m"},
					"strip_prefix": true,
					"provider":     providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-022 match_prefix 缺少尾部斜杠",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":       []string{"m"},
					"match_prefix": "openrouter",
					"provider":     providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-023 仅 match_prefix、strip_prefix=false",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":       []string{"openrouter/anthropic/claude-sonnet-4"},
					"match_prefix": "openrouter/",
					"strip_prefix": false,
					"provider":     providerPrefix,
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				llm, _ := data["llm_config"].(map[string]interface{})
				assert.Equal(t, "openrouter/", llm["match_prefix"])
				assert.Equal(t, false, llm["strip_prefix"])
			},
		},
		{
			name: "CL-1-024 未配置 match_prefix / strip_prefix",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":   []string{"deepseek-chat"},
					"provider": providerFull,
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				llm, _ := data["llm_config"].(map[string]interface{})
				v, ok := llm["match_prefix"]
				if ok && v != nil {
					assert.Equal(t, "", v, "match_prefix should be empty if present")
				}
			},
		},
		{
			name: "CL-1-025 非法 strip_prefix 类型",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":       []string{"m"},
					"match_prefix": "openrouter/",
					"strip_prefix": "true",
					"provider":     providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-026 合法 key_affinity 配置",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"key_affinity": map[string]interface{}{
						"enabled":        true,
						"ttl":            600,
						"redis_prefix":   "bfe:ai:key_affinity",
						"penalty_enable": true,
					},
					"provider": providerAffinity,
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				llm, _ := data["llm_config"].(map[string]interface{})
				affinity, _ := llm["key_affinity"].(map[string]interface{})
				assert.Equal(t, true, affinity["enabled"])
				assert.Equal(t, float64(600), affinity["ttl"])
				assert.Equal(t, "bfe:ai:key_affinity", affinity["redis_prefix"])
				assert.Equal(t, true, affinity["penalty_enable"])
			},
		},
		{
			name: "CL-1-027 key_affinity.ttl ≤ 0",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"m"},
					"key_affinity": map[string]interface{}{
						"enabled": true,
						"ttl":     0,
					},
					"provider": providerAffinity,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-028 key_affinity.redis_prefix 为空",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"m"},
					"key_affinity": map[string]interface{}{
						"redis_prefix": "",
					},
					"provider": providerAffinity,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-029 provider 不存在",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":   []string{"m"},
					"provider": providerNotExist,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-030 model 不在 provider 模型列表中",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models":   []string{"not-in-provider"},
					"provider": providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name: "CL-1-031 key name 不在 provider 中",
			body: map[string]interface{}{
				"name": testutil.UniqueClusterName(),
				"llm_config": map[string]interface{}{
					"models": []string{"deepseek-chat"},
					"keys": []interface{}{
						map[string]interface{}{"name": "not-exist", "weight": 100},
					},
					"provider": providerFull,
				},
			},
			wantCode: 422,
		},
		{
			name:     "CL-1-032 被动健康检查 failnum 负值",
			body:     clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"failnum": -1}),
			wantCode: 422,
			wantMsg:  "passive_health_check.failnum must be >= 0",
		},
		{
			name:     "CL-1-033 被动健康检查 interval 负值",
			body:     clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"interval": -1}),
			wantCode: 422,
			wantMsg:  "passive_health_check.interval must be >= 0",
		},
		{
			name:     "CL-1-034 被动健康检查 statuscode 超范围",
			body:     clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"statuscode": 999}),
			wantCode: 422,
			wantMsg:  "passive_health_check.statuscode must be 0 or in [100, 599]",
		},
		{
			name:     "CL-1-035 被动健康检查 uri 非 / 开头",
			body:     clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"uri": "healthz"}),
			wantCode: 422,
			wantMsg:  "passive_health_check.uri must be non-empty and start with '/'",
		},
		{
			name:     "CL-1-036 被动健康检查 uri 显式空串",
			body:     clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"uri": ""}),
			wantCode: 422,
			wantMsg:  "passive_health_check.uri must be non-empty and start with '/'",
		},
		{
			name: "CL-1-037 被动健康检查边界合法值",
			body: clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{
				"failnum":    0,
				"interval":   0,
				"statuscode": 0,
				"uri":        "/probe",
			}),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				phc, ok := data["passive_health_check"].(map[string]interface{})
				if assert.True(t, ok, "passive_health_check should be an object") {
					assert.Equal(t, float64(0), phc["failnum"])
					assert.Equal(t, float64(0), phc["interval"])
					assert.Equal(t, float64(0), phc["statuscode"])
					assert.Equal(t, "/probe", phc["uri"])
				}
			},
		},
		{
			name:     "CL-1-038 被动健康检查空对象走默认值",
			body:     clusterBodyWithPHC(testutil.UniqueClusterName(), providerFull, map[string]interface{}{}),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				phc, ok := data["passive_health_check"].(map[string]interface{})
				if assert.True(t, ok, "passive_health_check should be an object") {
					assert.Equal(t, float64(3), phc["failnum"])
					assert.Equal(t, float64(1000), phc["interval"])
					assert.Equal(t, float64(0), phc["statuscode"])
					assert.Equal(t, "/", phc["uri"])
				}
			},
		},
		{
			name:     "CL-1-039 basic.connection.max_idle_conn_per_rs 负值",
			body:     clusterBodyWithBasic(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"connection": map[string]interface{}{"max_idle_conn_per_rs": -1}}),
			wantCode: 422,
			wantMsg:  "basic.connection.max_idle_conn_per_rs must be >= 0",
		},
		{
			name:     "CL-1-040 basic.retries.max_retry_in_cluster 负值",
			body:     clusterBodyWithBasic(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"retries": map[string]interface{}{"max_retry_in_cluster": -1}}),
			wantCode: 422,
			wantMsg:  "basic.retries.max_retry_in_cluster must be >= 0",
		},
		{
			name:     "CL-1-041 basic.buffers.req_write_buffer_size 零值",
			body:     clusterBodyWithBasic(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"buffers": map[string]interface{}{"req_write_buffer_size": 0}}),
			wantCode: 422,
			wantMsg:  "basic.buffers.req_write_buffer_size must be > 0",
		},
		{
			name:     "CL-1-042 basic.timeouts.timeout_conn_serv 零值",
			body:     clusterBodyWithBasic(testutil.UniqueClusterName(), providerFull, map[string]interface{}{"timeouts": map[string]interface{}{"timeout_conn_serv": 0}}),
			wantCode: 422,
			wantMsg:  "basic.timeouts.timeout_conn_serv must be > 0",
		},
		{
			name: "CL-1-043 basic 边界合法值",
			body: clusterBodyWithBasic(testutil.UniqueClusterName(), providerFull, map[string]interface{}{
				"connection": map[string]interface{}{"max_idle_conn_per_rs": 0},
				"retries":    map[string]interface{}{"max_retry_in_cluster": 0},
				"buffers":    map[string]interface{}{"req_write_buffer_size": 1},
				"timeouts":   map[string]interface{}{"timeout_conn_serv": 1},
			}),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				basic, ok := data["basic"].(map[string]interface{})
				if assert.True(t, ok, "basic should be an object") {
					conn, _ := basic["connection"].(map[string]interface{})
					assert.Equal(t, float64(0), conn["max_idle_conn_per_rs"])
					retries, _ := basic["retries"].(map[string]interface{})
					assert.Equal(t, float64(0), retries["max_retry_in_cluster"])
					buffers, _ := basic["buffers"].(map[string]interface{})
					assert.Equal(t, float64(1), buffers["req_write_buffer_size"])
					timeouts, _ := basic["timeouts"].(map[string]interface{})
					assert.Equal(t, float64(1), timeouts["timeout_conn_serv"])
				}
			},
		},
		{
			name:     "CL-1-044 basic 空对象走默认值",
			body:     clusterBodyWithBasic(testutil.UniqueClusterName(), providerFull, map[string]interface{}{}),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				basic, ok := data["basic"].(map[string]interface{})
				if assert.True(t, ok, "basic should be an object") {
					conn, _ := basic["connection"].(map[string]interface{})
					assert.Equal(t, float64(0), conn["max_idle_conn_per_rs"])
					retries, _ := basic["retries"].(map[string]interface{})
					assert.Equal(t, float64(2), retries["max_retry_in_cluster"])
					buffers, _ := basic["buffers"].(map[string]interface{})
					assert.Equal(t, float64(512), buffers["req_write_buffer_size"])
					timeouts, _ := basic["timeouts"].(map[string]interface{})
					assert.Equal(t, float64(50000), timeouts["timeout_conn_serv"])
					assert.Equal(t, float64(60000), timeouts["timeout_write_client"])
				}
			},
		},
		{
			// clusters.md §1/§2.1：description 长度 0-256 字节，256 是合法上界
			name: "CL-1-045 description 恰好 256 字节",
			body: func() map[string]interface{} {
				b := minClusterBody(clusterDescBoundary, providerFull)
				b["description"] = strings.Repeat("b", 256)
				return b
			}(),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "description", strings.Repeat("b", 256))

				got, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterDescBoundary)
				if err != nil {
					t.Fatalf("request failed: %v", err)
				}
				testutil.AssertSuccess(t, got)
				testutil.AssertDataFieldEquals(t, got, "description", strings.Repeat("b", 256))
			},
		},
		{
			name: "CL-1-046 description 257 字节被拒绝",
			body: func() map[string]interface{} {
				b := minClusterBody(testutil.UniqueClusterName(), providerFull)
				b["description"] = strings.Repeat("c", 257)
				return b
			}(),
			wantCode: 422,
			wantMsg:  "description length must be <= 256",
		},
		{
			name: "CL-1-047 description 含控制字符被拒绝",
			body: func() map[string]interface{} {
				b := minClusterBody(testutil.UniqueClusterName(), providerFull)
				b["description"] = "abc\x01def"
				return b
			}(),
			wantCode: 422,
			wantMsg:  "control characters",
		},
	}

	// 预先创建重复集群
	if _, err := testutil.GetClient().Post("/open-api/v1/clusters", minClusterBody(clusterDup, providerFull)); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip != "" {
				t.Skip(tt.skip)
			}
			resp, err := testutil.GetClient().Post("/open-api/v1/clusters", tt.body)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.ErrNum != tt.wantCode {
				t.Errorf("expected ErrNum=%d, got ErrNum=%d, ErrMsg=%s", tt.wantCode, resp.ErrNum, resp.ErrMsg)
			}
			if tt.wantMsg != "" {
				assert.Contains(t, resp.ErrMsg, tt.wantMsg)
			}
			if tt.check != nil && resp.ErrNum == 200 {
				tt.check(t, resp)
			}
		})
	}

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterMin)
		testutil.DeleteCluster(clusterFull)
		testutil.DeleteCluster(clusterDup)
		testutil.DeleteCluster(clusterDescBoundary)
		testutil.DeleteProvider(providerFull)
		testutil.DeleteProvider(providerKeys)
		testutil.DeleteProvider(providerPrefix)
		testutil.DeleteProvider(providerAffinity)
	})
}

// TestClusters_Create_NormalizeUpstreamError covers the llm_config
// .normalize_upstream_error field (2026-10-06 upstream error normalization):
// full/partial create, invalid-value matrix with field-attributable errors,
// and InnerAPI export leak prevention for rejected configs.
// Design: tests/integration/tests/clusters/design.md §12.
func TestClusters_Create_NormalizeUpstreamError(t *testing.T) {
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
	bodyWithNue := func(name string, nue map[string]interface{}) map[string]interface{} {
		body := minClusterBody(name, provider)
		body["llm_config"].(map[string]interface{})["normalize_upstream_error"] = nue
		return body
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
		llm, ok := data["llm_config"].(map[string]interface{})
		if !assert.True(t, ok, "llm_config should be an object") {
			return nil
		}
		nue, _ := llm["normalize_upstream_error"].(map[string]interface{})
		return nue
	}
	assertExportMissing := func(t *testing.T, cluster string) {
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
		_, exists := config[cluster]
		assert.False(t, exists, "rejected cluster %s must not leak into export", cluster)
	}

	t.Run("CL-1-101 全字段创建", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithNue(name, fullNue()))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		nue := readBackNue(t, name)
		if assert.NotNil(t, nue, "normalize_upstream_error should exist") {
			assert.Equal(t, true, nue["enabled"])
			assert.Equal(t, true, nue["stream_enabled"])
			assert.Equal(t, "rewrite_generic", nue["unrecognized_action"])
			assert.Equal(t, float64(65536), nue["max_body_bytes"])
			assert.Equal(t, false, nue["redact_secrets"])
		}
	})

	t.Run("CL-1-102 部分字段创建（仅 enabled）", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithNue(name, map[string]interface{}{
			"enabled": true,
		}))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		nue := readBackNue(t, name)
		if assert.NotNil(t, nue, "normalize_upstream_error should exist") {
			assert.Equal(t, true, nue["enabled"])
			// unset fields read back as null (all-pointer struct, same as key_affinity)
			assert.Nil(t, nue["stream_enabled"])
			assert.Nil(t, nue["unrecognized_action"])
			assert.Nil(t, nue["max_body_bytes"])
			assert.Nil(t, nue["redact_secrets"])
		}
	})

	t.Run("CL-1-103 unrecognized_action 非法值 + 防泄漏", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		nue := fullNue()
		nue["unrecognized_action"] = "replace"
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithNue(name, nue))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 422 {
			t.Fatalf("expected 422, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		assert.Contains(t, resp.ErrMsg, "unrecognized_action")
		assertExportMissing(t, name)
	})

	t.Run("CL-1-104 max_body_bytes 越界（-1 / 4MB+1）+ 防泄漏", func(t *testing.T) {
		for _, bad := range []int64{-1, 4*1024*1024 + 1} {
			name := testutil.UniqueClusterName()
			nue := fullNue()
			nue["max_body_bytes"] = bad
			resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithNue(name, nue))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.ErrNum != 422 {
				t.Fatalf("max_body_bytes=%d: expected 422, got %d, ErrMsg=%s", bad, resp.ErrNum, resp.ErrMsg)
			}
			assert.Contains(t, resp.ErrMsg, "max_body_bytes")
			assertExportMissing(t, name)
		}
	})

	t.Run("CL-1-105 max_body_bytes=0（用默认）", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		defer testutil.DeleteCluster(name)
		nue := fullNue()
		nue["max_body_bytes"] = 0
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithNue(name, nue))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		nueBack := readBackNue(t, name)
		assert.Equal(t, float64(0), nueBack["max_body_bytes"])
	})

	t.Run("CL-1-106 max_body_bytes=4194304（上限边界）", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		defer testutil.DeleteCluster(name)
		nue := fullNue()
		nue["max_body_bytes"] = 4 * 1024 * 1024
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithNue(name, nue))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		nueBack := readBackNue(t, name)
		assert.Equal(t, float64(4*1024*1024), nueBack["max_body_bytes"])
	})
}

// TestClusters_Create_PriceMissAction covers llm_config.price_miss_action
// (issue #1479): the three-value enum round-trips, an unset value reads back
// as null, invalid values are rejected without creating the cluster, and the
// accepted value is exported to AIConf.PriceMissAction for BFE.
// Design: tests/integration/tests/clusters/design.md §13.
func TestClusters_Create_PriceMissAction(t *testing.T) {
	provider := testutil.UniqueProviderName()
	if _, err := testutil.CreateProvider(provider, map[string]interface{}{
		"models": []string{"deepseek-chat"},
	}); err != nil {
		t.Fatalf("setup provider failed: %v", err)
	}
	defer testutil.DeleteProvider(provider)

	bodyWithAction := func(name, action string) map[string]interface{} {
		body := minClusterBody(name, provider)
		body["llm_config"].(map[string]interface{})["price_miss_action"] = action
		return body
	}
	readBackAction := func(t *testing.T, cluster string) (interface{}, bool) {
		t.Helper()
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + cluster)
		if err != nil {
			t.Fatalf("get cluster failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		llm, _ := data["llm_config"].(map[string]interface{})
		v, ok := llm["price_miss_action"]
		return v, ok
	}
	exportedAction := func(t *testing.T, cluster string) interface{} {
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
		return aiConf["PriceMissAction"]
	}

	for _, action := range []string{"fallback_chat", "bill_zero", "reject"} {
		t.Run("CL-1-121 创建并回读 "+action, func(t *testing.T) {
			name := testutil.UniqueClusterName()
			defer testutil.DeleteCluster(name)
			resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithAction(name, action))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.ErrNum != 200 {
				t.Fatalf("expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
			}
			if got, ok := readBackAction(t, name); !assert.True(t, ok, "price_miss_action should exist") || got != action {
				t.Fatalf("read back price_miss_action = %v, want %q", got, action)
			}
			// snake_case (API) -> PascalCase (BFE export) mapping
			if got := exportedAction(t, name); got != action {
				t.Fatalf("exported AIConf.PriceMissAction = %v, want %q", got, action)
			}
		})
	}

	t.Run("CL-1-122 省略时回读为 null 且导出为空", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		defer testutil.DeleteCluster(name)
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", minClusterBody(name, provider))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 200 {
			t.Fatalf("expected 200, got %d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
		if got, ok := readBackAction(t, name); ok && got != nil {
			t.Fatalf("price_miss_action = %v, want null when unset", got)
		}
		// bfe normalizes the empty export to fallback_chat; the key may be
		// absent or empty here.
		if got := exportedAction(t, name); got != nil && got != "" {
			t.Fatalf("exported AIConf.PriceMissAction = %v, want empty when unset", got)
		}
	})

	t.Run("CL-1-123 非法值 4xx 且不创建集群", func(t *testing.T) {
		name := testutil.UniqueClusterName()
		resp, err := testutil.GetClient().Post("/open-api/v1/clusters", bodyWithAction(name, "reject_all"))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum == 200 {
			testutil.DeleteCluster(name)
			t.Fatalf("expected a 4xx for an invalid price_miss_action, got 200")
		}
		// read-back zero-change: the cluster must not exist
		getResp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + name)
		if err != nil {
			t.Fatalf("get cluster failed: %v", err)
		}
		if getResp.ErrNum == 200 {
			testutil.DeleteCluster(name)
			t.Fatalf("cluster %s must not exist after a rejected create", name)
		}
	})
}
