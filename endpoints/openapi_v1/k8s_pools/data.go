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
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ik8s_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
)

// PoolEntry is the OpenAPI detail payload of one K8s pool.
type PoolEntry struct {
	Name          string                       `json:"name"`
	Instances     []iprovider.ProviderInstance `json:"instances"`
	InstanceCount int                          `json:"instance_count"`
	LastSyncTime  int64                        `json:"last_sync_time"`
}

// PoolListEntry is the OpenAPI summary payload of one K8s pool.
type PoolListEntry struct {
	Name          string `json:"name"`
	InstanceCount int    `json:"instance_count"`
	LastSyncTime  int64  `json:"last_sync_time"`
}

func poolEntryOf(pool *ik8s_pool.K8sPool) *PoolEntry {
	instances := pool.Instances
	if instances == nil {
		instances = []iprovider.ProviderInstance{}
	}
	return &PoolEntry{
		Name:          pool.Name,
		Instances:     instances,
		InstanceCount: len(instances),
		LastSyncTime:  pool.UpdateTime,
	}
}
