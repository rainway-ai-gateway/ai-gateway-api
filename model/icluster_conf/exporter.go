// Copyright (c) 2021 The BFE Authors.
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
	"fmt"
	"net"

	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_table_conf"
	"github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/gslb_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
)

const (
	ConfigTopicClusterTable = "cluster_table"
	ConfigTopicGSLB         = "gslb"
)

type ClusterTableConf struct {
	cluster_table_conf.ClusterTableConf
}

func (ctc *ClusterTableConf) UpdateVersion(version string) error {
	ctc.Version = &version

	return nil
}

// clusterTableConfGenerator generates the cluster_table export. Clusters
// referencing a k8s_pool-mode provider resolve their instances in realtime
// from k8s_pools (single source of truth, no mirror/derived snapshot); all
// other clusters keep the historical snapshot path. All reads happen in the
// caller's transaction context, so cluster/provider/k8s_pools views are
// consistent; any read failure fails the whole export (fail-close) and the
// data plane keeps the last successful version.
func (rm *ClusterManager) clusterTableConfGenerator(ctx context.Context) (*iversion_control.ExportData, error) {
	clusters, err := rm.storager.FetchClusterList(ctx, nil)
	if err != nil {
		return nil, err
	}

	// Fetch all providers once and index them by name to avoid an N+1 lookup
	// per cluster. A missing provider (dirty data) falls back to the snapshot
	// path below, preserving the pre-refactor behavior.
	providerByName := map[string]*iprovider.Provider{}
	if rm.providerStorager != nil {
		providers, _, err := rm.providerStorager.FetchProviderList(ctx, nil)
		if err != nil {
			return nil, err
		}
		for _, p := range providers {
			providerByName[p.Name] = p
		}
	}

	// Request-level cache: one k8s_pools read per pool name even when several
	// clusters reference the same provider/pool.
	poolCache := map[string][]iprovider.ProviderInstance{}

	allClusters := cluster_table_conf.AllClusterBackend{}
	for _, cluster := range clusters {
		clusterBackend := map[string]cluster_table_conf.SubClusterBackend{}

		var provider *iprovider.Provider
		if cluster.LLMConfig != nil && cluster.LLMConfig.Provider != nil {
			provider = providerByName[*cluster.LLMConfig.Provider]
		}
		k8sMode := provider != nil && provider.InstanceSource == iprovider.InstanceSourceK8sPool

		var k8sInstances []iprovider.ProviderInstance
		if k8sMode {
			poolName := ""
			if provider.K8sPoolName != nil {
				poolName = *provider.K8sPoolName
			}
			if cached, ok := poolCache[poolName]; ok {
				k8sInstances = cached
			} else {
				if rm.instancePoolResolver == nil {
					return nil, xerror.WrapModelErrorWithMsg(
						"resolve k8s pool %q for cluster %q: instance pool resolver is not configured",
						poolName, cluster.Name)
				}
				inst, err := rm.instancePoolResolver.Effective(ctx, provider)
				if err != nil {
					return nil, xerror.WrapModelErrorWithMsg(
						"resolve k8s pool %q for cluster %q: %v", poolName, cluster.Name, err)
				}
				poolCache[poolName] = inst
				k8sInstances = inst
			}
		}

		for _, subCluster := range cluster.SubClusters {
			var instances []Instance
			if k8sMode {
				// Same semantics as the removed fan-out: the provider's k8s
				// instances apply to every subcluster of the cluster.
				instances = providerInstancesToClusterInstances(k8sInstances)
			} else {
				if subCluster.InstancePool == nil {
					continue
				}
				instances = subCluster.InstancePool.Instances
			}
			if len(instances) == 0 {
				continue
			}

			subClusterBackend := make(cluster_table_conf.SubClusterBackend, 0, len(instances))
			for _, instance := range instances {
				addr := instance.Addr
				// IPv6 地址需要加 [] 包裹
				if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
					addr = "[" + addr + "]"
				}
				name := fmt.Sprintf("%s_%d", addr, instance.Port)
				subClusterBackend = append(subClusterBackend, &cluster_table_conf.BackendConf{
					Name:   lib.PString(name),
					Addr:   lib.PString(addr),
					Port:   lib.PInt(instance.Port),
					Weight: lib.PInt(int(instance.Weight)),
				})
			}

			clusterBackend[subCluster.Name] = subClusterBackend
		}

		allClusters[cluster.Name] = clusterBackend
	}

	clusterTableConf := &ClusterTableConf{
		ClusterTableConf: cluster_table_conf.ClusterTableConf{
			Config: &allClusters,
		},
	}
	clusterTableConf.UpdateVersion(iversion_control.ZeroVersion)

	if err := clusterTableConf.ClusterTableConf.Config.Check(); err != nil {
		return nil, xerror.WrapModelError(err)
	}

	return &iversion_control.ExportData{
		Topic:              ConfigTopicClusterTable,
		DataWithoutVersion: clusterTableConf,
	}, nil
}

func (rm *ClusterManager) ExportClusterTable(ctx context.Context, lastVersion string) (*ClusterTableConf, error) {
	ed, err := rm.versionControlManager.ExportConfig(ctx, ConfigTopicClusterTable, rm.clusterTableConfGenerator)
	if err != nil {
		return nil, err
	}

	conf := ed.DataWithoutVersion.(*ClusterTableConf)
	if *conf.Version == lastVersion {
		return nil, nil
	}

	return conf, nil
}

type GSLBConf struct {
	Version string
	gslb_conf.GslbConf
}

func (gc *GSLBConf) UpdateVersion(version string) error {
	gc.Version = version
	gc.Ts = &version

	return nil
}

func (rm *ClusterManager) gslbConfGenerator(bfeClusterName string) func(ctx context.Context) (*iversion_control.ExportData, error) {
	return func(ctx context.Context) (*iversion_control.ExportData, error) {
		topic := ConfigTopicGSLB + "." + bfeClusterName
		clusters, err := rm.storager.FetchClusterList(ctx, nil)
		if err != nil {
			return nil, err
		}

		gslbClustersConf := gslb_conf.GslbClustersConf{}
		for _, cluster := range clusters {
			scheduler := cluster.Scheduler
			if len(scheduler) == 0 {
				continue
			}

			lbMatraix := scheduler[bfeClusterName]
			if lbMatraix == nil {
				return nil, xerror.WrapParamErrorWithMsg("BFECluster %s Not Exist", bfeClusterName)
			}

			gslbClustersConf[cluster.Name] = lbMatraix
		}

		gslbConf := &GSLBConf{
			GslbConf: gslb_conf.GslbConf{
				Clusters: &gslbClustersConf,
				Hostname: lib.PString("gslb.manual.com"),
			},
		}
		gslbConf.UpdateVersion(iversion_control.ZeroVersion)

		if err := gslbConf.GslbConf.Check(); err != nil {
			return nil, xerror.WrapModelError(err)
		}

		return &iversion_control.ExportData{
			Topic:              topic,
			DataWithoutVersion: gslbConf,
		}, nil
	}
}

func (rm *ClusterManager) ExportGSLB(ctx context.Context, lastVersion, bfeClusterName string) (*GSLBConf, error) {
	topic := ConfigTopicGSLB + "." + bfeClusterName
	ed, err := rm.versionControlManager.ExportConfig(ctx, topic, rm.gslbConfGenerator(bfeClusterName))
	if err != nil {
		return nil, err
	}

	conf := ed.DataWithoutVersion.(*GSLBConf)
	if conf.Version == lastVersion {
		return nil, nil
	}

	return conf, nil
}
