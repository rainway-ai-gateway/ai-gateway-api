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
	"net/http"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/imodel_price"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quota"
)

// Manager 批量任务管控管理器。依赖全部构造注入（AGENTS.md）：
// storager（DB）、redis（BATCH_* 键最小抽象，nil = 无 Redis 降级）、
// apiKeyQuerier（解析 product/entity）、providerQuerier（cancel/兜底结算
// 出网拼地址取 key）、priceQuerier（mode=batch 批量价）、httpClientFactory
// （出网 client，便于 mock）、clock。进程内不保留批量任务状态。
type Manager struct {
	storager             BatchStorager
	redis                RedisClient
	apiKeyQuerier        APIKeyQuerier
	providerQuerier      ProviderQuerier
	priceQuerier         BatchPriceQuerier
	httpClientFactory    func() *http.Client
	clock                quota.Clock
	operationLogManager  ioperlog.OperationLogRecorder
	instancePoolResolver *iprovider.InstancePoolResolver

	providerTimeout    time.Duration
	settleGrace        time.Duration
	advanceLimit       int
	reconcileLimit     int
	scanCount          int64
	maxOutputLineBytes int64
}

// ManagerOption 定制 Manager 可调参数。
type ManagerOption func(*Manager)

// WithProviderTimeout 控制面直调 provider（cancel/状态推进/兜底结算下载）
// 的出网超时。默认 10s。
func WithProviderTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) {
		if d > 0 {
			m.providerTimeout = d
		}
	}
}

// WithSettleGrace 兜底结算宽限：completed 进入终态超过该时长且仍为
// reserved 才触发 job 结算（给下载拦截结算让路，宽限内重复触发由
// BATCH_SETTLED 幂等收敛）。默认 30min。
func WithSettleGrace(d time.Duration) ManagerOption {
	return func(m *Manager) {
		if d > 0 {
			m.settleGrace = d
		}
	}
}

// WithAdvanceLimit 状态推进每轮处理的任务上限。默认 50。
func WithAdvanceLimit(n int) ManagerOption {
	return func(m *Manager) {
		if n > 0 {
			m.advanceLimit = n
		}
	}
}

// WithReconcileLimit 兜底结算每轮处理的任务上限。默认 50。
func WithReconcileLimit(n int) ManagerOption {
	return func(m *Manager) {
		if n > 0 {
			m.reconcileLimit = n
		}
	}
}

// WithScanCount Redis SCAN 每批 COUNT。默认 200。
func WithScanCount(n int64) ManagerOption {
	return func(m *Manager) {
		if n > 0 {
			m.scanCount = n
		}
	}
}

// WithMaxOutputLineBytes 兜底结算流式解析输出文件的单行缓冲上限。
// 默认 4MB；超长行丢弃并告警（与数据面 jsonl 扫描器同策略）。
func WithMaxOutputLineBytes(n int64) ManagerOption {
	return func(m *Manager) {
		if n > 0 {
			m.maxOutputLineBytes = n
		}
	}
}

const (
	defaultProviderTimeout    = 10 * time.Second
	defaultSettleGrace        = 30 * time.Minute
	defaultAdvanceLimit       = 50
	defaultReconcileLimit     = 50
	defaultScanCount          = 200
	defaultMaxOutputLineBytes = 4 << 20
)

// NewManager 创建批量任务管理器。nil redis 表示降级部署（无 Redis 时
// 回源/同步/释放/结算全部跳过，仅 DB 查询可用）；nil httpClientFactory
// 使用 http.DefaultClient（请求级 context 超时兜底）。
func NewManager(storager BatchStorager, redis RedisClient, apiKeyQuerier APIKeyQuerier, providerQuerier ProviderQuerier, priceQuerier BatchPriceQuerier, httpClientFactory func() *http.Client, clock quota.Clock, opts ...ManagerOption) *Manager {
	m := &Manager{
		storager:           storager,
		redis:              redis,
		apiKeyQuerier:      apiKeyQuerier,
		providerQuerier:    providerQuerier,
		priceQuerier:       priceQuerier,
		httpClientFactory:  httpClientFactory,
		clock:              clock,
		providerTimeout:    defaultProviderTimeout,
		settleGrace:        defaultSettleGrace,
		advanceLimit:       defaultAdvanceLimit,
		reconcileLimit:     defaultReconcileLimit,
		scanCount:          defaultScanCount,
		maxOutputLineBytes: defaultMaxOutputLineBytes,
	}
	if m.httpClientFactory == nil {
		m.httpClientFactory = func() *http.Client { return http.DefaultClient }
	}
	if m.clock == nil {
		m.clock = quota.NewRealClock()
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// SetOperationLogManager 注入审计记录器（cancel 写 operation_logs）。
func (m *Manager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// SetInstancePoolResolver 注入有效池解析器（k8s_pool 模式出网时按
// k8s_pool_name 实时解析实例）。
func (m *Manager) SetInstancePoolResolver(resolver *iprovider.InstancePoolResolver) {
	m.instancePoolResolver = resolver
}

// BatchManager 是 endpoints 层面向的批量任务管控接口（*Manager 满足）。
// 定义为接口以便端点测试用 fake 替换 container.BatchManager。
type BatchManager interface {
	FetchTask(ctx context.Context, batchID, productName string, allowRedisFallback bool) (*BatchTask, error)
	FetchFile(ctx context.Context, fileID, provider, productName string, allowRedisFallback bool) (*BatchFile, error)
	ListTasks(ctx context.Context, filter *BatchTaskFilter) (*BatchTaskListResult, error)
	Cancel(ctx context.Context, batchID, productName string) (*BatchTask, error)
	SetOperationLogManager(manager ioperlog.OperationLogRecorder)
}

var _ BatchManager = (*Manager)(nil)

// FetchTask 查询任务详情：DB 优先（batch_id + product_name 强制）；
// miss 且允许回源时读 Redis BATCH_TASK hash 组装（覆盖对账 job 落库前
// 窗口）。均 miss 返回 (nil, nil)。Redis 回源任务经 apiKey 解析校验
// product_name（不匹配按 miss 处理，保持 product_name 过滤口径一致）。
func (m *Manager) FetchTask(ctx context.Context, batchID, productName string, allowRedisFallback bool) (*BatchTask, error) {
	task, err := m.storager.FetchBatchTask(ctx, &BatchTaskFilter{
		BatchID:     &batchID,
		ProductName: &productName,
	})
	if err != nil {
		return nil, err
	}
	if task != nil {
		return task, nil
	}
	if !allowRedisFallback || m.redis == nil {
		return nil, nil
	}

	vals, err := m.redis.HGetAll(ctx, BatchTaskKey(batchID))
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}
	view, err := ParseRedisTaskView(batchID, vals)
	if err != nil {
		return nil, nil
	}
	apiKeys := newAPIKeyResolveCache()
	resolved, err := m.applyTaskView(ctx, apiKeys, view)
	if err != nil {
		return nil, err
	}
	if resolved == nil || resolved.ProductName != productName {
		return nil, nil
	}
	return resolved, nil
}

// FetchFile 查询文件元数据：DB 优先（file_id + provider，product 强制）；
// miss 且允许回源时读 Redis BATCH_FILE hash。均 miss 返回 (nil, nil)。
func (m *Manager) FetchFile(ctx context.Context, fileID, provider, productName string, allowRedisFallback bool) (*BatchFile, error) {
	file, err := m.storager.FetchBatchFile(ctx, &BatchFileFilter{
		FileID:      &fileID,
		Provider:    &provider,
		ProductName: &productName,
	})
	if err != nil {
		return nil, err
	}
	if file != nil {
		return file, nil
	}
	if !allowRedisFallback || m.redis == nil {
		return nil, nil
	}

	vals, err := m.redis.HGetAll(ctx, BatchFileKey(provider, fileID))
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}
	view := ParseRedisFileView(provider, fileID, vals)
	apiKeys := newAPIKeyResolveCache()
	resolved, err := m.applyFileView(ctx, apiKeys, view)
	if err != nil {
		return nil, err
	}
	if resolved == nil || resolved.ProductName != productName {
		return nil, nil
	}
	return resolved, nil
}

// ListTasks 批量任务列表：过滤 + 主键游标分页。product_name 强制——
// 缺失返回参数错误（产品线隔离维度不接受外部空值）。
func (m *Manager) ListTasks(ctx context.Context, filter *BatchTaskFilter) (*BatchTaskListResult, error) {
	if filter == nil {
		filter = &BatchTaskFilter{}
	}
	if filter.ProductName == nil || *filter.ProductName == "" {
		return nil, xerror.WrapParamErrorWithMsg("product_name is required")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	filter.Limit = limit

	tasks, err := m.storager.ListBatchTasks(ctx, filter)
	if err != nil {
		return nil, err
	}

	result := &BatchTaskListResult{Tasks: tasks}
	if len(tasks) >= limit && limit > 0 {
		result.NextCursor = tasks[len(tasks)-1].ID
	}
	return result, nil
}

// apiKeyResolveCache 单次调用内的 api_key 解析缓存
// （Redis SCAN 同步时同一 api_key_id 高频复用）。
type apiKeyResolveCache struct {
	cache map[string]*apiKeyLookup
}

type apiKeyLookup struct {
	key *api_key.APIKeyParam
}

func newAPIKeyResolveCache() *apiKeyResolveCache {
	return &apiKeyResolveCache{cache: map[string]*apiKeyLookup{}}
}

// resolve 查询 api_key（按 ID）。未找到返回 (nil, nil)；ctx 取消或 DB
// 错误返回 error（调用方决定 fail-open 跳过或中止）。
func (c *apiKeyResolveCache) resolve(ctx context.Context, querier APIKeyQuerier, apiKeyID string) (*api_key.APIKeyParam, error) {
	if hit, ok := c.cache[apiKeyID]; ok {
		return hit.key, nil
	}
	keys, err := querier.FetchAPIKeyList(ctx, &api_key.APIKeyFilter{ID: &apiKeyID})
	if err != nil {
		return nil, err
	}
	var found *api_key.APIKeyParam
	if len(keys) > 0 {
		found = keys[0]
	}
	c.cache[apiKeyID] = &apiKeyLookup{key: found}
	return found, nil
}

// ensure interface satisfactions at compile time.
var (
	_ ProviderQuerier   = (*iprovider.ProviderManager)(nil)
	_ BatchPriceQuerier = (*imodel_price.Manager)(nil)
)
