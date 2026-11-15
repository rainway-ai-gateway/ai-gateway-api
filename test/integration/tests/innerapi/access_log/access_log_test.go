// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package innerapi_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	accessLogConfigOpenPath   = "/open-api/v1/access-log/config"
	accessLogConfigExportPath = "/inner-api/v1/configs/mod-access-pb"
)

// accessLogExportKeys 为 Inner 导出顶层键（与 BFE mod_access_pb.data
// 加载器契约逐字一致；Version 内嵌文件，ai-route 形态）。
var accessLogExportKeys = []string{"Version", "access_header"}

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

type accessLogExportData struct {
	Version      string `json:"Version"`
	AccessHeader struct {
		ReqHeaders []string `json:"req_headers"`
		ResHeaders []string `json:"res_headers"`
	} `json:"access_header"`
}

func putAccessLogConfig(t *testing.T, body map[string]interface{}) {
	t.Helper()
	resp, err := testutil.GetClient().Put(accessLogConfigOpenPath, body)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
}

// exportAccessLogConfig 拉取导出；version 为空表示首拉。返回原始响应（Data 可能为 null）。
func exportAccessLogConfig(t *testing.T, version string) *testutil.APIResponse {
	t.Helper()
	var resp *testutil.APIResponse
	var err error
	if version == "" {
		resp, err = testutil.GetClient().Get(accessLogConfigExportPath)
	} else {
		resp, err = testutil.GetClient().Get(accessLogConfigExportPath, map[string]string{"version": version})
	}
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	return resp
}

// exportAccessLogConfigData 首拉导出（Data 必须非 null）并解析。
func exportAccessLogConfigData(t *testing.T) *accessLogExportData {
	t.Helper()
	resp := exportAccessLogConfig(t, "")
	testutil.AssertDataNotEmpty(t, resp)
	var data accessLogExportData
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	return &data
}

func accessLogExportTopKeys(t *testing.T) []string {
	t.Helper()
	resp := exportAccessLogConfig(t, "")
	testutil.AssertDataNotEmpty(t, resp)
	var top map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &top))
	keys := make([]string, 0, len(top))
	for k := range top {
		keys = append(keys, k)
	}
	return keys
}

// ---------- AL-E-001 首拉：空白名单也导出（Data 非 null，与 intent 语义不同） ----------

func TestAccessLogConfigExport_FirstPullExportsEmptyWhitelists(t *testing.T) {
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{},
		"res_headers": []interface{}{},
	})

	data := exportAccessLogConfigData(t)
	require.NotEmpty(t, data.Version)
	assert.Equal(t, []string{}, data.AccessHeader.ReqHeaders)
	assert.Equal(t, []string{}, data.AccessHeader.ResHeaders)

	// 顶层键精确匹配 BFE 文件契约（禁止幻影键）
	assert.ElementsMatch(t, accessLogExportKeys, accessLogExportTopKeys(t))
}

// ---------- AL-E-002 发布后导出：规范化白名单 + 版本递增 ----------

func TestAccessLogConfigExport_PublishedContent(t *testing.T) {
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"business_type", "X-REQUEST-ID"},
		"res_headers": []interface{}{"x-trace-id"},
	})

	data := exportAccessLogConfigData(t)
	require.NotEmpty(t, data.Version)
	assert.Equal(t, []string{"Business_type", "X-Request-Id"}, data.AccessHeader.ReqHeaders)
	assert.Equal(t, []string{"X-Trace-Id"}, data.AccessHeader.ResHeaders)

	// 版本未变 → Data null（conf-agent 跳过）
	resp := exportAccessLogConfig(t, data.Version)
	testutil.AssertDataNull(t, resp)
}

// ---------- AL-E-003 变更后导出：新 Version；清空后导出空数组 ----------

func TestAccessLogConfigExport_ChangeAndRetract(t *testing.T) {
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"business_type"},
		"res_headers": []interface{}{},
	})
	before := exportAccessLogConfigData(t)

	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-request-id"},
		"res_headers": []interface{}{"x-trace-id"},
	})
	after := exportAccessLogConfigData(t)
	require.NotEqual(t, before.Version, after.Version, "content change must bump the export version")
	assert.Equal(t, []string{"X-Request-Id"}, after.AccessHeader.ReqHeaders)

	// 清空：导出空数组（撤销已发布的白名单）
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{},
		"res_headers": []interface{}{},
	})
	cleared := exportAccessLogConfigData(t)
	require.NotEqual(t, after.Version, cleared.Version)
	assert.Equal(t, []string{}, cleared.AccessHeader.ReqHeaders)
	assert.Equal(t, []string{}, cleared.AccessHeader.ResHeaders)
}

// ---------- AL-E-004 同秒连续两次变更：导出版本严格递增（家族8/#142） ----------

func TestAccessLogConfigExport_SameSecondVersionMonotonic(t *testing.T) {
	// 两次变更与两次导出全部紧接执行，刻意落在同一墙钟秒内；
	// +1s 抬升机制保证版本号仍严格单调递增
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-monotonic-a"},
		"res_headers": []interface{}{},
	})
	v1 := exportAccessLogConfigData(t).Version

	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-monotonic-b"},
		"res_headers": []interface{}{},
	})
	v2 := exportAccessLogConfigData(t).Version

	require.NotEqual(t, v1, v2, "content changed within the same second must still bump the export version (issue #142)")
	assert.Greater(t, v2, v1, "export version must be strictly increasing (fixed-width timestamp comparison)")
}

// ---------- AL-E-005 伪造 version 参数：按首次拉取返回全量 ----------

func TestAccessLogConfigExport_BogusVersionReturnsFull(t *testing.T) {
	current := exportAccessLogConfigData(t)

	// 伪造/过期 version 不等于当前签名版本 → 返回全量（增量同步协议：未识别版本按首拉处理）
	resp := exportAccessLogConfig(t, "00000000000000")
	testutil.AssertDataNotEmpty(t, resp)

	var data accessLogExportData
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	assert.Equal(t, current.Version, data.Version)
}
