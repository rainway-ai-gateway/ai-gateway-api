//go:build mysql

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

package innerapi_test

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireMySQLBackend 确认当前测试服务器跑在真实 MySQL（InnoDB）上。
// TestMain 的 StartServer 在本机 MySQL 不可达时会回退 SQLite，而本用例的
// 并发语义（REPEATABLE READ 快照、唯一键行锁、死锁回放）只在 MySQL 下成立。
func requireMySQLBackend(t *testing.T) {
	t.Helper()
	db := testutil.OpenDB(t, sm)
	defer db.Close()
	var isolation string
	if err := db.QueryRow("SELECT @@transaction_isolation").Scan(&isolation); err != nil {
		t.Skipf("backend is not MySQL (%v), skipping InnoDB concurrency test", err)
	}
	t.Logf("backend isolation level: %s", isolation)
}

// TestInnerAPI_ConcurrentExportSameSecondMySQL 是 issue #318 / SC2101-TC072 的
// 自动化回归：导出内容刚变更后，多路调用方在同一墙钟秒内并发拉取
// server_data_conf，必须全部 2xx 且收敛到同一个新版本——预修复实现把唯一键
// 冲突（1062）整单上抛为 HTTP 500（3/4 路失败）。
//
// 与 TC072 的最小复现一致：串行基线导出 → 变更内容（删除 cluster）→ 并发轮
// 同时释放。内容未变的多路并发允许返回同一版本（设计第 421 行），因此即使
// 某轮竞争完全错开（无碰撞），断言仍成立；发生碰撞时由「storager 吸收 1062 +
// ExecuteWithRetry 回放 1213」收敛，断言同样成立——判据是并发契约本身。
func TestInnerAPI_ConcurrentExportSameSecondMySQL(t *testing.T) {
	requireMySQLBackend(t)

	const rounds = 3
	const racers = 4

	for round := 0; round < rounds; round++ {
		clusterName := testutil.UniqueClusterName()
		_, err := testutil.CreateCluster(clusterName)
		require.NoError(t, err, "round %d: setup cluster", round)

		// 基线导出：内容已含 cluster 的串行版本。
		v0, _ := fetchServerDataConfVersion(t, "")

		// 变更内容（删除 cluster），随后同秒释放并发轮。
		require.NoError(t, testutil.DeleteCluster(clusterName))

		type exportResult struct {
			version string
			resp    *testutil.APIResponse
			err     error
		}
		start := make(chan struct{})
		results := make(chan exportResult, racers)
		var wg sync.WaitGroup
		for i := 0; i < racers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				resp, err := testutil.GetClient().Get("/inner-api/v1/configs/tls_conf/server_data_conf")
				if err != nil {
					results <- exportResult{err: err}
					return
				}
				r := exportResult{resp: resp}
				if resp.ErrNum == 200 {
					var data struct {
						Version string `json:"Version"`
					}
					if err := json.Unmarshal(resp.Data, &data); err == nil {
						r.version = data.Version
					}
				}
				results <- r
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		winner := ""
		ok := 0
		for r := range results {
			require.NoError(t, r.err, "round %d: export request", round)
			assert.Equal(t, 200, r.resp.ErrNum,
				"round %d: concurrent export must not fail: %s", round, r.resp.ErrMsg)
			assert.False(t, testutil.LeaksDriverError(r.resp.ErrMsg),
				"round %d: ErrMsg must not leak driver details: %s", round, r.resp.ErrMsg)
			if r.resp.ErrNum != 200 {
				continue
			}
			require.NotEmpty(t, r.version, "round %d: export must carry a version", round)
			if winner == "" {
				winner = r.version
			}
			assert.Equal(t, winner, r.version,
				"round %d: all racing exports must converge on one version", round)
			ok++
		}
		require.Equal(t, racers, ok, "round %d: all %d racers must succeed", round, racers)
		assert.True(t, winner > v0,
			"round %d: concurrent version (%s) must advance past baseline (%s)", round, winner, v0)
	}
}
