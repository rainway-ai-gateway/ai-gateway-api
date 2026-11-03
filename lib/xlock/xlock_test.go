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

package xlock

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/glebarez/go-sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

func withNamedLock(t *testing.T, enabled bool) {
	t.Helper()
	orig := supportsNamedLock
	supportsNamedLock = func(*sql.DB) bool { return enabled }
	t.Cleanup(func() { supportsNamedLock = orig })
}

func TestAcquireNoopWithoutNamedLock(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	release, err := Acquire(context.Background(), db, "test", time.Second)
	require.NoError(t, err)
	require.NotNil(t, release)
	release() // must not panic or hit the database
}

func TestAcquireTakesAndReleasesLock(t *testing.T) {
	withNamedLock(t, true)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT GET_LOCK").WithArgs("l", 10).
		WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK"}).AddRow(int64(1)))
	mock.ExpectExec("SELECT RELEASE_LOCK").WithArgs("l").
		WillReturnResult(sqlmock.NewResult(0, 0))

	release, err := Acquire(context.Background(), db, "l", 10*time.Second)
	require.NoError(t, err)
	release()

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAcquireTimeoutIsConflict(t *testing.T) {
	withNamedLock(t, true)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT GET_LOCK").WithArgs("l", 10).
		WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK"}).AddRow(int64(0)))

	_, err = Acquire(context.Background(), db, "l", 10*time.Second)
	require.Error(t, err)

	rr := xerror.Resolve(err)
	assert.Equal(t, 409, rr.ErrNo)
	assert.Equal(t, "Conflict", rr.Type)
	assert.Contains(t, rr.Msg, "being replaced by another request")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAcquireDriverErrorIsDatabaseException(t *testing.T) {
	withNamedLock(t, true)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT GET_LOCK").WithArgs("l", 10).
		WillReturnError(errors.New("Error 1045 (28000): Access denied"))

	_, err = Acquire(context.Background(), db, "l", 10*time.Second)
	require.Error(t, err)

	rr := xerror.Resolve(err)
	assert.Equal(t, 500, rr.ErrNo)
	assert.Equal(t, "Database Exception", rr.Type)
	assert.Equal(t, "database operation failed", rr.Msg) // F-14: no driver text
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReleaseFailureDiscardsConnection(t *testing.T) {
	withNamedLock(t, true)

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT GET_LOCK").WithArgs("l", 10).
		WillReturnRows(sqlmock.NewRows([]string{"GET_LOCK"}).AddRow(int64(1)))
	mock.ExpectExec("SELECT RELEASE_LOCK").WithArgs("l").
		WillReturnError(errors.New("invalid connection"))
	mock.ExpectClose() // conn.Raw + Close must discard the physical connection

	release, err := Acquire(context.Background(), db, "l", 10*time.Second)
	require.NoError(t, err)
	release()

	require.NoError(t, mock.ExpectationsWereMet())
}
