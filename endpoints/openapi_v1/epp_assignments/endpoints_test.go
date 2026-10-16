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

package epp_assignments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"

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
	mu          sync.Mutex
	instances   map[string]*epp_pool.InstanceParam
	assignments map[string]*epp_pool.AssignmentParam
}

func newFakeEppPoolStorager() *fakeEppPoolStorager {
	return &fakeEppPoolStorager{
		instances:   map[string]*epp_pool.InstanceParam{},
		assignments: map[string]*epp_pool.AssignmentParam{},
	}
}

func (s *fakeEppPoolStorager) seedInstances(instances ...*epp_pool.InstanceParam) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, inst := range instances {
		s.instances[inst.ID] = inst
	}
}

func (s *fakeEppPoolStorager) seedAssignments(assignments ...*epp_pool.AssignmentParam) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, assignment := range assignments {
		s.assignments[assignment.Cluster] = assignment
	}
}

func (s *fakeEppPoolStorager) fetchAssignment(cluster string) *epp_pool.AssignmentParam {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.assignments[cluster]
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
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.assignments[cluster], nil
}

func (s *fakeEppPoolStorager) FetchAssignmentList(ctx context.Context) ([]*epp_pool.AssignmentParam, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rst := []*epp_pool.AssignmentParam{}
	for _, assignment := range s.assignments {
		rst = append(rst, assignment)
	}
	sort.Slice(rst, func(i, j int) bool { return rst[i].Cluster < rst[j].Cluster })
	return rst, nil
}

func (s *fakeEppPoolStorager) UpsertAssignment(ctx context.Context, param *epp_pool.AssignmentParam) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignments[param.Cluster] = param
	return nil
}

func (s *fakeEppPoolStorager) DeleteAssignment(ctx context.Context, cluster string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.assignments, cluster)
	return nil
}

type fakeClusterSource struct {
	clusters []*epp_pool.EPPClusterInfo
}

func (f *fakeClusterSource) FetchEPPClusters(ctx context.Context) ([]*epp_pool.EPPClusterInfo, error) {
	return f.clusters, nil
}

// setupManager swaps container.EppPoolManager with one backed by in-memory
// fakes and returns the storager plus a restore function.
func setupManager(t *testing.T) (*fakeEppPoolStorager, func()) {
	old := container.EppPoolManager
	storager := newFakeEppPoolStorager()
	storager.seedInstances(
		&epp_pool.InstanceParam{ID: "epp-a", Host: "10.0.0.1", Port: 9002, GroupName: "g1"},
		&epp_pool.InstanceParam{ID: "epp-b", Host: "10.0.0.2", Port: 9002, GroupName: "g1"},
		&epp_pool.InstanceParam{ID: "epp-c", Host: "10.0.0.3", Port: 9002, GroupName: "g2"},
	)
	storager.seedAssignments(&epp_pool.AssignmentParam{
		Cluster:           "cluster-a",
		GroupName:         "g1",
		PrimaryInstanceID: "epp-a",
	})
	clusterSource := &fakeClusterSource{clusters: []*epp_pool.EPPClusterInfo{
		{Name: "cluster-a", EppConfigJSON: `{"load_profile":"balanced"}`},
		{Name: "cluster-b", EppConfigJSON: `{"load_profile":"balanced"}`},
	}}
	container.EppPoolManager = epp_pool.NewEppPoolManager(
		&fakeTxn{},
		storager,
		clusterSource,
		nil,
		nil,
	)
	return storager, func() { container.EppPoolManager = old }
}

func newGetRequest(target string) *http.Request {
	return httptest.NewRequest(http.MethodGet, target, nil)
}

func newPutRequest(cluster, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/epp-assignments/"+cluster, strings.NewReader(body))
	return mux.SetURLVars(req, map[string]string{"cluster": cluster})
}

func TestEndpoints(t *testing.T) {
	require.Len(t, Endpoints, 2)
	for _, ep := range Endpoints {
		require.NotNil(t, ep)
		assert.NotEmpty(t, ep.Path)
		assert.NotEmpty(t, ep.Method)
	}
	assert.Equal(t, "/epp-assignments", Endpoints[0].Path)
	assert.Equal(t, http.MethodGet, Endpoints[0].Method)
	assert.Equal(t, "/epp-assignments/{cluster}", Endpoints[1].Path)
	assert.Equal(t, http.MethodPut, Endpoints[1].Method)
}

func TestGetAction(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	data, err := GetAction(newGetRequest("/epp-assignments"))
	require.NoError(t, err)

	view, ok := data.(*AssignmentsData)
	require.True(t, ok)
	require.Len(t, view.Clusters, 2)

	assert.Equal(t, "cluster-a", view.Clusters[0].Cluster)
	require.NotNil(t, view.Clusters[0].Group)
	assert.Equal(t, "g1", *view.Clusters[0].Group)
	require.NotNil(t, view.Clusters[0].Primary)
	assert.Equal(t, "epp-a", view.Clusters[0].Primary.ID)
	assert.False(t, view.Clusters[0].Degraded)

	// cluster-b is in EPP mode but has no assignment.
	assert.Equal(t, "cluster-b", view.Clusters[1].Cluster)
	assert.Nil(t, view.Clusters[1].Group)
	assert.Nil(t, view.Clusters[1].Primary)
	assert.Contains(t, view.UnassignedClusters, "cluster-b")

	// g2 has no primary occupation (standby roles do not occupy).
	assert.Equal(t, []string{"g2"}, view.IdleGroups)
}

func TestGetAction_ByCluster(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	data, err := GetAction(newGetRequest("/epp-assignments?cluster=cluster-a"))
	require.NoError(t, err)

	view, ok := data.(*AssignmentsData)
	require.True(t, ok)
	require.Len(t, view.Clusters, 1)
	assert.Equal(t, "cluster-a", view.Clusters[0].Cluster)
	assert.Empty(t, view.UnassignedClusters)
}

func TestGetAction_ByClusterUnassigned(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	data, err := GetAction(newGetRequest("/epp-assignments?cluster=cluster-b"))
	require.NoError(t, err)

	view, ok := data.(*AssignmentsData)
	require.True(t, ok)
	require.Len(t, view.Clusters, 1)
	assert.Equal(t, "cluster-b", view.Clusters[0].Cluster)
	assert.Contains(t, view.UnassignedClusters, "cluster-b")
}

func TestGetAction_ClusterNotEPP(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	// Unknown cluster and non-EPP cluster both resolve to the 404 semantic error.
	for _, cluster := range []string{"cluster-x", "cluster-wrr"} {
		_, err := GetAction(newGetRequest("/epp-assignments?cluster=" + cluster))
		require.Error(t, err, cluster)
		assert.Equal(t, 404, xerror.Resolve(err).ErrNo, cluster)
	}
}

func TestOverrideAction(t *testing.T) {
	storager, teardown := setupManager(t)
	defer teardown()

	data, err := OverrideAction(newPutRequest("cluster-a", `{"group_name":"g1","primary_instance_id":"epp-b"}`))
	require.NoError(t, err)

	view, ok := data.(*ClusterAssignmentOverrideData)
	require.True(t, ok)
	assert.Equal(t, "cluster-a", view.Cluster)
	require.NotNil(t, view.Group)
	assert.Equal(t, "g1", *view.Group)
	require.NotNil(t, view.Primary)
	assert.Equal(t, "epp-b", view.Primary.ID)

	// The OpenAPI override response must not carry a "standby" key at all
	// (the field was removed from the API contract, not nulled).
	bs, err := json.Marshal(data)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(bs, &decoded))
	assert.NotContains(t, decoded, "standby")

	stored := storager.fetchAssignment("cluster-a")
	require.NotNil(t, stored)
	assert.Equal(t, "g1", stored.GroupName)
	assert.Equal(t, "epp-b", stored.PrimaryInstanceID)
}

func TestOverrideAction_ClusterNotEPP(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	_, err := OverrideAction(newPutRequest("cluster-x", `{"group_name":"g1","primary_instance_id":"epp-a"}`))
	require.Error(t, err)
	assert.Equal(t, 404, xerror.Resolve(err).ErrNo)
}

func TestOverrideAction_ValidationErrors(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	cases := []struct {
		name  string
		body  string
		errNo int
	}{
		{"group not exist", `{"group_name":"gx","primary_instance_id":"epp-a"}`, 422},
		{"primary not in group", `{"group_name":"g1","primary_instance_id":"epp-c"}`, 422},
		{"group name empty", `{"group_name":"","primary_instance_id":"epp-a"}`, 422},
		{"primary empty", `{"group_name":"g1","primary_instance_id":""}`, 422},
		{"body empty", `{}`, 422},
		{"body not json", `{`, 422},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := OverrideAction(newPutRequest("cluster-a", tc.body))
			require.Error(t, err)
			assert.Equal(t, tc.errNo, xerror.Resolve(err).ErrNo)
		})
	}
}

func TestOverrideAction_SingleInstanceGroup(t *testing.T) {
	_, teardown := setupManager(t)
	defer teardown()

	// cluster-b is assigned to g2 whose only instance is epp-c.
	data, err := OverrideAction(newPutRequest("cluster-b", `{"group_name":"g2","primary_instance_id":"epp-c"}`))
	require.NoError(t, err)

	view, ok := data.(*ClusterAssignmentOverrideData)
	require.True(t, ok)
	assert.Equal(t, "cluster-b", view.Cluster)
	require.NotNil(t, view.Primary)
	assert.Equal(t, "epp-c", view.Primary.ID)
}
