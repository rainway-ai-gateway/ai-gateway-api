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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/openapi_v1/internal/testutil"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

type fakeAccessLogConfStorager struct {
	config *access_log.AccessLogConf
}

func (f *fakeAccessLogConfStorager) Fetch(ctx context.Context) (*access_log.AccessLogConf, error) {
	return f.config, nil
}

func (f *fakeAccessLogConfStorager) Upsert(ctx context.Context, config *access_log.AccessLogConf) error {
	f.config = config
	return nil
}

func setupAccessLogManager(storager access_log.AccessLogConfStorager) func() {
	old := container.AccessLogManager
	container.AccessLogManager = access_log.NewAccessLogManager(&testutil.FakeTxn{}, storager)
	return func() {
		container.AccessLogManager = old
	}
}

func TestAccessLogConfigGetAction(t *testing.T) {
	t.Run("configured returns canonical whitelists", func(t *testing.T) {
		store := &fakeAccessLogConfStorager{
			config: &access_log.AccessLogConf{
				Id:         access_log.AccessLogConfFixedID,
				ReqHeaders: `["Business_type","X-Request-Id"]`,
				ResHeaders: `["X-Trace-Id"]`,
			},
		}
		defer setupAccessLogManager(store)()

		req := httptest.NewRequest(http.MethodGet, "/access-log/config", nil)
		data, err := AccessLogConfigGetAction(req)
		require.NoError(t, err)

		bs, err := json.Marshal(data)
		require.NoError(t, err)
		assert.NotContains(t, string(bs), `"id"`)
		assert.Contains(t, string(bs), `"Business_type"`)

		param, ok := data.(*access_log.AccessLogConfigParam)
		require.True(t, ok)
		assert.Equal(t, []string{"Business_type", "X-Request-Id"}, param.ReqHeaders)
		assert.Equal(t, []string{"X-Trace-Id"}, param.ResHeaders)
	})

	t.Run("unconfigured renders empty arrays (not 404)", func(t *testing.T) {
		defer setupAccessLogManager(&fakeAccessLogConfStorager{})()

		req := httptest.NewRequest(http.MethodGet, "/access-log/config", nil)
		data, err := AccessLogConfigGetAction(req)
		require.NoError(t, err)

		bs, err := json.Marshal(data)
		require.NoError(t, err)
		assert.Contains(t, string(bs), `"req_headers":[]`)
		assert.Contains(t, string(bs), `"res_headers":[]`)
	})
}

func TestAccessLogConfigUpdateAction(t *testing.T) {
	t.Run("put canonicalizes and persists", func(t *testing.T) {
		store := &fakeAccessLogConfStorager{}
		defer setupAccessLogManager(store)()

		body := `{"req_headers":["business_type","X-REQUEST-ID"],"res_headers":["x-trace-id"]}`
		req := httptest.NewRequest(http.MethodPut, "/access-log/config", strings.NewReader(body))
		data, err := AccessLogConfigUpdateAction(req)
		require.NoError(t, err)

		param, ok := data.(*access_log.AccessLogConfigParam)
		require.True(t, ok)
		assert.Equal(t, []string{"Business_type", "X-Request-Id"}, param.ReqHeaders)
		assert.Equal(t, []string{"X-Trace-Id"}, param.ResHeaders)

		require.NotNil(t, store.config)
		assert.Equal(t, `["Business_type","X-Request-Id"]`, store.config.ReqHeaders)
	})

	t.Run("put empty arrays clears the whitelists", func(t *testing.T) {
		store := &fakeAccessLogConfStorager{
			config: &access_log.AccessLogConf{
				Id:         access_log.AccessLogConfFixedID,
				ReqHeaders: `["Business_type"]`,
				ResHeaders: `["X-Trace-Id"]`,
			},
		}
		defer setupAccessLogManager(store)()

		body := `{"req_headers":[],"res_headers":[]}`
		req := httptest.NewRequest(http.MethodPut, "/access-log/config", strings.NewReader(body))
		data, err := AccessLogConfigUpdateAction(req)
		require.NoError(t, err)

		param := data.(*access_log.AccessLogConfigParam)
		assert.Equal(t, []string{}, param.ReqHeaders)
		assert.Equal(t, []string{}, param.ResHeaders)
	})

	t.Run("sensitive header rejected without write", func(t *testing.T) {
		store := &fakeAccessLogConfStorager{}
		defer setupAccessLogManager(store)()

		body := `{"req_headers":["Authorization"],"res_headers":[]}`
		req := httptest.NewRequest(http.MethodPut, "/access-log/config", strings.NewReader(body))
		_, err := AccessLogConfigUpdateAction(req)
		require.Error(t, err)
		assert.Nil(t, store.config, "rejected PUT must not write")
	})

	t.Run("invalid header name rejected", func(t *testing.T) {
		store := &fakeAccessLogConfStorager{}
		defer setupAccessLogManager(store)()

		body := `{"req_headers":["bad name"],"res_headers":[]}`
		req := httptest.NewRequest(http.MethodPut, "/access-log/config", strings.NewReader(body))
		_, err := AccessLogConfigUpdateAction(req)
		require.Error(t, err)
		assert.Nil(t, store.config)
	})
}

func TestAccessLogConfigRoutes(t *testing.T) {
	t.Run("routes carry the AccessLog permission points", func(t *testing.T) {
		require.Len(t, Endpoints, 2)

		assert.Equal(t, "/access-log/config", AccessLogConfigGetRoute.Path)
		assert.Equal(t, http.MethodGet, AccessLogConfigGetRoute.Method)
		require.NotNil(t, AccessLogConfigGetRoute.Authorizer)
		require.NotNil(t, AccessLogConfigGetRoute.Authorizer.FeatureAuthorizer)
		assert.Equal(t, "AccessLog", string(AccessLogConfigGetRoute.Authorizer.FeatureAuthorizer.Feature))

		assert.Equal(t, "/access-log/config", AccessLogConfigUpdateRoute.Path)
		assert.Equal(t, http.MethodPut, AccessLogConfigUpdateRoute.Method)
		require.NotNil(t, AccessLogConfigUpdateRoute.Authorizer)
		require.NotNil(t, AccessLogConfigUpdateRoute.Authorizer.FeatureAuthorizer)
		assert.Equal(t, "AccessLog", string(AccessLogConfigUpdateRoute.Authorizer.FeatureAuthorizer.Feature))
	})
}
