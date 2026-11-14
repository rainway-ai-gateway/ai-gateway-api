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

package k8s_pool

import (
	"context"
	"encoding/json"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ik8s_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

// RDBK8sPoolStorager implements ik8s_pool.K8sPoolStorager using RDB.
type RDBK8sPoolStorager struct {
	dbCtxFactory lib.DBContextFactory
}

var _ ik8s_pool.K8sPoolStorager = &RDBK8sPoolStorager{}

// NewRDBK8sPoolStorager creates a new RDB-backed k8s pool storager.
func NewRDBK8sPoolStorager(dbCtxFactory lib.DBContextFactory) *RDBK8sPoolStorager {
	return &RDBK8sPoolStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

// marshalInstances always produces a non-nil JSON array, so an empty instance
// list is persisted as "[]" (zero instances) rather than a NULL column.
func marshalInstances(instances []iprovider.ProviderInstance) (*string, error) {
	if instances == nil {
		instances = []iprovider.ProviderInstance{}
	}
	data, err := json.Marshal(instances)
	if err != nil {
		return nil, xerror.WrapParamErrorWithMsg("instances marshal, err: %s", err)
	}
	return lib.PString(string(data)), nil
}

func (s *RDBK8sPoolStorager) UpsertPool(ctx context.Context, name string,
	instances []iprovider.ProviderInstance) error {

	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	detail, err := marshalInstances(instances)
	if err != nil {
		return err
	}

	existing, err := dao.TK8sPoolOne(dbCtx, &dao.TK8sPoolParam{Name: &name})
	if err != nil {
		return err
	}
	if existing == nil {
		_, err = dao.TK8sPoolCreate(dbCtx, &dao.TK8sPoolParam{
			Name:      &name,
			Instances: detail,
		})
		return err
	}

	_, err = dao.TK8sPoolUpdate(dbCtx, &dao.TK8sPoolParam{
		Instances: detail,
	}, &dao.TK8sPoolParam{Name: &name})
	return err
}

func (s *RDBK8sPoolStorager) FetchPool(ctx context.Context, name string) (*ik8s_pool.K8sPool, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	one, err := dao.TK8sPoolOne(dbCtx, &dao.TK8sPoolParam{Name: &name})
	if err != nil {
		return nil, err
	}
	return fromDAO(one), nil
}

// FetchK8sPoolInstances implements iprovider.K8sPoolInstanceReader: it
// returns the current instance list of the named pool for realtime
// resolution. A missing pool yields a non-nil empty list (zero instances),
// keeping "pool does not exist" equivalent to "zero instances" (I4).
func (s *RDBK8sPoolStorager) FetchK8sPoolInstances(ctx context.Context,
	name string) ([]iprovider.ProviderInstance, error) {

	pool, err := s.FetchPool(ctx, name)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		return []iprovider.ProviderInstance{}, nil
	}
	return pool.Instances, nil
}

func (s *RDBK8sPoolStorager) FetchPoolList(ctx context.Context) ([]*ik8s_pool.K8sPool, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	list, err := dao.TK8sPoolList(dbCtx, nil)
	if err != nil {
		return nil, err
	}

	rst := make([]*ik8s_pool.K8sPool, 0, len(list))
	for _, one := range list {
		rst = append(rst, fromDAO(one))
	}
	return rst, nil
}

func (s *RDBK8sPoolStorager) DeletePool(ctx context.Context, name string) error {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	_, err = dao.TK8sPoolDelete(dbCtx, &dao.TK8sPoolParam{Name: &name})
	return err
}

func fromDAO(one *dao.TK8sPool) *ik8s_pool.K8sPool {
	if one == nil {
		return nil
	}

	instances := []iprovider.ProviderInstance{}
	if one.Instances != "" && one.Instances != "null" {
		if err := json.Unmarshal([]byte(one.Instances), &instances); err != nil {
			instances = []iprovider.ProviderInstance{}
		}
	}

	return &ik8s_pool.K8sPool{
		Name:       one.Name,
		Instances:  instances,
		CreateTime: one.CreatedAt.Unix(),
		UpdateTime: one.UpdatedAt.Unix(),
	}
}
