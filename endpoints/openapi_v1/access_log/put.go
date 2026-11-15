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
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// AccessLogConfigUpdateAction full-replaces the access log config singleton
// in a single transaction and returns the re-read config (same shape as
// GET). A PUT rejected by validation (invalid header name, duplicate after
// canonicalization, or a sensitive header) records a failed audit and
// leaves the published config unchanged.
func AccessLogConfigUpdateAction(req *http.Request) (interface{}, error) {
	param := &access_log.AccessLogConfigParam{}
	if err := xreq.BindJSON(req, param); err != nil {
		return nil, err
	}

	if _, _, err := access_log.ValidateAccessLogConfig(param); err != nil {
		// Record the rejected write for audit. Identity and the before
		// snapshot are independent of the request body (issue #155).
		container.AccessLogManager.RecordPutAccessLogConfFailure(req.Context(), param, err)
		return nil, err
	}

	return container.AccessLogManager.Put(req.Context(), param)
}
