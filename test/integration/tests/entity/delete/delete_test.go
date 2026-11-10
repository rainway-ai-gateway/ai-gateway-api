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
	"os"
	"strconv"
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

func TestEntity_Delete(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	childTypeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := testutil.CreateEntityType(childTypeName, 2); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("E-6-001 删除 Entity", func(t *testing.T) {
		entityName := testutil.UniqueEntityName()
		entityID, err := testutil.CreateEntity(entityName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		resp, err := testutil.GetClient().Delete("/open-api/v1/entities/" + entityID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		resp, _ = testutil.GetClient().Get("/open-api/v1/entities/" + entityID)
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("E-6-002 删除存在子节点的 Entity", func(t *testing.T) {
		parentName := testutil.UniqueEntityName()
		parentID, err := testutil.CreateEntity(parentName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		childName := testutil.UniqueEntityName()
		childID, err := testutil.CreateEntity(childName, childTypeName, parentID)
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		resp, err := testutil.GetClient().Delete("/open-api/v1/entities/" + parentID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 409)
		t.Cleanup(func() {
			testutil.DeleteEntity(childID)
			testutil.DeleteEntity(parentID)
		})
	})

	t.Run("E-6-003 删除被 API-Key 挂载的 Entity", func(t *testing.T) {
		entityName := testutil.UniqueEntityName()
		entityID, err := testutil.CreateEntity(entityName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		apiKeyID, err := testutil.CreateAPIKey(testutil.UniqueAPIKeyDesc(), entityID)
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		resp, err := testutil.GetClient().Delete("/open-api/v1/entities/" + entityID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 409)
		resp, err = testutil.GetClient().Get("/open-api/v1/entities/" + entityID)
		if err != nil {
			t.Fatalf("get after conflict failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		t.Cleanup(func() {
			testutil.DeleteAPIKey(apiKeyID)
			testutil.DeleteEntity(entityID)
		})
	})

	t.Run("E-6-004 删除最大编号 Entity 后新建不复用 ID", func(t *testing.T) {
		entityName := testutil.UniqueEntityName()
		entityID, err := testutil.CreateEntity(entityName, typeName, "")
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		resp, err := testutil.GetClient().Delete("/open-api/v1/entities/" + entityID)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)

		newName := testutil.UniqueEntityName()
		newID, err := testutil.CreateEntity(newName, typeName, "")
		if err != nil {
			t.Fatalf("recreate failed: %v", err)
		}
		if newID == entityID {
			t.Errorf("recreated entity reused deleted id %s", entityID)
		}
		oldSeq, err := strconv.ParseInt(strings.TrimPrefix(entityID, "entity-"), 10, 64)
		if err != nil {
			t.Fatalf("parse old entity id %s: %v", entityID, err)
		}
		newSeq, err := strconv.ParseInt(strings.TrimPrefix(newID, "entity-"), 10, 64)
		if err != nil {
			t.Fatalf("parse new entity id %s: %v", newID, err)
		}
		if newSeq <= oldSeq {
			t.Errorf("new entity seq %d should be greater than deleted seq %d", newSeq, oldSeq)
		}
		t.Cleanup(func() {
			testutil.DeleteEntity(newID)
		})
	})

	t.Cleanup(func() {
		testutil.DeleteEntityType(typeName)
		testutil.DeleteEntityType(childTypeName)
	})
}
