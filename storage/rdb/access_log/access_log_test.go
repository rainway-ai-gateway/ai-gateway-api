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

package access_log

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorager(t *testing.T) (*AccessLogConfStorager, *sql.DB) {
	// The DAO layer consults stateful.DefaultConfig when recording SQL.
	if stateful.DefaultConfig == nil {
		stateful.DefaultConfig = &stateful.Config{}
	}

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	// Same shape as db_ddl_sqlite.sql.
	_, err = db.Exec(`
CREATE TABLE access_log_conf (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  req_headers TEXT NOT NULL,
  res_headers TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER access_log_conf_updated_at AFTER UPDATE ON access_log_conf
  FOR EACH ROW BEGIN UPDATE access_log_conf SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;
`)
	require.NoError(t, err)

	factory := lib.DBContextFactory(func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		return lib.NewDBContext(ctx, db), nil
	})
	return NewAccessLogConfStorager(factory), db
}

func TestFetchUnpublished(t *testing.T) {
	storager, _ := setupTestStorager(t)

	config, err := storager.Fetch(context.Background())
	require.NoError(t, err)
	assert.Nil(t, config)
}

func TestUpsertOverwriteSingleton(t *testing.T) {
	storager, db := setupTestStorager(t)
	ctx := context.Background()

	first := &access_log.AccessLogConf{
		Id:         access_log.AccessLogConfFixedID,
		ReqHeaders: `["Business_type"]`,
		ResHeaders: `[]`,
	}
	require.NoError(t, storager.Upsert(ctx, first))

	second := &access_log.AccessLogConf{
		Id:         access_log.AccessLogConfFixedID,
		ReqHeaders: `["X-Request-Id"]`,
		ResHeaders: `["X-Trace-Id"]`,
	}
	require.NoError(t, storager.Upsert(ctx, second))

	config, err := storager.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, `["X-Request-Id"]`, config.ReqHeaders)
	assert.Equal(t, `["X-Trace-Id"]`, config.ResHeaders)

	// still a single row
	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM access_log_conf").Scan(&count))
	assert.Equal(t, 1, count)
}

func TestUpsertThenReadJSONRoundTrip(t *testing.T) {
	storager, _ := setupTestStorager(t)
	ctx := context.Background()

	require.NoError(t, storager.Upsert(ctx, &access_log.AccessLogConf{
		Id:         access_log.AccessLogConfFixedID,
		ReqHeaders: `["Business_type","X-Request-Id"]`,
		ResHeaders: `["X-Trace-Id"]`,
	}))

	config, err := storager.Fetch(ctx)
	require.NoError(t, err)
	require.NotNil(t, config)

	var req, res []string
	require.NoError(t, json.Unmarshal([]byte(config.ReqHeaders), &req))
	require.NoError(t, json.Unmarshal([]byte(config.ResHeaders), &res))
	assert.Equal(t, []string{"Business_type", "X-Request-Id"}, req)
	assert.Equal(t, []string{"X-Trace-Id"}, res)
}
