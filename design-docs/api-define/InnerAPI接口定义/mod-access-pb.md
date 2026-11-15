# mod-access-pb 接口

## 1. 接口信息

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 导出访问日志动态配置（`mod_access_pb.data`） | 供 conf-agent 拉取并写入 BFE conf 目录，触发 `mod_access_pb3` 模块热加载（当前内容：请求/响应头采集白名单，见 OpenAPI [access-log-headers](../OpenAPI接口定义/access-log.md)） |
| 端点 | `/configs/mod-access-pb` | - |
| Method | GET | - |
| 鉴权 | `FeatureRoute + ActionExport` | 与 `/configs/ai-route` 一致 |

## 2. 请求参数

**Query 参数**

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| version | string | 否 | 上次返回的版本号，用于增量同步 | 可选；无强制格式/长度校验；为空或未传时按首次拉取处理 |
| bfe_cluster | string | 否 | BFE 集群标识（conf-agent 传入，当前导出内容全局共享，仅记录） | 可选 |

**请求示例**

```shell
curl -X GET "http://api-server:port/inner-api/v1/configs/mod-access-pb?version=00010101000000" \
  -H "Authorization:Token TOKEN_STRING"
```

## 3. 返回数据结构

### 3.1 顶层结构

与 BFE 动态配置文件 `mod_access_pb.data` 格式**逐字一致**（多段式 JSON，当前仅 `access_header` 一个段）：

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "Version": "20261115101530",
        "access_header": {
            "req_headers": ["Business_type", "X-Request-Id"],
            "res_headers": ["X-Trace-Id"]
        }
    },
    "WorkMode": "ModeNormal"
}
```

**字段说明**

| 字段 | 类型 | 说明 |
|------|------|------|
| `Data` | object/null | 数据文件内容；**版本未变时为 `null`**（conf-agent `NormalFileTask` 跳过不写文件、不触发 reload） |
| `Data.Version` | string | 配置版本：由数据签名机制生成（内容变化 → 新 Version 落库并返回），conf-agent 以文件内 `Version` 做本地比对 |
| `Data.access_header` | object | Header 白名单段；BFE 侧段缺失 = 空白名单（不采集） |
| `Data.access_header.req_headers` | array[string] | 请求头白名单（规范化形式，可为空数组） |
| `Data.access_header.res_headers` | array[string] | 响应头白名单（规范化形式，可为空数组） |

> 控制面未配置过（表为空）时导出空白名单 `{req_headers: [], res_headers: []}` + 签名 Version：BFE 按缺省运行，语义闭环。`Version` 由 `versionControlManager.ExportConfig` 生成，无人工维护负担。

## 4. 下发与热加载链路

```
ai-gateway-api 本端点（版本签名，未变返回 null）
  → conf-agent NormalFileTask 轮询（?version=<本地文件 Version>）
  → 写入 <BFEConfDir>/mod_access_pb3/mod_access_pb.data，切换符号链接
  → 触发 BFE /reload/mod_access_pb3（WebHandleReload，handler 名 = 模块名）
  → BFE 锁外校验、锁内整份原子替换；校验失败保留旧配置并返回 error（HTTP 200 + {"error":...} body）
```

conf-agent 侧配置（代码零改动，仅新增 reloader 段）：

```toml
[Reloaders.mod_access_pb3]
BFEReloadAPI    = "/reload/mod_access_pb3"
ReloadFile      = "mod_access_pb.data"
CopyFiles       = ["mod_access_pb.data", "mod_access_pb3.conf"]
[[Reloaders.mod_access_pb3.NormalFileTasks]]
ConfAPI         = "/inner-api/v1/configs/mod-access-pb"
ConfFileName    = "mod_access_pb.data"
```

## 5. 兼容性与边界

- 新端点，无既有接口变更；命名遵循现有 kebab-case 段（`ai-route`、`mod-api-key`、`traffic-mirror-rule`）；
- 导出 JSON **不携带未知顶层 key**：BFE 对未知段忽略 + Warn（前向兼容），未来新段（采样率、按 product 差异化等）由数据面/控制面同步演进；
- BFE 版本较旧时（无 `mod_access_pb.data` 加载能力）：该端点不下发即可，无交叉影响——BFE `DataConfPath` 为可选配置。
