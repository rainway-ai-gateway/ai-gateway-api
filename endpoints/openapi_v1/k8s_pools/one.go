// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s_pools

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// GetEndpoint fetches one K8s pool by name.
var GetEndpoint = &xreq.Endpoint{
	Path:       "/k8s_pools/{name}",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(GetAction),
	Authorizer: iauth.FA(iauth.FeatureK8sPool, iauth.ActionRead),
}

var _ xreq.Handler = GetAction

// GetAction handles GET /k8s_pools/{name}.
func GetAction(req *http.Request) (interface{}, error) {
	name := mux.Vars(req)["name"]
	if err := iprovider.K8sPoolName(name); err != nil {
		return nil, err
	}

	pool, err := container.K8sPoolManager.FetchPool(req.Context(), name)
	if err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, xerror.WrapRecordNotExist("k8s pool")
	}
	return poolEntryOf(pool), nil
}
