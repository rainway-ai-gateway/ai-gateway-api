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

func TestEntityType_List(t *testing.T) {
	typeName1 := testutil.UniqueEntityTypeName()
	typeName2 := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName1, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := testutil.CreateEntityType(typeName2, 2); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("ET-2-001 查询 Entity-Type 列表", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types", map[string]string{
			"page":      "1",
			"page_size": "20",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertListFieldLen(t, resp, "list", 2)
		testutil.AssertPagination(t, resp, 1, 20, 2)
	})

	t.Run("ET-2-002 分页参数边界", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types", map[string]string{
			"page":      "1",
			"page_size": "1",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertListFieldLen(t, resp, "list", 1)
		testutil.AssertPagination(t, resp, 1, 1, 2)
	})

	t.Run("ET-2-003 无分页参数时返回全部匹配记录", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 2, 2) // page_size == total

		list, err := testutil.GetDataListField(resp, "list")
		if err != nil {
			t.Fatalf("parse list: %v", err)
		}
		pagination, err := testutil.GetDataField(resp, "pagination")
		if err != nil {
			t.Fatalf("parse pagination: %v", err)
		}
		pag := pagination.(map[string]interface{})
		total := int(pag["total"].(float64))
		assert.Equal(t, total, len(list), "no-parameter request must return the whole list")
		assert.Equal(t, total, int(pag["page_size"].(float64)), "page_size must equal total in no-parameter mode")
	})

	t.Run("ET-2-004 过滤 + 无分页参数返回全部匹配", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types", map[string]string{"type_name": typeName1})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 1, 1)

		list, err := testutil.GetDataListField(resp, "list")
		if err != nil {
			t.Fatalf("parse list: %v", err)
		}
		assert.Len(t, list, 1)
	})

	t.Run("ET-2-005 无参模式不受 20 条上限截断", func(t *testing.T) {
		const extra = 25
		for i := 0; i < extra; i++ {
			name := testutil.UniqueEntityTypeName()
			if _, err := testutil.CreateEntityType(name, 1); err != nil {
				t.Fatalf("setup entity-type %d failed: %v", i, err)
			}
			t.Cleanup(func() { testutil.DeleteEntityType(name) })
		}

		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		list, err := testutil.GetDataListField(resp, "list")
		if err != nil {
			t.Fatalf("parse list: %v", err)
		}
		pagination, err := testutil.GetDataField(resp, "pagination")
		if err != nil {
			t.Fatalf("parse pagination: %v", err)
		}
		pag := pagination.(map[string]interface{})
		total := int(pag["total"].(float64))

		assert.GreaterOrEqual(t, total, extra, "fixtures must reach the previous 20-item cap")
		assert.Equal(t, total, len(list), "no-parameter request must not be truncated")
		assert.Equal(t, total, int(pag["page_size"].(float64)))

		// 显式分页仍生效。
		resp, err = testutil.GetClient().Get("/open-api/v1/entity-types", map[string]string{"page": "2", "page_size": "10"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 2, 10, total)
		testutil.AssertListFieldLen(t, resp, "list", 10)
	})

	t.Run("ET-2-006 显式分页的 clamp 语义保持", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types", map[string]string{"page_size": "500"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 100, 2)

		resp, err = testutil.GetClient().Get("/open-api/v1/entity-types", map[string]string{"page": "-1"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 20, 2)
	})

	t.Cleanup(func() {
		testutil.DeleteEntityType(typeName1)
		testutil.DeleteEntityType(typeName2)
	})
}
