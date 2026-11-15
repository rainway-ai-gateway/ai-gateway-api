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

package access_log

import (
	"context"
	"sync"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
)

type fakeTxn struct{}

func (f *fakeTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

var _ itxn.TxnStorager = (*fakeTxn)(nil)

// fakeAccessLogConfStorager emulates the singleton DB behavior: Upsert
// overwrites the only row (fixed id=1), Fetch reads it back.
type fakeAccessLogConfStorager struct {
	fetchFn  func(ctx context.Context) (*AccessLogConf, error)
	upsertFn func(ctx context.Context, config *AccessLogConf) error

	mu     sync.Mutex
	config *AccessLogConf
}

func (s *fakeAccessLogConfStorager) Fetch(ctx context.Context) (*AccessLogConf, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fetchFn != nil {
		return s.fetchFn(ctx)
	}
	return s.config, nil
}

func (s *fakeAccessLogConfStorager) Upsert(ctx context.Context, config *AccessLogConf) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upsertFn != nil {
		return s.upsertFn(ctx, config)
	}
	s.config = config
	return nil
}

var _ AccessLogConfStorager = (*fakeAccessLogConfStorager)(nil)

// fakeOperationLogRecorder collects the audit entries for assertions.
type fakeOperationLogRecorder struct {
	mu      sync.Mutex
	entries []*ioperlog.OperationLogEntry
}

func (r *fakeOperationLogRecorder) Record(ctx context.Context, entry *ioperlog.OperationLogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, entry)
}

func (r *fakeOperationLogRecorder) last() *ioperlog.OperationLogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) == 0 {
		return nil
	}
	return r.entries[len(r.entries)-1]
}

var _ ioperlog.OperationLogRecorder = (*fakeOperationLogRecorder)(nil)
