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

// Package http_semantics_test 覆盖跨接口的 HTTP 协议语义（issue #297）：
// 405 Method Not Allowed 必须携带 Allow，且只列该路径真实支持的方法；
// 404 / 200 等相邻响应不得新增该头。
package http_semantics_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sm *testutil.ServerManager

func TestMain(m *testing.M) {
	var err error
	sm, err = testutil.StartServer()
	if err != nil {
		panic("failed to start server: " + err.Error())
	}
	code := m.Run()
	sm.Shutdown()
	os.Exit(code)
}

// doRaw issues the request with net/http directly: testutil.Client only
// returns the JSON envelope, and these assertions need response headers.
func doRaw(t *testing.T, method, path string, headers map[string]string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, sm.ServerURL+path, nil)
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, body
}

func TestMethodNotAllowedAllowHeader(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantAllow string
	}{
		{
			name:      "post to get-only openapi route",
			method:    http.MethodPost,
			path:      "/open-api/v1/route-tables",
			wantCode:  http.StatusMethodNotAllowed,
			wantAllow: http.MethodGet,
		},
		{
			name:      "plain options to get-only openapi route",
			method:    http.MethodOptions,
			path:      "/open-api/v1/route-tables",
			wantCode:  http.StatusMethodNotAllowed,
			wantAllow: http.MethodGet,
		},
		{
			name:      "delete to get-only innerapi route",
			method:    http.MethodDelete,
			path:      "/inner-api/v1/configs/ai-route",
			wantCode:  http.StatusMethodNotAllowed,
			wantAllow: http.MethodGet,
		},
		{
			name:      "patch to multi-method route lists both",
			method:    http.MethodPatch,
			path:      "/open-api/v1/api-keys",
			wantCode:  http.StatusMethodNotAllowed,
			wantAllow: "GET, POST",
		},
		{
			name:      "registered method still succeeds",
			method:    http.MethodGet,
			path:      "/open-api/v1/route-tables",
			wantCode:  http.StatusOK,
			wantAllow: "",
		},
		{
			name:      "unknown path is 404 without allow",
			method:    http.MethodPost,
			path:      "/open-api/v1/nope",
			wantCode:  http.StatusNotFound,
			wantAllow: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doRaw(t, tc.method, tc.path, nil)

			assert.Equal(t, tc.wantCode, resp.StatusCode)
			assert.Equal(t, tc.wantAllow, resp.Header.Get("Allow"))

			if tc.wantCode != http.StatusMethodNotAllowed {
				assert.Empty(t, resp.Header.Get("Allow"), "non-405 responses must not carry Allow")
				return
			}

			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			var envelope struct {
				ErrNum int    `json:"ErrNum"`
				ErrMsg string `json:"ErrMsg"`
			}
			require.NoError(t, json.Unmarshal(body, &envelope))
			assert.Equal(t, http.StatusMethodNotAllowed, envelope.ErrNum)
			assert.Equal(t, "Method Not Allowed", envelope.ErrMsg)
		})
	}
}

// TestCORSAndReqIDHeaders 覆盖 issue #298：CORS 预检必须 2xx + Access-Control-Allow-*，
// 路由级 404/405 必须带 Req-ID（且与 200 的注入路径一致）。
func TestCORSAndReqIDHeaders(t *testing.T) {
	preflight := func(method string) map[string]string {
		return map[string]string{
			"Origin":                        "https://console.example.com",
			"Access-Control-Request-Method": method,
		}
	}

	t.Run("HS-2-001 preflight on registered route", func(t *testing.T) {
		resp, _ := doRaw(t, http.MethodOptions, "/open-api/v1/route-tables", preflight("PATCH"))
		require.GreaterOrEqual(t, resp.StatusCode, 200)
		require.Less(t, resp.StatusCode, 300)
		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, "PATCH", resp.Header.Get("Access-Control-Allow-Methods"))
	})

	t.Run("HS-2-002 preflight on unknown path", func(t *testing.T) {
		resp, _ := doRaw(t, http.MethodOptions, "/open-api/v1/definitely-not-a-route", preflight("GET"))
		require.GreaterOrEqual(t, resp.StatusCode, 200)
		require.Less(t, resp.StatusCode, 300)
		assert.NotEmpty(t, resp.Header.Get("Access-Control-Allow-Origin"))
	})

	t.Run("HS-2-003 404 carries Req-ID", func(t *testing.T) {
		resp, _ := doRaw(t, http.MethodGet, "/open-api/v1/definitely-not-a-route", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.NotEmpty(t, resp.Header.Get("Req-ID"))
	})

	t.Run("HS-2-004 405 carries Req-ID and Allow", func(t *testing.T) {
		resp, _ := doRaw(t, http.MethodPost, "/open-api/v1/route-tables", nil)
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.NotEmpty(t, resp.Header.Get("Req-ID"))
		assert.Equal(t, http.MethodGet, resp.Header.Get("Allow"))
	})

	t.Run("HS-2-005 200 still carries Req-ID", func(t *testing.T) {
		resp, _ := doRaw(t, http.MethodGet, "/open-api/v1/route-tables", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.NotEmpty(t, resp.Header.Get("Req-ID"))
	})

	t.Run("HS-2-006 plain OPTIONS keeps 405 with CORS", func(t *testing.T) {
		resp, _ := doRaw(t, http.MethodOptions, "/open-api/v1/route-tables",
			map[string]string{"Origin": "https://console.example.com"})
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		assert.Equal(t, http.MethodGet, resp.Header.Get("Allow"))
		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.NotEmpty(t, resp.Header.Get("Req-ID"))
	})

	t.Run("HS-2-007 Req-ID differs per request", func(t *testing.T) {
		respA, _ := doRaw(t, http.MethodGet, "/open-api/v1/definitely-not-a-route", nil)
		respB, _ := doRaw(t, http.MethodGet, "/open-api/v1/definitely-not-a-route", nil)
		require.NotEmpty(t, respA.Header.Get("Req-ID"))
		require.NotEmpty(t, respB.Header.Get("Req-ID"))
		assert.NotEqual(t, respA.Header.Get("Req-ID"), respB.Header.Get("Req-ID"))
	})
}
