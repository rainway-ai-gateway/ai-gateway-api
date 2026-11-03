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
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"

	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/innerapi_v1"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/middleware"
	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/openapi_v1"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
)

func fileHandler(root http.Dir, fs http.Handler) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/") {
			r.URL.Path = "/" + r.URL.Path
		}
		r.URL.Path = path.Clean(r.URL.Path)

		requestInfo := xreq.GetRequestInfo(r.Context())
		defer middleware.Record(requestInfo)

		f, err := root.Open(r.URL.Path)
		if err != nil {
			if os.IsNotExist(err) {
				r.URL.Path = "/"
			}
		} else {
			f.Close()
		}

		fs.ServeHTTP(rw, r)
	}
}

func RegisterRouters(router *mux.Router) {
	fileServerRoot, err := filepath.Abs(stateful.DefaultConfig.RunTime.StaticFilePath)
	if err != nil {
		panic(err)
	}
	root := http.Dir(fileServerRoot)
	fs := http.FileServer(root)
	fh := fileHandler(root, fs)

	router.MethodNotAllowedHandler = withBaseMiddlewares(methodNotAllowedHandler(router))

	router.Use(middleware.MCRecovery)
	router.Use(middleware.MCLogger)
	// 管理面 IP 白名单；位置约束：Logger 后（拒绝留痕）/ CORS 前（403 对预检生效）；勿移
	router.Use(middleware.McIPProbe)
	router.Use(middleware.MCCors)

	// 静态资源分支（NotFoundHandler）不经过 router.Use 链：mux v1.8 的
	// ServeHTTP 对 NotFoundHandler 不应用 Use 注册的中间件。此处手动包裹
	// 同一链条（执行序 Recovery→Logger→IPProbe→Cors，由内向外包），保证
	// 管理面 IP 白名单对 Dashboard 静态路径同样生效（MAC-1-003 捕获）。
	staticHandler := http.Handler(http.HandlerFunc(fh))
	staticHandler = middleware.MCCors.Middleware(staticHandler)
	staticHandler = middleware.McIPProbe.Middleware(staticHandler)
	staticHandler = middleware.MCLogger.Middleware(staticHandler)
	staticHandler = middleware.MCRecovery.Middleware(staticHandler)

	// API 路径未匹配时返回 JSON 404，避免返回静态文件 HTML。
	// 该分支同样不在 router.Use 链内（mux v1.8 只对命中的路由套中间件），
	// 手工套 Recovery→Logger→Cors，保证 CORS 预检与 Req-ID 不丢（issue #298）。
	// 静态分支已自带全链（含 McIPProbe），不可再包一层，否则双份访问日志。
	notFoundJSON := withBaseMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := &xreq.Result{Code: 404, ErrMsg: "Not Found"}
		xreq.Render(w, r, res)
	}))

	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/open-api/v1") || strings.HasPrefix(r.URL.Path, "/inner-api/v1") {
			notFoundJSON.ServeHTTP(w, r)
			return
		}

		staticHandler.ServeHTTP(w, r)
	})

	openapi_v1.RegisterEndpoints(router)
	innerapi_v1.RegisterRouter(router)
}

// withBaseMiddlewares wraps a handler with the middlewares that are independent
// of routing and must cover every response: panic recovery (which also mints the
// request ID), access logging, and CORS (which short-circuits a preflight with
// 204 + Access-Control-Allow-*).
//
// It exists because router.Use only wraps matched routes (mux v1.8): without this
// the JSON 404 and 405 fallbacks would answer preflight with 405 without any
// CORS headers and without a Req-ID (issue #298).
//
// The management-plane IP allowlist is deliberately NOT part of this chain:
// route-level 404/405 must stay 404/405 and never turn into 403 (API testing
// standard §5.5.2 #4). Matched routes still run McIPProbe and stay protected.
func withBaseMiddlewares(h http.Handler) http.Handler {
	h = middleware.MCCors.Middleware(h)
	h = middleware.MCLogger.Middleware(h)
	h = middleware.MCRecovery.Middleware(h)
	return h
}

// methodNotAllowedHandler renders the JSON 405 body and, per RFC 9110 §15.5.6,
// advertises the methods the matched path actually supports.
func methodNotAllowedHandler(router *mux.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow := allowedMethods(router, r); len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
		}

		res := &xreq.Result{Code: 405, ErrMsg: "Method Not Allowed"}
		xreq.Render(w, r, res)
	})
}

// allowProbeMethods is the set of methods probed when computing the Allow
// header. Keep it in sync with the method convention in
// design-docs/api-define/OpenAPI接口定义/00-common.md.
var allowProbeMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodOptions,
	http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
}

// allowedMethods returns the methods that route to r's path, in a stable order.
// It probes the same router so sub-routers, path variables and other matchers
// are honoured: Router.Match only exposes the route that failed the method
// matcher, and iterating router.routes cannot see through
// PathPrefix(...).Subrouter().
func allowedMethods(router *mux.Router, r *http.Request) []string {
	var allowed []string

	for _, method := range allowProbeMethods {
		if method == r.Method {
			continue
		}

		probe := r.Clone(r.Context())
		probe.Method = method

		var match mux.RouteMatch
		// Router.Match also reports true (with MatchErr set) for a method
		// mismatch or a not-found path once the custom handlers are installed,
		// so a clean match on a concrete route is required.
		if router.Match(probe, &match) && match.MatchErr == nil && match.Route != nil {
			allowed = append(allowed, method)
		}
	}

	return allowed
}
