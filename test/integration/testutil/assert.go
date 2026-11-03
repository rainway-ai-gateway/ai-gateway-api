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

package testutil

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertSuccess 验证 API 响应成功（ErrNum=200）
func AssertSuccess(t *testing.T, resp *APIResponse) {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.ErrNum != 200 {
		t.Errorf("expected ErrNum=200, got ErrNum=%d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
	}
}

// AssertErrCode 验证 API 响应错误码
func AssertErrCode(t *testing.T, resp *APIResponse, expectedErrNum int) {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.ErrNum != expectedErrNum {
		t.Errorf("expected ErrNum=%d, got ErrNum=%d, ErrMsg=%s", expectedErrNum, resp.ErrNum, resp.ErrMsg)
	}
}

// AssertDataNull 验证 Data 为 null
func AssertDataNull(t *testing.T, resp *APIResponse) {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if len(resp.Data) != 0 && string(resp.Data) != "null" {
		t.Errorf("expected Data null, got %s", string(resp.Data))
	}
}

// AssertDataNotEmpty 验证 Data 不为空
func AssertDataNotEmpty(t *testing.T, resp *APIResponse) {
	t.Helper()
	if resp == nil {
		t.Fatal("response is nil")
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		t.Error("expected Data not empty, got empty/null")
	}
}

// AssertDataFieldEquals 验证 Data 中指定字段的值
func AssertDataFieldEquals(t *testing.T, resp *APIResponse, field string, expected interface{}) {
	t.Helper()
	data, err := parseData(resp)
	if err != nil {
		t.Fatalf("parse data: %v", err)
	}

	actual, ok := data[field]
	if !ok {
		t.Errorf("field %s not found in Data", field)
		return
	}

	// 对 JSON 数字类型做特殊处理（json.Unmarshal 将数字解析为 float64）
	if expectedFloat, ok := expected.(float64); ok {
		if actualFloat, ok := actual.(float64); ok {
			assert.InDelta(t, expectedFloat, actualFloat, 0.001, "field %s", field)
			return
		}
	}
	if expectedInt, ok := expected.(int64); ok {
		if actualFloat, ok := actual.(float64); ok {
			assert.InDelta(t, float64(expectedInt), actualFloat, 0.001, "field %s", field)
			return
		}
	}

	assert.Equal(t, expected, actual, "field %s", field)
}

// AssertDataFieldNotEmpty 验证 Data 中指定字段不为空
func AssertDataFieldNotEmpty(t *testing.T, resp *APIResponse, field string) {
	t.Helper()
	data, err := parseData(resp)
	if err != nil {
		t.Fatalf("parse data: %v", err)
	}

	actual, ok := data[field]
	if !ok {
		t.Errorf("field %s not found in Data", field)
		return
	}

	if actual == nil {
		t.Errorf("field %s is nil", field)
		return
	}

	switch v := actual.(type) {
	case string:
		if v == "" {
			t.Errorf("field %s is empty string", field)
		}
	case []interface{}:
		if len(v) == 0 {
			t.Errorf("field %s is empty array", field)
		}
	case map[string]interface{}:
		if len(v) == 0 {
			t.Errorf("field %s is empty object", field)
		}
	}
}

// AssertListLen 验证 Data 中的列表长度
func AssertListLen(t *testing.T, resp *APIResponse, expectedLen int) {
	t.Helper()
	list, err := GetDataList(resp)
	if err != nil {
		t.Fatalf("parse data list: %v", err)
	}
	if len(list) != expectedLen {
		t.Errorf("expected list length=%d, got %d", expectedLen, len(list))
	}
}

// AssertPagination 验证分页信息
func AssertPagination(t *testing.T, resp *APIResponse, expectedPage, expectedPageSize, minTotal int) {
	t.Helper()
	data, err := parseData(resp)
	if err != nil {
		t.Fatalf("parse data: %v", err)
	}

	pagination, ok := data["pagination"]
	if !ok {
		t.Error("pagination not found in Data")
		return
	}

	pag, ok := pagination.(map[string]interface{})
	if !ok {
		t.Error("pagination is not an object")
		return
	}

	if page, ok := pag["page"].(float64); ok {
		assert.InDelta(t, float64(expectedPage), page, 0.001, "page")
	}
	if pageSize, ok := pag["page_size"].(float64); ok {
		assert.InDelta(t, float64(expectedPageSize), pageSize, 0.001, "page_size")
	}
	if total, ok := pag["total"].(float64); ok {
		if int(total) < minTotal {
			t.Errorf("expected total >= %d, got %d", minTotal, int(total))
		}
	}
}

// AssertListFieldLen 验证 Data 中 list 字段的列表长度
func AssertListFieldLen(t *testing.T, resp *APIResponse, field string, expectedLen int) {
	t.Helper()
	list, err := GetDataListField(resp, field)
	if err != nil {
		t.Fatalf("parse data list field: %v", err)
	}
	if len(list) != expectedLen {
		t.Errorf("expected list field %s length=%d, got %d", field, expectedLen, len(list))
	}
}

// MustPatch 发送 PATCH 并断言业务成功，用于测试前置配置。
// 前置写入只判 err 会静默吞掉校验失败（issue #293），让后续断言以误导性形式失败。
func MustPatch(t *testing.T, path string, body interface{}) *APIResponse {
	t.Helper()
	resp, err := GetClient().Patch(path, body)
	if err != nil {
		t.Fatalf("PATCH %s failed: %v", path, err)
	}
	AssertSuccess(t, resp)
	return resp
}

// LeaksDriverError reports whether an API error message carries database driver
// details (issue #295 / F-14: 500 responses must not echo SQL, DSN or driver
// errors).
func LeaksDriverError(msg string) bool {
	for _, s := range []string{"Database Exception", "Error 1213", "Error 1205", "Deadlock", "Lock wait timeout"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// parseData 将 Data 解析为 map
func parseData(resp *APIResponse) (map[string]interface{}, error) {
	if resp == nil {
		return nil, fmt.Errorf("response is nil")
	}
	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}
	return data, nil
}