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

package provider_test

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

func minProviderBody(name string) map[string]interface{} {
	return map[string]interface{}{
		"name": name,
		"instance_pool": []interface{}{
			map[string]interface{}{

				"addr":   "10.0.0.1",
				"weight": 100,
				"port":   8080,
			},
		},
		"model_protocols": []string{"openai"},
		"models":          []string{"deepseek-chat"},
	}
}

func TestProvider_Create(t *testing.T) {
	providerMin := testutil.UniqueProviderName()
	providerFull := testutil.UniqueProviderName()
	providerAnthropic := testutil.UniqueProviderName()
	providerGemini := testutil.UniqueProviderName()
	providerNoInstName := testutil.UniqueProviderName()
	providerDup := testutil.UniqueProviderName()
	providerProtoPaths := testutil.UniqueProviderName()

	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode int
		check    func(t *testing.T, resp *testutil.APIResponse)
	}{
		{
			name:     "PV-1-001 最小参数创建 Provider",
			body:     minProviderBody(providerMin),
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "name", providerMin)
				testutil.AssertDataFieldEquals(t, resp, "description", "")
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				models, _ := data["models"].([]interface{})
				assert.Equal(t, []interface{}{"deepseek-chat"}, models)
				keys, _ := data["keys"].([]interface{})
				assert.Empty(t, keys)
			},
		},
		{
			name: "PV-1-002 完整参数创建 Provider",
			body: map[string]interface{}{
				"name":        providerFull,
				"description": "完整 Provider",
				"model_endpoint": map[string]interface{}{
					"schema": "https",
					"uri":    "/v1/models",
				},
				"models": []string{"deepseek-chat", "deepseek-coder"},
				"keys": []interface{}{
					map[string]interface{}{
						"name": "key-primary",
						"key":  "sk-aaaaaaaaaaaa",
					},
					map[string]interface{}{
						"name": "key-secondary",
						"key":  "sk-bbbbbbbbbbbb",
					},
				},
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "api.deepseek.com",
						"weight": 100,
						"port":   443,
					},
				},
				"model_protocols": []string{"openai"},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "name", providerFull)
				testutil.AssertDataFieldEquals(t, resp, "description", "完整 Provider")
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				models, _ := data["models"].([]interface{})
				assert.Len(t, models, 2)
				keys, _ := data["keys"].([]interface{})
				assert.Len(t, keys, 2)
				insts, _ := data["instance_pool"].([]interface{})
				assert.Len(t, insts, 1)
			},
		},
		{
			name: "PV-1-002a 创建 anthropic 协议 Provider",
			body: map[string]interface{}{
				"name": providerAnthropic,
				"keys": []interface{}{
					map[string]interface{}{
						"name": "key-primary",
						"key":  "sk-ant-aaaaaaaaaaaa",
					},
				},
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "api.anthropic.com",
						"weight": 100,
						"port":   443,
					},
				},
				"model_protocols": []string{"anthropic"},
				"models":          []string{"claude-3-5-sonnet-20241022"},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				protocols, _ := data["model_protocols"].([]interface{})
				assert.Equal(t, []interface{}{"anthropic"}, protocols)
			},
		},
		{
			name: "PV-1-002c 创建 gemini 协议 Provider",
			body: map[string]interface{}{
				"name": providerGemini,
				"keys": []interface{}{
					map[string]interface{}{
						"name": "key-primary",
						"key":  "AIzaSyaaaaaaaaaaaa",
					},
				},
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "generativelanguage.googleapis.com",
						"weight": 100,
						"port":   443,
					},
				},
				"model_protocols": []string{"gemini"},
				"models":          []string{"gemini-2.5-pro"},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				protocols, _ := data["model_protocols"].([]interface{})
				assert.Equal(t, []interface{}{"gemini"}, protocols)
			},
		},
		{
			name: "PV-1-002b instance_pool 不再包含 name 字段",
			body: map[string]interface{}{
				"name": providerNoInstName,
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.99",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"models":          []string{"deepseek-chat"},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				insts, _ := data["instance_pool"].([]interface{})
				assert.Len(t, insts, 1)
				inst, _ := insts[0].(map[string]interface{})
				_, ok := inst["name"]
				assert.False(t, ok, "instance should not contain name field")
				assert.Equal(t, "10.0.0.99", inst["addr"])
			},
		},
		{
			name:     "PV-1-003 重复 Provider 名称",
			body:     minProviderBody(providerDup),
			wantCode: 555,
		},
		{
			name: "PV-1-004 缺少 instance_pool",
			body: map[string]interface{}{
				"name":            testutil.UniqueProviderName(),
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-005 缺少 model_protocols",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-006 非法 model_protocols",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"invalid"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-007 instance_pool 为空数组",
			body: map[string]interface{}{
				"name":            testutil.UniqueProviderName(),
				"instance_pool":   []interface{}{},
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-008 实例 port 非法",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   0,
					},
				},
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-009 非法 name",
			body: map[string]interface{}{
				"name": "-bad-name-",
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-010 weight 超过 100",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 101,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-011 重复实例 (addr+port)",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 50,
						"port":   8080,
					},
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 50,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-012 模型重复",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"models":          []string{"m", "m"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-013 keys 中存在重复 name",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"keys": []interface{}{
					map[string]interface{}{"name": "same", "key": "sk-1"},
					map[string]interface{}{"name": "same", "key": "sk-2"},
				},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-014 创建带 protocol_paths 的 Provider（双协议百炼形态）",
			body: map[string]interface{}{
				"name": providerProtoPaths,
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "dashscope.aliyuncs.com",
						"weight": 100,
						"port":   443,
					},
				},
				"model_protocols": []string{"openai", "anthropic"},
				"protocol_paths": map[string]interface{}{
					"openai":    "/compatible-mode/v1",
					"anthropic": "/apps/anthropic",
				},
				"models": []string{"deepseek-chat"},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "name", providerProtoPaths)
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				paths, ok := data["protocol_paths"].(map[string]interface{})
				if !assert.True(t, ok, "protocol_paths should be an object") {
					return
				}
				assert.Equal(t, "/compatible-mode/v1", paths["openai"])
				assert.Equal(t, "/apps/anthropic", paths["anthropic"])
			},
		},
		{
			name: "PV-1-015 protocol_paths 键未在 model_protocols 声明",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"protocol_paths":  map[string]interface{}{"anthropic": "/apps/anthropic"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-016 protocol_paths 非法协议键",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai", "anthropic", "gemini"},
				"protocol_paths":  map[string]interface{}{"gemini": "/v1beta"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-017 protocol_paths 值缺少 / 前缀",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"protocol_paths":  map[string]interface{}{"openai": "compatible-mode/v1"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-018 protocol_paths 值以 / 结尾",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"protocol_paths":  map[string]interface{}{"openai": "/compatible-mode/"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-019 缺少 models",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
			},
			wantCode: 422,
		},
		{
			name: "PV-1-020 models 为空数组",
			body: map[string]interface{}{
				"name": testutil.UniqueProviderName(),
				"instance_pool": []interface{}{
					map[string]interface{}{
						"addr":   "10.0.0.1",
						"weight": 100,
						"port":   8080,
					},
				},
				"model_protocols": []string{"openai"},
				"models":          []string{},
			},
			wantCode: 422,
		},
	}

	// 预先创建重复 Provider
	if _, err := testutil.GetClient().Post("/open-api/v1/providers", minProviderBody(providerDup)); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := testutil.GetClient().Post("/open-api/v1/providers", tt.body)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.ErrNum != tt.wantCode {
				t.Errorf("expected ErrNum=%d, got ErrNum=%d, ErrMsg=%s", tt.wantCode, resp.ErrNum, resp.ErrMsg)
			}
			if tt.check != nil && resp.ErrNum == 200 {
				tt.check(t, resp)
			}
		})
	}

	t.Run("PV-1-020 description 256 字节多字节边界（合法）", func(t *testing.T) {
		// issue #310：providers.md 统一为 UTF-8 字节口径后，85 个汉字（255
		// 字节）+ "a" = 256 字节，合法；修复前按字节误拒的边界必须放通。
		name := testutil.UniqueProviderName()
		defer testutil.DeleteProvider(name)
		desc := strings.Repeat("汉", 85) + "a"
		body := minProviderBody(name)
		body["description"] = desc
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", body)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", desc)
	})

	t.Run("PV-1-021 description 600 字节被拒绝", func(t *testing.T) {
		// 200 个汉字 = 600 字节 > 256 上限，按统一后的字节口径必须 422。
		name := testutil.UniqueProviderName()
		body := minProviderBody(name)
		body["description"] = strings.Repeat("汉", 200)
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", body)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("PV-1-022 instance addr 为非法 hostname（连字符开头）", func(t *testing.T) {
		// issue #321：E2E 复现形态，双实例 weight 0/100
		name := testutil.UniqueProviderName()
		body := map[string]interface{}{
			"name": name,
			"instance_pool": []interface{}{
				map[string]interface{}{"addr": "-bad", "weight": 0, "port": 8080},
				map[string]interface{}{"addr": "10.0.0.2", "weight": 100, "port": 8080},
			},
			"model_protocols": []string{"openai"},
			"models":          []string{"deepseek-chat"},
		}
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", body)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		// 非持久化守护：非法 Provider 不得落库
		resp, err = testutil.GetClient().Get("/open-api/v1/providers/" + name)
		if err != nil {
			t.Fatalf("readback failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("PV-1-023 instance addr 尾随连字符", func(t *testing.T) {
		body := minProviderBody(testutil.UniqueProviderName())
		body["instance_pool"] = []interface{}{
			map[string]interface{}{"addr": "bad-", "weight": 100, "port": 8080},
		}
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", body)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Cleanup(func() {
		testutil.DeleteProvider(providerMin)
		testutil.DeleteProvider(providerFull)
		testutil.DeleteProvider(providerAnthropic)
		testutil.DeleteProvider(providerGemini)
		testutil.DeleteProvider(providerNoInstName)
		testutil.DeleteProvider(providerDup)
		testutil.DeleteProvider(providerProtoPaths)
	})
}
