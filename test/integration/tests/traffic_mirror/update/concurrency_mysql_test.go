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

package traffic_mirror_update_test

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

// TestTrafficMirrorRules_Update_ConcurrentMySQL（TM-1-015，家族10）在 MySQL 后端下
// 并发执行全量替换 PUT。回归目标：delete-all+insert-all 的单事务路径在并发下不得
// 出现 5xx/deadlock/部分写入——所有请求必须 2xx，且最终读回集合与某一次提交的
// payload 完全相等（无混合状态）。
//
// SQLite 单写者结构性无法测并发，本用例按家族10 要求以 //go:build mysql 落盘。
// 需要真实 MySQL：DSN 取自 AIAPI_MYSQL_DSN（形如 root:pass@tcp(127.0.0.1:3306)/）。
func TestTrafficMirrorRules_Update_ConcurrentMySQL(t *testing.T) {
	adminDSN := os.Getenv("AIAPI_MYSQL_DSN")
	if adminDSN == "" {
		t.Skip("AIAPI_MYSQL_DSN not set, skipping MySQL concurrency test")
	}

	sm, err := testutil.StartServerWithMySQL(adminDSN)
	require.NoError(t, err, "start MySQL-backed server")
	defer sm.Shutdown()

	client := &testutil.Client{BaseURL: sm.ServerURL, HTTPClient: &http.Client{Timeout: 60 * time.Second}}

	// mirror_cluster 存在性校验需要真实 cluster：在 MySQL 后端服务上创建。
	// provider 必须带齐契约字段并断言业务码——裸 POST 会静默吞掉 422，
	// 让后续 cluster 创建失败伪装成并发缺陷（issue #295 setup 缺陷）。
	// 注意用本用例的 client（MySQL 服务实例），不能用 testutil 全局 client。
	providerName := testutil.UniqueProviderName()
	providerResp, err := client.Post("/open-api/v1/providers", map[string]interface{}{
		"name": providerName,
		"instance_pool": []interface{}{
			map[string]interface{}{"addr": "10.0.0.1", "weight": 100, "port": 8080},
		},
		"model_protocols": []string{"openai"},
		"models":          []string{"deepseek-chat"},
		"keys": []interface{}{
			map[string]interface{}{"name": "key-primary", "key": "sk-aaaaaaaaaaaa"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, providerResp.ErrNum, providerResp.ErrMsg)
	clusterName := testutil.UniqueClusterName()
	resp, err := client.Post("/open-api/v1/clusters", map[string]interface{}{
		"name": clusterName,
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": providerName,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 200, resp.ErrNum, resp.ErrMsg)

	const workers = 20
	names := make([]string, workers)
	for w := 0; w < workers; w++ {
		names[w] = fmt.Sprintf("tm-1-015-w%02d-%s", w, testutil.RandomString(8))
	}

	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			body := map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"name": names[w], "cond": validTMCond, "mirror_cluster": clusterName,
					},
				},
			}
			resp, err := client.Put(trafficMirrorRulesPath, body)
			if err != nil {
				errCh <- fmt.Errorf("worker %d: PUT error: %w", w, err)
				return
			}
			if resp.ErrNum != 200 {
				errCh <- fmt.Errorf("worker %d: PUT failed: %d %s", w, resp.ErrNum, resp.ErrMsg)
				// F-14：失败响应不得携带驱动错误原文（issue #295）。
				if testutil.LeaksDriverError(resp.ErrMsg) {
					errCh <- fmt.Errorf("worker %d: ErrMsg leaks driver error: %s", w, resp.ErrMsg)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Errorf("concurrent PUT failed (家族10): %v", e)
	}

	// 最终集合必须与某一次提交完全相等（无混合状态）。
	getResp, err := client.Get(trafficMirrorRulesPath)
	require.NoError(t, err)
	require.Equal(t, 200, getResp.ErrNum, getResp.ErrMsg)
	var data struct {
		Rules []struct {
			Name string `json:"name"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal(getResp.Data, &data))
	require.Len(t, data.Rules, 1, "final collection must equal exactly one submission")
	assert.Contains(t, names, data.Rules[0].Name, "final rule name must come from one of the submissions")
}
