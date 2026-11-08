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

package model_price_test

import (
	"encoding/json"
	"fmt"
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

func TestModelPrice_Create(t *testing.T) {
	t.Run("MP-2-001 最小参数创建", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		model := testutil.UniqueName("model")

		id, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":   provider,
			"model":      model,
			"base_model": model,
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("create model price failed: %v", err)
		}
		defer testutil.DeleteModelPrice(id)

		assert.Greater(t, id, int64(0))

		resp, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + int64ToStr(id))
		if err != nil {
			t.Fatalf("get model price failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "provider", provider)
		testutil.AssertDataFieldEquals(t, resp, "price_currency", "RMB")
		testutil.AssertDataFieldNotEmpty(t, resp, "create_time")
	})

	t.Run("MP-2-002 完整参数创建", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		model := testutil.UniqueName("model")

		id, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":             provider,
			"model":                model,
			"base_model":           model,
			"mode":                 "chat",
			"capabilities":         []string{"chat", "reasoning", "tools"},
			"supported_parameters": []string{"temperature", "max_tokens"},
			"limits": map[string]interface{}{
				"context_window":    128000,
				"max_input_tokens":  128000,
				"max_output_tokens": 8192,
			},
			"prices": map[string]interface{}{
				"input_cost_per_token":  0.000002,
				"output_cost_per_token": 0.000008,
			},
			"metadata": map[string]interface{}{
				"source": "https://example.com/pricing",
				"notes":  "test model",
			},
		})
		if err != nil {
			t.Fatalf("create model price failed: %v", err)
		}
		defer testutil.DeleteModelPrice(id)

		resp, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + int64ToStr(id))
		if err != nil {
			t.Fatalf("get model price failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal data: %v", err)
		}
		assert.Equal(t, provider, data["provider"])
		assert.Equal(t, model, data["model"])
		assert.Equal(t, "chat", data["mode"])
		assert.Equal(t, "RMB", data["price_currency"])

		caps := data["capabilities"].([]interface{})
		assert.Len(t, caps, 3)
	})

	t.Run("MP-2-003 缺少 provider", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/model-prices", map[string]interface{}{
			"model":      "test-model",
			"base_model": "test-model",
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-2-004 非法 mode", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/model-prices", map[string]interface{}{
			"provider":   testutil.UniqueName("provider"),
			"model":      "test-model",
			"base_model": "test-model",
			"mode":       "invalid_mode",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-2-005 prices 包含负数", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/model-prices", map[string]interface{}{
			"provider":   testutil.UniqueName("provider"),
			"model":      "test-model",
			"base_model": "test-model",
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": -0.001,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-2-007 limits 包含负数", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/model-prices", map[string]interface{}{
			"provider":   testutil.UniqueName("provider"),
			"model":      "test-model",
			"base_model": "test-model",
			"mode":       "chat",
			"limits": map[string]interface{}{
				"context_window": -1,
			},
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-2-008 未知 provider 可创建", func(t *testing.T) {
		provider := testutil.UniqueName("unknown-provider")
		model := testutil.UniqueName("model")

		id, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":   provider,
			"model":      model,
			"base_model": model,
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("create model price failed: %v", err)
		}
		defer testutil.DeleteModelPrice(id)

		assert.Greater(t, id, int64(0))

		resp, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + int64ToStr(id))
		if err != nil {
			t.Fatalf("get model price failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "provider", provider)
	})

	t.Run("MP-2-006 重复三元组", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		model := testutil.UniqueName("model")
		body := map[string]interface{}{
			"provider":   provider,
			"model":      model,
			"base_model": model,
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		}

		id, err := testutil.CreateModelPrice(body)
		if err != nil {
			t.Fatalf("create first model price failed: %v", err)
		}
		defer testutil.DeleteModelPrice(id)

		resp, err := testutil.GetClient().Post("/open-api/v1/model-prices", body)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 422 && resp.ErrNum != 500 {
			t.Errorf("expected ErrNum=422 or 500, got ErrNum=%d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
	})

	t.Run("MP-2-007 provider/model/base_model 超长拒绝且不落库", func(t *testing.T) {
		// issue #309：契约要求三字段长度 1-255，超长必须 422 且不落库
		base := func() map[string]interface{} {
			return map[string]interface{}{
				"provider":   testutil.UniqueName("provider"),
				"model":      testutil.UniqueName("model"),
				"base_model": testutil.UniqueName("model"),
				"mode":       "chat",
				"prices": map[string]interface{}{
					"input_cost_per_token": 0.001,
				},
			}
		}
		long := strings.Repeat("a", 256)
		for field, mutate := range map[string]func(map[string]interface{}){
			"provider":   func(b map[string]interface{}) { b["provider"] = long },
			"model":      func(b map[string]interface{}) { b["model"] = long },
			"base_model": func(b map[string]interface{}) { b["base_model"] = long },
		} {
			body := base()
			mutate(body)
			resp, err := testutil.GetClient().Post("/open-api/v1/model-prices", body)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertErrCode(t, resp, 422)

			// 修复前超长输入会进入存储层（严格模式 500 / 非严格截断落库），
			// 这里钉死：按 provider 过滤列表必须为空。
			listResp, err := testutil.GetClient().Get("/open-api/v1/model-prices", map[string]string{
				"provider": body["provider"].(string),
			})
			if err != nil {
				t.Fatalf("list failed: %v", err)
			}
			testutil.AssertSuccess(t, listResp)
			var list map[string]interface{}
			if err := json.Unmarshal(listResp.Data, &list); err != nil {
				t.Fatalf("unmarshal list: %v", err)
			}
			pagination := list["pagination"].(map[string]interface{})
			assert.Equal(t, float64(0), pagination["total"], "field %s: overlong record must not be persisted", field)
		}
	})

	t.Run("MP-2-008 255 字符边界创建成功", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		model := strings.Repeat("m", 255)

		id, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":   provider,
			"model":      model,
			"base_model": model,
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("create model price failed: %v", err)
		}
		defer testutil.DeleteModelPrice(id)

		resp, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + int64ToStr(id))
		if err != nil {
			t.Fatalf("get model price failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "model", model)
	})
}

func int64ToStr(v int64) string {
	return fmt.Sprintf("%d", v)
}
