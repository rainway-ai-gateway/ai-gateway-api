# /access-log

访问日志 Header 白名单（配合 BFE `mod_access_pb3` 模块）：按白名单采集指定的请求头/响应头，写入访问日志 proto 字段 `req_headers`（55）/`res_headers`（158），供下游 Doris 按 Header 维度做流量分析与问题排查（bfenetworks/bfe#1506）。

单例资源（全局一份，非集合）：不提供 `/{id}` 操作；`PUT` 为整组全量替换语义。

## 1. 数据模型

```json
{
  "req_headers": ["business_type", "x-request-id"],
  "res_headers": ["x-trace-id"]
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|------------|
| `req_headers` | array[string] | 需要采集的请求头白名单；写入访问日志 `req_headers` 字段 | 合法 HTTP header 名称 | 非必填；缺省/`null` 按 `[]` 处理（不采集任何请求头）；元素 trim 后非空、长度 ≤128 字节、RFC 7230 token 字符、归一后不得重复 |
| `res_headers` | array[string] | 需要采集的响应头白名单；写入访问日志 `res_headers` 字段 | 合法 HTTP header 名称 | 同上 |

**约束**

- **大小写不敏感**：元素在服务端归一为 HTTP 规范化形式（`http.CanonicalHeaderKey`，如 `business_type` → `Business_type`、`x-request-id` → `X-Request-Id`）存储与下发；BFE 侧运行时直接 map 查找（零开销），写入 proto 的 `key` 亦为规范化形式（下游 Doris 聚合键稳定）。`x-id` 与 `X-ID` 视为同一名称（归一后重复 → 422）。
- **敏感头硬拒绝**（大小写不敏感）：`Authorization`、`Proxy-Authorization` 出现在任一列表 → 422 参数错误，不落库。敏感头管控**只在控制面做**——BFE 数据面加载侧不限制白名单内容（避免限制逻辑分散两侧、误伤手工维护数据文件的部署），原始 API Key 由 BFE 运行时 `maskSensitiveCredentials` 统一掩码（bfenetworks/bfe#1357）。
- `Cookie`/`Set-Cookie`：允许配置（业务确有排查需要），服务端记录 Warn 级审计日志提示会话凭证风险。
- 格式校验（非空/长度/token 字符/归一去重）与 BFE `mod_access_pb_data.go` 的 `normalizeHeaderNames` 逐条对齐，防"控制面放行、BFE reload 失败"的配置穿透。
- 采集语义（由 BFE 数据面实现，控制面不感知）：多值头按 `,` 拼接为一个值；value 超过 2048 字节截断（对齐 Doris 列宽）；白名单缺省为空 = 不采集（存量部署行为不变）。

## 2. 接口列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/open-api/v1/access-log/config` | 返回当前访问日志配置（未配置过返回各段缺省值，当前为空白名单） |
| PUT | `/open-api/v1/access-log/config` | 整体更新访问日志配置（全量替换；当前为请求/响应头白名单） |

鉴权：`FeatureAccessLog` + `ActionRead`（GET）/ `ActionUpdate`（PUT）；写操作记 `operation_log`。

## 3. 请求/响应示例

```shell
# 更新
curl -X PUT "http://api-server:port/open-api/v1/access-log/config" \
  -H "Authorization:Token TOKEN_STRING" -H "Content-Type: application/json" \
  -d '{"req_headers":["business_type","x-request-id"],"res_headers":["x-trace-id"]}'

# 回显
curl "http://api-server:port/open-api/v1/access-log/config" \
  -H "Authorization:Token TOKEN_STRING"
```

响应（GET / PUT 同构）：

```json
{
  "req_headers": ["Business_type", "X-Request-Id"],
  "res_headers": ["X-Trace-Id"]
}
```

> 响应中元素为**归一后的规范化形式**（服务端存储形态），与请求允许任意大小写不矛盾。

## 4. 错误码

| 场景 | HTTP 状态 | 说明 |
|------|-----------|------|
| 元素为空/全空白 | 422 | 参数错误，指明具体元素 |
| 元素长度 >128 字节 | 422 | 同上 |
| 元素含非法字符（非 RFC 7230 token 字符） | 422 | 同上 |
| 归一后重复 | 422 | 指明重复名称 |
| 含 `Authorization`/`Proxy-Authorization` | 422 | 敏感头管控，拒绝写入 |

## 5. 兼容性

- 新端点，无既有接口变更；
- 未配置时 GET 返回 `{"req_headers":[],"res_headers":[]}`；导出到 BFE 的 `mod_access_pb.data` 为空白名单，BFE 按缺省运行（不采集），语义闭环；
- 与 InnerAPI 导出端点 `/configs/mod-access-pb` 配套：白名单变更经数据签名版本机制下发，conf-agent 拉取后触发 BFE `/reload/mod_access_pb3` 热生效（详见 [InnerAPI：mod-access-pb 接口](../InnerAPI接口定义/mod-access-pb.md)）。
