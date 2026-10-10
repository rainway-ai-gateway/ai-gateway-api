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
//limitations under the License. All rights reserved.

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

package keyrotate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bfenetworks/go-lib/log"
	"github.com/google/uuid"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xcrypto"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

const (
	DefaultBatchSize        = 100
	DefaultHeartbeatTTL     = 5 * time.Minute
	defaultMaxBatchAttempts = 3
)

// Storager abstracts the persistence layer (mocked in tests).
type Storager interface {
	AcquireTask(ctx context.Context, task *SweepTask) error
	GetTask(ctx context.Context, taskID string) (*SweepTask, error)
	ListTasks(ctx context.Context, filter *TaskFilter) ([]*SweepTask, int64, error)
	AddProgress(ctx context.Context, dbCtx lib.DBContexter, taskID string, scanned, rewritten int64) error
	FinishTask(ctx context.Context, taskID string, status Status, cause string,
		summary map[string]*TableCount, duration time.Duration) error
	ListRows(ctx context.Context, t SweepTable, cursor interface{}, limit int) ([]RowRef, error)
	RewriteRow(ctx context.Context, dbCtx lib.DBContexter, t SweepTable, pk interface{}, value string) error
}

// Manager runs sweep tasks. Tasks execute on the triggering instance; state
// lives in the DB so any instance can serve status queries.
type Manager struct {
	storager     Storager
	dbCtxFactory lib.DBContextFactory
	batchSize    int
}

func NewManager(storager Storager, dbCtxFactory lib.DBContextFactory) *Manager {
	return &Manager{storager: storager, dbCtxFactory: dbCtxFactory, batchSize: DefaultBatchSize}
}

// Trigger creates a task. *keyrotate.ErrConflict (via *ErrConflict) signals a
// running task; caller maps it to 409 with the holder task_id.
func (m *Manager) Trigger(ctx context.Context, mode Mode, dryRun bool, scope Scope, operator string) (*SweepTask, error) {
	if mode == "" {
		mode = ModeReencrypt
	}
	if mode != ModeReencrypt && mode != ModeDecrypt {
		return nil, fmt.Errorf("invalid mode %q", mode)
	}
	if scope == "" {
		scope = ScopeAll
	}
	if scope != ScopeAll && scope != ScopeProviders && scope != ScopeAPIKeys {
		return nil, fmt.Errorf("invalid scope %q", scope)
	}

	now := time.Now()
	activeID := 0
	if mode != ModeDecrypt {
		if kr := stateful.SecretRing(); kr != nil {
			activeID = int(kr.ActiveID())
		}
	}
	task := &SweepTask{
		TaskID:      "rsp-" + uuid.New().String()[:20],
		Status:      StatusRunning,
		Mode:        mode,
		DryRun:      dryRun,
		Scope:       scope,
		ActiveKeyID: activeID,
		StartedAt:   now,
		HeartbeatAt: now,
		CreatedBy:   operator,
	}
	if err := m.storager.AcquireTask(ctx, task); err != nil {
		var conflict *ErrConflict
		if errors.As(err, &conflict) {
			return &SweepTask{TaskID: conflict.HolderTaskID, Status: StatusRunning}, err
		}
		return nil, err
	}

	log.Logger.Info("keyrotate: sweep task %s started (mode=%s dry_run=%v scope=%s activeKeyID=%d by=%s)",
		task.TaskID, mode, dryRun, scope, activeID, operator)
	go m.run(task)
	return task, nil
}

// GetTask returns a task by id (any instance).
func (m *Manager) GetTask(ctx context.Context, taskID string) (*SweepTask, error) {
	return m.storager.GetTask(ctx, taskID)
}

// ListTasks queries sweep task history (OpenAPI GET .../reencrypt-sweeps).
func (m *Manager) ListTasks(ctx context.Context, filter *TaskFilter) ([]*SweepTask, int64, error) {
	return m.storager.ListTasks(ctx, filter)
}

// run executes the task on this instance.
func (m *Manager) run(task *SweepTask) {
	started := time.Now()
	stateful.MetricCryptoSweepRunning.Inc()
	defer stateful.MetricCryptoSweepRunning.Dec()

	summary := map[string]*TableCount{}
	var firstErr error
	for _, tbl := range TablesForScope(task.Scope) {
		cnt, err := m.sweepTable(context.Background(), task, tbl)
		summary[tbl.Name] = cnt
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	status := StatusSucceeded
	cause := ""
	if firstErr != nil {
		status = StatusFailed
		cause = firstErr.Error()
	}
	if task.DryRun {
		cause = "dry_run"
	}
	if err := m.storager.FinishTask(context.Background(), task.TaskID, status, cause, summary, time.Since(started)); err != nil {
		log.Logger.Warn("keyrotate: finish task %s: %v", task.TaskID, err)
	}
	log.Logger.Info("keyrotate: sweep task %s finished status=%s summary=%v", task.TaskID, status, summary)
}

func (m *Manager) sweepTable(ctx context.Context, task *SweepTask, tbl SweepTable) (*TableCount, error) {
	cnt := &TableCount{}
	var cursor interface{}
	for {
		rows, err := m.storager.ListRows(ctx, tbl, cursor, m.batchSize)
		if err != nil {
			return cnt, fmt.Errorf("scan %s: %v", tbl.Name, err)
		}
		if len(rows) == 0 {
			return cnt, nil
		}

		if err := m.execBatch(ctx, task, tbl, rows, cnt); err != nil {
			return cnt, err
		}
		cursor = rows[len(rows)-1].PK
	}
}

// execBatch rewrites one page inside a transaction together with the progress
// heartbeat, so progress always reflects committed work. Deadlocks retry with
// backoff (business traffic wins).
func (m *Manager) execBatch(ctx context.Context, task *SweepTask, tbl SweepTable,
	rows []RowRef, cnt *TableCount) error {
	var lastErr error
	for attempt := 0; attempt < defaultMaxBatchAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*200) * time.Millisecond)
		}
		lastErr = m.execBatchOnce(ctx, task, tbl, rows, cnt)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("rewrite %s batch: %v (after %d attempts)", tbl.Name, lastErr, defaultMaxBatchAttempts)
}

func (m *Manager) execBatchOnce(ctx context.Context, task *SweepTask, tbl SweepTable,
	rows []RowRef, cnt *TableCount) error {
	var rew int64
	core := func(ctx context.Context, dbCtx lib.DBContexter) error {
		defer func() {
			// best-effort progress inside the batch transaction; failure
			// here must not roll back rewritten rows
			_ = m.storager.AddProgress(ctx, dbCtx, task.TaskID, int64(len(rows)), rew)
		}()
		for _, row := range rows {
			target, skip, err := m.transform(task, row.Value)
			if err != nil {
				// decryption failure of a single row fails the task; the
				// ciphertext is never propagated (only keyID goes to logs)
				keyID, _ := xcrypto.KeyIDOf(row.Value)
				stateful.MetricCryptoDecryptFail.WithLabelValues(tbl.Name, fmt.Sprint(keyID)).Inc()
				return fmt.Errorf("%s pk=%v: decrypt fail (keyID=%d)", tbl.Name, row.PK, keyID)
			}
			if skip {
				cnt.Skipped++
				continue
			}
			if task.DryRun {
				cnt.Rewritten++
				rew++
				continue
			}
			if err := m.storager.RewriteRow(ctx, dbCtx, tbl, row.PK, target); err != nil {
				return err
			}
			cnt.Rewritten++
			rew++
		}
		return nil
	}
	if m.dbCtxFactory == nil {
		// test path: no transaction wrapper
		return core(ctx, nil)
	}
	dbCtx, err := m.dbCtxFactory(ctx, lib.OpenTxn())
	if err != nil {
		return err
	}
	return lib.RDBTxnExecute(dbCtx, func(ctx context.Context) error {
		return core(ctx, dbCtx)
	})
}

// transform computes the desired value for one cell.
// skip=true means the row already matches the target form.
func (m *Manager) transform(task *SweepTask, value string) (target string, skip bool, err error) {
	kr := stateful.SecretRing()
	switch task.Mode {
	case ModeDecrypt:
		if !xcrypto.IsEncrypted(value) {
			return "", true, nil
		}
		pt, err := decryptWith(kr, value)
		if err != nil {
			return "", false, err
		}
		return pt, false, nil
	default: // ModeReencrypt
		if xcrypto.IsEncrypted(value) {
			if id, ok := xcrypto.KeyIDOf(value); ok && kr != nil && int(id) == int(kr.ActiveID()) {
				return "", true, nil
			}
		}
		pt, err := decryptWith(kr, value)
		if err != nil {
			return "", false, err
		}
		if kr == nil {
			// no keyring configured: nothing to converge to
			return "", true, nil
		}
		ct, err := kr.Encrypt(pt)
		if err != nil {
			return "", false, err
		}
		return ct, false, nil
	}
}

// decryptWith decrypts when a keyring is configured; plaintext values (or a
// nil keyring with plaintext data) pass through.
func decryptWith(kr *xcrypto.Keyring, value string) (string, error) {
	if kr == nil {
		return value, nil
	}
	return kr.Decrypt(value)
}
