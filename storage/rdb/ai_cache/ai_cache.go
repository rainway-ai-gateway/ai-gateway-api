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

package ai_cache

import (
	"context"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xlock"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ai_cache"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

// replaceLockName guards the whole-collection replacement of this resource.
const replaceLockName = "ai_gateway:replace:ai_cache_rules"

// replaceLockTimeout bounds how long a concurrent replacement waits for the
// current one. Beyond it the request is told to retry (409) instead of piling
// up on row locks.
const replaceLockTimeout = 10 * time.Second

type AICacheStorager struct {
	dbCtxFactory lib.DBContextFactory
}

func NewAICacheStorager(dbCtxFactory lib.DBContextFactory) *AICacheStorager {
	return &AICacheStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

var _ ai_cache.AICacheStorager = &AICacheStorager{}

// FetchAll returns all AI cache rules ordered by id ascending.
func (s *AICacheStorager) FetchAll(ctx context.Context) ([]*ai_cache.AICacheRuleParam, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	where := &dao.TAICacheRuleParam{
		OrderBy: lib.PString("id ASC"),
	}
	list, err := dao.TAICacheRuleList(dbCtx, where)
	if err != nil {
		return nil, err
	}

	rst := make([]*ai_cache.AICacheRuleParam, 0, len(list))
	for _, one := range list {
		rst = append(rst, aiCacheRuleDataToParam(one))
	}

	return rst, nil
}

// ReplaceAll rebuilds the whole rule set (delete-all + insert-all in the
// given order). It must be called inside a transaction (itxn.TxnStorager).
func (s *AICacheStorager) ReplaceAll(ctx context.Context, rules []*ai_cache.AICacheRuleParam) error {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	// delete-all + insert-all takes next-key locks on the whole table; concurrent
	// replacements deadlock under InnoDB REPEATABLE READ (issue #295). Serialize
	// per collection across api replicas; SQLite is a no-op.
	release, err := xlock.Acquire(ctx, dbCtx.Conn(), replaceLockName, replaceLockTimeout)
	if err != nil {
		return err
	}
	defer release()

	data := make([]*dao.TAICacheRuleParam, 0, len(rules))
	for _, rule := range rules {
		data = append(data, aiCacheRuleParamToData(rule))
	}

	_, err = dao.TAICacheRuleReplaceAll(dbCtx, data...)
	return err
}

func aiCacheRuleParamToData(param *ai_cache.AICacheRuleParam) *dao.TAICacheRuleParam {
	if param == nil {
		return nil
	}

	return &dao.TAICacheRuleParam{
		ID:                  param.ID,
		Name:                param.Name,
		Cond:                param.Cond,
		CacheKeyStrategy:    param.CacheKeyStrategy,
		CacheTTL:            param.CacheTTL,
		MaxBodyBytes:        param.MaxBodyBytes,
		MaxValueBytes:       param.MaxValueBytes,
		EnableSemanticCache: param.EnableSemanticCache,
	}
}

func aiCacheRuleDataToParam(one *dao.TAICacheRule) *ai_cache.AICacheRuleParam {
	if one == nil {
		return nil
	}

	return &ai_cache.AICacheRuleParam{
		ID:                  &one.ID,
		Name:                &one.Name,
		Cond:                &one.Cond,
		CacheKeyStrategy:    &one.CacheKeyStrategy,
		CacheTTL:            &one.CacheTTL,
		MaxBodyBytes:        &one.MaxBodyBytes,
		MaxValueBytes:       &one.MaxValueBytes,
		EnableSemanticCache: &one.EnableSemanticCache,
	}
}
