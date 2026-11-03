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

// Package xlock serializes whole-collection replacements across api replicas.
//
// MySQL exposes named locks through GET_LOCK; SQLite serializes writers itself,
// so Acquire degrades to a no-op there.
package xlock

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// supportsNamedLock reports whether db speaks the MySQL named-lock protocol.
// Overridden in tests.
var supportsNamedLock = func(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*mysql.MySQLDriver)
	return ok
}

// Acquire takes the session-scoped named lock name and returns its release
// function, which is never nil. Drivers without named locks (SQLite) get a
// no-op release instead.
//
// Failing to take the lock within timeout is a business conflict, not a server
// error: it means another request is replacing the same collection right now,
// so callers should retry later.
func Acquire(ctx context.Context, db *sql.DB, name string, timeout time.Duration) (func(), error) {
	if !supportsNamedLock(db) {
		return func() {}, nil
	}

	// A named lock is bound to its session, so keep a dedicated connection for
	// the whole critical section instead of borrowing from the pool per query.
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, int(timeout/time.Second)).Scan(&got); err != nil {
		conn.Close()
		return nil, xerror.WrapDaoError(err)
	}
	if !got.Valid || got.Int64 != 1 {
		conn.Close()
		return nil, xerror.WrapConflictErrorWithMsg(
			"collection %q is being replaced by another request, retry later", name)
	}

	return func() {
		if _, err := conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", name); err != nil {
			// The lock lives as long as its session: discarding the physical
			// connection is what actually frees it. Handing it back to the pool
			// would strand the lock until the connection is recycled.
			_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}
