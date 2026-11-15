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

package imods

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
)

// ConfigTopicProductAccessLog is the configuration topic of the access log
// dynamic config export (config_versions name), independent version line
// like every other topic.
const ConfigTopicProductAccessLog = "mod_access_pb"

// AccessHeaderExport is the "access_header" section of mod_access_pb.data.
// The JSON tags are the frozen BFE file contract (snake_case; see
// bfe/docs/zh_cn/sys_design/mod_access_pb_data_conf.md): the exported Data
// IS the file content with the version embedded (ai-route shape).
type AccessHeaderExport struct {
	ReqHeaders []string `json:"req_headers"`
	ResHeaders []string `json:"res_headers"`
}

// ModAccessPbDataExport is the exported mod_access_pb.data payload consumed
// by BFE mod_access_pb3. Multi-section carrier: new dynamic config sections
// join as new top-level fields (BFE ignores unknown sections with a Warn,
// forward compatible).
type ModAccessPbDataExport struct {
	Version      string             `json:"Version"`
	AccessHeader AccessHeaderExport `json:"access_header"`
}

// UpdateVersion updates the configuration version.
func (conf *ModAccessPbDataExport) UpdateVersion(version string) error {
	conf.Version = version
	return nil
}

// AccessLogExporter exports the mod_access_pb.data payload for BFE
// mod_access_pb3 (bfenetworks/bfe#1506).
type AccessLogExporter struct {
	accessLogStorager     access_log.AccessLogConfStorager
	versionControlManager *iversion_control.VersionControlManager
}

// NewAccessLogExporter creates a new AccessLogExporter.
func NewAccessLogExporter(accessLogStorager access_log.AccessLogConfStorager,
	versionControlManager *iversion_control.VersionControlManager) *AccessLogExporter {
	return &AccessLogExporter{
		accessLogStorager:     accessLogStorager,
		versionControlManager: versionControlManager,
	}
}

// ConfigExport exports the mod_access_pb.data payload for BFE. When the
// exported version matches lastVersion (content unchanged since the last
// distribution), it returns nil (HTTP Data is null: conf-agent does not
// rewrite the file nor reload BFE).
//
// Unlike the intent config, an unconfigured state exports as EMPTY
// whitelists (not nil): an empty whitelist is the legitimate BFE default
// (collect nothing), and the distribution pipeline must be able to retract
// a previously published whitelist back to empty.
func (e *AccessLogExporter) ConfigExport(ctx context.Context, lastVersion string) (*ModAccessPbDataExport, error) {
	rst, err := e.versionControlManager.ExportConfig(ctx, ConfigTopicProductAccessLog, e.AccessLogGenerator)
	if err != nil {
		return nil, err
	}

	if rst.DataWithoutVersion == nil {
		return nil, fmt.Errorf("AccessLogGenerator.DataWithoutVersion is nil")
	}

	conf, ok := rst.DataWithoutVersion.(*ModAccessPbDataExport)
	if !ok {
		return nil, fmt.Errorf("convert AccessLogGenerator.DataWithoutVersion to ModAccessPbDataExport is error")
	}

	if conf.Version == lastVersion {
		return nil, nil
	}

	return conf, nil
}

// AccessLogGenerator generates the mod_access_pb.data export content: the
// singleton row converted into the frozen file contract. Stored header lists
// are already canonical (validated at PUT time). A missing row exports as
// empty arrays (never null).
func (e *AccessLogExporter) AccessLogGenerator(ctx context.Context) (*iversion_control.ExportData, error) {
	config, err := e.accessLogStorager.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch access log config error: %s", err.Error())
	}

	reqHeaders, err := storedHeaderList(config, true)
	if err != nil {
		return nil, err
	}
	resHeaders, err := storedHeaderList(config, false)
	if err != nil {
		return nil, err
	}

	conf := &ModAccessPbDataExport{
		AccessHeader: AccessHeaderExport{
			ReqHeaders: reqHeaders,
			ResHeaders: resHeaders,
		},
	}
	conf.UpdateVersion(iversion_control.ZeroVersion)

	return &iversion_control.ExportData{
		Topic:              ConfigTopicProductAccessLog,
		DataWithoutVersion: conf,
	}, nil
}

// storedHeaderList parses one stored whitelist column into a string slice;
// a nil row or an empty column exports as an empty array (never null).
func storedHeaderList(config *access_log.AccessLogConf, req bool) ([]string, error) {
	raw := ""
	if config != nil {
		if req {
			raw = config.ReqHeaders
		} else {
			raw = config.ResHeaders
		}
	}

	headers := []string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &headers); err != nil {
			field := "res_headers"
			if req {
				field = "req_headers"
			}
			return nil, fmt.Errorf("parse stored access log %s error: %s", field, err.Error())
		}
	}
	return headers, nil
}
