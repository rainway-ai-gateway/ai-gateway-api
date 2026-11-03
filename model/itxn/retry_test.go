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

package itxn

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// scriptedTxn replays a fixed error sequence, one entry per attempt.
type scriptedTxn struct {
	errs  []error
	calls int
}

func (f *scriptedTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	i := f.calls
	f.calls++
	if i < len(f.errs) {
		return f.errs[i]
	}
	return nil
}

var (
	deadlockErr = errors.New("Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction")
	boomErr     = errors.New("boom")
)

func TestExecuteWithRetrySucceedsFirstTry(t *testing.T) {
	txn := &scriptedTxn{}
	require.NoError(t, ExecuteWithRetry(context.Background(), txn, func(context.Context) error { return nil }))
	assert.Equal(t, 1, txn.calls)
}

func TestExecuteWithRetryRetriesTransient(t *testing.T) {
	txn := &scriptedTxn{errs: []error{deadlockErr, deadlockErr}}
	require.NoError(t, ExecuteWithRetry(context.Background(), txn, func(context.Context) error { return nil }))
	assert.Equal(t, 3, txn.calls)
}

func TestExecuteWithRetryExhaustedIsConflict(t *testing.T) {
	txn := &scriptedTxn{errs: []error{deadlockErr, deadlockErr, deadlockErr, deadlockErr}}
	err := ExecuteWithRetry(context.Background(), txn, func(context.Context) error { return nil })
	require.Error(t, err)
	assert.Equal(t, 3, txn.calls)

	rr := xerror.Resolve(err)
	assert.Equal(t, 409, rr.ErrNo)
	assert.Equal(t, "Conflict", rr.Type)
	assert.Contains(t, rr.Msg, "being replaced concurrently")
}

func TestExecuteWithRetryDoesNotRetryOtherErrors(t *testing.T) {
	txn := &scriptedTxn{errs: []error{boomErr}}
	err := ExecuteWithRetry(context.Background(), txn, func(context.Context) error { return nil })
	require.Error(t, err)
	assert.Equal(t, 1, txn.calls)
	assert.Equal(t, boomErr, err)
}

func TestExecuteWithRetryStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txn := &scriptedTxn{errs: []error{deadlockErr}}
	err := ExecuteWithRetry(ctx, txn, func(context.Context) error { return nil })
	require.Error(t, err)
	assert.Equal(t, 1, txn.calls)
	assert.Equal(t, deadlockErr, err)
}
