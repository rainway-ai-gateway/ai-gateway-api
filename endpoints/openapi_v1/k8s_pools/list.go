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

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// ListEndpoint lists all K8s pools.
var ListEndpoint = &xreq.Endpoint{
	Path:       "/k8s_pools",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(ListAction),
	Authorizer: iauth.FA(iauth.FeatureK8sPool, iauth.ActionRead),
}

var _ xreq.Handler = ListAction

// ListAction handles GET /k8s_pools.
func ListAction(req *http.Request) (interface{}, error) {
	pools, err := container.K8sPoolManager.FetchPoolList(req.Context())
	if err != nil {
		return nil, err
	}

	list := make([]*PoolListEntry, 0, len(pools))
	for _, pool := range pools {
		list = append(list, &PoolListEntry{
			Name:          pool.Name,
			InstanceCount: len(pool.Instances),
			LastSyncTime:  pool.UpdateTime,
		})
	}
	return map[string]interface{}{"list": list}, nil
}
