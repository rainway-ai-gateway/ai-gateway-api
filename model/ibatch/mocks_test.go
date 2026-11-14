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

package ibatch

import (
	"context"
	"sync"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/imodel_price"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
)

// fakeBatchStorager 手写 callback mock（TESTING.md 惯例）。
type fakeBatchStorager struct {
	upsertTaskFn     func(ctx context.Context, task *BatchTask) (int64, error)
	fetchTaskFn      func(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error)
	listTasksFn      func(ctx context.Context, filter *BatchTaskFilter) ([]*BatchTask, error)
	preemptCancelFn  func(ctx context.Context, batchID, productName string) (int64, error)
	updateSettleFn   func(ctx context.Context, batchID, productName, fromStatus string, patch *BatchSettlePatch) (int64, error)
	updateProgressFn func(ctx context.Context, id int64, patch *BatchTaskProgressPatch) (int64, error)
	upsertFileFn     func(ctx context.Context, file *BatchFile) (int64, error)
	fetchFileFn      func(ctx context.Context, filter *BatchFileFilter) (*BatchFile, error)
	listFilesFn      func(ctx context.Context, filter *BatchFileFilter) ([]*BatchFile, error)

	mu              sync.Mutex
	upsertedTasks   []*BatchTask
	upsertedFiles   []*BatchFile
	settlePatches   []*BatchSettlePatch
	progressPatches []*BatchTaskProgressPatch
}

func (f *fakeBatchStorager) UpsertBatchTask(ctx context.Context, task *BatchTask) (int64, error) {
	f.mu.Lock()
	f.upsertedTasks = append(f.upsertedTasks, task)
	f.mu.Unlock()
	if f.upsertTaskFn != nil {
		return f.upsertTaskFn(ctx, task)
	}
	return 0, nil
}

func (f *fakeBatchStorager) FetchBatchTask(ctx context.Context, filter *BatchTaskFilter) (*BatchTask, error) {
	if f.fetchTaskFn != nil {
		return f.fetchTaskFn(ctx, filter)
	}
	return nil, nil
}

func (f *fakeBatchStorager) ListBatchTasks(ctx context.Context, filter *BatchTaskFilter) ([]*BatchTask, error) {
	if f.listTasksFn != nil {
		return f.listTasksFn(ctx, filter)
	}
	return nil, nil
}

func (f *fakeBatchStorager) PreemptBatchTaskCancel(ctx context.Context, batchID, productName string) (int64, error) {
	if f.preemptCancelFn != nil {
		return f.preemptCancelFn(ctx, batchID, productName)
	}
	return 0, nil
}

func (f *fakeBatchStorager) UpdateBatchTaskSettleStatus(ctx context.Context, batchID, productName, fromStatus string, patch *BatchSettlePatch) (int64, error) {
	f.mu.Lock()
	f.settlePatches = append(f.settlePatches, patch)
	f.mu.Unlock()
	if f.updateSettleFn != nil {
		return f.updateSettleFn(ctx, batchID, productName, fromStatus, patch)
	}
	return 0, nil
}

func (f *fakeBatchStorager) UpdateBatchTaskProgress(ctx context.Context, id int64, patch *BatchTaskProgressPatch) (int64, error) {
	f.mu.Lock()
	f.progressPatches = append(f.progressPatches, patch)
	f.mu.Unlock()
	if f.updateProgressFn != nil {
		return f.updateProgressFn(ctx, id, patch)
	}
	return 0, nil
}

func (f *fakeBatchStorager) UpsertBatchFile(ctx context.Context, file *BatchFile) (int64, error) {
	f.mu.Lock()
	f.upsertedFiles = append(f.upsertedFiles, file)
	f.mu.Unlock()
	if f.upsertFileFn != nil {
		return f.upsertFileFn(ctx, file)
	}
	return 0, nil
}

func (f *fakeBatchStorager) FetchBatchFile(ctx context.Context, filter *BatchFileFilter) (*BatchFile, error) {
	if f.fetchFileFn != nil {
		return f.fetchFileFn(ctx, filter)
	}
	return nil, nil
}

func (f *fakeBatchStorager) ListBatchFiles(ctx context.Context, filter *BatchFileFilter) ([]*BatchFile, error) {
	if f.listFilesFn != nil {
		return f.listFilesFn(ctx, filter)
	}
	return nil, nil
}

// fakeRedisClient 手写 callback mock。maps 承载按 key 的应答。
type fakeRedisClient struct {
	hGetAllFn      func(ctx context.Context, key string) (map[string]string, error)
	hSetWithTTLFn  func(ctx context.Context, key string, ttlSeconds int, fields map[string]string) error
	setNXWithTTLFn func(ctx context.Context, key string, ttlSeconds int) (bool, error)
	reserveDecrFn  func(ctx context.Context, key string, delta int64) (int64, error)
	deleteFn       func(ctx context.Context, key string) error
	scanFn         func(ctx context.Context, cursor uint64, match string, count int64) (uint64, []string, error)

	mu          sync.Mutex
	hSets       []redisHSetRecord
	deletedKeys []string
	decrRecords []redisDecrRecord
}

type redisHSetRecord struct {
	key    string
	fields map[string]string
}

type redisDecrRecord struct {
	key   string
	delta int64
}

func (f *fakeRedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	if f.hGetAllFn != nil {
		return f.hGetAllFn(ctx, key)
	}
	return nil, nil
}

func (f *fakeRedisClient) HSetWithTTL(ctx context.Context, key string, ttlSeconds int, fields map[string]string) error {
	f.mu.Lock()
	f.hSets = append(f.hSets, redisHSetRecord{key: key, fields: fields})
	f.mu.Unlock()
	if f.hSetWithTTLFn != nil {
		return f.hSetWithTTLFn(ctx, key, ttlSeconds, fields)
	}
	return nil
}

func (f *fakeRedisClient) SetNXWithTTL(ctx context.Context, key string, ttlSeconds int) (bool, error) {
	if f.setNXWithTTLFn != nil {
		return f.setNXWithTTLFn(ctx, key, ttlSeconds)
	}
	return true, nil
}

func (f *fakeRedisClient) ReserveDecr(ctx context.Context, key string, delta int64) (int64, error) {
	f.mu.Lock()
	f.decrRecords = append(f.decrRecords, redisDecrRecord{key: key, delta: delta})
	f.mu.Unlock()
	if f.reserveDecrFn != nil {
		return f.reserveDecrFn(ctx, key, delta)
	}
	return delta, nil
}

func (f *fakeRedisClient) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	f.deletedKeys = append(f.deletedKeys, key)
	f.mu.Unlock()
	if f.deleteFn != nil {
		return f.deleteFn(ctx, key)
	}
	return nil
}

func (f *fakeRedisClient) Scan(ctx context.Context, cursor uint64, match string, count int64) (uint64, []string, error) {
	if f.scanFn != nil {
		return f.scanFn(ctx, cursor, match, count)
	}
	return 0, nil, nil
}

type fakeAPIKeyQuerier struct {
	fetchFn func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error)
}

func (f *fakeAPIKeyQuerier) FetchAPIKeyList(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, filter)
	}
	return nil, nil
}

// fakeK8sInstanceReader implements iprovider.K8sPoolInstanceReader for the
// realtime-resolution egress path.
type fakeK8sInstanceReader struct {
	instances []iprovider.ProviderInstance
	err       error
}

func (f *fakeK8sInstanceReader) FetchK8sPoolInstances(ctx context.Context, poolName string) ([]iprovider.ProviderInstance, error) {
	return f.instances, f.err
}

type fakeProviderQuerier struct {
	fetchFn func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error)
}

func (f *fakeProviderQuerier) FetchProvider(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, filter)
	}
	return nil, nil
}

type fakePriceQuerier struct {
	fetchFn func(ctx context.Context, filter *imodel_price.ModelPriceFilter) (*imodel_price.ModelPrice, error)
}

func (f *fakePriceQuerier) FetchModelPrice(ctx context.Context, filter *imodel_price.ModelPriceFilter) (*imodel_price.ModelPrice, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, filter)
	}
	return nil, nil
}

type fakeOperationLogRecorder struct {
	mu      sync.Mutex
	entries []*ioperlog.OperationLogEntry
}

func (f *fakeOperationLogRecorder) Record(ctx context.Context, entry *ioperlog.OperationLogEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, entry)
}

func (f *fakeOperationLogRecorder) last() *ioperlog.OperationLogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.entries) == 0 {
		return nil
	}
	return f.entries[len(f.entries)-1]
}

// fakeClock 可注入时钟（对齐 model/quota 测试惯例）。
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

func newTestManager(storager *fakeBatchStorager, redis *fakeRedisClient) *Manager {
	return NewManager(storager, redis,
		&fakeAPIKeyQuerier{}, &fakeProviderQuerier{}, &fakePriceQuerier{},
		nil, &fakeClock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)})
}
