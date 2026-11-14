//Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
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

package ai_context

import (
	"context"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xlock"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ai_context"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

// replaceLockName guards the whole-collection replacement of this resource.
const replaceLockName = "ai_gateway:replace:ai_context_rules"

// replaceLockTimeout bounds how long a concurrent replacement waits for the
// current one before the request is told to retry (409).
const replaceLockTimeout = 10 * time.Second

type AIContextStorager struct {
	dbCtxFactory lib.DBContextFactory
}

func NewAIContextStorager(dbCtxFactory lib.DBContextFactory) *AIContextStorager {
	return &AIContextStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

var _ ai_context.AIContextStorager = &AIContextStorager{}

// FetchAll returns all AI context rules ordered by id ascending.
func (s *AIContextStorager) FetchAll(ctx context.Context) ([]*ai_context.ContextRuleRow, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	where := &dao.TAIContextRuleParam{
		OrderBy: lib.PString("id ASC"),
	}
	list, err := dao.TAIContextRuleList(dbCtx, where)
	if err != nil {
		return nil, err
	}

	rst := make([]*ai_context.ContextRuleRow, 0, len(list))
	for _, one := range list {
		rst = append(rst, aiContextRuleDataToParam(one))
	}

	return rst, nil
}

// ReplaceAll rebuilds the whole rule set (delete-all + insert-all in the
// given order). It must be called inside a transaction (itxn.TxnStorager).
func (s *AIContextStorager) ReplaceAll(ctx context.Context, rules []*ai_context.ContextRuleRow) error {
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

	data := make([]*dao.TAIContextRuleParam, 0, len(rules))
	for _, rule := range rules {
		data = append(data, aiContextRuleParamToData(rule))
	}

	_, err = dao.TAIContextRuleReplaceAll(dbCtx, data...)
	return err
}

func aiContextRuleParamToData(param *ai_context.ContextRuleRow) *dao.TAIContextRuleParam {
	if param == nil {
		return nil
	}

	return &dao.TAIContextRuleParam{
		ID:               param.ID,
		Cond:             param.Cond,
		Mode:             param.Mode,
		MaxContextTokens: param.MaxContextTokens,
		ReserveTokens:    param.ReserveTokens,
		CreatedAt:        param.CreatedAt,
		UpdatedAt:        param.UpdatedAt,
	}
}

func aiContextRuleDataToParam(one *dao.TAIContextRule) *ai_context.ContextRuleRow {
	if one == nil {
		return nil
	}

	return &ai_context.ContextRuleRow{
		ID:               &one.ID,
		Cond:             &one.Cond,
		Mode:             &one.Mode,
		MaxContextTokens: &one.MaxContextTokens,
		ReserveTokens:    &one.ReserveTokens,
		CreatedAt:        &one.CreatedAt,
		UpdatedAt:        &one.UpdatedAt,
	}
}
