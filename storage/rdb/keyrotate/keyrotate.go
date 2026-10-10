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

package rdbkeyrotate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	keyrotate "github.com/rainway-ai-gateway/ai-gateway-api/model/keyrotate"
)

// taskColumns is the shared SELECT column list for keyrotate_sweep_tasks.
const taskColumns = `task_id, status, mode, dry_run, scope, active_key_id, scanned, rewritten,
       COALESCE(summary,''), COALESCE(error,''), heartbeat_at, started_at, finished_at, duration_ms, COALESCE(created_by,'')`

// scanTask reads one task row from a QueryRow or Rows scan target.
func scanTask(scan interface{ Scan(dest ...interface{}) error }) (*taskRow, error) {
	var r taskRow
	var finishedAt sql.NullTime
	var summary sql.NullString
	if err := scan.Scan(&r.TaskID, &r.Status, &r.Mode, &r.DryRun, &r.Scope, &r.ActiveKeyID,
		&r.Scanned, &r.Rewritten, &summary, &r.Error, &r.HeartbeatAt, &r.StartedAt,
		&finishedAt, &r.DurationMs, &r.CreatedBy); err != nil {
		return nil, err
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		r.FinishedAt = &t
	}
	r.Summary = summary.String
	return &r, nil
}

// taskRow mirrors keyrotate_sweep_tasks.
type taskRow struct {
	TaskID      string
	Status      string
	Mode        string
	DryRun      bool
	Scope       string
	ActiveKeyID int
	Scanned     int64
	Rewritten   int64
	Summary     string
	Error       string
	HeartbeatAt time.Time
	StartedAt   time.Time
	FinishedAt  *time.Time
	DurationMs  int64
	CreatedBy   string
}

// Storager persists sweep tasks and performs the batched column rewrites.
type Storager struct {
	dbCtxFactory lib.DBContextFactory
	txn          itxn.TxnStorager
	heartbeatTTL time.Duration
}

// NewStorager builds the storager. heartbeatTTL marks a running task whose
// heartbeat stalls as lost so a new trigger can take over.
func NewStorager(dbCtxFactory lib.DBContextFactory, txn itxn.TxnStorager, heartbeatTTL time.Duration) *Storager {
	return &Storager{dbCtxFactory: dbCtxFactory, txn: txn, heartbeatTTL: heartbeatTTL}
}

// AcquireTask atomically (single lock row) decides whether a new task may
// start: no fresh running task -> insert; fresh running -> *ErrConflict;
// stale running -> mark it failed(executor lost) and insert.
func (s *Storager) AcquireTask(ctx context.Context, task *keyrotate.SweepTask) error {
	return s.txn.AtomExecute(ctx, func(ctx context.Context) error {
		dbCtx, err := s.dbCtxFactory(ctx)
		if err != nil {
			return err
		}
		ex := dbCtx.Execer()

		var holder string
		if err := ex.QueryRowContext(ctx,
			`SELECT holder_task_id FROM keyrotate_sweep_lock WHERE id = 1 FOR UPDATE`).
			Scan(&holder); err != nil {
			return err
		}

		if holder != "" {
			row, err := s.taskByID(ctx, ex, holder)
			if err != nil {
				return err
			}
			if row != nil && row.Status == string(keyrotate.StatusRunning) {
				if time.Since(row.HeartbeatAt) < s.heartbeatTTL {
					return &keyrotate.ErrConflict{HolderTaskID: holder}
				}
				// lost executor: mark failed and take over
				if _, err := ex.ExecContext(ctx,
					`UPDATE keyrotate_sweep_tasks SET status=?, error=?, finished_at=? WHERE task_id=? AND status='running'`,
					string(keyrotate.StatusFailed), "executor lost", time.Now(), holder); err != nil {
					return err
				}
			}
		}

		if _, err := ex.ExecContext(ctx,
			`INSERT INTO keyrotate_sweep_tasks
(task_id, status, mode, dry_run, scope, active_key_id, scanned, rewritten, summary, error, heartbeat_at, started_at, finished_at, duration_ms, created_by)
VALUES (?,?,?,?,?,?,0,0,'','',?,?,NULL,0,?)`,
			task.TaskID, string(task.Status), string(task.Mode), task.DryRun, string(task.Scope),
			task.ActiveKeyID, task.HeartbeatAt, task.StartedAt, task.CreatedBy); err != nil {
			return err
		}
		_, err = ex.ExecContext(ctx,
			`UPDATE keyrotate_sweep_lock SET holder_task_id=? WHERE id=1`, task.TaskID)
		return err
	})
}

// GetTask fetches one task (no lock).
func (s *Storager) GetTask(ctx context.Context, taskID string) (*keyrotate.SweepTask, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}
	row, err := scanTask(dbCtx.Execer().QueryRowContext(ctx,
		"SELECT "+taskColumns+" FROM keyrotate_sweep_tasks WHERE task_id=?", taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, keyrotate.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.toTask(), nil
}

// taskFilterWhere builds the WHERE clause and args for ListTasks.
func taskFilterWhere(filter *keyrotate.TaskFilter) (string, []interface{}) {
	where := "1=1"
	args := []interface{}{}
	if filter.Status != nil {
		where += " AND status=?"
		args = append(args, string(*filter.Status))
	}
	if filter.Mode != nil {
		where += " AND mode=?"
		args = append(args, string(*filter.Mode))
	}
	if filter.Scope != nil {
		where += " AND scope=?"
		args = append(args, string(*filter.Scope))
	}
	if filter.DryRun != nil {
		where += " AND dry_run=?"
		args = append(args, *filter.DryRun)
	}
	if filter.StartTime != nil {
		where += " AND started_at>=?"
		args = append(args, *filter.StartTime)
	}
	if filter.EndTime != nil {
		where += " AND started_at<=?"
		args = append(args, *filter.EndTime)
	}
	return where, args
}

// ListTasks queries sweep task history with filters and offset pagination,
// ordered by started_at (default) or task_id. Returns the filtered total.
func (s *Storager) ListTasks(ctx context.Context, filter *keyrotate.TaskFilter) ([]*keyrotate.SweepTask, int64, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, 0, err
	}
	ex := dbCtx.Execer()

	where, args := taskFilterWhere(filter)

	var total int64
	if err := ex.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM keyrotate_sweep_tasks WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderCol := "started_at"
	if filter.SortBy == "task_id" {
		orderCol = "task_id"
	}
	orderDir := "DESC"
	if strings.EqualFold(filter.SortOrder, "asc") {
		orderDir = "ASC"
	}
	offset := (filter.Page - 1) * filter.PageSize
	if offset < 0 {
		offset = 0
	}

	rows, err := ex.QueryContext(ctx,
		fmt.Sprintf("SELECT %s FROM keyrotate_sweep_tasks WHERE %s ORDER BY %s %s LIMIT ? OFFSET ?",
			taskColumns, where, orderCol, orderDir),
		append(args, filter.PageSize, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tasks := []*keyrotate.SweepTask{}
	for rows.Next() {
		r, err := scanTask(rows)
		if err != nil {
			return nil, 0, err
		}
		tasks = append(tasks, r.toTask())
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// AddProgress bumps counters and heartbeat in the same statement as the
// caller's batch (the manager calls it inside the batch transaction).
func (s *Storager) AddProgress(ctx context.Context, dbCtx lib.DBContexter, taskID string, scanned, rewritten int64) error {
	_, err := dbCtx.Execer().ExecContext(ctx,
		`UPDATE keyrotate_sweep_tasks SET scanned=scanned+?, rewritten=rewritten+?, heartbeat_at=? WHERE task_id=?`,
		scanned, rewritten, time.Now(), taskID)
	return err
}

// FinishTask marks the task terminal and releases the lock.
func (s *Storager) FinishTask(ctx context.Context, taskID string, status keyrotate.Status, cause string,
	summary map[string]*keyrotate.TableCount, duration time.Duration) error {
	sumJSON, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	return s.txn.AtomExecute(ctx, func(ctx context.Context) error {
		dbCtx, err := s.dbCtxFactory(ctx)
		if err != nil {
			return err
		}
		ex := dbCtx.Execer()
		now := time.Now()
		if _, err := ex.ExecContext(ctx,
			`UPDATE keyrotate_sweep_tasks SET status=?, error=?, summary=?, finished_at=?, duration_ms=?, heartbeat_at=? WHERE task_id=?`,
			string(status), cause, string(sumJSON), now, duration.Milliseconds(), now, taskID); err != nil {
			return err
		}
		_, err = ex.ExecContext(ctx,
			`UPDATE keyrotate_sweep_lock SET holder_task_id='' WHERE id=1 AND holder_task_id=?`, taskID)
		return err
	})
}

// ListRows scans one page after cursor (pk > cursor), ordered by pk.
func (s *Storager) ListRows(ctx context.Context, t keyrotate.SweepTable, cursor interface{}, limit int) ([]keyrotate.RowRef, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}
	var query string
	var args []interface{}
	if cursor == nil {
		query = fmt.Sprintf(`SELECT %s, %s FROM %s ORDER BY %s LIMIT ?`, t.PKCol, t.ValCol, t.Name, t.PKCol)
	} else {
		query = fmt.Sprintf(`SELECT %s, %s FROM %s WHERE %s > ? ORDER BY %s LIMIT ?`, t.PKCol, t.ValCol, t.Name, t.PKCol, t.PKCol)
		args = append(args, cursor)
	}
	args = append(args, limit)

	rows, err := dbCtx.Execer().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []keyrotate.RowRef
	for rows.Next() {
		var r keyrotate.RowRef
		var val sql.NullString
		if err := rows.Scan(&r.PK, &val); err != nil {
			return nil, err
		}
		r.Value = val.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// RewriteRow writes the new sensitive value for one pk (inside the caller's
// batch transaction when dbCtx carries one).
func (s *Storager) RewriteRow(ctx context.Context, dbCtx lib.DBContexter, t keyrotate.SweepTable, pk interface{}, value string) error {
	_, err := dbCtx.Execer().ExecContext(ctx,
		fmt.Sprintf(`UPDATE %s SET %s=? WHERE %s=?`, t.Name, t.ValCol, t.PKCol), value, pk)
	return err
}

func (s *Storager) taskByID(ctx context.Context, ex lib.SqlExecutor, taskID string) (*taskRow, error) {
	row, err := scanTask(ex.QueryRowContext(ctx,
		"SELECT "+taskColumns+" FROM keyrotate_sweep_tasks WHERE task_id=?", taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func (r *taskRow) toTask() *keyrotate.SweepTask {
	t := &keyrotate.SweepTask{
		TaskID:      r.TaskID,
		Status:      keyrotate.Status(r.Status),
		Mode:        keyrotate.Mode(r.Mode),
		DryRun:      r.DryRun,
		Scope:       keyrotate.Scope(r.Scope),
		ActiveKeyID: r.ActiveKeyID,
		Scanned:     r.Scanned,
		Rewritten:   r.Rewritten,
		StartedAt:   r.StartedAt,
		FinishedAt:  r.FinishedAt,
		DurationMs:  r.DurationMs,
		FailedCause: r.Error,
		HeartbeatAt: r.HeartbeatAt,
		CreatedBy:   r.CreatedBy,
	}
	if r.Summary != "" {
		t.Summary = map[string]*keyrotate.TableCount{}
		json.Unmarshal([]byte(r.Summary), &t.Summary)
	}
	return t
}

// tablesForScope resolves the scope to the concrete table list.
