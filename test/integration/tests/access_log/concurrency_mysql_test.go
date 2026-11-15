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

//go:build mysql

// AL-1-011（家族10）：MySQL 后端下并发执行单例覆盖式 PUT。
// 回归目标：单行 upsert 的单事务路径在并发下不得出现 5xx/deadlock/部分写入——
// 所有请求必须 2xx，且最终读回的单例与某一次提交完全相等（req/res 白名单同属
// 一次提交，无混合状态）。
//
// SQLite 单写者结构性无法测并发，本用例按家族10 要求以 //go:build mysql 落盘。
// 需要真实 MySQL：DSN 取自 AIAPI_MYSQL_DSN（形如 root:pass@tcp(127.0.0.1:3306)/）。
package access_log_test

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

func TestAccessLogConfigPut_ConcurrentMySQL(t *testing.T) {
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
		reqMarker string
		resMarker string
	}
	subs := make([]submission, workers)
	for w := 0; w < workers; w++ {
		subs[w] = submission{
			reqMarker: fmt.Sprintf("x-conc-req-%d", w),
			resMarker: fmt.Sprintf("x-conc-res-%d", w),
		}
	}

	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			resp, err := client.Put("/open-api/v1/access-log/config", map[string]interface{}{
				"req_headers": []interface{}{subs[w].reqMarker},
				"res_headers": []interface{}{subs[w].resMarker},
			})
			if err != nil {
				errs[w] = err
				return
			}
			if resp.ErrNum != 200 {
				errs[w] = fmt.Errorf("worker %d: ErrNum=%d ErrMsg=%s", w, resp.ErrNum, resp.ErrMsg)
			}
		}(w)
	}
	wg.Wait()
	for w := 0; w < workers; w++ {
		require.NoError(t, errs[w], "every concurrent PUT must be 2xx (no 5xx/deadlock)")
	}

	// 最终读回的单例必须与某一次提交完全相等（last-write-wins，无混合状态）
	resp, err := client.Get("/open-api/v1/access-log/config")
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum)

	var data struct {
		ReqHeaders []string `json:"req_headers"`
		ResHeaders []string `json:"res_headers"`
	}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	require.Len(t, data.ReqHeaders, 1)
	require.Len(t, data.ResHeaders, 1)

	matched := false
	for _, sub := range subs {
		if data.ReqHeaders[0] == sub.reqMarker && data.ResHeaders[0] == sub.resMarker {
			matched = true
			break
		}
	}
	assert.True(t, matched, "final singleton must equal exactly one submission, got req=%v res=%v",
		data.ReqHeaders, data.ResHeaders)
}
