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

// 说明：
// 本集成测试使用 miniredis 作为嵌入式 Redis，测试进程可直接写入 Redis，
// 从而完整覆盖“仅修改 quota 时保留 used”的非零 used 路径。

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

func TestAPIKey_QuotaUpdate(t *testing.T) {
	apiKeyID, err := testutil.CreateAPIKey("quota-update-key", "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 初始化为有限 total_token 配额
	testutil.MustPatch(t, "/open-api/v1/api-keys/"+apiKeyID, map[string]interface{}{
		"quota_plan": map[string]interface{}{
			"unlimited":    false,
			"quota":        1000,
			"unit":         "total_token",
			"reset_period": "monthly",
		},
	})
	t.Run("AK-9-001 仅修改 quota（单位不变）后余额正确", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-reset", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     500,
				"unit":      "total_token",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// 当前无使用量，保留 used 语义下 remaining = new_quota - 0 = new_quota。
		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(500), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-002 RMB 配额仅修改 quota 后余额正确", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-rmb-reset", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000.1234,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     800.0000,
				"unit":      "RMB",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// 当前无使用量，保留 used 语义下 remaining = new_quota - 0 = new_quota。
		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(800), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-003 修改 unit 重置 used 与 remaining", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-unit-change", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     888.88,
				"unit":      "RMB",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(888.88), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-004 unlimited 由 false 改为 true 返回 sentinel balance", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-unlimited-true", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": true,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(100000000), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-005 unlimited 由 true 改为 false 按新 quota 初始化", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-unlimited-false", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": true,
			},
		})

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     500,
				"unit":      "total_token",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(500), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-006 普通属性修改不影响配额余额", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-no-impact", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"enabled":     false,
			"description": "updated-desc",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(1000), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-007 仅修改 quota（单位不变）时保留非零 used", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-keep-used", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		// 构造已使用量：Redis 中剩余 400，即 used = 600
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, 400, "total_token")

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     800,
				"unit":      "total_token",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(600), balance["used"], 0.00001)
		assert.InDelta(t, float64(200), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-008 RMB 配额仅修改 quota 时保留非零 used", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-rmb-keep-used", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000.1234,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		// 构造已使用量：Redis 中剩余 400.0000，即 used = 600.1234
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, 400.0000, "RMB")

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     800.0000,
				"unit":      "RMB",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(600.1234), balance["used"], 0.00001)
		assert.InDelta(t, float64(199.8766), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-009 配额总量修改为 0 后剩余额度展示清零、欠费保留在 Redis（回归 issue #136 / #1476）", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-to-zero", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		// 构造已使用量：Redis 中剩余 400，即 used = 600
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, 400, "total_token")

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     0,
				"unit":      "total_token",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// issue #1476：remaining = 0 - 600 = -600 不再钳零，欠费保留在 Redis
		// （负值表示新配额仍不足以覆盖已用量，BFE 鉴权继续拒绝）；对外展示
		// 的 remaining 钳 0，used 如实为 600。
		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(600), balance["used"], 0.00001)
		assert.InDelta(t, float64(0), balance["remaining"], 0.00001)

		assert.InDelta(t, float64(-600), sm.GetQuotaRemaining(apiKeyValue, "total_token"), 0.00001)
	})

	t.Run("AK-9-010 RMB 配额总量修改为 0 后剩余额度展示清零、欠费保留在 Redis", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-update-rmb-to-zero", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000.1234,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		// 构造已使用量：Redis 中剩余 400.0000，即 used = 600.1234
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, 400.0000, "RMB")

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     0,
				"unit":      "RMB",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// issue #1476：同 AK-9-009，负数保留在 Redis（RMB 为 1e-8 定点口径）。
		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(600.1234), balance["used"], 0.00001)
		assert.InDelta(t, float64(0), balance["remaining"], 0.00001)
		assert.InDelta(t, float64(-600.1234), sm.GetQuotaRemaining(apiKeyValue, "RMB"), 0.00001)
	})

	t.Run("AK-9-011 欠费余额（Redis 为负）下 used 超 quota、remaining 展示 0", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-overage-display", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		// issue #1476：BFE 全额扣减后余额可为负——模拟欠费 100，
		// 实际消耗 used = 1000 - (-100) = 1100，允许大于 quota。
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, -100, "total_token")

		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(1100), balance["used"], 0.00001)
		assert.InDelta(t, float64(0), balance["remaining"], 0.00001)
	})

	t.Run("AK-9-012 欠费状态下调大配额仍不足时 remaining 保持为负", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-overage-raise-insufficient", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		// used = 1000 - (-100) = 1100
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, -100, "total_token")

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     1050,
				"unit":      "total_token",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// issue #1476：targetRemaining = 1050 - 1100 = -50 不再钳零，
		// 欠费不会被调额凭空抹掉；used 保持 1100，展示 remaining = 0。
		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(1100), balance["used"], 0.00001)
		assert.InDelta(t, float64(0), balance["remaining"], 0.00001)
		assert.InDelta(t, float64(-50), sm.GetQuotaRemaining(apiKeyValue, "total_token"), 0.00001)
	})

	t.Run("AK-9-013 欠费状态下调大配额覆盖欠费后 remaining 转正", func(t *testing.T) {
		id, err := testutil.CreateAPIKey("quota-overage-raise-cover", "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteAPIKey(id)

		testutil.MustPatch(t, "/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		// used = 1000 - (-100) = 1100
		apiKeyValue := getAPIKeyValue(t, id)
		sm.SetQuotaRemaining(apiKeyValue, -100, "total_token")

		resp, err := testutil.GetClient().Patch("/open-api/v1/api-keys/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": false,
				"quota":     2000,
				"unit":      "total_token",
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		// targetRemaining = 2000 - 1100 = 900，鉴权恢复放行。
		balance := fetchAPIKeyBalance(t, id)
		assert.InDelta(t, float64(1100), balance["used"], 0.00001)
		assert.InDelta(t, float64(900), balance["remaining"], 0.00001)
		assert.InDelta(t, float64(900), sm.GetQuotaRemaining(apiKeyValue, "total_token"), 0.00001)
	})

	t.Cleanup(func() {
		testutil.DeleteAPIKey(apiKeyID)
	})
}

func fetchAPIKeyQuotaPlan(t *testing.T, apiKeyID string) map[string]interface{} {
	resp, err := testutil.GetClient().Get("/open-api/v1/api-keys/" + apiKeyID + "/quota-plan")
	if err != nil {
		t.Fatalf("query quota-plan failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal quota-plan data: %v", err)
	}
	return data
}

func fetchAPIKeyBalance(t *testing.T, apiKeyID string) map[string]interface{} {
	data := fetchAPIKeyQuotaPlan(t, apiKeyID)
	balance, ok := data["balance"].(map[string]interface{})
	if !ok {
		t.Fatalf("balance is not an object")
	}
	return balance
}

func getAPIKeyValue(t *testing.T, apiKeyID string) string {
	resp, err := testutil.GetClient().Get("/open-api/v1/api-keys/" + apiKeyID)
	if err != nil {
		t.Fatalf("get api-key failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)

	key, err := testutil.GetDataField(resp, "key")
	if err != nil {
		t.Fatalf("get api-key value failed: %v", err)
	}
	keyStr, ok := key.(string)
	if !ok {
		t.Fatalf("api-key value is not a string")
	}
	return keyStr
}
