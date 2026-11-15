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

package access_log_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const accessLogConfigPath = "/open-api/v1/access-log/config"

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

func getAccessLogConfig(t *testing.T) map[string]interface{} {
	t.Helper()
	resp, err := testutil.GetClient().Get(accessLogConfigPath)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	return data
}

func putAccessLogConfig(t *testing.T, body map[string]interface{}) *testutil.APIResponse {
	t.Helper()
	resp, err := testutil.GetClient().Put(accessLogConfigPath, body)
	require.NoError(t, err)
	return resp
}

// ---------- AL-1-001 未配置：GET 返回空数组（非 404） ----------

func TestAccessLogConfigGet_Unconfigured(t *testing.T) {
	data := getAccessLogConfig(t)
	assert.Equal(t, []interface{}{}, data["req_headers"])
	assert.Equal(t, []interface{}{}, data["res_headers"])
}

// ---------- AL-1-002 PUT 大小写混合 → GET 回显规范化形式 ----------

func TestAccessLogConfigPut_CanonicalizedRoundTrip(t *testing.T) {
	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"business_type", "X-REQUEST-ID"},
		"res_headers": []interface{}{"x-trace-id"},
	})
	testutil.AssertSuccess(t, resp)

	data := getAccessLogConfig(t)
	assert.Equal(t, []interface{}{"Business_type", "X-Request-Id"}, data["req_headers"])
	assert.Equal(t, []interface{}{"X-Trace-Id"}, data["res_headers"])
}

// ---------- AL-1-003 敏感头硬拒绝（422 且不落库） ----------

func TestAccessLogConfigPut_SensitiveHeaderRejected(t *testing.T) {
	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"authorization"},
		"res_headers": []interface{}{"Proxy-Authorization"},
	})
	testutil.AssertErrCode(t, resp, 422)

	// 配置保持不变（仍为上一条用例的合法值）
	data := getAccessLogConfig(t)
	assert.Equal(t, []interface{}{"Business_type", "X-Request-Id"}, data["req_headers"])
}

// ---------- AL-1-004 非法头名/归一重复（422 + 回读零变更，家族3/4） ----------

func TestAccessLogConfigPut_InvalidNameRejected(t *testing.T) {
	// 前置：先落一份合法配置，负向用例拒绝后必须逐字段保持该值（回读零变更）
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"business_type"},
		"res_headers": []interface{}{"x-trace-id"},
	})
	before := getAccessLogConfig(t)

	cases := map[string]interface{}{
		"bad name": []interface{}{"bad name"},     // 非法字符（空格）
		"dup":      []interface{}{"x-id", "X-ID"}, // 归一后重复
		"empty":    []interface{}{""},             // 空名称
	}
	for name, headers := range cases {
		t.Run(name, func(t *testing.T) {
			resp := putAccessLogConfig(t, map[string]interface{}{
				"req_headers": headers,
				"res_headers": []interface{}{},
			})
			testutil.AssertErrCode(t, resp, 422)
			// 错误可归因（家族4）：消息指明字段与原因
			assert.Contains(t, resp.ErrMsg, "req_headers")

			// 回读零变更（家族3）：与被操作前逐字段一致
			after := getAccessLogConfig(t)
			assert.Equal(t, before, after)
		})
	}
}

// ---------- AL-1-005 PUT 空数组：清空白名单 ----------

func TestAccessLogConfigPut_Clear(t *testing.T) {
	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{},
		"res_headers": []interface{}{},
	})
	testutil.AssertSuccess(t, resp)

	data := getAccessLogConfig(t)
	assert.Equal(t, []interface{}{}, data["req_headers"])
	assert.Equal(t, []interface{}{}, data["res_headers"])
}

// ---------- AL-1-006 边界与字符集矩阵（家族4：边界±1 + 字符集白名单 + 正向真实形态） ----------

func TestAccessLogConfigPut_BoundaryAndCharset(t *testing.T) {
	name128 := strings.Repeat("a", 128)
	name129 := strings.Repeat("a", 129)

	// 边界 -1/边界/边界 +1
	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{strings.Repeat("a", 127)},
		"res_headers": []interface{}{},
	})
	testutil.AssertSuccess(t, resp)

	resp = putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{name128},
		"res_headers": []interface{}{},
	})
	testutil.AssertSuccess(t, resp)
	assert.Equal(t, []interface{}{"A" + strings.Repeat("a", 127)}, getAccessLogConfig(t)["req_headers"])

	resp = putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{name129},
		"res_headers": []interface{}{},
	})
	testutil.AssertErrCode(t, resp, 422)

	// token 字符集正向用例（#135 纪律：收紧字符集必须同时补真实形态正向）
	resp = putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-request-id", "x_business.type+v1"},
		"res_headers": []interface{}{},
	})
	testutil.AssertSuccess(t, resp)

	// 字符集负向：非 token 字符逐一拒绝
	for _, bad := range []string{"bad:name", "bad name", "bad	name", "bad[name]"} {
		resp := putAccessLogConfig(t, map[string]interface{}{
			"req_headers": []interface{}{bad},
			"res_headers": []interface{}{},
		})
		testutil.AssertErrCode(t, resp, 422)
		assert.Contains(t, resp.ErrMsg, "req_headers")
	}
}

// ---------- AL-1-007 省略字段 = 清空（全量替换语义锁定，家族1 变体） ----------

func TestAccessLogConfigPut_OmittedFieldClears(t *testing.T) {
	// 前置：两份列表均非空
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"business_type"},
		"res_headers": []interface{}{"x-trace-id"},
	})

	// PUT 只提交 req_headers：res_headers 被清空（合同：非必填字段缺省按 [] 处理，
	// 全量替换无 PATCH 式合并——若实现退化为合并，本用例立即失败）
	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-request-id"},
	})
	testutil.AssertSuccess(t, resp)

	data := getAccessLogConfig(t)
	assert.Equal(t, []interface{}{"X-Request-Id"}, data["req_headers"])
	assert.Equal(t, []interface{}{}, data["res_headers"])
}

// ---------- AL-1-008 防泄漏：被拒配置不得出现在 InnerAPI 导出产物（家族4） ----------

func TestAccessLogConfigPut_RejectedConfigNotExported(t *testing.T) {
	// 前置：合法配置导出基线
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"business_type"},
		"res_headers": []interface{}{},
	})
	exportBefore := exportModAccessPbData(t, "")
	require.NotContains(t, string(exportBefore.RawBody), "evil-header")

	// 提交被拒配置（非法字符）
	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"evil header"},
		"res_headers": []interface{}{},
	})
	testutil.AssertErrCode(t, resp, 422)

	// 导出产物不得包含被拒配置（#172 类缺陷：非法值入库会导致 BFE reload 失败、
	// 阻塞全部下发）；内容未变时导出版本不变、强拉全量比对
	exportAfter := exportModAccessPbData(t, "")
	assert.NotContains(t, string(exportAfter.RawBody), "evil header")
	assert.JSONEq(t, string(exportBefore.RawBody), string(exportAfter.RawBody),
		"rejected PUT must not change the exported payload")
}

// exportModAccessPbData 拉取 InnerAPI 导出，返回原始响应。
func exportModAccessPbData(t *testing.T, version string) *testutil.APIResponse {
	t.Helper()
	var resp *testutil.APIResponse
	var err error
	if version == "" {
		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/mod-access-pb")
	} else {
		resp, err = testutil.GetClient().Get("/inner-api/v1/configs/mod-access-pb", map[string]string{"version": version})
	}
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	return resp
}
