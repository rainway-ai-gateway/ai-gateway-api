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

func TestAPIKey_List(t *testing.T) {
	id1, err := testutil.CreateAPIKey("enabled-true-key", "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err = testutil.GetClient().Patch("/open-api/v1/api-keys/"+id1, map[string]interface{}{
		"quota_plan": map[string]interface{}{"unlimited": false, "quota": 1000000, "unit": "total_token", "reset_period": "monthly"},
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	id2, err := testutil.CreateAPIKey("enabled-false-key", "")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err = testutil.GetClient().Patch("/open-api/v1/api-keys/"+id2, map[string]interface{}{"enabled": false})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("AK-2-001 列表返回包含 balance", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/api-keys", map[string]string{"page": "1", "page_size": "20"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		list := data["list"].([]interface{})
		assert.GreaterOrEqual(t, len(list), 2)
		found := false
		for _, item := range list {
			apiKey := item.(map[string]interface{})
			qp := apiKey["quota_plan"].(map[string]interface{})
			if unlimited, ok := qp["unlimited"].(bool); ok && !unlimited {
				assert.Contains(t, qp, "balance")
				found = true
			}
		}
		if !found {
			t.Skip("no non-unlimited api-key found with balance")
		}
	})

	t.Run("AK-2-002 列表分页参数", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/api-keys", map[string]string{"page": "1", "page_size": "1"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertListFieldLen(t, resp, "list", 1)
		testutil.AssertPagination(t, resp, 1, 1, 2)
	})

	t.Run("AK-2-003 按 enabled 过滤", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/api-keys", map[string]string{"enabled": "false"})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		json.Unmarshal(resp.Data, &data)
		for _, item := range data["list"].([]interface{}) {
			assert.Equal(t, false, item.(map[string]interface{})["enabled"])
		}
	})

	t.Run("AK-2-004 无分页参数时的默认分页", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/api-keys")
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
	})

	t.Run("AK-2-005 相邻页不重复", func(t *testing.T) {
		ids := make([]string, 0, 2)
		for _, page := range []string{"1", "2"} {
			resp, err := testutil.GetClient().Get("/open-api/v1/api-keys", map[string]string{"page": page, "page_size": "1"})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertSuccess(t, resp)
			testutil.AssertListFieldLen(t, resp, "list", 1)

			list, err := testutil.GetDataListField(resp, "list")
			if err != nil {
				t.Fatalf("parse list: %v", err)
			}
			id, _ := list[0].(map[string]interface{})["id"].(string)
			ids = append(ids, id)
		}
		assert.NotEqual(t, ids[0], ids[1], "adjacent pages must not repeat the same object")
	})

	t.Run("AK-2-006 非法分页值仍返回 422", func(t *testing.T) {
		for _, tc := range []struct {
			query   map[string]string
			wantMsg string
		}{
			{map[string]string{"page": "0"}, "page must be > 0"},
			{map[string]string{"page": "-1"}, "page must be > 0"},
			{map[string]string{"page_size": "0"}, "page_size must be between 1 and 100"},
			{map[string]string{"page_size": "101"}, "page_size must be between 1 and 100"},
		} {
			resp, err := testutil.GetClient().Get("/open-api/v1/api-keys", tc.query)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			assert.Equal(t, 422, resp.ErrNum, "query=%v", tc.query)
			assert.Contains(t, resp.ErrMsg, tc.wantMsg, "query=%v", tc.query)
		}
	})

	t.Run("AK-2-007 过滤条件下 total 与 list 一致", func(t *testing.T) {
		for _, tc := range []struct {
			enabled string
			want    int
		}{
			{"false", 1},
			{"true", 1},
		} {
			resp, err := testutil.GetClient().Get("/open-api/v1/api-keys", map[string]string{"enabled": tc.enabled})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertSuccess(t, resp)
			// 默认分页仍生效（page=1/page_size=20），total 仅统计过滤后的记录（issue #303）。
			testutil.AssertPagination(t, resp, 1, 20, tc.want)

			list, err := testutil.GetDataListField(resp, "list")
			if err != nil {
				t.Fatalf("parse list: %v", err)
			}
			assert.Len(t, list, tc.want, "enabled=%s", tc.enabled)
		}
	})

	t.Cleanup(func() {
		testutil.DeleteAPIKey(id1)
		testutil.DeleteAPIKey(id2)
	})
}
