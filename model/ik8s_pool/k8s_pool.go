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

// Package ik8s_pool implements the K8s instance pool domain (k8s-pools.md):
// the pool is a control-plane resource whose member list is written only by
// the K8s discovery component through the /k8s_pools InnerAPI. k8s_pools is
// the single source of truth for k8s-sourced instances: providers referencing
// the pool (instance_source=k8s_pool, k8s_pool_name=name) keep only the name
// reference, and every consumer (cluster_table export, batch egress) resolves
// instances at read time through iprovider.InstancePoolResolver. PUT/DELETE
// therefore write only the k8s_pools table — no provider mirror, no fan-out.
package ik8s_pool

import (
	"context"
	"fmt"
	"strings"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
)

// DefaultInstanceWeight is the weight assigned to a discovered instance when
// the discovery component omits it (k8s-pools.md §2).
const DefaultInstanceWeight int64 = 100

// K8sPool is the control-plane representation of one k8s_pools row.
type K8sPool struct {
	Name       string
	Instances  []iprovider.ProviderInstance
	CreateTime int64
	UpdateTime int64
}

// InstanceParam is one element of the PUT /k8s_pools/{name}/instances body.
type InstanceParam struct {
	Addr   *string `json:"addr"`
	Port   *int    `json:"port"`
	Weight *int64  `json:"weight,omitempty"`
}

// ValidateInstances normalizes and validates a full-replacement instance
// snapshot: addr required, port within 1-65535, weight defaulting to 100 and
// within 0-100, and (addr, port) unique within the pool. An empty list is
// valid and means "zero instances".
func ValidateInstances(params []InstanceParam) ([]iprovider.ProviderInstance, error) {
	rst := make([]iprovider.ProviderInstance, 0, len(params))
	comboSet := map[string]struct{}{}
	for i, p := range params {
		if p.Addr == nil || strings.TrimSpace(*p.Addr) == "" {
			return nil, xerror.WrapParamErrorWithMsg("instances[%d]: addr is required", i)
		}
		if p.Port == nil || *p.Port < 1 || *p.Port > 65535 {
			return nil, xerror.WrapParamErrorWithMsg("instances[%d]: port must be between 1 and 65535", i)
		}
		weight := DefaultInstanceWeight
		if p.Weight != nil {
			if *p.Weight < 0 || *p.Weight > 100 {
				return nil, xerror.WrapParamErrorWithMsg("instances[%d]: weight must be between 0 and 100", i)
			}
			weight = *p.Weight
		}
		key := fmt.Sprintf("%s|%d", *p.Addr, *p.Port)
		if _, ok := comboSet[key]; ok {
			return nil, xerror.WrapParamErrorWithMsg("instances: duplicate addr/port combination: %s", key)
		}
		comboSet[key] = struct{}{}
		rst = append(rst, iprovider.ProviderInstance{
			Addr:   *p.Addr,
			Port:   *p.Port,
			Weight: weight,
		})
	}
	return rst, nil
}

// K8sPoolStorager defines persistence operations for k8s_pools. It embeds
// iprovider.K8sPoolInstanceReader so the same storager backs the realtime
// resolution path (InstancePoolResolver) without a second read implementation.
type K8sPoolStorager interface {
	iprovider.K8sPoolInstanceReader
	// UpsertPool creates or replaces the instance list of the named pool.
	UpsertPool(ctx context.Context, name string, instances []iprovider.ProviderInstance) error
	// FetchPool returns the named pool, or (nil, nil) when it does not exist.
	FetchPool(ctx context.Context, name string) (*K8sPool, error)
	// FetchPoolList returns all pools ordered by id.
	FetchPoolList(ctx context.Context) ([]*K8sPool, error)
	// DeletePool deletes the named pool.
	DeletePool(ctx context.Context, name string) error
}

// K8sPoolManager provides business-level operations for k8s_pools.
type K8sPoolManager struct {
	txn              itxn.TxnStorager
	storager         K8sPoolStorager
	providerStorager iprovider.ProviderStorager
}

// NewK8sPoolManager creates a K8sPoolManager. Dependencies are injected (no
// stateful.Default* access) so tests can mock them. The provider storager is
// only read (reference checks); the manager never writes providers, clusters
// or derived pools — k8s_pools is the only table it mutates.
func NewK8sPoolManager(txn itxn.TxnStorager, storager K8sPoolStorager,
	providerStorager iprovider.ProviderStorager) *K8sPoolManager {

	return &K8sPoolManager{
		txn:              txn,
		storager:         storager,
		providerStorager: providerStorager,
	}
}

// ReplaceInstances replaces the pool's instance list (idempotent upsert; an
// empty list means "zero instances"). Only the k8s_pools table is written;
// consumers pick the new list up at their next read (cluster_table export
// version bumps automatically via the export signature).
func (m *K8sPoolManager) ReplaceInstances(ctx context.Context, name string,
	instances []iprovider.ProviderInstance) (*K8sPool, error) {

	if err := m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		return m.storager.UpsertPool(ctx, name, instances)
	}); err != nil {
		return nil, err
	}

	return m.storager.FetchPool(ctx, name)
}

// DeletePool deletes the named pool with reference awareness: when no
// provider references it the row is removed (404 afterwards); when providers
// still reference it the row is kept with its instances cleared (so GET
// returns 200 + instance_count=0 and last_sync_time stays inspectable). Both
// paths write only the k8s_pools table within a single transaction; the
// provider table is only read. "Pool does not exist" is equivalent to "zero
// instances" for consumers either way.
func (m *K8sPoolManager) DeletePool(ctx context.Context, name string) error {
	existing, err := m.storager.FetchPool(ctx, name)
	if err != nil {
		return err
	}
	if existing == nil {
		return xerror.WrapRecordNotExist("k8s pool")
	}

	referenced, err := m.hasReferencingProviders(ctx, name)
	if err != nil {
		return err
	}

	return m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		if referenced {
			return m.storager.UpsertPool(ctx, name, []iprovider.ProviderInstance{})
		}
		return m.storager.DeletePool(ctx, name)
	})
}

// FetchPool fetches a single pool by name.
func (m *K8sPoolManager) FetchPool(ctx context.Context, name string) (*K8sPool, error) {
	return m.storager.FetchPool(ctx, name)
}

// FetchPoolList fetches all pools.
func (m *K8sPoolManager) FetchPoolList(ctx context.Context) ([]*K8sPool, error) {
	return m.storager.FetchPoolList(ctx)
}

// hasReferencingProviders reports whether any provider references the named
// pool (instance_source=k8s_pool && k8s_pool_name==name). Read-only.
func (m *K8sPoolManager) hasReferencingProviders(ctx context.Context, name string) (bool, error) {
	k8sSource := iprovider.InstanceSourceK8sPool
	providers, _, err := m.providerStorager.FetchProviderList(ctx, &iprovider.ProviderFilter{
		InstanceSource: &k8sSource,
		K8sPoolName:    &name,
	})
	if err != nil {
		return false, err
	}
	return len(providers) > 0, nil
}
