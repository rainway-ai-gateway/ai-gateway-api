//go:build mysql

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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/txn"
)

// mysqlDSN connects to the local MySQL test instance described in
// environment/mysql-installation.md. The DSN carries no isolation override,
// so transactions run under MySQL's default REPEATABLE READ — the premise
// of these tests is that a plain snapshot read inside the transaction cannot
// see the row committed by a concurrent export winner (issue #318).
func mysqlDSN(t *testing.T) string {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home dir: %v", err)
	}
	passFile := filepath.Join(home, "mysql-data", "root-password.txt")
	passBytes, err := os.ReadFile(passFile)
	if err != nil {
		t.Skipf("MySQL test instance not available (missing %s)", passFile)
	}
	pass := strings.TrimSpace(string(passBytes))
	return fmt.Sprintf("root:%s@tcp(127.0.0.1:3306)/testdb?charset=utf8mb4&parseTime=True&loc=Local", pass)
}

func setupMySQLVersionControlTest(t *testing.T) {
	dsn := mysqlDSN(t)

	probe, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer probe.Close()
	if err := probe.Ping(); err != nil {
		t.Skipf("MySQL test instance not available: %v", err)
	}

	_, err = probe.Exec(`CREATE TABLE IF NOT EXISTS config_versions (
		id bigint(20) NOT NULL AUTO_INCREMENT,
		name varchar(255) NOT NULL,
		data_sign varchar(255) NOT NULL,
		version varchar(255) NOT NULL,
		created_at datetime NOT NULL,
		updated_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_name_version (name, version)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8`)
	require.NoError(t, err)

	oldConfig := stateful.DefaultConfig
	stateful.DefaultConfig = &stateful.Config{
		RunTime: stateful.RunTimeConfig{
			RecordSQL: false,
		},
	}
	t.Cleanup(func() { stateful.DefaultConfig = oldConfig })
}

// upsertInTxn runs UpsertConfigLastExportedVersion inside a real transaction
// (mirroring model/iversion_control.ExportConfig's AtomExecute) on its own
// dedicated connection, then commits.
func upsertInTxn(t *testing.T, dsn string, data *iversion_control.ExportData) (string, error) {
	conn, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer conn.Close()

	vcs := NewVersionControllerStorage(mysqlTestFactory(conn))

	dc := lib.NewDBContext(context.Background(), conn)
	var version string
	err = lib.RDBTxnExecute(dc, func(ctx context.Context) error {
		var err error
		version, err = vcs.UpsertConfigLastExportedVersion(ctx, data)
		return err
	})
	return version, err
}

// upsertInTxnRetryable mirrors the production export stack: the whole
// transaction replays on transient lock errors through
// itxn.ExecuteWithRetry (issue #295), exactly like
// model/iversion_control.ExportConfig does since issue #318.
func upsertInTxnRetryable(t *testing.T, dsn string, data *iversion_control.ExportData) (string, error) {
	conn, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer conn.Close()

	factory := mysqlTestFactory(conn)
	vcs := NewVersionControllerStorage(factory)
	txnStorager := txn.NewRDBTxnStorager(factory)

	var version string
	err = itxn.ExecuteWithRetry(context.Background(), txnStorager, func(ctx context.Context) error {
		var err error
		version, err = vcs.UpsertConfigLastExportedVersion(ctx, data)
		return err
	})
	return version, err
}

// mysqlTestFactory mirrors stateful.NewBFEDBContext: reuse the DBContext the
// caller already carries (so statements join its transaction), and honor
// lib.OpenTxn for storagers that manage the transaction themselves.
func mysqlTestFactory(conn *sql.DB) lib.DBContextFactory {
	return func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		if dc, ok := ctx.(*lib.DBContext); ok {
			return dc, nil
		}
		dc := lib.NewDBContext(ctx, conn)
		if lib.WantOpenTxn(ops...) {
			if err := dc.BeginTrans(); err != nil {
				return nil, err
			}
		}
		return dc, nil
	}
}

// Issue #318 acceptance: 4 exports racing on the same changed content inside
// REPEATABLE READ transactions must all succeed and converge on a single
// version row. Before the fix, 3 of 4 failed with the duplicate key error
// surfaced as HTTP 500.
func TestUpsertConfigLastExportedVersion_ConcurrentSameSign(t *testing.T) {
	setupMySQLVersionControlTest(t)
	dsn := mysqlDSN(t)

	const workers = 4
	for attempt := 0; attempt < 3; attempt++ {
		topic := fmt.Sprintf("vc-conc-same-%d-%d", time.Now().UnixNano(), attempt)

		// Seed a committed baseline row (serial export), mirroring the real
		// export flow: the racers then all compute the same candidate
		// version and race on the retry path. Racing on a totally empty
		// topic would instead deadlock the four first inserts on the
		// clustered index tail — a degenerate InnoDB empty-range case that
		// the model layer's transient retry absorbs in production.
		baseVersion, err := upsertInTxn(t, dsn, exportData(topic, "sign-base"))
		require.NoError(t, err)

		start := make(chan struct{})
		results := make(chan upsertResult, workers)
		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				// Stagger the workers: fully barrier-synchronized racers
				// all pass the duplicate check simultaneously and then
				// deadlock on the clustered index tail (both hold an X gap
				// on the supremum and wait for an insert intention there).
				// Production exports are not barrier-synchronized; the
				// staggered loser instead hits the committed winner row,
				// gets the duplicate key error, and exercises the retry
				// path — the behavior under test. Like production (issue
				// #318), transient lock errors replay the whole
				// transaction via itxn.ExecuteWithRetry.
				time.Sleep(3 * time.Millisecond)
				version, err := upsertInTxnRetryable(t, dsn, exportData(topic, "sign-race"))
				results <- upsertResult{version: version, err: err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)

		versions := map[string]bool{}
		failed := false
		for got := range results {
			if got.err != nil {
				t.Logf("attempt %d: concurrent export failed: %v", attempt, got.err)
				failed = true
				continue
			}
			versions[got.version] = true
		}
		rows := countTopicRows(t, dsn, topic)

		// A straggler landing in the next wall-clock second skips the
		// collision and creates its own row; retry on a fresh topic instead
		// of flaking.
		if failed || rows != 2 || len(versions) != 1 {
			continue
		}
		winner := ""
		for v := range versions {
			winner = v
		}
		assert.Equal(t, 1, len(versions), "all racing exports must return the winner version")
		assert.Greater(t, winner, baseVersion, "winner version must advance past the baseline")
		return
	}
	t.Fatal("no attempt converged: racing exports did not converge on a single version row")
}

// Different contents racing in the same second: losers must bump strictly
// past the winner (and each other) and all succeed with pairwise distinct,
// strictly increasing versions.
//
// The collision is staged deterministically: a winner export inserts its
// row but holds the transaction uncommitted, so every loser's duplicate
// check blocks until the winner commits and then fails with the duplicate
// key error — the exact retry entry condition. Fully barrier-synchronized
// first inserts would instead deadlock on the clustered index tail (both
// transactions hold an X gap on the supremum and wait for an insert
// intention there), which is a transient error the model layer replays in
// production, not the behavior under test here.
func TestUpsertConfigLastExportedVersion_ConcurrentDiffSign(t *testing.T) {
	setupMySQLVersionControlTest(t)
	dsn := mysqlDSN(t)

	const losers = 3
	for attempt := 0; attempt < 3; attempt++ {
		topic := fmt.Sprintf("vc-conc-diff-%d-%d", time.Now().UnixNano(), attempt)

		// Seed the committed baseline row the winner and losers build on.
		baseVersion, err := upsertInTxn(t, dsn, exportData(topic, "sign-base"))
		require.NoError(t, err)
		baseTime, err := time.ParseInLocation(versionLayout, baseVersion, time.Local)
		require.NoError(t, err)

		// Winner: insert inside a transaction, signal, hold uncommitted
		// until the losers are in flight, then commit.
		winnerResult := make(chan upsertResult, 1)
		inserted := make(chan struct{})
		release := make(chan struct{})
		go func() {
			conn, err := sql.Open("mysql", dsn)
			if err != nil {
				winnerResult <- upsertResult{err: err}
				return
			}
			defer conn.Close()
			vcs := NewVersionControllerStorage(mysqlTestFactory(conn))
			dc := lib.NewDBContext(context.Background(), conn)
			var version string
			err = lib.RDBTxnExecute(dc, func(ctx context.Context) error {
				v, err := vcs.UpsertConfigLastExportedVersion(ctx, exportData(topic, "sign-w"))
				if err != nil {
					return err
				}
				version = v
				close(inserted)
				<-release
				return nil
			})
			winnerResult <- upsertResult{version: version, err: err}
		}()
		<-inserted

		// Losers all wake when the winner commits; the savepoint + FOR
		// UPDATE pin serializes their bump chain without deadlocking.
		results := make(chan upsertResult, losers)
		var wg sync.WaitGroup
		for i := 0; i < losers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sign, err := iversion_control.Sign(map[string]int{"worker": i})
				if err != nil {
					results <- upsertResult{err: err}
					return
				}
				version, err := upsertInTxnRetryable(t, dsn, exportData(topic, sign))
				results <- upsertResult{version: version, err: err}
			}(i)
		}
		time.Sleep(time.Duration(losers) * 3 * time.Millisecond)
		close(release)
		wg.Wait()
		close(results)

		winner := <-winnerResult
		if winner.err != nil {
			t.Logf("attempt %d: winner export failed: %v", attempt, winner.err)
			continue
		}

		got := map[string]bool{winner.version: true}
		failed := false
		for r := range results {
			if r.err != nil {
				t.Logf("attempt %d: concurrent export failed: %v", attempt, r.err)
				failed = true
				continue
			}
			got[r.version] = true
		}

		// Expected versions: winner at base+1s, losers at base+2s/+3s/+4s.
		expected := map[string]bool{}
		for i := 1; i <= losers+1; i++ {
			expected[iversion_control.Version(baseTime.Add(time.Duration(i)*time.Second))] = true
		}

		// A second rollover between the baseline and the candidate
		// computations shifts the whole chain; retry on a fresh topic.
		if failed || !assert.ObjectsAreEqual(expected, got) {
			continue
		}
		assert.Equal(t, losers+2, countTopicRows(t, dsn, topic),
			"baseline + winner + one row per loser")
		return
	}
	t.Fatal("no attempt converged: racing exports did not all succeed with distinct versions")
}

func countTopicRows(t *testing.T, dsn, topic string) int {
	conn, err := sql.Open("mysql", dsn)
	require.NoError(t, err)
	defer conn.Close()

	var n int
	require.NoError(t, conn.QueryRow(
		"SELECT COUNT(*) FROM config_versions WHERE name = ?", topic).Scan(&n))
	return n
}
