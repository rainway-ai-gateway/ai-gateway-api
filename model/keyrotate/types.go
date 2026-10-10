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

// Package keyrotate implements the secret-at-rest sweep manager: batch
// reencrypt/decrypt of sensitive columns (see design-docs modifications
// 2026-10-05-db-encryption-at-rest).
package keyrotate

import (
	"errors"
	"time"
)

type Mode string

const (
	// ModeReencrypt rewrites non-target rows (plaintext / stale keyID) to
	// active-key ciphertext. Covers initial enablement and rotation converge.
	ModeReencrypt Mode = "reencrypt"
	// ModeDecrypt rewrites all ciphertext rows back to plaintext; rollback
	// preparation only.
	ModeDecrypt Mode = "decrypt"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Scope string

const (
	ScopeAll       Scope = "all"
	ScopeProviders Scope = "providers"
	ScopeAPIKeys   Scope = "api_keys"
)

// ErrConflict is returned when another sweep is running; HolderTaskID
// identifies the running task to poll.
type ErrConflict struct {
	HolderTaskID string
}

// TaskFilter selects sweep tasks for the list endpoint (OpenAPI GET
// .../reencrypt-sweeps). Nil fields are ignored. Page/PageSize are
// 1-based and normalized by the caller; SortBy is "started_at" or
// "task_id"; SortOrder is "asc" or "desc".
type TaskFilter struct {
	Status    *Status
	Mode      *Mode
	Scope     *Scope
	DryRun    *bool
	StartTime *time.Time
	EndTime   *time.Time
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
}

func (e *ErrConflict) Error() string {
	return "another reencrypt sweep is running: " + e.HolderTaskID
}

// TableCount holds per-table progress counters.
type TableCount struct {
	Scanned   int64 `json:"scanned"`
	Rewritten int64 `json:"rewritten"`
	Skipped   int64 `json:"skipped"`
}

// SweepTask is one sweep execution record.
type SweepTask struct {
	TaskID      string
	Status      Status
	Mode        Mode
	DryRun      bool
	Scope       Scope
	ActiveKeyID int

	Scanned   int64
	Rewritten int64

	StartedAt   time.Time
	FinishedAt  *time.Time
	DurationMs  int64
	FailedCause string

	// Summary is the per-table breakdown returned by GET.
	Summary map[string]*TableCount

	HeartbeatAt time.Time
	CreatedBy   string
}

// HasError is a helper for templates.
var ErrTaskNotFound = errors.New("sweep task not found")
