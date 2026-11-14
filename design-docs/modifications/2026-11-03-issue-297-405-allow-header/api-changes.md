# 接口变更说明：#297 405 响应新增 `Allow` 头

## 1. 结论

- **无接口新增/删除，无请求字段与响应体结构变化**；变更只发生在 405 响应**头部**；
- 所有 OpenAPI / InnerAPI 路由的 405 响应将携带 `Allow`，取值为该路径实际支持的方法（逗号分隔、顺序稳定）；
- 404 / 422 / 500 等其它响应不新增该头（不扩大化）；
- `00-common.md` 的 ErrNum 说明补 405 语义（此前只列了 404/409，未列 405）。

## 2. 行为对照

| 请求 | 变更前 | 变更后 |
|---|---|---|
| `POST /open-api/v1/route-tables`（仅 GET） | `405`，无 `Allow` | `405`，`Allow: GET` |
| `OPTIONS /open-api/v1/route-tables`（普通 OPTIONS） | `405`，无 `Allow` | `405`，`Allow: GET` |
| `HEAD /open-api/v1/route-tables` | `405`，无 `Allow` | `405`，`Allow: GET`（不声明 HEAD，因 mux 不隐式支持 HEAD） |
| `PUT` 到「同时注册 GET+POST」的路径 | `405`，无 `Allow` | `405`，`Allow: GET, POST` |
| `DELETE /inner-api/v1/configs/ai-route`（仅 GET） | `405`，无 `Allow` | `405`，`Allow: GET` |
| `GET` 已注册路由 | `200` | `200`，无 `Allow`（不变） |
| 未知路径 | `404` JSON | `404` JSON，无 `Allow`（不变） |

响应体保持原样：

```json
{"ErrNum":405,"ErrMsg":"Method Not Allowed"}
```

## 3. 响应头约定

| 头 | 何时出现 | 取值 | 说明 |
|---|---|---|---|
| `Allow` | 仅 405 且能确定方法集合时 | `GET` / `GET, POST` / `GET, PATCH, DELETE` 等 | 逗号 + 单空格分隔；顺序固定 `GET, HEAD, OPTIONS, POST, PUT, PATCH, DELETE` |
| `Allow` | 理论上 405 但探测不出方法时 | 不出现 | 避免写空头（实践中不应发生） |

`Allow` 的值只包含**当前服务端真实注册**的方法；不按 `GET ⇒ HEAD/OPTIONS` 推断，
因此客户端可据此做方法协商与降级重试。

## 4. 兼容性

| 方面 | 评估 |
|---|---|
| 现有客户端 | 只新增响应头，HTTP 语义向后兼容；不依赖该头的客户端行为不变 |
| 错误码/文案 | 不变（仍 `ErrNum=405`、`ErrMsg="Method Not Allowed"`） |
| 缓存/代理 | 405 不携带 `Cache-Control`，本改动不引入缓存语义变化 |
| CORS | 普通 OPTIONS 仍为 405；预检（带 `Access-Control-Request-Method`）由 CORS 中间件处理，行为不变 |
| 数据面 | 无影响，纯控制面协议头 |

## 5. 文档修改点

| 文件 | 位置 | 内容 |
|---|---|---|
| `design-docs/api-define/OpenAPI接口定义/00-common.md` | ErrNum 列表（`404` 与 `409` 之间） | 新增 `405：请求方法不被目标路由支持（响应头 Allow 列出该资源支持的方法）` |
| 同上 | 「Method约定」小节后 | 新增说明：不支持的 method 返回 405 且带 `Allow`；普通 OPTIONS 亦返回 405 + `Allow`，预检由 CORS 中间件处理 |
| `design-docs/api-define/InnerAPI接口定义/01-common.md` | 响应约定 | 可选：补 405 说明（InnerAPI 与 OpenAPI 共用同一 405 handler） |

## 6. 调用方建议

1. 收到 405 时优先读 `Allow` 判断可用方法，而不是硬编码 API 文档；
2. 需要判断「资源是否存在」请用 `ErrNum=404`，405 只表示路径存在而方法不支持；
3. `Allow` 不含 `OPTIONS` 时表示服务端未开放 OPTIONS 探测，不要据此认为接口不可用。