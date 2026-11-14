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

package ai_cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storageRule(id int64, name, cond string) *AICacheRuleParam {
	return &AICacheRuleParam{
		ID:               lib.PInt64(id),
		Name:             lib.PString(name),
		Cond:             lib.PString(cond),
		CacheKeyStrategy: lib.PString("lastQuestion"),
		CacheTTL:         lib.PInt(3600),
		MaxBodyBytes:     lib.PInt64(1048576),
		MaxValueBytes:    lib.PInt64(1048576),
	}
}

func sharedRule(name, cond string) *shared.AICacheRuleParam {
	return &shared.AICacheRuleParam{
		Name: lib.PString(name),
		Cond: lib.PString(cond),
	}
}

func TestAICacheManager_GetAICacheRules(t *testing.T) {
	ctx := context.Background()

	t.Run("returns rules in storage order", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{
					storageRule(1, "rule-a", "default_t()"),
					storageRule(2, "rule-b", "req_path_in(\"/v1\", false)"),
				}, nil
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		rules, err := m.GetAICacheRules(ctx)
		require.NoError(t, err)
		require.Len(t, rules, 2)
		assert.Equal(t, "rule-a", *rules[0].Name)
		assert.Equal(t, "rule-b", *rules[1].Name)
		assert.Equal(t, 3600, *rules[0].CacheTTL)
	})

	t.Run("empty collection returns empty slice", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{}, nil
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		rules, err := m.GetAICacheRules(ctx)
		require.NoError(t, err)
		assert.NotNil(t, rules)
		assert.Len(t, rules, 0)
	})

	t.Run("storage error propagates", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return nil, errors.New("db down")
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		_, err := m.GetAICacheRules(ctx)
		require.Error(t, err)
	})
}

func TestAICacheManager_SetAICacheRules(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces collection in slice order with defaults filled", func(t *testing.T) {
		var replaced [][]*AICacheRuleParam
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{}, nil
			},
			replaceAllFn: func(ctx context.Context, rules []*AICacheRuleParam) error {
				replaced = append(replaced, rules)
				return nil
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		updated, err := m.SetAICacheRules(ctx, &shared.AICacheRulesParam{
			Rules: []*shared.AICacheRuleParam{
				sharedRule("first", "default_t()"),
				{
					Name:             lib.PString("second"),
					Cond:             lib.PString("req_path_in(\"/v1\", false)"),
					CacheKeyStrategy: lib.PString("allQuestions"),
					CacheTTL:         lib.PInt(0),
					MaxBodyBytes:     lib.PInt64(2048),
					MaxValueBytes:    lib.PInt64(4096),
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, updated)

		// ReplaceAll must be invoked exactly once, in array order.
		require.Len(t, replaced, 1)
		require.Len(t, replaced[0], 2)
		assert.Equal(t, "first", *replaced[0][0].Name)
		assert.Equal(t, "second", *replaced[0][1].Name)

		// Omitted optional fields are filled with documented defaults.
		assert.Equal(t, DefaultCacheKeyStrategy, *replaced[0][0].CacheKeyStrategy)
		assert.Equal(t, DefaultCacheTTL, *replaced[0][0].CacheTTL)
		assert.Equal(t, int64(DefaultMaxBodyBytes), *replaced[0][0].MaxBodyBytes)
		assert.Equal(t, int64(DefaultMaxValueBytes), *replaced[0][0].MaxValueBytes)

		// Explicit values are preserved.
		assert.Equal(t, "allQuestions", *replaced[0][1].CacheKeyStrategy)
		assert.Equal(t, int64(2048), *replaced[0][1].MaxBodyBytes)
	})

	t.Run("nil param and nil rules clear the collection", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		_, err := m.SetAICacheRules(ctx, nil)
		require.NoError(t, err)
		_, err = m.SetAICacheRules(ctx, &shared.AICacheRulesParam{})
		require.NoError(t, err)

		require.Len(t, store.replaceAllCalls, 2)
		assert.Len(t, store.replaceAllCalls[0], 0)
		assert.Len(t, store.replaceAllCalls[1], 0)
	})

	t.Run("replace failure rolls back and records failed audit", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		store := &fakeAICacheRuleStorager{
			replaceAllFn: func(ctx context.Context, rules []*AICacheRuleParam) error {
				return errors.New("replace error")
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		_, err := m.SetAICacheRules(ctx, &shared.AICacheRulesParam{
			Rules: []*shared.AICacheRuleParam{sharedRule("r1", "default_t()")},
		})
		require.Error(t, err)

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
		assert.Equal(t, string(ioperlog.ResourceTypeAICacheRule), entry.ResourceType)
		assert.Equal(t, ioperlog.StatusFailed, entry.Status)
		assert.NotEmpty(t, entry.ErrorMsg)
	})

	t.Run("deadlock retries then surfaces a 409 conflict", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		store := &fakeAICacheRuleStorager{
			replaceAllFn: func(ctx context.Context, rules []*AICacheRuleParam) error {
				return errors.New("Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction")
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		_, err := m.SetAICacheRules(ctx, &shared.AICacheRulesParam{
			Rules: []*shared.AICacheRuleParam{sharedRule("r1", "default_t()")},
		})
		require.Error(t, err)

		// The replacement is retried and never surfaces as a database error.
		assert.Len(t, store.replaceAllCalls, 3)
		rr := xerror.Resolve(err)
		assert.Equal(t, 409, rr.ErrNo)
		assert.Equal(t, "Conflict", rr.Type)

		// Only the final outcome is audited, not each retry.
		require.Len(t, recorder.entries, 1)
		assert.Equal(t, ioperlog.StatusFailed, recorder.entries[0].Status)
	})

	t.Run("records successful audit with collection snapshots", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		calls := 0
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				calls++
				if calls == 1 {
					return []*AICacheRuleParam{storageRule(1, "old-rule", "default_t()")}, nil
				}
				return []*AICacheRuleParam{storageRule(2, "new-rule", "default_t()")}, nil
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		_, err := m.SetAICacheRules(ctx, &shared.AICacheRulesParam{
			Rules: []*shared.AICacheRuleParam{sharedRule("new-rule", "default_t()")},
		})
		require.NoError(t, err)

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, ioperlog.StatusSuccess, entry.Status)
		assert.Empty(t, entry.ErrorMsg)

		before, ok := entry.ChangeSummary["before"].(map[string]interface{})
		require.True(t, ok)
		beforeRules, ok := before["rules"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, beforeRules, 1)
		assert.Equal(t, "old-rule", beforeRules[0]["name"])

		after, ok := entry.ChangeSummary["after"].(map[string]interface{})
		require.True(t, ok)
		afterRules, ok := after["rules"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, afterRules, 1)
		assert.Equal(t, "new-rule", afterRules[0]["name"])
		// Snapshot keys use the API lowercase vocabulary; internal id and the
		// per-rule timestamps are absent (timestamps dropped in issue #258).
		_, hasID := afterRules[0]["id"]
		assert.False(t, hasID)
		_, hasCreatedAt := afterRules[0]["created_at"]
		assert.False(t, hasCreatedAt)
		_, hasUpdatedAt := afterRules[0]["updated_at"]
		assert.False(t, hasUpdatedAt)
	})

	t.Run("records failed audit for validation rejection", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{storageRule(1, "old-rule", "default_t()")}, nil
			},
		}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		m.RecordSetAICacheRulesFailure(ctx, &shared.AICacheRulesParam{
			Rules: []*shared.AICacheRuleParam{sharedRule("bad-rule", "cond ! compile")},
		}, errors.New("cond.Build(): syntax error"))

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
		assert.Equal(t, string(ioperlog.ResourceTypeAICacheRule), entry.ResourceType)
		assert.Equal(t, aiCacheRulesResourceID, entry.ResourceID)
		assert.Equal(t, ioperlog.StatusFailed, entry.Status)
		assert.NotEmpty(t, entry.ErrorMsg)

		// Identity and before snapshot come from storage, not the request.
		before, ok := entry.ChangeSummary["before"].(map[string]interface{})
		require.True(t, ok)
		beforeRules, ok := before["rules"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, beforeRules, 1)
		assert.Equal(t, "old-rule", beforeRules[0]["name"])

		after, ok := entry.ChangeSummary["after"].(map[string]interface{})
		require.True(t, ok)
		afterRules, ok := after["rules"].([]map[string]interface{})
		require.True(t, ok)
		require.Len(t, afterRules, 1)
		assert.Equal(t, "bad-rule", afterRules[0]["name"])
	})

	t.Run("no audit when operation log manager is nil", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{}
		m := NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		_, err := m.SetAICacheRules(ctx, &shared.AICacheRulesParam{
			Rules: []*shared.AICacheRuleParam{sharedRule("r1", "default_t()")},
		})
		require.NoError(t, err)
	})
}

func TestAICacheManager_AICacheRuleGenerator(t *testing.T) {
	ctx := context.Background()

	newManager := func(store *fakeAICacheRuleStorager, product string) *AICacheManager {
		return NewAICacheManager(&fakeTxn{}, store, &fakeAICacheSemanticSettingsStorager{}, nil, product)
	}

	t.Run("exports rules in id ascending order under injected product name", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{
					storageRule(1, "first", "default_t()"),
					storageRule(2, "second", "req_path_in(\"/v1\", false)"),
				}, nil
			},
		}
		m := newManager(store, "CustomProduct")

		data, err := m.AICacheRuleGenerator(ctx)
		require.NoError(t, err)
		assert.Equal(t, ConfigTopicProductAICache, data.Topic)

		conf, ok := data.DataWithoutVersion.(*ExportAICacheRuleConfig)
		require.True(t, ok)
		require.Contains(t, conf.Config, "CustomProduct")
		require.Len(t, conf.Config["CustomProduct"], 2)
		assert.Equal(t, "default_t()", conf.Config["CustomProduct"][0].Cond)
		assert.Equal(t, "req_path_in(\"/v1\", false)", conf.Config["CustomProduct"][1].Cond)
		assert.Equal(t, iversion_control.ZeroVersion, conf.Version)
	})

	t.Run("empty table exports empty array with product key present", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{}, nil
			},
		}
		m := newManager(store, "AI_product")

		data, err := m.AICacheRuleGenerator(ctx)
		require.NoError(t, err)
		conf := data.DataWithoutVersion.(*ExportAICacheRuleConfig)

		rules, ok := conf.Config["AI_product"]
		require.True(t, ok)
		require.NotNil(t, rules)
		assert.Len(t, rules, 0)

		bs, err := json.Marshal(conf)
		require.NoError(t, err)
		assert.Contains(t, string(bs), `"AI_product":[]`)
	})

	t.Run("marshaled JSON keys match the frozen contract verbatim", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return []*AICacheRuleParam{storageRule(1, "r1", "default_t()")}, nil
			},
		}
		m := newManager(store, "AI_product")

		data, err := m.AICacheRuleGenerator(ctx)
		require.NoError(t, err)
		conf := data.DataWithoutVersion.(*ExportAICacheRuleConfig)
		conf.UpdateVersion("20260924103000")

		bs, err := json.Marshal(conf)
		require.NoError(t, err)

		var top map[string]interface{}
		require.NoError(t, json.Unmarshal(bs, &top))
		assert.ElementsMatch(t, []string{"Version", "Semantic", "Config"}, keysOf(top))

		semantic := top["Semantic"].(map[string]interface{})
		assert.ElementsMatch(t, []string{"topK", "threshold", "thresholdRelation"}, keysOf(semantic))

		config := top["Config"].(map[string]interface{})
		assert.ElementsMatch(t, []string{"AI_product"}, keysOf(config))

		rules := config["AI_product"].([]interface{})
		require.Len(t, rules, 1)
		rule := rules[0].(map[string]interface{})
		assert.ElementsMatch(t,
			[]string{"cond", "cacheKeyStrategy", "cacheTTL", "maxBodyBytes", "maxValueBytes", "enableSemanticCache"},
			keysOf(rule))
	})

	t.Run("storage error propagates", func(t *testing.T) {
		store := &fakeAICacheRuleStorager{
			fetchAllFn: func(ctx context.Context) ([]*AICacheRuleParam, error) {
				return nil, errors.New("db down")
			},
		}
		m := newManager(store, "AI_product")

		_, err := m.AICacheRuleGenerator(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetch ai cache rules error")
	})
}

func TestAICacheManager_ConfigExport(t *testing.T) {
	ctx := context.Background()

	buildManager := func(versionStore *fakeVersionControlStorager, product string) *AICacheManager {
		var vcm *iversion_control.VersionControlManager
		if versionStore != nil {
			vcm = iversion_control.NewVersionControllerManager(&fakeTxn{}, versionStore)
		}
		return NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, &fakeAICacheSemanticSettingsStorager{}, vcm, product)
	}

	t.Run("returns nil when version unchanged", func(t *testing.T) {
		versionStore := &fakeVersionControlStorager{
			upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
				return iversion_control.ZeroVersion, nil
			},
		}
		m := buildManager(versionStore, "AI_product")

		conf, err := m.ConfigExport(ctx, iversion_control.ZeroVersion)
		require.NoError(t, err)
		assert.Nil(t, conf)
	})

	t.Run("returns config with new version", func(t *testing.T) {
		versionStore := &fakeVersionControlStorager{
			upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
				return "20260924103000", nil
			},
		}
		m := buildManager(versionStore, "AI_product")

		conf, err := m.ConfigExport(ctx, iversion_control.ZeroVersion)
		require.NoError(t, err)
		require.NotNil(t, conf)
		assert.Equal(t, "20260924103000", conf.Version)
		assert.Contains(t, conf.Config, "AI_product")
	})

	t.Run("version control error propagates", func(t *testing.T) {
		versionStore := &fakeVersionControlStorager{
			upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
				return "", errors.New("version db error")
			},
		}
		m := buildManager(versionStore, "AI_product")

		_, err := m.ConfigExport(ctx, iversion_control.ZeroVersion)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "version db error")
	})
}

func TestAICacheRuleParam_Conversions(t *testing.T) {
	t.Run("nil inputs stay nil", func(t *testing.T) {
		assert.Nil(t, aiCacheRuleParamFromShared(nil))
		assert.Nil(t, aiCacheRuleParamToShared(nil))
	})

	t.Run("nil optional fields are filled with defaults", func(t *testing.T) {
		rule := aiCacheRuleParamFromShared(sharedRule("r1", "default_t()"))
		require.NotNil(t, rule)
		assert.Equal(t, "r1", *rule.Name)
		assert.Equal(t, DefaultCacheKeyStrategy, *rule.CacheKeyStrategy)
		assert.Equal(t, DefaultCacheTTL, *rule.CacheTTL)
		assert.Equal(t, int64(DefaultMaxBodyBytes), *rule.MaxBodyBytes)
		assert.Equal(t, int64(DefaultMaxValueBytes), *rule.MaxValueBytes)
	})

	t.Run("round trip preserves business fields", func(t *testing.T) {
		original := &shared.AICacheRuleParam{
			Name:                lib.PString("r1"),
			Cond:                lib.PString("default_t()"),
			CacheKeyStrategy:    lib.PString("disabled"),
			CacheTTL:            lib.PInt(60),
			MaxBodyBytes:        lib.PInt64(1024),
			MaxValueBytes:       lib.PInt64(2048),
			EnableSemanticCache: lib.PBool(true),
		}

		rule := aiCacheRuleParamFromShared(original)
		rst := aiCacheRuleParamToShared(rule)
		assert.Equal(t, original, rst)
	})
}

// TestAICacheRuleJSONContract locks the response vocabulary of a rule: the
// per-rule timestamps are intentionally absent (full-replace resource, issue
// #258). Re-adding either field flips this test before it can leak into the
// wire contract.
func TestAICacheRuleJSONContract(t *testing.T) {
	rule := &shared.AICacheRuleParam{
		Name:                lib.PString("r1"),
		Cond:                lib.PString("default_t()"),
		CacheKeyStrategy:    lib.PString("lastQuestion"),
		CacheTTL:            lib.PInt(0),
		MaxBodyBytes:        lib.PInt64(1048576),
		MaxValueBytes:       lib.PInt64(1048576),
		EnableSemanticCache: lib.PBool(false),
	}

	bs, err := json.Marshal(rule)
	require.NoError(t, err)

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(bs, &m))
	assert.ElementsMatch(t, []string{
		"name", "cond", "cache_key_strategy", "cache_ttl",
		"max_body_bytes", "max_value_bytes", "enable_semantic_cache",
	}, keysOf(m), "rule vocabulary must be exactly the 7 business fields (issue #258)")
}

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func semanticSettingsRow(topK int, threshold float64, relation string) *SemanticSettingsRow {
	now := time.Date(2026, 9, 30, 10, 30, 0, 0, time.UTC)
	return &SemanticSettingsRow{
		ID:                lib.PInt64(1),
		TopK:              lib.PInt(topK),
		Threshold:         lib.PFloat64(threshold),
		ThresholdRelation: lib.PString(relation),
		CreatedAt:         &now,
		UpdatedAt:         &now,
	}
}

func TestAICacheManager_GetSemanticSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("empty table returns documented defaults without timestamps", func(t *testing.T) {
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, &fakeAICacheSemanticSettingsStorager{}, nil, "AI_product")

		settings, err := m.GetSemanticSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)
		assert.Equal(t, DefaultSemanticTopK, *settings.TopK)
		assert.Equal(t, DefaultSemanticThreshold, *settings.Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *settings.ThresholdRelation)
		assert.Nil(t, settings.CreatedAt)
		assert.Nil(t, settings.UpdatedAt)
	})

	t.Run("existing row returns stored values with timestamps", func(t *testing.T) {
		store := &fakeAICacheSemanticSettingsStorager{
			getFn: func(ctx context.Context) (*SemanticSettingsRow, error) {
				return semanticSettingsRow(5, 0.5, "gte"), nil
			},
		}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")

		settings, err := m.GetSemanticSettings(ctx)
		require.NoError(t, err)
		require.NotNil(t, settings)
		assert.Equal(t, 5, *settings.TopK)
		assert.Equal(t, 0.5, *settings.Threshold)
		assert.Equal(t, "gte", *settings.ThresholdRelation)
		assert.NotNil(t, settings.CreatedAt)
		assert.NotNil(t, settings.UpdatedAt)
	})

	t.Run("storage error propagates", func(t *testing.T) {
		store := &fakeAICacheSemanticSettingsStorager{
			getFn: func(ctx context.Context) (*SemanticSettingsRow, error) {
				return nil, errors.New("db down")
			},
		}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")

		_, err := m.GetSemanticSettings(ctx)
		require.Error(t, err)
	})
}

func TestAICacheManager_SetSemanticSettings(t *testing.T) {
	ctx := context.Background()

	t.Run("insert on empty table fills defaults for omitted fields", func(t *testing.T) {
		store := &fakeAICacheSemanticSettingsStorager{}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")

		updated, err := m.SetSemanticSettings(ctx, &shared.AICacheSemanticSettingsParam{
			Threshold: lib.PFloat64(0.5),
		})
		require.NoError(t, err)
		require.NotNil(t, updated)
		assert.Equal(t, DefaultSemanticTopK, *updated.TopK)
		assert.Equal(t, 0.5, *updated.Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *updated.ThresholdRelation)

		// Upsert invoked exactly once, inside the transaction, with the full row.
		require.Len(t, store.upsertCalls, 1)
		assert.Equal(t, DefaultSemanticTopK, *store.upsertCalls[0].TopK)
		assert.Equal(t, 0.5, *store.upsertCalls[0].Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *store.upsertCalls[0].ThresholdRelation)
	})

	t.Run("explicit values are preserved", func(t *testing.T) {
		store := &fakeAICacheSemanticSettingsStorager{}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")

		updated, err := m.SetSemanticSettings(ctx, &shared.AICacheSemanticSettingsParam{
			TopK:              lib.PInt(10),
			Threshold:         lib.PFloat64(2),
			ThresholdRelation: lib.PString("lte"),
		})
		require.NoError(t, err)
		assert.Equal(t, 10, *updated.TopK)
		assert.Equal(t, 2.0, *updated.Threshold)
		assert.Equal(t, "lte", *updated.ThresholdRelation)
	})

	t.Run("nil param upserts the documented defaults", func(t *testing.T) {
		store := &fakeAICacheSemanticSettingsStorager{}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")

		updated, err := m.SetSemanticSettings(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, DefaultSemanticTopK, *updated.TopK)
		assert.Equal(t, DefaultSemanticThreshold, *updated.Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *updated.ThresholdRelation)
		require.Len(t, store.upsertCalls, 1)
	})

	t.Run("records successful audit with default before snapshot on empty table", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		store := &fakeAICacheSemanticSettingsStorager{}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		_, err := m.SetSemanticSettings(ctx, &shared.AICacheSemanticSettingsParam{
			TopK: lib.PInt(3),
		})
		require.NoError(t, err)

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
		assert.Equal(t, string(ioperlog.ResourceTypeAICacheSemanticSettings), entry.ResourceType)
		assert.Equal(t, aiCacheSemanticSettingsResourceID, entry.ResourceID)
		assert.Equal(t, ioperlog.StatusSuccess, entry.Status)

		before, ok := entry.ChangeSummary["before"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, DefaultSemanticTopK, before["top_k"])
		assert.Equal(t, DefaultSemanticThreshold, before["threshold"])
		assert.Equal(t, DefaultSemanticThresholdRelation, before["threshold_relation"])

		after, ok := entry.ChangeSummary["after"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, 3, after["top_k"])
	})

	t.Run("records failed audit when upsert errors", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		store := &fakeAICacheSemanticSettingsStorager{
			upsertFn: func(ctx context.Context, row *SemanticSettingsRow) error {
				return errors.New("upsert error")
			},
		}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		_, err := m.SetSemanticSettings(ctx, &shared.AICacheSemanticSettingsParam{})
		require.Error(t, err)

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, ioperlog.StatusFailed, entry.Status)
		assert.NotEmpty(t, entry.ErrorMsg)
	})

	t.Run("records failed audit for validation rejection", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}
		store := &fakeAICacheSemanticSettingsStorager{}
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, store, nil, "AI_product")
		m.SetOperationLogManager(recorder)

		m.RecordSetSemanticSettingsFailure(ctx, &shared.AICacheSemanticSettingsParam{
			TopK: lib.PInt(100),
		}, errors.New("top_k out of range"))

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
		assert.Equal(t, string(ioperlog.ResourceTypeAICacheSemanticSettings), entry.ResourceType)
		assert.Equal(t, ioperlog.StatusFailed, entry.Status)

		after, ok := entry.ChangeSummary["after"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, 100, after["top_k"])
	})
}

func TestAICacheManager_AICacheRuleGeneratorSemantic(t *testing.T) {
	ctx := context.Background()

	generate := func(t *testing.T, settingsStore *fakeAICacheSemanticSettingsStorager) *ExportAICacheRuleConfig {
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, settingsStore, nil, "AI_product")
		data, err := m.AICacheRuleGenerator(ctx)
		require.NoError(t, err)
		conf, ok := data.DataWithoutVersion.(*ExportAICacheRuleConfig)
		require.True(t, ok)
		return conf
	}

	t.Run("empty settings table exports documented defaults", func(t *testing.T) {
		conf := generate(t, &fakeAICacheSemanticSettingsStorager{})

		require.NotNil(t, conf.Semantic)
		assert.Equal(t, DefaultSemanticTopK, *conf.Semantic.TopK)
		assert.Equal(t, DefaultSemanticThreshold, *conf.Semantic.Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *conf.Semantic.ThresholdRelation)
	})

	t.Run("existing settings row exports stored values", func(t *testing.T) {
		conf := generate(t, &fakeAICacheSemanticSettingsStorager{
			getFn: func(ctx context.Context) (*SemanticSettingsRow, error) {
				return semanticSettingsRow(4, 0.8, "gt"), nil
			},
		})

		require.NotNil(t, conf.Semantic)
		assert.Equal(t, 4, *conf.Semantic.TopK)
		assert.Equal(t, 0.8, *conf.Semantic.Threshold)
		assert.Equal(t, "gt", *conf.Semantic.ThresholdRelation)
	})

	t.Run("Semantic block is still exported when the rule set is empty", func(t *testing.T) {
		conf := generate(t, &fakeAICacheSemanticSettingsStorager{})
		require.NotNil(t, conf.Semantic)
		assert.Empty(t, conf.Config["AI_product"])
	})

	t.Run("settings change drives the export signature", func(t *testing.T) {
		signs := map[string]string{}
		for name, relation := range map[string]string{"defaults": "lt", "custom": "gte"} {
			var captured *iversion_control.ExportData
			versionStore := &fakeVersionControlStorager{
				upsertFn: func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
					captured = css
					return "20260930103000", nil
				},
			}
			vcm := iversion_control.NewVersionControllerManager(&fakeTxn{}, versionStore)

			threshold := DefaultSemanticThreshold
			if relation == "gte" {
				threshold = 0.9
			}
			settingsStore := &fakeAICacheSemanticSettingsStorager{
				getFn: func(ctx context.Context) (*SemanticSettingsRow, error) {
					return semanticSettingsRow(DefaultSemanticTopK, threshold, relation), nil
				},
			}
			m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, settingsStore, vcm, "AI_product")

			_, err := m.ConfigExport(ctx, iversion_control.ZeroVersion)
			require.NoError(t, err)
			require.NotNil(t, captured)
			signs[name] = captured.DataSignWithoutVersion
		}

		assert.NotEqual(t, signs["defaults"], signs["custom"])
	})

	t.Run("settings fetch error propagates", func(t *testing.T) {
		m := NewAICacheManager(&fakeTxn{}, &fakeAICacheRuleStorager{}, &fakeAICacheSemanticSettingsStorager{
			getFn: func(ctx context.Context) (*SemanticSettingsRow, error) {
				return nil, errors.New("db down")
			},
		}, nil, "AI_product")

		_, err := m.AICacheRuleGenerator(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fetch ai cache semantic settings error")
	})
}

func TestSemanticSettingsRow_Conversions(t *testing.T) {
	t.Run("nil row converts to documented defaults", func(t *testing.T) {
		param := semanticSettingsRowToShared(nil)
		require.NotNil(t, param)
		assert.Equal(t, DefaultSemanticTopK, *param.TopK)
		assert.Equal(t, DefaultSemanticThreshold, *param.Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *param.ThresholdRelation)
	})

	t.Run("nil param stays nil and nil fields are filled with defaults", func(t *testing.T) {
		assert.Nil(t, semanticSettingsRowFromShared(nil))

		row := semanticSettingsRowFromShared(&shared.AICacheSemanticSettingsParam{
			TopK: lib.PInt(7),
		})
		require.NotNil(t, row)
		assert.Equal(t, 7, *row.TopK)
		assert.Equal(t, DefaultSemanticThreshold, *row.Threshold)
		assert.Equal(t, DefaultSemanticThresholdRelation, *row.ThresholdRelation)
	})

	t.Run("round trip preserves values", func(t *testing.T) {
		original := semanticSettingsRow(2, 1.5, "lte")
		rst := semanticSettingsRowToShared(semanticSettingsRowFromShared(semanticSettingsRowToShared(original)))
		assert.Equal(t, original.TopK, rst.TopK)
		assert.Equal(t, original.Threshold, rst.Threshold)
		assert.Equal(t, original.ThresholdRelation, rst.ThresholdRelation)
		assert.Equal(t, original.CreatedAt, rst.CreatedAt)
		assert.Equal(t, original.UpdatedAt, rst.UpdatedAt)
	})
}
