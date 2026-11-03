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

package endpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMethodNotAllowedTestRouter mirrors the production topology: both APIs hang
// off root sub-routers, and the shared 405 handler lives on the root router.
func newMethodNotAllowedTestRouter() *mux.Router {
	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	router.MethodNotAllowedHandler = methodNotAllowedHandler(router)

	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	openAPI := router.PathPrefix("/open-api/v1").Subrouter()
	openAPI.HandleFunc("/get-only", ok).Methods(http.MethodGet)
	openAPI.HandleFunc("/multi", ok).Methods(http.MethodGet, http.MethodPost)

	innerAPI := router.PathPrefix("/inner-api/v1").Subrouter()
	innerAPI.HandleFunc("/configs/ai-route", ok).Methods(http.MethodGet)

	router.HandleFunc("/root-post", ok).Methods(http.MethodPost)

	return router
}

func TestMethodNotAllowedAllowHeader(t *testing.T) {
	cases := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantAllow string
	}{
		{"post to get-only sub-route", http.MethodPost, "/open-api/v1/get-only", http.StatusMethodNotAllowed, "GET"},
		{"options to get-only sub-route", http.MethodOptions, "/open-api/v1/get-only", http.StatusMethodNotAllowed, "GET"},
		{"head to get-only sub-route", http.MethodHead, "/open-api/v1/get-only", http.StatusMethodNotAllowed, "GET"},
		{"put to multi-method sub-route", http.MethodPut, "/open-api/v1/multi", http.StatusMethodNotAllowed, "GET, POST"},
		{"delete to innerapi get-only", http.MethodDelete, "/inner-api/v1/configs/ai-route", http.StatusMethodNotAllowed, "GET"},
		{"get to root post-only", http.MethodGet, "/root-post", http.StatusMethodNotAllowed, "POST"},
		{"matched get keeps 200", http.MethodGet, "/open-api/v1/get-only", http.StatusOK, ""},
		{"unknown path keeps 404", http.MethodGet, "/open-api/v1/nope", http.StatusNotFound, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newMethodNotAllowedTestRouter()
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			assert.Equal(t, tc.wantCode, rec.Code)
			assert.Equal(t, tc.wantAllow, rec.Header().Get("Allow"))

			if tc.wantCode != http.StatusMethodNotAllowed {
				assert.Empty(t, rec.Header().Get("Allow"), "non-405 responses must not carry Allow")
				return
			}

			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			var body struct {
				ErrNum int    `json:"ErrNum"`
				ErrMsg string `json:"ErrMsg"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, http.StatusMethodNotAllowed, body.ErrNum)
			assert.Equal(t, "Method Not Allowed", body.ErrMsg)
		})
	}
}

func TestAllowedMethodsSkipsRequestMethod(t *testing.T) {
	router := newMethodNotAllowedTestRouter()
	req := httptest.NewRequest(http.MethodPut, "/open-api/v1/multi", nil)

	assert.Equal(t, []string{http.MethodGet, http.MethodPost}, allowedMethods(router, req))
}
