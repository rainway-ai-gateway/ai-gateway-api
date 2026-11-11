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

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/epp_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeEppPoolStorager struct {
	fetchInstanceListFn   func(ctx context.Context, filter *epp_pool.InstanceFilter) ([]*epp_pool.InstanceParam, error)
	createInstancesFn     func(ctx context.Context, params []*epp_pool.InstanceParam) (int64, error)
	deleteInstancesFn     func(ctx context.Context, filter *epp_pool.InstanceFilter) (int64, error)
	fetchAssignmentFn     func(ctx context.Context, cluster string) (*epp_pool.AssignmentParam, error)
	fetchAssignmentListFn func(ctx context.Context) ([]*epp_pool.AssignmentParam, error)
	upsertAssignmentFn    func(ctx context.Context, param *epp_pool.AssignmentParam) error
	deleteAssignmentFn    func(ctx context.Context, cluster string) error
}

func (f *fakeEppPoolStorager) FetchInstanceList(ctx context.Context, filter *epp_pool.InstanceFilter) ([]*epp_pool.InstanceParam, error) {
	if f.fetchInstanceListFn != nil {
		return f.fetchInstanceListFn(ctx, filter)
	}
	return nil, nil
}

func (f *fakeEppPoolStorager) CreateInstances(ctx context.Context, params []*epp_pool.InstanceParam) (int64, error) {
	if f.createInstancesFn != nil {
		return f.createInstancesFn(ctx, params)
	}
	return 0, nil
}

func (f *fakeEppPoolStorager) DeleteInstances(ctx context.Context, filter *epp_pool.InstanceFilter) (int64, error) {
	if f.deleteInstancesFn != nil {
		return f.deleteInstancesFn(ctx, filter)
	}
	return 0, nil
}

func (f *fakeEppPoolStorager) FetchAssignment(ctx context.Context, cluster string) (*epp_pool.AssignmentParam, error) {
	if f.fetchAssignmentFn != nil {
		return f.fetchAssignmentFn(ctx, cluster)
	}
	return nil, nil
}

func (f *fakeEppPoolStorager) FetchAssignmentList(ctx context.Context) ([]*epp_pool.AssignmentParam, error) {
	if f.fetchAssignmentListFn != nil {
		return f.fetchAssignmentListFn(ctx)
	}
	return nil, nil
}

func (f *fakeEppPoolStorager) UpsertAssignment(ctx context.Context, param *epp_pool.AssignmentParam) error {
	if f.upsertAssignmentFn != nil {
		return f.upsertAssignmentFn(ctx, param)
	}
	return nil
}

func (f *fakeEppPoolStorager) DeleteAssignment(ctx context.Context, cluster string) error {
	if f.deleteAssignmentFn != nil {
		return f.deleteAssignmentFn(ctx, cluster)
	}
	return nil
}

// newFakeEppPoolManager builds an EppPoolManager (test validation mode) whose
// assignment upserts are recorded in assigned.
func newFakeEppPoolManager(assigned map[string]*epp_pool.AssignmentParam, storagerErr error) *epp_pool.EppPoolManager {
	storager := &fakeEppPoolStorager{
		fetchInstanceListFn: func(ctx context.Context, filter *epp_pool.InstanceFilter) ([]*epp_pool.InstanceParam, error) {
			if storagerErr != nil {
				return nil, storagerErr
			}
			return []*epp_pool.InstanceParam{
				{ID: "epp-a", Host: "10.0.0.1", Port: 9002, GroupName: "g1"},
			}, nil
		},
		fetchAssignmentListFn: func(ctx context.Context) ([]*epp_pool.AssignmentParam, error) {
			if storagerErr != nil {
				return nil, storagerErr
			}
			rst := []*epp_pool.AssignmentParam{}
			for _, a := range assigned {
				rst = append(rst, a)
			}
			return rst, nil
		},
		upsertAssignmentFn: func(ctx context.Context, param *epp_pool.AssignmentParam) error {
			assigned[param.Cluster] = param
			return storagerErr
		},
	}
	return epp_pool.NewEppPoolManager(&fakeTxn{}, storager, nil, nil, nil)
}

const validEppConfigJSON = `{"load_profile":"balanced","kv_cache_utilization_max":0.9}`

func TestValidateClusterBalanceConfig(t *testing.T) {
	t.Run("default WRR without any input", func(t *testing.T) {
		mode, raw, err := validateClusterBalanceConfig(nil, "", nil, "")
		require.NoError(t, err)
		assert.Equal(t, BalanceModeWRR, mode)
		assert.Equal(t, "", raw)
	})

	t.Run("invalid balance_mode", func(t *testing.T) {
		_, _, err := validateClusterBalanceConfig(lib.PString("ROUND_ROBIN"), "", nil, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "balance_mode must be one of")
	})

	t.Run("EPP requires epp_config", func(t *testing.T) {
		for _, cfg := range []*string{nil, lib.PString(""), lib.PString("  "), lib.PString("{}")} {
			_, _, err := validateClusterBalanceConfig(lib.PString(BalanceModeEPP), "", cfg, "")
			require.Error(t, err, "cfg=%v", cfg)
			assert.Contains(t, err.Error(), "epp_config is required")
		}
	})

	t.Run("EPP with valid epp_config", func(t *testing.T) {
		mode, raw, err := validateClusterBalanceConfig(lib.PString(BalanceModeEPP), "", lib.PString(validEppConfigJSON), "")
		require.NoError(t, err)
		assert.Equal(t, BalanceModeEPP, mode)
		assert.Equal(t, validEppConfigJSON, raw)
	})

	t.Run("WRR accepts hibernated epp_config", func(t *testing.T) {
		mode, raw, err := validateClusterBalanceConfig(nil, "", lib.PString(validEppConfigJSON), "")
		require.NoError(t, err)
		assert.Equal(t, BalanceModeWRR, mode)
		assert.Equal(t, validEppConfigJSON, raw)
	})

	t.Run("non-empty epp_config validated regardless of balance_mode", func(t *testing.T) {
		for _, cfg := range []string{
			`{"load_profile":"bogus"}`,
			`{"affinity":"extreme"}`,
			`{"kv_cache_utilization_max":1.5}`,
			`{"waiting_queue_max":-1}`,
			`{"metrics_staleness_threshold_ms":0}`,
			`{"unknown_field":true}`,
			`{"scheduling_profile":"balanced"}`,
			`{"cache_affinity":"low"}`,
			`{"flow_control":{"queue_ttl":-1}}`,
			`{"flow_control":{"enable_eviction":true}}`,
			`{"session_affinity_enabled":true}`,
		} {
			_, _, err := validateClusterBalanceConfig(nil, BalanceModeWRR, lib.PString(cfg), "")
			require.Error(t, err, "cfg=%s", cfg)
		}
	})

	t.Run("update keeps old mode and stored epp_config when not carried", func(t *testing.T) {
		mode, raw, err := validateClusterBalanceConfig(nil, BalanceModeEPP, nil, validEppConfigJSON)
		require.NoError(t, err)
		assert.Equal(t, BalanceModeEPP, mode)
		assert.Equal(t, validEppConfigJSON, raw)
	})

	t.Run("update to EPP reuses hibernated epp_config", func(t *testing.T) {
		mode, raw, err := validateClusterBalanceConfig(lib.PString(BalanceModeEPP), BalanceModeWRR, nil, validEppConfigJSON)
		require.NoError(t, err)
		assert.Equal(t, BalanceModeEPP, mode)
		assert.Equal(t, validEppConfigJSON, raw)
	})

	t.Run("carried epp_config is trimmed", func(t *testing.T) {
		_, raw, err := validateClusterBalanceConfig(nil, "", lib.PString("  "+validEppConfigJSON+"  "), "")
		require.NoError(t, err)
		assert.Equal(t, validEppConfigJSON, raw)
	})
}

func eppCreateClusterStores() (*fakeClusterStorager, *fakeSubClusterStorager, *fakePoolStorager, *fakeProviderStorager) {
	clusterStore := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			return nil, nil
		},
		clusterCreateFn: func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
			return 20, nil
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
	}
	providerStore := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			return &iprovider.Provider{
				Name: "deepseek",
				Models: []string{
					"m1",
				},
				InstancePool: []iprovider.ProviderInstance{
					{Addr: "5.6.7.8", Port: 443, Weight: 100},
				},
			}, nil
		},
	}
	return clusterStore, subClusterStore, poolStore, providerStore
}

func TestClusterManager_CreateCluster_EPP(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}
	bfeClusterStore := &fakeBFEClusterStorager{
		fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
			return []*ibasic.BFECluster{{Name: stateful.DefaultConfig.RunTime.DefaultAIClusterName}}, nil
		},
	}
	llmConfig := func() *LLMConfig {
		return &LLMConfig{Provider: lib.PString("deepseek"), Models: []string{"m1"}}
	}

	t.Run("EPP creates Role=EPP pool with provider instances synced and assigns", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		poolCreated := false
		clusterWritten := false
		poolStore.createPoolFn = func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
			poolCreated = true
			require.NotNil(t, data.Role)
			assert.Equal(t, ProductPoolRoleEPP, *data.Role)
			require.Len(t, data.Instances, 1)
			assert.Equal(t, "5.6.7.8", data.Instances[0].Addr)
			return &Pool{ID: 1, Name: *data.Name, Role: *data.Role, Instances: data.Instances}, nil
		}
		clusterStore.clusterCreateFn = func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
			clusterWritten = true
			require.NotNil(t, param.BalanceMode)
			assert.Equal(t, BalanceModeEPP, *param.BalanceMode)
			require.NotNil(t, param.EppConfig)
			assert.Equal(t, validEppConfigJSON, *param.EppConfig)
			return 20, nil
		}

		assigned := map[string]*epp_pool.AssignmentParam{}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, nil))

		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
			EppConfig:   lib.PString(validEppConfigJSON),
			LLMConfig:   llmConfig(),
		})
		require.NoError(t, err)
		assert.True(t, poolCreated)
		assert.True(t, clusterWritten)
		require.Contains(t, assigned, "c1")
		assert.Equal(t, "g1", assigned["c1"].GroupName)
	})

	t.Run("WRR keeps COMMON pool with provider instances", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		poolStore.createPoolFn = func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
			require.NotNil(t, data.Role)
			assert.Equal(t, ProductPoolRoleCommon, *data.Role)
			require.Len(t, data.Instances, 1)
			return &Pool{ID: 1, Name: *data.Name, Role: *data.Role}, nil
		}

		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:      lib.PString("c1"),
			LLMConfig: llmConfig(),
		})
		require.NoError(t, err)
	})

	t.Run("EPP without epp_config rejected", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
			LLMConfig:   llmConfig(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "epp_config is required")
	})

	t.Run("WRR with invalid epp_config rejected", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:      lib.PString("c1"),
			EppConfig: lib.PString(`{"load_profile":"bogus"}`),
			LLMConfig: llmConfig(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "load_profile")
	})

	t.Run("assignment failure does not block create", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		poolStore.createPoolFn = func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
			return &Pool{ID: 1, Name: *data.Name, Role: *data.Role}, nil
		}

		assigned := map[string]*epp_pool.AssignmentParam{}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, errors.New("epp storage down")))

		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
			EppConfig:   lib.PString(validEppConfigJSON),
			LLMConfig:   llmConfig(),
		})
		require.NoError(t, err)
		assert.Empty(t, assigned)
	})
}

func TestClusterManager_UpdateCluster_EPP(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	wrrCluster := func() *Cluster {
		c := newTestClusterBase()
		c.ID = 1
		return c
	}

	t.Run("WRR to EPP assigns after successful write", func(t *testing.T) {
		assigned := map[string]*epp_pool.AssignmentParam{}
		updated := false
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				updated = true
				require.NotNil(t, param.BalanceMode)
				assert.Equal(t, BalanceModeEPP, *param.BalanceMode)
				require.NotNil(t, param.EppConfig)
				assert.Equal(t, validEppConfigJSON, *param.EppConfig)
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, nil))

		err := m.UpdateCluster(ctx, product, wrrCluster(), &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
			EppConfig:   lib.PString(validEppConfigJSON),
		})
		require.NoError(t, err)
		assert.True(t, updated)
		require.Contains(t, assigned, "c1")
	})

	t.Run("WRR to EPP without epp_config rejected", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, wrrCluster(), &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "epp_config is required")
	})

	t.Run("WRR to EPP reuses hibernated epp_config", func(t *testing.T) {
		assigned := map[string]*epp_pool.AssignmentParam{}
		old := wrrCluster()
		old.EppConfig = validEppConfigJSON
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				assert.Nil(t, param.EppConfig) // not carried: stored value retained
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, nil))

		err := m.UpdateCluster(ctx, product, old, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
		})
		require.NoError(t, err)
		require.Contains(t, assigned, "c1")
	})

	t.Run("assignment failure does not block update", func(t *testing.T) {
		assigned := map[string]*epp_pool.AssignmentParam{}
		updated := false
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				updated = true
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, errors.New("epp storage down")))

		err := m.UpdateCluster(ctx, product, wrrCluster(), &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
			EppConfig:   lib.PString(validEppConfigJSON),
		})
		require.NoError(t, err)
		assert.True(t, updated)
		assert.Empty(t, assigned)
	})

	t.Run("EPP to WRR keeps config dormant and does not assign", func(t *testing.T) {
		assigned := map[string]*epp_pool.AssignmentParam{}
		old := wrrCluster()
		old.BalanceMode = BalanceModeEPP
		old.EppConfig = validEppConfigJSON
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				require.NotNil(t, param.BalanceMode)
				assert.Equal(t, BalanceModeWRR, *param.BalanceMode)
				assert.Nil(t, param.EppConfig) // dormant retention: not carried, not written
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, nil))

		err := m.UpdateCluster(ctx, product, old, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeWRR),
		})
		require.NoError(t, err)
		assert.Empty(t, assigned)
	})

	t.Run("EPP to WRR still validates carried epp_config", func(t *testing.T) {
		old := wrrCluster()
		old.BalanceMode = BalanceModeEPP
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, old, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeWRR),
			EppConfig:   lib.PString(`{"kv_cache_utilization_max":2}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "kv_cache_utilization_max")
	})
}

func TestClusterManager_FetchEPPClusters(t *testing.T) {
	ctx := context.Background()

	t.Run("filters EPP clusters with raw epp_config", func(t *testing.T) {
		eppCluster := newTestClusterEPP()
		eppCluster.Name = "c-epp"
		wrrCluster := newTestClusterBase()
		wrrCluster.Name = "c-wrr"
		wrrCluster.EppConfig = validEppConfigJSON
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{eppCluster, wrrCluster}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)

		got, err := m.FetchEPPClusters(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "c-epp", got[0].Name)
		assert.Equal(t, eppCluster.EppConfig, got[0].EppConfigJSON)
	})

	t.Run("storager error propagates", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("db down")
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		_, err := m.FetchEPPClusters(ctx)
		require.Error(t, err)
	})
}

func TestClusterManager_assignClusterToEPP(t *testing.T) {
	ctx := context.Background()

	t.Run("nil manager logs and returns", func(t *testing.T) {
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		// Must not panic; the assignment is deferred to the reconciler.
		m.assignClusterToEPP(ctx, "c1")
	})

	t.Run("manager error does not panic", func(t *testing.T) {
		assigned := map[string]*epp_pool.AssignmentParam{}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(assigned, errors.New("epp storage down")))
		m.assignClusterToEPP(ctx, "c1")
		assert.Empty(t, assigned)
	})
}

func TestClusterManager_CreateCluster_EppRuntime(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}
	bfeClusterStore := &fakeBFEClusterStorager{
		fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
			return []*ibasic.BFECluster{{Name: stateful.DefaultConfig.RunTime.DefaultAIClusterName}}, nil
		},
	}
	llmConfig := func() *LLMConfig {
		return &LLMConfig{Provider: lib.PString("deepseek"), Models: []string{"m1"}}
	}

	t.Run("EPP carries defaulted epp_runtime", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		var captured *string
		clusterStore.clusterCreateFn = func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
			captured = param.EppRuntime
			return 20, nil
		}
		poolStore.createPoolFn = func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
			return &Pool{ID: 1, Name: *data.Name, Role: *data.Role}, nil
		}

		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		m.SetEppPoolManager(newFakeEppPoolManager(map[string]*epp_pool.AssignmentParam{}, nil))

		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeEPP),
			EppConfig:   lib.PString(validEppConfigJSON),
			EppRuntime:  lib.PString(`{"timeout":{"connect":"1s"}}`),
			LLMConfig:   llmConfig(),
		})
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.JSONEq(t, `{"timeout":{"connect":"1s","call":"11s"}}`, *captured)
	})

	t.Run("WRR keeps dormant epp_runtime and validates it", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		var captured *string
		clusterStore.clusterCreateFn = func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
			captured = param.EppRuntime
			return 20, nil
		}

		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:       lib.PString("c1"),
			EppRuntime: lib.PString(`{"check":{"check_interval":"5s"}}`),
			LLMConfig:  llmConfig(),
		})
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Contains(t, *captured, `"check_interval":"5s"`)
	})

	t.Run("invalid epp_runtime rejected before write", func(t *testing.T) {
		clusterStore, subClusterStore, poolStore, providerStore := eppCreateClusterStores()
		written := false
		clusterStore.clusterCreateFn = func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
			written = true
			return 20, nil
		}

		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore, providerStore, nil, nil, nil)
		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name:       lib.PString("c1"),
			EppRuntime: lib.PString(`{"check":{"fail_threshold":0}}`),
			LLMConfig:  llmConfig(),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fail_threshold")
		assert.False(t, written)
	})
}

func TestClusterManager_UpdateCluster_EppRuntime(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}
	base := func() *Cluster {
		c := newTestClusterBase()
		c.ID = 1
		return c
	}

	t.Run("not carried keeps stored value", func(t *testing.T) {
		var captured *string
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				captured = param.EppRuntime
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, base(), &ClusterParam{Name: lib.PString("c1")})
		require.NoError(t, err)
		assert.Nil(t, captured)
	})

	t.Run("carried replaces with normalized value", func(t *testing.T) {
		var captured *string
		clusterStore := &fakeClusterStorager{
			clusterUpdateFn: func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
				captured = param.EppRuntime
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, base(), &ClusterParam{
			Name:       lib.PString("c1"),
			EppRuntime: lib.PString(`{"check":{"cooldown":"10s"}}`),
		})
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Contains(t, *captured, `"cooldown":"10s"`)
		assert.Contains(t, *captured, `"fail_threshold":3`)
	})

	t.Run("EPP to WRR still validates carried epp_runtime", func(t *testing.T) {
		old := base()
		old.BalanceMode = BalanceModeEPP
		old.EppConfig = validEppConfigJSON
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		err := m.UpdateCluster(ctx, product, old, &ClusterParam{
			Name:        lib.PString("c1"),
			BalanceMode: lib.PString(BalanceModeWRR),
			EppRuntime:  lib.PString(`{"check":{"fail_threshold":0}}`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fail_threshold")
	})
}
