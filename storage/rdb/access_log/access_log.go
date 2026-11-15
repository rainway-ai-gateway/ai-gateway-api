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

package access_log

import (
	"context"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

// AccessLogConfStorager is the RDB implementation of the access log config
// singleton storage.
type AccessLogConfStorager struct {
	dbCtxFactory lib.DBContextFactory
}

func NewAccessLogConfStorager(dbCtxFactory lib.DBContextFactory) *AccessLogConfStorager {
	return &AccessLogConfStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

var _ access_log.AccessLogConfStorager = &AccessLogConfStorager{}

// Fetch reads the singleton row; returns nil, nil when never configured.
func (s *AccessLogConfStorager) Fetch(ctx context.Context) (*access_log.AccessLogConf, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	one, err := dao.TAccessLogConfOne(dbCtx, &dao.TAccessLogConfParam{
		Id: lib.PInt64(access_log.AccessLogConfFixedID),
	})
	if err != nil {
		return nil, err
	}
	if one == nil {
		return nil, nil
	}

	return accessLogConfDataToModel(one), nil
}

// Upsert overwrites the singleton row (fixed id=1: insert when absent,
// overwrite when present). It must be called inside a transaction
// (itxn.TxnStorager).
func (s *AccessLogConfStorager) Upsert(ctx context.Context, config *access_log.AccessLogConf) error {
	if config == nil {
		return nil
	}

	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	return dao.TAccessLogConfUpsert(dbCtx, accessLogConfModelToData(config))
}

func accessLogConfModelToData(config *access_log.AccessLogConf) *dao.TAccessLogConfParam {
	if config == nil {
		return nil
	}

	return &dao.TAccessLogConfParam{
		Id:         lib.PInt64(config.Id),
		ReqHeaders: lib.PString(config.ReqHeaders),
		ResHeaders: lib.PString(config.ResHeaders),
		CreatedAt:  &config.CreatedAt,
		UpdatedAt:  &config.UpdatedAt,
	}
}

func accessLogConfDataToModel(one *dao.TAccessLogConf) *access_log.AccessLogConf {
	if one == nil {
		return nil
	}

	return &access_log.AccessLogConf{
		Id:         one.Id,
		ReqHeaders: one.ReqHeaders,
		ResHeaders: one.ResHeaders,
		CreatedAt:  one.CreatedAt,
		UpdatedAt:  one.UpdatedAt,
	}
}
