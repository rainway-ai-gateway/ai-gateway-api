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

type pagination struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

type listResponse struct {
	List       []modelPriceDTO `json:"list"`
	Pagination pagination      `json:"pagination"`
}

type modelPriceDTO struct {
	ID        int64              `json:"id"`
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	BaseModel string             `json:"base_model"`
	Mode      string             `json:"mode"`
	Prices    map[string]float64 `json:"prices"`
}

func TestModelPrice_Import(t *testing.T) {
	t.Run("MP-1-001 replace 模式全量替换", func(t *testing.T) {
		// 先创建一条旧记录
		oldProvider := testutil.UniqueName("provider")
		_, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":   oldProvider,
			"model":      "old-model",
			"base_model": "old-model",
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.001,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		provider1 := testutil.UniqueName("provider")
		provider2 := testutil.UniqueName("provider")

		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider1,
				"model":    "gpt-4o",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0001,
				},
			},
			{
				"provider": provider2,
				"model":    "deepseek-v3",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.000002,
				},
			},
		})

		result, resp, err := testutil.ImportModelPricesWithResult([]byte(yaml), "replace")
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		assert.Equal(t, 2, result.ImportedCount)
		assert.Equal(t, 0, result.SkippedCount)

		list, err := fetchModelPriceList()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		assert.Equal(t, int64(2), list.Pagination.Total)

		providers := map[string]bool{}
		for _, item := range list.List {
			providers[item.Provider] = true
		}
		assert.True(t, providers[provider1], "expected provider1 in list")
		assert.True(t, providers[provider2], "expected provider2 in list")
		assert.False(t, providers[oldProvider], "expected old record removed")

		// cleanup via replace with empty
		_, _, _ = testutil.ImportModelPricesWithResult([]byte(buildYAML(nil)), "replace")
	})

	t.Run("MP-1-002 merge 模式增量合并", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		model := "gpt-4o"
		id, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":   provider,
			"model":      model,
			"base_model": model,
			"mode":       "chat",
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.0001,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		newProvider := testutil.UniqueName("provider")

		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    model,
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0002,
				},
			},
			{
				"provider": newProvider,
				"model":    "new-model",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0003,
				},
			},
		})

		result, resp, err := testutil.ImportModelPricesWithResult([]byte(yaml), "merge")
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		assert.Equal(t, 2, result.ImportedCount)

		one, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + fmt.Sprintf("%d", id))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertSuccess(t, one)

		var data map[string]interface{}
		if err := json.Unmarshal(one.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		prices := data["prices"].(map[string]interface{})
		assert.InDelta(t, 0.0002, prices["input_cost_per_token"], 0.0000001)

		list, err := fetchModelPriceList()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		assert.GreaterOrEqual(t, list.Pagination.Total, int64(2))

		_, _, _ = testutil.ImportModelPricesWithResult([]byte(buildYAML(nil)), "replace")
	})

	t.Run("MP-1-011 merge 模式整行覆盖：最小记录清空可选字段（契约钉死）", func(t *testing.T) {
		// 创建一条携带可选字段的已有记录
		provider := testutil.UniqueName("provider")
		model := "merge-cover-model"
		id, err := testutil.CreateModelPrice(map[string]interface{}{
			"provider":             provider,
			"model":                model,
			"base_model":           model,
			"mode":                 "chat",
			"capabilities":         []string{"chat", "reasoning", "tools", "embedding", "rerank"},
			"supported_parameters": []string{"temperature", "max_tokens"},
			"limits":               map[string]interface{}{"context_window": 128000},
			"metadata":             map[string]interface{}{"source": "test"},
			"prices": map[string]interface{}{
				"input_cost_per_token": 0.0001,
			},
		})
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		// merge 导入同键最小记录（仅必填字段），按 §3.1 第 9 步契约：
		// 命中已有记录时整行覆盖，未提供的可选字段被清空
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    model,
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0002,
				},
			},
		})

		result, resp, err := testutil.ImportModelPricesWithResult([]byte(yaml), "merge")
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		assert.Equal(t, 1, result.ImportedCount)

		one, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + fmt.Sprintf("%d", id))
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertSuccess(t, one)

		var data map[string]interface{}
		if err := json.Unmarshal(one.Data, &data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// 必填字段按导入值更新
		prices := data["prices"].(map[string]interface{})
		assert.InDelta(t, 0.0002, prices["input_cost_per_token"], 0.0000001)
		// 未提供的可选字段被清空（序列化 omitempty → 字段缺失）
		for _, field := range []string{"capabilities", "supported_parameters", "limits", "metadata", "tier_prices"} {
			_, ok := data[field]
			assert.False(t, ok, "expected field %s cleared after merge overwrite", field)
		}

		_, _, _ = testutil.ImportModelPricesWithResult([]byte(buildYAML(nil)), "replace")
	})

	t.Run("MP-1-003 默认 replace 模式", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "default-model",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0001,
				},
			},
		})

		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte(yaml), nil)
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		var result testutil.ImportModelPricesResult
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		assert.Equal(t, 1, result.ImportedCount)

		list, err := fetchModelPriceList()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		assert.Equal(t, int64(1), list.Pagination.Total)

		_, _, _ = testutil.ImportModelPricesWithResult([]byte(buildYAML(nil)), "replace")
	})

	t.Run("MP-1-004 非法 mode", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "m",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0001,
				},
			},
		})

		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte(yaml), map[string]string{
			"mode": "invalid",
		})
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-1-005 非法 YAML", func(t *testing.T) {
		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte("not a yaml: ["), map[string]string{
			"mode": "replace",
		})
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-1-007 limits 包含负数", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "m",
				"mode":     "chat",
				"limits": map[string]interface{}{
					"context_window": -1,
				},
				"prices": map[string]float64{
					"input_cost_per_token": 0.0001,
				},
			},
		})

		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte(yaml), map[string]string{
			"mode": "replace",
		})
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-1-006 重复三元组", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "dup-model",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0001,
				},
			},
			{
				"provider": provider,
				"model":    "dup-model",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0002,
				},
			},
		})

		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte(yaml), map[string]string{
			"mode": "replace",
		})
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-1-008 未知 provider 可导入", func(t *testing.T) {
		provider := testutil.UniqueName("unknown-provider")
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "unknown-model",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token": 0.0001,
				},
			},
		})

		result, resp, err := testutil.ImportModelPricesWithResult([]byte(yaml), "replace")
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		assert.Equal(t, 1, result.ImportedCount)
		assert.Equal(t, 0, result.SkippedCount)

		list, err := fetchModelPriceList()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		assert.Equal(t, int64(1), list.Pagination.Total)

		_, _, _ = testutil.ImportModelPricesWithResult([]byte(buildYAML(nil)), "replace")
	})

	t.Run("MP-1-009 科学计数法价格导入", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		// buildYAML uses %v formatting, which prints small float64 values
		// in scientific notation (e.g. 7.6234102728e-08).
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "sci-notation-model",
				"mode":     "chat",
				"prices": map[string]float64{
					"input_cost_per_token":  7.6234102728e-08,
					"output_cost_per_token": 4.141631732e-06,
				},
			},
		})
		assert.Contains(t, yaml, "7.6234102728e-08")

		result, resp, err := testutil.ImportModelPricesWithResult([]byte(yaml), "replace")
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		assert.Equal(t, 1, result.ImportedCount)

		list, err := fetchModelPriceList()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		var item *modelPriceDTO
		for i := range list.List {
			if list.List[i].Provider == provider {
				item = &list.List[i]
				break
			}
		}
		if item == nil {
			t.Fatalf("imported price not found for provider %s", provider)
		}
		// Scientific notation and decimal notation are equivalent float64 values.
		assert.InDelta(t, 7.6234102728e-08, item.Prices["input_cost_per_token"], 1e-20)
		assert.InDelta(t, 4.141631732e-06, item.Prices["output_cost_per_token"], 1e-20)

		_, _, _ = testutil.ImportModelPricesWithResult([]byte(buildYAML(nil)), "replace")
	})

	t.Run("MP-1-010 超精度价格拒绝", func(t *testing.T) {
		provider := testutil.UniqueName("provider")
		yaml := buildYAML([]map[string]interface{}{
			{
				"provider": provider,
				"model":    "excessive-precision-model",
				"mode":     "chat",
				"prices": map[string]float64{
					// 1e8 * 1e8 = 1e16 >= 2^53 (~9.007e15), exceeds the representable limit.
					"input_cost_per_token": 1e8,
				},
			},
		})

		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte(yaml), map[string]string{
			"mode": "replace",
		})
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("MP-1-012 导入超长 base_model 拒绝且不清空存量", func(t *testing.T) {
		// issue #309：导入路径同样必须 422；校验先于 replace 的 DeleteAll，
		// 预置记录必须存活（否则非法导入会静默清空全表）。
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
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteModelPrice(id)

		yaml := fmt.Sprintf(`version: v1.0
default_currency: RMB
models:
  - provider: %s
    model: %s
    base_model: %s
    mode: chat
    prices:
      input_cost_per_token: 0.001
`, provider, model, strings.Repeat("a", 256))

		resp, err := testutil.GetClient().PostMultipartFile("/open-api/v1/model-prices/import", "file", "model-list.yaml", []byte(yaml), map[string]string{
			"mode": "replace",
		})
		if err != nil {
			t.Fatalf("import failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		after, err := testutil.GetClient().Get("/open-api/v1/model-prices/" + fmt.Sprintf("%d", id))
		if err != nil {
			t.Fatalf("get model price failed: %v", err)
		}
		testutil.AssertSuccess(t, after)
	})
}

func buildYAML(models []map[string]interface{}) string {
	yaml := "version: v1.0\ndefault_currency: RMB\n"
	if len(models) == 0 {
		yaml += "models: []\n"
		return yaml
	}
	yaml += "models:\n"
	for _, m := range models {
		yaml += fmt.Sprintf("  - provider: %s\n", m["provider"])
		yaml += fmt.Sprintf("    model: %s\n", m["model"])
		yaml += fmt.Sprintf("    base_model: %s\n", m["model"])
		yaml += fmt.Sprintf("    mode: %s\n", m["mode"])
		if limits, ok := m["limits"].(map[string]interface{}); ok {
			yaml += "    limits:\n"
			for k, v := range limits {
				yaml += fmt.Sprintf("      %s: %v\n", k, v)
			}
		}
		prices := m["prices"].(map[string]float64)
		yaml += "    prices:\n"
		for k, v := range prices {
			yaml += fmt.Sprintf("      %s: %v\n", k, v)
		}
	}
	return yaml
}

func fetchModelPriceList() (*listResponse, error) {
	resp, err := testutil.GetClient().Get("/open-api/v1/model-prices")
	if err != nil {
		return nil, err
	}
	if resp.ErrNum != 200 {
		return nil, fmt.Errorf("list failed: %d %s", resp.ErrNum, resp.ErrMsg)
	}
	var list listResponse
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		return nil, err
	}
	return &list, nil
}
