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

package lib

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDBContext(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := NewDBContext(context.Background(), db)
	assert.Equal(t, db, ctx.Conn())
}

func TestDBContextBeginTrans(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()

	ctx := NewDBContext(context.Background(), db)
	require.NoError(t, ctx.BeginTrans())
	assert.NotNil(t, ctx.tx)

	// Second BeginTrans is a no-op.
	require.NoError(t, ctx.BeginTrans())

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDBContextExecer(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	ctx := NewDBContext(context.Background(), db)

	// Without a transaction, Execer routes to the connection pool.
	assert.Equal(t, db, ctx.Execer())

	// With a transaction, Execer routes to the transaction.
	mock.ExpectBegin()
	require.NoError(t, ctx.BeginTrans())
	assert.Equal(t, ctx.tx, ctx.Execer())

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestOpHelpers(t *testing.T) {
	assert.True(t, WantBlockWrite(BlockWrite()))
	assert.False(t, WantBlockWrite())
	assert.False(t, WantBlockWrite(OpenTxn()))

	assert.True(t, WantOpenTxn(OpenTxn()))
	assert.False(t, WantOpenTxn())
	assert.False(t, WantOpenTxn(BlockWrite()))
}

func TestTransactionCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	err = Transaction(db, func(tx *sql.Tx) error {
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestTransactionRollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectRollback()

	expectedErr := errors.New("boom")
	err = Transaction(db, func(tx *sql.Tx) error {
		return expectedErr
	})
	assert.Equal(t, expectedErr, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDuplicateEntryError(t *testing.T) {
	assert.False(t, DuplicateEntryError(nil))
	assert.False(t, DuplicateEntryError(errors.New("some error")))
	assert.True(t, DuplicateEntryError(errors.New("Error 1062: Duplicate entry 'x'")))
	assert.True(t, DuplicateEntryError(errors.New("UNIQUE constraint failed")))
	assert.True(t, DuplicateEntryError(errors.New("duplicate key value violates unique constraint")))
}

func TestTransientLockError(t *testing.T) {
	assert.False(t, TransientLockError(nil))

	for _, msg := range []string{
		"Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction",
		"Deadlock found when trying to get lock",
		"Error 1205 (HY000): Lock wait timeout exceeded; try restarting transaction",
		"database is locked (5) (SQLITE_BUSY)",
	} {
		assert.True(t, TransientLockError(errors.New(msg)), msg)
	}

	for _, msg := range []string{
		"Error 1062: Duplicate entry 'x'",
		"Error 1146: Table 'ai_gateway.t' doesn't exist",
		"context canceled",
	} {
		assert.False(t, TransientLockError(errors.New(msg)), msg)
	}
}
