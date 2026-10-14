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

package epp_pool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/epp_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTxn struct{}

func (f *fakeTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

var _ itxn.TxnStorager = (*fakeTxn)(nil)

// fakeEppPoolStorager is a hand-written in-memory EppPoolStorager for
// handler tests.
type fakeEppPoolStorager struct {
	mu        sync.Mutex
	instances map[string]*epp_pool.InstanceParam
}

func newFakeEppPoolStorager() *fakeEppPoolStorager {
	return &fakeEppPoolStorager{instances: map[string]*epp_pool.InstanceParam{}}
}

func (s *fakeEppPoolStorager) seedInstances(instances ...*epp_pool.InstanceParam) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inst := range instances {
		s.instances[inst.ID] = inst
	}
}

func (s *fakeEppPoolStorager) FetchInstanceList(ctx context.Context, filter *epp_pool.InstanceFilter) ([]*epp_pool.InstanceParam, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rst := []*epp_pool.InstanceParam{}
	for _, inst := range s.instances {
		if filter != nil {
			if filter.ID != nil && *filter.ID != inst.ID {
				continue
			}
			if filter.GroupName != nil && *filter.GroupName != inst.GroupName {
				continue
			}
		}
		rst = append(rst, inst)
	}
	sort.Slice(rst, func(i, j int) bool { return rst[i].ID < rst[j].ID })
	return rst, nil
}

func (s *fakeEppPoolStorager) CreateInstances(ctx context.Context, params []*epp_pool.InstanceParam) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, param := range params {
		s.instances[param.ID] = param
	}
	return int64(len(params)), nil
}

func (s *fakeEppPoolStorager) DeleteInstances(ctx context.Context, filter *epp_pool.InstanceFilter) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var affected int64
	for id, inst := range s.instances {
		if filter != nil {
			if filter.ID != nil && *filter.ID != inst.ID {
				continue
			}
			if filter.GroupName != nil && *filter.GroupName != inst.GroupName {
				continue
			}
		}
		delete(s.instances, id)
		affected++
	}
	return affected, nil
}

func (s *fakeEppPoolStorager) FetchAssignment(ctx context.Context, cluster string) (*epp_pool.AssignmentParam, error) {
	return nil, nil
}

func (s *fakeEppPoolStorager) FetchAssignmentList(ctx context.Context) ([]*epp_pool.AssignmentParam, error) {
	return []*epp_pool.AssignmentParam{}, nil
}

func (s *fakeEppPoolStorager) UpsertAssignment(ctx context.Context, param *epp_pool.AssignmentParam) error {
	return nil
}

func (s *fakeEppPoolStorager) DeleteAssignment(ctx context.Context, cluster string) error {
	return nil
}

type fakeClusterSource struct{}

func (f *fakeClusterSource) FetchEPPClusters(ctx context.Context) ([]*epp_pool.EPPClusterInfo, error) {
	return []*epp_pool.EPPClusterInfo{}, nil
}

// setupManager swaps container.EppPoolManager with one backed by in-memory
// fakes and returns the storager plus a restore function.
func setupManager(t *testing.T) (*fakeEppPoolStorager, func()) {
	old := container.EppPoolManager
	storager := newFakeEppPoolStorager()
	container.EppPoolManager = epp_pool.NewEppPoolManager(
		&fakeTxn{},
		storager,
		&fakeClusterSource{},
		nil,
		nil,
	)
	return storager, func() { container.EppPoolManager = old }
}

func newJSONRequest(method, target, body string) *http.Request {
	return httptest.NewRequest(method, target, strings.NewReader(body))
}

func TestEndpoints(t *testing.T) {
	require.Len(t, Endpoints, 2)
	for _, ep := range Endpoints {
		require.NotNil(t, ep)
		assert.NotEmpty(t, ep.Path)
		assert.NotEmpty(t, ep.Method)
	}
	assert.Equal(t, "/epp-pool", Endpoints[0].Path)
	assert.Equal(t, "/epp-pool", Endpoints[1].Path)
	assert.Equal(t, http.MethodGet, Endpoints[0].Method)
	assert.Equal(t, http.MethodPatch, Endpoints[1].Method)
}

func TestGetAction(t *testing.T) {
	storager, teardown := setupManager(t)
	defer teardown()

	storager.seedInstances(
		&epp_pool.InstanceParam{ID: "epp-b", Host: "10.0.0.2", Port: 9002, GroupName: "g1"},
		&epp_pool.InstanceParam{ID: "epp-a", Host: "10.0.0.1", Port: 9002, GroupName: "g1"},
		&epp_pool.InstanceParam{ID: "epp-c", Host: "10.0.0.3", Port: 9002, GroupName: "g2"},
	)

	data, err := GetAction(newJSONRequest(http.MethodGet, "/epp-pool", ""))
	require.NoError(t, err)

	pool, ok := data.(*PoolData)
	require.True(t, ok)
	assert.Equal(t, epp_pool.DefaultEPPInstancePoolName, pool.Name)
	require.Len(t, pool.Groups, 2)
	assert.Equal(t, "g1", pool.Groups[0].Name)
	require.Len(t, pool.Groups[0].Instances, 2)
	assert.Equal(t, "epp-a", pool.Groups[0].Instances[0].ID)
	assert.Equal(t, "10.0.0.1", pool.Groups[0].Instances[0].Host)
	assert.Equal(t, 9002, pool.Groups[0].Instances[0].Port)

	// The exposed instance shape carries id/host/port only (no group_name).
	raw, err := json.Marshal(pool)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "group_name")
}

func TestGetAction_PoolNotExist(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	// Per epp-pool.md §2.1 the pool does not exist until the first PATCH:
	// GET must surface the record-not-exist error instead of an empty pool.
	_, err := GetAction(newJSONRequest(http.MethodGet, "/epp-pool", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Record Not Exist")
}

func TestPatchAction(t *testing.T) {
	storager, teardown := setupManager(t)
	defer teardown()

	storager.seedInstances(
		&epp_pool.InstanceParam{ID: "old-a", Host: "10.0.0.9", Port: 9002, GroupName: "old"},
	)

	body := `{"groups":[
		{"name":"g1","instances":[
			{"id":"epp-a","host":"10.0.0.1","port":9002}]},
		{"name":"g2","instances":[
			{"id":"epp-c","host":"10.0.0.3","port":9002}]}]}`

	data, err := PatchAction(newJSONRequest(http.MethodPatch, "/epp-pool", body))
	require.NoError(t, err)

	pool, ok := data.(*PoolData)
	require.True(t, ok)
	require.Len(t, pool.Groups, 2)
	assert.Equal(t, "g1", pool.Groups[0].Name)
	assert.Equal(t, "g2", pool.Groups[1].Name)

	// Full replacement semantics: the previous instance is gone.
	instances, err := storager.FetchInstanceList(context.Background(), &epp_pool.InstanceFilter{})
	require.NoError(t, err)
	assert.Len(t, instances, 2)
}

func TestPatchAction_ValidationErrors(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	cases := []struct {
		name string
		body string
	}{
		{"groups missing", `{}`},
		{"groups empty", `{"groups":[]}`},
		{"group name empty", `{"groups":[{"name":"","instances":[{"id":"a","host":"10.0.0.1","port":9002}]}]}`},
		{"group name duplicated", `{"groups":[
			{"name":"g1","instances":[{"id":"a","host":"10.0.0.1","port":9002}]},
			{"name":"g1","instances":[{"id":"b","host":"10.0.0.2","port":9002}]}]}`},
		{"instances empty", `{"groups":[{"name":"g1","instances":[]}]}`},
		{"instance nil", `{"groups":[{"name":"g1","instances":[null]}]}`},
		{"instance id empty", `{"groups":[{"name":"g1","instances":[{"id":"","host":"10.0.0.1","port":9002}]}]}`},
		{"instance id duplicated", `{"groups":[
			{"name":"g1","instances":[{"id":"a","host":"10.0.0.1","port":9002}]},
			{"name":"g2","instances":[{"id":"a","host":"10.0.0.2","port":9002}]}]}`},
		{"host empty", `{"groups":[{"name":"g1","instances":[{"id":"a","host":"","port":9002}]}]}`},
		{"host invalid", `{"groups":[{"name":"g1","instances":[{"id":"a","host":"-bad-","port":9002}]}]}`},
		{"port zero", `{"groups":[{"name":"g1","instances":[{"id":"a","host":"10.0.0.1","port":0}]}]}`},
		{"port out of range", `{"groups":[{"name":"g1","instances":[{"id":"a","host":"10.0.0.1","port":70000}]}]}`},
		{"address duplicated", `{"groups":[
			{"name":"g1","instances":[{"id":"a","host":"10.0.0.1","port":9002}]},
			{"name":"g2","instances":[{"id":"b","host":"10.0.0.1","port":9002}]}]}`},
		{"body not json", `{`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PatchAction(newJSONRequest(http.MethodPatch, "/epp-pool", tc.body))
			require.Error(t, err)
			assert.Equal(t, 422, xerror.Resolve(err).ErrNo)
		})
	}
}

func TestPatchAction_GroupSize(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	// Single instance group is allowed.
	one := `{"groups":[{"name":"g1","instances":[{"id":"a","host":"10.0.0.1","port":9002}]}]}`
	data, err := PatchAction(newJSONRequest(http.MethodPatch, "/epp-pool", one))
	require.NoError(t, err)
	pool, ok := data.(*PoolData)
	require.True(t, ok)
	require.Len(t, pool.Groups, 1)
	assert.Len(t, pool.Groups[0].Instances, 1)

	// Two or more instances per group are rejected.
	two := `{"groups":[{"name":"g1","instances":[
		{"id":"a","host":"10.0.0.1","port":9002},
		{"id":"b","host":"10.0.0.2","port":9002}]}]}`
	_, err = PatchAction(newJSONRequest(http.MethodPatch, "/epp-pool", two))
	require.Error(t, err)
	assert.Equal(t, 422, xerror.Resolve(err).ErrNo)

	// Three or more instances per group are rejected.
	three := `{"groups":[{"name":"g1","instances":[
		{"id":"a","host":"10.0.0.1","port":9002},
		{"id":"b","host":"10.0.0.2","port":9002},
		{"id":"c","host":"10.0.0.3","port":9002}]}]}`
	_, err = PatchAction(newJSONRequest(http.MethodPatch, "/epp-pool", three))
	require.Error(t, err)
	assert.Equal(t, 422, xerror.Resolve(err).ErrNo)
}
