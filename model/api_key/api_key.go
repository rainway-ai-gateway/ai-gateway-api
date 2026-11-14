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

package api_key

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quotacache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

// APIKeyParam defines the parameters for API key operations
type APIKeyParam struct {
	ID          *string    `json:"id"`
	Enable      *bool      `json:"enabled"`
	Status      *string    `json:"status,omitempty"`
	CreateTime  *int64     `json:"create_time,omitempty"`
	UpdatedTime *int64     `json:"update_time,omitempty"`
	KeyCreateAt *time.Time `json:"-"`

	// Key is the actual API key string, format: product line name + multiple randomly generated segments
	Key *string `json:"key"`

	// Description is the API key description
	Description *string `json:"description,omitempty"`

	// UnlimitedQuota indicates whether quota is unlimited
	UnlimitedQuota *bool `json:"unlimited_quota,omitempty"`

	// ExpiredTime defines the expiration time as Unix timestamp (seconds)
	// -1: Never expires
	// Other values: Unix timestamp
	ExpiredTime       *int64   `json:"expired_time,omitempty"`
	Models            []string `json:"models,omitempty"`
	Subnet            []string `json:"subnet,omitempty"`
	EntityID          *string  `json:"entity_id,omitempty"`
	QuotaPlanID       *int64   `json:"-"`
	RateLimitPolicyID *int64   `json:"-"`
	RouteRulesID      *int64   `json:"-"`
	ProductName       *string  `json:"-"`
	InnerID           *int64   `json:"-"`
	RemainingQuota    *float64 `json:"remaining_quota,omitempty"`

	QuotaPlan       *shared.QuotaPlanParam       `json:"quota_plan,omitempty"`
	RateLimitPolicy *shared.RateLimitPolicyParam `json:"rate_limit_policy,omitempty"`
	RouteRules      *shared.RouteRulesParam      `json:"route_rules,omitempty"`
	Entity          *shared.EntitySummary        `json:"entity,omitempty"`
}

// APIKeyTokenParam defines parameters for API key token operations
type APIKeyTokenParam struct {
	Key       *string
	CreatedAt *time.Time
}

// APIKeyTokenFilter defines filters for querying API key tokens
type APIKeyTokenFilter struct {
	Key *string
	ID  *int64
}

// APIKeyFilter defines filters for querying API keys
type APIKeyFilter struct {
	ProductName    *string
	ProductNames   []string
	ALBGroupName   *string
	ID             *string
	Key            *string
	InnerID        *int64
	QuotaPlanID    *int64
	RouteRulesID   *int64
	Page           *int
	PageSize       *int
	Enabled        *bool
	EntityID       *string
	UnlimitedQuota *bool
}

// APIKeyStorager interface defines storage operations for API keys
type APIKeyStorager interface {
	FetchAPIKeyList(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error)
	CreateAPIKey(ctx context.Context, param *APIKeyParam) (int64, error)
	UpdateAPIKey(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error)
	DeleteAPIKey(ctx context.Context, filter *APIKeyFilter) error

	CreateAPIKeyToken(ctx context.Context, param *APIKeyTokenParam) (int64, error)
	UpdateAPIKeyToken(ctx context.Context, filter *APIKeyTokenFilter, param *APIKeyTokenParam) error
	FetchAPIKeyTokenList(ctx context.Context, filter *APIKeyTokenFilter) ([]*APIKeyTokenParam, error)
}

// APIKeyManager manages API key operations with transaction support
type APIKeyManager struct {
	storager                APIKeyStorager
	txn                     itxn.TxnStorager
	quotaPlanStorager       QuotaPlanStorager
	rateLimitPolicyStorager RateLimitPolicyStorager
	routeRulesStorager      shared.RouteRulesStorager
	entityStorager          shared.EntityStorager
	quotaCache              quotacache.QuotaCache
	operationLogManager     ioperlog.OperationLogRecorder
	quotaPlanAuditor        shared.QuotaPlanAuditor
	rateLimitPolicyAuditor  shared.RateLimitPolicyAuditor
}

// QuotaPlanStorager interface defines storage operations for quota plans
type QuotaPlanStorager interface {
	CreateQuotaPlan(ctx context.Context, param *shared.QuotaPlanParam) (int64, error)
	UpdateQuotaPlan(ctx context.Context, id int64, param *shared.QuotaPlanParam) (int64, error)
	DeleteQuotaPlan(ctx context.Context, id int64) error
	FetchQuotaPlan(ctx context.Context, id int64) (*shared.QuotaPlanParam, error)
}

// RateLimitPolicyStorager interface defines storage operations for rate limit policies
type RateLimitPolicyStorager interface {
	CreateRateLimitPolicy(ctx context.Context, param *shared.RateLimitPolicyParam) (int64, error)
	UpdateRateLimitPolicy(ctx context.Context, id int64, param *shared.RateLimitPolicyParam) (int64, error)
	DeleteRateLimitPolicy(ctx context.Context, id int64) error
	FetchRateLimitPolicy(ctx context.Context, id int64) (*shared.RateLimitPolicyParam, error)
}

// NewAPIKeyManager creates a new APIKeyManager instance
func NewAPIKeyManager(txn itxn.TxnStorager, storager APIKeyStorager,
	quotaPlanStorager QuotaPlanStorager, rateLimitPolicyStorager RateLimitPolicyStorager,
	routeRulesStorager shared.RouteRulesStorager, entityStorager shared.EntityStorager,
	quotaCache quotacache.QuotaCache) *APIKeyManager {
	return &APIKeyManager{
		txn:                     txn,
		storager:                storager,
		quotaPlanStorager:       quotaPlanStorager,
		rateLimitPolicyStorager: rateLimitPolicyStorager,
		routeRulesStorager:      routeRulesStorager,
		entityStorager:          entityStorager,
		quotaCache:              quotaCache,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (rppm *APIKeyManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	rppm.operationLogManager = manager
}

// SetQuotaPlanAuditor injects the auditor that records nested quota-plan
// operation logs (issue #161). Nil disables the nested audit entries.
func (rppm *APIKeyManager) SetQuotaPlanAuditor(auditor shared.QuotaPlanAuditor) {
	rppm.quotaPlanAuditor = auditor
}

// SetRateLimitPolicyAuditor injects the auditor that records nested
// rate-limit-policy operation logs (issue #161). Nil disables the entries.
func (rppm *APIKeyManager) SetRateLimitPolicyAuditor(auditor shared.RateLimitPolicyAuditor) {
	rppm.rateLimitPolicyAuditor = auditor
}

// GetRemainingQuota calculates the remaining quota for an API key.
// Redis stores RMB quotas as fixed-point integers (quota * 1e8) to avoid Lua floating point errors.
func GetRemainingQuota(ctx context.Context, quotaCache quotacache.QuotaCache, param *APIKeyParam) (*float64, error) {
	if param.UnlimitedQuota != nil && *param.UnlimitedQuota {
		return nil, nil
	}

	// If the associated quota plan is unlimited, there is no remaining quota to track.
	if param.QuotaPlan != nil && param.QuotaPlan.Unlimited != nil && *param.QuotaPlan.Unlimited {
		return nil, nil
	}

	if param.QuotaPlan == nil || param.QuotaPlan.Quota == nil {
		return nil, nil
	}

	if quotaCache == nil {
		return param.QuotaPlan.Quota, nil
	}

	remain, err := quotaCache.GetRemaining(ctx, *param.Key, param.QuotaPlan.Unit)
	if err != nil {
		return nil, fmt.Errorf("get %s-%d from cache is error:%s", *param.Key, param.KeyCreateAt.Unix(), err.Error())
	}

	if remain < 0 {
		remain = 0
	}

	return &remain, nil
}

// FetchAPIKeyList retrieves API keys based on filter criteria
func (rppm *APIKeyManager) FetchAPIKeyList(ctx context.Context,
	filter *APIKeyFilter) (list []*APIKeyParam, err error) {
	err = rppm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		list, err = rppm.storager.FetchAPIKeyList(ctx, filter)
		if err != nil {
			return err
		}
		for _, one := range list {
			if err := rppm.populateAssociatedData(ctx, one); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 事务外批量从 Redis 读取实时余额（最终一致，失败不影响主数据返回）。
	if err := rppm.populateQuotaBalances(ctx, list); err != nil {
		stateful.AccessLogger.Warn("failed to populate quota balances for api key list: %v", err)
	}

	return
}

// FetchAPIKey retrieves a single API key based on filter criteria
func (rppm *APIKeyManager) FetchAPIKey(ctx context.Context,
	filter *APIKeyFilter) (one *APIKeyParam, err error) {
	err = rppm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		list, err := rppm.storager.FetchAPIKeyList(ctx, filter)
		if err != nil {
			return err
		}
		if len(list) > 0 {
			one = list[0]
			return rppm.populateAssociatedData(ctx, one)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 事务外从 Redis 读取实时余额（最终一致，失败不影响主数据返回）。
	if one != nil {
		if err := rppm.populateQuotaBalance(ctx, one); err != nil {
			stateful.AccessLogger.Warn("failed to populate quota balance for api key: %v", err)
		}
	}

	return
}

func (rppm *APIKeyManager) populateAssociatedData(ctx context.Context, one *APIKeyParam) error {
	if one.QuotaPlan == nil {
		one.QuotaPlan = &shared.QuotaPlanParam{}
	}
	if one.RateLimitPolicy == nil {
		one.RateLimitPolicy = &shared.RateLimitPolicyParam{}
	}
	if one.RouteRules == nil {
		one.RouteRules = &shared.RouteRulesParam{}
	}
	if one.RouteRules.Enabled == nil {
		enabled := false
		one.RouteRules.Enabled = &enabled
	}
	if one.RouteRules.Rules == nil {
		one.RouteRules.Rules = []*shared.AiRouteRuleParam{}
	}

	if one.QuotaPlanID != nil && rppm.quotaPlanStorager != nil {
		quotaPlan, err := rppm.quotaPlanStorager.FetchQuotaPlan(ctx, *one.QuotaPlanID)
		if err != nil {
			return err
		}
		if quotaPlan != nil {
			one.QuotaPlan = quotaPlan
		}
	}

	if one.RateLimitPolicyID != nil && rppm.rateLimitPolicyStorager != nil {
		rateLimitPolicy, err := rppm.rateLimitPolicyStorager.FetchRateLimitPolicy(ctx, *one.RateLimitPolicyID)
		if err != nil {
			return err
		}
		if rateLimitPolicy != nil {
			one.RateLimitPolicy = rateLimitPolicy
		}
	}

	if one.RouteRulesID != nil && rppm.routeRulesStorager != nil {
		routeRules, err := rppm.routeRulesStorager.FetchRouteRulesByID(ctx, *one.RouteRulesID)
		if err != nil {
			return err
		}
		if routeRules != nil {
			one.RouteRules = routeRules
		}
	}

	if one.EntityID != nil && rppm.entityStorager != nil {
		entity, err := rppm.entityStorager.FetchEntity(ctx, &shared.EntityFilter{EntityID: one.EntityID})
		if err != nil {
			return err
		}
		one.Entity = entity
	}

	return nil
}

const unlimitedSentinel = float64(100000000)

// fillQuotaBalance 根据 Redis 实时剩余量填充 quotaPlan.Balance。
// remaining 可为负（issue #1476：BFE 全额扣减，欠费保留在余额中）：
// used = quota - remaining 如实反映实际消耗、允许大于 quota；
// 对外展示的 Remaining 钳 0。
func fillQuotaBalance(quotaPlan *shared.QuotaPlanParam, remaining float64) {
	if quotaPlan == nil || quotaPlan.Quota == nil {
		return
	}
	used := *quotaPlan.Quota - remaining
	if used < 0 {
		used = 0 // 防御：数据异常（remaining > quota）时兜底
	}
	displayRemaining := remaining
	if displayRemaining < 0 {
		displayRemaining = 0
	}
	if quotaPlan.Balance == nil {
		quotaPlan.Balance = &shared.BalanceSummary{}
	}
	quotaPlan.Balance.Used = &used
	quotaPlan.Balance.Remaining = &displayRemaining
}

// fillUnlimitedQuotaBalance 为无限配额填充 sentinel 余额（used=0, remaining=100000000）。
func fillUnlimitedQuotaBalance(quotaPlan *shared.QuotaPlanParam) {
	if quotaPlan == nil {
		return
	}
	used := float64(0)
	remaining := unlimitedSentinel
	if quotaPlan.Balance == nil {
		quotaPlan.Balance = &shared.BalanceSummary{}
	}
	quotaPlan.Balance.Used = &used
	quotaPlan.Balance.Remaining = &remaining
}

// populateQuotaBalance 为单个 API-Key 从 Redis 实时读取剩余量并填充 Balance。
func (rppm *APIKeyManager) populateQuotaBalance(ctx context.Context, one *APIKeyParam) error {
	if rppm.quotaCache == nil {
		return nil
	}
	if one.QuotaPlan == nil || one.QuotaPlan.Quota == nil || one.Key == nil {
		return nil
	}
	if one.QuotaPlan.Unlimited != nil && *one.QuotaPlan.Unlimited {
		fillUnlimitedQuotaBalance(one.QuotaPlan)
		return nil
	}

	remaining, err := rppm.quotaCache.GetRemaining(ctx, *one.Key, one.QuotaPlan.Unit)
	if err != nil {
		return fmt.Errorf("get %s-%d from cache is error:%s", *one.Key, one.KeyCreateAt.Unix(), err.Error())
	}
	fillQuotaBalance(one.QuotaPlan, remaining)
	return nil
}

// populateQuotaBalances 为 API-Key 列表批量从 Redis 读取剩余量并填充 Balance。
func (rppm *APIKeyManager) populateQuotaBalances(ctx context.Context, list []*APIKeyParam) error {
	if rppm.quotaCache == nil {
		return nil
	}

	type item struct {
		one *APIKeyParam
		key string
	}
	groups := make(map[string][]item)
	for _, one := range list {
		if one.QuotaPlan == nil || one.QuotaPlan.Quota == nil || one.Key == nil {
			continue
		}
		if one.QuotaPlan.Unlimited != nil && *one.QuotaPlan.Unlimited {
			fillUnlimitedQuotaBalance(one.QuotaPlan)
			continue
		}
		unit := ""
		if one.QuotaPlan.Unit != nil {
			unit = *one.QuotaPlan.Unit
		}
		groups[unit] = append(groups[unit], item{one: one, key: *one.Key})
	}

	for unit, items := range groups {
		keys := make([]string, len(items))
		for i, it := range items {
			keys[i] = it.key
		}
		var unitPtr *string
		if unit != "" {
			unitPtr = &unit
		}
		result, err := rppm.quotaCache.BatchGetRemaining(ctx, keys, unitPtr)
		if err != nil {
			return err
		}
		for _, it := range items {
			fillQuotaBalance(it.one.QuotaPlan, result[it.key])
		}
	}
	return nil
}

// DeleteAPIKey deletes an API key based on filter criteria
func (rppm *APIKeyManager) DeleteAPIKey(ctx context.Context, filter *APIKeyFilter) error {
	var (
		quotaKey           string
		rateLimitKeys      []string
		oldAPIKey          *APIKeyParam
		oldQuotaPlan       *shared.QuotaPlanParam
		oldRateLimitPolicy *shared.RateLimitPolicyParam
	)

	err := rppm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		list, err := rppm.storager.FetchAPIKeyList(ctx, filter)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return xerror.WrapRecordNotExist("APIKey")
		}

		one := list[0]
		oldAPIKey = one

		if one.Key != nil {
			quotaKey = *one.Key
		}

		if one.RateLimitPolicyID != nil && rppm.rateLimitPolicyStorager != nil {
			policy, err := rppm.rateLimitPolicyStorager.FetchRateLimitPolicy(ctx, *one.RateLimitPolicyID)
			if err != nil {
				return err
			}
			oldRateLimitPolicy = policy
			if policy != nil && policy.Rules != nil {
				rateLimitKeys = shared.BuildRateLimitRedisKeys(*one.RateLimitPolicyID, policy.Rules)
			}
		}

		// Soft-fetch the quota plan for the nested audit before-snapshot
		// (a fetch failure only leaves the before map empty).
		if one.QuotaPlanID != nil && rppm.quotaPlanStorager != nil {
			if plan, planErr := rppm.quotaPlanStorager.FetchQuotaPlan(ctx, *one.QuotaPlanID); planErr == nil {
				oldQuotaPlan = plan
			}
		}

		if one.QuotaPlanID != nil && rppm.quotaPlanStorager != nil {
			if err := rppm.quotaPlanStorager.DeleteQuotaPlan(ctx, *one.QuotaPlanID); err != nil {
				return err
			}
		}

		if one.RateLimitPolicyID != nil && rppm.rateLimitPolicyStorager != nil {
			if err := rppm.rateLimitPolicyStorager.DeleteRateLimitPolicy(ctx, *one.RateLimitPolicyID); err != nil {
				return err
			}
		}

		if one.RouteRulesID != nil && rppm.routeRulesStorager != nil {
			if err := rppm.routeRulesStorager.DeleteRouteRules(ctx, *one.RouteRulesID); err != nil {
				return err
			}
		}

		return rppm.storager.DeleteAPIKey(ctx, filter)
	})
	if err != nil {
		rppm.recordAPIKeyOperation(ctx, string(ioperlog.ActionDelete), oldAPIKey, nil, apiKeyParamToMap(oldAPIKey), err)
		rppm.auditNestedQuotaPlanDelete(ctx, oldAPIKey, oldQuotaPlan, err)
		rppm.auditNestedRateLimitPolicyDelete(ctx, oldAPIKey, oldRateLimitPolicy, err)
		return err
	}

	// 事务提交成功后清理 Redis Key
	rppm.cleanupRedisKeys(ctx, quotaKey, rateLimitKeys)

	rppm.recordAPIKeyOperation(ctx, string(ioperlog.ActionDelete), oldAPIKey, nil, apiKeyParamToMap(oldAPIKey), nil)
	rppm.auditNestedQuotaPlanDelete(ctx, oldAPIKey, oldQuotaPlan, nil)
	rppm.auditNestedRateLimitPolicyDelete(ctx, oldAPIKey, oldRateLimitPolicy, nil)
	return nil
}

// cleanupRedisKeys 清理 Quota Key 与 Rate-Limit Key，错误仅记录日志不返回。
func (rppm *APIKeyManager) cleanupRedisKeys(ctx context.Context, quotaKey string, rateLimitKeys []string) {
	if rppm.quotaCache == nil {
		return
	}

	var keysToDelete []string
	if quotaKey != "" {
		keysToDelete = append(keysToDelete, stateful.AIUsedQuotaKey(quotaKey))
	}
	if len(rateLimitKeys) > 0 {
		keysToDelete = append(keysToDelete, rateLimitKeys...)
	}
	if len(keysToDelete) == 0 {
		return
	}

	if err := rppm.quotaCache.DeleteKeys(ctx, keysToDelete); err != nil {
		stateful.AccessLogger.Warn("failed to cleanup redis keys for api key: %v", err)
	}
}

// UpdateAPIKey updates an existing API key
func (rppm *APIKeyManager) UpdateAPIKey(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) error {
	var (
		rateLimitKeysToDelete  []string
		oldAPIKey              *APIKeyParam
		createdQuotaPlanID     int64
		createdRateLimitPolicy int64
		oldQuotaPlan           *shared.QuotaPlanParam
		oldRateLimitPolicy     *shared.RateLimitPolicyParam
	)

	err := rppm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		list, err := rppm.storager.FetchAPIKeyList(ctx, filter)
		if err != nil {
			return err
		}
		if len(list) == 0 {
			return xerror.WrapRecordNotExist("API-Key")
		}

		one := list[0]
		oldAPIKey = one

		// key is immutable through update endpoints
		param.Key = nil

		// Check if entity_id exists
		if param.EntityID != nil && *param.EntityID != "" && rppm.entityStorager != nil {
			entity, err := rppm.entityStorager.FetchEntity(ctx, &shared.EntityFilter{EntityID: param.EntityID})
			if err != nil {
				return err
			}
			if entity == nil {
				return xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("Entity not found: %s", *param.EntityID))
			}
		}

		if param.QuotaPlan != nil && rppm.quotaPlanStorager != nil {
			if one.QuotaPlanID != nil {
				if plan, planErr := rppm.quotaPlanStorager.FetchQuotaPlan(ctx, *one.QuotaPlanID); planErr == nil {
					oldQuotaPlan = plan
				}
				_, err = rppm.quotaPlanStorager.UpdateQuotaPlan(ctx, *one.QuotaPlanID, param.QuotaPlan)
				if err != nil {
					return err
				}
			} else {
				createdQuotaPlanID, err = rppm.quotaPlanStorager.CreateQuotaPlan(ctx, param.QuotaPlan)
				if err != nil {
					return err
				}
				param.QuotaPlanID = &createdQuotaPlanID

			}
		}

		if param.RateLimitPolicy != nil && rppm.rateLimitPolicyStorager != nil {
			if one.RateLimitPolicyID != nil {
				oldPolicy, err := rppm.rateLimitPolicyStorager.FetchRateLimitPolicy(ctx, *one.RateLimitPolicyID)
				if err != nil {
					return err
				}
				oldRateLimitPolicy = oldPolicy
				_, err = rppm.rateLimitPolicyStorager.UpdateRateLimitPolicy(ctx, *one.RateLimitPolicyID, param.RateLimitPolicy)
				if err != nil {
					return err
				}
				if oldPolicy != nil && oldPolicy.Rules != nil {
					rateLimitKeysToDelete = shared.DiffRateLimitRedisKeys(*one.RateLimitPolicyID, oldPolicy.Rules, param.RateLimitPolicy.Rules)
				}
			} else {
				createdRateLimitPolicy, err = rppm.rateLimitPolicyStorager.CreateRateLimitPolicy(ctx, param.RateLimitPolicy)
				if err != nil {
					return err
				}
				param.RateLimitPolicyID = &createdRateLimitPolicy
			}
		}

		if param.RouteRules != nil && rppm.routeRulesStorager != nil {
			if one.RouteRulesID != nil {
				_, err = rppm.routeRulesStorager.UpdateRouteRules(ctx, *one.RouteRulesID, param.RouteRules)
				if err != nil {
					return err
				}
			} else {
				routeRulesID, err := rppm.routeRulesStorager.CreateRouteRules(ctx, shared.RouteRulesTypeAPIKey, one.ID, param.RouteRules)
				if err != nil {
					return err
				}
				param.RouteRulesID = &routeRulesID
			}
		}

		_, err = rppm.storager.UpdateAPIKey(ctx, &APIKeyFilter{
			InnerID: one.InnerID,
		}, param)
		return err
	})
	if err != nil {
		rppm.recordAPIKeyOperation(ctx, string(ioperlog.ActionUpdate), oldAPIKey, apiKeyParamToMap(oldAPIKey), apiKeyParamToMap(param), err)
		rppm.auditNestedQuotaPlanChange(ctx, oldAPIKey, param, 0, oldQuotaPlan, err)
		rppm.auditNestedRateLimitPolicyChange(ctx, oldAPIKey, param, 0, oldRateLimitPolicy, err)
		return err
	}

	if len(rateLimitKeysToDelete) > 0 {
		rppm.cleanupRedisKeys(ctx, "", rateLimitKeysToDelete)
	}

	rppm.recordAPIKeyOperation(ctx, string(ioperlog.ActionUpdate), oldAPIKey, apiKeyParamToMap(oldAPIKey), apiKeyParamToMap(param), nil)
	rppm.auditNestedQuotaPlanChange(ctx, oldAPIKey, param, createdQuotaPlanID, oldQuotaPlan, nil)
	rppm.auditNestedRateLimitPolicyChange(ctx, oldAPIKey, param, createdRateLimitPolicy, oldRateLimitPolicy, nil)
	return nil
}

// CreateAPIKey creates a new API key
func (rppm *APIKeyManager) CreateAPIKey(ctx context.Context,
	param *APIKeyParam) (err error) {
	var (
		quotaPlanID       int64
		rateLimitPolicyID int64
	)
	err = rppm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		// Generate ID if not provided
		if param.ID == nil || *param.ID == "" {
			id := uuid.New().String()
			param.ID = &id
		}

		// Check for duplicate API key ID within the same product
		list, err := rppm.storager.FetchAPIKeyList(ctx, &APIKeyFilter{
			ID:          param.ID,
			ProductName: param.ProductName,
		})
		if err != nil {
			return err
		}

		if len(list) > 0 {
			return xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("Duplicate id with product:%s", *param.ProductName))
		}

		// Check if entity_id exists
		if param.EntityID != nil && *param.EntityID != "" && rppm.entityStorager != nil {
			entity, err := rppm.entityStorager.FetchEntity(ctx, &shared.EntityFilter{EntityID: param.EntityID})
			if err != nil {
				return err
			}
			if entity == nil {
				return xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("Entity not found: %s", *param.EntityID))
			}
		}

		updatedTime := time.Now().Unix()
		param.UpdatedTime = &updatedTime

		// Check global uniqueness of the API key value
		if param.Key != nil && *param.Key != "" {
			tokens, err := rppm.storager.FetchAPIKeyTokenList(ctx, &APIKeyTokenFilter{Key: param.Key})
			if err != nil {
				return err
			}
			if len(tokens) > 1 {
				return xerror.WrapDirtyDataErrorWithMsg("%s", fmt.Sprintf("API-Key-Token:%s", ioperlog.MaskAPIKeyToken(*param.Key)))
			}

			existingKeys, err := rppm.storager.FetchAPIKeyList(ctx, &APIKeyFilter{Key: param.Key})
			if err != nil {
				return err
			}
			if len(existingKeys) > 0 {
				return xerror.WrapParamErrorWithMsg("API-Key value %s already exists", ioperlog.MaskAPIKeyToken(*param.Key))
			}

			// Set updated time based on existing token if reused
			if len(tokens) > 0 {
				updatedTime := tokens[0].CreatedAt.Unix()
				param.UpdatedTime = &updatedTime
			}
		}

		// Create QuotaPlan if provided
		if param.QuotaPlan != nil && rppm.quotaPlanStorager != nil {
			quotaPlanID, err = rppm.quotaPlanStorager.CreateQuotaPlan(ctx, param.QuotaPlan)
			if err != nil {
				return err
			}
			param.QuotaPlanID = &quotaPlanID

		}

		// Create RateLimitPolicy if provided
		if param.RateLimitPolicy != nil && rppm.rateLimitPolicyStorager != nil {
			rateLimitPolicyID, err = rppm.rateLimitPolicyStorager.CreateRateLimitPolicy(ctx, param.RateLimitPolicy)
			if err != nil {
				return err
			}
			param.RateLimitPolicyID = &rateLimitPolicyID
		}

		// Create RouteRules if provided
		if param.RouteRules != nil && rppm.routeRulesStorager != nil {
			routeRulesID, err := rppm.routeRulesStorager.CreateRouteRules(ctx, shared.RouteRulesTypeAPIKey, param.ID, param.RouteRules)
			if err != nil {
				return err
			}
			param.RouteRulesID = &routeRulesID
		}

		_, err = rppm.storager.CreateAPIKey(ctx, param)
		return err
	})
	if err != nil {
		rppm.recordAPIKeyOperation(ctx, string(ioperlog.ActionCreate), param, nil, apiKeyParamToMap(param), err)
		rppm.auditNestedQuotaPlanCreate(ctx, param, 0, err)
		rppm.auditNestedRateLimitPolicyCreate(ctx, param, 0, err)
		return err
	}

	// Sync Redis remaining quota after DB transaction commits (best-effort).
	if param.Key != nil && param.QuotaPlan != nil &&
		(param.QuotaPlan.Unlimited == nil || !*param.QuotaPlan.Unlimited) &&
		param.QuotaPlan.Quota != nil && rppm.quotaCache != nil {
		if cacheErr := rppm.quotaCache.SetRemaining(ctx, *param.Key, param.QuotaPlan.Quota, param.QuotaPlan.Unit); cacheErr != nil {
			stateful.AccessLogger.Warn("failed to set quota cache for api_key %s: %v", *param.Key, cacheErr)
		}
	}

	rppm.recordAPIKeyOperation(ctx, string(ioperlog.ActionCreate), param, nil, apiKeyParamToMap(param), nil)
	rppm.auditNestedQuotaPlanCreate(ctx, param, quotaPlanID, nil)
	rppm.auditNestedRateLimitPolicyCreate(ctx, param, rateLimitPolicyID, nil)
	return nil
}

// CreateAPIKeyToken creates a new API key token
func (rppm *APIKeyManager) CreateAPIKeyToken(ctx context.Context,
	param *APIKeyTokenParam) (id int64, err error) {
	err = rppm.txn.AtomExecute(ctx, func(ctx context.Context) error {
		id, err = rppm.storager.CreateAPIKeyToken(ctx, param)
		if err != nil {
			return err
		}

		// Update the token with the full key format: key-id
		return rppm.storager.UpdateAPIKeyToken(ctx, &APIKeyTokenFilter{ID: &id}, &APIKeyTokenParam{
			Key: lib.PString(fmt.Sprintf("%s-%d", *param.Key, id)),
		})
	})

	return
}
