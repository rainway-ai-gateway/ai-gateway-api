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

package traffic_mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xlock"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/traffic_mirror"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

// replaceLockName guards the whole-collection replacement of this resource.
const replaceLockName = "ai_gateway:replace:traffic_mirror_rules"

// replaceLockTimeout bounds how long a concurrent replacement waits for the
// current one before the request is told to retry (409).
const replaceLockTimeout = 10 * time.Second

type TrafficMirrorStorager struct {
	dbCtxFactory lib.DBContextFactory
}

func NewTrafficMirrorStorager(dbCtxFactory lib.DBContextFactory) *TrafficMirrorStorager {
	return &TrafficMirrorStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

var _ traffic_mirror.TrafficMirrorStorager = &TrafficMirrorStorager{}

// FetchAll returns all traffic mirror rules ordered by id ascending.
func (s *TrafficMirrorStorager) FetchAll(ctx context.Context) ([]*traffic_mirror.TrafficMirrorRuleParam, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	where := &dao.TTrafficMirrorRuleParam{
		OrderBy: lib.PString("id ASC"),
	}
	list, err := dao.TTrafficMirrorRuleList(dbCtx, where)
	if err != nil {
		return nil, err
	}

	rst := make([]*traffic_mirror.TrafficMirrorRuleParam, 0, len(list))
	for _, one := range list {
		param, err := trafficMirrorRuleDataToParam(one)
		if err != nil {
			return nil, err
		}
		rst = append(rst, param)
	}

	return rst, nil
}

// ReplaceAll rebuilds the whole rule set (delete-all + insert-all in the given
// order). It must be called inside a transaction (itxn.TxnStorager).
func (s *TrafficMirrorStorager) ReplaceAll(ctx context.Context, rules []*traffic_mirror.TrafficMirrorRuleParam) error {
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

	data := make([]*dao.TTrafficMirrorRuleParam, 0, len(rules))
	for _, rule := range rules {
		one, err := trafficMirrorRuleParamToData(rule)
		if err != nil {
			return err
		}
		data = append(data, one)
	}

	_, err = dao.TTrafficMirrorRuleReplaceAll(dbCtx, data...)
	return err
}

// trafficMirrorRuleParamToData converts a storage-level rule into the DAO row.
// The complex columns are serialized to JSON text; a nil optional field stays
// nil so the column keeps its NULL state (NULL remove_headers is a distinct
// state from an explicit '[]').
func trafficMirrorRuleParamToData(param *traffic_mirror.TrafficMirrorRuleParam) (*dao.TTrafficMirrorRuleParam, error) {
	if param == nil {
		return nil, nil
	}

	data := &dao.TTrafficMirrorRuleParam{
		ID:            param.ID,
		Name:          param.Name,
		Cond:          param.Cond,
		MirrorCluster: param.MirrorCluster,
		Percentage:    param.Percentage,
		PathRewrite:   param.PathRewrite,
		CreatedAt:     param.CreatedAt,
		UpdatedAt:     param.UpdatedAt,
	}

	if param.RemoveHeaders != nil {
		bs, err := json.Marshal(*param.RemoveHeaders)
		if err != nil {
			return nil, fmt.Errorf("marshal traffic mirror rule remove_headers error: %s", err.Error())
		}
		data.RemoveHeaders = lib.PString(string(bs))
	}

	if param.SetHeaders != nil {
		bs, err := json.Marshal(param.SetHeaders)
		if err != nil {
			return nil, fmt.Errorf("marshal traffic mirror rule set_headers error: %s", err.Error())
		}
		data.SetHeaders = lib.PString(string(bs))
	}

	if len(param.BodyRewrites) > 0 {
		bs, err := json.Marshal(param.BodyRewrites)
		if err != nil {
			return nil, fmt.Errorf("marshal traffic mirror rule body_rewrites error: %s", err.Error())
		}
		data.BodyRewrites = lib.PString(string(bs))
	}

	return data, nil
}

// trafficMirrorRuleDataToParam converts a DAO row into the storage-level rule,
// deserializing the JSON text columns. A NULL column stays a nil field.
func trafficMirrorRuleDataToParam(one *dao.TTrafficMirrorRule) (*traffic_mirror.TrafficMirrorRuleParam, error) {
	if one == nil {
		return nil, nil
	}

	param := &traffic_mirror.TrafficMirrorRuleParam{
		ID:            &one.ID,
		Name:          &one.Name,
		Cond:          &one.Cond,
		MirrorCluster: &one.MirrorCluster,
		Percentage:    &one.Percentage,
		PathRewrite:   one.PathRewrite,
		CreatedAt:     &one.CreatedAt,
		UpdatedAt:     &one.UpdatedAt,
	}

	if one.RemoveHeaders != nil {
		var headers []string
		if err := json.Unmarshal([]byte(*one.RemoveHeaders), &headers); err != nil {
			return nil, fmt.Errorf("unmarshal traffic mirror rule remove_headers error: %s", err.Error())
		}
		param.RemoveHeaders = &headers
	}

	if one.SetHeaders != nil {
		headers := map[string]string{}
		if err := json.Unmarshal([]byte(*one.SetHeaders), &headers); err != nil {
			return nil, fmt.Errorf("unmarshal traffic mirror rule set_headers error: %s", err.Error())
		}
		param.SetHeaders = headers
	}

	if one.BodyRewrites != nil {
		var rewrites []*traffic_mirror.TrafficMirrorBodyRewriteParam
		if err := json.Unmarshal([]byte(*one.BodyRewrites), &rewrites); err != nil {
			return nil, fmt.Errorf("unmarshal traffic mirror rule body_rewrites error: %s", err.Error())
		}
		param.BodyRewrites = rewrites
	}

	return param, nil
}
