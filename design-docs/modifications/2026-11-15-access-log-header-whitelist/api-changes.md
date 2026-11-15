# 接口变更说明：访问日志 Header 白名单（`/access-log/config` + `/configs/mod-access-pb`）

## 1. 结论

- **OpenAPI 新增** 2 个端点（1 个资源，GET/PUT 成对，模式同 `ai-cache-rules`）；
- **InnerAPI 新增** 1 个导出端点（conf-agent 拉取通道，模式同 `/configs/ai-route`）；
- 无既有接口的字段/语义变更；无路径或方法变化到存量端点。

## 2. OpenAPI

### 2.1 端点

| 方法 | 路径 | 说明 | 鉴权 |
|------|------|------|------|
| GET | `/open-api/v1/access-log/config` | 返回当前请求/响应头白名单 | `FeatureAccessLog` + `ActionRead` |
| PUT | `/open-api/v1/access-log/config` | 整体更新白名单（全量替换） | `FeatureAccessLog` + `ActionUpdate` |

### 2.2 请求/响应体（GET 响应与 PUT 请求同构）

```json
{
    "req_headers": ["business_type", "x-request-id"],
    "res_headers": ["x-trace-id"]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `req_headers` | []string | N | 需要采集的请求头白名单；缺省/空 = 不采集任何请求头 |
| `res_headers` | []string | N | 需要采集的响应头白名单；缺省/空 = 不采集任何响应头 |

语义要点：

- **大小写不敏感**：名称在服务端归一为 HTTP 规范化形式（`http.CanonicalHeaderKey`）存储与下发；写入访问日志的 `key` 亦为规范化形式（下游 Doris 聚合键稳定）；
- **敏感头硬拒绝**（大小写不敏感）：`Authorization`、`Proxy-Authorization` 出现在任一列表 → 422 参数错误，**不落库**——敏感头管控只在控制面做（BFE 数据面加载侧不限制，由 `maskSensitiveCredentials` 运行时兜底原始 API Key）；
- `Cookie`/`Set-Cookie`：允许配置（业务确有排查需要），服务端记录 Warn 级审计日志提示会话凭证风险；
- 其余校验：元素 trim 后非空、长度 ≤128 字节、RFC 7230 token 字符、归一后去重（重复 → 422）——与 BFE `mod_access_pb_data.go` 的 `normalizeHeaderNames` 逐条对齐，防"控制面放行、BFE reload 失败"的配置穿透。

### 2.3 示例

```bash
# 更新
curl -X PUT "$API/open-api/v1/access-log/config" \
  -H "Authorization: Token $TOKEN" -H "Content-Type: application/json" \
  -d '{"req_headers":["business_type","x-request-id"],"res_headers":["x-trace-id"]}'

# 回显（未配置过则返回空数组）
curl "$API/open-api/v1/access-log/config" -H "Authorization: Token $TOKEN"
# → {"req_headers":[],"res_headers":[]}
```

错误响应沿用既有 422 参数错误结构（`xerror.WrapParamErrorWithMsg`），错误信息指明具体元素与原因。

## 3. InnerAPI

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/inner-api/v1/configs/mod-access-pb?version=<last>&bfe_cluster=<cluster>` | 导出 `mod_access_pb.data` 内容（conf-agent `NormalFileTask` 拉取） |

响应（`Data` 字段，即数据文件内容；**版本未变时 `Data=null`**）：

```json
{
    "ErrNum": 200,
    "Data": {
        "Version": "20261115101530",
        "access_header": {
            "req_headers": ["business_type", "x-request-id"],
            "res_headers": ["x-trace-id"]
        }
    }
}
```

- `Version` 由 `versionControlManager.ExportConfig` 的数据签名机制生成：内容变化 → 新 Version 落库并返回；内容未变 → `Data=null`（conf-agent 跳过，既有语义，框架零改动）；
- 导出 JSON 的顶层 key 与 BFE 加载器契约**逐字一致**（`Version` / `access_header` / `req_headers` / `res_headers`），未知 key 不出现（BFE 侧对未知段忽略 + Warn，前向兼容由数据面承担）；
- 命名遵循现有 kebab-case 段（`ai-route`、`mod-api-key`、`traffic-mirror-rule`）。
