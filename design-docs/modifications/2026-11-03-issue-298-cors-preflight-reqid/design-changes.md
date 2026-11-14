# 设计变更：#298 让 CORS 预检与 `Req-ID` 覆盖路由级 404/405

## 1. 现状

### 1.1 中间件装配方式

```go
// endpoints/router.go:67-71
router.Use(middleware.MCRecovery)
router.Use(middleware.MCLogger)
router.Use(middleware.McIPProbe)
router.Use(middleware.MCCors)
```

`router.Use` 的效果由 mux 内部决定（`mux.go` `Router.Match`）：

```go
for _, route := range r.routes {
    if route.Match(req, match) {
        if match.MatchErr == nil {           // ← 只有「干净命中」才装配中间件
            for i := len(r.middlewares) - 1; i >= 0; i-- {
                match.Handler = r.middlewares[i].Middleware(match.Handler)
            }
        }
        return true
    }
}
if match.MatchErr == ErrMethodMismatch {
    if r.MethodNotAllowedHandler != nil { match.Handler = r.MethodNotAllowedHandler; return true }
}
if r.NotFoundHandler != nil { match.Handler = r.NotFoundHandler; match.MatchErr = ErrNotFound; return true }
```

因此 `NotFoundHandler` / `MethodNotAllowedHandler` **天然不在 `router.Use` 链内**。

### 1.2 `Req-ID` 的来源

```go
// endpoints/middleware/recovery.go:42-45
ctx := lib.NewLogContext(req.Context())
ctx, requestInfo := xreq.InitRequestInfo(ctx, req)
requestInfo.LogID = lib.GainLogID(ctx)
req = req.WithContext(ctx)

// lib/xreq/result.go:199
w.Header().Add("Req-ID", requestInfo.LogID)
```

`xreq.Render` 里会 `InitRequestInfo`：ctx 里没有就用空 `LogID` 新建一个。所以 404/405 直接 Render 时
`Req-ID` 为空字符串，且 `MCLogger` 的 `Record` 从未执行（无访问日志）。

### 1.3 预检为何 405

`rs/cors@v1.8.0` 的 negroni 适配：

```go
func (c *Cors) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		c.handlePreflight(w, r)
		if c.optionPassthrough { next(w, r) } else { w.WriteHeader(http.StatusNoContent) }
		return
	}
	c.handleActualRequest(w, r)
	next(w, r)
}
```

它本身就会把预检短路成 **204**（`optionPassthrough` 默认 false）。问题只是它挂在 `router.Use` 上，
预检请求在 mux 里匹配不到任何 route（三个 API 都没有注册 OPTIONS），于是根本不进这条链，落到 405 handler。

### 1.4 既有先例

`router.go:73-81` 静态分支已手工套链（Recovery→Logger→IPProbe→Cors），注释写明 mux 不套 Use 链。
本变更沿用同一手法，只是 API 分支**不套 IPProbe**（见 §3）。

## 2. 方案

### 2.1 helper

```go
// endpoints/router.go
// withBaseMiddlewares wraps a handler with the middlewares that are independent
// of routing and must cover every response: panic recovery + Req-ID, access log
// and CORS (which short-circuits preflight with 204 + Access-Control-Allow-*).
//
// The management-plane IP allowlist is deliberately NOT included: route-level
// 404/405 must stay 404/405 and never turn into 403 (API testing standard
// §5.5.2 #4). Real requests remain protected because matched routes still run
// McIPProbe.
func withBaseMiddlewares(h http.Handler) http.Handler {
	h = middleware.MCCors.Middleware(h)
	h = middleware.MCLogger.Middleware(h)
	h = middleware.MCRecovery.Middleware(h)
	return h
}
```

包装顺序即执行顺序：`Recovery → Logger → Cors → handler`（`Middleware` 由内向外包，最后包的在最外层）。

### 2.2 接线（替换 `router.go:83-92` 与 `router.go:65`）

```go
notFoundJSON := withBaseMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	res := &xreq.Result{Code: 404, ErrMsg: "Not Found"}
	xreq.Render(w, r, res)
}))

router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// API 路径未匹配时返回 JSON 404，避免返回静态文件 HTML
	if strings.HasPrefix(r.URL.Path, "/open-api/v1") || strings.HasPrefix(r.URL.Path, "/inner-api/v1") {
		notFoundJSON.ServeHTTP(w, r)
		return
	}
	// 静态分支保持自身全链（含 IPProbe），不能在这里再包一层，否则双重访问日志
	staticHandler.ServeHTTP(w, r)
})

router.MethodNotAllowedHandler = withBaseMiddlewares(methodNotAllowedHandler(router))
```

`methodNotAllowedHandler` 本身不变（issue #297 的 `Allow` 逻辑保留），只是外层多了一层链。

### 2.3 请求/响应流

| 请求类型 | 路径 | 链路 | 结果 |
|---|---|---|---|
| 预检（OPTIONS + Origin + ACRM） | 已注册路由 | Recovery → Logger → **Cors（短路）** | `204` + `ACAO: *` + `ACAM: <请求方法>` (+`ACAH`/`ACAC`)；不写 `Req-ID`（Cors 直接写状态，不经 Render） |
| 预检 | 未知 API 路径 | Recovery → Logger → Cors（短路） | `204` + 同上（浏览器随后发起真实请求，拿到 404） |
| 预检 | 静态路径 | 静态全链（Recovery→Logger→IPProbe→Cors） | 与现状一致：IP 允许 `204`；IP 拒绝 `403` |
| 普通 OPTIONS（无 ACRM） | 已注册路由 | Recovery → Logger → Cors（actual）→ 405 handler | `405` + `Allow` + `Req-ID` +（有 Origin 时）`ACAO` |
| 任意方法 | 未知 API 路径 | Recovery → Logger → Cors（actual）→ JSON 404 | `404` + `Req-ID` +（有 Origin 时）`ACAO` |
| 不支持的方法 | 已注册路由 | Recovery → Logger → Cors（actual）→ 405 handler | `405` + `Allow`（#297）+ `Req-ID` |
| 正常请求 | 已注册路由 | Recovery → Logger → IPProbe → Cors → … → handler | 不变（200 + `Req-ID`） |

## 3. 契约与取舍

1. **路由级 404/405 不得执行认证/授权/IP 白名单**（标准 §5.5.2 第 4 条）：
   helper **不含** `McIPProbe`，也不含 `McProductProbe` / `McUserProbe`（后者本来就只挂在子路由）。
2. **预检不经过 IP 白名单**：这是本方案的已知取舍 —— 预检不携带凭据、不触达业务逻辑，不构成授权面；
   真实请求仍会被 `McIPProbe` 403（`router.Use` 链在命中路由上照常生效）。
   若安全评审要求「预检也受 IP 白名单约束」，可改为在 helper 里对预检条件性插入 `McIPProbe`
   （见 §9 方案 B），代价是受拒预检拿不到 CORS 头、浏览器侧表现为 CORS 错误而非 403。
3. **静态分支不动**：其链已含 IPProbe，且注释里固化了「IP 白名单对 Dashboard 路径生效」的 MAC-1-003 契约。
4. **不改变 404/405 的状态码与响应体**（仍 `404`/`405` + `{"ErrNum":..,"ErrMsg":..}`，405 仍带 `Allow`）。

## 4. 边界与细节

| 边界 | 处理/说明 |
|---|---|
| 双重访问日志 | 只给 API JSON 404 与 405 包链；静态分支保持原样，避免 Recovery/Logger 执行两次 |
| `MCLogger` 的 `rw.(negroni.ResponseWriter)` 断言 | 生产链路最外层是 negroni（`main.go:135 n.UseHandler(router)`），404/405 拿到的仍是 negroni 包装过的 writer；静态分支已有同样依赖。单测里用 `negroni.NewResponseWriter(recorder)` 构造 |
| `stateful.AccessLogger` 未初始化（单测） | `log4go.Logger` 是 map 类型，nil map 上调用 `Info/Warn` 安全；测试也可显式赋空 `log4go.Logger{}` |
| 预检 204 无 `Req-ID` | 验收只要求 404/405 带 `Req-ID`；预检仍会被 Logger 记录（日志里有 ID），响应头级别若要统一注入需在更外层（另案） |
| `Vary` 头 | `rs/cors` 在预检与实测请求上都会 `Vary: Origin, Access-Control-Request-Method, Access-Control-Request-Headers`，新增属正确行为 |
| 405 + 预检共存 | 预检请求被 Cors 短路，永远到不了 405 handler，因此 #297 的 `Allow` 只在非预检 OPTIONS/错误方法上出现，互不干扰 |
| 静态文件 405 | 由 `net/http` FileServer 产生，不在本 issue 范围（与 #297 同一边界） |
| HEAD 语义（issue 观察项 C4） | `AllowedMethods` 含 HEAD 但 mux 不隐式支持 HEAD，属另案裁定，本变更不处理 |

## 5. 单元测试

`endpoints/router_test.go` 追加（复用 #297 已有的 `newMethodNotAllowedTestRouter`）：

```go
func TestCorsPreflightIsShortCircuited(t *testing.T) {
	stateful.AccessLogger = log4go.Logger{} // 避免依赖真实日志初始化

	router := newMethodNotAllowedTestRouter() // 内部需按 §2.2 接线
	req := httptest.NewRequest(http.MethodOptions, "/open-api/v1/get-only", nil)
	req.Header.Set("Origin", "https://console.example.com")
	req.Header.Set("Access-Control-Request-Method", "PATCH")

	rec := httptest.NewRecorder()
	router.ServeHTTP(negroni.NewResponseWriter(rec), req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin")) // AllowedOrigins: * 时 rs/cors 返回 *
	assert.Equal(t, "PATCH", rec.Header().Get("Access-Control-Allow-Methods"))
}

func TestRouteLevelErrorsCarryReqID(t *testing.T) {
	// 404：未知 API 路径；405：错误方法；200：命中路由
	// 三者都断言 Req-ID 非空；405 另断言 Allow == "GET"
}
```

覆盖点：预检 204 + ACAO/ACAM（含未知路径）、404/405 `Req-ID` 非空、405 仍带 `Allow`、200 不受影响。

注意：200 分支的 `Req-ID` 依赖 `router.Use(MCRecovery)`，单测里需同样调用
`router.Use(middleware.MCRecovery)` 等（与生产一致），否则命中路由的请求拿不到 `LogID`——
这一点在方案原型中实测确认过（见 §10）。

## 6. 集成测试

扩展 `test/integration/tests/http_semantics`（#297 新建的包，已有真实服务 + `net/http` 直读响应头）：

| TC | 请求 | 断言 |
|---|---|---|
| HS-2-001 | `OPTIONS /open-api/v1/route-tables` + `Origin` + `ACRM: PATCH` | 2xx；`Access-Control-Allow-Origin` 非空且等于 origin；`Access-Control-Allow-Methods` 含 `PATCH` |
| HS-2-002 | `OPTIONS /open-api/v1/nope` + `Origin` + `ACRM: GET` | 2xx；`Access-Control-Allow-Origin` 非空 |
| HS-2-003 | `GET /open-api/v1/nope` | 404；`Req-ID` 非空 |
| HS-2-004 | `POST /open-api/v1/route-tables` | 405；`Req-ID` 非空；`Allow` 仍为 `GET` |
| HS-2-005 | `GET /open-api/v1/route-tables` | 200；`Req-ID` 非空（对照） |
| HS-2-006（可选） | 同一 404 请求两次 | `Req-ID` 不同（每个请求独立 ID），确认不是常量 |

IP 白名单不生效的验证（验收 3）：默认 `[AccessControl]` 关闭，集成环境无法直接构造拒绝；
可单测断言 helper 不含 `McIPProbe`（例如用 `ip_probe` 的 403 行为做反向证明），或在 MySQL/配置环境跑一次。

## 7. 备选方案

| 方案 | 做法 | 结论 |
|---|---|---|
| A（采纳） | helper 包裹 API JSON 404 与 405，链为 Recovery→Logger→Cors | 改动局部、与静态分支先例一致；匹配路由行为零变化 |
| B | 全局链：`main.go` 里 `n.UseHandler(withBaseMiddlewares(router))`，同时从 `router.Use` 摘掉 Recovery/Logger/Cors | 结构最干净（所有响应一条链），但会改变 IPProbe 与 CORS 的相对顺序、并需重排静态分支的包裹（否则双日志）；回归面大，暂不做 |
| C | 为三个 API 各注册一条 `OPTIONS /{path:.*}` 路由 | 会让预检进入路由与鉴权链（违反 §5.5.2 #4 的例外要求）、且要维护通配路由；预检仍需 CORS 头，等于绕远路 |
| D | 只在 `main.go` 之外再加一层「预检专用」handler | 与 B 类似的全局改动，且仍解决不了 404/405 的 `Req-ID` |
| E | 在 `xreq.Render` 里兜底生成 `Req-ID` | 能补头但补不上访问日志与 panic 兜底，且 ID 与访问日志不一致（违反验收 2 的一致性要求） |

## 8. 验证步骤

```bash
# 1) 单测
go test ./endpoints/ -run 'TestCorsPreflight|TestMethodNotAllowed|TestRouteLevelErrors' -count=1 -v

# 2) 集成
go build -o ai-gateway-api.exe .
cd test/integration
AIAPI_FORCE_SQLITE=1 go test -count=1 -run 'TestMethodNotAllowed|TestCORS|TestHTTPHeaders' -v ./tests/http_semantics/

# 3) 手工核对
curl -i -X OPTIONS http://127.0.0.1:8183/open-api/v1/route-tables \
  -H 'Origin: https://console.example.com' -H 'Access-Control-Request-Method: PATCH'
# 期望 204 + Access-Control-Allow-Origin/-Methods

curl -i http://127.0.0.1:8183/open-api/v1/definitely-not-a-route
# 期望 404 + Req-ID 非空

curl -i -X POST http://127.0.0.1:8183/open-api/v1/route-tables
# 期望 405 + Allow: GET + Req-ID 非空

# 4) 回归
go test ./... -count=1
AIAPI_FORCE_SQLITE=1 go test -count=1 ./tests/...
```

外部 harness（`CROSS-ROUTE-001` 的 R2/R6、`cross-route-001-report.json`）不在本仓库，需由测试方复跑。

## 9. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 误把静态分支也包一层 → 双重日志 | 接线图已明确只包 API JSON 404 与 405；review 时按 §2.2 对照 |
| 单测因 `stateful.AccessLogger`/negroni writer 缺失而 panic | 测试里显式 `stateful.AccessLogger = log4go.Logger{}` + `negroni.NewResponseWriter` |
| 预检被 IP 白名单放行（安全评审关注） | 记录为已知取舍（§3.2）；需要时按方案 B 收紧 |
| 404/405 访问日志新增造成噪音 | 非 200 走 Warn 级别；如需采样可后续调整 Logger |

回滚：把 `router.NotFoundHandler` / `router.MethodNotAllowedHandler` 改回直接 Render 即可（一行级回退），
`withBaseMiddlewares` 保留不用也没有副作用。

## 10. 实施与验证记录（2026-11-03）

§2 的方案已落地（`endpoints/router.go`），单测与集成实测结果与原型一致：

| 请求 | status | ACAO | ACAM | Allow | Req-ID |
|---|---|---|---|---|---|
| 预检（已注册路由，Origin + ACRM: PATCH） | 204 | `*` | `PATCH` | — | 空（Cors 短路，不经 Render） |
| 预检（未知路径） | 204 | `*` | `PATCH` | — | 空 |
| 未知路径 `GET` | 404 | — | — | — | 非空 |
| 已注册路由 `POST`（仅 GET） | 405 | — | — | `GET` | 非空 |
| 普通 `OPTIONS`（带 Origin） | 405 | `*` | — | `GET` | 非空 |
| 已注册路由 `GET` | 200 | — | — | — | 非空 |

单测落在 `endpoints/router_test.go` 的 `TestPreflightAndRouteErrorsCarryHeaders`（用
`newMiddlewareWiredTestRouter` 复刻生产接线：404/405 带 base 链、命中路由带 `router.Use(MCRecovery)`），
集成用例为 `tests/http_semantics` 的 HS-2-001~007。

同时确认了两个事实，避免文档写错：

1. `rs/cors v1.8.0` 在 `AllowedOrigins: ["*"]` 时一律返回 `ACAO: *`（不因 `AllowCredentials` 回显 Origin）；
2. 预检响应会带 `Vary: Origin, Access-Control-Request-Method, Access-Control-Request-Headers`，
   但不带 `Req-ID`（Cors 直接写状态码，不经过 `xreq.Render`）——与验收判据 2（只要求 404/405 带 `Req-ID`）一致。

实现中的一个注意点：命中路由的 `Req-ID` 由 `router.Use(MCRecovery)` 提供，因此单测若只包 404/405
而不装 `router.Use`，200 分支将没有 `Req-ID`（原型阶段踩到过）；生产链路由 `RegisterRouters` 装配，无此问题。