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

package icluster_conf

import (
	"context"
	"errors"
	"testing"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/imodel_price"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	if stateful.DefaultConfig == nil {
		stateful.DefaultConfig = &stateful.Config{
			RunTime: stateful.RunTimeConfig{
				DefaultAIClusterName: "BFE-AI",
			},
		}
	}
}

func TestProviderInstancesToClusterInstances(t *testing.T) {
	instances := []iprovider.ProviderInstance{
		{Addr: "1.2.3.4", Port: 443, Weight: 100},
		{Addr: "5.6.7.8", Port: 443, Weight: 100},
	}
	got := providerInstancesToClusterInstances(instances)
	require.Len(t, got, 2)
	assert.Equal(t, "1.2.3.4_443", got[0].Name)
	assert.Equal(t, "5.6.7.8_443", got[1].Name)
}

func TestClusterManager_ProviderInstancePoolSyncer(t *testing.T) {
	ctx := context.Background()
	providerName := "deepseek"

	makeCluster := func(name string, provider *string, instances []Instance) *Cluster {
		return &Cluster{
			ID:   1,
			Name: name,
			LLMConfig: &LLMConfig{
				Provider: provider,
			},
			SubClusters: []*SubCluster{
				{
					ID:   1,
					Name: "sc1",
					InstancePool: &Pool{
						ID:        1,
						Name:      "p." + name,
						Instances: instances,
					},
				},
			},
		}
	}

	t.Run("updates clusters referencing the provider", func(t *testing.T) {
		updatedPools := map[int64][]Instance{}
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeCluster("c1", &providerName, []Instance{
						{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100},
					}),
					makeCluster("c2", lib.PString("other"), []Instance{
						{Name: "other", Addr: "9.8.7.6", Port: 443, Weight: 100},
					}),
				}, nil
			},
		}
		poolStorager := &fakePoolStorager{
			updatePoolFn: func(ctx context.Context, oldData *Pool, diff *PoolParam) error {
				updatedPools[oldData.ID] = diff.Instances
				return nil
			},
		}

		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, poolStorager, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name: providerName,
			InstancePool: []iprovider.ProviderInstance{
				{Addr: "5.6.7.8", Port: 443, Weight: 100},
			},
		}
		err := cm.ProviderInstancePoolSyncer(ctx, nil, newProvider)
		require.NoError(t, err)

		require.Len(t, updatedPools, 1)
		require.Contains(t, updatedPools, int64(1))
		assert.Equal(t, "5.6.7.8_443", updatedPools[1][0].Name)
	})

	t.Run("syncs EPP sub-clusters", func(t *testing.T) {
		updatedPools := map[int64][]Instance{}
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				c := makeCluster("c1", &providerName, []Instance{
					{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100},
				})
				c.SubClusters[0].Role = ProductPoolRoleEPP
				return []*Cluster{c}, nil
			},
		}
		poolStorager := &fakePoolStorager{
			updatePoolFn: func(ctx context.Context, oldData *Pool, diff *PoolParam) error {
				updatedPools[oldData.ID] = diff.Instances
				return nil
			},
		}

		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, poolStorager, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name: providerName,
			InstancePool: []iprovider.ProviderInstance{
				{Addr: "5.6.7.8", Port: 443, Weight: 100},
			},
		}
		err := cm.ProviderInstancePoolSyncer(ctx, nil, newProvider)
		require.NoError(t, err)
		require.Len(t, updatedPools, 1)
		require.Contains(t, updatedPools, int64(1))
		assert.Equal(t, "5.6.7.8_443", updatedPools[1][0].Name)
	})

	t.Run("propagates update pool error", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeCluster("c1", &providerName, []Instance{
						{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100},
					}),
				}, nil
			},
		}
		poolStorager := &fakePoolStorager{
			updatePoolFn: func(ctx context.Context, oldData *Pool, diff *PoolParam) error {
				return errors.New("update failed")
			},
		}

		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, poolStorager, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name: providerName,
			InstancePool: []iprovider.ProviderInstance{
				{Addr: "5.6.7.8", Port: 443, Weight: 100},
			},
		}
		err := cm.ProviderInstancePoolSyncer(ctx, nil, newProvider)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
	})
}

func TestClusterManager_ProviderKeyRefChecker(t *testing.T) {
	ctx := context.Background()
	providerName := "deepseek"

	makeCluster := func(name string, provider *string, keyNames []string) *Cluster {
		keys := make([]ClusterKeyRef, 0, len(keyNames))
		for _, k := range keyNames {
			keys = append(keys, ClusterKeyRef{Name: lib.PString(k), Weight: lib.PInt(100)})
		}
		return &Cluster{
			ID:   1,
			Name: name,
			LLMConfig: &LLMConfig{
				Provider: provider,
				Keys:     keys,
			},
		}
	}

	t.Run("blocks removal of referenced key", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeCluster("c1", &providerName, []string{"k1"}),
					makeCluster("c2", lib.PString("other"), []string{"k1"}),
				}, nil
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name: providerName,
			Keys: []iprovider.ProviderKey{{Name: "k2", Key: "sk-new"}},
		}
		err := cm.ProviderKeyRefChecker(ctx, nil, newProvider)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key k1 is referenced by cluster c1")
	})

	t.Run("allows rename when old keys are still covered", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeCluster("c1", &providerName, []string{"k1", "k2"}),
				}, nil
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name: providerName,
			Keys: []iprovider.ProviderKey{
				{Name: "k1", Key: "sk-1"},
				{Name: "k2", Key: "sk-2"},
				{Name: "k3", Key: "sk-3"},
			},
		}
		err := cm.ProviderKeyRefChecker(ctx, nil, newProvider)
		require.NoError(t, err)
	})

	t.Run("skips when keys unchanged", func(t *testing.T) {
		called := false
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				called = true
				return []*Cluster{
					makeCluster("c1", &providerName, []string{"k1"}),
				}, nil
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		oldProvider := &iprovider.Provider{
			Name: providerName,
			Keys: []iprovider.ProviderKey{{Name: "k1", Key: "sk-same"}},
		}
		newProvider := &iprovider.Provider{
			Name: providerName,
			Keys: []iprovider.ProviderKey{{Name: "k1", Key: "sk-same"}},
		}
		err := cm.ProviderKeyRefChecker(ctx, oldProvider, newProvider)
		require.NoError(t, err)
		assert.False(t, called, "fetch should be skipped when keys are unchanged")
	})

	t.Run("propagates fetch error", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("db down")
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		err := cm.ProviderKeyRefChecker(ctx, nil, &iprovider.Provider{Name: providerName})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}

func TestClusterManager_ProviderModelRefChecker(t *testing.T) {
	ctx := context.Background()
	providerName := "deepseek"

	makeCluster := func(name string, provider *string, models []string) *Cluster {
		return &Cluster{
			ID:   1,
			Name: name,
			LLMConfig: &LLMConfig{
				Provider: provider,
				Models:   models,
			},
		}
	}

	t.Run("blocks removal of referenced model", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeCluster("c1", &providerName, []string{"m1"}),
					makeCluster("c2", lib.PString("other"), []string{"m1"}),
				}, nil
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name:   providerName,
			Models: []string{"m2"},
		}
		err := cm.ProviderModelRefChecker(ctx, nil, newProvider)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model m1 is referenced by cluster c1")
	})

	t.Run("allows addition of new models", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeCluster("c1", &providerName, []string{"m1"}),
				}, nil
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		newProvider := &iprovider.Provider{
			Name:   providerName,
			Models: []string{"m1", "m2"},
		}
		err := cm.ProviderModelRefChecker(ctx, nil, newProvider)
		require.NoError(t, err)
	})

	t.Run("skips when models unchanged", func(t *testing.T) {
		called := false
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				called = true
				return []*Cluster{
					makeCluster("c1", &providerName, []string{"m1"}),
				}, nil
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		oldProvider := &iprovider.Provider{Name: providerName, Models: []string{"m1"}}
		newProvider := &iprovider.Provider{Name: providerName, Models: []string{"m1"}}
		err := cm.ProviderModelRefChecker(ctx, oldProvider, newProvider)
		require.NoError(t, err)
		assert.False(t, called, "fetch should be skipped when models are unchanged")
	})

	t.Run("propagates fetch error", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("db down")
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		err := cm.ProviderModelRefChecker(ctx, nil, &iprovider.Provider{Name: providerName})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}

func newTestClusterBase() *Cluster {
	return &Cluster{
		ID:   1,
		Name: "c1",
		Basic: &ClusterBasic{
			Protocol: lib.PString("http"),
			Connection: &ClusterBasicConnection{
				MaxIdleConnPerRs:    10,
				CancelOnClientClose: false,
			},
			Retries: &ClusterBasicRetries{
				MaxRetryInSubcluster:    1,
				MaxRetryCrossSubcluster: 1,
			},
			Buffers: &ClusterBasicBuffers{
				ReqWriteBufferSize: 1024,
				ReqFlushInterval:   100,
				ResFlushInterval:   100,
			},
			Timeouts: &ClusterBasicTimeouts{
				TimeoutConnServ:        1000,
				TimeoutResponseHeader:  1000,
				TimeoutReadbodyClient:  1000,
				TimeoutReadClientAgain: 1000,
				TimeoutWriteClient:     1000,
			},
		},
		StickySessions: &ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  0,
			HashHeader:    "",
		},
		PassiveHealthCheck: &ClusterPassiveHealthCheck{
			Schema:     "http",
			Interval:   10,
			Failnum:    3,
			Statuscode: 500,
			Host:       "",
			Uri:        "/",
		},
		SubClusters: []*SubCluster{
			{
				ID:   1,
				Name: "sc1",
				InstancePool: &Pool{
					ID:   1,
					Name: "p1",
					Instances: []Instance{
						{Name: "rs1", Addr: "127.0.0.1", Port: 80, Weight: 10},
					},
				},
			},
		},
		Scheduler: map[string]map[string]int{
			"bfe1": {
				"sc1": 100,
			},
		},
	}
}

func newTestClusterEPP() *Cluster {
	c := newTestClusterBase()
	c.BalanceMode = BalanceModeEPP
	c.EppConfig = `{"load_profile":"balanced"}`
	return c
}

func newTestClusterEPPEppRuntime(runtimeJSON string) *Cluster {
	c := newTestClusterEPP()
	c.EppRuntime = runtimeJSON
	return c
}

func newTestClusterDomain() *Cluster {
	c := newTestClusterBase()
	c.SubClusters[0].InstancePool.Instances[0].Addr = "example.com"
	return c
}

func newTestClusterHTTPS() *Cluster {
	c := newTestClusterBase()
	c.Basic.Protocol = lib.PString("https")
	return c
}

func newTestClusterLLM() *Cluster {
	c := newTestClusterBase()
	c.LLMConfig = &LLMConfig{
		Provider: lib.PString("openai"),
		Models:   []string{"m1", "m2"},
		ModelMappings: []*Mapping{
			{SourceModel: lib.PString("old"), TargetModel: lib.PString("new")},
		},
		Keys: []ClusterKeyRef{
			{Name: lib.PString("key-primary"), Weight: lib.PInt(70)},
			{Name: lib.PString("key-secondary"), Weight: lib.PInt(30)},
		},
		KeyPolicy: &KeyPolicy{
			Strategy:            lib.PString("weighted_random"),
			MaxRetries:          lib.PInt(3),
			RetryBackoffInitial: lib.PInt(500),
			RetryBackoffMax:     lib.PInt(5000),
		},
		KeyAffinity: &KeyAffinity{
			Enabled:       lib.PBool(true),
			TTL:           lib.PInt(600),
			RedisPrefix:   lib.PString("bfe:ai:key_affinity"),
			PenaltyEnable: lib.PBool(true),
		},
		NormalizeUpstreamError: &NormalizeUpstreamError{
			Enabled:            lib.PBool(true),
			StreamEnabled:      lib.PBool(true),
			UnrecognizedAction: lib.PString("passthrough"),
			MaxBodyBytes:       lib.PInt64(65536),
			RedactSecrets:      lib.PBool(true),
		},
		MatchPrefix: lib.PString("openrouter/"),
		StripPrefix: lib.PBool(true),
	}
	return c
}

// newTestClusterLLMWithoutNue returns the LLM test cluster without the
// normalize_upstream_error config (the historical shape before the
// upstream-error-normalization change).
func newTestClusterLLMWithoutNue() *Cluster {
	c := newTestClusterLLM()
	c.LLMConfig.NormalizeUpstreamError = nil
	return c
}

func TestCluster_SubClusterNames(t *testing.T) {
	c := newTestClusterBase()
	assert.Equal(t, []string{"sc1"}, c.SubClusterNames())
}

func TestCluster_getBalanceMode(t *testing.T) {
	t.Run("default WRR when unset", func(t *testing.T) {
		c := newTestClusterBase()
		assert.Equal(t, BalanceModeWRR, c.getBalanceMode())
	})
	t.Run("explicit WRR", func(t *testing.T) {
		c := newTestClusterBase()
		c.BalanceMode = BalanceModeWRR
		assert.Equal(t, BalanceModeWRR, c.getBalanceMode())
	})

	t.Run("EPP", func(t *testing.T) {
		c := newTestClusterEPP()
		assert.Equal(t, BalanceModeEPP, c.getBalanceMode())
	})

	t.Run("SubCluster Role no longer derives balance mode", func(t *testing.T) {
		c := newTestClusterBase()
		c.SubClusters[0].Role = ProductPoolRoleEPP
		assert.Equal(t, BalanceModeWRR, c.getBalanceMode())
	})
}

func TestClusterList2MapByName(t *testing.T) {
	c1 := &Cluster{ID: 1, Name: "a"}
	c2 := &Cluster{ID: 2, Name: "b"}
	got := ClusterList2MapByName([]*Cluster{c1, c2})
	assert.Equal(t, map[string]*Cluster{"a": c1, "b": c2}, got)
}

func TestClusterList2MapByID(t *testing.T) {
	c1 := &Cluster{ID: 1, Name: "a"}
	c2 := &Cluster{ID: 2, Name: "b"}
	got := ClusterList2MapByID([]*Cluster{c1, c2})
	assert.Equal(t, map[int64]*Cluster{1: c1, 2: c2}, got)
}

func TestNewClusterManager(t *testing.T) {
	m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
	require.NotNil(t, m)
}

func TestClusterManager_FetchClusterList(t *testing.T) {
	ctx := context.Background()
	expected := []*Cluster{{ID: 1, Name: "c1"}}
	store := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			assert.Equal(t, lib.PString("c1"), param.Name)
			return expected, nil
		},
	}
	m := NewClusterManager(&fakeTxn{}, store, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)

	got, err := m.FetchClusterList(ctx, &ClusterFilter{Name: lib.PString("c1")})
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestClusterManager_FetchCluster(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		store := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{{ID: 1, Name: "c1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, store, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		got, err := m.FetchCluster(ctx, &ClusterFilter{Name: lib.PString("c1")})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, int64(1), got.ID)
	})

	t.Run("not found", func(t *testing.T) {
		store := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, store, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		got, err := m.FetchCluster(ctx, &ClusterFilter{Name: lib.PString("c1")})
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("error", func(t *testing.T) {
		store := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("db down")
			},
		}
		m := NewClusterManager(&fakeTxn{}, store, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		_, err := m.FetchCluster(ctx, &ClusterFilter{})
		require.Error(t, err)
	})
}

func TestClusterManager_CreateCluster(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	t.Run("success with existing sub cluster", func(t *testing.T) {
		bindCalled := false
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
			clusterCreateFn: func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
				assert.Equal(t, int64(2), product.ID)
				assert.Equal(t, []string{"sc1"}, param.SubClusters)
				return 10, nil
			},
			bindSubClusterFn: func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
				bindCalled = true
				assert.Equal(t, int64(10), cluster.ID)
				assert.Len(t, appendSubClusters, 1)
				assert.Equal(t, "sc1", appendSubClusters[0].Name)
				return nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				assert.Equal(t, []string{"sc1"}, param.Names)
				return []*SubCluster{{ID: 1, Name: "sc1", Ready: true}}, nil
			},
		}
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1"},
		})
		require.NoError(t, err)
		assert.True(t, bindCalled)
	})

	t.Run("cluster already existed", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{{Name: "c1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{Name: lib.PString("c1")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cluster Record Existed")
	})

	t.Run("success with auto instance pool", func(t *testing.T) {
		bindCalled := false
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
			clusterCreateFn: func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
				return 20, nil
			},
			bindSubClusterFn: func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
				bindCalled = true
				return nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				return []*SubCluster{{ID: 2, Name: "c1", Ready: true}}, nil
			},
		}
		poolStore := &fakePoolStorager{
			fetchPoolFn: func(ctx context.Context, name string) (*Pool, error) {
				return nil, nil
			},
			createPoolFn: func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
				assert.Equal(t, "test.c1", *data.Name)
				return &Pool{ID: 1, Name: "test.c1"}, nil
			},
		}
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: stateful.DefaultConfig.RunTime.DefaultAIClusterName}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, nil, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name: lib.PString("c1"),
			InstancePool: []Instance{
				{Name: "rs1", Addr: "127.0.0.1", Port: 80, Weight: 10},
			},
		})
		require.NoError(t, err)
		assert.True(t, bindCalled)
	})

	t.Run("EPP must single subcluster", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				return []*SubCluster{
					{ID: 1, Name: "sc1", Role: ProductPoolRoleEPP, Ready: true},
					{ID: 2, Name: "sc2", Role: ProductPoolRoleEPP, Ready: true},
				}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1", "sc2"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subcluster is EPP")
	})

	t.Run("sub cluster not exist", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				return []*SubCluster{{ID: 1, Name: "sc1", Ready: true}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1", "sc2"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SubCluster sc2 Not Exist")
	})

	t.Run("manual LB invalid", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				return []*SubCluster{{ID: 1, Name: "sc1", Ready: true}}, nil
			},
		}
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1"},
			Scheduler: map[string]map[string]int{
				"bfe1": {"sc1": 50},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Total Rate Is 50")
	})
}

func TestClusterManager_constructDefaultScheduler(t *testing.T) {
	ctx := context.Background()
	bfeClusterStore := &fakeBFEClusterStorager{
		fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
			return []*ibasic.BFECluster{{Name: "bfe1"}, {Name: "bfe2"}}, nil
		},
	}
	m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
	got, err := m.constructDefaultScheduler(ctx, []*SubCluster{{Name: "sc1"}, {Name: "sc2"}})
	require.NoError(t, err)
	assert.Equal(t, map[string]map[string]int{
		"bfe1": {"sc1": 50, "sc2": 50, BlackHole: 0},
		"bfe2": {"sc1": 50, "sc2": 50, BlackHole: 0},
	}, got)
}

func TestClusterManager_checkManualLB(t *testing.T) {
	ctx := context.Background()

	t.Run("nil scheduler", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkManualLB(ctx, nil, &ClusterParam{Name: lib.PString("c1")})
		require.NoError(t, err)
	})

	t.Run("valid scheduler", func(t *testing.T) {
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkManualLB(ctx, nil, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1"},
			Scheduler: map[string]map[string]int{
				"bfe1": {"sc1": 100},
			},
		})
		require.NoError(t, err)
	})

	t.Run("bfe cluster count mismatch", func(t *testing.T) {
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}, {Name: "bfe2"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkManualLB(ctx, nil, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1"},
			Scheduler: map[string]map[string]int{
				"bfe1": {"sc1": 100},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Want All BFE Cluster Exist")
	})

	t.Run("sub cluster not in scheduler", func(t *testing.T) {
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkManualLB(ctx, nil, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1"},
			Scheduler: map[string]map[string]int{
				"bfe1": {"sc2": 100},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SubCluster bfe1 Not In BFE Cluster sc2 Config")
	})

	t.Run("total not 100", func(t *testing.T) {
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkManualLB(ctx, nil, &ClusterParam{
			Name:        lib.PString("c1"),
			SubClusters: []string{"sc1"},
			Scheduler: map[string]map[string]int{
				"bfe1": {"sc1": 90},
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Total Rate Is 90")
	})

	t.Run("use old sub clusters", func(t *testing.T) {
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: "bfe1"}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, bfeClusterStore, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkManualLB(ctx, &Cluster{
			SubClusters: []*SubCluster{{Name: "sc1"}},
		}, &ClusterParam{
			Name: lib.PString("c1"),
			Scheduler: map[string]map[string]int{
				"bfe1": {"sc1": 100},
			},
		})
		require.NoError(t, err)
	})
}

func TestClusterManager_checkBindingSubClusters(t *testing.T) {
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkBindingSubClusters(ctx, nil, []string{}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Cluster Want At Least On SubCluster")
	})

	t.Run("mounted to other cluster", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkBindingSubClusters(ctx, &Cluster{ID: 1}, []string{"sc1"}, []*SubCluster{{ID: 1, Name: "sc1", ClusterID: 2, Ready: true}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "be Mounted With Cluster 2")
	})

	t.Run("not ready", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkBindingSubClusters(ctx, nil, []string{"sc1"}, []*SubCluster{{ID: 1, Name: "sc1", Ready: false}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Not Ready")
	})

	t.Run("success", func(t *testing.T) {
		stateful.IgnoreBNSStatusCheck = true
		defer func() { stateful.IgnoreBNSStatusCheck = false }()
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.checkBindingSubClusters(ctx, &Cluster{ID: 1}, []string{"sc1"}, []*SubCluster{{ID: 1, Name: "sc1", ClusterID: 1, Ready: false}})
		require.NoError(t, err)
	})
}

func TestClusterManager_UpdateCluster(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	t.Run("success", func(t *testing.T) {
		updated := false
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				updated = true
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, newTestClusterBase(), &ClusterParam{Name: lib.PString("c1")})
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("provider change syncs instance pool", func(t *testing.T) {
		poolUpdated := false
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				return nil
			},
		}
		poolStore := &fakePoolStorager{
			updatePoolFn: func(ctx context.Context, oldData *Pool, diff *PoolParam) error {
				poolUpdated = true
				assert.Equal(t, "p1", oldData.Name)
				return nil
			},
		}
		providerStore := &fakeProviderStorager{}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, poolStore, providerStore, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, newTestClusterBase(), &ClusterParam{
			Name: lib.PString("c1"),
			LLMConfig: &LLMConfig{
				Provider: lib.PString("openai"),
				Models:   []string{"m1"},
			},
		})
		require.NoError(t, err)
		assert.True(t, poolUpdated)
	})

	t.Run("EPP count invalid", func(t *testing.T) {
		c := newTestClusterEPP()
		c.SubClusters[0].Role = ProductPoolRoleEPP
		c.SubClusters = append(c.SubClusters, &SubCluster{ID: 2, Name: "sc2", Role: ProductPoolRoleEPP})
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, c, &ClusterParam{Name: lib.PString("c1")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "subcluster is EPP")
	})

	t.Run("update checker blocks model removal", func(t *testing.T) {
		checkerCalled := false
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, map[string]func(context.Context, *ibasic.Product, *Cluster, *ClusterParam) error{
			"route_rules": func(ctx context.Context, p *ibasic.Product, c *Cluster, param *ClusterParam) error {
				checkerCalled = true
				return errors.New("model in use")
			},
		})
		c := newTestClusterBase()
		c.LLMConfig = &LLMConfig{Models: []string{"m1", "m2"}}
		err := m.UpdateCluster(ctx, product, c, &ClusterParam{
			Name:      lib.PString("c1"),
			LLMConfig: &LLMConfig{Models: []string{"m1"}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model in use")
		assert.True(t, checkerCalled)
	})
}

func TestClusterManager_RebindSubCluster(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	t.Run("no change", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.RebindSubCluster(ctx, product, newTestClusterBase(), []string{"sc1"})
		require.NoError(t, err)
	})

	t.Run("unbind with non zero rate", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.RebindSubCluster(ctx, product, newTestClusterBase(), []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Rate is 100")
	})

	t.Run("append sub cluster", func(t *testing.T) {
		bindCalled := false
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				return nil
			},
			bindSubClusterFn: func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
				bindCalled = true
				assert.Len(t, appendSubClusters, 1)
				assert.Equal(t, "sc2", appendSubClusters[0].Name)
				return nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				return []*SubCluster{
					{ID: 1, Name: "sc1", ClusterID: 1, Ready: true},
					{ID: 2, Name: "sc2", Ready: true},
				}, nil
			},
		}
		c := newTestClusterBase()
		c.Scheduler["bfe1"]["sc2"] = 0
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.RebindSubCluster(ctx, product, c, []string{"sc1", "sc2"})
		require.NoError(t, err)
		assert.True(t, bindCalled)
	})
}

func TestClusterManager_DeleteCluster(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	t.Run("success", func(t *testing.T) {
		deleted := false
		bindCalled := false
		subClusterDeleted := false
		poolDeleted := false
		clusterStore := &fakeClusterStorager{
			bindSubClusterFn: func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
				bindCalled = true
				return nil
			},
			clusterDeleteFn: func(ctx context.Context, product *ibasic.Product, cluster *Cluster) error {
				deleted = true
				return nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			deleteSubClusterFn: func(ctx context.Context, param *SubCluster) error {
				subClusterDeleted = true
				return nil
			},
		}
		poolStore := &fakePoolStorager{
			deletePoolFn: func(ctx context.Context, pool *Pool) error {
				poolDeleted = true
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, &fakeBFEClusterStorager{}, poolStore, nil, nil, nil, nil)
		err := m.DeleteCluster(ctx, product, newTestClusterBase())
		require.NoError(t, err)
		assert.True(t, bindCalled)
		assert.True(t, deleted)
		assert.True(t, subClusterDeleted)
		assert.True(t, poolDeleted)
	})

	t.Run("delete checker error", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, map[string]func(context.Context, *ibasic.Product, *Cluster) error{
			"route": func(ctx context.Context, p *ibasic.Product, c *Cluster) error {
				return errors.New("route in use")
			},
		}, nil)
		err := m.DeleteCluster(ctx, product, newTestClusterBase())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "route in use")
	})
}

func TestClusterManager_IsBFEClusterUsed(t *testing.T) {
	ctx := context.Background()

	t.Run("used", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchLBMatrixListFn: func(ctx context.Context) (map[int64]map[string]map[string]int, error) {
				return map[int64]map[string]map[string]int{
					1: {"bfe1": {"sc1": 100}},
				}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		used, err := m.IsBFEClusterUsed(ctx, "bfe1")
		require.NoError(t, err)
		assert.True(t, used)
	})

	t.Run("not used", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchLBMatrixListFn: func(ctx context.Context) (map[int64]map[string]map[string]int, error) {
				return map[int64]map[string]map[string]int{
					1: {"bfe2": {"sc1": 100}},
				}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		used, err := m.IsBFEClusterUsed(ctx, "bfe1")
		require.NoError(t, err)
		assert.False(t, used)
	})
}

func TestAppendAdvancedRuleCluster(t *testing.T) {
	list := []*Cluster{{ID: 1, Name: "c1"}}
	got := AppendAdvancedRuleCluster(list)
	require.Len(t, got, 2)
	assert.Equal(t, RouteAdvancedModeClusterID, got[1].ID)
	assert.Equal(t, RouteAdvancedModeClusterName4DP, got[1].Name)
}

func TestNewBfeClusterConf(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterBase()}, nil, nil, nil, nil, nil, nil)
		assert.NoError(t, err)
		require.NotNil(t, conf)
		require.NotNil(t, conf.Config)
		assert.Equal(t, "v1", *conf.Version)
		cConf, ok := (*conf.Config)["c1"]
		require.True(t, ok)
		assert.NotNil(t, cConf.BackendConf)
		assert.NotNil(t, cConf.CheckConf)
		assert.NotNil(t, cConf.GslbBasic)
		assert.NotNil(t, cConf.ClusterBasic)
	})

	t.Run("skip system route", func(t *testing.T) {
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{
			newTestClusterBase(),
			{ID: RouteAdvancedModeClusterID, Name: RouteAdvancedModeClusterName},
		}, nil, nil, nil, nil, nil, nil)
		assert.NoError(t, err)
		require.Len(t, *conf.Config, 1)
	})

	t.Run("EPP addrs from assignment", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return []string{"10.0.0.1:9002", "10.0.0.2:9002"}, true, nil
			},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterEPP()}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		require.NotNil(t, cConf.GslbBasic.EPPAddr)
		assert.Equal(t, []string{"10.0.0.1:9002", "10.0.0.2:9002"}, *cConf.GslbBasic.EPPAddr)
		require.NotNil(t, cConf.GslbBasic.BalanceMode)
		assert.Equal(t, BalanceModeEPP, *cConf.GslbBasic.BalanceMode)
	})

	t.Run("EPP with epp_runtime exports check/timeout plus fixed TLS and breaker", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return []string{"10.0.0.1:9002", "10.0.0.2:9002"}, true, nil
			},
		}
		cluster := newTestClusterEPPEppRuntime(`{"check":{"disabled":true,"check_interval":"5s","fail_threshold":4,"cooldown":"30s","success_threshold":3},"timeout":{"connect":"1s","call":"4s"}}`)
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{cluster}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		g := (*conf.Config)["c1"].GslbBasic

		require.NotNil(t, g.EPPTLS)
		assert.True(t, g.EPPTLS.Plaintext)
		require.NotNil(t, g.EPPBreaker)
		assert.True(t, g.EPPBreaker.Disabled)
		require.NotNil(t, g.EPPCheck)
		assert.True(t, g.EPPCheck.Disabled)
		assert.Equal(t, "5s", *g.EPPCheck.CheckInterval)
		assert.Equal(t, 4, *g.EPPCheck.FailThreshold)
		assert.Equal(t, "30s", *g.EPPCheck.Cooldown)
		assert.Equal(t, 3, *g.EPPCheck.SuccessThreshold)
		require.NotNil(t, g.EPPTimeout)
		assert.Equal(t, "1s", *g.EPPTimeout.Connect)
		assert.Equal(t, "4s", *g.EPPTimeout.Call)
	})

	t.Run("EPP without epp_runtime exports only fixed TLS and breaker", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return []string{"10.0.0.1:9002", "10.0.0.2:9002"}, true, nil
			},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterEPP()}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		g := (*conf.Config)["c1"].GslbBasic

		require.NotNil(t, g.EPPTLS)
		assert.True(t, g.EPPTLS.Plaintext)
		require.NotNil(t, g.EPPBreaker)
		assert.True(t, g.EPPBreaker.Disabled)
		assert.Nil(t, g.EPPCheck)
		assert.Nil(t, g.EPPTimeout)
	})

	t.Run("EPP without assignment exports no EPP runtime fields", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return nil, false, nil
			},
		}
		cluster := newTestClusterEPPEppRuntime(`{"timeout":{"connect":"1s"}}`)
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{cluster}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		g := (*conf.Config)["c1"].GslbBasic

		require.NotNil(t, g.BalanceMode)
		assert.Equal(t, BalanceModeWRR, *g.BalanceMode)
		assert.Nil(t, g.EPPTLS)
		assert.Nil(t, g.EPPCheck)
		assert.Nil(t, g.EPPTimeout)
		assert.Nil(t, g.EPPBreaker)
	})

	t.Run("WRR never exports EPP runtime fields", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return []string{"10.0.0.1:9002"}, true, nil
			},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterBase()}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		g := (*conf.Config)["c1"].GslbBasic

		assert.Nil(t, g.EPPTLS)
		assert.Nil(t, g.EPPCheck)
		assert.Nil(t, g.EPPTimeout)
		assert.Nil(t, g.EPPBreaker)
	})

	t.Run("EPP without assignment degrades to WRR", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return nil, false, nil
			},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterEPP()}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		assert.Nil(t, cConf.GslbBasic.EPPAddr)
		require.NotNil(t, cConf.GslbBasic.BalanceMode)
		assert.Equal(t, BalanceModeWRR, *cConf.GslbBasic.BalanceMode)
	})

	t.Run("EPP assignment lookup error degrades to WRR", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return nil, false, errors.New("db down")
			},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterEPP()}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		assert.Nil(t, cConf.GslbBasic.EPPAddr)
		require.NotNil(t, cConf.GslbBasic.BalanceMode)
		assert.Equal(t, BalanceModeWRR, *cConf.GslbBasic.BalanceMode)
	})

	t.Run("EPP without resolver degrades to WRR", func(t *testing.T) {
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterEPP()}, nil, nil, nil, nil, nil, nil)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		assert.Nil(t, cConf.GslbBasic.EPPAddr)
		require.NotNil(t, cConf.GslbBasic.BalanceMode)
		assert.Equal(t, BalanceModeWRR, *cConf.GslbBasic.BalanceMode)
	})

	t.Run("WRR cluster never exports EPPAddr even when assigned", func(t *testing.T) {
		resolver := &fakeEPPAssignmentResolver{
			endpointsFn: func(ctx context.Context, clusterName string) ([]string, bool, error) {
				return []string{"10.0.0.1:9002"}, true, nil
			},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterBase()}, nil, nil, nil, nil, nil, resolver)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		assert.Nil(t, cConf.GslbBasic.EPPAddr)
		require.NotNil(t, cConf.GslbBasic.BalanceMode)
		assert.Equal(t, BalanceModeWRR, *cConf.GslbBasic.BalanceMode)
	})

	t.Run("https conf", func(t *testing.T) {
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterHTTPS()}, nil, nil, nil, nil, nil, nil)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		require.NotNil(t, cConf.HTTPSConf)
		assert.True(t, *cConf.HTTPSConf.RSInsecureSkipVerify)
	})

	t.Run("domain pool disable checks", func(t *testing.T) {
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterDomain()}, nil, nil, nil, nil, nil, nil)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		assert.True(t, *cConf.ClusterBasic.DisableHealthCheck)
		assert.True(t, *cConf.ClusterBasic.DisableHostHeader)
	})

	t.Run("normalize_upstream_error three states", func(t *testing.T) {
		providerKeyTable := map[string][]iprovider.ProviderKey{
			"openai": {
				{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"},
			},
		}
		providerProtocolTable := map[string][]string{
			"openai": {"openai"},
		}

		exportNue := func(c *Cluster) *cluster_conf.UpstreamErrorNormalizeConf {
			conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{c}, nil,
				providerKeyTable, providerProtocolTable, nil, nil, nil)
			assert.NoError(t, err)
			return (*conf.Config)["c1"].AIConf.NormalizeUpstreamError
		}

		// state 1: unset -> field absent (BFE disabled)
		assert.Nil(t, exportNue(newTestClusterLLMWithoutNue()))

		// state 2: partial fields -> unset fields export as zero values
		partial := newTestClusterLLMWithoutNue()
		partial.LLMConfig.NormalizeUpstreamError = &NormalizeUpstreamError{
			Enabled: lib.PBool(true),
		}
		nue := exportNue(partial)
		require.NotNil(t, nue)
		assert.True(t, nue.Enabled)
		assert.False(t, nue.StreamEnabled)
		assert.Equal(t, "", nue.UnrecognizedAction)
		assert.Equal(t, int64(0), nue.MaxBodyBytes)
		assert.Nil(t, nue.RedactSecrets) // unset stays nil: BFE defaults to true

		// state 3: explicit redact_secrets=false must survive the export
		disabled := newTestClusterLLMWithoutNue()
		disabled.LLMConfig.NormalizeUpstreamError = &NormalizeUpstreamError{
			Enabled:       lib.PBool(true),
			RedactSecrets: lib.PBool(false),
		}
		nue = exportNue(disabled)
		require.NotNil(t, nue)
		require.NotNil(t, nue.RedactSecrets)
		assert.False(t, *nue.RedactSecrets)
	})

	t.Run("price_miss_action export (issue #1479)", func(t *testing.T) {
		providerKeyTable := map[string][]iprovider.ProviderKey{
			"openai": {{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"}},
		}
		providerProtocolTable := map[string][]string{"openai": {"openai"}}

		exportAction := func(c *Cluster) string {
			conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{c}, nil,
				providerKeyTable, providerProtocolTable, nil, nil, nil)
			assert.NoError(t, err)
			return (*conf.Config)["c1"].AIConf.PriceMissAction
		}

		// unset -> empty string; BFE normalizes it to fallback_chat.
		assert.Equal(t, "", exportAction(newTestClusterLLMWithoutNue()))

		set := newTestClusterLLMWithoutNue()
		set.LLMConfig.PriceMissAction = lib.PString("reject")
		assert.Equal(t, "reject", exportAction(set))
	})

	t.Run("LLM config", func(t *testing.T) {
		providerKeyTable := map[string][]iprovider.ProviderKey{
			"openai": {
				{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"},
				{Name: "key-secondary", Key: "sk-bbbbbbbbbbbb"},
			},
		}
		providerProtocolTable := map[string][]string{
			"openai": {"openai"},
		}
		providerProtocolPathsTable := map[string]map[string]string{
			"openai": {"openai": "/compatible-mode/v1"},
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{newTestClusterLLM()}, nil, providerKeyTable, providerProtocolTable, providerProtocolPathsTable, nil, nil)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		require.NotNil(t, cConf.AIConf)
		require.Len(t, cConf.AIConf.Keys, 2)
		assert.Equal(t, "key-primary", cConf.AIConf.Keys[0].Name)
		assert.Equal(t, "sk-aaaaaaaaaaaa", cConf.AIConf.Keys[0].Key)
		assert.Equal(t, 70, cConf.AIConf.Keys[0].Weight)
		assert.Equal(t, "key-secondary", cConf.AIConf.Keys[1].Name)
		assert.Equal(t, "sk-bbbbbbbbbbbb", cConf.AIConf.Keys[1].Key)
		assert.Equal(t, 30, cConf.AIConf.Keys[1].Weight)
		require.NotNil(t, cConf.AIConf.KeyPolicy)
		assert.Equal(t, "weighted_random", cConf.AIConf.KeyPolicy.Strategy)
		assert.Equal(t, 3, cConf.AIConf.KeyPolicy.MaxRetries)
		assert.Equal(t, 500, cConf.AIConf.KeyPolicy.RetryBackoffInitial)
		assert.Equal(t, 5000, cConf.AIConf.KeyPolicy.RetryBackoffMax)
		assert.True(t, cConf.AIConf.KeyPolicy.SessionAffinity)
		assert.Equal(t, 600, cConf.AIConf.KeyPolicy.SessionAffinityTTL)
		assert.Equal(t, "bfe:ai:key_affinity", cConf.AIConf.KeyPolicy.SessionAffinityRedisPrefix)
		assert.True(t, cConf.AIConf.KeyPolicy.SessionAffinityPenaltyEnable)
		require.NotNil(t, cConf.AIConf.ModelMapping)
		assert.Equal(t, "new", (*cConf.AIConf.ModelMapping)["old"])
		assert.Equal(t, "openrouter/", cConf.AIConf.MatchPrefix)
		assert.True(t, cConf.AIConf.StripPrefix)
		assert.Equal(t, []string{"openai"}, cConf.AIConf.ModelProtocols)
		assert.Equal(t, map[string]string{"openai": "/compatible-mode/v1"}, cConf.AIConf.ProtocolPaths)
		require.NotNil(t, cConf.AIConf.NormalizeUpstreamError)
		assert.True(t, cConf.AIConf.NormalizeUpstreamError.Enabled)
		assert.True(t, cConf.AIConf.NormalizeUpstreamError.StreamEnabled)
		assert.Equal(t, "passthrough", cConf.AIConf.NormalizeUpstreamError.UnrecognizedAction)
		assert.Equal(t, int64(65536), cConf.AIConf.NormalizeUpstreamError.MaxBodyBytes)
		require.NotNil(t, cConf.AIConf.NormalizeUpstreamError.RedactSecrets)
		assert.True(t, *cConf.AIConf.NormalizeUpstreamError.RedactSecrets)
	})

	t.Run("LLM config with tiered pricing", func(t *testing.T) {
		providerModelTable := map[string][]*imodel_price.ModelPrice{
			"deepseek": {
				{
					Provider:  "deepseek",
					Model:     "deepseek-v4-pro",
					BaseModel: "deepseek-v4-pro",
					Mode:      "chat",
					Prices: imodel_price.PriceMap{
						"input_cost_per_token":                   0.0000045,
						"output_cost_per_token":                  0.0000135,
						"cache_read_input_token_cost":            0.00000015,
						"cache_creation_input_token_cost_1h":     0.0000002,
						"input_cost_per_token_above_256k_tokens": 0.000009,
					},
					TierPrices: imodel_price.TierPriceMap{
						"peak": {
							"input_cost_per_token":        0.000009,
							"output_cost_per_token":       0.000027,
							"cache_read_input_token_cost": 0.0000003,
						},
					},
				},
			},
		}
		providerPricingTable := map[string]ProviderPricingInfo{
			"deepseek": {
				TimeZone: "Asia/Shanghai",
				Tiers: []cluster_conf.PriceTier{
					{
						Name: "peak",
						TimeRanges: []cluster_conf.TimeRange{
							{Weekdays: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "12:00"},
							{Weekdays: []int{1, 2, 3, 4, 5}, Start: "14:00", End: "18:00"},
						},
					},
				},
			},
		}
		c := newTestClusterLLM()
		c.LLMConfig.Provider = lib.PString("deepseek")
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{c}, providerModelTable, nil, nil, nil, providerPricingTable, nil)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		require.NotNil(t, cConf.AIConf)
		require.NotNil(t, cConf.AIConf.ModelTable)
		assert.Equal(t, "RMB", cConf.AIConf.ModelTable.Currency)
		assert.Equal(t, "Asia/Shanghai", cConf.AIConf.ModelTable.TimeZone)
		require.Len(t, cConf.AIConf.ModelTable.Tiers, 1)
		assert.Equal(t, "peak", cConf.AIConf.ModelTable.Tiers[0].Name)
		require.Len(t, cConf.AIConf.ModelTable.Models, 1)
		model := cConf.AIConf.ModelTable.Models[0]
		assert.Equal(t, "deepseek-v4-pro", model.Model)
		require.NotNil(t, model.TierPrices)
		assert.Equal(t, 0.000009, model.TierPrices["peak"]["input_cost_per_token"])
		assert.Equal(t, 0.0000002, model.Prices["cache_creation_input_token_cost_1h"])
		assert.Equal(t, 0.000009, model.Prices["input_cost_per_token_above_256k_tokens"])
	})

	t.Run("disabled sticky sessions with empty hash_header falls back to CLIENT_IP_ONLY", func(t *testing.T) {
		c := newTestClusterBase()
		c.StickySessions = &ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  ClusterHashStrategyClientIDOnlyI,
			HashHeader:    "",
		}
		conf, err := NewBfeClusterConf(context.Background(), "v1", []*Cluster{c}, nil, nil, nil, nil, nil, nil)
		assert.NoError(t, err)
		cConf := (*conf.Config)["c1"]
		require.NotNil(t, cConf.GslbBasic)
		require.NotNil(t, cConf.GslbBasic.HashConf)
		require.NotNil(t, cConf.GslbBasic.HashConf.HashStrategy)
		assert.Equal(t, ClusterHashStrategyClientIPOnlyI, int32(*cConf.GslbBasic.HashConf.HashStrategy))
	})
}

func TestNormalizeBFEHashStrategy(t *testing.T) {
	t.Run("nil sticky sessions", func(t *testing.T) {
		assert.Equal(t, ClusterHashStrategyClientIPOnlyI, normalizeBFEHashStrategy(nil))
	})

	t.Run("disabled CLIENT_ID_ONLY with empty header", func(t *testing.T) {
		assert.Equal(t, ClusterHashStrategyClientIPOnlyI, normalizeBFEHashStrategy(&ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  ClusterHashStrategyClientIDOnlyI,
			HashHeader:    "",
		}))
	})

	t.Run("disabled CLIENT_ID_PREFERRED with empty header", func(t *testing.T) {
		assert.Equal(t, ClusterHashStrategyClientIPOnlyI, normalizeBFEHashStrategy(&ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  ClusterHashStrategyClientIDPreferedI,
			HashHeader:    "",
		}))
	})

	t.Run("disabled CLIENT_ID_ONLY with non-empty header stays", func(t *testing.T) {
		assert.Equal(t, ClusterHashStrategyClientIDOnlyI, normalizeBFEHashStrategy(&ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  ClusterHashStrategyClientIDOnlyI,
			HashHeader:    "Cookie:USERID",
		}))
	})

	t.Run("enabled CLIENT_ID_ONLY stays", func(t *testing.T) {
		assert.Equal(t, ClusterHashStrategyClientIDOnlyI, normalizeBFEHashStrategy(&ClusterStickySessions{
			SessionSticky: true,
			HashStrategy:  ClusterHashStrategyClientIDOnlyI,
			HashHeader:    "Cookie:USERID",
		}))
	})

	t.Run("disabled CLIENT_IP_ONLY stays", func(t *testing.T) {
		assert.Equal(t, ClusterHashStrategyClientIPOnlyI, normalizeBFEHashStrategy(&ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  ClusterHashStrategyClientIPOnlyI,
			HashHeader:    "",
		}))
	})
}

func TestConvertToBFEModelMapping(t *testing.T) {
	t.Run("non empty", func(t *testing.T) {
		got := convertToBFEModelMapping([]*Mapping{
			{SourceModel: lib.PString("k1"), TargetModel: lib.PString("v1")},
		})
		require.NotNil(t, got)
		assert.Equal(t, map[string]string{"k1": "v1"}, *got)
	})

	t.Run("empty", func(t *testing.T) {
		got := convertToBFEModelMapping(nil)
		assert.Nil(t, got)
	})
}

func TestIsDomainPool(t *testing.T) {
	t.Run("IP", func(t *testing.T) {
		assert.False(t, isDomainPool(newTestClusterBase().SubClusters))
	})

	t.Run("domain", func(t *testing.T) {
		assert.True(t, isDomainPool(newTestClusterDomain().SubClusters))
	})
}

func TestClusterPassiveHealthCheck_toBackendCheck(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var c *ClusterPassiveHealthCheck
		assert.Nil(t, c.toBackendCheck())
	})

	t.Run("values", func(t *testing.T) {
		c := &ClusterPassiveHealthCheck{
			Schema:     "http",
			Interval:   10,
			Failnum:    3,
			Statuscode: 500,
			Host:       "h",
			Uri:        "/",
		}
		got := c.toBackendCheck()
		require.NotNil(t, got)
		assert.Equal(t, "http", *got.Schem)
	})
}

func TestClusterTableConf_UpdateVersion(t *testing.T) {
	ctc := &ClusterTableConf{}
	require.NoError(t, ctc.UpdateVersion("v1"))
	assert.Equal(t, "v1", *ctc.Version)
}

func TestGSLBConf_UpdateVersion(t *testing.T) {
	gc := &GSLBConf{}
	require.NoError(t, gc.UpdateVersion("v1"))
	assert.Equal(t, "v1", gc.Version)
	assert.Equal(t, "v1", *gc.Ts)
}

func newClusterManagerForExport(t *testing.T, version string) *ClusterManager {
	t.Helper()
	vcs := &fakeVersionControlStorager{
		upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
			return version, nil
		},
	}
	vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, vcs)
	clusterStore := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			return []*Cluster{newTestClusterBase()}, nil
		},
	}
	return NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, vcm, nil, nil)
}

type fakeOperationLogRecorder struct {
	entries []*ioperlog.OperationLogEntry
}

func (r *fakeOperationLogRecorder) Record(ctx context.Context, entry *ioperlog.OperationLogEntry) {
	r.entries = append(r.entries, entry)
}

func TestClusterManager_DeleteCluster_RecordsFailedOperationLog(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}
	recorder := &fakeOperationLogRecorder{}

	m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, map[string]func(context.Context, *ibasic.Product, *Cluster) error{
		"route": func(ctx context.Context, p *ibasic.Product, c *Cluster) error {
			return errors.New("route in use")
		},
	}, nil)
	m.SetOperationLogManager(recorder)

	err := m.DeleteCluster(ctx, product, newTestClusterBase())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "route in use")

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionDelete), entry.Action)
	assert.Equal(t, string(ioperlog.ResourceTypeCluster), entry.ResourceType)
	assert.Equal(t, ioperlog.StatusFailed, entry.Status)
	assert.Contains(t, entry.ErrorMsg, "route in use")
}
