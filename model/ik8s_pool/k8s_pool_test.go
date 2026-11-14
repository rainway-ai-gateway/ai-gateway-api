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

package ik8s_pool

import (
	"context"
	"errors"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateInstances(t *testing.T) {
	t.Run("empty list is valid zero instances", func(t *testing.T) {
		instances, err := ValidateInstances(nil)
		require.NoError(t, err)
		require.NotNil(t, instances)
		assert.Len(t, instances, 0)
	})

	t.Run("weight defaults to 100", func(t *testing.T) {
		instances, err := ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(8000)},
		})
		require.NoError(t, err)
		require.Len(t, instances, 1)
		assert.Equal(t, int64(100), instances[0].Weight)
		assert.Equal(t, "10.0.0.1", instances[0].Addr)
		assert.Equal(t, 8000, instances[0].Port)
	})

	t.Run("explicit weight kept", func(t *testing.T) {
		instances, err := ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(8000), Weight: lib.PInt64(0)},
			{Addr: lib.PString("10.0.0.2"), Port: lib.PInt(8000), Weight: lib.PInt64(100)},
		})
		require.NoError(t, err)
		require.Len(t, instances, 2)
		assert.Equal(t, int64(0), instances[0].Weight)
		assert.Equal(t, int64(100), instances[1].Weight)
	})

	t.Run("weight out of range rejected", func(t *testing.T) {
		_, err := ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(8000), Weight: lib.PInt64(101)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "weight")

		_, err = ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(8000), Weight: lib.PInt64(-1)},
		})
		require.Error(t, err)
	})

	t.Run("missing addr rejected", func(t *testing.T) {
		_, err := ValidateInstances([]InstanceParam{
			{Port: lib.PInt(8000)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "addr")

		_, err = ValidateInstances([]InstanceParam{
			{Addr: lib.PString("  "), Port: lib.PInt(8000)},
		})
		require.Error(t, err)
	})

	t.Run("port out of range rejected", func(t *testing.T) {
		_, err := ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(0)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "port")

		_, err = ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(65536)},
		})
		require.Error(t, err)
	})

	t.Run("duplicate addr/port rejected", func(t *testing.T) {
		_, err := ValidateInstances([]InstanceParam{
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(8000)},
			{Addr: lib.PString("10.0.0.1"), Port: lib.PInt(8000), Weight: lib.PInt64(50)},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate")
	})
}

func TestK8sPoolManager_ReplaceInstances(t *testing.T) {
	ctx := context.Background()

	t.Run("upserts pool and returns the fresh entry", func(t *testing.T) {
		poolStore := &fakeK8sPoolStorager{
			fetchFn: func(ctx context.Context, name string) (*K8sPool, error) {
				return &K8sPool{
					Name:       name,
					Instances:  []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 8000, Weight: 100}},
					UpdateTime: 1720000000,
				}, nil
			},
		}
		providerStore := &fakeProviderStorager{}
		m := NewK8sPoolManager(&fakeTxn{}, poolStore, providerStore)

		pool, err := m.ReplaceInstances(ctx, "svc-a", []iprovider.ProviderInstance{
			{Addr: "10.0.0.1", Port: 8000, Weight: 100},
		})
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "svc-a", pool.Name)
		assert.Len(t, pool.Instances, 1)
		assert.Equal(t, []string{"svc-a"}, poolStore.upserted)
		// Single-table write: providers are never touched.
		assert.Empty(t, providerStore.updated)
	})

	t.Run("pool upsert failure aborts", func(t *testing.T) {
		providerStore := &fakeProviderStorager{}
		m := NewK8sPoolManager(&fakeTxn{}, &fakeK8sPoolStorager{
			upsertFn: func(ctx context.Context, name string, instances []iprovider.ProviderInstance) error {
				return errors.New("upsert failed")
			},
		}, providerStore)

		_, err := m.ReplaceInstances(ctx, "svc-a", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upsert failed")
		assert.Empty(t, providerStore.updated)
	})
}

func TestK8sPoolManager_DeletePool(t *testing.T) {
	ctx := context.Background()

	referencingProviderList := func(poolName string) func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
		return func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
			require.NotNil(t, filter.InstanceSource)
			assert.Equal(t, iprovider.InstanceSourceK8sPool, *filter.InstanceSource)
			require.NotNil(t, filter.K8sPoolName)
			assert.Equal(t, poolName, *filter.K8sPoolName)
			return []*iprovider.Provider{
				{Name: "p1", InstanceSource: iprovider.InstanceSourceK8sPool, K8sPoolName: &poolName},
			}, 1, nil
		}
	}

	t.Run("deletes an unreferenced pool", func(t *testing.T) {
		poolStore := &fakeK8sPoolStorager{
			fetchFn: func(ctx context.Context, name string) (*K8sPool, error) {
				return &K8sPool{Name: name}, nil
			},
		}
		m := NewK8sPoolManager(&fakeTxn{}, poolStore, &fakeProviderStorager{})

		require.NoError(t, m.DeletePool(ctx, "svc-a"))
		assert.Equal(t, []string{"svc-a"}, poolStore.deleted)
		assert.Empty(t, poolStore.upserted)
	})

	t.Run("missing pool returns not exist", func(t *testing.T) {
		m := NewK8sPoolManager(&fakeTxn{}, &fakeK8sPoolStorager{}, &fakeProviderStorager{})
		err := m.DeletePool(ctx, "svc-a")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Not Exist")
	})

	t.Run("referenced pool keeps the row and clears its instances", func(t *testing.T) {
		poolName := "svc-a"
		poolStore := &fakeK8sPoolStorager{
			fetchFn: func(ctx context.Context, name string) (*K8sPool, error) {
				return &K8sPool{Name: name}, nil
			},
		}
		providerStore := &fakeProviderStorager{fetchListFn: referencingProviderList(poolName)}
		m := NewK8sPoolManager(&fakeTxn{}, poolStore, providerStore)

		require.NoError(t, m.DeletePool(ctx, poolName))

		// The row is kept (so GET returns 200 + instance_count=0 and
		// last_sync_time stays inspectable) and its instances are cleared.
		assert.Empty(t, poolStore.deleted)
		require.Len(t, poolStore.upserted, 1)
		assert.Equal(t, poolName, poolStore.upserted[0])
		// Providers are only read, never written.
		assert.Empty(t, providerStore.updated)
	})

	t.Run("reference check failure aborts before any write", func(t *testing.T) {
		poolName := "svc-a"
		poolStore := &fakeK8sPoolStorager{
			fetchFn: func(ctx context.Context, name string) (*K8sPool, error) {
				return &K8sPool{Name: name}, nil
			},
		}
		providerStore := &fakeProviderStorager{
			fetchListFn: func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
				return nil, 0, errors.New("provider list failed")
			},
		}
		m := NewK8sPoolManager(&fakeTxn{}, poolStore, providerStore)

		err := m.DeletePool(ctx, poolName)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider list failed")
		assert.Empty(t, poolStore.deleted)
		assert.Empty(t, poolStore.upserted)
	})
}

func TestK8sPoolManager_Fetch(t *testing.T) {
	ctx := context.Background()

	t.Run("fetch pool passthrough", func(t *testing.T) {
		poolStore := &fakeK8sPoolStorager{
			fetchFn: func(ctx context.Context, name string) (*K8sPool, error) {
				return &K8sPool{Name: name, Instances: []iprovider.ProviderInstance{
					{Addr: "10.0.0.1", Port: 8000, Weight: 100},
				}}, nil
			},
		}
		m := NewK8sPoolManager(&fakeTxn{}, poolStore, &fakeProviderStorager{})

		pool, err := m.FetchPool(ctx, "svc-a")
		require.NoError(t, err)
		require.NotNil(t, pool)
		assert.Equal(t, "svc-a", pool.Name)
		assert.Len(t, pool.Instances, 1)
	})

	t.Run("fetch pool list passthrough", func(t *testing.T) {
		poolStore := &fakeK8sPoolStorager{
			fetchListFn: func(ctx context.Context) ([]*K8sPool, error) {
				return []*K8sPool{{Name: "svc-a"}, {Name: "svc-b"}}, nil
			},
		}
		m := NewK8sPoolManager(&fakeTxn{}, poolStore, &fakeProviderStorager{})

		pools, err := m.FetchPoolList(ctx)
		require.NoError(t, err)
		require.Len(t, pools, 2)
	})
}
