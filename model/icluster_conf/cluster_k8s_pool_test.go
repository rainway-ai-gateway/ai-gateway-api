// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package icluster_conf

import (
	"context"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// k8sProviderStorager returns a k8s_pool-mode provider with an empty dormant
// instance_pool, mirroring a provider created after the pool was populated
// (the original online incident shape).
func k8sProviderStorager(name, poolName string) *fakeProviderStorager {
	return &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
			return &iprovider.Provider{
				Name:           name,
				Models:         []string{"m1"},
				Keys:           []iprovider.ProviderKey{{Name: "k1", Key: "sk-xxx"}},
				InstanceSource: iprovider.InstanceSourceK8sPool,
				K8sPoolName:    lib.PString(poolName),
				InstancePool:   []iprovider.ProviderInstance{},
				ModelProtocols: []string{"openai"},
			}, nil
		},
	}
}

// TestClusterManager_CreateCluster_K8sEmptyPool is the regression for the
// online incident: a cluster referencing a k8s_pool-mode provider whose pool
// has no resolvable instances yet must still be created successfully — the
// derived pool is an empty placeholder row (schema reference only), and the
// cluster/subcluster creation must not fail on the empty instance list.
func TestClusterManager_CreateCluster_K8sEmptyPool(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	newFixture := func(t *testing.T) (*ClusterManager, *fakePoolStorager, *fakeSubClusterStorager) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
			clusterCreateFn: func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
				return 20, nil
			},
			bindSubClusterFn: func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
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
				return &Pool{ID: 1, Name: *data.Name}, nil
			},
		}
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: stateful.DefaultConfig.RunTime.DefaultAIClusterName}}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore,
			k8sProviderStorager("k8s-prov", "svc-a"), nil, nil, nil)
		return m, poolStore, subClusterStore
	}

	t.Run("empty k8s pool placeholder: create succeeds with an empty (non-nil) derived pool", func(t *testing.T) {
		m, poolStore, subClusterStore := newFixture(t)

		var createdPool *PoolParam
		poolStore.createPoolFn = func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
			createdPool = data
			return &Pool{ID: 1, Name: *data.Name}, nil
		}
		subClusterCreated := false
		subClusterStore.createSubClusterFn = func(ctx context.Context, param *SubClusterParam) error {
			subClusterCreated = true
			require.NotNil(t, param.InstancePool, "subcluster must bind the placeholder pool")
			return nil
		}

		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name: lib.PString("c1"),
			LLMConfig: &LLMConfig{
				Models:   []string{"m1"},
				Provider: lib.PString("k8s-prov"),
			},
		})
		require.NoError(t, err, "creating a cluster on an empty k8s pool must succeed")

		require.True(t, subClusterCreated, "subcluster must be created alongside the cluster")
		require.NotNil(t, createdPool, "derived pool must be created")
		require.NotNil(t, createdPool.Instances, "placeholder instances must be an explicit empty list, not NULL")
		assert.Len(t, createdPool.Instances, 0)
	})

	t.Run("k8s pool not yet created (name references a missing pool): create still succeeds", func(t *testing.T) {
		m, poolStore, _ := newFixture(t)

		var createdPool *PoolParam
		poolStore.createPoolFn = func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
			createdPool = data
			return &Pool{ID: 1, Name: *data.Name}, nil
		}

		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name: lib.PString("c1"),
			LLMConfig: &LLMConfig{
				Models:   []string{"m1"},
				Provider: lib.PString("k8s-prov"),
			},
		})
		require.NoError(t, err, "a missing k8s pool is zero instances, not an error")
		require.NotNil(t, createdPool)
		require.NotNil(t, createdPool.Instances)
		assert.Len(t, createdPool.Instances, 0)
	})

	t.Run("manual provider keeps snapshotting its instance_pool", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, nil
			},
			clusterCreateFn: func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
				return 20, nil
			},
			bindSubClusterFn: func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
				return nil
			},
		}
		subClusterStore := &fakeSubClusterStorager{
			fetchSubClusterListFn: func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
				return []*SubCluster{{ID: 2, Name: "c1", Ready: true}}, nil
			},
		}
		var createdPool *PoolParam
		poolStore := &fakePoolStorager{
			fetchPoolFn: func(ctx context.Context, name string) (*Pool, error) {
				return nil, nil
			},
			createPoolFn: func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
				createdPool = data
				return &Pool{ID: 1, Name: *data.Name}, nil
			},
		}
		bfeClusterStore := &fakeBFEClusterStorager{
			fetchBFEClustersFn: func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
				return []*ibasic.BFECluster{{Name: stateful.DefaultConfig.RunTime.DefaultAIClusterName}}, nil
			},
		}
		manualStore := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
				return &iprovider.Provider{
					Name:           "manual-prov",
					Models:         []string{"m1"},
					InstanceSource: iprovider.InstanceSourceInstancePool,
					InstancePool:   []iprovider.ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
					ModelProtocols: []string{"openai"},
				}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, subClusterStore, bfeClusterStore, poolStore,
			manualStore, nil, nil, nil)

		err := m.CreateCluster(ctx, product, &ClusterParam{
			Name: lib.PString("c1"),
			LLMConfig: &LLMConfig{
				Models:   []string{"m1"},
				Provider: lib.PString("manual-prov"),
			},
		})
		require.NoError(t, err)
		require.NotNil(t, createdPool)
		require.NotNil(t, createdPool.Instances)
		require.Len(t, createdPool.Instances, 1)
		assert.Equal(t, "1.2.3.4", createdPool.Instances[0].Addr)
	})
}

// TestClusterManager_UpdateCluster_K8sProvider pins the update path: patching
// a cluster that references a k8s provider must not write instance data into
// the derived pool (the placeholder stays empty), while a provider reference
// change still clears any stale manual snapshot.
func TestClusterManager_UpdateCluster_K8sProvider(t *testing.T) {
	ctx := context.Background()
	product := &ibasic.Product{ID: 2, Name: "test"}

	oldCluster := func(provider string) *Cluster {
		return &Cluster{
			ID:   1,
			Name: "c1",
			LLMConfig: &LLMConfig{
				Models:   []string{"m1"},
				Provider: lib.PString(provider),
			},
			SubClusters: []*SubCluster{
				{
					ID:   1,
					Name: "sc1",
					InstancePool: &Pool{
						ID:        1,
						Name:      "p.c1",
						Instances: []Instance{{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100}},
					},
				},
			},
		}
	}

	t.Run("k8s provider unchanged: no pool write at all", func(t *testing.T) {
		poolWrites := 0
		poolStore := &fakePoolStorager{
			updatePoolFn: func(ctx context.Context, oldData *Pool, diff *PoolParam) error {
				poolWrites++
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{},
			&fakeBFEClusterStorager{}, poolStore, k8sProviderStorager("k8s-prov", "svc-a"), nil, nil, nil)

		err := m.UpdateCluster(ctx, product, oldCluster("k8s-prov"), &ClusterParam{
			Name: lib.PString("c1"),
			LLMConfig: &LLMConfig{
				Models:   []string{"m1"},
				Provider: lib.PString("k8s-prov"),
			},
		})
		require.NoError(t, err)
		assert.Equal(t, 0, poolWrites, "k8s mode must not write instance data on cluster update")
	})

	t.Run("switching from manual to k8s provider clears the stale manual snapshot", func(t *testing.T) {
		var cleared []Instance
		poolStore := &fakePoolStorager{
			updatePoolFn: func(ctx context.Context, oldData *Pool, diff *PoolParam) error {
				cleared = diff.Instances
				return nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, &fakeSubClusterStorager{},
			&fakeBFEClusterStorager{}, poolStore, k8sProviderStorager("k8s-prov", "svc-a"), nil, nil, nil)

		err := m.UpdateCluster(ctx, product, oldCluster("manual-prov"), &ClusterParam{
			Name: lib.PString("c1"),
			LLMConfig: &LLMConfig{
				Models:   []string{"m1"},
				Provider: lib.PString("k8s-prov"),
			},
		})
		require.NoError(t, err)
		require.NotNil(t, cleared, "stale manual snapshot must be cleared to the empty placeholder")
		assert.Len(t, cleared, 0)
	})
}
