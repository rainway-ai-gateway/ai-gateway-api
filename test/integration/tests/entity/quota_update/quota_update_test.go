package entity_test

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

func TestEntity_QuotaUpdate(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	entityName := testutil.UniqueEntityName()
	entityID, err := testutil.CreateEntity(entityName, typeName, "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// 初始化为有限 total_token 配额
	_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+entityID, map[string]interface{}{
		"quota_plan": map[string]interface{}{
			"unlimited":    false,
			"quota":        1000,
			"unit":         "total_token",
			"reset_period": "monthly",
		},
	})
	if err != nil {
		t.Fatalf("setup quota failed: %v", err)
	}

	t.Run("E-9-001 仅修改 quota（单位不变）后余额正确", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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
		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(500), balance["remaining"], 0.00001)
	})

	t.Run("E-9-002 RMB 配额仅修改 quota 后余额正确", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000.1234,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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
		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(800), balance["remaining"], 0.00001)
	})

	t.Run("E-9-003 修改 unit 重置 used 与 remaining", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(888.88), balance["remaining"], 0.00001)
	})

	t.Run("E-9-004 unlimited 由 false 改为 true 返回 sentinel balance", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": true,
			},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(100000000), balance["remaining"], 0.00001)
	})

	t.Run("E-9-005 unlimited 由 true 改为 false 按新 quota 初始化", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited": true,
			},
		})
		if err != nil {
			t.Fatalf("setup unlimited failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(500), balance["remaining"], 0.00001)
	})

	t.Run("E-9-006 普通属性修改不影响配额余额", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"allow_models": []string{"gpt-4"},
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(1000), balance["remaining"], 0.00001)
	})

	t.Run("E-9-007 仅修改 quota（单位不变）时保留非零 used", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		// 构造已使用量：Redis 中剩余 400，即 used = 600
		sm.SetQuotaRemaining(id, 400, "total_token")

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(600), balance["used"], 0.00001)
		assert.InDelta(t, float64(200), balance["remaining"], 0.00001)
	})

	t.Run("E-9-008 RMB 配额仅修改 quota 时保留非零 used", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000.1234,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		// 构造已使用量：Redis 中剩余 400.0000，即 used = 600.1234
		sm.SetQuotaRemaining(id, 400.0000, "RMB")

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(600.1234), balance["used"], 0.00001)
		assert.InDelta(t, float64(199.8766), balance["remaining"], 0.00001)
	})

	t.Run("E-9-009 配额总量修改为 0 后剩余额度清零（回归 issue #136）", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000,
				"unit":         "total_token",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		// 构造已使用量：Redis 中剩余 400，即 used = 600
		sm.SetQuotaRemaining(id, 400, "total_token")

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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

		// remaining = max(0, 0 - 600) = 0
		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(0), balance["remaining"], 0.00001)

		// Redis 余额被同步清零，BFE 将立即拒绝该实体的请求（QuotaExhausted）
		assert.InDelta(t, float64(0), sm.GetQuotaRemaining(id, "total_token"), 0.00001)
	})

	t.Run("E-9-010 RMB 配额总量修改为 0 后剩余额度清零", func(t *testing.T) {
		name := testutil.UniqueEntityName()
		id, err := testutil.CreateEntity(name, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(id)

		_, err = testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        1000.1234,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		if err != nil {
			t.Fatalf("setup quota failed: %v", err)
		}

		// 构造已使用量：Redis 中剩余 400.0000，即 used = 600.1234
		sm.SetQuotaRemaining(id, 400.0000, "RMB")

		resp, err := testutil.GetClient().Patch("/open-api/v1/entities/"+id, map[string]interface{}{
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

		balance := fetchEntityBalance(t, id)
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(0), balance["remaining"], 0.00001)
		assert.InDelta(t, float64(0), sm.GetQuotaRemaining(id, "RMB"), 0.00001)
	})

	t.Cleanup(func() {
		testutil.DeleteEntity(entityID)
		testutil.DeleteEntityType(typeName)
	})
}

func fetchEntityQuotaPlan(t *testing.T, entityID string) map[string]interface{} {
	resp, err := testutil.GetClient().Get("/open-api/v1/entities/" + entityID + "/quota-plan")
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

func fetchEntityBalance(t *testing.T, entityID string) map[string]interface{} {
	data := fetchEntityQuotaPlan(t, entityID)
	balance, ok := data["balance"].(map[string]interface{})
	if !ok {
		t.Fatalf("balance is not an object")
	}
	return balance
}
