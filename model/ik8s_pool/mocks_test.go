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

package ik8s_pool

import (
	"context"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
)

type fakeTxn struct{}

func (f *fakeTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

type fakeK8sPoolStorager struct {
	upsertFn    func(ctx context.Context, name string, instances []iprovider.ProviderInstance) error
	fetchFn     func(ctx context.Context, name string) (*K8sPool, error)
	fetchListFn func(ctx context.Context) ([]*K8sPool, error)
	deleteFn    func(ctx context.Context, name string) error
	upserted    []string
	deleted     []string
}

func (s *fakeK8sPoolStorager) FetchK8sPoolInstances(ctx context.Context, name string) ([]iprovider.ProviderInstance, error) {
	pool, err := s.FetchPool(ctx, name)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		return []iprovider.ProviderInstance{}, nil
	}
	return pool.Instances, nil
}

func (s *fakeK8sPoolStorager) UpsertPool(ctx context.Context, name string, instances []iprovider.ProviderInstance) error {
	s.upserted = append(s.upserted, name)
	if s.upsertFn != nil {
		return s.upsertFn(ctx, name, instances)
	}
	return nil
}

func (s *fakeK8sPoolStorager) FetchPool(ctx context.Context, name string) (*K8sPool, error) {
	if s.fetchFn != nil {
		return s.fetchFn(ctx, name)
	}
	return nil, nil
}

func (s *fakeK8sPoolStorager) FetchPoolList(ctx context.Context) ([]*K8sPool, error) {
	if s.fetchListFn != nil {
		return s.fetchListFn(ctx)
	}
	return nil, nil
}

func (s *fakeK8sPoolStorager) DeletePool(ctx context.Context, name string) error {
	s.deleted = append(s.deleted, name)
	if s.deleteFn != nil {
		return s.deleteFn(ctx, name)
	}
	return nil
}

type fakeProviderStorager struct {
	fetchListFn func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error)
	updated     map[string]*iprovider.ProviderParam
}

func (s *fakeProviderStorager) CreateProvider(ctx context.Context, param *iprovider.ProviderParam) (int64, error) {
	return 1, nil
}

func (s *fakeProviderStorager) UpdateProvider(ctx context.Context, name string, param *iprovider.ProviderParam) error {
	if s.updated == nil {
		s.updated = map[string]*iprovider.ProviderParam{}
	}
	s.updated[name] = param
	return nil
}

func (s *fakeProviderStorager) DeleteProvider(ctx context.Context, name string) error {
	return nil
}

func (s *fakeProviderStorager) FetchProvider(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
	return nil, nil
}

func (s *fakeProviderStorager) FetchProviderList(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
	if s.fetchListFn != nil {
		return s.fetchListFn(ctx, filter)
	}
	return nil, 0, nil
}

func (s *fakeProviderStorager) FetchProviderNames(ctx context.Context) ([]string, error) {
	return nil, nil
}
