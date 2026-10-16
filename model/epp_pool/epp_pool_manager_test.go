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
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testManager(store *memoryEppPoolStorager, source EPPClusterSource) *EppPoolManager {
	return NewEppPoolManager(&fakeTxn{}, store, source, nil, &ManagerOptions{
		PoolName: "EPP.pool",
	})
}

func TestGetPool(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	store.seedInstances(
		inst("epp-b", "g1", "10.0.0.2", 9002),
		inst("epp-a", "g1", "10.0.0.1", 9002),
		inst("epp-c", "g2", "10.0.0.3", 9002),
	)

	m := testManager(store, &fakeClusterSource{})

	pool, err := m.GetPool(ctx)
	require.NoError(t, err)
	assert.Equal(t, "EPP.pool", pool.Name)
	require.Len(t, pool.Groups, 2)
	assert.Equal(t, "g1", pool.Groups[0].Name)
	require.Len(t, pool.Groups[0].Instances, 2)
	// Instances id-ordered within a group.
	assert.Equal(t, "epp-a", pool.Groups[0].Instances[0].ID)
	assert.Equal(t, "epp-b", pool.Groups[0].Instances[1].ID)
	assert.Equal(t, "g2", pool.Groups[1].Name)
}

func TestGetPool_NotExist(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	m := testManager(store, &fakeClusterSource{})

	// Pool never patched: record-not-exist per epp-pool.md §2.1.
	_, err := m.GetPool(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Record Not Exist")
}

func TestPatchPool_FullReplace(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	store.seedInstances(inst("epp-old", "g0", "10.0.0.9", 9002))

	m := testManager(store, &fakeClusterSource{})

	pool, err := m.PatchPool(ctx, []*InstanceGroup{
		{Name: "g1", Instances: []*InstanceParam{
			{ID: "epp-a", Host: "10.0.0.1", Port: 9002},
		}},
		{Name: "g2", Instances: []*InstanceParam{
			{ID: "epp-b", Host: "10.0.0.2", Port: 9002},
		}},
	})
	require.NoError(t, err)
	require.Len(t, pool.Groups, 2)
	assert.Equal(t, "g1", pool.Groups[0].Name)

	// Old instances fully replaced.
	instances, err := store.FetchInstanceList(ctx, &InstanceFilter{})
	require.NoError(t, err)
	require.Len(t, instances, 2)
	groups := map[string]bool{}
	for _, one := range instances {
		assert.NotEqual(t, "epp-old", one.ID)
		groups[one.GroupName] = true
	}
	assert.True(t, groups["g1"])
	assert.True(t, groups["g2"])
}

func TestPatchPool_Validation(t *testing.T) {
	ctx := context.Background()

	validGroup := func() *InstanceGroup {
		return &InstanceGroup{Name: "g1", Instances: []*InstanceParam{
			{ID: "epp-a", Host: "10.0.0.1", Port: 9002},
		}}
	}

	cases := []struct {
		name   string
		groups []*InstanceGroup
	}{
		{"empty groups", nil},
		{"empty group name", []*InstanceGroup{{Name: "", Instances: validGroup().Instances}}},
		{"duplicated group name", []*InstanceGroup{validGroup(), validGroup()}},
		{"empty instance id", []*InstanceGroup{{Name: "g1", Instances: []*InstanceParam{{ID: "", Host: "10.0.0.1", Port: 9002}}}}},
		{"duplicated instance id", []*InstanceGroup{
			{Name: "g1", Instances: []*InstanceParam{{ID: "epp-a", Host: "10.0.0.1", Port: 9002}}},
			{Name: "g2", Instances: []*InstanceParam{{ID: "epp-a", Host: "10.0.0.2", Port: 9002}}},
		}},
		{"duplicated address", []*InstanceGroup{
			{Name: "g1", Instances: []*InstanceParam{{ID: "epp-a", Host: "10.0.0.1", Port: 9002}}},
			{Name: "g2", Instances: []*InstanceParam{{ID: "epp-b", Host: "10.0.0.1", Port: 9002}}},
		}},
		{"bad host", []*InstanceGroup{{Name: "g1", Instances: []*InstanceParam{{ID: "epp-a", Host: "-bad", Port: 9002}}}}},
		{"bad port", []*InstanceGroup{{Name: "g1", Instances: []*InstanceParam{{ID: "epp-a", Host: "10.0.0.1", Port: 0}}}}},
		{"three instances rejected", []*InstanceGroup{{Name: "g1", Instances: []*InstanceParam{{ID: "epp-a", Host: "10.0.0.1", Port: 9002}, {ID: "epp-b", Host: "10.0.0.2", Port: 9002}, {ID: "epp-c", Host: "10.0.0.3", Port: 9002}}}}},
		{"empty group rejected", []*InstanceGroup{{Name: "g1", Instances: nil}}},
		{"nil group", []*InstanceGroup{nil}},
		{"nil instance", []*InstanceGroup{{Name: "g1", Instances: []*InstanceParam{nil}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemoryEppPoolStorager()
			m := testManager(store, &fakeClusterSource{})
			_, err := m.PatchPool(ctx, tc.groups)
			require.Error(t, err)
		})
	}

	t.Run("single instance group allowed", func(t *testing.T) {
		store := newMemoryEppPoolStorager()
		m := testManager(store, &fakeClusterSource{})
		_, err := m.PatchPool(ctx, []*InstanceGroup{
			{Name: "g1", Instances: []*InstanceParam{{ID: "epp-a", Host: "10.0.0.1", Port: 9002}}},
		})
		require.NoError(t, err)
	})

	t.Run("two instances rejected with new message", func(t *testing.T) {
		store := newMemoryEppPoolStorager()
		m := testManager(store, &fakeClusterSource{})
		_, err := m.PatchPool(ctx, []*InstanceGroup{
			{Name: "g1", Instances: []*InstanceParam{
				{ID: "epp-a", Host: "10.0.0.1", Port: 9002},
				{ID: "epp-b", Host: "10.0.0.2", Port: 9002},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "requires exactly 1 instance")
	})
}

func TestAssignCluster(t *testing.T) {
	ctx := context.Background()

	setup := func() (*EppPoolManager, *memoryEppPoolStorager) {
		store := newMemoryEppPoolStorager()
		store.seedInstances(
			inst("epp-a", "g1", "10.0.0.1", 9002),
			inst("epp-b", "g1", "10.0.0.2", 9002),
			inst("epp-c", "g2", "10.0.0.3", 9002),
			inst("epp-d", "g2", "10.0.0.4", 9002),
		)
		return testManager(store, &fakeClusterSource{}), store
	}

	t.Run("assigns greedily", func(t *testing.T) {
		m, store := setup()
		require.NoError(t, m.AssignCluster(ctx, "cluster-a"))

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		require.NotNil(t, assignment)
		assert.Equal(t, "g1", assignment.GroupName)
		assert.Equal(t, "epp-a", assignment.PrimaryInstanceID)
	})

	t.Run("idempotent on valid existing assignment", func(t *testing.T) {
		m, store := setup()
		require.NoError(t, m.AssignCluster(ctx, "cluster-a"))
		store.upsertCalls = 0

		require.NoError(t, m.AssignCluster(ctx, "cluster-a"))
		assert.Equal(t, 0, store.upsertCalls)

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "epp-a", assignment.PrimaryInstanceID)
	})

	t.Run("reassigns dangling assignment", func(t *testing.T) {
		m, store := setup()
		store.seedAssignments(&AssignmentParam{Cluster: "cluster-a", GroupName: "g9", PrimaryInstanceID: "epp-x"})

		require.NoError(t, m.AssignCluster(ctx, "cluster-a"))

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "g1", assignment.GroupName)
	})

	t.Run("empty cluster name rejected", func(t *testing.T) {
		m, _ := setup()
		require.Error(t, m.AssignCluster(ctx, ""))
	})
}

func TestAssignCluster_UpsertConflictRetry(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	store.seedInstances(
		inst("epp-a", "g1", "10.0.0.1", 9002),
		inst("epp-b", "g1", "10.0.0.2", 9002),
	)
	m := testManager(store, &fakeClusterSource{})

	// First upsert conflicts (concurrent writer), retry must succeed.
	store.upsertConflictAt = 1
	require.NoError(t, m.AssignCluster(ctx, "cluster-a"))

	assignment, err := store.FetchAssignment(ctx, "cluster-a")
	require.NoError(t, err)
	require.NotNil(t, assignment)
	assert.Equal(t, "epp-a", assignment.PrimaryInstanceID)
}

func TestRepairDangling(t *testing.T) {
	ctx := context.Background()

	setup := func() (*EppPoolManager, *memoryEppPoolStorager) {
		store := newMemoryEppPoolStorager()
		store.seedInstances(
			inst("epp-a", "g1", "10.0.0.1", 9002),
			inst("epp-b", "g1", "10.0.0.2", 9002),
			inst("epp-c", "g2", "10.0.0.3", 9002),
			inst("epp-d", "g2", "10.0.0.4", 9002),
		)
		return testManager(store, &fakeClusterSource{}), store
	}

	t.Run("same group reselect", func(t *testing.T) {
		m, store := setup()
		store.seedAssignments(&AssignmentParam{Cluster: "cluster-a", GroupName: "g1", PrimaryInstanceID: "epp-x"})

		require.NoError(t, m.RepairDangling(ctx))

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "g1", assignment.GroupName)
		assert.Equal(t, "epp-a", assignment.PrimaryInstanceID)
	})

	t.Run("cross group reallocate", func(t *testing.T) {
		m, store := setup()
		// g9 removed from pool entirely.
		store.seedAssignments(&AssignmentParam{Cluster: "cluster-a", GroupName: "g9", PrimaryInstanceID: "epp-x"})

		require.NoError(t, m.RepairDangling(ctx))

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "g1", assignment.GroupName)
	})

	t.Run("clear when pool has no candidate groups", func(t *testing.T) {
		store := newMemoryEppPoolStorager()
		store.seedAssignments(&AssignmentParam{Cluster: "cluster-a", GroupName: "g1", PrimaryInstanceID: "epp-a"})
		m := testManager(store, &fakeClusterSource{})

		require.NoError(t, m.RepairDangling(ctx))

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Nil(t, assignment)
	})

	t.Run("valid assignments untouched", func(t *testing.T) {
		m, store := setup()
		store.seedAssignments(&AssignmentParam{Cluster: "cluster-a", GroupName: "g2", PrimaryInstanceID: "epp-c"})

		require.NoError(t, m.RepairDangling(ctx))

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "epp-c", assignment.PrimaryInstanceID)
	})
}

func TestOverrideAssignment(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	store.seedInstances(
		inst("epp-a", "g1", "10.0.0.1", 9002),
		inst("epp-b", "g1", "10.0.0.2", 9002),
	)
	source := &fakeClusterSource{clusters: []*EPPClusterInfo{{Name: "cluster-a"}}}
	m := testManager(store, source)

	t.Run("valid override", func(t *testing.T) {
		view, err := m.OverrideAssignment(ctx, "cluster-a", "g1", "epp-b")
		require.NoError(t, err)
		require.NotNil(t, view)
		assert.Equal(t, "cluster-a", view.Cluster)
		assert.Equal(t, "epp-b", view.Primary.ID)
		require.NotNil(t, view.Standby)
		assert.Equal(t, "epp-a", view.Standby.ID)
		assert.False(t, view.Degraded)

		assignment, err := store.FetchAssignment(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "epp-b", assignment.PrimaryInstanceID)
	})

	t.Run("unknown group", func(t *testing.T) {
		_, err := m.OverrideAssignment(ctx, "cluster-a", "g9", "epp-a")
		require.Error(t, err)
	})

	t.Run("primary not in group", func(t *testing.T) {
		_, err := m.OverrideAssignment(ctx, "cluster-a", "g1", "epp-x")
		require.Error(t, err)
	})

	t.Run("cluster not EPP", func(t *testing.T) {
		// cluster-x is unknown and cluster-wrr is not in the EPP cluster list:
		// both resolve to the record-not-exist error and must not upsert.
		for _, cluster := range []string{"cluster-x", "cluster-wrr"} {
			_, err := m.OverrideAssignment(ctx, cluster, "g1", "epp-a")
			require.Error(t, err, cluster)
			assert.Equal(t, 404, xerror.Resolve(err).ErrNo, cluster)

			assignment, err := store.FetchAssignment(ctx, cluster)
			require.NoError(t, err, cluster)
			assert.Nil(t, assignment, cluster)
		}
	})

	t.Run("cluster source not configured", func(t *testing.T) {
		m := testManager(store, nil)
		_, err := m.OverrideAssignment(ctx, "cluster-a", "g1", "epp-a")
		require.Error(t, err)
	})

	t.Run("empty args", func(t *testing.T) {
		_, err := m.OverrideAssignment(ctx, "", "g1", "epp-a")
		require.Error(t, err)
		_, err = m.OverrideAssignment(ctx, "cluster-a", "", "epp-a")
		require.Error(t, err)
		_, err = m.OverrideAssignment(ctx, "cluster-a", "g1", "")
		require.Error(t, err)
	})
}

func TestGetAssignmentsView(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	store.seedInstances(
		inst("epp-a", "g1", "10.0.0.1", 9002),
		inst("epp-b", "g1", "10.0.0.2", 9002),
	)
	source := &fakeClusterSource{clusters: []*EPPClusterInfo{
		{Name: "cluster-a"},
		{Name: "cluster-b"},
	}}
	store.seedAssignments(&AssignmentParam{Cluster: "cluster-a", GroupName: "g1", PrimaryInstanceID: "epp-a"})

	m := testManager(store, source)

	t.Run("full view", func(t *testing.T) {
		view, err := m.GetAssignmentsView(ctx)
		require.NoError(t, err)
		require.Len(t, view.Clusters, 2)
		assert.Equal(t, "epp-a", view.Clusters[0].Primary.ID)
		assert.Equal(t, []string{"cluster-b"}, view.UnassignedClusters)
		assert.Empty(t, view.IdleGroups)
	})

	t.Run("single cluster view", func(t *testing.T) {
		entry, err := m.GetAssignmentView(ctx, "cluster-a")
		require.NoError(t, err)
		assert.Equal(t, "epp-a", entry.Primary.ID)

		_, err = m.GetAssignmentView(ctx, "cluster-x")
		require.Error(t, err)
	})

	t.Run("nil cluster source", func(t *testing.T) {
		m := testManager(store, nil)
		_, err := m.GetAssignmentsView(ctx)
		require.Error(t, err)
	})
}

func TestReconcile_BackfillsMissingAssignments(t *testing.T) {
	ctx := context.Background()
	store := newMemoryEppPoolStorager()
	store.seedInstances(
		inst("epp-a", "g1", "10.0.0.1", 9002),
		inst("epp-b", "g1", "10.0.0.2", 9002),
	)
	source := &fakeClusterSource{clusters: []*EPPClusterInfo{
		{Name: "cluster-a"},
		{Name: "cluster-b"},
	}}
	m := testManager(store, source)

	require.NoError(t, m.Reconcile(ctx))

	assignmentA, err := store.FetchAssignment(ctx, "cluster-a")
	require.NoError(t, err)
	require.NotNil(t, assignmentA)

	assignmentB, err := store.FetchAssignment(ctx, "cluster-b")
	require.NoError(t, err)
	require.NotNil(t, assignmentB)

	// Reconcile is idempotent: a second pass performs no writes.
	store.upsertCalls = 0
	require.NoError(t, m.Reconcile(ctx))
	assert.Equal(t, 0, store.upsertCalls)
}

func TestReconcile_NilSource(t *testing.T) {
	m := testManager(newMemoryEppPoolStorager(), nil)
	require.NoError(t, m.Reconcile(context.Background()))
}

func TestManagerOptionsDefaults(t *testing.T) {
	m := NewEppPoolManager(&fakeTxn{}, newMemoryEppPoolStorager(), nil, nil, nil)
	assert.Equal(t, DefaultEPPInstancePoolName, m.PoolName())
	assert.Equal(t, DefaultReconcileInterval, m.ReconcileInterval())
}

func TestManager_ReconcilerLifecycle(t *testing.T) {
	m := NewEppPoolManager(&fakeTxn{}, newMemoryEppPoolStorager(), nil, nil, &ManagerOptions{
		ReconcileInterval: 10,
	})
	m.StartReconciler()
	m.StopReconciler()
}
