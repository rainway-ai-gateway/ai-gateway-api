// Copyright (c) 2021 The BFE Authors.
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

package innerapi_v1

import (
	"github.com/gorilla/mux"

	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/ai_cache"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/ai_context"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/ai_route"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/epp_data"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/extra_file"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/gslb_data"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/intent_config_export"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/k8s_pools"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/mod_api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/mod_body_process"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/protocol"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/quota_reset"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/rate_limit_policy"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/server_data"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1/traffic_mirror"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/middleware"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
)

func endpoints() []*xreq.Endpoint {
	return []*xreq.Endpoint{
		server_data.ExportEndpoint,
		gslb_data.ExportGSLBEndpoint,
		gslb_data.ExportClusterTableEndpoint,
		protocol.ServertCertExportEndpoint,
		extra_file.ExportExtraFileEndpoint,
		mod_api_key.ExportRoute,
		mod_body_process.ExportRoute,
		quota_reset.TriggerResetRoute,
		rate_limit_policy.ExportRoute,
		ai_cache.ExportRoute,
		ai_context.ExportRoute,
		traffic_mirror.ExportRoute,
		ai_route.ExportRoute,
		intent_config_export.ExportRoute,
		access_log.ExportRoute,
		epp_data.ExportRoute,
		k8s_pools.ReplaceInstancesRoute,
		k8s_pools.GetPoolRoute,
		k8s_pools.ListPoolsRoute,
		k8s_pools.DeletePoolRoute,
	}
}

func RegisterRouter(router *mux.Router) *mux.Router {
	innerAPIV1Router := router.PathPrefix("/inner-api/v1").Subrouter()
	innerAPIV1Router.Use(middleware.McUserProbe)

	for _, one := range endpoints() {
		one.Register(innerAPIV1Router)
	}

	return innerAPIV1Router
}
