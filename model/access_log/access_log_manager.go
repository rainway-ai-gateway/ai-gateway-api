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

package access_log

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bfenetworks/go-lib/log"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
)

// accessLogConfResourceID identifies the singleton in operation logs (the
// singleton is the only addressable resource; the fixed row id is internal).
const accessLogConfResourceID = "access_log_conf"

// AccessLogManager manages the access log config singleton (OpenAPI side).
// The export side lives in model/imods/access_log_exporter.go.
type AccessLogManager struct {
	txn                 itxn.TxnStorager
	storager            AccessLogConfStorager
	operationLogManager ioperlog.OperationLogRecorder
}

// NewAccessLogManager creates a new AccessLogManager.
func NewAccessLogManager(txn itxn.TxnStorager, storager AccessLogConfStorager) *AccessLogManager {
	return &AccessLogManager{
		txn:      txn,
		storager: storager,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (m *AccessLogManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// Get returns the current access log config. Unlike the intent config, an
// unconfigured state renders as empty whitelists (not 404): an empty
// whitelist is the BFE default behavior, and GET doubles as the "current
// effective config" view for the console.
func (m *AccessLogManager) Get(ctx context.Context) (*AccessLogConfigParam, error) {
	config, err := m.storager.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	return accessLogConfToParam(config), nil
}

// Put full-replaces the singleton (single-row upsert, fixed id=1) in one
// transaction: validate and canonicalize the whitelists, upsert the row and
// return the re-read config. On any failure the row stays unchanged and a
// failed audit is recorded.
func (m *AccessLogManager) Put(ctx context.Context, param *AccessLogConfigParam) (*AccessLogConfigParam, error) {
	reqCanonical, resCanonical, err := ValidateAccessLogConfig(param)
	if err != nil {
		return nil, err
	}

	if ContainsCookieHeader(reqCanonical, resCanonical) {
		// Allowed but worth a warning: cookie-like headers may carry
		// session credentials into access logs.
		log.Logger.Warn("access_log: whitelist contains Cookie/Set-Cookie, session credentials may leak into access logs: req=%v res=%v",
			reqCanonical, resCanonical)
	}

	before, _ := m.storager.Fetch(ctx)

	reqJSON, err := json.Marshal(reqCanonical)
	if err != nil {
		return nil, xerror.WrapParamErrorWithMsg("marshal req_headers failed: %s", err.Error())
	}
	resJSON, err := json.Marshal(resCanonical)
	if err != nil {
		return nil, xerror.WrapParamErrorWithMsg("marshal res_headers failed: %s", err.Error())
	}

	err = m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		return m.storager.Upsert(ctx, &AccessLogConf{
			Id:         AccessLogConfFixedID,
			ReqHeaders: string(reqJSON),
			ResHeaders: string(resJSON),
		})
	})
	if err != nil {
		m.recordPutOperation(ctx, accessLogConfToMap(before), accessLogParamToMap(param), err)
		return nil, err
	}

	after, err := m.storager.Fetch(ctx)
	if err != nil {
		m.recordPutOperation(ctx, accessLogConfToMap(before), accessLogParamToMap(param), err)
		return nil, err
	}

	m.recordPutOperation(ctx, accessLogConfToMap(before), accessLogConfToMap(after), nil)
	return accessLogConfToParam(after), nil
}

func (m *AccessLogManager) recordPutOperation(ctx context.Context, before, after map[string]interface{}, err error) {
	if m.operationLogManager == nil {
		return
	}

	status := ioperlog.StatusSuccess
	errorMsg := ""
	if err != nil {
		status = ioperlog.StatusFailed
		errorMsg = ioperlog.TruncateErrorMessageDefault(err)
	}

	entry := &ioperlog.OperationLogEntry{
		Action:       string(ioperlog.ActionUpdate),
		ResourceType: string(ioperlog.ResourceTypeAccessLogConf),
		ResourceID:   accessLogConfResourceID,
		ResourceName: accessLogConfResourceID,
		Status:       status,
		ErrorMsg:     errorMsg,
		CreatedAt:    time.Now(),
	}

	entry.ChangeSummary = ioperlog.BuildChangeSummary(string(ioperlog.ActionUpdate), before, after)

	m.operationLogManager.Record(ctx, entry)
}

// accessLogConfToParam converts the storage-level singleton into the Open
// API representation; a nil row renders as empty whitelists.
func accessLogConfToParam(config *AccessLogConf) *AccessLogConfigParam {
	param := &AccessLogConfigParam{
		ReqHeaders: []string{},
		ResHeaders: []string{},
	}
	if config == nil {
		return param
	}
	if config.ReqHeaders != "" {
		_ = json.Unmarshal([]byte(config.ReqHeaders), &param.ReqHeaders)
	}
	if config.ResHeaders != "" {
		_ = json.Unmarshal([]byte(config.ResHeaders), &param.ResHeaders)
	}
	return param
}

func accessLogParamToMap(param *AccessLogConfigParam) map[string]interface{} {
	if param == nil {
		return nil
	}
	return map[string]interface{}{
		"req_headers": param.ReqHeaders,
		"res_headers": param.ResHeaders,
	}
}

func accessLogConfToMap(config *AccessLogConf) map[string]interface{} {
	if config == nil {
		return nil
	}
	m := map[string]interface{}{
		"updated_at": config.UpdatedAt,
	}
	var req, res []string
	if config.ReqHeaders != "" {
		_ = json.Unmarshal([]byte(config.ReqHeaders), &req)
	}
	if config.ResHeaders != "" {
		_ = json.Unmarshal([]byte(config.ResHeaders), &res)
	}
	m["req_headers"] = req
	m["res_headers"] = res
	return m
}
