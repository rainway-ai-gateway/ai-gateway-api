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

package traffic_mirror

import (
	"context"
	"fmt"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
)

// ConfigTopicProductTrafficMirror is the configuration topic for mod_traffic_mirror.
const ConfigTopicProductTrafficMirror = "mod_traffic_mirror"

// TrafficMirrorBodyRewriteConfFile is the per-rewrite export structure consumed
// by the BFE mod_traffic_mirror rule loader (MirrorBodyRewriteConfFile). The
// JSON tags are frozen contract (see design-docs/modifications/2026-09-25
// -traffic-mirror-rule-export design-changes.md section 4.2) and must stay
// verbatim in sync with BFE.
type TrafficMirrorBodyRewriteConfFile struct {
	Path  *string `json:"path"`
	Value *string `json:"value"`
}

// TrafficMirrorRuleConfFile is the per-rule export structure consumed by the
// BFE mod_traffic_mirror rule loader (MirrorRuleConfFile). The JSON tags are
// frozen contract (see design-docs/modifications/2026-09-25-traffic-mirror
// -rule-export design-changes.md section 4.2) and must stay verbatim in sync
// with BFE. No omitempty anywhere: every rule field is always exported so the
// artifact stays explicit and diff-able.
type TrafficMirrorRuleConfFile struct {
	Cond          *string                             `json:"cond"`
	MirrorCluster *string                             `json:"mirrorCluster"`
	Percentage    *int                                `json:"percentage"`
	RemoveHeaders []string                            `json:"removeHeaders"`
	SetHeaders    map[string]string                   `json:"setHeaders"`
	BodyRewrites  []*TrafficMirrorBodyRewriteConfFile `json:"bodyRewrites"`
	PathRewrite   *string                             `json:"pathRewrite"`
}

// ExportTrafficMirrorRuleConfig is the exported mirror_rule.data payload.
// Version/Config capitalized keys are the hard contract with conf-agent.
type ExportTrafficMirrorRuleConfig struct {
	Version *string                                   `json:"Version"`
	Config  *map[string]*[]*TrafficMirrorRuleConfFile `json:"Config"`
}

// UpdateVersion updates the configuration version.
func (conf *ExportTrafficMirrorRuleConfig) UpdateVersion(version string) error {
	conf.Version = &version
	return nil
}

// TrafficMirrorManager manages the traffic mirror rule collection and its
// config export.
type TrafficMirrorManager struct {
	txn                     itxn.TxnStorager
	storager                TrafficMirrorStorager
	versionControlManager   *iversion_control.VersionControlManager
	operationLogManager     ioperlog.OperationLogRecorder
	aiRouteInnerProductName string
}

// NewTrafficMirrorManager creates a new TrafficMirrorManager.
// aiRouteInnerProductName is the runtime AI inner product name injected by the
// assembly point.
func NewTrafficMirrorManager(txn itxn.TxnStorager, storager TrafficMirrorStorager, versionControlManager *iversion_control.VersionControlManager, aiRouteInnerProductName string) *TrafficMirrorManager {
	return &TrafficMirrorManager{
		txn:                     txn,
		storager:                storager,
		versionControlManager:   versionControlManager,
		aiRouteInnerProductName: aiRouteInnerProductName,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (m *TrafficMirrorManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// GetTrafficMirrorRules returns the whole rule set ordered by id ascending
// (first-match-wins priority order). An empty collection returns an empty slice.
func (m *TrafficMirrorManager) GetTrafficMirrorRules(ctx context.Context) ([]*shared.TrafficMirrorRuleParam, error) {
	rules, err := m.storager.FetchAll(ctx)
	if err != nil {
		return nil, err
	}

	rst := make([]*shared.TrafficMirrorRuleParam, 0, len(rules))
	for _, rule := range rules {
		rst = append(rst, trafficMirrorRuleParamToShared(rule))
	}

	return rst, nil
}

// SetTrafficMirrorRules atomically replaces the whole rule set (delete-all +
// insert-all in slice order, single transaction). A nil rules list clears the
// collection. The returned param is the re-read collection after the rebuild.
func (m *TrafficMirrorManager) SetTrafficMirrorRules(ctx context.Context, param *shared.TrafficMirrorRulesParam) (*shared.TrafficMirrorRulesParam, error) {
	var rules []*shared.TrafficMirrorRuleParam
	if param != nil {
		rules = param.Rules
	}
	if rules == nil {
		rules = []*shared.TrafficMirrorRuleParam{}
	}

	// Fetch the current collection for the audit snapshot; ignore errors here
	// because ReplaceAll will re-read inside its transaction.
	before, _ := m.storager.FetchAll(ctx)
	beforeMap := trafficMirrorRulesSnapshotToMap(before)

	replaceRules := make([]*TrafficMirrorRuleParam, 0, len(rules))
	for _, rule := range rules {
		replaceRules = append(replaceRules, trafficMirrorRuleParamFromShared(rule))
	}

	err := itxn.ExecuteWithRetry(ctx, m.txn, func(ctx context.Context) error {
		return m.storager.ReplaceAll(ctx, replaceRules)
	})
	if err != nil {
		m.recordTrafficMirrorRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, trafficMirrorRulesParamToMap(rules), err)
		return nil, err
	}

	after, err := m.storager.FetchAll(ctx)
	if err != nil {
		m.recordTrafficMirrorRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, trafficMirrorRulesParamToMap(rules), err)
		return nil, err
	}

	m.recordTrafficMirrorRulesOperation(ctx, string(ioperlog.ActionUpdate), beforeMap, trafficMirrorRulesSnapshotToMap(after), nil)

	rst := make([]*shared.TrafficMirrorRuleParam, 0, len(after))
	for _, rule := range after {
		rst = append(rst, trafficMirrorRuleParamToShared(rule))
	}

	return &shared.TrafficMirrorRulesParam{Rules: rst}, nil
}

// ConfigExport exports the mirror_rule.data payload for BFE mod_traffic_mirror.
// When the generated content signature matches the last exported version, it
// returns nil (incremental, HTTP Data is null).
func (m *TrafficMirrorManager) ConfigExport(ctx context.Context, lastVersion string) (*ExportTrafficMirrorRuleConfig, error) {
	rst, err := m.versionControlManager.ExportConfig(ctx, ConfigTopicProductTrafficMirror, m.TrafficMirrorRuleGenerator)
	if err != nil {
		return nil, err
	}

	if rst.DataWithoutVersion == nil {
		return nil, fmt.Errorf("TrafficMirrorRuleGenerator.DataWithoutVersion is nil")
	}

	conf, ok := rst.DataWithoutVersion.(*ExportTrafficMirrorRuleConfig)
	if ok {
		if conf.Version == nil || *conf.Version == lastVersion {
			return nil, nil
		}

		return conf, nil
	}

	return nil, fmt.Errorf("convert TrafficMirrorRuleGenerator.DataWithoutVersion to ExportTrafficMirrorRuleConfig is error")
}

// TrafficMirrorRuleGenerator generates the mod_traffic_mirror export data: the
// full rule set (id ascending, no enabled filter) keyed by the AI inner
// product name. An empty table exports an empty array while the product key
// stays present. A rule with NULL remove_headers exports the default
// sensitive-header blacklist; an explicit empty array exports as-is (strip
// nothing).
func (m *TrafficMirrorManager) TrafficMirrorRuleGenerator(ctx context.Context) (*iversion_control.ExportData, error) {
	rules, err := m.storager.FetchAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch traffic mirror rules error: %s", err.Error())
	}

	exportRules := make([]*TrafficMirrorRuleConfFile, 0, len(rules))
	for _, rule := range rules {
		exportRule := &TrafficMirrorRuleConfFile{
			Cond:          rule.Cond,
			MirrorCluster: rule.MirrorCluster,
			Percentage:    rule.Percentage,
			PathRewrite:   rule.PathRewrite,
		}

		// Optional pointer fields fall back to their zero values so every rule
		// field is always present in the exported artifact.
		if exportRule.Cond == nil {
			exportRule.Cond = lib.PString("")
		}
		if exportRule.Percentage == nil {
			exportRule.Percentage = lib.PInt(DefaultTrafficMirrorPercentage)
		}
		if exportRule.PathRewrite == nil {
			exportRule.PathRewrite = lib.PString("")
		}

		if rule.RemoveHeaders != nil {
			exportRule.RemoveHeaders = *rule.RemoveHeaders
		} else {
			exportRule.RemoveHeaders = append([]string{}, DefaultSensitiveHeaders...)
		}

		if rule.SetHeaders != nil {
			exportRule.SetHeaders = rule.SetHeaders
		} else {
			exportRule.SetHeaders = map[string]string{}
		}

		exportRule.BodyRewrites = make([]*TrafficMirrorBodyRewriteConfFile, 0, len(rule.BodyRewrites))
		for _, rw := range rule.BodyRewrites {
			if rw == nil {
				continue
			}
			exportRule.BodyRewrites = append(exportRule.BodyRewrites, &TrafficMirrorBodyRewriteConfFile{
				Path:  rw.Path,
				Value: rw.Value,
			})
		}

		exportRules = append(exportRules, exportRule)
	}

	conf := &ExportTrafficMirrorRuleConfig{
		Config: &map[string]*[]*TrafficMirrorRuleConfFile{
			m.aiRouteInnerProductName: &exportRules,
		},
	}
	conf.UpdateVersion(iversion_control.ZeroVersion)

	return &iversion_control.ExportData{
		Topic:              ConfigTopicProductTrafficMirror,
		DataWithoutVersion: conf,
	}, nil
}
