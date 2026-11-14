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
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

// Cancel 取消批量任务（批量任务与对账.md §6 五步流程）：
//
//  1. 查任务：DB → Redis BATCH_TASK 回源，均 miss 返回 ErrBatchNotFound；
//  2. DB 条件更新抢占 SET status='cancelling'
//     WHERE batch_id=? AND product_name=? AND status NOT IN (终态, cancelling)，
//     affected=0 时按当前状态判别：已终态 → 409；已 cancelling → 200 幂等
//     重放（见 cancelReplay；仅首个转换者成功，双方言一致性见
//     storage/rdb/batch.PreemptBatchTaskCancel 注释）；
//  3. 取 provider 记录与 key_name 对应明文 key，实例地址 +
//     protocol_paths.openai 拼 cancel URL，出网 POST（超时控制）；
//  4. 成功（2xx）后幂等释放 Redis 预留（BATCH_RESERVE_BATCH 簿记冲正，
//     已清零则跳过）；provider 失败不回滚 cancelling——由状态推进
//     （AdvanceStatus）经 provider 对账收敛终态；
//  5. operation_logs 审计（change_summary 带 batch_id 与前后状态）。
//
// 抢占在出网之前：任务已终态时快速 409、避免无谓出网。竞态由
// BATCH_SETTLED（结算去重）+ 预留清零（释放去重）+ settle_status 条件
// 更新三道防线收敛（§6.2）。
func (m *Manager) Cancel(ctx context.Context, batchID, productName string) (*BatchTask, error) {
	task, err := m.FetchTask(ctx, batchID, productName, true)
	if err != nil {
		m.recordCancelOperation(ctx, batchID, "", err)
		return nil, err
	}
	if task == nil {
		err := ErrBatchNotFound
		m.recordCancelOperation(ctx, batchID, "", err)
		return nil, err
	}
	prevStatus := task.Status

	affected, err := m.storager.PreemptBatchTaskCancel(ctx, batchID, productName)
	if err != nil {
		m.recordCancelOperation(ctx, batchID, prevStatus, err)
		return nil, err
	}
	if affected == 0 {
		// Redis 回源窗口：DB 无行时先按幂等键补登（sync_source=redis）
		// 再抢占一次，消除"任务在途但未落库"的 409 窗口；仍 affected=0
		// 才是真正的终态/竞态冲突。
		if task.ID == 0 {
			if _, upsertErr := m.storager.UpsertBatchTask(ctx, task); upsertErr == nil {
				affected, err = m.storager.PreemptBatchTaskCancel(ctx, batchID, productName)
			}
		}
		if err != nil {
			m.recordCancelOperation(ctx, batchID, prevStatus, err)
			return nil, err
		}
		if affected == 0 {
			// 抢占失败区分"已终态"（409）与"已 cancelling"（幂等重放 200，
			// issue #274）：cancelling 可能来自首次抢占成功后出网失败/进程
			// 崩溃的残留——重放重发 provider cancel 恢复取消意图落地。
			if cur, ferr := m.FetchTask(ctx, batchID, productName, false); ferr == nil &&
				cur != nil && cur.Status == TaskStatusCancelling {
				return m.cancelReplay(ctx, cur)
			}
			err := errBatchConflict(batchID)
			m.recordCancelOperation(ctx, batchID, prevStatus, err)
			return nil, err
		}
	}

	access, err := m.resolveProviderAccess(ctx, task.Provider, task.KeyName)
	if err != nil {
		m.recordCancelOperation(ctx, batchID, prevStatus, err)
		return nil, err
	}

	cancelURL := access.baseURL + access.path + "/batches/" + batchID + "/cancel"
	if err := m.postProviderCancel(ctx, access, cancelURL); err != nil {
		m.recordCancelOperation(ctx, batchID, prevStatus, err)
		return nil, err
	}

	if m.redis != nil {
		if err := m.releaseReserve(ctx, batchID); err != nil {
			// 释放失败不使 cancel 失败：幂等语义保证重试（下个 tick 的
			// 对账释放路径）不会双倍冲正。
			stateful.AccessLogger.Warn("ibatch: cancel release reserve failed, batch=%s: %v", batchID, err)
		}
	}

	task.Status = TaskStatusCancelling
	m.recordCancelOperation(ctx, batchID, prevStatus, nil)
	return task, nil
}

// cancelReplay 已 cancelling 任务的幂等重放（issue #274）：重发 provider
// cancel 保证取消意图落地（provider cancel 幂等）；预留释放为幂等 no-op
// （簿记已删），不新增 release/charge/settlement transition；出网失败按原错误
// 映射返回（502/超时），状态保持 cancelling，由下轮重放或对账补发恢复。
func (m *Manager) cancelReplay(ctx context.Context, task *BatchTask) (*BatchTask, error) {
	access, err := m.resolveProviderAccess(ctx, task.Provider, task.KeyName)
	if err != nil {
		m.recordCancelOperation(ctx, task.BatchID, TaskStatusCancelling, err)
		return nil, err
	}
	cancelURL := access.baseURL + access.path + "/batches/" + task.BatchID + "/cancel"
	if err := m.postProviderCancel(ctx, access, cancelURL); err != nil {
		m.recordCancelOperation(ctx, task.BatchID, TaskStatusCancelling, err)
		return nil, err
	}
	if m.redis != nil {
		if err := m.releaseReserve(ctx, task.BatchID); err != nil {
			// 释放失败不使重放失败：幂等语义保证下轮冲正不双倍。
			stateful.AccessLogger.Warn("ibatch: cancel replay release reserve failed, batch=%s: %v", task.BatchID, err)
		}
	}
	task.Status = TaskStatusCancelling
	m.recordCancelOperation(ctx, task.BatchID, TaskStatusCancelling, nil)
	return task, nil
}

// postProviderCancel 出网 POST provider cancel 端点。
// 错误语义：网络错/超时/非 2xx（404 除外）→ ErrUpstreamUnreachable；
// provider 404 → ErrUpstreamBatchNotFound（透传 provider 语义）。
func (m *Manager) postProviderCancel(ctx context.Context, access *providerAccess, url string) error {
	reqCtx, cancel := context.WithTimeout(ctx, m.providerTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	if access.authHeader != "" {
		req.Header.Set(access.authHeader, access.authValue)
	}

	resp, err := m.httpClientFactory().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstreamUnreachable, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusNotFound {
		return ErrUpstreamBatchNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: provider POST %s returned %d", ErrUpstreamUnreachable, url, resp.StatusCode)
	}
	return nil
}

// providerAccess 是出网访问 provider 的解析结果（cancel / 状态推进 /
// 兜底结算共用）：baseURL（scheme://host:port）、协议路径前缀
// （protocol_paths.openai，规范化后无尾斜杠）与认证头。
// authHeader 为空表示 keyless provider——出网请求不携带鉴权头（issue #274）。
type providerAccess struct {
	baseURL    string
	path       string
	authHeader string
	authValue  string
}

// resolveProviderAccess 解析 provider 出网三要素。provider 记录不存在、
// 无可用实例、配置了 key 但无可用明文时返回 errProviderUnavailable（控制
// 面数据异常）。provider keys 为空数组是合法 keyless 配置（providers.md
// keys 条款：非必填、默认 []；clusters.md：空数组 = 不配置 API-Key），此时
// 不携带鉴权头出网，与数据面 keyless cluster 转发语义一致（issue #274）。
// key 在 storage 层已完成信封解密（providers.keys 读出即明文）。
func (m *Manager) resolveProviderAccess(ctx context.Context, providerName, keyName string) (*providerAccess, error) {
	if m.providerQuerier == nil {
		return nil, errProviderUnavailable(providerName)
	}
	provider, err := m.providerQuerier.FetchProvider(ctx, &iprovider.ProviderFilter{Name: &providerName})
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, errProviderUnavailable(providerName)
	}

	inst, err := m.pickProviderInstance(ctx, provider)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, errProviderUnavailable(providerName)
	}

	access := &providerAccess{
		baseURL: buildInstanceBaseURL(*inst),
		path:    "/v1",
	}
	if p, ok := provider.ProtocolPaths["openai"]; ok && strings.TrimSpace(p) != "" {
		access.path = normalizeProviderPath(p)
	}

	if len(provider.Keys) == 0 {
		// keyless provider：不携带鉴权头出网。
		return access, nil
	}
	key := pickProviderKey(provider, keyName)
	if key == nil {
		return nil, errProviderUnavailable(providerName)
	}
	access.authHeader, access.authValue = iprovider.BuildAuthHeader("openai", key.Key)
	return access, nil
}

// releaseReserve 幂等释放批量预留：读 BATCH_RESERVE_BATCH:<batch_id>
// 簿记（planRedisKey -> units），逐 plan 原子扣减 BATCH_RESERVE 镜像
// （钳制不为负），最后删除簿记键。簿记键不存在即"已清零"，直接返回
// ——重复调用、cancel×job、cancel×cancel 均收敛（§6.2 释放去重锁）。
// 注意：失败应 fail-open（调用方告警），不得中断主流程。
func (m *Manager) releaseReserve(ctx context.Context, batchID string) error {
	bookKey := BatchReserveBatchKey(batchID)
	vals, err := m.redis.HGetAll(ctx, bookKey)
	if err != nil {
		return err
	}
	if len(vals) == 0 {
		return nil
	}
	for planKey, unitsStr := range vals {
		units, err := strconv.ParseInt(unitsStr, 10, 64)
		if err != nil || units <= 0 {
			continue
		}
		if _, err := m.redis.ReserveDecr(ctx, BatchReserveMirrorKey(planKey), units); err != nil {
			stateful.AccessLogger.Warn("ibatch: release reserve mirror failed, plan=%s batch=%s: %v", planKey, batchID, err)
		}
	}
	return m.redis.Delete(ctx, bookKey)
}

// pickProviderInstance 选取第一个未禁用的有效实例。有效实例经
// InstancePoolResolver 实时解析：手工来源读 instance_pool，k8s 来源按
// k8s_pool_name 实时查 k8s_pools（单一事实源，无镜像）。
func (m *Manager) pickProviderInstance(ctx context.Context,
	provider *iprovider.Provider) (*iprovider.ProviderInstance, error) {

	if provider == nil {
		return nil, nil
	}
	instances, err := m.instancePoolResolver.Effective(ctx, provider)
	if err != nil {
		return nil, xerror.WrapModelError(err)
	}
	for _, inst := range instances {
		if inst.Disable {
			continue
		}
		if inst.Addr == "" || inst.Port <= 0 {
			continue
		}
		return &inst, nil
	}
	return nil, nil
}

// pickProviderKey 按 key_name 选取 provider key；key_name 为空或匹配
// 缺失时回退第一个可用 key。
func pickProviderKey(provider *iprovider.Provider, keyName string) *iprovider.ProviderKey {
	for i := range provider.Keys {
		if keyName != "" && provider.Keys[i].Name == keyName {
			return &provider.Keys[i]
		}
	}
	for i := range provider.Keys {
		if provider.Keys[i].Key != "" {
			return &provider.Keys[i]
		}
	}
	return nil
}

// buildInstanceBaseURL 由实例地址拼 base URL。Addr 自带 scheme 前缀时
// 原样使用（防御配置）；否则按 BFE 后端默认 http 拼 host:port。
func buildInstanceBaseURL(inst iprovider.ProviderInstance) string {
	addr := inst.Addr
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return strings.TrimRight(addr, "/")
	}
	return "http://" + net.JoinHostPort(addr, strconv.Itoa(inst.Port))
}

// normalizeProviderPath 规范化 protocol_paths 前缀：保证前导斜杠、
// 去除尾斜杠（拼接 /batches/{id} 时使用）。
func normalizeProviderPath(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}
