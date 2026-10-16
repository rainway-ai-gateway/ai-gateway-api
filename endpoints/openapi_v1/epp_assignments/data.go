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
	"github.com/rainway-ai-gateway/ai-gateway-api/model/epp_pool"
)

// InstanceData is one EPP instance as exposed by the OpenAPI
// (same shape as the /epp-pool instance: id/host/port only).
type InstanceData struct {
	ID   string `json:"id"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// ClusterAssignmentData is the per-cluster expanded assignment view
// (epp-assignments.md §2.1 clusters[] element). The OpenAPI view exposes
// the primary only; the model-layer standby expansion stays internal
// (InnerAPI epp_data / EPPAddr exports).
type ClusterAssignmentData struct {
	Cluster  string        `json:"cluster"`
	Group    *string       `json:"group"`
	Primary  *InstanceData `json:"primary"`
	Degraded bool          `json:"degraded"`
}

// AssignmentsData is the full assignment view: per-cluster expansion plus
// the unassigned cluster list and the idle group list.
type AssignmentsData struct {
	Clusters           []*ClusterAssignmentData `json:"clusters"`
	UnassignedClusters []string                 `json:"unassigned_clusters"`
	IdleGroups         []string                 `json:"idle_groups"`
}

// ClusterAssignmentOverrideData is the response of the manual override
// (epp-assignments.md §2.2): the cluster assignment view without the
// degraded marker.
type ClusterAssignmentOverrideData struct {
	Cluster string        `json:"cluster"`
	Group   *string       `json:"group"`
	Primary *InstanceData `json:"primary"`
}

func newInstanceData(inst *epp_pool.InstanceParam) *InstanceData {
	if inst == nil {
		return nil
	}
	return &InstanceData{
		ID:   inst.ID,
		Host: inst.Host,
		Port: inst.Port,
	}
}

func newClusterAssignmentData(view *epp_pool.ClusterAssignmentView) *ClusterAssignmentData {
	return &ClusterAssignmentData{
		Cluster:  view.Cluster,
		Group:    view.Group,
		Primary:  newInstanceData(view.Primary),
		Degraded: view.Degraded,
	}
}

func newAssignmentsData(view *epp_pool.AssignmentsView) *AssignmentsData {
	rst := &AssignmentsData{
		Clusters:           []*ClusterAssignmentData{},
		UnassignedClusters: view.UnassignedClusters,
		IdleGroups:         view.IdleGroups,
	}
	for _, entry := range view.Clusters {
		rst.Clusters = append(rst.Clusters, newClusterAssignmentData(entry))
	}
	return rst
}

func newClusterAssignmentOverrideData(view *epp_pool.ClusterAssignmentView) *ClusterAssignmentOverrideData {
	return &ClusterAssignmentOverrideData{
		Cluster: view.Cluster,
		Group:   view.Group,
		Primary: newInstanceData(view.Primary),
	}
}
