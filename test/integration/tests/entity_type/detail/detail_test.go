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

func TestEntityType_Detail(t *testing.T) {
	typeName := testutil.UniqueEntityTypeName()
	if _, err := testutil.CreateEntityType(typeName, 1); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("ET-3-001 查询单个 Entity-Type", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "type_name", typeName)
		testutil.AssertDataFieldEquals(t, resp, "level", float64(1))
	})

	t.Run("ET-3-002 查询不存在的 Entity-Type", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/non_existent_type")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("ET-3-003 查询非法名（连字符开头）", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/-a")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("ET-3-004 查询非法名（连字符结尾）", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/a-")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("ET-3-005 查询非法名（大写字符）", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/A")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("ET-3-006 非法名请求无副作用", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/entity-types/-a")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
		resp, err = testutil.GetClient().Get("/open-api/v1/entity-types/" + typeName)
		if err != nil {
			t.Fatalf("readback failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
	})

	t.Cleanup(func() {
		testutil.DeleteEntityType(typeName)
	})
}
