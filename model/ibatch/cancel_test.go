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
//limitations under the License. All rights reserved.

package ibatch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
)

// setupCancelFixture 组装 cancel 测试夹具：httptest provider +
// Manager。handler 为 provider 侧行为；preempt 为抢占 affected。
func setupCancelFixture(t *testing.T, handler http.HandlerFunc, preempt int64) (*Manager, *fakeBatchStorager, *fakeRedisClient, *fakeOperationLogRecorder, *httptest.Server) {
	return setupCancelFixtureWithKeys(t, handler, preempt,
		[]iprovider.ProviderKey{{Name: "k1", Key: "sk-test"}})
}

// setupCancelFixtureWithKeys 与 setupCancelFixture 相同，但允许指定 provider
// keys（nil/空切片 = keyless provider，issue #274）。
func setupCancelFixtureWithKeys(t *testing.T, handler http.HandlerFunc, preempt int64, keys []iprovider.ProviderKey) (*Manager, *fakeBatchStorager, *fakeRedisClient, *fakeOperationLogRecorder, *httptest.Server) {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	storager := &fakeBatchStorager{
		fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
			return &BatchTask{
				ID:          11,
				BatchID:     "b-1",
				ProductName: "p1",
				Provider:    "openai",
				KeyName:     "k1",
				Status:      TaskStatusInProgress,
			}, nil
		},
		preemptCancelFn: func(ctx context.Context, batchID, productName string) (int64, error) {
			return preempt, nil
		},
	}
	redis := &fakeRedisClient{}
	audit := &fakeOperationLogRecorder{}
	providers := &fakeProviderQuerier{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			port := 0
			// 实例地址直接指向 httptest server（Addr 带端口语义由
			// buildInstanceBaseURL 拼 http://addr:port）。
			addr := server.URL[len("http://"):]
			for i := len(addr) - 1; i >= 0; i-- {
				if addr[i] == ':' {
					port = atoiOrZero(addr[i+1:])
					addr = addr[:i]
					break
				}
			}
			name := "openai"
			return &iprovider.Provider{
				Name:          name,
				Keys:          keys,
				InstancePool:  []iprovider.ProviderInstance{{Addr: addr, Port: port}},
				ProtocolPaths: map[string]string{"openai": "/v1/"},
			}, nil
		},
	}
	m := NewManager(storager, redis,
		&fakeAPIKeyQuerier{}, providers, &fakePriceQuerier{},
		func() *http.Client { return server.Client() },
		&fakeClock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)})
	m.SetOperationLogManager(audit)
	return m, storager, redis, audit, server
}

func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func TestCancel_Success(t *testing.T) {
	var gotPath, gotAuth string
	m, _, redis, audit, _ := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}, 1)

	// 预留簿记：plan-a -> 500。
	redis.hGetAllFn = func(ctx context.Context, key string) (map[string]string, error) {
		assert.Equal(t, "BATCH_RESERVE_BATCH:b-1", key)
		return map[string]string{"plan-a": "500"}, nil
	}

	task, err := m.Cancel(context.Background(), "b-1", "p1")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskStatusCancelling, task.Status)
	assert.Equal(t, "/v1/batches/b-1/cancel", gotPath, "protocol_paths 尾斜杠应规范化")
	assert.Equal(t, "Bearer sk-test", gotAuth)

	// 释放预留：镜像扣减 + 簿记删除。
	require.Len(t, redis.decrRecords, 1)
	assert.Equal(t, "BATCH_RESERVE:plan-a", redis.decrRecords[0].key)
	assert.Equal(t, int64(500), redis.decrRecords[0].delta)
	assert.Contains(t, redis.deletedKeys, "BATCH_RESERVE_BATCH:b-1")

	// 审计。
	entry := audit.last()
	require.NotNil(t, entry)
	assert.Equal(t, "batch_task", entry.ResourceType)
	assert.Equal(t, string(TaskStatusInProgress), entry.ChangeSummary["before"].(map[string]interface{})["status"])
	assert.Equal(t, TaskStatusCancelling, entry.ChangeSummary["after"].(map[string]interface{})["status"])
}

func TestCancel_NotFound(t *testing.T) {
	storager := &fakeBatchStorager{
		fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
			return nil, nil
		},
	}
	redis := &fakeRedisClient{
		hGetAllFn: func(ctx context.Context, key string) (map[string]string, error) {
			return nil, nil
		},
	}
	audit := &fakeOperationLogRecorder{}
	m := NewManager(storager, redis,
		&fakeAPIKeyQuerier{}, &fakeProviderQuerier{}, &fakePriceQuerier{}, nil, &fakeClock{})
	m.SetOperationLogManager(audit)

	_, err := m.Cancel(context.Background(), "b-x", "p1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrBatchNotFound))
	assert.Equal(t, 404, xerror.Resolve(err).ErrNo)
	require.NotNil(t, audit.last(), "失败也应审计")
	assert.Equal(t, int8(2), audit.last().Status)
}

func TestCancel_Conflict(t *testing.T) {
	m, _, _, audit, server := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("终态冲突不应出网")
	}, 0)

	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.Error(t, err)
	assert.Equal(t, 409, xerror.Resolve(err).ErrNo)
	require.NotNil(t, audit.last())
	_ = server
}

func TestCancel_RedisFallbackUpsertThenPreempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	preemptCalls := 0
	var upserted *BatchTask
	storager := &fakeBatchStorager{
		fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
			return nil, nil // DB 未落库
		},
		preemptCancelFn: func(ctx context.Context, batchID, productName string) (int64, error) {
			preemptCalls++
			if preemptCalls == 1 {
				return 0, nil
			}
			return 1, nil
		},
		upsertTaskFn: func(ctx context.Context, task *BatchTask) (int64, error) {
			upserted = task
			return 1, nil
		},
	}
	redis := &fakeRedisClient{
		hGetAllFn: func(ctx context.Context, key string) (map[string]string, error) {
			return map[string]string{
				"api_key_id": "ak", "provider": "openai", "status": "in_progress",
			}, nil
		},
	}
	apiKeys := &fakeAPIKeyQuerier{
		fetchFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
			product := "p1"
			return []*api_key.APIKeyParam{{ID: filter.ID, ProductName: &product}}, nil
		},
	}
	providers := &fakeProviderQuerier{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			return &iprovider.Provider{
				Name:          "openai",
				Keys:          []iprovider.ProviderKey{{Name: "k1", Key: "sk"}},
				InstancePool:  []iprovider.ProviderInstance{{Addr: "127.0.0.1", Port: 1}},
				ProtocolPaths: map[string]string{"openai": "/v1"},
			}, nil
		},
	}
	m := NewManager(storager, redis, apiKeys, providers, &fakePriceQuerier{},
		func() *http.Client { return server.Client() }, &fakeClock{})

	// provider 不可达（port 1）——这里只验证补登+二次抢占前置路径；
	// 出网失败允许，关键断言是补登发生且抢占重试。
	_, _ = m.Cancel(context.Background(), "b-1", "p1")
	require.NotNil(t, upserted, "Redis 回源任务应先补登再抢占")
	assert.Equal(t, "b-1", upserted.IdemKey)
	assert.Equal(t, 2, preemptCalls)
}

func TestCancel_ProviderNotFound(t *testing.T) {
	m, _, _, _, _ := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}, 1)

	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpstreamBatchNotFound))
}

func TestCancel_ProviderUnreachable(t *testing.T) {
	t.Run("5xx", func(t *testing.T) {
		m, _, _, _, _ := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}, 1)
		_, err := m.Cancel(context.Background(), "b-1", "p1")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpstreamUnreachable))
	})

	t.Run("连接失败", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := server.URL
		server.Close() // 立即关闭制造不可达

		storager := &fakeBatchStorager{
			fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
				return &BatchTask{ID: 1, BatchID: "b-1", ProductName: "p1", Provider: "openai", Status: TaskStatusQueued}, nil
			},
			preemptCancelFn: func(ctx context.Context, batchID, productName string) (int64, error) {
				return 1, nil
			},
		}
		providers := &fakeProviderQuerier{
			fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
				addr := url[len("http://"):]
				port := 0
				for i := len(addr) - 1; i >= 0; i-- {
					if addr[i] == ':' {
						port = atoiOrZero(addr[i+1:])
						addr = addr[:i]
						break
					}
				}
				return &iprovider.Provider{
					Name:         "openai",
					Keys:         []iprovider.ProviderKey{{Name: "k1", Key: "sk"}},
					InstancePool: []iprovider.ProviderInstance{{Addr: addr, Port: port}},
				}, nil
			},
		}
		m := NewManager(storager, &fakeRedisClient{},
			&fakeAPIKeyQuerier{}, providers, &fakePriceQuerier{}, nil, &fakeClock{},
			WithProviderTimeout(200*time.Millisecond))
		_, err := m.Cancel(context.Background(), "b-1", "p1")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpstreamUnreachable))
	})
}

func TestCancel_ProviderMissing(t *testing.T) {
	storager := &fakeBatchStorager{
		fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
			return &BatchTask{ID: 1, BatchID: "b-1", ProductName: "p1", Provider: "ghost", Status: TaskStatusQueued}, nil
		},
		preemptCancelFn: func(ctx context.Context, batchID, productName string) (int64, error) {
			return 1, nil
		},
	}
	providers := &fakeProviderQuerier{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			return nil, nil
		},
	}
	m := NewManager(storager, &fakeRedisClient{}, &fakeAPIKeyQuerier{}, providers, &fakePriceQuerier{}, nil, &fakeClock{})
	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.Error(t, err)
	assert.Equal(t, 500, xerror.Resolve(err).ErrNo)
}

func TestCancel_ReleaseIdempotent(t *testing.T) {
	m, _, redis, _, _ := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, 1)

	// 簿记已清零：HGetAll 为空 → 不扣减、不删除。
	redis.hGetAllFn = func(ctx context.Context, key string) (map[string]string, error) {
		return map[string]string{}, nil
	}

	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.NoError(t, err)
	assert.Empty(t, redis.decrRecords)
	assert.NotContains(t, redis.deletedKeys, "BATCH_RESERVE_BATCH:b-1")
}

func TestCancel_ReleaseErrorNotFatal(t *testing.T) {
	m, _, redis, _, _ := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, 1)

	redis.hGetAllFn = func(ctx context.Context, key string) (map[string]string, error) {
		return nil, errors.New("redis down")
	}

	// 释放失败不影响 cancel 成功语义。
	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.NoError(t, err)
}

func TestCancel_NoRedis(t *testing.T) {
	m, _, _, _, _ := setupCancelFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, 1)
	m.redis = nil // 无 Redis 降级：跳过释放

	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.NoError(t, err)
}

func TestReleaseReserve_MultiPlan(t *testing.T) {
	m := newTestManager(&fakeBatchStorager{}, &fakeRedisClient{
		hGetAllFn: func(ctx context.Context, key string) (map[string]string, error) {
			return map[string]string{"plan-a": "100", "plan-b": "250", "bad": "x", "zero": "0"}, nil
		},
	})
	err := m.releaseReserve(context.Background(), "b-1")
	require.NoError(t, err)
	require.Len(t, m.redis.(*fakeRedisClient).decrRecords, 2)
}

func TestPickProviderInstance(t *testing.T) {
	ctx := context.Background()
	m := NewManager(&fakeBatchStorager{}, &fakeRedisClient{},
		&fakeAPIKeyQuerier{}, &fakeProviderQuerier{}, &fakePriceQuerier{}, nil, &fakeClock{})

	inst, err := m.pickProviderInstance(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, inst)

	p := &iprovider.Provider{InstancePool: []iprovider.ProviderInstance{
		{Addr: "1.1.1.1", Port: 1, Disable: true},
		{Addr: "", Port: 2},
		{Addr: "2.2.2.2", Port: 0},
	}}
	inst, err = m.pickProviderInstance(ctx, p)
	require.NoError(t, err)
	assert.Nil(t, inst, "禁用/缺地址/缺端口实例均不可用")

	p.InstancePool = append(p.InstancePool, iprovider.ProviderInstance{Addr: "3.3.3.3", Port: 443})
	inst, err = m.pickProviderInstance(ctx, p)
	require.NoError(t, err)
	require.NotNil(t, inst)
	assert.Equal(t, "3.3.3.3", inst.Addr)
}

func TestPickProviderInstance_K8sRealtime(t *testing.T) {
	ctx := context.Background()

	newK8sManager := func(instances []iprovider.ProviderInstance, readerErr error) *Manager {
		m := NewManager(&fakeBatchStorager{}, &fakeRedisClient{},
			&fakeAPIKeyQuerier{}, &fakeProviderQuerier{}, &fakePriceQuerier{}, nil, &fakeClock{})
		m.SetInstancePoolResolver(iprovider.NewInstancePoolResolver(&fakeK8sInstanceReader{
			instances: instances,
			err:       readerErr,
		}))
		return m
	}

	poolName := "svc-a"
	k8sProvider := &iprovider.Provider{
		Name:           "k8s-prov",
		InstanceSource: iprovider.InstanceSourceK8sPool,
		K8sPoolName:    &poolName,
	}

	t.Run("k8s provider resolves instances from the reader", func(t *testing.T) {
		m := newK8sManager([]iprovider.ProviderInstance{
			{Addr: "10.0.0.1", Port: 8000, Weight: 100},
			{Addr: "10.0.0.2", Port: 8000, Weight: 100, Disable: true},
		}, nil)
		inst, err := m.pickProviderInstance(ctx, k8sProvider)
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Equal(t, "10.0.0.1", inst.Addr)
	})

	t.Run("k8s provider with no instances yields unavailable", func(t *testing.T) {
		m := newK8sManager(nil, nil)
		inst, err := m.pickProviderInstance(ctx, k8sProvider)
		require.NoError(t, err)
		assert.Nil(t, inst)
	})

	t.Run("reader error propagates", func(t *testing.T) {
		m := newK8sManager(nil, errors.New("k8s_pools down"))
		_, err := m.pickProviderInstance(ctx, k8sProvider)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "k8s_pools down")
	})

	t.Run("unconfigured resolver on a k8s provider is an error", func(t *testing.T) {
		m := NewManager(&fakeBatchStorager{}, &fakeRedisClient{},
			&fakeAPIKeyQuerier{}, &fakeProviderQuerier{}, &fakePriceQuerier{}, nil, &fakeClock{})
		_, err := m.pickProviderInstance(ctx, k8sProvider)
		require.Error(t, err)
	})
}

func TestBuildInstanceBaseURL(t *testing.T) {
	assert.Equal(t, "http://10.0.0.1:8080", buildInstanceBaseURL(iprovider.ProviderInstance{Addr: "10.0.0.1", Port: 8080}))
	assert.Equal(t, "http://10.0.0.1:8080", buildInstanceBaseURL(iprovider.ProviderInstance{Addr: "http://10.0.0.1:8080", Port: 0}))
	assert.Equal(t, "https://api.openai.com", buildInstanceBaseURL(iprovider.ProviderInstance{Addr: "https://api.openai.com/", Port: 0}))
}

func TestNormalizeProviderPath(t *testing.T) {
	assert.Equal(t, "/v1", normalizeProviderPath("/v1/"))
	assert.Equal(t, "/v1", normalizeProviderPath("v1"))
	assert.Equal(t, "/openai/v1", normalizeProviderPath("/openai/v1/"))
}

// issue #274：keyless provider（keys: []）是合法配置（providers.md keys 条款），
// cancel 应无鉴权出网并 200。修复前此用例失败（500 errProviderUnavailable）。
func TestCancel_KeylessProvider(t *testing.T) {
	var gotPath string
	var authPresent bool
	m, _, redis, _, _ := setupCancelFixtureWithKeys(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, authPresent = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	}, 1, nil)

	redis.hGetAllFn = func(ctx context.Context, key string) (map[string]string, error) {
		return map[string]string{"plan-a": "500"}, nil
	}

	task, err := m.Cancel(context.Background(), "b-1", "p1")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskStatusCancelling, task.Status)
	assert.Equal(t, "/v1/batches/b-1/cancel", gotPath)
	assert.False(t, authPresent, "keyless provider 不得携带 Authorization 头")
}

// issue #274：已 cancelling 任务的重放为幂等 200（重发 provider cancel 保证意图
// 落地，释放为幂等 no-op）。修复前此用例失败（409 errBatchConflict 假冲突）。
func TestCancel_ReplayIdempotent(t *testing.T) {
	cancelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancelCalls++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	addr, port := splitAddrPort(t, server.URL)
	storager := &fakeBatchStorager{
		fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
			return &BatchTask{
				ID: 11, BatchID: "b-1", ProductName: "p1",
				Provider: "openai", KeyName: "k1", Status: TaskStatusCancelling,
			}, nil
		},
		preemptCancelFn: func(ctx context.Context, batchID, productName string) (int64, error) {
			return 0, nil
		},
	}
	providers := &fakeProviderQuerier{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			return &iprovider.Provider{
				Name:         "openai",
				Keys:         []iprovider.ProviderKey{{Name: "k1", Key: "sk"}},
				InstancePool: []iprovider.ProviderInstance{{Addr: addr, Port: port}},
			}, nil
		},
	}
	m := NewManager(storager, &fakeRedisClient{},
		&fakeAPIKeyQuerier{}, providers, &fakePriceQuerier{},
		func() *http.Client { return server.Client() }, &fakeClock{})

	task, err := m.Cancel(context.Background(), "b-1", "p1")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskStatusCancelling, task.Status)
	assert.Equal(t, 1, cancelCalls, "幂等重放应重发 provider cancel 保证意图落地")
}

// 终态任务重放仍 409——防 R3 把终态误判为幂等重放。
func TestCancel_ReplayTerminalConflict(t *testing.T) {
	storager := &fakeBatchStorager{
		fetchTaskFn: func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
			return &BatchTask{
				ID: 11, BatchID: "b-1", ProductName: "p1",
				Provider: "openai", Status: TaskStatusCompleted,
			}, nil
		},
		preemptCancelFn: func(ctx context.Context, batchID, productName string) (int64, error) {
			return 0, nil
		},
	}
	providers := &fakeProviderQuerier{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			t.Errorf("终态冲突不应解析 provider 或出网")
			return nil, nil
		},
	}
	m := NewManager(storager, &fakeRedisClient{},
		&fakeAPIKeyQuerier{}, providers, &fakePriceQuerier{}, nil, &fakeClock{})

	_, err := m.Cancel(context.Background(), "b-1", "p1")
	require.Error(t, err)
	assert.Equal(t, 409, xerror.Resolve(err).ErrNo)
}

// issue #274：resolveProviderAccess 三分支——keyless 放行（authHeader 空）、
// keyName 命中与回退、配置了 key 但无可用明文报数据异常。
func TestResolveProviderAccess_KeylessAndBranches(t *testing.T) {
	mkManager := func(pq *fakeProviderQuerier) *Manager {
		return NewManager(&fakeBatchStorager{}, &fakeRedisClient{},
			&fakeAPIKeyQuerier{}, pq, &fakePriceQuerier{}, nil, &fakeClock{})
	}
	fetchFixed := func(p *iprovider.Provider, err error) func(context.Context, *iprovider.ProviderFilter) (*iprovider.Provider, error) {
		return func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			return p, err
		}
	}
	pool := []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 8080}}

	t.Run("keys nil → 无鉴权头", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name: "openai", InstancePool: pool}, nil)}
		access, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "k1")
		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Empty(t, access.authHeader)
		assert.Empty(t, access.authValue)
		assert.Equal(t, "http://10.0.0.1:8080", access.baseURL)
	})

	t.Run("keys 空切片 → 无鉴权头", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name: "openai", Keys: []iprovider.ProviderKey{}, InstancePool: pool}, nil)}
		access, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "")
		require.NoError(t, err)
		require.NotNil(t, access)
		assert.Empty(t, access.authHeader)
	})

	t.Run("keyName 命中", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name:         "openai",
			Keys:         []iprovider.ProviderKey{{Name: "k1", Key: "sk1"}, {Name: "k2", Key: "sk2"}},
			InstancePool: pool}, nil)}
		access, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "k2")
		require.NoError(t, err)
		assert.Equal(t, "Authorization", access.authHeader)
		assert.Equal(t, "Bearer sk2", access.authValue)
	})

	t.Run("keyName 缺失回退第一个可用明文", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name:         "openai",
			Keys:         []iprovider.ProviderKey{{Name: "k1", Key: "sk1"}},
			InstancePool: pool}, nil)}
		access, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "ghost")
		require.NoError(t, err)
		assert.Equal(t, "Bearer sk1", access.authValue)
	})

	t.Run("keyName 为空回退第一个可用明文", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name:         "openai",
			Keys:         []iprovider.ProviderKey{{Name: "k1", Key: "sk1"}},
			InstancePool: pool}, nil)}
		access, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "")
		require.NoError(t, err)
		assert.Equal(t, "Bearer sk1", access.authValue)
	})

	t.Run("配置了 key 但无可用明文 → 数据异常 500", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name:         "openai",
			Keys:         []iprovider.ProviderKey{{Name: "k1", Key: ""}},
			InstancePool: pool}, nil)}
		_, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "")
		require.Error(t, err)
		assert.Equal(t, 500, xerror.Resolve(err).ErrNo)
	})

	t.Run("provider 不存在 → 500", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(nil, nil)}
		_, err := mkManager(pq).resolveProviderAccess(context.Background(), "ghost", "")
		require.Error(t, err)
		assert.Equal(t, 500, xerror.Resolve(err).ErrNo)
	})

	t.Run("无可用实例 → 500", func(t *testing.T) {
		pq := &fakeProviderQuerier{fetchFn: fetchFixed(&iprovider.Provider{
			Name: "openai"}, nil)}
		_, err := mkManager(pq).resolveProviderAccess(context.Background(), "openai", "")
		require.Error(t, err)
		assert.Equal(t, 500, xerror.Resolve(err).ErrNo)
	})
}
