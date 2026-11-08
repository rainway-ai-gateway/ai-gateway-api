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

package entity_type_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
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

func TestEntityType_Update(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("ET-4-001 更新 Entity-Type 描述", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": "更新后的描述",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "type_name", typeName)
		testutil.AssertDataFieldEquals(t, resp, "description", "更新后的描述")
		testutil.AssertDataFieldEquals(t, resp, "level", float64(1))
	})

	t.Run("ET-4-002 更新后查询一致性", func(t *testing.T) {
		_, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": "一致性校验描述",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", "一致性校验描述")
	})

	t.Run("ET-4-003 更新不存在的 Entity-Type", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/non_existent_type", map[string]interface{}{
			"description": "x",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("ET-4-004 更新 description 超过最大长度", func(t *testing.T) {
		// entity-types.md §1：0-255 字节。恰好 256 是契约边界（必须拒绝），
		// 300 为"远超限"臂，保留既有覆盖。
		for _, n := range []int{256, 300} {
			resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
				"description": strings.Repeat("a", n),
			})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.ErrNum != 422 {
				t.Errorf("len=%d: expected ErrNum=422, got ErrNum=%d, ErrMsg=%s", n, resp.ErrNum, resp.ErrMsg)
			}
		}
	})

	t.Run("ET-4-005 更新 description 恰好 255 字节（合法边界）", func(t *testing.T) {
		desc := strings.Repeat("b", 255)
		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": desc,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", desc)

		got, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "description", desc)
	})

	t.Run("ET-4-006 更新 description 含控制字符被拒绝且旧值保留", func(t *testing.T) {
		baseline := "控制字符基线"
		if _, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": baseline,
		}); err != nil {
			t.Fatalf("request failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": "abc\x01def",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		got, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "description", baseline)
	})

	t.Run("ET-4-007 非法 description 不污染存量值（原子性）", func(t *testing.T) {
		baseline := "原子性基线"
		if _, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": baseline,
		}); err != nil {
			t.Fatalf("request failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"description": strings.Repeat("c", 256),
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		got, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "description", baseline)
	})

	t.Run("ET-4-008 越界 level 被拒绝且原值保留", func(t *testing.T) {
		// issue #313：更新路径此前缺少创建接口已有的 1-5 校验，level=9 曾
		// 成功落库导致类型实质不可用。
		for _, level := range []int{0, -1, 6, 9} {
			resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
				"level": level,
			})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertErrCode(t, resp, 422)
		}

		got, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "level", float64(1))
	})

	t.Run("ET-4-009 合法 level 修改且其余字段不变", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"level": 3,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "level", float64(3))

		got, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "level", float64(3))
		testutil.AssertDataFieldEquals(t, got, "type_name", typeName)
		testutil.AssertDataFieldNotEmpty(t, got, "create_time")

		// 还原，避免影响其他子测试对该类型的复用。
		if _, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+typeName, map[string]interface{}{
			"level": 1,
		}); err != nil {
			t.Fatalf("restore failed: %v", err)
		}
	})

	t.Cleanup(func() {
		testutil.DeleteEntityType(typeName)
	})
}

// TestEntityType_Update_LevelConsistency 验证修改 level 破坏既有 Entity
// 父子层级约束时返回 409 且不做任何修改（issue #313，产品裁定：拒绝、不级联）。
func TestEntityType_Update_LevelConsistency(t *testing.T) {
	parentType := testutil.UniqueEntityTypeName()
	childType := testutil.UniqueEntityTypeName()

	if _, err := testutil.CreateEntityType(parentType, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := testutil.CreateEntityType(childType, 2); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	parentID, err := testutil.CreateEntity(testutil.UniqueEntityName(), parentType, "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	childID, err := testutil.CreateEntity(testutil.UniqueEntityName(), childType, parentID)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Cleanup(func() {
		testutil.DeleteEntity(childID)
		testutil.DeleteEntity(parentID)
		testutil.DeleteEntityType(childType)
		testutil.DeleteEntityType(parentType)
	})

	t.Run("子类型 level 降到父类型同级必须 409 且关系不变", func(t *testing.T) {
		// childType 2 -> 1：父类型 level 1，违反"父 level 必须小于子 level"。
		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+childType, map[string]interface{}{
			"level": 1,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 409)

		got, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + childType)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, got)
		testutil.AssertDataFieldEquals(t, got, "level", float64(2))

		child, err := testutil.GetClient().Get("/open-api/v1/entities/" + childID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, child)
		testutil.AssertDataFieldEquals(t, child, "parent_id", parentID)
	})

	t.Run("解除冲突后同一修改成功", func(t *testing.T) {
		if err := testutil.DeleteEntity(childID); err != nil {
			t.Fatalf("cleanup child failed: %v", err)
		}

		resp, err := testutil.GetClient().Patch("/open-api/v1/entity-types/"+childType, map[string]interface{}{
			"level": 1,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "level", float64(1))
	})
}
