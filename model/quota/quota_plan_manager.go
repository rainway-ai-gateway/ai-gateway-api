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
//limitations under the License.

package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quotacache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

// QuotaPlanManager 定义配额计划管理器
type QuotaPlanManager struct {
	txn                 itxn.TxnStorager
	storager            QuotaPlanStorager
	apiKeyStorager      api_key.APIKeyStorager
	entityStorager      entity.EntityStorager
	quotaCache          quotacache.QuotaCache
	operationLogManager ioperlog.OperationLogRecorder
}

// NewQuotaPlanManager 创建配额计划管理器
func NewQuotaPlanManager(txn itxn.TxnStorager, storager QuotaPlanStorager,
	apiKeyStorager api_key.APIKeyStorager, entityStorager entity.EntityStorager, quotaCache quotacache.QuotaCache) *QuotaPlanManager {
	return &QuotaPlanManager{
		txn:            txn,
		storager:       storager,
		apiKeyStorager: apiKeyStorager,
		entityStorager: entityStorager,
		quotaCache:     quotaCache,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (m *QuotaPlanManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// CreateQuotaPlan 创建配额计划
// owner 为归属资源（嵌套写路径传入所属 Entity/API Key），写入审计日志 resource_parent_id。
func (m *QuotaPlanManager) CreateQuotaPlan(ctx context.Context, param *QuotaPlanParam, owner shared.ResourceOwner) (int64, error) {
	id, err := m.storager.CreateQuotaPlan(ctx, param)
	if err != nil {
		m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionCreate), "", owner.ID, nil, quotaPlanParamToMap(param), err)
		return 0, err
	}

	m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionCreate), quotaPlanIDString(id), owner.ID, nil, quotaPlanParamToMap(param), nil)
	return id, nil
}

// FetchQuotaPlan 查询单个配额计划
func (m *QuotaPlanManager) FetchQuotaPlan(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
	return m.storager.FetchQuotaPlan(ctx, filter)
}

// FetchQuotaPlanList 查询配额计划列表
func (m *QuotaPlanManager) FetchQuotaPlanList(ctx context.Context, filter *QuotaPlanFilter) ([]*QuotaPlanParam, error) {
	return m.storager.FetchQuotaPlanList(ctx, filter)
}

// UpdateQuotaPlan 更新配额计划
// owner 为归属资源（嵌套写路径传入所属 Entity/API Key），写入审计日志 resource_parent_id。
func (m *QuotaPlanManager) UpdateQuotaPlan(ctx context.Context, filter *QuotaPlanFilter, param *QuotaPlanParam, owner shared.ResourceOwner) (int64, error) {
	oldPlan, err := m.storager.FetchQuotaPlan(ctx, filter)
	if err != nil {
		resourceID := ""
		if filter != nil && filter.ID != nil {
			resourceID = quotaPlanIDString(*filter.ID)
		}
		m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionUpdate), resourceID, owner.ID, nil, quotaPlanParamToMap(param), err)
		return 0, err
	}

	affected, err := m.storager.UpdateQuotaPlan(ctx, filter, param)
	if err != nil {
		resourceID := ""
		if filter != nil && filter.ID != nil {
			resourceID = quotaPlanIDString(*filter.ID)
		}
		m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionUpdate), resourceID, owner.ID, quotaPlanParamToMap(oldPlan), quotaPlanParamToMap(param), err)
		return affected, err
	}

	resourceID := ""
	if filter != nil && filter.ID != nil {
		resourceID = quotaPlanIDString(*filter.ID)
	}
	m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionUpdate), resourceID, owner.ID, quotaPlanParamToMap(oldPlan), quotaPlanParamToMap(param), nil)
	return affected, nil
}

// DeleteQuotaPlan 删除配额计划
// owner 为归属资源（嵌套写路径传入所属 Entity/API Key），写入审计日志 resource_parent_id。
func (m *QuotaPlanManager) DeleteQuotaPlan(ctx context.Context, filter *QuotaPlanFilter, owner shared.ResourceOwner) error {
	oldPlan, err := m.storager.FetchQuotaPlan(ctx, filter)
	if err != nil {
		resourceID := ""
		if filter != nil && filter.ID != nil {
			resourceID = quotaPlanIDString(*filter.ID)
		}
		m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionDelete), resourceID, owner.ID, nil, nil, err)
		return err
	}

	if err := m.storager.DeleteQuotaPlan(ctx, filter); err != nil {
		resourceID := ""
		if filter != nil && filter.ID != nil {
			resourceID = quotaPlanIDString(*filter.ID)
		}
		m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionDelete), resourceID, owner.ID, quotaPlanParamToMap(oldPlan), nil, err)
		return err
	}

	resourceID := ""
	if filter != nil && filter.ID != nil {
		resourceID = quotaPlanIDString(*filter.ID)
	}
	m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionDelete), resourceID, owner.ID, quotaPlanParamToMap(oldPlan), nil, nil)
	return nil
}

// ResetBalance 重置配额余额
// updateLastResetAt: 是否更新 last_reset_at 字段
// - true: 用于定期重置调度，会更新 last_reset_at，影响下次重置判断
// - false: 用于手动重置接口，不更新 last_reset_at，避免影响定期重置调度
func (m *QuotaPlanManager) ResetBalance(ctx context.Context, planID int64, newQuota *float64, updateLastResetAt bool, owner shared.ResourceOwner) error {
	var resetQuota *float64
	var planUnit *string
	var oldPlan *QuotaPlanParam

	err := m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		// 1. 获取 QuotaPlan
		plan, err := m.storager.FetchQuotaPlan(ctx, &QuotaPlanFilter{ID: &planID})
		if err != nil {
			return err
		}
		if plan == nil {
			return fmt.Errorf("quota_plan not found")
		}
		oldPlan = plan

		// 2. 如果是无限配额，返回错误（PARAM 语义错误 → 422 Param Illegal，issue #183）
		if plan.Unlimited != nil && *plan.Unlimited {
			return xerror.WrapParamErrorWithMsg("cannot reset balance for unlimited quota")
		}

		// 3. 确定重置后的配额总量
		resetQuota = plan.Quota
		if newQuota != nil {
			resetQuota = newQuota

			_, err = m.storager.UpdateQuotaPlan(ctx, &QuotaPlanFilter{ID: &planID}, &QuotaPlanParam{
				Quota: resetQuota,
			})
			if err != nil {
				return err
			}
		}
		planUnit = plan.Unit

		// 4. 根据参数决定是否更新 quota_plans.last_reset_at
		if updateLastResetAt {
			now := time.Now()
			_, err = m.storager.UpdateQuotaPlan(ctx, &QuotaPlanFilter{ID: &planID}, &QuotaPlanParam{
				LastResetAt: &now,
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		afterMap := quotaPlanParamToMap(oldPlan)
		if afterMap == nil {
			afterMap = map[string]interface{}{}
		}
		if resetQuota != nil {
			afterMap["quota"] = *resetQuota
		}
		if updateLastResetAt {
			now := time.Now()
			afterMap["last_reset_at"] = now
		}
		m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionReset), quotaPlanIDString(planID), owner.ID, quotaPlanParamToMap(oldPlan), afterMap, err)
		return err
	}

	// Record reset operation after transaction commits successfully.
	afterMap := quotaPlanParamToMap(oldPlan)
	if resetQuota != nil {
		afterMap["quota"] = *resetQuota
	}
	if updateLastResetAt {
		now := time.Now()
		afterMap["last_reset_at"] = now
	}
	m.recordQuotaPlanOperation(ctx, string(ioperlog.ActionReset), quotaPlanIDString(planID), owner.ID, quotaPlanParamToMap(oldPlan), afterMap, nil)

	// 5. 重置该 quota_plan 下所有 API-Key / Entity 的 Redis 剩余量（事务外，最终一致）
	if m.quotaCache == nil || resetQuota == nil {
		return nil
	}

	apiKeys, err := m.apiKeyStorager.FetchAPIKeyList(ctx, &api_key.APIKeyFilter{QuotaPlanID: &planID})
	if err != nil {
		stateful.AccessLogger.Warn("failed to fetch api keys for quota plan %d: %v", planID, err)
		return nil
	}
	for _, apiKey := range apiKeys {
		if apiKey.Key == nil {
			continue
		}
		if cacheErr := m.quotaCache.ResetToQuota(ctx, *apiKey.Key, resetQuota, planUnit); cacheErr != nil {
			stateful.AccessLogger.Warn("failed to reset quota cache for api_key %s: %v", *apiKey.Key, cacheErr)
		}
	}

	if m.entityStorager != nil {
		entities, err := m.entityStorager.FetchEntityList(ctx, &entity.EntityFilter{QuotaPlanID: &planID})
		if err != nil {
			stateful.AccessLogger.Warn("failed to fetch entities for quota plan %d: %v", planID, err)
			return nil
		}
		for _, entity := range entities {
			if entity.EntityID == nil {
				continue
			}
			if cacheErr := m.quotaCache.ResetToQuota(ctx, *entity.EntityID, resetQuota, planUnit); cacheErr != nil {
				stateful.AccessLogger.Warn("failed to reset quota cache for entity %s: %v", *entity.EntityID, cacheErr)
			}
		}
	}

	return nil
}

// ApplyQuotaPlanChange 比较新旧配额计划，仅在 quota / unit / unlimited 发生变化时调整余额。
// 该方法供 API-Key / Entity 更新接口调用，避免普通属性修改导致配额使用量被清零。
// newPlan 中未传出的字段（nil）视为“保持原值不变”。
func (m *QuotaPlanManager) ApplyQuotaPlanChange(ctx context.Context, planID int64, oldPlan, newPlan *shared.QuotaPlanParam) error {
	if newPlan == nil {
		return nil
	}
	if !quotaPlanChanged(oldPlan, newPlan) {
		return nil
	}
	return m.adjustQuota(ctx, planID, oldPlan, newPlan)
}

// adjustQuota 在 quota / unit / unlimited 变化时调整 Redis 余额。
// - quota 变化、单位不变：保留 used，remaining = newQuota - used（不钳零，
//   可为负；issue #1476 后 used 可大于旧 quota，负值表示新配额仍不足覆盖已用量）。
// - 新建计划、unit 变化 或 unlimited 切换：used 清零，remaining 置为新配额（unlimited 时用 sentinel）。
func (m *QuotaPlanManager) adjustQuota(ctx context.Context, planID int64, oldPlan, newPlan *shared.QuotaPlanParam) error {
	var (
		newQuota *float64
		oldQuota *float64
		planUnit *string
		useReset bool
	)

	err := m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		plan, err := m.storager.FetchQuotaPlan(ctx, &QuotaPlanFilter{ID: &planID})
		if err != nil {
			return err
		}
		if plan == nil {
			return fmt.Errorf("quota_plan not found")
		}

		oldUnlimited := oldPlan != nil && oldPlan.Unlimited != nil && *oldPlan.Unlimited
		newUnlimited := oldUnlimited
		if newPlan.Unlimited != nil {
			newUnlimited = *newPlan.Unlimited
		}

		oldUnit := ""
		if oldPlan != nil && oldPlan.Unit != nil {
			oldUnit = *oldPlan.Unit
		}
		newUnit := oldUnit
		if newPlan.Unit != nil {
			newUnit = *newPlan.Unit
		}

		newQuota = plan.Quota
		if newPlan.Quota != nil {
			newQuota = newPlan.Quota
		}
		if oldPlan != nil {
			oldQuota = oldPlan.Quota
		}

		if plan.Unit != nil {
			planUnit = plan.Unit
		} else {
			planUnit = &newUnit
		}

		switch {
		case newUnlimited:
			sentinel := float64(100000000)
			newQuota = &sentinel
			useReset = true
		case oldPlan == nil || oldUnlimited || oldUnit != newUnit:
			// 新建配额计划、从无限制切换为有限额、或单位变化：按全新配额重置。
			useReset = true
		default:
			// 配额总额变化，单位不变：保留已使用量，在事务外根据 Redis 实时剩余量计算。
			useReset = false
		}

		return nil
	})
	if err != nil {
		return err
	}

	if m.quotaCache == nil || newQuota == nil {
		return nil
	}

	apiKeys, err := m.apiKeyStorager.FetchAPIKeyList(ctx, &api_key.APIKeyFilter{QuotaPlanID: &planID})
	if err != nil {
		stateful.AccessLogger.Warn("failed to fetch api keys for quota plan %d: %v", planID, err)
		return nil
	}

	entities, err := m.fetchEntitiesByPlan(ctx, planID)
	if err != nil {
		return nil
	}

	oldQuotaValue := float64(0)
	if oldQuota != nil {
		oldQuotaValue = *oldQuota
	}

	if useReset {
		for _, ak := range apiKeys {
			if ak.Key == nil {
				continue
			}
			if cacheErr := m.quotaCache.ResetToQuota(ctx, *ak.Key, newQuota, planUnit); cacheErr != nil {
				stateful.AccessLogger.Warn("failed to reset quota cache for api_key %s: %v", *ak.Key, cacheErr)
			}
		}
		for _, ent := range entities {
			if ent.EntityID == nil {
				continue
			}
			if cacheErr := m.quotaCache.ResetToQuota(ctx, *ent.EntityID, newQuota, planUnit); cacheErr != nil {
				stateful.AccessLogger.Warn("failed to reset quota cache for entity %s: %v", *ent.EntityID, cacheErr)
			}
		}
		return nil
	}

	// quota 变化且单位不变：批量读取当前剩余量，按新配额保留 used 后下发。
	allKeys := make([]string, 0, len(apiKeys)+len(entities))
	keyOwners := make([]struct {
		key      string
		isAPIKey bool
	}, 0, len(apiKeys)+len(entities))
	for _, ak := range apiKeys {
		if ak.Key == nil {
			continue
		}
		allKeys = append(allKeys, *ak.Key)
		keyOwners = append(keyOwners, struct {
			key      string
			isAPIKey bool
		}{key: *ak.Key, isAPIKey: true})
	}
	for _, ent := range entities {
		if ent.EntityID == nil {
			continue
		}
		allKeys = append(allKeys, *ent.EntityID)
		keyOwners = append(keyOwners, struct {
			key      string
			isAPIKey bool
		}{key: *ent.EntityID, isAPIKey: false})
	}

	remainMap, err := m.quotaCache.BatchGetRemaining(ctx, allKeys, planUnit)
	if err != nil {
		stateful.AccessLogger.Warn("failed to batch get remaining for quota plan %d: %v", planID, err)
		return nil
	}

	for _, owner := range keyOwners {
		currentRemaining := remainMap[owner.key]
		// remaining 可为负（issue #1476）：used 如实反映实际消耗、可大于
		// oldQuota；targetRemaining = newQuota - used 允许为负，负值表示新
		// 配额仍不足以覆盖已用量（BFE 鉴权继续拒绝）。不再钳零，否则欠费
		// 会在调额时被凭空抹掉。
		used := oldQuotaValue - currentRemaining
		if used < 0 {
			used = 0 // 防御：数据异常时兜底
		}
		targetRemaining := *newQuota - used
		if cacheErr := m.quotaCache.SetRemaining(ctx, owner.key, &targetRemaining, planUnit); cacheErr != nil {
			stateful.AccessLogger.Warn("failed to set quota cache for owner %s: %v", owner.key, cacheErr)
		}
	}

	return nil
}

// fetchEntitiesByPlan 获取指定配额计划下的所有 Entity。
func (m *QuotaPlanManager) fetchEntitiesByPlan(ctx context.Context, planID int64) ([]*entity.EntityParam, error) {
	if m.entityStorager == nil {
		return nil, nil
	}
	entities, err := m.entityStorager.FetchEntityList(ctx, &entity.EntityFilter{QuotaPlanID: &planID})
	if err != nil {
		stateful.AccessLogger.Warn("failed to fetch entities for quota plan %d: %v", planID, err)
		return nil, err
	}
	return entities, nil
}

// quotaPlanChanged 判断两个配额计划是否在影响余额的字段上存在差异。
// 仅当 newPlan 显式传入了对应字段时才进行比较；未传出的字段视为不变。
func quotaPlanChanged(oldPlan, newPlan *shared.QuotaPlanParam) bool {
	if oldPlan == nil || newPlan == nil {
		return true
	}
	if newPlan.Unlimited != nil && !ptrBoolEqual(oldPlan.Unlimited, newPlan.Unlimited) {
		return true
	}
	if newPlan.Quota != nil && !ptrFloat64Equal(oldPlan.Quota, newPlan.Quota) {
		return true
	}
	if newPlan.Unit != nil && !ptrStringEqual(oldPlan.Unit, newPlan.Unit) {
		return true
	}
	return false
}

func ptrBoolEqual(a, b *bool) bool {
	av := false
	if a != nil {
		av = *a
	}
	bv := false
	if b != nil {
		bv = *b
	}
	return av == bv
}

func ptrFloat64Equal(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ptrStringEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
