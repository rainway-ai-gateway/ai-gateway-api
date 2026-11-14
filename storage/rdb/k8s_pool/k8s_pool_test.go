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
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorager(t *testing.T) *RDBK8sPoolStorager {
	if stateful.DefaultConfig == nil {
		stateful.DefaultConfig = &stateful.Config{}
	}

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
CREATE TABLE k8s_pools (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  instances TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
)`)
	require.NoError(t, err)

	factory := lib.DBContextFactory(func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		return lib.NewDBContext(ctx, db), nil
	})

	return NewRDBK8sPoolStorager(factory)
}

func TestFetchK8sPoolInstances(t *testing.T) {
	ctx := context.Background()

	t.Run("existing pool returns its current instances (realtime)", func(t *testing.T) {
		s := setupTestStorager(t)
		require.NoError(t, s.UpsertPool(ctx, "svc-a", []iprovider.ProviderInstance{
			{Addr: "10.0.0.1", Port: 8000, Weight: 100},
		}))

		inst, err := s.FetchK8sPoolInstances(ctx, "svc-a")
		require.NoError(t, err)
		require.Len(t, inst, 1)
		assert.Equal(t, "10.0.0.1", inst[0].Addr)

		// A later upsert is reflected immediately (no caching, no mirror).
		require.NoError(t, s.UpsertPool(ctx, "svc-a", []iprovider.ProviderInstance{
			{Addr: "10.0.0.2", Port: 8000, Weight: 100},
		}))
		inst, err = s.FetchK8sPoolInstances(ctx, "svc-a")
		require.NoError(t, err)
		require.Len(t, inst, 1)
		assert.Equal(t, "10.0.0.2", inst[0].Addr)
	})

	t.Run("missing pool yields a non-nil empty list", func(t *testing.T) {
		s := setupTestStorager(t)
		inst, err := s.FetchK8sPoolInstances(ctx, "ghost")
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Len(t, inst, 0)
	})

	t.Run("empty pool yields a non-nil empty list", func(t *testing.T) {
		s := setupTestStorager(t)
		require.NoError(t, s.UpsertPool(ctx, "svc-a", nil))
		inst, err := s.FetchK8sPoolInstances(ctx, "svc-a")
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Len(t, inst, 0)
	})
}
