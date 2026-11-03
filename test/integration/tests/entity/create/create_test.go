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
	"strconv"
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

func TestEntity_Create(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	typeName2 := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := testutil.CreateEntityType(typeName2, 2); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	parentName := testutil.UniqueEntityName()
	parentID, err := testutil.CreateEntity(parentName, typeName, "")
	if err != nil {
		t.Fatalf("setup parent failed: %v", err)
	}

	entRoot := testutil.UniqueEntityName()
	entQuota := testutil.UniqueEntityName()
	entDup := testutil.UniqueEntityName()
	entTeam := testutil.UniqueEntityName()
	entBadParent := testutil.UniqueEntityName()

	// 预先创建重复名称
	if _, err := testutil.CreateEntity(entDup, typeName, ""); err != nil {
		t.Fatalf("setup dup failed: %v", err)
	}

	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode int
		check    func(t *testing.T, resp *testutil.APIResponse)
	}{
		{
			name:     "E-1-001 创建 Entity（仅必填）",
			body:     map[string]interface{}{"name": entRoot, "type": typeName},
			wantCode: 200,
		},
		{
			name: "E-1-002 创建 Entity（含 quota_plan）",
			body: map[string]interface{}{
				"name": entQuota,
				"type": typeName,
				"quota_plan": map[string]interface{}{
					"unlimited":    false,
					"quota":        1000000,
					"unit":         "total_token",
					"reset_period": "monthly",
				},
			},
			wantCode: 200,
		},
		{
			name:     "E-1-003 缺少 name",
			body:     map[string]interface{}{"type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-004 缺少 type",
			body:     map[string]interface{}{"name": testutil.UniqueEntityName()},
			wantCode: 422,
		},
		{
			name:     "E-1-005 type 不存在",
			body:     map[string]interface{}{"name": testutil.UniqueEntityName(), "type": "not_exist"},
			wantCode: 422,
		},
		{
			name:     "E-1-006 重复 name",
			body:     map[string]interface{}{"name": entDup, "type": typeName},
			wantCode: 556,
		},
		{
			name: "E-1-007 创建层级 Entity（合法 parent）",
			body: map[string]interface{}{
				"name":      entTeam,
				"type":      typeName2,
				"parent_id": parentID,
			},
			wantCode: 200,
		},
		{
			name: "E-1-008 创建层级 Entity（非法 parent level）",
			body: map[string]interface{}{
				"name":      entBadParent,
				"type":      typeName,
				"parent_id": parentID,
			},
			wantCode: 422,
		},
		{
			name:     "E-1-009 type 格式非法（含大写）",
			body:     map[string]interface{}{"name": testutil.UniqueEntityName(), "type": "BadType"},
			wantCode: 422,
		},
		{
			name:     "E-1-010 Entity name 包含首尾空白",
			body:     map[string]interface{}{"name": " badname ", "type": typeName},
			wantCode: 422,
		},
		{
			name: "E-1-011 创建 Entity 并指定 RMB 配额",
			body: map[string]interface{}{
				"name": testutil.UniqueEntityName(),
				"type": typeName,
				"quota_plan": map[string]interface{}{
					"unlimited":    false,
					"quota":        5555.5555,
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
				assert.InDelta(t, float64(5555.5555), qp["quota"], 0.00001)

				id, _ := data["id"].(string)
				qpResp, err := testutil.GetClient().Get("/open-api/v1/entities/" + id + "/quota-plan")
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
				assert.InDelta(t, float64(5555.5555), balance["remaining"], 0.00001)
				assert.InDelta(t, float64(0), balance["used"], 0.00001)
			},
		},
		{
			name: "E-1-012 创建 Entity 时 RMB 配额超过 9000 万元上限",
			body: map[string]interface{}{
				"name": testutil.UniqueEntityName(),
				"type": typeName,
				"quota_plan": map[string]interface{}{
					"unlimited":    false,
					"quota":        90000000.01,
					"unit":         "RMB",
					"reset_period": "monthly",
				},
			},
			wantCode: 422,
		},
		{
			name:     "E-1-013 创建 Entity name 含大写字母",
			body:     map[string]interface{}{"name": "BadName", "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-014 创建 Entity name 含空格",
			body:     map[string]interface{}{"name": "bad name", "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-015 创建 Entity name 以 - 开头",
			body:     map[string]interface{}{"name": "-badname", "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-016 创建 Entity name 以 _ 结尾",
			body:     map[string]interface{}{"name": "badname_", "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-017 创建 Entity name 长度为 64",
			body:     map[string]interface{}{"name": strings.Repeat("a", 64), "type": typeName},
			wantCode: 200,
		},
		{
			name:     "E-1-018 创建 Entity name 长度为 65",
			body:     map[string]interface{}{"name": strings.Repeat("a", 65), "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-019 创建 Entity name 含 @（用户名@项目名 形式）",
			body:     map[string]interface{}{"name": testutil.UniqueEntityName() + "@default", "type": typeName},
			wantCode: 200,
		},
		{
			name:     "E-1-020 创建 Entity name 以 @ 开头",
			body:     map[string]interface{}{"name": "@badname", "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-021 创建 Entity name 以 @ 结尾",
			body:     map[string]interface{}{"name": "badname@", "type": typeName},
			wantCode: 422,
		},
		{
			name:     "E-1-022 创建 Entity name 含 @ 以外的特殊字符",
			body:     map[string]interface{}{"name": "bad#name", "type": typeName},
			wantCode: 422,
		},
		{
			name: "E-1-023 创建 Entity 携带 description（回读一致）",
			body: map[string]interface{}{
				"name":        testutil.UniqueEntityName(),
				"type":        typeName,
				"description": "运营部，负责线上业务",
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				if err := json.Unmarshal(resp.Data, &data); err != nil {
					t.Fatalf("unmarshal data: %v", err)
				}
				assert.Equal(t, "运营部，负责线上业务", data["description"])
				id, _ := data["id"].(string)
				detail, err := testutil.GetClient().Get("/open-api/v1/entities/" + id)
				if err != nil {
					t.Fatalf("query entity failed: %v", err)
				}
				testutil.AssertSuccess(t, detail)
				testutil.AssertDataFieldEquals(t, detail, "description", "运营部，负责线上业务")
			},
		},
		{
			name: "E-1-024 创建 Entity description 长度为 255",
			body: map[string]interface{}{
				"name":        testutil.UniqueEntityName(),
				"type":        typeName,
				"description": strings.Repeat("a", 255),
			},
			wantCode: 200,
		},
		{
			name: "E-1-025 创建 Entity description 长度为 256",
			body: map[string]interface{}{
				"name":        testutil.UniqueEntityName(),
				"type":        typeName,
				"description": strings.Repeat("a", 256),
			},
			wantCode: 422,
		},
		{
			name: "E-1-026 创建 Entity description 含控制字符",
			body: map[string]interface{}{
				"name":        testutil.UniqueEntityName(),
				"type":        typeName,
				"description": "abc\tdef",
			},
			wantCode: 422,
		},
		{
			name: "E-1-027 创建 Entity 省略 allow_models 默认 [\"*\"]（issue #202）",
			body: map[string]interface{}{
				"name": testutil.UniqueEntityName(),
				"type": typeName,
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				if err := json.Unmarshal(resp.Data, &data); err != nil {
					t.Fatalf("unmarshal data: %v", err)
				}
				assert.Equal(t, []interface{}{"*"}, data["allow_models"])
				assert.Equal(t, []interface{}{}, data["block_models"])
				id, _ := data["id"].(string)
				detail, err := testutil.GetClient().Get("/open-api/v1/entities/" + id)
				if err != nil {
					t.Fatalf("query entity failed: %v", err)
				}
				testutil.AssertSuccess(t, detail)
				testutil.AssertDataFieldEquals(t, detail, "allow_models", []interface{}{"*"})
				testutil.AssertDataFieldEquals(t, detail, "block_models", []interface{}{})
			},
		},
		{
			name: "E-1-028 创建 Entity 显式指定 allow_models 原样回读（issue #202）",
			body: map[string]interface{}{
				"name":         testutil.UniqueEntityName(),
				"type":         typeName,
				"allow_models": []string{"gpt-4"},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				if err := json.Unmarshal(resp.Data, &data); err != nil {
					t.Fatalf("unmarshal data: %v", err)
				}
				assert.Equal(t, []interface{}{"gpt-4"}, data["allow_models"])
				id, _ := data["id"].(string)
				detail, err := testutil.GetClient().Get("/open-api/v1/entities/" + id)
				if err != nil {
					t.Fatalf("query entity failed: %v", err)
				}
				testutil.AssertSuccess(t, detail)
				testutil.AssertDataFieldEquals(t, detail, "allow_models", []interface{}{"gpt-4"})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := testutil.GetClient().Post("/open-api/v1/entities", tt.body)
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
		testutil.DeleteEntity(parentID)
		testutil.DeleteEntityType(typeName)
		testutil.DeleteEntityType(typeName2)
	})
}

func TestEntity_CreateAutoID(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("E-1-101 自动生成 ID 格式为 entity-N", func(t *testing.T) {
		entityName := testutil.UniqueEntityName()
		resp, err := testutil.GetClient().Post("/open-api/v1/entities", map[string]interface{}{
			"name": entityName,
			"type": typeName,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		id, err := testutil.GetDataField(resp, "id")
		if err != nil {
			t.Fatalf("get id field: %v", err)
		}
		idStr, ok := id.(string)
		if !ok || !strings.HasPrefix(idStr, "entity-") {
			t.Fatalf("auto generated id should have entity- prefix, got %v", id)
		}
		seq, err := strconv.ParseInt(strings.TrimPrefix(idStr, "entity-"), 10, 64)
		if err != nil || seq <= 0 {
			t.Errorf("auto generated id should be entity-<positive seq>, got %s", idStr)
		}
		t.Cleanup(func() {
			testutil.DeleteEntity(idStr)
		})
	})

	t.Run("E-1-102 连续创建 Entity ID 单调递增", func(t *testing.T) {
		// 并发正确性由 DAO 层单元测试 TestTEntityIDSeqAllocate_Concurrent 覆盖；
		// 集成测试环境为 SQLite 文件库，多连接并发写会触发 busy 等待，故此处串行验证。
		const count = 5
		prevSeq := int64(0)
		for i := 0; i < count; i++ {
			entityName := testutil.UniqueEntityName()
			resp, err := testutil.GetClient().Post("/open-api/v1/entities", map[string]interface{}{
				"name": entityName,
				"type": typeName,
			})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertSuccess(t, resp)
			id, err := testutil.GetDataField(resp, "id")
			if err != nil {
				t.Fatalf("get id field: %v", err)
			}
			idStr, ok := id.(string)
			if !ok {
				t.Fatalf("id should be a string, got %v", id)
			}
			seq, err := strconv.ParseInt(strings.TrimPrefix(idStr, "entity-"), 10, 64)
			if err != nil {
				t.Fatalf("parse entity id %s: %v", idStr, err)
			}
			if i > 0 && seq <= prevSeq {
				t.Errorf("entity id seq should increase monotonically, prev %d got %d", prevSeq, seq)
			}
			prevSeq = seq
			idCopy := idStr
			t.Cleanup(func() {
				testutil.DeleteEntity(idCopy)
			})
		}
	})

	t.Cleanup(func() {
		testutil.DeleteEntityType(typeName)
	})
}

// TestEntity_CreateRMBQuotaDecimals 覆盖 unit=RMB 时 quota 的小数位判定
// （issue #293）。0.28 / 600.1234 在修复前被 422 拒绝。
func TestEntity_CreateRMBQuotaDecimals(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() { testutil.DeleteEntityType(typeName) })

	post := func(t *testing.T, quota float64) *testutil.APIResponse {
		t.Helper()
		resp, err := testutil.GetClient().Post("/open-api/v1/entities", map[string]interface{}{
			"name": testutil.UniqueEntityName(),
			"type": typeName,
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
		{"E-1-029 Entity RMB 配额两位小数", 0.28},
		{"E-1-030 Entity RMB 配额四位小数", 600.1234},
		{"E-1-031 Entity RMB 配额八位小数", 1000.12345678},
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
			t.Cleanup(func() { testutil.DeleteEntity(id) })

			qpResp, err := testutil.GetClient().Get("/open-api/v1/entities/" + id + "/quota-plan")
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

	t.Run("E-1-032 Entity RMB 配额九位小数", func(t *testing.T) {
		resp := post(t, 100.123456789)
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "decimal places")
	})

	t.Run("E-1-033 Entity RMB 配额八位小数但超过 9000 万元上限", func(t *testing.T) {
		resp := post(t, 90000000.00000001)
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "must be <=")
	})
}
