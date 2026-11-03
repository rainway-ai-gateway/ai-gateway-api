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

	"github.com/stretchr/testify/assert"
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

func TestEntity_List(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ent1 := testutil.UniqueEntityName()
	ent2 := testutil.UniqueEntityName()
	id1, err := testutil.CreateEntity(ent1, typeName, "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err = testutil.CreateEntity(ent2, typeName, "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("E-2-001 Entity 列表分页", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entities", map[string]string{"page": "1", "page_size": "10"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 10, 2)
	})

	t.Run("E-2-002 按 type 过滤", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entities", map[string]string{"type": typeName})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		for _, item := range data["list"].([]interface{}) {
			assert.Equal(t, typeName, item.(map[string]interface{})["type"])
		}
	})

	t.Run("E-2-003 分页参数边界", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entities", map[string]string{"page": "1", "page_size": "1"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertListFieldLen(t, resp, "list", 1)
		testutil.AssertPagination(t, resp, 1, 1, 2)
	})

	t.Run("E-2-004 无分页参数时的默认分页与 clamp", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entities")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 20, 2)

		list, err := testutil.GetDataListField(resp, "list")
		if err != nil {
			t.Fatalf("parse list: %v", err)
		}
		assert.LessOrEqual(t, len(list), 20, "default page_size must truncate the list")

		// clamp 语义：page_size>100 → 100，page<=0 → 1（不返回 422）。
		resp, err = testutil.GetClient().Get("/open-api/v1/entities", map[string]string{"page_size": "500"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 100, 2)

		resp, err = testutil.GetClient().Get("/open-api/v1/entities", map[string]string{"page": "-1"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 20, 2)
	})

	t.Run("E-2-005 过滤条件下 total 与过滤一致", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entities", map[string]string{"name": ent1})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertPagination(t, resp, 1, 20, 1)

		list, err := testutil.GetDataListField(resp, "list")
		if err != nil {
			t.Fatalf("parse list: %v", err)
		}
		assert.Len(t, list, 1)
	})

	t.Cleanup(func() {
		testutil.DeleteEntity(id1)
		testutil.DeleteEntityType(typeName)
	})
}
