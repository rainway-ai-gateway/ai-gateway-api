// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
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
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpoints(t *testing.T) {
	eps := endpoints()
	require.Len(t, eps, 20)

	paths := make([]string, 0, len(eps))
	for _, ep := range eps {
		require.NotNil(t, ep)
		if ep.RegisterHandler != nil {
			paths = append(paths, "")
		} else {
			paths = append(paths, ep.Path)
		}
	}

	assert.Equal(t, []string{
		"/configs/tls_conf/server_data_conf",
		"/configs/gslb_data/gslb",
		"/configs/gslb_data/cluster_table",
		"/configs/protocol/server_cert_conf",
		"",
		"/configs/mod-api-key",
		"/configs/mod-body-process",
		"/quota/trigger-reset",
		"/configs/rate-limit-policy",
		"/configs/ai-cache-rule",
		"/configs/ai-context-rule",
		"/configs/traffic-mirror-rule",
		"/configs/ai-route",
		"/configs/mod-ai-intent",
		"/configs/mod-access-pb",
		"/configs/epp_data/config",
		"/k8s_pools/{name}/instances",
		"/k8s_pools/{name}",
		"/k8s_pools",
		"/k8s_pools/{name}",
	}, paths)
}

func TestRegisterRouter(t *testing.T) {
	root := mux.NewRouter()
	sub := RegisterRouter(root)

	require.NotNil(t, sub)

	var matchedPaths []string
	err := sub.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		path, err := route.GetPathTemplate()
		if err == nil {
			matchedPaths = append(matchedPaths, path)
		}
		return nil
	})
	require.NoError(t, err)

	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/tls_conf/server_data_conf")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/gslb_data/gslb")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/gslb_data/cluster_table")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/protocol/server_cert_conf")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/extra_files/")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/mod-api-key")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/mod-body-process")
	assert.Contains(t, matchedPaths, "/inner-api/v1/quota/trigger-reset")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/rate-limit-policy")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/ai-cache-rule")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/ai-context-rule")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/traffic-mirror-rule")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/ai-route")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/mod-ai-intent")
	assert.Contains(t, matchedPaths, "/inner-api/v1/configs/epp_data/config")
	assert.Contains(t, matchedPaths, "/inner-api/v1/k8s_pools/{name}/instances")
	assert.Contains(t, matchedPaths, "/inner-api/v1/k8s_pools/{name}")
	assert.Contains(t, matchedPaths, "/inner-api/v1/k8s_pools")
}
