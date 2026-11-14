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
	"errors"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeProviderRefCluster(name string, provider *string, instances []Instance) *Cluster {
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

func TestClusterManager_SyncProviderInstances(t *testing.T) {
	ctx := context.Background()
	providerName := "deepseek"

	t.Run("updates only clusters referencing the provider", func(t *testing.T) {
		updatedPools := map[int64][]Instance{}
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeProviderRefCluster("c1", &providerName, []Instance{
						{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100},
					}),
					makeProviderRefCluster("c2", lib.PString("other"), []Instance{
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
		require.NoError(t, cm.SyncProviderInstances(ctx, providerName,
			[]iprovider.ProviderInstance{{Addr: "5.6.7.8", Port: 443, Weight: 100}}))

		require.Len(t, updatedPools, 1)
		require.Contains(t, updatedPools, int64(1))
		assert.Equal(t, "5.6.7.8_443", updatedPools[1][0].Name)
	})

	t.Run("empty instance list clears the derived pools", func(t *testing.T) {
		updatedPools := map[int64][]Instance{}
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeProviderRefCluster("c1", &providerName, []Instance{
						{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100},
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
		require.NoError(t, cm.SyncProviderInstances(ctx, providerName, nil))

		require.Len(t, updatedPools, 1)
		instances, ok := updatedPools[int64(1)]
		require.True(t, ok)
		require.NotNil(t, instances, "clear must write an explicit empty list, not a nil skip")
		assert.Len(t, instances, 0)
	})

	t.Run("propagates update pool error", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeProviderRefCluster("c1", &providerName, []Instance{
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
		err := cm.SyncProviderInstances(ctx, providerName,
			[]iprovider.ProviderInstance{{Addr: "5.6.7.8", Port: 443, Weight: 100}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "update failed")
	})
}

func TestClusterManager_ProviderInstancePoolSyncer_SourceSplit(t *testing.T) {
	ctx := context.Background()
	providerName := "deepseek"

	newRecordingCM := func(t *testing.T) (*ClusterManager, *map[int64][]Instance) {
		updatedPools := map[int64][]Instance{}
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{
					makeProviderRefCluster("c1", &providerName, []Instance{
						{Name: "old", Addr: "1.2.3.4", Port: 443, Weight: 100},
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
		return cm, &updatedPools
	}

	t.Run("manual -> k8s clears the old manual snapshot", func(t *testing.T) {
		cm, updatedPools := newRecordingCM(t)
		oldProvider := &iprovider.Provider{
			Name:         providerName,
			InstancePool: []iprovider.ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
		}
		newProvider := &iprovider.Provider{
			Name:           providerName,
			InstanceSource: iprovider.InstanceSourceK8sPool,
			K8sPoolName:    lib.PString("svc-a"),
			InstancePool:   []iprovider.ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
		}
		require.NoError(t, cm.ProviderInstancePoolSyncer(ctx, oldProvider, newProvider))
		require.Len(t, *updatedPools, 1)
		assert.Len(t, (*updatedPools)[1], 0, "derived pool must be cleared to the empty placeholder")
	})

	t.Run("k8s -> k8s pool name switch is a no-op", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("FetchClusterList should not be called")
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)

		oldProvider := &iprovider.Provider{
			Name:           providerName,
			InstanceSource: iprovider.InstanceSourceK8sPool,
			K8sPoolName:    lib.PString("pool-a"),
		}
		newProvider := &iprovider.Provider{
			Name:           providerName,
			InstanceSource: iprovider.InstanceSourceK8sPool,
			K8sPoolName:    lib.PString("pool-b"),
		}
		require.NoError(t, cm.ProviderInstancePoolSyncer(ctx, oldProvider, newProvider))
	})

	t.Run("k8s -> manual writes the new manual pool", func(t *testing.T) {
		cm, updatedPools := newRecordingCM(t)
		oldProvider := &iprovider.Provider{
			Name:           providerName,
			InstanceSource: iprovider.InstanceSourceK8sPool,
			K8sPoolName:    lib.PString("svc-a"),
		}
		newProvider := &iprovider.Provider{
			Name:         providerName,
			InstancePool: []iprovider.ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
		}
		require.NoError(t, cm.ProviderInstancePoolSyncer(ctx, oldProvider, newProvider))
		require.Len(t, *updatedPools, 1)
		assert.Equal(t, "1.2.3.4_443", (*updatedPools)[1][0].Name)
	})

	t.Run("manual instance_pool change writes the new pool", func(t *testing.T) {
		cm, updatedPools := newRecordingCM(t)
		oldProvider := &iprovider.Provider{
			Name:         providerName,
			InstancePool: []iprovider.ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
		}
		newProvider := &iprovider.Provider{
			Name:         providerName,
			InstancePool: []iprovider.ProviderInstance{{Addr: "5.6.7.8", Port: 443, Weight: 100}},
		}
		require.NoError(t, cm.ProviderInstancePoolSyncer(ctx, oldProvider, newProvider))
		require.Len(t, *updatedPools, 1)
		assert.Equal(t, "5.6.7.8_443", (*updatedPools)[1][0].Name)
	})

	t.Run("equal manual pools are a no-op", func(t *testing.T) {
		clusterStorager := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("FetchClusterList should not be called")
			},
		}
		cm := NewClusterManager(&fakeTxn{}, clusterStorager, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)

		pool := []iprovider.ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}}
		oldProvider := &iprovider.Provider{Name: providerName, InstancePool: pool}
		newProvider := &iprovider.Provider{Name: providerName, InstancePool: pool}
		require.NoError(t, cm.ProviderInstancePoolSyncer(ctx, oldProvider, newProvider))
	})

	t.Run("nil new provider is a no-op", func(t *testing.T) {
		cm := NewClusterManager(&fakeTxn{}, &fakeClusterStorager{}, nil, nil, &fakePoolStorager{}, nil, nil, nil, nil)
		require.NoError(t, cm.ProviderInstancePoolSyncer(ctx, nil, nil))
	})
}
