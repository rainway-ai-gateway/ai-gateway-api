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
	"math/rand"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// replaceTxAttempts bounds how often a whole-collection replacement is retried.
const replaceTxAttempts = 3

// ExecuteWithRetry runs do inside a transaction, retrying transient lock
// conflicts (MySQL 1213 deadlock / 1205 lock wait timeout, SQLite busy) with
// jittered backoff.
//
// do may run more than once, so it must be idempotent and free of side effects
// outside the transaction: this helper is meant for whole-collection
// replacement (delete-all + insert-all), not for general business writes.
//
// Exhausting the retries yields a 409 conflict error, so concurrent management
// writes never surface as 500 with a driver error (issue #295).
func ExecuteWithRetry(ctx context.Context, txn TxnStorager, do func(context.Context) error) error {
	var err error
	for attempt := 1; ; attempt++ {
		if err = txn.AtomExecute(ctx, do); err == nil {
			return nil
		}
		if attempt >= replaceTxAttempts || !lib.TransientLockError(err) {
			break
		}

		delay := time.Duration(attempt) * 20 * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(delay/2) + 1))
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay + jitter):
		}
	}

	if lib.TransientLockError(err) {
		return xerror.WrapConflictErrorWithMsg(
			"collection is being replaced concurrently, retry later")
	}
	return err
}
