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

	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/export_util"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// ExportRoute is the InnerAPI endpoint exporting mod_access_pb.data for BFE
// mod_access_pb3 (bfenetworks/bfe#1506).
var ExportRoute = &xreq.Endpoint{
	Path:       "/configs/mod-access-pb",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(ExportAction),
	Authorizer: iauth.FA(iauth.FeatureRoute, iauth.ActionExport),
}

var _ xreq.Handler = ExportAction

// ExportAction returns the mod_access_pb.data payload (Data is null when
// the version is unchanged).
func ExportAction(req *http.Request) (interface{}, error) {
	param, err := export_util.NewExportFromReq(req)
	if err != nil {
		return nil, err
	}

	return container.AccessLogExporter.ConfigExport(req.Context(), param.Version)
}
