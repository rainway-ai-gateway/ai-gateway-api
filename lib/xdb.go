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

package lib

import (
	"context"
	"database/sql"
	"strings"
)

type DBContexter interface {
	context.Context
	Conn() *sql.DB
	// Execer returns the effective SQL executor: the active transaction
	// when inside one, or the connection pool otherwise. dao statements
	// must go through Execer so AtomExecute closures actually run in the
	// transaction instead of autocommitting through the pool.
	Execer() SqlExecutor
}

// SqlExecutor is the common statement-execution surface implemented by
// both *sql.DB (connection pool, autocommit) and *sql.Tx (transaction).
type SqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

type DBContext struct {
	conn *sql.DB
	tx   *sql.Tx

	context.Context
}

func NewDBContext(ctx context.Context, conn *sql.DB) *DBContext {
	return &DBContext{
		conn:    conn,
		Context: ctx,
	}
}

func (ctx *DBContext) Conn() *sql.DB {
	return ctx.conn
}

// Execer returns the effective SQL executor: the active transaction when
// one is open, or the connection pool otherwise.
func (ctx *DBContext) Execer() SqlExecutor {
	if ctx.tx != nil {
		return ctx.tx
	}
	return ctx.conn
}

func (ctx *DBContext) BeginTrans() error {
	if ctx.tx != nil {
		return nil
	}

	var err error
	ctx.tx, err = ctx.conn.Begin()
	return err
}

type Op struct {
	blockWrite bool
	openTxn    bool
}

func BlockWrite() *Op {
	return &Op{
		blockWrite: true,
	}
}

func WantBlockWrite(ops ...*Op) bool {
	for _, op := range ops {
		if op.blockWrite {
			return true
		}
	}

	return false
}

func OpenTxn() *Op {
	return &Op{
		openTxn: true,
	}
}

func WantOpenTxn(ops ...*Op) bool {
	for _, op := range ops {
		if op.openTxn {
			return true
		}
	}

	return false
}

type DBContextFactory func(ctx context.Context, ops ...*Op) (*DBContext, error)

func RDBTxnExecute(dc *DBContext, handler func(context.Context) error) error {
	var err error
	if dc.tx == nil {
		dc.tx, err = dc.conn.Begin()
		if err != nil {
			return err
		}
	}

	err = handler(dc)
	return commitOrRollback(dc.tx, err)
}

func commitOrRollback(tx *sql.Tx, err error) error {
	if err != nil {
		if strings.Contains(err.Error(), "invalid connection") {
			return err
		}

		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func Transaction(conn *sql.DB, do func(*sql.Tx) error) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}

	if err := do(tx); err != nil {

		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func DuplicateEntryError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Error 1062: Duplicate entry") ||
		strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}

// TransientLockError reports whether err is a retryable lock conflict:
// MySQL 1213 deadlock, MySQL 1205 lock wait timeout, or a SQLite busy error.
// Whole-collection replacements are idempotent, so their transactions can be
// replayed safely (issue #295).
func TransientLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range []string{
		"Error 1213",
		"Deadlock found",
		"Error 1205",
		"Lock wait timeout exceeded",
		"database is locked",
		"database table is locked",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
