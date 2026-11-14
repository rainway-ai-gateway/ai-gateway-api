// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
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
	"context"
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

func setupConfigVersionsTestDB(t *testing.T) (*sql.DB, lib.DBContexter) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	oldConfig := stateful.DefaultConfig
	stateful.DefaultConfig = &stateful.Config{
		RunTime: stateful.RunTimeConfig{
			RecordSQL: false,
		},
	}
	t.Cleanup(func() { stateful.DefaultConfig = oldConfig })

	_, err = db.Exec(`
CREATE TABLE config_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  data_sign TEXT NOT NULL,
  version TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name, version)
);`)
	require.NoError(t, err)

	return db, lib.NewDBContext(context.Background(), db)
}

// TConfigVersionOneForUpdate degrades to a plain query on non-MySQL drivers:
// SQLite does not support FOR UPDATE syntax, so the raw locking query must
// only be generated on the MySQL branch (issue #318).
func TestTConfigVersionOneForUpdate_NonMySQLFallback(t *testing.T) {
	_, dbCtx := setupConfigVersionsTestDB(t)

	for _, v := range []string{"20200101000000", "20200101000001"} {
		_, err := TConfigVersionCreate(dbCtx, &TConfigVersionParam{
			Name:     lib.PString("topic-a"),
			DataSign: lib.PString("sign-" + v),
			Version:  lib.PString(v),
		})
		require.NoError(t, err)
	}

	row, err := TConfigVersionOneForUpdate(dbCtx, "topic-a", "20200101000001")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "20200101000001", row.Version)
	assert.Equal(t, "sign-20200101000001", row.DataSign)

	// Unknown (name, version) returns (nil, nil), same as TConfigVersionOne.
	none, err := TConfigVersionOneForUpdate(dbCtx, "topic-a", "20200101000002")
	require.NoError(t, err)
	assert.Nil(t, none)
}
