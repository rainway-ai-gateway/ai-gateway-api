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

package provider

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/txn"
)

// setupTxnTestEnv wires a real provider storager and a real RDBTxnStorager
// over a shared sqlite database. The factory mirrors production
// stateful.NewBFEDBContext: it inherits an enclosing *lib.DBContext from the
// context and begins a transaction only when lib.OpenTxn() is requested.
// This exercises the real issue #156 code path: dao statements inside an
// AtomExecute closure must run on the transaction (dc.Execer()), not on the
// connection pool.
func setupTxnTestEnv(t *testing.T) (*RDBProviderStorager, *txn.RDBTxnStorager) {
	if stateful.DefaultConfig == nil {
		stateful.DefaultConfig = &stateful.Config{}
	}

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
CREATE TABLE providers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  model_endpoint TEXT,
  models TEXT,
  api_keys TEXT,
  instance_pool TEXT NOT NULL,
  instance_source TEXT NOT NULL DEFAULT 'instance_pool',
  k8s_pool_name TEXT,
  model_protocols TEXT NOT NULL,
  protocol_paths TEXT,
  time_zone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  tiers TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);`)
	require.NoError(t, err)

	factory := func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		if dc, ok := ctx.(*lib.DBContext); ok {
			return dc, nil
		}
		dc := lib.NewDBContext(ctx, db)
		if lib.WantOpenTxn(ops...) {
			if err := dc.BeginTrans(); err != nil {
				return nil, err
			}
		}
		return dc, nil
	}

	return NewRDBProviderStorager(factory), txn.NewRDBTxnStorager(factory)
}

func createTwoModelProvider(t *testing.T, storager *RDBProviderStorager) {
	t.Helper()
	_, err := storager.CreateProvider(context.Background(), &iprovider.ProviderParam{
		Name: lib.PString("txn-provider"),
		Models: []string{
			"controlled-model-a",
			"controlled-model-b",
		},
	})
	require.NoError(t, err)
}

// TestAtomExecute_UpdateThenRefCheckFailureRollsBack is the issue #156
// anchor: a provider update followed by a failing reference-check hook
// inside one AtomExecute must leave the provider row untouched. Before the
// fix, dao statements autocommitted through the pool and the rollback
// applied to an empty transaction, so the 409-path update persisted.
func TestAtomExecute_UpdateThenRefCheckFailureRollsBack(t *testing.T) {
	storager, txnStorager := setupTxnTestEnv(t)
	createTwoModelProvider(t, storager)

	conflictErr := errors.New("Conflict: model is referenced by cluster")
	err := txnStorager.AtomExecute(context.Background(), func(ctx context.Context) error {
		existing, err := storager.FetchProvider(ctx, &iprovider.ProviderFilter{
			Name: lib.PString("txn-provider"),
		})
		if err != nil {
			return err
		}
		if existing == nil {
			return errors.New("provider not found")
		}

		// Persist the update first, then run the (failing) reference check,
		// mirroring ProviderManager.UpdateProvider's write-then-validate flow.
		if err := storager.UpdateProvider(ctx, "txn-provider", &iprovider.ProviderParam{
			Name:   lib.PString("txn-provider"),
			Models: []string{"controlled-model-a"},
		}); err != nil {
			return err
		}
		return conflictErr
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, conflictErr)

	got, err := storager.FetchProvider(context.Background(), &iprovider.ProviderFilter{
		Name: lib.PString("txn-provider"),
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"controlled-model-a", "controlled-model-b"}, got.Models,
		"409 path must roll back the provider update")
}

// TestAtomExecute_CommitPersistsUpdates is the control arm: when the closure
// succeeds, the update commits.
func TestAtomExecute_CommitPersistsUpdates(t *testing.T) {
	storager, txnStorager := setupTxnTestEnv(t)
	createTwoModelProvider(t, storager)

	err := txnStorager.AtomExecute(context.Background(), func(ctx context.Context) error {
		return storager.UpdateProvider(ctx, "txn-provider", &iprovider.ProviderParam{
			Name:   lib.PString("txn-provider"),
			Models: []string{"controlled-model-a"},
		})
	})
	require.NoError(t, err)

	got, err := storager.FetchProvider(context.Background(), &iprovider.ProviderFilter{
		Name: lib.PString("txn-provider"),
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"controlled-model-a"}, got.Models)
}

// TestAtomExecute_ReadYourWrites anchors that a write inside the transaction
// is visible to a read on the same DBContext before commit.
func TestAtomExecute_ReadYourWrites(t *testing.T) {
	storager, txnStorager := setupTxnTestEnv(t)
	createTwoModelProvider(t, storager)

	err := txnStorager.AtomExecute(context.Background(), func(ctx context.Context) error {
		if err := storager.UpdateProvider(ctx, "txn-provider", &iprovider.ProviderParam{
			Name:   lib.PString("txn-provider"),
			Models: []string{"controlled-model-a"},
		}); err != nil {
			return err
		}
		got, err := storager.FetchProvider(ctx, &iprovider.ProviderFilter{
			Name: lib.PString("txn-provider"),
		})
		if err != nil {
			return err
		}
		if got == nil || len(got.Models) != 1 || got.Models[0] != "controlled-model-a" {
			return errors.New("read-your-writes violated inside transaction")
		}
		return nil
	})
	require.NoError(t, err)
}

// TestAtomExecute_RollbackDiscardsCreate anchors that inserts inside a
// failing transaction are discarded as well.
func TestAtomExecute_RollbackDiscardsCreate(t *testing.T) {
	storager, txnStorager := setupTxnTestEnv(t)

	err := txnStorager.AtomExecute(context.Background(), func(ctx context.Context) error {
		if _, err := storager.CreateProvider(ctx, &iprovider.ProviderParam{
			Name: lib.PString("txn-provider"),
			Models: []string{
				"controlled-model-a",
				"controlled-model-b",
			},
		}); err != nil {
			return err
		}
		return errors.New("boom")
	})
	require.Error(t, err)

	got, err := storager.FetchProvider(context.Background(), &iprovider.ProviderFilter{
		Name: lib.PString("txn-provider"),
	})
	require.NoError(t, err)
	assert.Nil(t, got, "failed transaction must discard the insert")
}
