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
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterManager_ExportClusterTable(t *testing.T) {
	ctx := context.Background()

	t.Run("unchanged", func(t *testing.T) {
		m := newClusterManagerForExport(t, iversion_control.ZeroVersion)
		got, err := m.ExportClusterTable(ctx, iversion_control.ZeroVersion)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("changed", func(t *testing.T) {
		m := newClusterManagerForExport(t, "20240102000000")
		got, err := m.ExportClusterTable(ctx, "old")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "20240102000000", *got.Version)
	})

	t.Run("export error", func(t *testing.T) {
		vcs := &fakeVersionControlStorager{
			upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
				return "", errors.New("upsert failed")
			},
		}
		vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, vcs)
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return []*Cluster{newTestClusterBase()}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, vcm, nil, nil)
		_, err := m.ExportClusterTable(ctx, "")
		require.Error(t, err)
	})

	t.Run("generator fetch error", func(t *testing.T) {
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				return nil, errors.New("db down")
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, nil, nil, nil)
		_, err := m.clusterTableConfGenerator(ctx)
		require.Error(t, err)
	})
}

func TestClusterManager_ExportGSLB(t *testing.T) {
	ctx := context.Background()

	t.Run("unchanged", func(t *testing.T) {
		m := newClusterManagerForExportGSLB(t, iversion_control.ZeroVersion)
		got, err := m.ExportGSLB(ctx, iversion_control.ZeroVersion, "bfe1")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("changed", func(t *testing.T) {
		m := newClusterManagerForExportGSLB(t, "20240102000000")
		got, err := m.ExportGSLB(ctx, "old", "bfe1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "20240102000000", got.Version)
	})

	t.Run("bfe cluster not exist", func(t *testing.T) {
		m := newClusterManagerForExportGSLB(t, iversion_control.ZeroVersion)
		_, err := m.ExportGSLB(ctx, "", "bfe-missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BFECluster bfe-missing Not Exist")
	})

	t.Run("export error", func(t *testing.T) {
		vcs := &fakeVersionControlStorager{
			upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
				return "", errors.New("upsert failed")
			},
		}
		vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, vcs)
		clusterStore := &fakeClusterStorager{
			fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
				c := newTestClusterBase()
				return []*Cluster{c}, nil
			},
		}
		m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, vcm, nil, nil)
		_, err := m.ExportGSLB(ctx, "", "bfe1")
		require.Error(t, err)
	})
}

func TestClusterManager_gslbConfGenerator(t *testing.T) {
	ctx := context.Background()
	m := newClusterManagerForExportGSLB(t, iversion_control.ZeroVersion)
	ed, err := m.gslbConfGenerator("bfe1")(ctx)
	require.NoError(t, err)
	require.NotNil(t, ed)
	assert.Equal(t, ConfigTopicGSLB+".bfe1", ed.Topic)
	conf := ed.DataWithoutVersion.(*GSLBConf)
	require.NotNil(t, conf.Clusters)
	require.NotNil(t, (*conf.Clusters)["c1"])
}

func newClusterManagerForExportGSLB(t *testing.T, version string) *ClusterManager {
	t.Helper()
	vcs := &fakeVersionControlStorager{
		upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
			return version, nil
		},
	}
	vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, vcs)
	c := newTestClusterBase()
	c.Scheduler = map[string]map[string]int{
		"bfe1": {"sc1": 100},
	}
	clusterStore := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			return []*Cluster{c}, nil
		},
	}
	return NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, vcm, nil, nil)
}

func TestClusterTableConf_clusterWithIPv6(t *testing.T) {
	ctx := context.Background()
	vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, &fakeVersionControlStorager{
		upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
			return "20240102000000", nil
		},
	})
	c := newTestClusterBase()
	c.SubClusters[0].InstancePool.Instances[0].Addr = "2001:db8::1"
	clusterStore := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			return []*Cluster{c}, nil
		},
	}
	m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, vcm, nil, nil)
	conf, err := m.ExportClusterTable(ctx, "old")
	require.NoError(t, err)
	require.NotNil(t, conf)
	require.NotNil(t, conf.Config)
	backends := (*conf.Config)["c1"]["sc1"]
	require.Len(t, backends, 1)
	assert.Equal(t, "[2001:db8::1]", *backends[0].Addr)
	assert.Equal(t, "[2001:db8::1]_80", *backends[0].Name)
}

// newK8sExportManager builds a ClusterManager whose cluster list contains the
// given clusters, plus a provider storager and a fake k8s_pools reader for
// the realtime-resolution path.
func newK8sExportManager(t *testing.T, clusters []*Cluster, providers []*iprovider.Provider,
	reader *fakeK8sPoolInstanceReader) *ClusterManager {

	t.Helper()
	vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, &fakeVersionControlStorager{
		upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
			return "20240102000000", nil
		},
	})
	clusterStore := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			return clusters, nil
		},
	}
	providerStore := &fakeProviderStorager{
		fetchListFn: func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
			return providers, int64(len(providers)), nil
		},
	}
	m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{},
		&fakePoolStorager{}, providerStore, vcm, nil, nil)
	m.SetInstancePoolResolver(iprovider.NewInstancePoolResolver(reader))
	return m
}

func k8sRefCluster(name, provider, pool string) *Cluster {
	c := makeProviderRefCluster(name, lib.PString(provider), nil)
	c.SubClusters[0].InstancePool = &Pool{ID: 1, Name: "p." + name, Instances: []Instance{}}
	return c
}

func TestClusterTableConf_k8sRealtimeResolution(t *testing.T) {
	ctx := context.Background()

	k8sProvider := func(name, pool string) *iprovider.Provider {
		return &iprovider.Provider{
			Name:           name,
			InstanceSource: iprovider.InstanceSourceK8sPool,
			K8sPoolName:    lib.PString(pool),
		}
	}

	t.Run("k8s cluster resolves instances from k8s_pools at export time", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{
			instances: []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 8000, Weight: 100}},
		}
		m := newK8sExportManager(t,
			[]*Cluster{k8sRefCluster("c1", "prov-k8s", "svc-a")},
			[]*iprovider.Provider{k8sProvider("prov-k8s", "svc-a")},
			reader)

		conf, err := m.ExportClusterTable(ctx, "old")
		require.NoError(t, err)
		require.NotNil(t, conf)
		backends := (*conf.Config)["c1"]["sc1"]
		require.Len(t, backends, 1)
		assert.Equal(t, "10.0.0.1_8000", *backends[0].Name)
		assert.Equal(t, 1, reader.calls)
	})

	t.Run("multiple clusters sharing one pool read it only once", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{
			instances: []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 8000, Weight: 100}},
		}
		m := newK8sExportManager(t,
			[]*Cluster{
				k8sRefCluster("c1", "prov-k8s", "svc-a"),
				k8sRefCluster("c2", "prov-k8s", "svc-a"),
			},
			[]*iprovider.Provider{k8sProvider("prov-k8s", "svc-a")},
			reader)

		conf, err := m.ExportClusterTable(ctx, "old")
		require.NoError(t, err)
		require.NotNil(t, conf)
		assert.Len(t, (*conf.Config)["c1"]["sc1"], 1)
		assert.Len(t, (*conf.Config)["c2"]["sc1"], 1)
		assert.Equal(t, 1, reader.calls, "request-level pool cache must dedupe reads")
	})

	t.Run("empty pool exports the cluster as an empty entry", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{}
		m := newK8sExportManager(t,
			[]*Cluster{k8sRefCluster("c1", "prov-k8s", "svc-a")},
			[]*iprovider.Provider{k8sProvider("prov-k8s", "svc-a")},
			reader)

		conf, err := m.ExportClusterTable(ctx, "old")
		require.NoError(t, err)
		require.NotNil(t, conf)
		entry, ok := (*conf.Config)["c1"]
		require.True(t, ok, "cluster must still be exported as an empty entry")
		assert.Len(t, entry, 0)
	})

	t.Run("manual cluster keeps the snapshot path without any k8s read", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{}
		c := makeProviderRefCluster("c1", lib.PString("prov-manual"), []Instance{
			{Name: "rs1", Addr: "127.0.0.1", Port: 80, Weight: 10},
		})
		m := newK8sExportManager(t,
			[]*Cluster{c},
			[]*iprovider.Provider{{
				Name:           "prov-manual",
				InstanceSource: iprovider.InstanceSourceInstancePool,
				InstancePool:   []iprovider.ProviderInstance{{Addr: "9.9.9.9", Port: 443, Weight: 100}},
			}},
			reader)

		conf, err := m.ExportClusterTable(ctx, "old")
		require.NoError(t, err)
		require.NotNil(t, conf)
		backends := (*conf.Config)["c1"]["sc1"]
		require.Len(t, backends, 1)
		assert.Equal(t, "127.0.0.1_80", *backends[0].Name, "snapshot wins for manual clusters")
		assert.Equal(t, 0, reader.calls)
	})

	t.Run("missing provider falls back to the snapshot", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{}
		c := makeProviderRefCluster("c1", lib.PString("ghost"), []Instance{
			{Name: "rs1", Addr: "127.0.0.1", Port: 80, Weight: 10},
		})
		m := newK8sExportManager(t, []*Cluster{c}, nil, reader)

		conf, err := m.ExportClusterTable(ctx, "old")
		require.NoError(t, err)
		require.NotNil(t, conf)
		assert.Len(t, (*conf.Config)["c1"]["sc1"], 1)
		assert.Equal(t, 0, reader.calls)
	})

	t.Run("resolver read failure fails the whole export (fail-close)", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{err: errors.New("k8s_pools down")}
		m := newK8sExportManager(t,
			[]*Cluster{k8sRefCluster("c1", "prov-k8s", "svc-a")},
			[]*iprovider.Provider{k8sProvider("prov-k8s", "svc-a")},
			reader)

		_, err := m.ExportClusterTable(ctx, "old")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "k8s_pools down")
	})

	t.Run("unconfigured resolver fails the export instead of draining silently", func(t *testing.T) {
		m := newK8sExportManager(t,
			[]*Cluster{k8sRefCluster("c1", "prov-k8s", "svc-a")},
			[]*iprovider.Provider{k8sProvider("prov-k8s", "svc-a")},
			&fakeK8sPoolInstanceReader{})
		m.SetInstancePoolResolver(nil)

		_, err := m.ExportClusterTable(ctx, "old")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolver is not configured")
	})
}

func TestClusterTableConf_backendNameUsesAddrPort(t *testing.T) {
	ctx := context.Background()
	vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, &fakeVersionControlStorager{
		upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
			return "20240102000000", nil
		},
	})
	c := newTestClusterBase()
	clusterStore := &fakeClusterStorager{
		fetchClusterListFn: func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
			return []*Cluster{c}, nil
		},
	}
	m := NewClusterManager(&fakeTxn{}, clusterStore, &fakeSubClusterStorager{}, &fakeBFEClusterStorager{}, &fakePoolStorager{}, nil, vcm, nil, nil)
	conf, err := m.ExportClusterTable(ctx, "old")
	require.NoError(t, err)
	require.NotNil(t, conf)
	require.NotNil(t, conf.Config)
	backends := (*conf.Config)["c1"]["sc1"]
	require.Len(t, backends, 1)
	assert.Equal(t, "127.0.0.1_80", *backends[0].Name)
}
