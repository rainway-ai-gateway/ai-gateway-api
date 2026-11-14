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

package dao

import (
	"time"

	"github.com/didi/gendry/scanner"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao/internal"
)

const tProviderTableName = "providers"

// TProvider is the DAO representation of a providers row.
type TProvider struct {
	ID             int64     `db:"id"`
	Name           string    `db:"name"`
	Description    string    `db:"description"`
	ModelEndpoint  string    `db:"model_endpoint"`
	Models         string    `db:"models"`
	Keys           string    `db:"api_keys"`
	InstancePool   string    `db:"instance_pool"`
	InstanceSource string    `db:"instance_source"`
	K8sPoolName    string    `db:"k8s_pool_name"`
	ModelProtocols string    `db:"model_protocols"`
	ProtocolPaths  string    `db:"protocol_paths"`
	TimeZone       string    `db:"time_zone"`
	Tiers          string    `db:"tiers"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// TProviderOne queries one provider.
func TProviderOne(dbCtx lib.DBContexter, where *TProviderParam) (*TProvider, error) {
	t := &TProvider{}
	err := internal.QueryOne(dbCtx, tProviderTableName, where, t)
	if err == nil {
		return t, nil
	}
	if xerror.Cause(err) == internal.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

// TProviderList queries multiple providers.
func TProviderList(dbCtx lib.DBContexter, where *TProviderParam) ([]*TProvider, error) {
	t := []*TProvider{}
	err := internal.QueryList(dbCtx, tProviderTableName, where, &t)
	if err == nil {
		return t, nil
	}
	if xerror.Cause(err) == internal.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

// TProviderListAll queries all providers without pagination.
func TProviderListAll(dbCtx lib.DBContexter, where *TProviderParam) ([]*TProvider, error) {
	if where == nil {
		where = &TProviderParam{}
	}
	orderBy := "id"
	where.OrderBy = &orderBy
	t := []*TProvider{}
	err := internal.QueryList(dbCtx, tProviderTableName, where, &t)
	if err == nil {
		return t, nil
	}
	if xerror.Cause(err) == internal.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

// TProviderListWithPagination queries providers with pagination.
func TProviderListWithPagination(dbCtx lib.DBContexter, where *TProviderParam, page, pageSize int) ([]*TProvider, error) {
	whereMap := internal.Struct2Where(where)
	whereMap["_limit"] = []uint{uint((page - 1) * pageSize), uint(pageSize)}
	whereMap["_orderby"] = "id"

	build := internal.NewSelectBuilder(tProviderTableName, whereMap, nil)
	sql, args, err := build.Compile()
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	rows, err := dbCtx.Execer().QueryContext(dbCtx, sql, args...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	t := []*TProvider{}
	if err := scanner.Scan(rows, &t); err != nil {
		if err == scanner.ErrEmptyResult {
			return nil, nil
		}
		return nil, xerror.WrapDaoError(err)
	}
	return t, nil
}

// TProviderNames returns all provider names ordered by name ascending.
func TProviderNames(dbCtx lib.DBContexter) ([]string, error) {
	whereMap := map[string]interface{}{
		"_orderby": "name asc",
	}

	build := internal.NewSelectBuilder(tProviderTableName, whereMap, []string{"name"})
	sql, args, err := build.Compile()
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	rows, err := dbCtx.Execer().QueryContext(dbCtx, sql, args...)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, xerror.WrapDaoError(err)
		}
		names = append(names, name)
	}
	return names, nil
}

// TProviderCount returns the total count matched by where.
func TProviderCount(dbCtx lib.DBContexter, where *TProviderParam) (int64, error) {
	type countResult struct {
		Count int64 `db:"count(*)"`
	}
	list := []*countResult{}
	whereMap := internal.Struct2Where(where)
	build := internal.NewSelectBuilder(tProviderTableName, whereMap, []string{"count(*)"})
	sql, args, err := build.Compile()
	if err != nil {
		return 0, xerror.WrapDaoError(err)
	}

	rows, err := dbCtx.Execer().QueryContext(dbCtx, sql, args...)
	if err != nil {
		return 0, xerror.WrapDaoError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var c countResult
		if err := rows.Scan(&c.Count); err != nil {
			return 0, xerror.WrapDaoError(err)
		}
		list = append(list, &c)
	}
	if len(list) == 0 {
		return 0, nil
	}
	return list[0].Count, nil
}

// TProviderParam is used for create/update/delete/where conditions.
type TProviderParam struct {
	ID             *int64     `db:"id"`
	Name           *string    `db:"name"`
	Names          []string   `db:"name,in"`
	Description    *string    `db:"description"`
	ModelEndpoint  *string    `db:"model_endpoint"`
	Models         *string    `db:"models"`
	Keys           *string    `db:"api_keys"`
	InstancePool   *string    `db:"instance_pool"`
	InstanceSource *string    `db:"instance_source"`
	K8sPoolName    *string    `db:"k8s_pool_name"`
	ModelProtocols *string    `db:"model_protocols"`
	ProtocolPaths  *string    `db:"protocol_paths"`
	TimeZone       *string    `db:"time_zone"`
	Tiers          *string    `db:"tiers"`
	CreatedAt      *time.Time `db:"created_at"`
	UpdatedAt      *time.Time `db:"updated_at"`

	OrderBy *string `db:"_orderby"`
}

// TProviderCreate creates one or more provider records.
func TProviderCreate(dbCtx lib.DBContexter, data ...*TProviderParam) (int64, error) {
	if len(data) == 1 {
		if data[0].CreatedAt == nil {
			data[0].CreatedAt = internal.PTimeNow()
		}
		if data[0].UpdatedAt == nil {
			data[0].UpdatedAt = data[0].CreatedAt
		}
		return internal.Create(dbCtx, tProviderTableName, data[0])
	}

	list := make([]interface{}, len(data))
	for i, one := range data {
		if one.CreatedAt == nil {
			one.CreatedAt = internal.PTimeNow()
		}
		if one.UpdatedAt == nil {
			one.UpdatedAt = one.CreatedAt
		}
		list[i] = one
	}
	return internal.Create(dbCtx, tProviderTableName, list...)
}

// TProviderUpdate updates provider records.
func TProviderUpdate(dbCtx lib.DBContexter, val, where *TProviderParam) (int64, error) {
	val.UpdatedAt = lib.PTimeNow()
	return internal.Update(dbCtx, tProviderTableName, where, val)
}

// TProviderDelete deletes provider records matched by where.
func TProviderDelete(dbCtx lib.DBContexter, where *TProviderParam) (int64, error) {
	return internal.Delete(dbCtx, tProviderTableName, where)
}
