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

package quota

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaPlanManager_ResetBalance(t *testing.T) {
	ctx := context.Background()

	t.Run("plan not found", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return nil, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		err := m.ResetBalance(ctx, 1, nil, true, shared.ResourceOwner{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quota_plan not found")
	})

	t.Run("unlimited plan cannot reset", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{Unlimited: lib.PBool(true)}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		err := m.ResetBalance(ctx, 1, nil, true, shared.ResourceOwner{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot reset balance for unlimited quota")

		// issue #183：语义错误必须定型为 PARAM → 422 Param Illegal，不得兜底 500。
		rr := xerror.Resolve(err)
		assert.Equal(t, 422, rr.ErrNo)
		assert.Equal(t, "Param Illegal", rr.Type)
		assert.Equal(t, "cannot reset balance for unlimited quota", fmt.Sprintf("%v", xerror.Cause(err)))
	})

	t.Run("reset updates last_reset_at", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(1), Quota: lib.PFloat64(1000)}, nil
			},
			updateFn: func(ctx context.Context, filter *QuotaPlanFilter, param *QuotaPlanParam) (int64, error) {
				return 1, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		err := m.ResetBalance(ctx, 1, nil, true, shared.ResourceOwner{})
		require.NoError(t, err)

		require.Len(t, planStore.updated, 1)
		assert.Equal(t, int64(1), *planStore.updated[0].filter.ID)
		assert.NotNil(t, planStore.updated[0].param.LastResetAt)
	})

	t.Run("reset without updating last_reset_at", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(1), Quota: lib.PFloat64(1000)}, nil
			},
			updateFn: func(ctx context.Context, filter *QuotaPlanFilter, param *QuotaPlanParam) (int64, error) {
				return 1, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		err := m.ResetBalance(ctx, 1, nil, false, shared.ResourceOwner{})
		require.NoError(t, err)

		assert.Empty(t, planStore.updated)
	})

	t.Run("reset with new quota updates plan", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(1), Quota: lib.PFloat64(1000)}, nil
			},
			updateFn: func(ctx context.Context, filter *QuotaPlanFilter, param *QuotaPlanParam) (int64, error) {
				return 1, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		newQuota := float64(2000)
		err := m.ResetBalance(ctx, 1, &newQuota, false, shared.ResourceOwner{})
		require.NoError(t, err)

		require.Len(t, planStore.updated, 1)
		assert.Equal(t, float64(2000), *planStore.updated[0].param.Quota)
	})

	t.Run("propagate plan update error", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(1), Quota: lib.PFloat64(1000)}, nil
			},
			updateFn: func(ctx context.Context, filter *QuotaPlanFilter, param *QuotaPlanParam) (int64, error) {
				return 0, errors.New("plan update failed")
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		err := m.ResetBalance(ctx, 1, nil, true, shared.ResourceOwner{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan update failed")
	})
}

func TestQuotaPlanManager_CRUD(t *testing.T) {
	ctx := context.Background()

	t.Run("CreateQuotaPlan", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			createFn: func(ctx context.Context, param *QuotaPlanParam) (int64, error) {
				return 7, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		id, err := m.CreateQuotaPlan(ctx, &QuotaPlanParam{Quota: lib.PFloat64(100)}, shared.ResourceOwner{})
		require.NoError(t, err)
		assert.Equal(t, int64(7), id)
	})

	t.Run("FetchQuotaPlan", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(7), Quota: lib.PFloat64(100)}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		plan, err := m.FetchQuotaPlan(ctx, &QuotaPlanFilter{ID: lib.PInt64(7)})
		require.NoError(t, err)
		require.NotNil(t, plan)
		assert.Equal(t, float64(100), *plan.Quota)
	})

	t.Run("FetchQuotaPlanList", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			listFn: func(ctx context.Context, filter *QuotaPlanFilter) ([]*QuotaPlanParam, error) {
				return []*QuotaPlanParam{{ID: lib.PInt64(7)}}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		list, err := m.FetchQuotaPlanList(ctx, &QuotaPlanFilter{})
		require.NoError(t, err)
		assert.Len(t, list, 1)
	})

	t.Run("DeleteQuotaPlan", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			deleteFn: func(ctx context.Context, filter *QuotaPlanFilter) error {
				return nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, nil, nil, nil)

		require.NoError(t, m.DeleteQuotaPlan(ctx, &QuotaPlanFilter{ID: lib.PInt64(7)}, shared.ResourceOwner{}))
		assert.Len(t, planStore.deleted, 1)
	})
}

func TestQuotaPlanManager_UpdateQuotaPlan(t *testing.T) {
	ctx := context.Background()
	store := &fakeQuotaPlanStorager{
		updateFn: func(ctx context.Context, filter *QuotaPlanFilter, param *QuotaPlanParam) (int64, error) {
			assert.Equal(t, int64(7), *filter.ID)
			assert.Equal(t, float64(500), *param.Quota)
			return 1, nil
		},
	}
	m := NewQuotaPlanManager(&fakeTxn{}, store, nil, nil, nil)

	affected, err := m.UpdateQuotaPlan(ctx, &QuotaPlanFilter{ID: lib.PInt64(7)}, &QuotaPlanParam{Quota: lib.PFloat64(500)}, shared.ResourceOwner{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
}

func TestQuotaPlanManager_ApplyQuotaPlanChange(t *testing.T) {
	ctx := context.Background()
	planID := int64(1)
	apiKeyID := "ak-1"
	entityID := "ent-1"

	t.Run("no change", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{}
		cache := &fakeQuotaCache{}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, &fakeAPIKeyStorager{}, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)
		assert.Empty(t, planStore.updated)
		assert.Empty(t, cache.setRemainingCalls)
		assert.Empty(t, cache.resetToQuotaCalls)
	})

	t.Run("quota increased preserves used", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Quota: lib.PFloat64(150), Unit: lib.PString("total_token")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		cache := &fakeQuotaCache{
			batchGetRemainingFn: func(ctx context.Context, keys []string, unit *string) (map[string]float64, error) {
				return map[string]float64{apiKeyID: 40}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(150), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.setRemainingCalls, 1)
		assert.Equal(t, apiKeyID, cache.setRemainingCalls[0].key)
		assert.Equal(t, float64(90), *cache.setRemainingCalls[0].quota)
		assert.Empty(t, cache.resetToQuotaCalls)
	})

	t.Run("quota decreased preserves used", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Quota: lib.PFloat64(50), Unit: lib.PString("total_token")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		cache := &fakeQuotaCache{
			batchGetRemainingFn: func(ctx context.Context, keys []string, unit *string) (map[string]float64, error) {
				return map[string]float64{apiKeyID: 40}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(50), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.setRemainingCalls, 1)
		assert.Equal(t, apiKeyID, cache.setRemainingCalls[0].key)
		// issue #1476: used(60) > newQuota(50)，targetRemaining 允许为负，
		// 表示新配额仍不足以覆盖已用量；不再钳零（钳零会凭空抹掉欠费）。
		assert.Equal(t, float64(-10), *cache.setRemainingCalls[0].quota)
	})

	t.Run("quota increased while in arrears keeps overage", func(t *testing.T) {
		// issue #1476: remaining 为负（欠费）时 used = oldQuota - remaining
		// 如实大于 oldQuota；调大配额后 targetRemaining = newQuota - used。
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Quota: lib.PFloat64(200), Unit: lib.PString("total_token")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		cache := &fakeQuotaCache{
			batchGetRemainingFn: func(ctx context.Context, keys []string, unit *string) (map[string]float64, error) {
				return map[string]float64{apiKeyID: -40}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(200), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.setRemainingCalls, 1)
		// used = 100 - (-40) = 140；targetRemaining = 200 - 140 = 60。
		assert.Equal(t, float64(60), *cache.setRemainingCalls[0].quota)
	})

	t.Run("quota raised but still below usage stays negative", func(t *testing.T) {
		// issue #1476: 新配额仍低于 used 时 targetRemaining 为负，鉴权继续拒绝。
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Quota: lib.PFloat64(120), Unit: lib.PString("total_token")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		cache := &fakeQuotaCache{
			batchGetRemainingFn: func(ctx context.Context, keys []string, unit *string) (map[string]float64, error) {
				return map[string]float64{apiKeyID: -40}, nil
			},
		}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(120), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.setRemainingCalls, 1)
		// used = 140；targetRemaining = 120 - 140 = -20。
		assert.Equal(t, float64(-20), *cache.setRemainingCalls[0].quota)
	})

	t.Run("unit changed resets to new quota", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Quota: lib.PFloat64(10), Unit: lib.PString("RMB")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		cache := &fakeQuotaCache{}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(10), Unit: lib.PString("RMB")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.resetToQuotaCalls, 1)
		assert.Equal(t, apiKeyID, cache.resetToQuotaCalls[0].key)
		assert.Equal(t, float64(10), *cache.resetToQuotaCalls[0].quota)
		assert.Equal(t, "RMB", *cache.resetToQuotaCalls[0].unit)
	})

	t.Run("newly created quota plan resets both api key and entity", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Quota: lib.PFloat64(200), Unit: lib.PString("total_token")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		entityStore := &fakeEntityStorager{
			listFn: func(ctx context.Context, filter *entity.EntityFilter) ([]*entity.EntityParam, error) {
				return []*entity.EntityParam{{EntityID: &entityID}}, nil
			},
		}
		cache := &fakeQuotaCache{}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, entityStore, cache)

		newPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(200), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, nil, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.resetToQuotaCalls, 2)
		assert.Equal(t, apiKeyID, cache.resetToQuotaCalls[0].key)
		assert.Equal(t, entityID, cache.resetToQuotaCalls[1].key)
	})

	t.Run("switch to unlimited resets with sentinel", func(t *testing.T) {
		planStore := &fakeQuotaPlanStorager{
			fetchFn: func(ctx context.Context, filter *QuotaPlanFilter) (*QuotaPlanParam, error) {
				return &QuotaPlanParam{ID: lib.PInt64(planID), Unlimited: lib.PBool(true), Unit: lib.PString("total_token")}, nil
			},
		}
		apiKeyStore := &fakeAPIKeyStorager{
			fetchListFn: func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
				return []*api_key.APIKeyParam{{Key: &apiKeyID}}, nil
			},
		}
		cache := &fakeQuotaCache{}
		m := NewQuotaPlanManager(&fakeTxn{}, planStore, apiKeyStore, &fakeEntityStorager{}, cache)

		oldPlan := &shared.QuotaPlanParam{Quota: lib.PFloat64(100), Unit: lib.PString("total_token")}
		newPlan := &shared.QuotaPlanParam{Unlimited: lib.PBool(true), Unit: lib.PString("total_token")}
		err := m.ApplyQuotaPlanChange(ctx, planID, oldPlan, newPlan)
		require.NoError(t, err)

		require.Len(t, cache.resetToQuotaCalls, 1)
		assert.Equal(t, float64(100000000), *cache.resetToQuotaCalls[0].quota)
	})
}
