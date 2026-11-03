# Issue #297：405 Method Not Allowed 响应缺少 `Allow` 头

## 1. 问题来源

[rainway-ai-gateway/ai-gateway-api#297](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/297)

> 对已注册路由发送不支持的 method（如 `POST /open-api/v1/route-tables`，该路由仅注册 GET）时，
> 服务端返回 `405` / `ErrNum=405`，但响应缺少 `Allow` 头；对同一路由发送普通 `OPTIONS` 同样 405 且无 `Allow`。
> RFC 9110 §15.5.6 要求 405 生成 `Allow`，测试标准 §5.5.2 第 2 条明确「缺 `Allow` 应记录为 `FAIL_PRODUCT`」。

## 2. 根因

`endpoints/router.go:65-69` 为所有路由注册了统一的 405 handler：

```go
router.MethodNotAllowedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	res := &xreq.Result{Code: 405, ErrMsg: "Method Not Allowed"}
	xreq.Render(w, r, res)   // 只写 Content-Type / Req-ID / body，无 Allow
})
```

该 handler 被 gorilla/mux 在「路径匹配但方法不匹配」时调用（`Router.Match` 检测到
`match.MatchErr == ErrMethodMismatch` 时优先使用 `MethodNotAllowedHandler`），因此 **所有**
OpenAPI / InnerAPI 路由的 405 都走这里，而它从未设置 `Allow`。

**对 issue 表述的一处修正**：issue 称「mux 默认 405 处理会设置 `Allow`，被自定义 handler 覆盖」。
核对 mux v1.8.0 源码（`mux.go` 的 `methodNotAllowed`）后确认：**mux 的默认 405 只 `WriteHeader(405)`，
同样不设置 `Allow`**，也没有任何 `Allow` 相关逻辑。所以这不是「覆盖了默认行为」，而是
「依赖的框架同样不提供该能力」——结论不变：`Allow` 必须由我们自己计算并写入。

## 3. 实测证据（2026-11-03，HEAD `8ad4d62`，SQLite，`testutil` 起的真实服务）

| 请求 | 状态 | `Allow` | `Content-Type` |
|---|---|---|---|
| `POST /open-api/v1/route-tables`（仅注册 GET） | 405 | 空 | `application/json` |
| `OPTIONS /open-api/v1/route-tables` | 405 | 空 | `application/json` |
| `GET /open-api/v1/route-tables` | 200 | 空 | `application/json` |
| `POST /open-api/v1/nope`（未知路径） | 404 | 空 | `application/json` |
| `DELETE /inner-api/v1/configs/ai-route`（仅注册 GET） | 405 | 空 | `application/json` |

结论：405 分支在 OpenAPI 与 InnerAPI 上均可达、均缺 `Allow`；200/404 相邻行为不受影响。

## 4. 目标

1. 任何 405 响应携带 `Allow`，取值列出该路径**实际**支持的方法（逗号分隔、顺序稳定）；
2. `Allow` 不得虚报：HEAD 未被任何路由注册时不出现（mux 不会为 GET 路由隐式支持 HEAD）；
3. 其它 4xx/5xx（404/422/500 等）不因本改动新增 `Allow`；
4. 405 响应体、状态码、`Content-Type`、`Req-ID` 等现有行为不变；
5. 404（未知路径 JSON）与已注册方法的 200 行为不回归；
6. 配套更新 `00-common.md` 的 ErrNum 说明（405 语义）。

## 5. 方案概览

在 405 handler 内**用同一 router 逐方法探测**，把命中的方法集合写入 `Allow`：

```go
func methodNotAllowedHandler(router *mux.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow := allowedMethods(router, r); len(allow) > 0 {
			w.Header().Set("Allow", strings.Join(allow, ", "))
		}
		res := &xreq.Result{Code: 405, ErrMsg: "Method Not Allowed"}
		xreq.Render(w, r, res)
	})
}

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
		if router.Match(probe, &match) && match.MatchErr == nil && match.Route != nil {
			allowed = append(allowed, method)
		}
	}
	return allowed
}
```

要点：

- 探测走 `Router.Match`，天然穿透 `PathPrefix(...).Subrouter()`（OpenAPI / InnerAPI 都是子路由），
  比遍历 `router.routes` 更正确；
- 必须同时判 `match.MatchErr == nil && match.Route != nil`：因为根 router 设置了
  `MethodNotAllowedHandler` / `NotFoundHandler` 后，`Router.Match` 在方法不匹配或路径未命中时
  会**返回 true** 并设置 `MatchErr`（`ErrMethodMismatch` / `ErrNotFound`），只看返回值会把 405/404 误判为「允许」；
- 方法顺序固定为 `GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE`，便于测试与缓存；
- 计算不出任何方法（理论上 405 不会发生）时不写 `Allow`，避免写空头。

详细设计（含边界、测试、验证）见 [design-changes.md](./design-changes.md)；
接口行为与文档同步见 [api-changes.md](./api-changes.md)。

## 6. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api` |
| 代码 | `endpoints/router.go`（405 handler 改为按 router 计算 `Allow`，新增 `allowedMethods`）；`endpoints/router_test.go`（新增单测） |
| 文档 | `design-docs/api-define/OpenAPI接口定义/00-common.md`（ErrNum 列表补 405 与 `Allow` 约定）；本目录 |
| 测试 | 新增单元测试 + 新增集成用例（405/OPTIONS/HEAD/404/200 五条断言，OpenAPI 与 InnerAPI 各一例） |
| 行为影响 | 仅 405 响应新增响应头；不涉及数据、DB、配置导出、数据面 |
| 兼容性 | 纯增强（新增头部字段），无字段/错误码语义变化 |

## 7. 风险

| 风险 | 评估 | 缓解 |
|------|------|------|
| 探测成本 | 405 属错误路径，单次最多 7 次 `Router.Match`，微秒级 | 无 |
| `Allow` 虚报 HEAD/OPTIONS | 中 | 用同一 router 探测真实可路由性，不按 GET⇒HEAD 推断；单测覆盖 |
| 影响 404/静态分支 | 低 | 只改 405 handler；404 handler 与静态分支不动 |
| 中间件不生效 | 既有行为：405 不走 `router.Use` 链（无访问日志/CORS） | 本 issue 不扩大范围，作为后续观察项记录 |

## 8. 上线前检查清单

1. `go test ./endpoints/ -run TestMethodNotAllowed -count=1` 通过；
2. `AIAPI_FORCE_SQLITE=1 go test -count=1 -run 'TestMethodNotAllowed' ./tests/http_semantics/` 通过；
3. `curl -i -X POST http://<server>/open-api/v1/route-tables` → `405` + `Allow: GET`；
4. `curl -i -X OPTIONS http://<server>/open-api/v1/route-tables` → `405` + `Allow: GET`；
5. `curl -i -X DELETE http://<server>/inner-api/v1/configs/ai-route` → `405` + `Allow: GET`；
6. `curl -i -X POST http://<server>/open-api/v1/nope` → `404`，响应头无 `Allow`；
7. 既有 405 之外的接口用例全量回归（`go test ./...`、SQLite 集成套件）。

## 9. 实施记录（Step 5 / Step 6）

### 9.1 代码改动

| 文件 | 改动 |
|------|------|
| `endpoints/router.go` | 405 handler 改为 `methodNotAllowedHandler(router)`：先按成功探测结果写 `Allow`，再渲染原 JSON 405；新增 `allowedMethods()` 与 `allowProbeMethods` |
| `endpoints/router_test.go`（新增） | 合成路由（含 `/open-api/v1`、`/inner-api/v1` 子路由与根级路由）覆盖 8 个请求场景 + `allowedMethods` 单测 |

### 9.2 文档改动

| 文件 | 改动 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/00-common.md` | ErrNum 列表在 404 与 409 之间补 405（含 `Allow` 说明） |
| `design-docs/api-define/InnerAPI接口定义/01-common.md` | 新增「方法不支持（405）」小节（InnerAPI 与 OpenAPI 共用同一 handler） |
| `design-docs/sys-design/接口层设计文档.md` | 新增 §4.1.6 405 与 `Allow` 头（含探测条件与 mux 语义说明） |
| 本目录 | 三份变更说明 |

### 9.3 测试改动

| 文件 | 内容 |
|------|------|
| `test/integration/tests/http_semantics/method_not_allowed_test.go`（新增包） | HS 场景：OpenAPI / InnerAPI 的 405 带 `Allow`、多方法路径（`/api-keys` → `GET, POST`）、200/404 不带；用 `net/http` 直接读响应头（`testutil.Client` 不回传 header） |

### 9.4 验证结果（2026-11-03，Go 1.25.2 / windows-amd64）

| 验证项 | 结果 |
|---|---|
| `go test ./endpoints/ -run 'TestMethodNotAllowed\|TestAllowedMethods'` | PASS（8 个子场景 + `allowedMethods`） |
| `go test ./...` | 104 个包 `ok`，0 FAIL |
| 集成 `tests/http_semantics`（基线 SQLite） | PASS：`POST`/`OPTIONS` OpenAPI 路由 → 405 + `Allow: GET`；`DELETE` InnerAPI 路由 → 405 + `Allow: GET`；`PATCH /open-api/v1/api-keys`（GET+POST）→ 405 + `Allow: GET, POST`；`GET` → 200 无 `Allow`；未知路径 → 404 无 `Allow` |
| 集成全量 `./tests/...`（基线 SQLite，并行） | 130 个包 `ok`，0 FAIL |
| 修复前对照（同一服务实例，HEAD `8ad4d62`） | `POST`/`OPTIONS`/`DELETE` 均 405 且 `Allow=""`（见 §3 证据表） |

### 9.5 沉淀判断（Step 6）

不新增 `sys-design/details/` 文档：改动是路由层一个 handler 的实现修正（约 50 行），
判据与 mux 语义已完整写进《接口层设计文档》§4.1.6 与 `api-changes.md`；
`details/` 定位是配额同步、导出与版本控制这类复杂机制，再拆一份会造成双份维护。

### 9.6 遗留

1. 405 响应仍不经过 `router.Use` 中间件链（mux 语义：中间件只包裹命中的路由），因此没有访问日志与 CORS 头。
   这是既有行为，本 issue 不扩大范围；如需，可按 `staticHandler` 同样的手工包裹方式处理（另案）；
2. 静态资源分支的 405 由 `net/http` FileServer 产生，不在本 issue 范围，行为未改动；
3. 外部 harness（`CROSS-ROUTE-001` / `CR-1-001` / `CR-1-002` 与 `cross-route-001-report.json`）不在本仓库，
   需由测试方复跑确认 R3/R5 转 `pass`。

## 10. 参考

- [Issue #297](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/297)
- RFC 9110 §15.5.6（405 Method Not Allowed：`Allow` 为必需头部）、§10.2.1（`Allow` 语义）
- `test/docs/api-test-reorg/openapi-interface-testing-standard.md` §5.5.2 第 2/6 条（缺 `Allow` 判 `FAIL_PRODUCT`；OPTIONS 与预检分开验证）
- `endpoints/router.go:65-69`（405 handler）、`lib/xreq/result.go:164-217`（`Render` 写头/写体顺序）
- gorilla/mux v1.8.0：`mux.go` `Router.Match`（`ErrMethodMismatch` 触发 `MethodNotAllowedHandler`）、
  `methodNotAllowed`（默认 handler 不设置 `Allow`）、`middleware.go` `getAllMethodsForRoute`（仅能覆盖直接注册的路由，无法穿透子路由）