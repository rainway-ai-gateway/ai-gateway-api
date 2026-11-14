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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ik8s_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

type fakeK8sPoolStorager struct {
	pools        []*ik8s_pool.K8sPool
	fetchErr     error
	fetchListErr error
	fetchedName  string
}

func (s *fakeK8sPoolStorager) UpsertPool(ctx context.Context, name string, instances []iprovider.ProviderInstance) error {
	return nil
}

func (s *fakeK8sPoolStorager) FetchPool(ctx context.Context, name string) (*ik8s_pool.K8sPool, error) {
	s.fetchedName = name
	if s.fetchErr != nil {
		return nil, s.fetchErr
	}
	for _, pool := range s.pools {
		if pool.Name == name {
			return pool, nil
		}
	}
	return nil, nil
}

func (s *fakeK8sPoolStorager) FetchK8sPoolInstances(ctx context.Context, name string) ([]iprovider.ProviderInstance, error) {
	pool, err := s.FetchPool(ctx, name)
	if err != nil || pool == nil {
		return []iprovider.ProviderInstance{}, err
	}
	return pool.Instances, nil
}

func (s *fakeK8sPoolStorager) FetchPoolList(ctx context.Context) ([]*ik8s_pool.K8sPool, error) {
	if s.fetchListErr != nil {
		return nil, s.fetchListErr
	}
	return s.pools, nil
}

func (s *fakeK8sPoolStorager) DeletePool(ctx context.Context, name string) error {
	return nil
}

func setupManager(storager ik8s_pool.K8sPoolStorager) func() {
	old := container.K8sPoolManager
	container.K8sPoolManager = ik8s_pool.NewK8sPoolManager(nil, storager, nil)
	return func() { container.K8sPoolManager = old }
}

func requestWithPoolName(name string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/k8s_pools/"+name, nil)
	return mux.SetURLVars(req, map[string]string{"name": name})
}

func TestEndpoints(t *testing.T) {
	require.Len(t, Endpoints, 2)
	assert.Equal(t, "/k8s_pools", Endpoints[0].Path)
	assert.Equal(t, http.MethodGet, Endpoints[0].Method)
	assert.NotNil(t, Endpoints[0].Authorizer)

	assert.Equal(t, "/k8s_pools/{name}", Endpoints[1].Path)
	assert.Equal(t, http.MethodGet, Endpoints[1].Method)
	assert.NotNil(t, Endpoints[1].Authorizer)
}

func TestListAction(t *testing.T) {
	storager := &fakeK8sPoolStorager{
		pools: []*ik8s_pool.K8sPool{
			{
				Name: "svc-a",
				Instances: []iprovider.ProviderInstance{
					{Addr: "10.0.0.1", Port: 8000, Weight: 100},
					{Addr: "10.0.0.2", Port: 8000, Weight: 50},
				},
				UpdateTime: 1716883200,
			},
			{
				Name:       "svc-empty",
				Instances:  []iprovider.ProviderInstance{},
				UpdateTime: 1716883201,
			},
		},
	}
	defer setupManager(storager)()

	data, err := ListAction(httptest.NewRequest(http.MethodGet, "/k8s_pools", nil))
	require.NoError(t, err)

	body, ok := data.(map[string]interface{})
	require.True(t, ok)
	list, ok := body["list"].([]*PoolListEntry)
	require.True(t, ok)
	require.Len(t, list, 2)

	assert.Equal(t, "svc-a", list[0].Name)
	assert.Equal(t, 2, list[0].InstanceCount)
	assert.Equal(t, int64(1716883200), list[0].LastSyncTime)

	assert.Equal(t, "svc-empty", list[1].Name)
	assert.Equal(t, 0, list[1].InstanceCount)
	assert.Equal(t, int64(1716883201), list[1].LastSyncTime)
}

func TestListActionEmpty(t *testing.T) {
	defer setupManager(&fakeK8sPoolStorager{pools: []*ik8s_pool.K8sPool{}})()

	data, err := ListAction(httptest.NewRequest(http.MethodGet, "/k8s_pools", nil))
	require.NoError(t, err)

	body, ok := data.(map[string]interface{})
	require.True(t, ok)
	list, ok := body["list"].([]*PoolListEntry)
	require.True(t, ok)
	assert.Empty(t, list)

	raw, err := json.Marshal(data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"list":[]}`, string(raw))
}

func TestListActionError(t *testing.T) {
	storager := &fakeK8sPoolStorager{fetchListErr: errors.New("db down")}
	defer setupManager(storager)()

	data, err := ListAction(httptest.NewRequest(http.MethodGet, "/k8s_pools", nil))
	assert.Nil(t, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestGetAction(t *testing.T) {
	storager := &fakeK8sPoolStorager{
		pools: []*ik8s_pool.K8sPool{
			{
				Name: "svc-a",
				Instances: []iprovider.ProviderInstance{
					{Addr: "10.0.0.1", Port: 8000, Weight: 100, Disable: false},
					{Addr: "10.0.0.2", Port: 8000, Weight: 50, Disable: true},
				},
				UpdateTime: 1716883200,
			},
		},
	}
	defer setupManager(storager)()

	data, err := GetAction(requestWithPoolName("svc-a"))
	require.NoError(t, err)

	entry, ok := data.(*PoolEntry)
	require.True(t, ok)
	assert.Equal(t, "svc-a", entry.Name)
	assert.Equal(t, 2, entry.InstanceCount)
	assert.Equal(t, int64(1716883200), entry.LastSyncTime)
	require.Len(t, entry.Instances, 2)
	assert.Equal(t, "10.0.0.1", entry.Instances[0].Addr)
	assert.Equal(t, 8000, entry.Instances[0].Port)
	assert.Equal(t, int64(100), entry.Instances[0].Weight)
	assert.True(t, entry.Instances[1].Disable)
	assert.Equal(t, "svc-a", storager.fetchedName)

	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"disable"`)
	assert.Contains(t, string(raw), `"instance_count"`)
	assert.Contains(t, string(raw), `"last_sync_time"`)
}

func TestGetActionEmptyPool(t *testing.T) {
	storager := &fakeK8sPoolStorager{
		pools: []*ik8s_pool.K8sPool{
			{Name: "svc-empty", Instances: nil, UpdateTime: 1716883200},
		},
	}
	defer setupManager(storager)()

	data, err := GetAction(requestWithPoolName("svc-empty"))
	require.NoError(t, err)

	entry, ok := data.(*PoolEntry)
	require.True(t, ok)
	assert.Equal(t, 0, entry.InstanceCount)
	assert.NotNil(t, entry.Instances)
	assert.Empty(t, entry.Instances)

	raw, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"instances":[]`)
}

func TestGetActionNotFound(t *testing.T) {
	defer setupManager(&fakeK8sPoolStorager{pools: []*ik8s_pool.K8sPool{}})()

	data, err := GetAction(requestWithPoolName("missing"))
	assert.Nil(t, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Record Not Exist")
}

func TestGetActionInvalidName(t *testing.T) {
	defer setupManager(&fakeK8sPoolStorager{})()

	data, err := GetAction(requestWithPoolName("-bad-"))
	assert.Nil(t, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot start or end")
}

func TestGetActionError(t *testing.T) {
	storager := &fakeK8sPoolStorager{fetchErr: errors.New("db down")}
	defer setupManager(storager)()

	data, err := GetAction(requestWithPoolName("svc-a"))
	assert.Nil(t, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
