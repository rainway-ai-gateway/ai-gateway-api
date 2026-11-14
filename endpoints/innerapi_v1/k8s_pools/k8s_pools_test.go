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

package k8s_pools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ik8s_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeK8sPoolStorager struct {
	upsertFn    func(ctx context.Context, name string, instances []iprovider.ProviderInstance) error
	fetchFn     func(ctx context.Context, name string) (*ik8s_pool.K8sPool, error)
	fetchListFn func(ctx context.Context) ([]*ik8s_pool.K8sPool, error)
	deleteFn    func(ctx context.Context, name string) error
}

func (f *fakeK8sPoolStorager) UpsertPool(ctx context.Context, name string, instances []iprovider.ProviderInstance) error {
	return f.upsertFn(ctx, name, instances)
}

func (f *fakeK8sPoolStorager) FetchPool(ctx context.Context, name string) (*ik8s_pool.K8sPool, error) {
	return f.fetchFn(ctx, name)
}

func (f *fakeK8sPoolStorager) FetchK8sPoolInstances(ctx context.Context, name string) ([]iprovider.ProviderInstance, error) {
	pool, err := f.fetchFn(ctx, name)
	if err != nil || pool == nil {
		return []iprovider.ProviderInstance{}, err
	}
	return pool.Instances, nil
}

func (f *fakeK8sPoolStorager) FetchPoolList(ctx context.Context) ([]*ik8s_pool.K8sPool, error) {
	return f.fetchListFn(ctx)
}

func (f *fakeK8sPoolStorager) DeletePool(ctx context.Context, name string) error {
	return f.deleteFn(ctx, name)
}

type fakeProviderStorager struct {
	fetchListFn func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error)
}

func (f *fakeProviderStorager) CreateProvider(ctx context.Context, param *iprovider.ProviderParam) (int64, error) {
	return 0, nil
}

func (f *fakeProviderStorager) UpdateProvider(ctx context.Context, name string, param *iprovider.ProviderParam) error {
	return nil
}

func (f *fakeProviderStorager) DeleteProvider(ctx context.Context, name string) error {
	return nil
}

func (f *fakeProviderStorager) FetchProvider(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
	return nil, nil
}

func (f *fakeProviderStorager) FetchProviderList(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
	return f.fetchListFn(ctx, filter)
}

func (f *fakeProviderStorager) FetchProviderNames(ctx context.Context) ([]string, error) {
	return nil, nil
}

type fakeTxn struct{}

func (f *fakeTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

// newManager swaps in a K8sPoolManager backed by the given fakes and returns a
// restore function. The provider list is empty, so the fan-out never reaches
// the (nil) ClusterManager.
func swapManager(t *testing.T, poolStorager ik8s_pool.K8sPoolStorager) {
	t.Helper()
	orig := container.K8sPoolManager
	t.Cleanup(func() { container.K8sPoolManager = orig })
	container.K8sPoolManager = ik8s_pool.NewK8sPoolManager(&fakeTxn{}, poolStorager,
		&fakeProviderStorager{
			fetchListFn: func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
				return nil, 0, nil
			},
		})
}

func reqWithName(method, target, name, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if name != "" {
		req = mux.SetURLVars(req, map[string]string{"name": name})
	}
	return req
}

func TestRoutes(t *testing.T) {
	assert.Equal(t, "/k8s_pools/{name}/instances", ReplaceInstancesRoute.Path)
	assert.Equal(t, http.MethodPut, ReplaceInstancesRoute.Method)
	assert.NotNil(t, ReplaceInstancesRoute.Handler)
	assert.Equal(t, "/k8s_pools/{name}", GetPoolRoute.Path)
	assert.Equal(t, http.MethodGet, GetPoolRoute.Method)
	assert.NotNil(t, GetPoolRoute.Handler)
	assert.Equal(t, "/k8s_pools", ListPoolsRoute.Path)
	assert.Equal(t, http.MethodGet, ListPoolsRoute.Method)
	assert.NotNil(t, ListPoolsRoute.Handler)
	assert.Equal(t, "/k8s_pools/{name}", DeletePoolRoute.Path)
	assert.Equal(t, http.MethodDelete, DeletePoolRoute.Method)
	assert.NotNil(t, DeletePoolRoute.Handler)
}

func TestPoolNameFromPath(t *testing.T) {
	_, err := poolNameFromPath(reqWithName(http.MethodGet, "/inner-api/v1/k8s_pools/x", "", ""))
	require.Error(t, err)

	_, err = poolNameFromPath(reqWithName(http.MethodGet, "/inner-api/v1/k8s_pools/x", "-bad-", ""))
	require.Error(t, err)

	name, err := poolNameFromPath(reqWithName(http.MethodGet, "/inner-api/v1/k8s_pools/svc-a", "svc-a", ""))
	require.NoError(t, err)
	assert.Equal(t, "svc-a", name)
}

func TestReplaceInstancesAction(t *testing.T) {
	var upsertedName string
	var upserted []iprovider.ProviderInstance
	swapManager(t, &fakeK8sPoolStorager{
		upsertFn: func(ctx context.Context, name string, instances []iprovider.ProviderInstance) error {
			upsertedName = name
			upserted = instances
			return nil
		},
		fetchFn: func(ctx context.Context, name string) (*ik8s_pool.K8sPool, error) {
			return &ik8s_pool.K8sPool{Name: name, Instances: upserted, UpdateTime: 1716883200}, nil
		},
	})

	// malformed body
	_, err := ReplaceInstancesAction(reqWithName(http.MethodPut, "/inner-api/v1/k8s_pools/svc-a/instances", "svc-a", `{"not":"an array"}`))
	require.Error(t, err)

	// missing addr
	_, err = ReplaceInstancesAction(reqWithName(http.MethodPut, "/inner-api/v1/k8s_pools/svc-a/instances", "svc-a", `[{"port":8000}]`))
	require.Error(t, err)

	// port out of range
	_, err = ReplaceInstancesAction(reqWithName(http.MethodPut, "/inner-api/v1/k8s_pools/svc-a/instances", "svc-a", `[{"addr":"10.0.0.1","port":70000}]`))
	require.Error(t, err)

	// weight out of range
	_, err = ReplaceInstancesAction(reqWithName(http.MethodPut, "/inner-api/v1/k8s_pools/svc-a/instances", "svc-a", `[{"addr":"10.0.0.1","port":8000,"weight":101}]`))
	require.Error(t, err)

	// duplicate addr/port
	_, err = ReplaceInstancesAction(reqWithName(http.MethodPut, "/inner-api/v1/k8s_pools/svc-a/instances", "svc-a", `[{"addr":"10.0.0.1","port":8000},{"addr":"10.0.0.1","port":8000}]`))
	require.Error(t, err)

	// valid: weight defaults to 100
	resp, err := ReplaceInstancesAction(reqWithName(http.MethodPut, "/inner-api/v1/k8s_pools/svc-a/instances", "svc-a", `[{"addr":"10.0.0.1","port":8000},{"addr":"10.0.0.2","port":8000,"weight":50}]`))
	require.NoError(t, err)
	assert.Equal(t, "svc-a", upsertedName)
	require.Len(t, upserted, 2)
	assert.Equal(t, ik8s_pool.DefaultInstanceWeight, upserted[0].Weight)
	assert.Equal(t, int64(50), upserted[1].Weight)

	entry, ok := resp.(*PoolEntry)
	require.True(t, ok)
	assert.Equal(t, "svc-a", entry.Name)
	assert.Equal(t, 2, entry.InstanceCount)
	assert.Equal(t, int64(1716883200), entry.LastSyncTime)
}

func TestGetPoolAction(t *testing.T) {
	swapManager(t, &fakeK8sPoolStorager{
		fetchFn: func(ctx context.Context, name string) (*ik8s_pool.K8sPool, error) {
			if name == "gone" {
				return nil, nil
			}
			return &ik8s_pool.K8sPool{
				Name:       name,
				Instances:  []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 8000, Weight: 100}},
				UpdateTime: 1716883200,
			}, nil
		},
	})

	_, err := GetPoolAction(reqWithName(http.MethodGet, "/inner-api/v1/k8s_pools/gone", "gone", ""))
	require.Error(t, err)

	resp, err := GetPoolAction(reqWithName(http.MethodGet, "/inner-api/v1/k8s_pools/svc-a", "svc-a", ""))
	require.NoError(t, err)
	entry, ok := resp.(*PoolEntry)
	require.True(t, ok)
	assert.Equal(t, 1, entry.InstanceCount)
	assert.Equal(t, int64(1716883200), entry.LastSyncTime)
}

func TestListPoolsAction(t *testing.T) {
	swapManager(t, &fakeK8sPoolStorager{
		fetchListFn: func(ctx context.Context) ([]*ik8s_pool.K8sPool, error) {
			return []*ik8s_pool.K8sPool{
				{Name: "svc-a", Instances: []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 8000, Weight: 100}}, UpdateTime: 1716883200},
				{Name: "svc-b", Instances: nil, UpdateTime: 1716883201},
			}, nil
		},
	})

	resp, err := ListPoolsAction(httptest.NewRequest(http.MethodGet, "/inner-api/v1/k8s_pools", nil))
	require.NoError(t, err)
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{"list":[
		{"name":"svc-a","instance_count":1,"last_sync_time":1716883200},
		{"name":"svc-b","instance_count":0,"last_sync_time":1716883201}
	]}`, string(data))
}

func TestDeletePoolAction(t *testing.T) {
	var deleted string
	swapManager(t, &fakeK8sPoolStorager{
		fetchFn: func(ctx context.Context, name string) (*ik8s_pool.K8sPool, error) {
			return &ik8s_pool.K8sPool{Name: name}, nil
		},
		deleteFn: func(ctx context.Context, name string) error {
			deleted = name
			return nil
		},
	})

	resp, err := DeletePoolAction(reqWithName(http.MethodDelete, "/inner-api/v1/k8s_pools/svc-a", "svc-a", ""))
	require.NoError(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, "svc-a", deleted)
}
