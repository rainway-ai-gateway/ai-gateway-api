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

package api_key_test

import (
	"encoding/json"
	"os"
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

func TestAPIKey_Create(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	entityName := testutil.UniqueEntityName()
	entityID, err := testutil.CreateEntity(entityName, typeName, "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	importedKey := "my-imported-key-" + testutil.RandomString(6)

	// 预先创建重复 key
	if _, err := testutil.GetClient().Post("/open-api/v1/api-keys", map[string]interface{}{
		"description": "dup-key",
		"key":         importedKey,
	}); err != nil {
		t.Fatalf("setup dup key failed: %v", err)
	}

	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode int
		check    func(t *testing.T, resp *testutil.APIResponse)
	}{
		{
			name:     "AK-1-001 最小参数创建",
			body:     map[string]interface{}{"description": "test-key-min"},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "description", "test-key-min")
				testutil.AssertDataFieldEquals(t, resp, "enabled", true)
				testutil.AssertDataFieldNotEmpty(t, resp, "quota_plan")
			},
		},
		{
			name: "AK-1-002 完整参数创建",
			body: map[string]interface{}{
				"description":     "test-key-full",
				"expired_time":    -1,
				"enabled":         true,
				"unlimited_quota": false,
				"models":          []string{"gpt-4"},
				"subnet":          []string{"10.0.0.0/8"},
				"quota_plan": map[string]interface{}{
					"unlimited":    false,
					"quota":        1000000,
					"unit":         "total_token",
					"reset_period": "monthly",
				},
				"rate_limit_policy": map[string]interface{}{
					"enabled": true,
					"rules": map[string]interface{}{
						"tpm": []interface{}{
							map[string]interface{}{
								"name": "1m", "model": "*", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1,
							},
						},
						"max_concurrency": 50,
					},
				},
				"entity_id": entityID,
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "description", "test-key-full")
				testutil.AssertDataFieldNotEmpty(t, resp, "entity")
			},
		},
		{
			name:     "AK-1-003 导入外部 key",
			body:     map[string]interface{}{"description": "test-key-import", "key": "my-imported-key-001"},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				testutil.AssertDataFieldEquals(t, resp, "key", "my-imported-key-001")
			},
		},
		{
			name:     "AK-1-004 导入重复 key",
			body:     map[string]interface{}{"description": "test-key-dup", "key": importedKey},
			wantCode: 422,
		},
		{
			name:     "AK-1-005 缺少 description",
			body:     map[string]interface{}{},
			wantCode: 422,
		},
		{
			name:     "AK-1-006 非法 key 字符",
			body:     map[string]interface{}{"description": "test-key-bad-char", "key": "invalid@key"},
			wantCode: 422,
		},
		{
			name:     "AK-1-007 非法 subnet",
			body:     map[string]interface{}{"description": "test-key-bad-subnet", "subnet": []string{"not-a-cidr"}},
			wantCode: 422,
		},
		{
			name: "AK-1-008 quota_plan 非法 unit",
			body: map[string]interface{}{
				"description": "test-key-bad-unit",
				"quota_plan": map[string]interface{}{
					"unlimited": false,
					"quota":     100,
					"unit":      "invalid_unit",
				},
			},
			wantCode: 422,
		},
		{
			name: "AK-1-009 rate_limit_policy 非法 window_minutes",
			body: map[string]interface{}{
				"description": "test-key-bad-window",
				"rate_limit_policy": map[string]interface{}{
					"enabled": true,
					"rules": map[string]interface{}{
						"tpm": []interface{}{
							map[string]interface{}{"name": "t1", "model": "*", "window_minutes": 0, "max_tokens": 100, "step_minutes": 1},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "AK-1-010 route_rules 规则名称重复",
			body: map[string]interface{}{
				"description": "test-key-dup-rule",
				"route_rules": map[string]interface{}{
					"enabled": true,
					"rules": []interface{}{
						map[string]interface{}{
							"name": "dup",
							"cond": "default_t()",
							"targets": []interface{}{
								map[string]interface{}{"cluster_name": "c1", "weight": 100},
							},
						},
						map[string]interface{}{
							"name": "dup",
							"cond": "default_t()",
							"targets": []interface{}{
								map[string]interface{}{"cluster_name": "c2", "weight": 100},
							},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "AK-1-011 创建 API-Key 并指定 RMB 配额",
			body: map[string]interface{}{
				"description": "test-key-rmb-quota",
				"quota_plan": map[string]interface{}{
					"unlimited":    false,
					"quota":        1234.5678,
					"unit":         "RMB",
					"reset_period": "monthly",
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				if err := json.Unmarshal(resp.Data, &data); err != nil {
					t.Fatalf("unmarshal data: %v", err)
				}
				qp, ok := data["quota_plan"].(map[string]interface{})
				if !assert.True(t, ok, "quota_plan should be an object") {
					return
				}
				assert.Equal(t, "RMB", qp["unit"])
				assert.InDelta(t, float64(1234.5678), qp["quota"], 0.00001)

				id, _ := data["id"].(string)
				qpResp, err := testutil.GetClient().Get("/open-api/v1/api-keys/" + id + "/quota-plan")
				if err != nil {
					t.Fatalf("query quota-plan failed: %v", err)
				}
				testutil.AssertSuccess(t, qpResp)
				var qpData map[string]interface{}
				if err := json.Unmarshal(qpResp.Data, &qpData); err != nil {
					t.Fatalf("unmarshal quota-plan data: %v", err)
				}
				balance, ok := qpData["balance"].(map[string]interface{})
				if !assert.True(t, ok, "balance should be an object") {
					return
				}
				assert.InDelta(t, float64(1234.5678), balance["remaining"], 0.00001)
				assert.InDelta(t, float64(0), balance["used"], 0.00001)
			},
		},
		{
			name: "AK-1-012 创建 API-Key 时 RMB 配额超过 9000 万元上限",
			body: map[string]interface{}{
				"description": "test-key-rmb-quota-over-limit",
				"quota_plan": map[string]interface{}{
					"unlimited":    false,
					"quota":        90000000.01,
					"unit":         "RMB",
					"reset_period": "monthly",
				},
			},
			wantCode: 422,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := testutil.GetClient().Post("/open-api/v1/api-keys", tt.body)
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

	t.Cleanup(func() {
		testutil.DeleteEntity(entityID)
		testutil.DeleteEntityType(typeName)
	})
}

// TestAPIKey_CreateRMBQuotaDecimals 覆盖 unit=RMB 时 quota 的小数位判定
// （issue #293）。0.14 / 600.1234 在修复前被 422 拒绝；九位小数与超上限必须拒绝。
func TestAPIKey_CreateRMBQuotaDecimals(t *testing.T) {
	post := func(t *testing.T, quota float64) *testutil.APIResponse {
		t.Helper()
		resp, err := testutil.GetClient().Post("/open-api/v1/api-keys", map[string]interface{}{
			"description": "test-key-rmb-decimals",
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        quota,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp
	}

	for _, tc := range []struct {
		name  string
		quota float64
	}{
		{"AK-1-013 RMB 配额两位小数", 0.14},
		{"AK-1-014 RMB 配额四位小数", 600.1234},
		{"AK-1-015 RMB 配额八位小数", 1000.12345678},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := post(t, tc.quota)
			testutil.AssertSuccess(t, resp)

			var data map[string]interface{}
			if err := json.Unmarshal(resp.Data, &data); err != nil {
				t.Fatalf("unmarshal data: %v", err)
			}
			qp, ok := data["quota_plan"].(map[string]interface{})
			if !assert.True(t, ok, "quota_plan should be an object") {
				return
			}
			assert.Equal(t, "RMB", qp["unit"])
			assert.InDelta(t, tc.quota, qp["quota"], 1e-8)

			id, _ := data["id"].(string)
			t.Cleanup(func() { testutil.DeleteAPIKey(id) })

			qpResp, err := testutil.GetClient().Get("/open-api/v1/api-keys/" + id + "/quota-plan")
			if err != nil {
				t.Fatalf("query quota-plan failed: %v", err)
			}
			testutil.AssertSuccess(t, qpResp)
			var qpData map[string]interface{}
			if err := json.Unmarshal(qpResp.Data, &qpData); err != nil {
				t.Fatalf("unmarshal quota-plan data: %v", err)
			}
			balance, ok := qpData["balance"].(map[string]interface{})
			if !assert.True(t, ok, "balance should be an object") {
				return
			}
			assert.InDelta(t, tc.quota, balance["remaining"], 1e-8)
			assert.InDelta(t, float64(0), balance["used"], 1e-8)
		})
	}

	t.Run("AK-1-016 RMB 配额九位小数", func(t *testing.T) {
		resp := post(t, 100.123456789)
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "decimal places")
	})

	t.Run("AK-1-017 RMB 配额八位小数但超过 9000 万元上限", func(t *testing.T) {
		resp := post(t, 90000000.00000001)
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "must be <=")
	})
}
