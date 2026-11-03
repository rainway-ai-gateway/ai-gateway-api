# Issue #298：CORS 预检返回 405 且无 `Access-Control-Allow-*`；路由级 404/405 缺 `Req-ID`

## 1. 问题来源

[rainway-ai-gateway/ai-gateway-api#298](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/298)

> 服务已配置 CORS（`AllowedOrigins: *`、`AllowedMethods` 含 HEAD），但带 `Origin` + `Access-Control-Request-Method`
> 的 CORS 预检返回 405 且无任何 `Access-Control-Allow-*` 响应头；同时路由级 404/405 响应缺少 `Req-ID`。

## 2. 根因

### 2.1 链路（源码直读）

```
main.go:134-136                 negroni（无中间件）→ mux router
endpoints/router.go:67-71       router.Use(MCRecovery, MCLogger, McIPProbe, MCCors)   ← 只对「匹配成功的路由」生效
endpoints/router.go:83-92       router.NotFoundHandler = 直接 xreq.Render(JSON 404)    ← 未套链
endpoints/router.go:65          router.MethodNotAllowedHandler = methodNotAllowedHandler(router) ← 未套链
```

gorilla/mux v1.8.0 的 `Router.Use` 注册的中间件只在**路由命中**时包装 handler（`Router.Match` 里
`if match.MatchErr == nil` 才装配 middleware 链）；`NotFoundHandler` / `MethodNotAllowedHandler`
是「未命中 / 方法不匹配」的兜底，**不经过 Use 链**。

由此产生两个后果：

1. **预检拿不到 CORS 头**：OPTIONS 预检不匹配任何 route（三个 API 都只注册 GET/POST/PUT/PATCH/DELETE），
   走 405 handler → `MCCors` 从未执行 → 无 `Access-Control-Allow-Origin/-Methods` → 浏览器按 CORS 协议阻断。
2. **`Req-ID` 为空**：`Req-ID` 来自 `xreq.Render`（`lib/xreq/result.go:199` 写 `requestInfo.LogID`），
   而 `LogID` 只在 `MCRecovery` 里初始化（`endpoints/middleware/recovery.go:45` `lib.GainLogID(ctx)`）。
   404/405 不走 Recovery → `LogID` 为空字符串 → 响应头 `Req-ID` 为空，且没有访问日志条目可对照。

### 2.2 既有旁证

`endpoints/router.go:73-81` 的静态资源分支已经**手工包裹**了同一套链
（`MCCors → McIPProbe → MCLogger → MCRecovery`，执行序相反），注释明确写了
「mux v1.8 的 ServeHTTP 对 NotFoundHandler 不应用 Use 注册的中间件」——
说明该绕过已被识别，只是当时只修了静态分支，API 分支的 JSON 404/405 没有跟进。

## 3. 实测证据（2026-11-03，HEAD `db153ef`，SQLite，真实服务）

| 请求 | 状态 | `ACAO` | `ACAM` | `Req-ID` |
|---|---|---|---|---|
| 预检 `OPTIONS /open-api/v1/route-tables`（Origin + ACRM: PATCH） | 405 | 空 | 空 | 空 |
| 预检 `OPTIONS /open-api/v1/nope`（同上） | 404 | 空 | 空 | 空 |
| `GET /open-api/v1/nope` | 404 | 空 | 空 | 空 |
| `POST /open-api/v1/route-tables` | 405 | 空 | 空 | 空 |
| `GET /open-api/v1/route-tables` | 200 | 空 | 空 | `1793684079703241400_101` |
| 普通 `OPTIONS /open-api/v1/route-tables`（无 ACRM） | 405 | 空 | 空 | 空 |

与 issue 记录一致：预检 405 + 无 ACAO；路由级 404/405 `Req-ID` 为空；正常路由 200 有 `Req-ID`（对照组）。

## 4. 目标

1. CORS 预检（OPTIONS + `Origin` + `Access-Control-Request-Method`）返回 **2xx**，且
   `Access-Control-Allow-Origin` / `Access-Control-Allow-Methods` 非空；
2. 路由级 404 / 405 响应 `Req-ID` 非空，并产生对应访问日志（ID 一致）；
3. **不改变**「路由级 404/405 不执行认证/授权/IP 白名单」的既有契约（不得变成 401/403，
   见标准 §5.5.2 第 4 条）；
4. 相邻行为不回归：已注册方法仍 200 + `Req-ID`；未知路径仍 JSON 404；405 仍带 `Allow`（issue #297）；
5. 不引入重复日志/重复中间件执行。

## 5. 方案概览

把 **Recovery → Logger → CORS** 这三层「与路由无关、必须覆盖所有响应」的中间件，
手工套到两个兜底 handler 上（与静态分支同一思路，抽出 helper 复用）；**不含 `McIPProbe`**，
以守住 §5.5.2 第 4 条。预检由 `rs/cors` 直接短路返回 204（不进入路由逻辑）：

```go
// endpoints/router.go
// 路由级 404/405 不进 router.Use 链（mux v1.8 只对命中路由套中间件），
// 但 CORS 预检与 Req-ID 必须覆盖它们。
func withBaseMiddlewares(h http.Handler) http.Handler {
	h = middleware.MCCors.Middleware(h)    // 预检在此短路返回 204 + ACAO/ACAM
	h = middleware.MCLogger.Middleware(h)  // 访问日志（含 404/405/预检）
	h = middleware.MCRecovery.Middleware(h)// LogID 初始化 + panic 兜底
	return h
}

notFoundJSON := withBaseMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	xreq.Render(w, r, &xreq.Result{Code: 404, ErrMsg: "Not Found"})
}))

router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/open-api/v1") || strings.HasPrefix(r.URL.Path, "/inner-api/v1") {
		notFoundJSON.ServeHTTP(w, r)   // 只包 API 分支，静态分支保持自身全链（含 IPProbe）
		return
	}
	staticHandler.ServeHTTP(w, r)
})

router.MethodNotAllowedHandler = withBaseMiddlewares(methodNotAllowedHandler(router))
```

关键点：

- **预检短路**：`rs/cors v1.8.0` 的 negroni 适配 `ServeHTTP` 在识别到预检时
  `handlePreflight` + `w.WriteHeader(204)` 且**不再调用 next**（`optionPassthrough` 默认 false），
  因此 405 handler 的 `Allow` 逻辑不会干扰预检，预检也不会被 405；
- **不含 IPProbe**：路由级 404/405 一旦被 IP 白名单拦成 403 就违反 §5.5.2 第 4 条；
  预检本身不构成授权面（真实请求仍会被 IP 白名单 403），该取舍记录在 design-changes §3；
- **静态分支不动**：静态 handler 已含 IPProbe + CORS + Logger + Recovery，若再被外层包一次会出现
  **双重访问日志**，因此 helper 只包 API 的 JSON 404 分支和 405 handler。

详细设计、边界矩阵、测试与验证见 [design-changes.md](./design-changes.md)；
响应头与状态码变化见 [api-changes.md](./api-changes.md)。

## 6. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api` |
| 代码 | `endpoints/router.go`（新增 `withBaseMiddlewares`，改写 404/405 接线） |
| 文档 | `design-docs/api-define/OpenAPI接口定义/00-common.md`（预检/`Req-ID` 约定）、`sys-design/接口层设计文档.md`（中间件链覆盖范围）、本目录 |
| 测试 | `endpoints/router_test.go` 补预检/Req-ID 用例；`test/integration/tests/http_semantics` 补 HS-2-00x |
| 数据库/数据面 | 无影响（纯 HTTP 语义与可观测性） |
| 兼容性 | 预检 405 → 204（行为变化，符合 CORS 协议）；404/405 新增 `Req-ID`；其它不变 |

## 7. 风险

| 风险 | 评估 | 缓解 |
|------|------|------|
| 双重中间件执行导致重复日志 | 中 | 只包 API JSON 404 与 405 两处；静态分支保持原样；集成测试断言 404 只有一条访问日志（可选） |
| `MCLogger` 依赖 `negroni.ResponseWriter` | 中 | 生产链路由 negroni 包裹（`main.go:135`）；单测用 `negroni.NewResponseWriter` 包装 recorder；与静态分支同一前提 |
| 预检绕过 IP 白名单 | 中 | 仅影响预检（无数据面）；真实请求仍 403；已在 design-changes §3 记录权衡与替代方案 |
| 预检 204 不带 `Req-ID` | 低 | 验收只要求 404/405 带 `Req-ID`；如需可在更外层统一注入（另案） |
| 404/405 访问日志量增加 | 低 | 属预期（可观测性目标），日志级别非 200 为 Warn |

## 8. 上线前检查清单

1. `go test ./endpoints/ -run 'TestCors|TestNotFound|TestMethodNotAllowed' -count=1` 通过；
2. `AIAPI_FORCE_SQLITE=1 go test -count=1 -run TestHTTPHeaders ./tests/http_semantics/` 通过；
3. `curl -i -X OPTIONS http://<server>/open-api/v1/route-tables -H 'Origin: https://console.example.com' -H 'Access-Control-Request-Method: PATCH'`
   → `204`，`Access-Control-Allow-Origin: *`、`Access-Control-Allow-Methods: PATCH`；
4. `curl -i http://<server>/open-api/v1/definitely-not-a-route` → `404` 且 `Req-ID` 非空；
5. `curl -i -X POST http://<server>/open-api/v1/route-tables` → `405` + `Allow: GET` + `Req-ID` 非空；
6. 未在白名单的 IP 访问 API 未知路径仍为 `404`（不因 IP 白名单变 403）；
7. `go test ./...` 与 SQLite 集成全量回归。

## 9. 实施记录（Step 5 / Step 6）

### 9.1 代码改动

| 文件 | 改动 |
|------|------|
| `endpoints/router.go` | 新增 `withBaseMiddlewares`（CORS → Logger → Recovery，执行序相反）；405 handler 与 API 的 JSON 404 分支改为经该链包装；静态分支保持原样（避免双份访问日志） |
| `endpoints/router_test.go` | 新增 `newMiddlewareWiredTestRouter`（与生产接线一致）与 `TestPreflightAndRouteErrorsCarryHeaders`：预检 204 + ACAO/ACAM（含未知路径）、404/405/200 的 `Req-ID`、405 仍带 `Allow`、普通 OPTIONS 仍 405 |
| `test/integration/tests/http_semantics/method_not_allowed_test.go` | `doRaw` 支持自定义请求头；新增 `TestCORSAndReqIDHeaders`（HS-2-001 ~ 007，含「两次 404 的 Req-ID 不同」） |

### 9.2 文档改动

| 文件 | 改动 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/00-common.md` | 返回值格式补「响应头」表（`Req-ID` / `Allow`）与「CORS 与预检」小节 |
| `design-docs/api-define/InnerAPI接口定义/01-common.md` | 新增「响应头与 CORS」小节（原「版本控制机制」顺延为 §5） |
| `design-docs/sys-design/接口层设计文档.md` | 新增 §3.1「未命中路由的中间件覆盖」（helper、执行序、三条约束） |
| 本目录 | 三份变更说明 |

### 9.3 验证结果（2026-11-03，Go 1.25.2 / windows-amd64）

| 验证项 | 结果 |
|---|---|
| `go test ./endpoints/ -run 'TestMethodNotAllowed\|TestAllowedMethods\|TestPreflightAndRouteErrors'` | PASS（6 子场景 + 既有 #297 用例） |
| `go test ./...` | 104 个包 `ok`，0 FAIL |
| 集成 `tests/http_semantics` | PASS：HS-2-001~007 全部通过（预检 204 + `ACAO: *` + `ACAM`、404/405 `Req-ID` 非空、405 保留 `Allow: GET`、普通 OPTIONS 405 + CORS 头、两次 404 的 `Req-ID` 不同） |
| 集成全量 `./tests/...`（SQLite） | 见 §9.4 |

### 9.4 全量集成运行的说明（含对照实验）

| 运行 | 环境 | 结果 |
|---|---|---|
| A | #298 构建 + 全量单元测试**并发**跑 | 125 ok / 5 fail（`api_key/quota_update`、`entity/quota_update`、`auth/password_hash`、`clusters/create`、`innerapi/tls_conf`） |
| B | #298 构建，只跑集成 | 127 ok / 3 fail（`api_key/quota_update`、`innerapi/tls_conf`、`provider/k8s_pool`） |
| C（对照） | **回退 #298 代码** 重新构建，只跑集成 | 127 ok / 3 fail（`clusters/delete`、`innerapi/tls_conf`、`http_semantics`） |
| D（复核） | #298 构建，MySQL 8.4.9 后端（`AIAPI_MYSQL_DSN`，默认并行） | **130 个包 `ok`，0 FAIL** |

运行 D 使用同一份代码与同一命令，仅把后端从 SQLite 换成 MySQL（`AIAPI_MYSQL_DSN`，每包独立
`ai_gateway_it_*` 库、用完 `DROP`），全量 130 包一次通过、无任何 DB-500 —— 进一步印证 A/B/C 的失败
源于 SQLite 在并行跑测下的写竞争（`database is locked` 类），而非产品缺陷。
另用 `-tags mysql` 复跑了并发专项：`AC-1-012`、`CTX-1-005`、`TM-1-015`、`TestAPIKey_CreateConcurrentMySQL`、
`TestEntity_CreateConcurrentMySQL`、`TestIntentConfig_Update_ConcurrentMySQL` 以及 #295 的
`AC-1-019` / `CTX-1-006`（409 语义）全部 PASS。

失败形态统一为 `500 Database Exception: database operation failed`（#295 F-14 脱敏后的 DAO 错误文案），
且**失败的包每次都不一样**；把失败的包单独串行重跑（`-p 1`）全部 `ok`。对照运行 C 在**没有** #298 改动时
同样出现同类 DB-500 失败（包不同），因此判断为并行跑测时的 SQLite 写竞争 / 负载问题，与本变更无关
（本变更只在 404/405 兜底 handler 外层加中间件，不触碰任何写路径）。

运行 C 中 `tests/http_semantics` 如预期 **FAIL**（该包断言 #298 的行为）——这正好构成反向对照：
新增用例确实能捕获本缺陷（回退代码后失败，修复后通过）。

> 附带观察：#295 的 F-14 脱敏让这类 500 的响应体只剩「database operation failed」，
> 现场排查必须依赖服务端日志（`ResolveResult.FullMsg()`）。这是脱敏的预期代价，如需在响应里保留
> 错误分类（而不泄露原文），可另案加一个稳定的 `error_code` 字段。

### 9.5 沉淀判断（Step 6）

不新增 `sys-design/details/` 文档：改动是路由层一个 helper + 两处接线（约 20 行），
中间件覆盖面与约束已写进《接口层设计文档》§3.1，接口侧约定写进 `00-common.md`；
`details/` 定位是配额同步、导出与版本控制这类复杂机制，此处再拆会造成双份维护。

### 9.6 遗留

1. **预检不带 `Req-ID`**：CORS 短路直接写 204，不经过 `xreq.Render`；验收只要求 404/405 带，
   如需全响应统一注入，应在更外层（main.go 的 negroni 链）加一个 header 中间件（另案）；
2. **预检绕过 IP 白名单**：见 design-changes §3.2 的取舍；若安全评审要求收紧，按 §7 方案 B；
3. **`rs/cors v1.8.0` 在 `AllowedOrigins: *` 下固定返回 `ACAO: *`**：带 credentials 的浏览器请求会被拒，
   属既有配置+依赖版本行为；要回显具体 Origin 需升级依赖或改白名单（另案）；
4. 外部 harness（`CROSS-ROUTE-001` R2/R6、`cross-route-001-report.json`）不在本仓库，需测试方复跑确认。

## 10. 参考

- [Issue #298](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/298)
- WHATWG Fetch / CORS 协议（预检须 2xx 且带 `Access-Control-Allow-*`）；项目标准 §5.5.2 第 4/5 条、F-10
- `endpoints/router.go:65-92`、`endpoints/middleware/recovery.go:42-45`（`LogID`）、
  `endpoints/middleware/access_logger.go:56-70`（负尼 ResponseWriter 依赖）、`lib/xreq/result.go:199`（`Req-ID`）
- gorilla/mux v1.8.0 `mux.go` `Router.Match`（中间件只对命中路由装配）
- `github.com/rs/cors@v1.8.0` `cors.go` `ServeHTTP`（预检 `handlePreflight` + `204` 短路）
- 既有先例：`endpoints/router.go:73-81`（静态分支手工套链）
- 相关修复：[issue #297](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/297)（405 `Allow`）