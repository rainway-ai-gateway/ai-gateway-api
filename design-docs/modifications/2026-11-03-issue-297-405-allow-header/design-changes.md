# 设计变更：#297 405 响应补 `Allow` 头

## 1. 现状

### 1.1 路由与 405 入口

```
main.go:134-136        mux.NewRouter() → endpoints.RegisterRouters(router) → negroni
endpoints/router.go:65 router.MethodNotAllowedHandler = JSON 405（无 Allow）
endpoints/openapi_v1/endpoints.go:54   router.PathPrefix("/open-api/v1").Subrouter()
endpoints/innerapi_v1/endpoints.go:64  router.PathPrefix("/inner-api/v1").Subrouter()
```

405 handler 只对**一个**场景生效：路径（含其它 matcher）命中某条路由，但 `Method` matcher 不匹配。

### 1.2 mux v1.8.0 的实际行为（源码核对）

```go
// mux.go: Router.Match —— 路径匹配失败/方法不匹配时的收尾
if match.MatchErr == ErrMethodMismatch {
    if r.MethodNotAllowedHandler != nil {
        match.Handler = r.MethodNotAllowedHandler
        return true          // 注意：返回 true
    }
    return false
}
if r.NotFoundHandler != nil {
    match.Handler = r.NotFoundHandler
    match.MatchErr = ErrNotFound
    return true              // 注意：返回 true
}

// mux.go: 默认 405 handler —— 只写状态码，不写 Allow
func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusMethodNotAllowed)
}
```

三条对本设计有决定性影响的结论：

1. **mux 从不设置 `Allow`**（issue 中「默认实现会写 Allow」的表述不成立），必须自己算；
2. `Router.Match` 在 `ErrMethodMismatch` / `ErrNotFound` 时也**返回 true**，因此「探测」不能只看返回值；
3. `Route.GetMethods()` 只能拿到单条直接路由的方法集合；`middleware.go:getAllMethodsForRoute`
   只遍历 `r.routes`，无法穿透 `PathPrefix(...).Subrouter()`，对本项目拓扑不可用。

### 1.3 实测（2026-11-03，HEAD `8ad4d62`）

`POST /open-api/v1/route-tables` → `405`、`Allow=""`；`OPTIONS` 同路径 → `405`、`Allow=""`；
`POST /open-api/v1/nope` → `404`；`GET` 同路径 → `200`。与 issue 一致。

## 2. 方案

### 2.1 计算 `Allow` 的方式：按方法探测

```go
// endpoints/router.go
func methodNotAllowedHandler(router *mux.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow := allowedMethods(router, r); len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
		}
		res := &xreq.Result{Code: 405, ErrMsg: "Method Not Allowed"}
		xreq.Render(w, r, res)
	})
}

// allowedMethods returns the methods that actually route to r's path, in a
// stable order. It probes the same router so sub-routers, path variables and
// other matchers are honoured; asking the matched route is not enough because
// Router.Match only exposes the route that failed the method matcher.
func allowedMethods(router *mux.Router, r *http.Request) []string {
	var allowed []string
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
	} {
		if method == r.Method {
			continue
		}
		probe := r.Clone(r.Context())
		probe.Method = method
		var match mux.RouteMatch
		// Router.Match returns true (with MatchErr set) for both method
		// mismatch and not-found when the custom handlers are installed, so
		// require a clean match on a concrete route.
		if router.Match(probe, &match) && match.MatchErr == nil && match.Route != nil {
			allowed = append(allowed, method)
		}
	}
	return allowed
}
```

注册处改为 `router.MethodNotAllowedHandler = methodNotAllowedHandler(router)`。

细节与理由：

| 决策 | 理由 |
|---|---|
| 探测而非遍历 `router.routes` | 本项目全部接口都在子路由下，遍历根 routes 只能看到 `PathPrefix` 路由，拿不到方法集合 |
| 探测方法集合取 7 个标准方法 | 覆盖本项目已注册的全部方法（`grep Methods(` 范围内只有 GET/POST/PUT/PATCH/DELETE）；OPTIONS/HEAD 也探测以如实反映 |
| `r.Clone(ctx)` 后再改 `Method` | 避免修改原请求；`Clone` 深拷贝 URL/Header，`Match` 不会读 body |
| 顺序固定 | 输出稳定（`GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE`），便于断言与客户端缓存 |
| 空集合不写头 | 理论上 405 必有至少一个可路由方法；写空 `Allow` 反而更差 |
| 405 body 不变 | 避免破坏依赖 `{"ErrNum":405,"ErrMsg":"Method Not Allowed"}` 的调用方 |
| OPTIONS 仍返回 405（而非 200） | issue 验收允许「405 或 200」；保持 405 语义最小改动，且避免与 CORS 预检语义混淆 |

### 2.2 为什么 HEAD 不在 GET 路由上「顺带」声明

gorilla/mux 不会为 `Methods("GET")` 的路由隐式匹配 HEAD（只有 `net/http.ServeMux` 会）。
实测 `HEAD /open-api/v1/route-tables` 也返回 405，因此 `Allow` 里不能出现 HEAD——否则等于告诉客户端
「HEAD 可用」而实际不可用。用同一套探测即可自动满足（HEAD 探测不命中）。

### 2.3 行为矩阵（目标）

| 请求 | 路由情况 | 状态 | `Allow` |
|---|---|---|---|
| `POST /open-api/v1/route-tables` | 仅 `GET` | 405 | `GET` |
| `OPTIONS /open-api/v1/route-tables` | 仅 `GET` | 405 | `GET` |
| `HEAD /open-api/v1/route-tables` | 仅 `GET`（mux 不隐式支持 HEAD） | 405 | `GET` |
| `PUT /open-api/v1/xxx`（该路径同时注册 GET+POST） | `GET, POST` | 405 | `GET, POST` |
| `DELETE /inner-api/v1/configs/ai-route` | 仅 `GET` | 405 | `GET` |
| `GET /open-api/v1/route-tables` | 命中 | 200 | 不写（非 405 分支） |
| `POST /open-api/v1/nope` | 路径未命中 | 404 | 不写 |
| `POST /dashboard/whatever`（静态分支） | 未命中静态文件 | 404/200 | 不写 |

## 3. 单元测试

新增 `endpoints/router_test.go`（同包，纯 `mux` + `httptest`，不依赖 `stateful` 初始化）：

```go
func newTestRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/get-only", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }).Methods(http.MethodGet)
	r.HandleFunc("/multi", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }).Methods(http.MethodGet, http.MethodPost)
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	r.MethodNotAllowedHandler = methodNotAllowedHandler(r)
	return r
}

func TestMethodNotAllowedAllowHeader(t *testing.T) {
	r := newTestRouter()
	cases := []struct {
		method, path, wantAllow string
		wantCode                int
	}{
		{"POST", "/get-only", "GET", 405},
		{"OPTIONS", "/get-only", "GET", 405},
		{"HEAD", "/get-only", "GET", 405},
		{"PUT", "/multi", "GET, POST", 405},
		{"DELETE", "/multi", "GET, POST", 405},
		{"GET", "/nope", "", 404},
		{"GET", "/get-only", "", 200},
	}
	// 逐条断言状态码与 Allow（404/200 分支要求 Allow 为空）
}
```

覆盖点：单方法、多方法、OPTIONS、HEAD、404 不带 `Allow`、正常 200 不带 `Allow`。

## 4. 集成测试

新增包 `test/integration/tests/http_semantics/`（`TestMain` 沿用既有 `testutil.StartServer()` 模板），
用 `net/http` 直接发请求以读取响应头（现有 `testutil.Client` 只回 `APIResponse`，不含 header）：

| TC | 请求 | 断言 |
|---|---|---|
| HS-1-001 | `POST /open-api/v1/route-tables` | 405，`Allow` 含 `GET`，`Content-Type: application/json`，body `ErrNum=405` |
| HS-1-002 | `OPTIONS /open-api/v1/route-tables` | 405，`Allow` 非空且含 `GET` |
| HS-1-003 | `GET /open-api/v1/route-tables` | 200，无 `Allow` |
| HS-1-004 | `DELETE /inner-api/v1/configs/ai-route` | 405，`Allow` 含 `GET`（InnerAPI 同一 handler） |
| HS-1-005 | `POST /open-api/v1/nope` | 404，无 `Allow` |

如不希望新增包，可放在 `tests/schema/openapi/`（已有 `sm` 实例）——但 HTTP 语义检查与 OpenAPI schema
职责不同，建议独立包，后续 OPTIONS/HEAD/缓存相关用例可继续扩展。

## 5. 文档同步

| 位置 | 改动 |
|---|---|
| `design-docs/api-define/OpenAPI接口定义/00-common.md:23-33` | ErrNum 列表在 404 与 409 之间新增：`405：请求方法不被目标路由支持（响应头 Allow 列出该资源支持的方法）` |
| 同上（Method约定小节后） | 补一句：不支持的 method 返回 405 且带 `Allow`；普通 OPTIONS 亦返回 405 + `Allow` |
| `design-docs/api-define/InnerAPI接口定义/01-common.md` | 可选：错误码小节补 405 说明（InnerAPI 共用同一 handler） |
| 本目录 | `change-summary.md` / `design-changes.md` / `api-changes.md` |

## 6. 验证步骤

```bash
# 1) 单测
go test ./endpoints/ -run TestMethodNotAllowed -count=1 -v

# 2) 集成（SQLite）
go build -o ai-gateway-api.exe .
cd test/integration
AIAPI_FORCE_SQLITE=1 go test -count=1 -run TestMethodNotAllowed ./tests/http_semantics/ -v

# 3) 手工核对（服务已起）
curl -i -X POST    http://127.0.0.1:8183/open-api/v1/route-tables     # 405 + Allow: GET
curl -i -X OPTIONS http://127.0.0.1:8183/open-api/v1/route-tables     # 405 + Allow: GET
curl -i -X DELETE  http://127.0.0.1:8183/inner-api/v1/configs/ai-route # 405 + Allow: GET
curl -i -X POST    http://127.0.0.1:8183/open-api/v1/nope             # 404，无 Allow

# 4) 全量回归
go test ./... -count=1
AIAPI_FORCE_SQLITE=1 go test -count=1 ./tests/...   # 或按 CI 既有范围
```

外部 harness（issue 中的 `CROSS-ROUTE-001` / `CR-1-001` / `CR-1-002` 与
`cross-route-001-report.json`）不在本仓库，需修复后由测试方复跑，R3/R5 转 `pass` 后关闭 `P2-05-C3`/`C5`。

## 7. 备选方案

| 方案 | 做法 | 结论 |
|---|---|---|
| A（采纳） | 405 handler 内逐方法 `Router.Match` 探测 | 穿透子路由，如实反映 matcher（含路径变量/Header 约束），无状态、无缓存失效问题 |
| B | 遍历 `router.routes` + `Route.GetMethods()` | mux 自带 `getAllMethodsForRoute` 的做法，但**无法穿透 `Subrouter()`**，在本项目几乎拿不到方法 |
| C | 维护「路径 → 方法集合」静态映射表 | 需与路由注册同步维护，漏改即错；且路径变量/前缀匹配难对齐 |
| D | 把 `Allow` 放到中间件统一注入 | 中间件只包裹命中的路由（mux 语义），405 不经过中间件，无法生效 |
| E | OPTIONS 返回 200 + `Allow` | 语义更友好，但会改变现有 405 行为并可能与 CORS 预检混淆；issue 验收允许 405，故不做 |

## 8. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 探测算法把 404/405 的 `MatchErr` 误当命中 | 显式要求 `MatchErr == nil && match.Route != nil`；单测覆盖 404 与多方法路径 |
| 未来新增自定义 method（如 `PROPFIND`）不在探测列表 | 探测列表与 `00-common.md` 的 Method 约定保持同步；如需扩展，追加到 `allowedMethodProbe` 常量切片 |
| 405 响应仍不经过 `router.Use` 中间件（无访问日志/CORS） | 既有行为，本 issue 不改；如需，另案在 handler 内手动包 middleware 链（与 `staticHandler` 同样的做法） |

回滚：把 `router.MethodNotAllowedHandler` 改回内联 JSON 405 即可，改动局部且无数据影响。

## 9. 方案原型验证（2026-11-03）

§2.1 的探测算法已用合成 router（`mux.NewRouter()` + `PathPrefix(...).Subrouter()`，与生产拓扑同构）
原型跑通，7 条断言全部符合预期，验证后删除临时文件：

| 请求 | 实测 `Allow` | 说明 |
|---|---|---|
| `POST /open-api/v1/get-only`（子路由，仅 GET） | `GET` | 证实探测可穿透子路由 |
| `OPTIONS /open-api/v1/get-only` | `GET` | 普通 OPTIONS 同样拿到 Allow |
| `HEAD /open-api/v1/get-only` | `GET` | 不虚报 HEAD 支持 |
| `PUT /open-api/v1/multi`（GET+POST） | `GET, POST` | 多方法路径顺序稳定 |
| `GET /root-post`（根路由，仅 POST） | `POST` | 根级路由同样正确 |
| `GET /open-api/v1/get-only` | 无 | 200 分支不带 Allow |
| `GET /nope` | 无 | 404 分支不带 Allow |

同时确认了 §1.2 的关键前提：当 `MethodNotAllowedHandler` / `NotFoundHandler` 已安装时，
`Router.Match` 对 405/404 也返回 `true`，因此探测条件必须同时要求 `MatchErr == nil && match.Route != nil`。

实现落地后的验证（2026-11-03）：

- `endpoints/router_test.go`（`TestMethodNotAllowedAllowHeader` 8 子场景 + `TestAllowedMethodsSkipsRequestMethod`）通过；
- 集成 `test/integration/tests/http_semantics`：真实服务上 `POST`/`OPTIONS /open-api/v1/route-tables`
  与 `DELETE /inner-api/v1/configs/ai-route` 均返回 405 + `Allow: GET`，`GET` 200 与未知路径 404 均无 `Allow`；
- 修复前同一服务实例上这三个请求 `Allow` 均为空（§1.3 实测表），构成修复前后的对照。