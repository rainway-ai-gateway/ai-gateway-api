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

package version_control

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

func setupVersionControlTestDB(t *testing.T) (*sql.DB, lib.DBContextFactory) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Use a single connection so the in-memory database is stable across the
	// pooled connection lifecycle.
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

	factory := func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		return lib.NewDBContext(ctx, db), nil
	}
	return db, factory
}

func exportData(topic string, sign string) *iversion_control.ExportData {
	return &iversion_control.ExportData{
		Topic:                  topic,
		DataSignWithoutVersion: sign,
	}
}

func listVersions(t *testing.T, db *sql.DB, topic string) []string {
	rows, err := db.Query("SELECT version FROM config_versions WHERE name = ? ORDER BY version", topic)
	require.NoError(t, err)
	defer rows.Close()

	versions := []string{}
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		versions = append(versions, v)
	}
	require.NoError(t, rows.Err())
	return versions
}

func TestUpsertConfigLastExportedVersion_FirstCreate(t *testing.T) {
	db, factory := setupVersionControlTestDB(t)
	vcs := NewVersionControllerStorage(factory)

	version, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.NoError(t, err)
	assert.Equal(t, iversion_control.Version(time.Now()), version)
	assert.Len(t, listVersions(t, db, "topic-a"), 1)
}

func TestUpsertConfigLastExportedVersion_SameContentShortCircuit(t *testing.T) {
	db, factory := setupVersionControlTestDB(t)
	vcs := NewVersionControllerStorage(factory)

	v1, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.NoError(t, err)

	v2, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.NoError(t, err)
	assert.Equal(t, v1, v2)
	assert.Len(t, listVersions(t, db, "topic-a"), 1, "unchanged content must not create a new version row")
}

func TestUpsertConfigLastExportedVersion_SameSecondChangesMonotonic(t *testing.T) {
	db, factory := setupVersionControlTestDB(t)
	vcs := NewVersionControllerStorage(factory)

	v1, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.NoError(t, err)

	// Issue #142: two content changes exported within the same wall-clock
	// second must still get strictly increasing versions.
	v2, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-b"))
	require.NoError(t, err)
	assert.Greater(t, v2, v1, "second change must get a newer version than the first")

	t1, err := time.ParseInLocation(versionLayout, v1, time.Local)
	require.NoError(t, err)
	t2, err := time.ParseInLocation(versionLayout, v2, time.Local)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, t2, t1.Add(time.Second), "version must advance by at least one second granularity")

	assert.Equal(t, []string{v1, v2}, listVersions(t, db, "topic-a"))
}

func TestUpsertConfigLastExportedVersion_ContentBounceStillAdvances(t *testing.T) {
	db, factory := setupVersionControlTestDB(t)
	vcs := NewVersionControllerStorage(factory)

	v1, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.NoError(t, err)
	v2, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-b"))
	require.NoError(t, err)

	// Content changes back to sign-a. The sign-a row is not the latest one,
	// so a new version is required; reusing v1 would freeze version progress.
	v3, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.NoError(t, err)
	assert.Greater(t, v3, v2)
	assert.NotEqual(t, v1, v3)
	assert.Len(t, listVersions(t, db, "topic-a"), 3)
}

func TestUpsertConfigLastExportedVersion_BumpsPastFutureLatest(t *testing.T) {
	db, factory := setupVersionControlTestDB(t)
	vcs := NewVersionControllerStorage(factory)

	// A latest version ahead of the wall clock (e.g. clock rollback, or a row
	// written by a clock-skewed peer) must not cause a duplicate or a
	// backwards version.
	_, err := db.Exec("INSERT INTO config_versions (name, data_sign, version, created_at) VALUES (?, ?, ?, ?)",
		"topic-a", "sign-old", "29990101000000", time.Now())
	require.NoError(t, err)

	version, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-new"))
	require.NoError(t, err)
	assert.Equal(t, "29990101000001", version)
	assert.Equal(t, []string{"29990101000000", "29990101000001"}, listVersions(t, db, "topic-a"))
}

func TestUpsertConfigLastExportedVersion_ConcurrentExports(t *testing.T) {
	db, factory := setupVersionControlTestDB(t)
	vcs := NewVersionControllerStorage(factory)

	// Two exports racing on an empty topic must both succeed with distinct
	// monotonic versions: the loser of the (name, version) race retries with
	// a bumped version instead of failing the export (issue #142).
	const workers = 2
	versions := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sign, err := iversion_control.Sign(map[string]int{"worker": i})
			require.NoError(t, err)
			version, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", sign))
			require.NoError(t, err)
			versions <- version
		}(i)
	}
	wg.Wait()
	close(versions)

	got := listVersions(t, db, "topic-a")
	assert.Len(t, got, workers, "each racing export must create its own version row")
	seen := map[string]struct{}{}
	for v := range versions {
		assert.NotContains(t, seen, v)
		seen[v] = struct{}{}
	}
	for i := 1; i < len(got); i++ {
		assert.Greater(t, got[i], got[i-1], "versions must be strictly increasing")
	}
}

func TestUpsertConfigLastExportedVersion_NonDuplicateErrorPassthrough(t *testing.T) {
	db, _ := setupVersionControlTestDB(t)
	db.Close()

	factory := func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		return lib.NewDBContext(ctx, db), nil
	}
	vcs := NewVersionControllerStorage(factory)

	_, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.Error(t, err)
	assert.False(t, lib.DuplicateEntryError(err))
}

func TestUpsertConfigLastExportedVersion_DBCtxFactoryError(t *testing.T) {
	factory := func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		return nil, errors.New("factory boom")
	}
	vcs := NewVersionControllerStorage(factory)

	_, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData("topic-a", "sign-a"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "factory boom")
}

func TestBumpVersion(t *testing.T) {
	base := iversion_control.Version(time.Now())
	next := iversion_control.Version(time.Now().Add(time.Second))

	assert.Equal(t, next, bumpVersion(base, base), "same-second collision must be bumped by one second")
	assert.Equal(t, next, bumpVersion(base, next), "candidate already ahead stays unchanged")
	assert.Equal(t, "29990101000001", bumpVersion("29990101000000", base))
	assert.Equal(t, base, bumpVersion("not-a-version", base), "unparseable latest keeps candidate")
}

func TestLatestConfigVersion_OrdersByVersionThenIDDesc(t *testing.T) {
	_, factory := setupVersionControlTestDB(t)

	dbCtx, err := factory(context.Background())
	require.NoError(t, err)

	old := "20200101000000"
	mid := "20200101000001"
	for _, v := range []string{old, mid} {
		_, err = dao.TConfigVersionCreate(dbCtx, &dao.TConfigVersionParam{
			Name:     lib.PString("topic-a"),
			DataSign: lib.PString("sign-" + v),
			Version:  lib.PString(v),
		})
		require.NoError(t, err)
	}
	_, err = dao.TConfigVersionCreate(dbCtx, &dao.TConfigVersionParam{
		Name:     lib.PString("topic-b"),
		DataSign: lib.PString("sign-b"),
		Version:  lib.PString("29990101000000"),
	})
	require.NoError(t, err)

	latest, err := latestConfigVersion(dbCtx, "topic-a")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, mid, latest.Version, "must return the newest version of the topic")

	latest, err = latestConfigVersion(dbCtx, "topic-none")
	require.NoError(t, err)
	assert.Nil(t, latest)
}

// setupFileVersionControlTestDB opens two handles to a file-based SQLite
// database: db feeds the storager, blocker is a second handle used to hold
// an uncommitted "winner" row so the storager's first read misses it. Both
// handles are limited to a single connection each so the busy_timeout pragma
// applies to every pooled connection.
func setupFileVersionControlTestDB(t *testing.T) (db, blocker *sql.DB, factory lib.DBContextFactory) {
	path := filepath.Join(t.TempDir(), "vc_test.db")
	open := func() *sql.DB {
		h, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(3000)")
		require.NoError(t, err)
		h.SetMaxOpenConns(1)
		_, err = h.Exec(`
CREATE TABLE IF NOT EXISTS config_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  data_sign TEXT NOT NULL,
  version TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name, version)
);`)
		require.NoError(t, err)
		t.Cleanup(func() { h.Close() })
		return h
	}
	db = open()
	blocker = open()

	oldConfig := stateful.DefaultConfig
	stateful.DefaultConfig = &stateful.Config{
		RunTime: stateful.RunTimeConfig{
			RecordSQL: false,
		},
	}
	t.Cleanup(func() { stateful.DefaultConfig = oldConfig })

	// Same pass-through semantics as stateful.NewBFEDBContext: when the
	// caller context already carries a DBContext, reuse it.
	factory = func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		if dc, ok := ctx.(*lib.DBContext); ok {
			return dc, nil
		}
		return lib.NewDBContext(ctx, db), nil
	}
	return db, blocker, factory
}

type upsertResult struct {
	version string
	err     error
}

// upsertAgainstUncommittedWinner reproduces the issue #318 retry entry
// condition on SQLite: a winner row for (topic, winnerSign, winnerVersion)
// is inserted through the blocker connection but left uncommitted, so the
// storager's first read misses it while its first insert — same (name,
// version) candidate — blocks on the write lock. Committing the blocker
// transaction afterwards makes that insert fail with a unique constraint
// error, entering the duplicate-key retry path.
func upsertAgainstUncommittedWinner(t *testing.T, blocker *sql.DB, factory lib.DBContextFactory,
	topic, winnerSign, winnerVersion, upsertSign string) upsertResult {
	tx, err := blocker.Begin()
	require.NoError(t, err)
	_, err = tx.Exec(
		"INSERT INTO config_versions (name, data_sign, version, created_at) VALUES (?, ?, ?, ?)",
		topic, winnerSign, winnerVersion, time.Now())
	require.NoError(t, err)

	result := make(chan upsertResult, 1)
	go func() {
		vcs := NewVersionControllerStorage(factory)
		version, err := vcs.UpsertConfigLastExportedVersion(context.Background(), exportData(topic, upsertSign))
		result <- upsertResult{version: version, err: err}
	}()

	// Give the goroutine time to pass its first read and reach the blocked
	// insert, then release the winner row.
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, tx.Commit())

	got := <-result
	require.NoError(t, got.err)
	return got
}

// Issue #318 / design line 421: when a concurrent export already created a
// version for the same content, the loser of the (name, version) race must
// return the winner's version instead of failing or creating a redundant
// row.
func TestUpsertConfigLastExportedVersion_RetryReturnsWinnerVersion(t *testing.T) {
	db, blocker, factory := setupFileVersionControlTestDB(t)

	for attempt := 0; attempt < 3; attempt++ {
		topic := fmt.Sprintf("topic-retry-same-%d", attempt)
		winner := iversion_control.Version(time.Now())

		got := upsertAgainstUncommittedWinner(t, blocker, factory, topic, "sign-a", winner, "sign-a")

		// A second rollover between the winner insert and the candidate
		// computation makes the insert skip the collision; retry on a fresh
		// topic instead of flaking.
		if got.version != winner {
			continue
		}
		assert.Equal(t, winner, got.version)
		assert.Equal(t, []string{winner}, listVersions(t, db, topic),
			"loser must reuse the winner version row, not create a new one")
		return
	}
	t.Fatal("no attempt hit the duplicate-key retry path")
}

// Issue #318 / design line 97: when the winner's content differs, the loser
// must bump strictly past the winner's version and succeed.
func TestUpsertConfigLastExportedVersion_RetryBumpsPastWinner(t *testing.T) {
	db, blocker, factory := setupFileVersionControlTestDB(t)

	for attempt := 0; attempt < 3; attempt++ {
		topic := fmt.Sprintf("topic-retry-diff-%d", attempt)
		winner := iversion_control.Version(time.Now())

		got := upsertAgainstUncommittedWinner(t, blocker, factory, topic, "sign-a", winner, "sign-b")

		wt, err := time.ParseInLocation(versionLayout, winner, time.Local)
		require.NoError(t, err)
		bumped := iversion_control.Version(wt.Add(time.Second))

		// Same second-rollover escape as the same-sign test: without the
		// collision the export succeeds directly at the bumped version, so
		// only an unexpected result needs a retry.
		if got.version != bumped {
			continue
		}
		assert.Equal(t, bumped, got.version)
		assert.Equal(t, []string{winner, bumped}, listVersions(t, db, topic),
			"winner row kept, loser row appended with a strictly greater version")
		return
	}
	t.Fatal("no attempt hit the duplicate-key retry path")
}
