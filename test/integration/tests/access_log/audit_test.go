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

// AL-1-009/010：操作审计断言（ai-gateway-api-integration-test-design skill 家族7）。
// 单例资源的审计身份固定（resource_id/resource_name 恒为 access_log_conf），
// 不依赖请求体（#155）；change_summary 键集合与 diff_keys 必须精确匹配（#201）。
package access_log_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const accessLogAuditResourceID = "access_log_conf"

// waitForAccessLogAudit 轮询操作日志，返回匹配 match 的首条记录。
func waitForAccessLogAudit(t *testing.T, startTs int64, match func(*testutil.OperationLogEntry) bool) *testutil.OperationLogEntry {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var seen []string
	for time.Now().Before(deadline) {
		result, _, err := testutil.QueryOperationLogs(map[string]string{
			"resource_type": "access_log_conf",
			"action":        "update",
			"start_time":    fmt.Sprintf("%d", startTs),
			"page_size":     "100",
		})
		require.NoError(t, err)
		seen = seen[:0]
		for i := range result.List {
			seen = append(seen, fmt.Sprintf("status=%v err=%q", result.List[i].Status, result.List[i].ErrorMsg))
			if match(&result.List[i]) {
				return &result.List[i]
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("operation log not found after 15s (since=%d), seen=%v", startTs, seen)
	return nil
}

// ---------- AL-1-009 成功审计：身份/状态/键集合/diff 精确（家族7/#201） ----------

func TestAccessLogConfigPut_SuccessAudit(t *testing.T) {
	// 前置：先落一行，保证被审计 PUT 的 before 快照存在
	// （BuildChangeSummary 在 before 为空时省略 before 键，键集合断言需确定性）
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-audit-pre"},
		"res_headers": []interface{}{},
	})

	startTs := time.Now().Unix()
	marker := testutil.UniqueName("al-1-009")

	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{marker},
		"res_headers": []interface{}{"x-trace-id"},
	})

	entry := waitForAccessLogAudit(t, startTs, func(e *testutil.OperationLogEntry) bool {
		after, ok := e.ChangeSummary["after"].(map[string]interface{})
		if !ok {
			return false
		}
		raw, _ := json.Marshal(after)
		// 审计快照为规范化形式（首字母大写），匹配忽略大小写
		return strings.Contains(strings.ToLower(string(raw)), strings.ToLower(marker))
	})
	require.NotNil(t, entry)

	assert.Equal(t, "update", entry.Action)
	assert.Equal(t, "access_log_conf", entry.ResourceType)
	// 单例身份固定取自存储快照语义，不依赖请求体（#155）
	assert.Equal(t, accessLogAuditResourceID, entry.ResourceID)
	assert.Equal(t, accessLogAuditResourceID, entry.ResourceName)
	assert.Empty(t, entry.ResourceParentID)
	assert.Equal(t, float64(1), entry.Status)
	assert.Empty(t, entry.ErrorMsg)

	// change_summary 键集合精确为 before/after/diff_keys（#201：禁止 Contains）
	cs := entry.ChangeSummary
	require.NotNil(t, cs)
	assert.ElementsMatch(t, []string{"before", "after", "diff_keys"}, keysOfAL(cs))

	// diff_keys 精确匹配语义键（家族7/#201）：updated_at 仅在跨秒落库时才出现
	// （SQLite CURRENT_TIMESTAMP 为秒级精度，同秒 PUT 不触发时间戳变化），
	// 属于时序噪声而非语义 diff，断言前剔除。
	diffKeys, ok := cs["diff_keys"].([]interface{})
	require.True(t, ok, "diff_keys should be an array, got %v", cs["diff_keys"])
	gotDiff := make([]string, 0, len(diffKeys))
	for _, d := range diffKeys {
		if d.(string) != "updated_at" {
			gotDiff = append(gotDiff, d.(string))
		}
	}
	assert.ElementsMatch(t, []string{"req_headers", "res_headers"}, gotDiff,
		"diff_keys must match exactly on the semantic keys (家族7/#201)")

	// after 快照为 API 词汇（小写键），值为服务端归一形态（首字母大写）
	after, ok := cs["after"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{canonicalHeaderName(marker)}, after["req_headers"])

	// 快照 JSON 序列化后不含内部 "id" 键
	raw, err := json.Marshal(cs)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"id"`, "audit snapshot must not leak internal id")
}

// ---------- AL-1-010 失败审计：4xx 也记账，身份不依赖请求体（家族7/#155） ----------

func TestAccessLogConfigPut_FailureAudit(t *testing.T) {
	// 前置：先落一行（fresh DB 单独跑本用例时保证 before 键存在）
	putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"x-audit-pre"},
		"res_headers": []interface{}{},
	})

	startTs := time.Now().Unix()

	resp := putAccessLogConfig(t, map[string]interface{}{
		"req_headers": []interface{}{"bad name"},
		"res_headers": []interface{}{},
	})
	testutil.AssertErrCode(t, resp, 422)

	entry := waitForAccessLogAudit(t, startTs, func(e *testutil.OperationLogEntry) bool {
		return e.Status == 2 && strings.Contains(e.ErrorMsg, "req_headers")
	})
	require.NotNil(t, entry)

	assert.Equal(t, "update", entry.Action)
	assert.Equal(t, "access_log_conf", entry.ResourceType)
	assert.Equal(t, accessLogAuditResourceID, entry.ResourceID, "identity must come from the storage snapshot, not the request body")
	assert.Equal(t, accessLogAuditResourceID, entry.ResourceName)
	assert.Equal(t, float64(2), entry.Status)
	// 错误可归因：消息指明字段与原因
	assert.Contains(t, entry.ErrorMsg, "req_headers")

	// 失败审计也带 before/after/diff_keys 结构（after = 被拒提交的文档快照）
	cs := entry.ChangeSummary
	require.NotNil(t, cs)
	assert.ElementsMatch(t, []string{"before", "after", "diff_keys"}, keysOfAL(cs))
}

func keysOfAL(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// canonicalHeaderName 计算服务端归一形态：HTTP 规范化键——每个连字符段的首字母
// 大写、其余小写（textproto.CanonicalMIMEHeaderKey）。测试 marker 均为小写 token
// 字符，逐段首字母大写即可。
func canonicalHeaderName(s string) string {
	parts := strings.Split(s, "-")
	for i, part := range parts {
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "-")
}
