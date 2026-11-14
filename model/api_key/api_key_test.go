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

package api_key

import (
	"context"
	"errors"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quotacache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrBool(b bool) *bool          { return &b }
func ptrInt64(i int64) *int64       { return &i }
func ptrFloat64(f float64) *float64 { return &f }
func ptrString(s string) *string    { return &s }

func TestGetRemainingQuota(t *testing.T) {
	ctx := context.Background()

	t.Run("unlimited quota", func(t *testing.T) {
		remain, err := GetRemainingQuota(ctx, nil, &APIKeyParam{UnlimitedQuota: ptrBool(true)})
		require.NoError(t, err)
		assert.Nil(t, remain)
	})

	t.Run("unlimited plan", func(t *testing.T) {
		remain, err := GetRemainingQuota(ctx, nil, &APIKeyParam{
			QuotaPlan: &shared.QuotaPlanParam{Unlimited: ptrBool(true)},
		})
		require.NoError(t, err)
		assert.Nil(t, remain)
	})

	t.Run("nil quota plan", func(t *testing.T) {
		remain, err := GetRemainingQuota(ctx, nil, &APIKeyParam{})
		require.NoError(t, err)
		assert.Nil(t, remain)
	})

	t.Run("no cache", func(t *testing.T) {
		remain, err := GetRemainingQuota(ctx, nil, &APIKeyParam{
			Key:       ptrString("key"),
			QuotaPlan: &shared.QuotaPlanParam{Quota: ptrFloat64(100)},
		})
		require.NoError(t, err)
		assert.Equal(t, float64(100), *remain)
	})

	t.Run("redis returns remain", func(t *testing.T) {
		redisClient := stateful.NewMockRedisClient()
		redisClient.IncrBy(stateful.AIUsedQuotaKey("key"), 30)
		cache := quotacache.NewRedisQuotaCache(redisClient)

		remain, err := GetRemainingQuota(ctx, cache, &APIKeyParam{
			Key:       ptrString("key"),
			QuotaPlan: &shared.QuotaPlanParam{Quota: ptrFloat64(100)},
		})
		require.NoError(t, err)
		assert.Equal(t, float64(30), *remain)
	})

	t.Run("redis negative remain", func(t *testing.T) {
		redisClient := stateful.NewMockRedisClient()
		redisClient.IncrBy(stateful.AIUsedQuotaKey("key"), -5)
		cache := quotacache.NewRedisQuotaCache(redisClient)

		remain, err := GetRemainingQuota(ctx, cache, &APIKeyParam{
			Key:       ptrString("key"),
			QuotaPlan: &shared.QuotaPlanParam{Quota: ptrFloat64(100)},
		})
		require.NoError(t, err)
		assert.Equal(t, float64(0), *remain)
	})
}

func TestFillUnlimitedQuotaBalance(t *testing.T) {
	quotaPlan := &shared.QuotaPlanParam{Unlimited: ptrBool(true)}
	fillUnlimitedQuotaBalance(quotaPlan)
	require.NotNil(t, quotaPlan.Balance)
	assert.Equal(t, float64(0), *quotaPlan.Balance.Used)
	assert.Equal(t, float64(100000000), *quotaPlan.Balance.Remaining)
}

func TestFillQuotaBalance(t *testing.T) {
	t.Run("normal remaining", func(t *testing.T) {
		quotaPlan := &shared.QuotaPlanParam{Quota: ptrFloat64(100)}
		fillQuotaBalance(quotaPlan, 40)
		require.NotNil(t, quotaPlan.Balance)
		assert.Equal(t, float64(60), *quotaPlan.Balance.Used)
		assert.Equal(t, float64(40), *quotaPlan.Balance.Remaining)
	})

	t.Run("negative remaining records overage", func(t *testing.T) {
		// issue #1476: BFE 全额扣减后余额可为负，used 如实反映实际消耗、
		// 允许大于 quota；对外展示的 Remaining 钳 0。
		quotaPlan := &shared.QuotaPlanParam{Quota: ptrFloat64(100)}
		fillQuotaBalance(quotaPlan, -25)
		require.NotNil(t, quotaPlan.Balance)
		assert.Equal(t, float64(125), *quotaPlan.Balance.Used)
		assert.Equal(t, float64(0), *quotaPlan.Balance.Remaining)
	})

	t.Run("remaining above quota guards used", func(t *testing.T) {
		// 防御：数据异常（remaining > quota）时 used 兜底为 0。
		quotaPlan := &shared.QuotaPlanParam{Quota: ptrFloat64(100)}
		fillQuotaBalance(quotaPlan, 120)
		require.NotNil(t, quotaPlan.Balance)
		assert.Equal(t, float64(0), *quotaPlan.Balance.Used)
		assert.Equal(t, float64(120), *quotaPlan.Balance.Remaining)
	})
}

func newAPIKeyManager(storager *fakeAPIKeyStorager) *APIKeyManager {
	return NewAPIKeyManager(&fakeTxn{}, storager, &fakeQuotaPlanStorager{}, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, &fakeEntityStorager{}, nil)
}

func TestAPIKeyManager_FetchAPIKeyList(t *testing.T) {
	ctx := context.Background()
	expected := []*APIKeyParam{{Key: ptrString("k1")}}
	store := &fakeAPIKeyStorager{
		fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
			return expected, nil
		},
	}
	m := newAPIKeyManager(store)
	got, err := m.FetchAPIKeyList(ctx, &APIKeyFilter{})
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestAPIKeyManager_FetchAPIKeyList_MissingAssociatedData(t *testing.T) {
	ctx := context.Background()
	store := &fakeAPIKeyStorager{
		fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
			return []*APIKeyParam{{
				Key:               ptrString("k1"),
				QuotaPlanID:       ptrInt64(1),
				RateLimitPolicyID: ptrInt64(2),
			}}, nil
		},
	}
	m := newAPIKeyManager(store)
	got, err := m.FetchAPIKeyList(ctx, &APIKeyFilter{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "k1", *got[0].Key)
	assert.NotNil(t, got[0].QuotaPlan)
	assert.NotNil(t, got[0].RateLimitPolicy)
}

func TestAPIKeyManager_FetchAPIKey(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{Key: ptrString("k1"), InnerID: ptrInt64(1)}}, nil
			},
		}
		m := newAPIKeyManager(store)
		got, err := m.FetchAPIKey(ctx, &APIKeyFilter{})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "k1", *got.Key)
	})

	t.Run("not found", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
		}
		m := newAPIKeyManager(store)
		got, err := m.FetchAPIKey(ctx, &APIKeyFilter{})
		require.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestAPIKeyManager_DeleteAPIKey(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.DeleteAPIKey(ctx, &APIKeyFilter{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "APIKey Record Not Exist")
	})

	t.Run("success", func(t *testing.T) {
		deleted := false
		quotaDeleted := false
		rateLimitDeleted := false
		routeDeleted := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:               ptrString("k1"),
					QuotaPlanID:       ptrInt64(1),
					RateLimitPolicyID: ptrInt64(2),
					RouteRulesID:      ptrInt64(3),
				}}, nil
			},
			deleteAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter) error {
				deleted = true
				return nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store,
			&fakeQuotaPlanStorager{
				deleteQuotaPlanFn: func(ctx context.Context, id int64) error {
					quotaDeleted = true
					return nil
				},
			},
			&fakeRateLimitPolicyStorager{
				deleteRateLimitPolicyFn: func(ctx context.Context, id int64) error {
					rateLimitDeleted = true
					return nil
				},
			},
			&fakeRouteRulesStorager{
				deleteRouteRulesFn: func(ctx context.Context, id int64) error {
					routeDeleted = true
					return nil
				},
			},
			&fakeEntityStorager{}, nil)
		err := m.DeleteAPIKey(ctx, &APIKeyFilter{})
		require.NoError(t, err)
		assert.True(t, deleted)
		assert.True(t, quotaDeleted)
		assert.True(t, rateLimitDeleted)
		assert.True(t, routeDeleted)
	})

	t.Run("cleanup redis keys after delete", func(t *testing.T) {
		key := "ak-cleanup"
		policyID := int64(2)
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:               ptrString(key),
					QuotaPlanID:       ptrInt64(1),
					RateLimitPolicyID: &policyID,
				}}, nil
			},
			deleteAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter) error {
				return nil
			},
		}
		rateLimitStore := &fakeRateLimitPolicyStorager{
			fetchRateLimitPolicyFn: func(ctx context.Context, id int64) (*shared.RateLimitPolicyParam, error) {
				return &shared.RateLimitPolicyParam{
					Rules: &shared.RateLimitRules{
						TpmConfigs: []shared.TPMConfig{{Name: "tpm-1", Model: "*", WindowMinutes: 1, MaxTokens: 100, StepMinutes: 1}},
						RpmConfigs: []shared.RPMConfig{{Name: "rpm-1", Model: "*", WindowMinutes: 1, MaxRequests: 10}},
					},
				}, nil
			},
			deleteRateLimitPolicyFn: func(ctx context.Context, id int64) error {
				return nil
			},
		}
		cache := &fakeQuotaCache{}
		m := NewAPIKeyManager(&fakeTxn{}, store, &fakeQuotaPlanStorager{deleteQuotaPlanFn: func(ctx context.Context, id int64) error { return nil }}, rateLimitStore, &fakeRouteRulesStorager{}, &fakeEntityStorager{}, cache)

		err := m.DeleteAPIKey(ctx, &APIKeyFilter{})
		require.NoError(t, err)
		require.Len(t, cache.deleteKeysCalls, 1)
		assert.ElementsMatch(t, []string{
			stateful.AIUsedQuotaKey(key),
			"default_bfe_rlp-2_RL_TPM_rlp-2_tpm-1",
			"default_bfe_rlp-2_RL_RPM_rlp-2_rpm-1",
		}, cache.deleteKeysCalls[0])
	})
}

func TestAPIKeyManager_UpdateAPIKey(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "API-Key Record Not Exist")
	})

	t.Run("success", func(t *testing.T) {
		updated := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:       ptrString("k1"),
					InnerID:   ptrInt64(1),
					QuotaPlan: &shared.QuotaPlanParam{Unlimited: ptrBool(true)},
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				updated = true
				assert.Nil(t, param.Key)
				return 1, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{Key: ptrString("should-be-cleared")})
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("entity not found", func(t *testing.T) {
		updated := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:     ptrString("k1"),
					InnerID: ptrInt64(1),
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				updated = true
				return 1, nil
			},
		}
		entityStore := &fakeEntityStorager{
			fetchEntityFn: func(ctx context.Context, filter *shared.EntityFilter) (*shared.EntitySummary, error) {
				assert.Equal(t, "e1", *filter.EntityID)
				return nil, nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store, &fakeQuotaPlanStorager{}, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, entityStore, nil)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{EntityID: ptrString("e1")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Entity not found: e1")
		assert.False(t, updated)
	})

	t.Run("empty entity id skips existence check", func(t *testing.T) {
		updated := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:     ptrString("k1"),
					InnerID: ptrInt64(1),
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				updated = true
				return 1, nil
			},
		}
		entityStore := &fakeEntityStorager{
			fetchEntityFn: func(ctx context.Context, filter *shared.EntityFilter) (*shared.EntitySummary, error) {
				t.Fatal("FetchEntity must not be called for empty entity_id")
				return nil, nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store, &fakeQuotaPlanStorager{}, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, entityStore, nil)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{EntityID: ptrString("")})
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("update quota plan", func(t *testing.T) {
		updated := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:         ptrString("k1"),
					InnerID:     ptrInt64(1),
					QuotaPlanID: ptrInt64(10),
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				updated = true
				return 1, nil
			},
		}
		quotaPlanStore := &fakeQuotaPlanStorager{
			updateQuotaPlanFn: func(ctx context.Context, id int64, param *shared.QuotaPlanParam) (int64, error) {
				assert.Equal(t, int64(10), id)
				return 1, nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store, quotaPlanStore, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, &fakeEntityStorager{}, nil)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{
			QuotaPlan: &shared.QuotaPlanParam{Quota: ptrFloat64(100)},
		})
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("create new quota plan", func(t *testing.T) {
		created := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:     ptrString("k1"),
					InnerID: ptrInt64(1),
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				assert.Equal(t, int64(20), *param.QuotaPlanID)
				return 1, nil
			},
		}
		quotaPlanStore := &fakeQuotaPlanStorager{
			createQuotaPlanFn: func(ctx context.Context, param *shared.QuotaPlanParam) (int64, error) {
				created = true
				return 20, nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store, quotaPlanStore, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, &fakeEntityStorager{}, nil)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{
			QuotaPlan: &shared.QuotaPlanParam{Quota: ptrFloat64(100)},
		})
		require.NoError(t, err)
		assert.True(t, created)
	})

	t.Run("update does not sync redis", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					Key:         ptrString("k1"),
					InnerID:     ptrInt64(1),
					QuotaPlanID: ptrInt64(10),
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				return 1, nil
			},
		}
		quotaPlanStore := &fakeQuotaPlanStorager{
			updateQuotaPlanFn: func(ctx context.Context, id int64, param *shared.QuotaPlanParam) (int64, error) {
				return 1, nil
			},
		}
		cache := &fakeQuotaCache{}
		m := NewAPIKeyManager(&fakeTxn{}, store, quotaPlanStore, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, &fakeEntityStorager{}, cache)
		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{
			Description: ptrString("new desc"),
			QuotaPlan:   &shared.QuotaPlanParam{Quota: ptrFloat64(200)},
		})
		require.NoError(t, err)
		assert.Empty(t, cache.setRemainingCalls)
		assert.Empty(t, cache.resetToQuotaCalls)
	})

	t.Run("cleanup redis keys for removed rate-limit rules", func(t *testing.T) {
		policyID := int64(5)
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{
					InnerID:           ptrInt64(1),
					RateLimitPolicyID: &policyID,
				}}, nil
			},
			updateAPIKeyFn: func(ctx context.Context, filter *APIKeyFilter, param *APIKeyParam) (int64, error) {
				return 1, nil
			},
		}
		rateLimitStore := &fakeRateLimitPolicyStorager{
			fetchRateLimitPolicyFn: func(ctx context.Context, id int64) (*shared.RateLimitPolicyParam, error) {
				return &shared.RateLimitPolicyParam{
					Rules: &shared.RateLimitRules{
						TpmConfigs: []shared.TPMConfig{
							{Name: "keep", Model: "*", WindowMinutes: 1, MaxTokens: 100, StepMinutes: 1},
							{Name: "remove", Model: "*", WindowMinutes: 1, MaxTokens: 200, StepMinutes: 1},
						},
					},
				}, nil
			},
			updateRateLimitPolicyFn: func(ctx context.Context, id int64, param *shared.RateLimitPolicyParam) (int64, error) {
				return 1, nil
			},
		}
		cache := &fakeQuotaCache{}
		m := NewAPIKeyManager(&fakeTxn{}, store, &fakeQuotaPlanStorager{}, rateLimitStore, &fakeRouteRulesStorager{}, &fakeEntityStorager{}, cache)

		err := m.UpdateAPIKey(ctx, &APIKeyFilter{}, &APIKeyParam{
			RateLimitPolicy: &shared.RateLimitPolicyParam{
				Rules: &shared.RateLimitRules{
					TpmConfigs: []shared.TPMConfig{
						{Name: "keep", Model: "*", WindowMinutes: 1, MaxTokens: 150, StepMinutes: 1},
					},
				},
			},
		})
		require.NoError(t, err)
		require.Len(t, cache.deleteKeysCalls, 1)
		assert.Equal(t, []string{"default_bfe_rlp-5_RL_TPM_rlp-5_remove"}, cache.deleteKeysCalls[0])
	})
}

func TestAPIKeyManager_CreateAPIKey(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		created := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
			createAPIKeyFn: func(ctx context.Context, param *APIKeyParam) (int64, error) {
				created = true
				return 1, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.CreateAPIKey(ctx, &APIKeyParam{
			ID:          ptrString("id1"),
			ProductName: ptrString("test"),
			Key:         ptrString("key"),
		})
		require.NoError(t, err)
		assert.True(t, created)
	})

	t.Run("duplicate id", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return []*APIKeyParam{{}}, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.CreateAPIKey(ctx, &APIKeyParam{
			ID:          ptrString("id1"),
			ProductName: ptrString("test"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Duplicate id")
	})

	t.Run("entity not found", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
		}
		entityStore := &fakeEntityStorager{
			fetchEntityFn: func(ctx context.Context, filter *shared.EntityFilter) (*shared.EntitySummary, error) {
				return nil, nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store, &fakeQuotaPlanStorager{}, &fakeRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, entityStore, nil)
		err := m.CreateAPIKey(ctx, &APIKeyParam{
			ID:          ptrString("id1"),
			ProductName: ptrString("test"),
			EntityID:    ptrString("e1"),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Entity not found")
	})

	t.Run("duplicate key value", func(t *testing.T) {
		rawKey := "testproduct-abcdef012345"
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				if filter.Key != nil {
					return []*APIKeyParam{{Key: ptrString(rawKey)}}, nil
				}
				return nil, nil
			},
			fetchAPIKeyTokenListFn: func(ctx context.Context, filter *APIKeyTokenFilter) ([]*APIKeyTokenParam, error) {
				return nil, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.CreateAPIKey(ctx, &APIKeyParam{
			ID:          ptrString("id1"),
			ProductName: ptrString("test"),
			Key:         ptrString(rawKey),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		assert.NotContains(t, err.Error(), rawKey)
		assert.Contains(t, err.Error(), ioperlog.MaskAPIKeyToken(rawKey))
	})

	t.Run("duplicate key token dirty data masks key", func(t *testing.T) {
		rawKey := "testproduct-abcdef012345"
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
			fetchAPIKeyTokenListFn: func(ctx context.Context, filter *APIKeyTokenFilter) ([]*APIKeyTokenParam, error) {
				return []*APIKeyTokenParam{{Key: ptrString(rawKey)}, {Key: ptrString(rawKey)}}, nil
			},
		}
		m := newAPIKeyManager(store)
		err := m.CreateAPIKey(ctx, &APIKeyParam{
			ID:          ptrString("id1"),
			ProductName: ptrString("test"),
			Key:         ptrString(rawKey),
		})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), rawKey)
		assert.Contains(t, err.Error(), ioperlog.MaskAPIKeyToken(rawKey))
	})

	t.Run("create quota plan and route rules", func(t *testing.T) {
		createdKey := false
		quotaCreated := false
		rateLimitCreated := false
		routeCreated := false
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, nil
			},
			fetchAPIKeyTokenListFn: func(ctx context.Context, filter *APIKeyTokenFilter) ([]*APIKeyTokenParam, error) {
				return nil, nil
			},
			createAPIKeyFn: func(ctx context.Context, param *APIKeyParam) (int64, error) {
				createdKey = true
				return 1, nil
			},
		}
		quotaPlanStore := &fakeQuotaPlanStorager{
			createQuotaPlanFn: func(ctx context.Context, param *shared.QuotaPlanParam) (int64, error) {
				quotaCreated = true
				return 10, nil
			},
		}
		rateLimitStore := &fakeRateLimitPolicyStorager{
			createRateLimitPolicyFn: func(ctx context.Context, param *shared.RateLimitPolicyParam) (int64, error) {
				rateLimitCreated = true
				return 20, nil
			},
		}
		routeRulesStore := &fakeRouteRulesStorager{
			createRouteRulesFn: func(ctx context.Context, ruleType string, owner *string, param *shared.RouteRulesParam) (int64, error) {
				routeCreated = true
				assert.Equal(t, shared.RouteRulesTypeAPIKey, ruleType)
				return 30, nil
			},
		}
		m := NewAPIKeyManager(&fakeTxn{}, store, quotaPlanStore, rateLimitStore, routeRulesStore, &fakeEntityStorager{}, nil)
		err := m.CreateAPIKey(ctx, &APIKeyParam{
			ID:              ptrString("id1"),
			ProductName:     ptrString("test"),
			Key:             ptrString("key1"),
			QuotaPlan:       &shared.QuotaPlanParam{Quota: ptrFloat64(100)},
			RateLimitPolicy: &shared.RateLimitPolicyParam{Enabled: ptrBool(true)},
			RouteRules:      &shared.RouteRulesParam{Enabled: ptrBool(true)},
		})
		require.NoError(t, err)
		assert.True(t, createdKey)
		assert.True(t, quotaCreated)
		assert.True(t, rateLimitCreated)
		assert.True(t, routeCreated)
	})

	t.Run("fetch api key list error", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			fetchAPIKeyListFn: func(ctx context.Context, filter *APIKeyFilter) ([]*APIKeyParam, error) {
				return nil, errors.New("db down")
			},
		}
		m := newAPIKeyManager(store)
		err := m.CreateAPIKey(ctx, &APIKeyParam{ProductName: ptrString("test")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}

func TestAPIKeyManager_CreateAPIKeyToken(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		updated := false
		store := &fakeAPIKeyStorager{
			createAPIKeyTokenFn: func(ctx context.Context, param *APIKeyTokenParam) (int64, error) {
				assert.Equal(t, "key1", *param.Key)
				return 5, nil
			},
			updateAPIKeyTokenFn: func(ctx context.Context, filter *APIKeyTokenFilter, param *APIKeyTokenParam) error {
				updated = true
				assert.Equal(t, "key1-5", *param.Key)
				return nil
			},
		}
		m := newAPIKeyManager(store)
		id, err := m.CreateAPIKeyToken(ctx, &APIKeyTokenParam{Key: ptrString("key1"), CreatedAt: &fixedTestTime})
		require.NoError(t, err)
		assert.Equal(t, int64(5), id)
		assert.True(t, updated)
	})

	t.Run("create error", func(t *testing.T) {
		store := &fakeAPIKeyStorager{
			createAPIKeyTokenFn: func(ctx context.Context, param *APIKeyTokenParam) (int64, error) {
				return 0, errors.New("db down")
			},
		}
		m := newAPIKeyManager(store)
		_, err := m.CreateAPIKeyToken(ctx, &APIKeyTokenParam{Key: ptrString("key1"), CreatedAt: &fixedTestTime})
		require.Error(t, err)
	})
}
