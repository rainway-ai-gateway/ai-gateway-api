// Copyright (c) 2021 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"database/sql"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao/internal"
)

const tConfigVersionTableName = "config_versions"

// IsMySQL reports whether the context's connection uses the MySQL driver.
// Lock lifetimes and supported syntax differ per driver (issue #318), so
// storage-layer concurrency handling branches on it.
func IsMySQL(dbCtx lib.DBContexter) bool {
	return isMySQL(dbCtx.Conn())
}

// TConfigVersion Query Result
type TConfigVersion struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	DataSign  string    `db:"data_sign"`
	Version   string    `db:"version"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// TConfigVersionOne Query One
// return (nil, nil) if record not existed
func TConfigVersionOne(dbCtx lib.DBContexter, where *TConfigVersionParam) (*TConfigVersion, error) {
	t := &TConfigVersion{}
	err := internal.QueryOne(dbCtx, tConfigVersionTableName, where, t)
	if err == nil {
		return t, nil
	}
	if xerror.Cause(err) == internal.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

// TConfigVersionOneForUpdate pins the exact (name, version) row with a
// current read. MySQL uses SELECT ... FOR UPDATE with equality on the
// unique key uk_name_version: a record lock only (no gap locks), and it
// re-evaluates against the latest committed data, so the duplicate-key
// retry path inside a REPEATABLE READ transaction sees the concurrent
// winner's row instead of its stale snapshot (issue #318). Callers must
// release the failed INSERT's shared locks first (ROLLBACK TO SAVEPOINT),
// otherwise the upgrade to this exclusive lock deadlocks concurrent losers
// against each other. Non-MySQL drivers (e.g. SQLite, which does not
// support FOR UPDATE syntax) fall back to a plain query.
func TConfigVersionOneForUpdate(dbCtx lib.DBContexter, name, version string) (*TConfigVersion, error) {
	if !isMySQL(dbCtx.Conn()) {
		return TConfigVersionOne(dbCtx, &TConfigVersionParam{
			Name:    &name,
			Version: &version,
		})
	}

	row := dbCtx.Execer().QueryRowContext(dbCtx,
		"SELECT id, name, data_sign, version, created_at, updated_at "+
			"FROM config_versions WHERE name = ? AND version = ? FOR UPDATE",
		name, version)

	t := &TConfigVersion{}
	err := row.Scan(&t.ID, &t.Name, &t.DataSign, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}
	return t, nil
}

// TConfigVersionList Query Multiple
func TConfigVersionList(dbCtx lib.DBContexter, where *TConfigVersionParam) ([]*TConfigVersion, error) {
	t := []*TConfigVersion{}
	err := internal.QueryList(dbCtx, tConfigVersionTableName, where, &t)
	if err == nil {
		return t, nil
	}
	if xerror.Cause(err) == internal.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

// TConfigVersionParamCreate/Update/Where Data Carrier
// See: https://github.com/didi/gendry/blob/master/builder/README.md
type TConfigVersionParam struct {
	// IDs              []int64     `db:"id,in"`

	ID        *int64     `db:"id"`
	Name      *string    `db:"name"`
	DataSign  *string    `db:"data_sign"`
	Version   *string    `db:"version"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	OrderBy *string `db:"_orderby"`
}

// TConfigVersionCreate One/Multiple
func TConfigVersionCreate(dbCtx lib.DBContexter, data ...*TConfigVersionParam) (int64, error) {
	if len(data) == 1 {
		if data[0].CreatedAt == nil {
			data[0].CreatedAt = internal.PTimeNow()
		}
		return internal.Create(dbCtx, tConfigVersionTableName, data[0])
	}

	list := make([]interface{}, len(data))
	for i, one := range data {
		if one.CreatedAt == nil {
			one.CreatedAt = internal.PTimeNow()
		}
		list[i] = one
	}

	return internal.Create(dbCtx, tConfigVersionTableName, list...)
}

// TConfigVersionUpdate Update One
func TConfigVersionUpdate(dbCtx lib.DBContexter, val, where *TConfigVersionParam) (int64, error) {
	return internal.Update(dbCtx, tConfigVersionTableName, where, val)
}

// TConfigVersionDelete Delete One/Multiple
func TConfigVersionDelete(dbCtx lib.DBContexter, where *TConfigVersionParam) (int64, error) {
	return internal.Delete(dbCtx, tConfigVersionTableName, where)
}
