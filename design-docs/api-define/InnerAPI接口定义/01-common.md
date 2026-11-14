# 通用说明

## 1. 基本信息

| 项目 | 值 | 说明 |
|------|------|------|
| URL 格式 | `http://api_server:port/inner-api/v1/{endpoint}?{arg=value}` | 例：`http://127.0.0.1:8086/inner-api/v1/configs/mod-api-key` |
| 鉴权方式 | McUserProbe | HTTP Header 中间件鉴权 |
| 版本控制 | 基于 version 参数 | 配置未变化时返回空响应 |

## 2. 返回值格式

所有 Inner-API 的返回值格式：

```json
{
    "ErrNum": 200,
    "Data": {},
    "ErrMsg": "success",
    "WorkMode": "current mode"
}
```

**字段说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| ErrNum | int | 返回码，200 表示成功 |
| Data | object | 返回的数据结构，配置未变化时返回 null |
| ErrMsg | string | 文本消息，成功时为 "success"，失败时为错误信息 |
| WorkMode | string | 控制台工作模式 |

## 3. 方法不支持（405）

InnerAPI 与 OpenAPI 共用同一根路由的 405 处理（`endpoints/router.go`）：

- 路径存在但 method 不支持时返回 `405`，响应头 `Allow` 列出该路径实际支持的方法（逗号 + 空格分隔，顺序固定）；
- 响应体为 `{"ErrNum":405,"ErrMsg":"Method Not Allowed"}`；
- 其它错误（404/422/500 等）不携带 `Allow`。

## 4. 响应头与 CORS

InnerAPI 与 OpenAPI 共用同一套 HTTP 语义（`endpoints/router.go`）：

- `Req-ID`：所有响应（含路由级 404 / 405）都注入，与访问日志中的请求 ID 一致（issue #298）；
- CORS 预检（`OPTIONS` + `Origin` + `Access-Control-Request-Method`）返回 `204` 并带 `Access-Control-Allow-*`；
  普通 `OPTIONS` 按「方法不支持」返回 `405` + `Allow`。

## 5. 版本控制机制

所有导出接口支持 `version` 查询参数：

- **首次拉取**：不传 `version` 或传空字符串，返回完整配置
- **增量拉取**：传入上次返回的 `version`，若配置未变化返回 `Data: null`
- **配置变化**：返回新的配置数据和新的 `version`

---

