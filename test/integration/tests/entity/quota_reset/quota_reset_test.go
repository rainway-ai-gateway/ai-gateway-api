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

package entity_test

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

func TestEntity_QuotaReset(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	entityName := testutil.UniqueEntityName()
	entityID, err := testutil.CreateEntity(entityName, typeName, "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	testutil.MustPatch(t, "/open-api/v1/entities/"+entityID, map[string]interface{}{
		"quota_plan": map[string]interface{}{
			"unlimited":    false,
			"quota":        1000000,
			"unit":         "total_token",
			"reset_period": "monthly",
		},
	})
	t.Run("E-8-001 重置配额余额", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/entities/"+entityID+"/quota-plan/reset", map[string]interface{}{
			"reason": "test reset",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "id", entityID)
	})

	t.Run("E-8-002 重置并修改 quota", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/entities/"+entityID+"/quota-plan/reset", map[string]interface{}{
			"quota":  200000,
			"reason": "reset",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "new_quota", float64(200000))
	})

	t.Run("E-8-003 重置 RMB 配额余额", func(t *testing.T) {
		rmbEntityName := testutil.UniqueEntityName()
		rmbEntityID, err := testutil.CreateEntity(rmbEntityName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(rmbEntityID)

		testutil.MustPatch(t, "/open-api/v1/entities/"+rmbEntityID, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        50.5,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Post("/open-api/v1/entities/"+rmbEntityID+"/quota-plan/reset", map[string]interface{}{
			"quota":  300.1234,
			"reason": "adjust entity rmb quota",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal data: %v", err)
		}
		assert.InDelta(t, float64(50.5), data["previous_quota"], 0.00001)
		assert.InDelta(t, float64(300.1234), data["new_quota"], 0.00001)
		balance := data["balance"].(map[string]interface{})
		assert.InDelta(t, float64(0), balance["used"], 0.00001)
		assert.InDelta(t, float64(300.1234), balance["new_remaining"], 0.00001)
	})

	t.Run("E-8-004 重置 RMB 配额超过 9000 万元上限", func(t *testing.T) {
		rmbEntityName := testutil.UniqueEntityName()
		rmbEntityID, err := testutil.CreateEntity(rmbEntityName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(rmbEntityID)

		testutil.MustPatch(t, "/open-api/v1/entities/"+rmbEntityID, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        50.5,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})
		resp, err := testutil.GetClient().Post("/open-api/v1/entities/"+rmbEntityID+"/quota-plan/reset", map[string]interface{}{
			"quota":  90000000.01,
			"reason": "exceed rmb quota limit",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("E-8-005 重置 RMB 配额八位小数", func(t *testing.T) {
		rmbEntityID, err := testutil.CreateEntity(testutil.UniqueEntityName(), typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(rmbEntityID)

		// 前置配额 0.28 元：修复前该值本身即被 422 拒绝（issue #293）。
		testutil.MustPatch(t, "/open-api/v1/entities/"+rmbEntityID, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        0.28,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})

		resp, err := testutil.GetClient().Post("/open-api/v1/entities/"+rmbEntityID+"/quota-plan/reset", map[string]interface{}{
			"quota":  1000.12345678,
			"reason": "8-decimal rmb quota",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			t.Fatalf("unmarshal data: %v", err)
		}
		assert.InDelta(t, float64(0.28), data["previous_quota"], 1e-8)
		assert.InDelta(t, 1000.12345678, data["new_quota"], 1e-8)
		balance := data["balance"].(map[string]interface{})
		assert.InDelta(t, float64(0), balance["used"], 1e-8)
		assert.InDelta(t, 1000.12345678, balance["new_remaining"], 1e-8)
	})

	t.Run("E-8-006 重置 RMB 配额九位小数", func(t *testing.T) {
		rmbEntityID, err := testutil.CreateEntity(testutil.UniqueEntityName(), typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		defer testutil.DeleteEntity(rmbEntityID)

		testutil.MustPatch(t, "/open-api/v1/entities/"+rmbEntityID, map[string]interface{}{
			"quota_plan": map[string]interface{}{
				"unlimited":    false,
				"quota":        200.8888,
				"unit":         "RMB",
				"reset_period": "monthly",
			},
		})

		resp, err := testutil.GetClient().Post("/open-api/v1/entities/"+rmbEntityID+"/quota-plan/reset", map[string]interface{}{
			"quota":  100.123456789,
			"reason": "9-decimal rmb quota",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Cleanup(func() {
		testutil.DeleteEntity(entityID)
		testutil.DeleteEntityType(typeName)
	})
}
