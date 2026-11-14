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

package ai_cache

import (
	"context"
	"fmt"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
)

// ConfigTopicProductAICache is the configuration topic for mod_ai_cache.
const ConfigTopicProductAICache = "mod_ai_cache"

// ExportAICacheRule is the per-rule export structure consumed by the BFE
// mod_ai_cache rule loader (ProductRuleConfFile). The JSON tags are frozen
// contract (see design-docs/modifications/2026-09-24-ai-cache-rule-export
// design-changes.md section 4.2, extended by 2026-09-30-ai-cache-semantic-cache)
// and must stay verbatim in sync with BFE.
// Phase 1 exports only cond/cacheKeyStrategy/cacheTTL/maxBodyBytes/maxValueBytes;
// phase 2 adds enableSemanticCache; Cond is a non-pointer required field so a
// missing cond fails at compile time.
type ExportAICacheRule struct {
	Cond                string  `json:"cond"`
	CacheKeyStrategy    *string `json:"cacheKeyStrategy"`
	CacheTTL            *int    `json:"cacheTTL"`
	MaxBodyBytes        *int64  `json:"maxBodyBytes"`
	MaxValueBytes       *int64  `json:"maxValueBytes"`
	EnableSemanticCache *bool   `json:"enableSemanticCache"`
}

// ExportAICacheRuleConfig is the exported ai_cache.data payload. Version/
// Semantic/Config capitalized keys are the top-level contract with the BFE
// rule loader; Version/Config capitalized keys are the hard contract with
// conf-agent. Semantic is always exported (defaults when the settings table
// is empty).
type ExportAICacheRuleConfig struct {
	Version  string                          `json:"Version"`
	Semantic *SemanticConf                   `json:"Semantic"`
	Config   map[string][]*ExportAICacheRule `json:"Config"`
}

// UpdateVersion updates the configuration version.
func (conf *ExportAICacheRuleConfig) UpdateVersion(version string) error {
	conf.Version = version
	return nil
}

// AICacheManager manages the AI cache rule collection, the singleton semantic
// settings and the config export.
type AICacheManager struct {
	txn                     itxn.TxnStorager
	storager                AICacheStorager
	settingsStorager        AICacheSemanticSettingsStorager
	versionControlManager   *iversion_control.VersionControlManager
	operationLogManager     ioperlog.OperationLogRecorder
	aiRouteInnerProductName string
}

// NewAICacheManager creates a new AICacheManager. aiRouteInnerProductName is
// the runtime AI inner product name injected by the assembly point.
func NewAICacheManager(txn itxn.TxnStorager, storager AICacheStorager, settingsStorager AICacheSemanticSettingsStorager, versionControlManager *iversion_control.VersionControlManager, aiRouteInnerProductName string) *AICacheManager {
	return &AICacheManager{
		txn:                     txn,
		storager:                storager,
		settingsStorager:        settingsStorager,
		versionControlManager:   versionControlManager,
		aiRouteInnerProductName: aiRouteInnerProductName,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (m *AICacheManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// GetAICacheRules returns the whole rule set ordered by id ascending
// (first-match-wins priority order). An empty collection returns an empty slice.
func (m *AICacheManager) GetAICacheRules(ctx context.Context) ([]*shared.AICacheRuleParam, error) {
	rules, err := m.storager.FetchAll(ctx)
	if err != nil {
		return nil, err
	}

	rst := make([]*shared.AICacheRuleParam, 0, len(rules))
	for _, rule := range rules {
		rst = append(rst, aiCacheRuleParamToShared(rule))
	}

	return rst, nil
}

// SetAICacheRules atomically replaces the whole rule set (delete-all +
// insert-all in slice order, single transaction). A nil rules list clears the
// collection. The returned param is the re-read collection after the rebuild.
func (m *AICacheManager) SetAICacheRules(ctx context.Context, param *shared.AICacheRulesParam) (*shared.AICacheRulesParam, error) {
	var rules []*shared.AICacheRuleParam
	if param != nil {
		rules = param.Rules
	}
	if rules == nil {
		rules = []*shared.AICacheRuleParam{}
	}

	// Fetch the current collection for the audit snapshot; ignore errors here
	// because ReplaceAll will re-read inside its transaction.
	before, _ := m.storager.FetchAll(ctx)
	beforeMap := aiCacheRulesSnapshotToMap(before)

	replaceRules := make([]*AICacheRuleParam, 0, len(rules))
	for _, rule := range rules {
		replaceRules = append(replaceRules, aiCacheRuleParamFromShared(rule))
	}

	err := itxn.ExecuteWithRetry(ctx, m.txn, func(ctx context.Context) error {
		return m.storager.ReplaceAll(ctx, replaceRules)
	})
	if err != nil {
		m.recordAICacheRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiCacheRulesParamToMap(rules), err)
		return nil, err
	}

	after, err := m.storager.FetchAll(ctx)
	if err != nil {
		m.recordAICacheRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiCacheRulesParamToMap(rules), err)
		return nil, err
	}

	m.recordAICacheRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiCacheRulesSnapshotToMap(after), nil)

	rst := make([]*shared.AICacheRuleParam, 0, len(after))
	for _, rule := range after {
		rst = append(rst, aiCacheRuleParamToShared(rule))
	}

	return &shared.AICacheRulesParam{Rules: rst}, nil
}

// GetSemanticSettings returns the singleton semantic settings. An empty table
// yields the documented defaults (1 / 0.15 / lt, no timestamps); otherwise
// the stored row is returned with its timestamps.
func (m *AICacheManager) GetSemanticSettings(ctx context.Context) (*shared.AICacheSemanticSettingsParam, error) {
	row, err := m.settingsStorager.Get(ctx)
	if err != nil {
		return nil, err
	}

	return semanticSettingsRowToShared(row), nil
}

// SetSemanticSettings upserts the singleton semantic settings in a single
// transaction (delete-all + insert) and returns the re-read settings (same
// shape as GET). The audit before snapshot falls back to the documented
// defaults when the table is empty.
func (m *AICacheManager) SetSemanticSettings(ctx context.Context, param *shared.AICacheSemanticSettingsParam) (*shared.AICacheSemanticSettingsParam, error) {
	if param == nil {
		param = &shared.AICacheSemanticSettingsParam{}
	}

	before, _ := m.settingsStorager.Get(ctx)
	beforeMap := aiCacheSemanticSettingsToMap(semanticSettingsRowToShared(before))

	row := semanticSettingsRowFromShared(param)
	afterMap := aiCacheSemanticSettingsToMap(semanticSettingsRowToShared(row))

	err := m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		return m.settingsStorager.Upsert(ctx, row)
	})
	if err != nil {
		m.recordSemanticSettingsOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, afterMap, err)
		return nil, err
	}

	after, err := m.settingsStorager.Get(ctx)
	if err != nil {
		m.recordSemanticSettingsOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, afterMap, err)
		return nil, err
	}

	m.recordSemanticSettingsOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiCacheSemanticSettingsToMap(semanticSettingsRowToShared(after)), nil)

	return semanticSettingsRowToShared(after), nil
}

// ConfigExport exports the ai_cache.data payload for BFE mod_ai_cache. When
// the generated content signature matches the last exported version, it
// returns nil (incremental, HTTP Data is null).
func (m *AICacheManager) ConfigExport(ctx context.Context, lastVersion string) (*ExportAICacheRuleConfig, error) {
	rst, err := m.versionControlManager.ExportConfig(ctx, ConfigTopicProductAICache, m.AICacheRuleGenerator)
	if err != nil {
		return nil, err
	}

	if rst.DataWithoutVersion == nil {
		return nil, fmt.Errorf("AICacheRuleGenerator.DataWithoutVersion is nil")
	}

	conf, ok := rst.DataWithoutVersion.(*ExportAICacheRuleConfig)
	if ok {
		if conf.Version == lastVersion {
			return nil, nil
		}

		return conf, nil
	}

	return nil, fmt.Errorf("convert AICacheRuleGenerator.DataWithoutVersion to ExportAICacheRuleConfig is error")
}

// AICacheRuleGenerator generates the mod_ai_cache export data: the full rule
// set (id ascending, no enabled filter) keyed by the AI inner product name,
// plus the top-level Semantic block merged from the singleton settings
// (defaults when the settings table is empty; always exported). An empty rule
// table exports an empty array while the product key stays present.
func (m *AICacheManager) AICacheRuleGenerator(ctx context.Context) (*iversion_control.ExportData, error) {
	rules, err := m.storager.FetchAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch ai cache rules error: %s", err.Error())
	}

	settingsRow, err := m.settingsStorager.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch ai cache semantic settings error: %s", err.Error())
	}
	settings := semanticSettingsRowToShared(settingsRow)

	exportRules := make([]*ExportAICacheRule, 0, len(rules))
	for _, rule := range rules {
		exportRule := &ExportAICacheRule{
			CacheKeyStrategy:    rule.CacheKeyStrategy,
			CacheTTL:            rule.CacheTTL,
			MaxBodyBytes:        rule.MaxBodyBytes,
			MaxValueBytes:       rule.MaxValueBytes,
			EnableSemanticCache: rule.EnableSemanticCache,
		}
		if rule.Cond != nil {
			exportRule.Cond = *rule.Cond
		}
		exportRules = append(exportRules, exportRule)
	}

	conf := &ExportAICacheRuleConfig{
		Semantic: &SemanticConf{
			TopK:              settings.TopK,
			Threshold:         settings.Threshold,
			ThresholdRelation: settings.ThresholdRelation,
		},
		Config: map[string][]*ExportAICacheRule{
			m.aiRouteInnerProductName: exportRules,
		},
	}
	conf.UpdateVersion(iversion_control.ZeroVersion)

	return &iversion_control.ExportData{
		Topic:              ConfigTopicProductAICache,
		DataWithoutVersion: conf,
	}, nil
}
