//Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
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

package ai_context

import (
	"context"
	"fmt"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
)

// ConfigTopicProductAIContext is the configuration topic for mod_ai_context.
const ConfigTopicProductAIContext = "mod_ai_context"

// RuleConf is the per-rule export structure consumed by the BFE mod_ai_context
// rule loader (ContextRuleConfFile). The JSON tags are frozen contract (see
// design-docs/modifications/2026-10-01-ai-context-compress-control-plane
// design-changes.md section 4.3) and must stay verbatim in sync with BFE.
// Cond/Mode are non-pointer required fields so a missing cond/mode fails at
// compile time on the control plane side.
type RuleConf struct {
	Cond             string `json:"cond"`
	Mode             string `json:"mode"`
	MaxContextTokens *int   `json:"maxContextTokens"`
	ReserveTokens    *int   `json:"reserveTokens"`
}

// ExportContextRuleConfig is the exported context_rule.data payload.
// Version/Defaults/Config capitalized keys are the top-level contract with
// the BFE rule loader and the hard contract with conf-agent. Defaults is
// always exported (defaults when the settings table is empty) and
// participates in the MD5 signature.
type ExportContextRuleConfig struct {
	Version  string                 `json:"Version"`
	Defaults *DefaultsConf          `json:"Defaults"`
	Config   map[string][]*RuleConf `json:"Config"`
}

// UpdateVersion updates the configuration version.
func (conf *ExportContextRuleConfig) UpdateVersion(version string) error {
	conf.Version = version
	return nil
}

// AIContextManager manages the AI context rule collection, the singleton
// global settings and the config export.
type AIContextManager struct {
	txn                     itxn.TxnStorager
	storager                AIContextStorager
	settingsStorager        AIContextSettingsStorager
	versionControlManager   *iversion_control.VersionControlManager
	operationLogManager     ioperlog.OperationLogRecorder
	aiRouteInnerProductName string
}

// NewAIContextManager creates a new AIContextManager. aiRouteInnerProductName
// is the runtime AI inner product name injected by the assembly point.
func NewAIContextManager(txn itxn.TxnStorager, storager AIContextStorager, settingsStorager AIContextSettingsStorager, versionControlManager *iversion_control.VersionControlManager, aiRouteInnerProductName string) *AIContextManager {
	return &AIContextManager{
		txn:                     txn,
		storager:                storager,
		settingsStorager:        settingsStorager,
		versionControlManager:   versionControlManager,
		aiRouteInnerProductName: aiRouteInnerProductName,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (m *AIContextManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// GetRules returns the whole rule set ordered by id ascending
// (first-match-wins priority order). An empty collection returns an empty slice.
func (m *AIContextManager) GetRules(ctx context.Context) ([]*shared.AIContextRuleParam, error) {
	rules, err := m.storager.FetchAll(ctx)
	if err != nil {
		return nil, err
	}

	rst := make([]*shared.AIContextRuleParam, 0, len(rules))
	for _, rule := range rules {
		rst = append(rst, contextRuleRowToShared(rule))
	}

	return rst, nil
}

// SetRules atomically replaces the whole rule set (delete-all + insert-all in
// slice order, single transaction). A nil rules list clears the collection.
// The returned param is the re-read collection after the rebuild.
func (m *AIContextManager) SetRules(ctx context.Context, param *shared.AIContextRulesParam) (*shared.AIContextRulesParam, error) {
	var rules []*shared.AIContextRuleParam
	if param != nil {
		rules = param.Rules
	}
	if rules == nil {
		rules = []*shared.AIContextRuleParam{}
	}

	// Fetch the current collection for the audit snapshot; ignore errors here
	// because ReplaceAll will re-read inside its transaction.
	before, _ := m.storager.FetchAll(ctx)
	beforeMap := aiContextRulesSnapshotToMap(before)

	replaceRules := make([]*ContextRuleRow, 0, len(rules))
	for _, rule := range rules {
		replaceRules = append(replaceRules, contextRuleRowFromShared(rule))
	}

	err := itxn.ExecuteWithRetry(ctx, m.txn, func(ctx context.Context) error {
		return m.storager.ReplaceAll(ctx, replaceRules)
	})
	if err != nil {
		m.recordRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiContextRulesParamToMap(rules), err)
		return nil, err
	}

	after, err := m.storager.FetchAll(ctx)
	if err != nil {
		m.recordRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiContextRulesParamToMap(rules), err)
		return nil, err
	}

	m.recordRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiContextRulesSnapshotToMap(after), nil)

	rst := make([]*shared.AIContextRuleParam, 0, len(after))
	for _, rule := range after {
		rst = append(rst, contextRuleRowToShared(rule))
	}

	return &shared.AIContextRulesParam{Rules: rst}, nil
}

// GetSettings returns the singleton global settings. An empty table yields
// the documented defaults (0.7 / 2 / 2000 / trim-all-but-last / 4 / 1200 /
// lite / 0.95, no timestamps); otherwise the stored row is returned with its
// timestamps.
func (m *AIContextManager) GetSettings(ctx context.Context) (*shared.AIContextSettingsParam, error) {
	row, err := m.settingsStorager.Get(ctx)
	if err != nil {
		return nil, err
	}

	return settingsRowToShared(row), nil
}

// SetSettings upserts the singleton global settings in a single transaction
// (delete-all + insert) and returns the re-read settings (same shape as GET).
// The audit before snapshot falls back to the documented defaults when the
// table is empty.
func (m *AIContextManager) SetSettings(ctx context.Context, param *shared.AIContextSettingsParam) (*shared.AIContextSettingsParam, error) {
	if param == nil {
		param = &shared.AIContextSettingsParam{}
	}

	before, _ := m.settingsStorager.Get(ctx)
	beforeMap := aiContextSettingsToMap(settingsRowToShared(before))

	row := settingsRowFromShared(param)
	afterMap := aiContextSettingsToMap(settingsRowToShared(row))

	err := m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		return m.settingsStorager.Upsert(ctx, row)
	})
	if err != nil {
		m.recordSettingsOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, afterMap, err)
		return nil, err
	}

	after, err := m.settingsStorager.Get(ctx)
	if err != nil {
		m.recordSettingsOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, afterMap, err)
		return nil, err
	}

	m.recordSettingsOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, aiContextSettingsToMap(settingsRowToShared(after)), nil)

	return settingsRowToShared(after), nil
}

// ConfigExport exports the context_rule.data payload for BFE mod_ai_context.
// When the generated content signature matches the last exported version, it
// returns nil (incremental, HTTP Data is null).
func (m *AIContextManager) ConfigExport(ctx context.Context, lastVersion string) (*ExportContextRuleConfig, error) {
	rst, err := m.versionControlManager.ExportConfig(ctx, ConfigTopicProductAIContext, m.ContextRuleGenerator)
	if err != nil {
		return nil, err
	}

	if rst.DataWithoutVersion == nil {
		return nil, fmt.Errorf("ContextRuleGenerator.DataWithoutVersion is nil")
	}

	conf, ok := rst.DataWithoutVersion.(*ExportContextRuleConfig)
	if ok {
		if conf.Version == lastVersion {
			return nil, nil
		}

		return conf, nil
	}

	return nil, fmt.Errorf("convert ContextRuleGenerator.DataWithoutVersion to ExportContextRuleConfig is error")
}

// ContextRuleGenerator generates the mod_ai_context export data: the full
// rule set (id ascending, no enabled filter) keyed by the AI inner product
// name, plus the top-level Defaults block merged from the singleton settings
// (defaults when the settings table is empty; always exported, participating
// in the MD5 signature). An empty rule table exports an empty array while the
// product key stays present.
func (m *AIContextManager) ContextRuleGenerator(ctx context.Context) (*iversion_control.ExportData, error) {
	rules, err := m.storager.FetchAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch ai context rules error: %s", err.Error())
	}

	settingsRow, err := m.settingsStorager.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch ai context settings error: %s", err.Error())
	}

	exportRules := make([]*RuleConf, 0, len(rules))
	for _, rule := range rules {
		exportRule := &RuleConf{
			MaxContextTokens: rule.MaxContextTokens,
			ReserveTokens:    rule.ReserveTokens,
		}
		if rule.Cond != nil {
			exportRule.Cond = *rule.Cond
		}
		if rule.Mode != nil {
			exportRule.Mode = *rule.Mode
		}
		exportRules = append(exportRules, exportRule)
	}

	conf := &ExportContextRuleConfig{
		Defaults: defaultsConfFromSettingsRow(settingsRow),
		Config: map[string][]*RuleConf{
			m.aiRouteInnerProductName: exportRules,
		},
	}
	conf.UpdateVersion(iversion_control.ZeroVersion)

	return &iversion_control.ExportData{
		Topic:              ConfigTopicProductAIContext,
		DataWithoutVersion: conf,
	}, nil
}
