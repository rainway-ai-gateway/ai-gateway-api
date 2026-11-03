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

//go:build mysql

package ai_context_update_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAIContextRules_Update_ConcurrentMySQL（CTX-1-005，家族10）在 MySQL 后端下
// 并发执行全量替换 PUT。回归目标：delete-all+insert-all 的单事务路径在并发
// 下不得出现 5xx/deadlock/部分写入——所有请求必须 2xx，且最终读回集合与
// 某一次提交的 payload 完全相等（无混合状态）。
//
// SQLite 单写者结构性无法测并发，本用例按家族10 要求以 //go:build mysql 落盘。
// 需要真实 MySQL：DSN 取自 AIAPI_MYSQL_DSN（形如 root:pass@tcp(127.0.0.1:3306)/）。
func TestAIContextRules_Update_ConcurrentMySQL(t *testing.T) {
	adminDSN := os.Getenv("AIAPI_MYSQL_DSN")
	if adminDSN == "" {
		t.Skip("AIAPI_MYSQL_DSN not set, skipping MySQL concurrency test")
	}

	sm, err := testutil.StartServerWithMySQL(adminDSN)
	require.NoError(t, err, "start MySQL-backed server")
	defer sm.Shutdown()

	client := &testutil.Client{BaseURL: sm.ServerURL, HTTPClient: &http.Client{Timeout: 60 * time.Second}}

	const workers = 20
	type submission struct {
		cond string
		mode string
	}
	submissions := make([]submission, workers)
	for w := 0; w < workers; w++ {
		submissions[w] = submission{
			// cond 各不相同，保证每次提交的集合内容可区分。
			cond: fmt.Sprintf(`req_path_in("/v1/chat/completions", false) && req_body_json_in("model", "ctx-worker-%02d", false)`, w),
			mode: "balanced",
		}
	}

	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			resp, err := client.Put(aiContextRulesPath, map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{"cond": submissions[w].cond, "mode": submissions[w].mode},
				},
			})
			if err != nil {
				errs <- fmt.Errorf("request error: %w", err)
				return
			}
			// 管理写操作只允许 2xx/4xx，出现 5xx 即缺陷（家族7/#183）。
			if resp.ErrNum >= 500 {
				errs <- fmt.Errorf("worker %d: server error ErrNum=%d %s", w, resp.ErrNum, resp.ErrMsg)
			}
			// F-14：失败响应不得携带驱动错误原文（issue #295）。
			if resp.ErrNum != 200 && testutil.LeaksDriverError(resp.ErrMsg) {
				errs <- fmt.Errorf("worker %d: ErrMsg leaks driver error: %s", w, resp.ErrMsg)
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Errorf("concurrent PUT failed: %v", e)
	}

	// 最终一致性：读回集合必须与某一提交完全相等（无混合状态）。
	resp, err := client.Get(aiContextRulesPath)
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, "final read failed: %s", resp.ErrMsg)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	rules, ok := data["rules"].([]interface{})
	require.True(t, ok, "rules should be an array")
	require.Len(t, rules, 1, "each submission is a single-rule collection; final state must be one of them")

	got := rules[0].(map[string]interface{})
	matched := false
	for _, s := range submissions {
		if got["cond"] == s.cond && got["mode"] == s.mode {
			matched = true
			break
		}
	}
	assert.True(t, matched, "surviving rule %v must exactly match one submitted payload (no mixed state)", got)
	assert.Equal(t, float64(0), got["max_context_tokens"], "default backfill")
	assert.Equal(t, float64(0), got["reserve_tokens"], "default backfill")
}
