# 接口变更说明：#298 CORS 预检与路由级 `Req-ID`

## 1. 结论

- **无接口新增/删除，无请求/响应体结构变化**；变更集中在响应头与「预检」这一请求形态的状态码；
- CORS 预检（`OPTIONS` + `Origin` + `Access-Control-Request-Method`）由 `405` 变为 **`204`**，并携带
  `Access-Control-Allow-Origin` / `-Methods` /（按需）`-Headers` / `-Credentials`；
- 路由级 **404 / 405** 响应新增非空 `Req-ID`，并写入访问日志（与响应头 ID 一致）；
- 普通 `OPTIONS`（无 `Access-Control-Request-Method`）仍是 `405` + `Allow`（issue #297），并补上 `Req-ID`；
- 路由级 404/405 仍不执行认证/授权/IP 白名单，不返回 `401/403`。

## 2. 行为对照

| 请求 | 变更前 | 变更后 |
|---|---|---|
| 预检 `OPTIONS /open-api/v1/route-tables`（Origin + ACRM） | `405`，无 `Access-Control-*` | `204` + `Access-Control-Allow-Origin: *` + `Access-Control-Allow-Methods: <method>` + `Access-Control-Allow-Credentials: true` |
| 预检 `OPTIONS /open-api/v1/nope`（Origin + ACRM） | `404`，无 `Access-Control-*` | `204` + 同上（随后真实请求仍为 `404`） |
| 普通 `OPTIONS`（无 ACRM，带 Origin） | `405` + `Allow`，无 `Req-ID` | `405` + `Allow` + `Req-ID` + `Access-Control-Allow-Origin` |
| `GET /open-api/v1/nope`（404，带 Origin） | `404`，无 `Req-ID` | `404` + `Req-ID` + `Access-Control-Allow-Origin` |
| 不支持的方法（405，带 Origin） | `405` + `Allow`，无 `Req-ID` | `405` + `Allow` + `Req-ID` + `Access-Control-Allow-Origin` |
| 正常 `GET`（200） | `200` + `Req-ID` | 不变 |

预检响应示例：

```
HTTP/1.1 204 No Content
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: PATCH
Access-Control-Allow-Credentials: true
Access-Control-Allow-Headers: content-type        # 仅当预检请求带 Access-Control-Request-Headers
Vary: Origin, Access-Control-Request-Method, Access-Control-Request-Headers
```

> 当前依赖 `rs/cors v1.8.0`：`AllowedOrigins: ["*"]` 时一律返回 `Access-Control-Allow-Origin: *`
> （即使同时开启 `AllowCredentials`）。带 credentials 的浏览器请求会因此拒绝该响应——
> 这是既有 CORS 配置与其依赖版本的既有行为，本 issue 不改变；如需回显具体 Origin 需升级
> `rs/cors` 或改为显式白名单（另案）。

## 3. 响应头约定（全接口）

| 头 | 出现条件 | 语义 |
|---|---|---|
| `Req-ID` | 所有经 `xreq.Render` 的响应（含 404/405；不含 Cors 短路的 204 预检） | 请求唯一 ID，与访问日志一致，用于排障关联 |
| `Access-Control-Allow-Origin` | 请求带 `Origin` 时（预检与实测请求均设置） | 当前配置（`AllowedOrigins: *`）下固定为 `*` |
| `Access-Control-Allow-Methods` | 预检响应 | 回显 `Access-Control-Request-Method`（受 `AllowedMethods` 白名单限制） |
| `Access-Control-Allow-Headers` | 预检请求带 `Access-Control-Request-Headers` 时 | 回显允许的请求头 |
| `Access-Control-Allow-Credentials` | 预检与实际请求 | `true`（配置开启） |
| `Vary` | 预检与实际请求 | `Origin, Access-Control-Request-Method, Access-Control-Request-Headers` |
| `Allow` | 405（非预检） | 该路径实际支持的方法（issue #297） |

## 4. 兼容性

| 方面 | 评估 |
|---|---|
| 浏览器 / 跨域控制台 | 修复前预检失败会被浏览器直接阻断；修复后可直接使用（功能恢复） |
| 非浏览器客户端 | 预检状态码 405→204：仅当客户端主动发 `OPTIONS + ACRM` 时才可见；此类请求本就属于 CORS 协议，按 2xx 处理是期望行为 |
| 依赖 `ErrNum` 的脚本 | 预检不再返回 JSON `{"ErrNum":405}`（改为空体 204）；预检不属于业务调用，无 `ErrNum` 语义 |
| 404/405 排障 | 新增 `Req-ID` 可与访问日志/操作日志关联；原有 body/状态码不变 |
| 安全边界 | 路由级 404/405 仍不触发认证/授权/IP 白名单；预检不触达业务逻辑，真实请求仍受 IP 白名单保护 |
| 数据面 | 无影响 |

## 5. 文档修改点

| 文件 | 位置 | 内容 |
|---|---|---|
| `design-docs/api-define/OpenAPI接口定义/00-common.md` | 响应约定小节 | 补「`Req-ID` 对所有错误响应（含 404/405）注入，且与访问日志一致」 |
| 同上 | 新增「CORS 与预检」小节 | 预检请求（`OPTIONS` + `Origin` + `Access-Control-Request-Method`）→ `204` + `Access-Control-Allow-*`；普通 `OPTIONS` → `405` + `Allow`；`AllowedOrigins: *` 时回显 Origin |
| `design-docs/api-define/InnerAPI接口定义/01-common.md` | 返回值格式小节 | 说明 InnerAPI 同样适用上述 `Req-ID` / CORS 约定 |
| `design-docs/sys-design/接口层设计文档.md` | §3 中间件设计 | 明确 `router.Use` 只覆盖命中路由，路由级 404/405 由 `withBaseMiddlewares`（Recovery→Logger→CORS，不含 IPProbe）覆盖 |

## 6. 调用方建议

1. 跨域前端无需再为预检失败做兜底；同源调用不受影响；
2. 排障时以响应头 `Req-ID` 关联服务端访问日志（404/405 现在也有日志条目）；
3. 若需要区分「路径不存在」与「方法不支持」，仍按 `ErrNum=404` / `ErrNum=405`（+ `Allow`）判断，
   不要依赖预检请求的返回。