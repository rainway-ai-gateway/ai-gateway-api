# epp_data 接口

## 1. 接口信息

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 导出 EPP 配置（epp_config + assignment 全量视图，合并单端点） | 供 EPP 实例拉取调度配置与实例组分配视图；单 topic（`ConfigTopicEppData`），两段配置同一 version 快照 |
| 端点 | `/configs/epp_data/config` | - |
| Method | GET | - |
| 鉴权 | `FeatureRoute + ActionExport` | - |

## 2. 请求参数

**Query 参数**

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| version | string | 否 | 上次返回的版本号，用于增量同步 | 可选；无强制格式/长度校验；为空或未传时按首次拉取处理 |

**请求示例**

```shell
curl -X GET "http://api-server:port/inner-api/v1/configs/epp_data/config?version=00010101000000" \
  -H "Authorization:Token TOKEN_STRING"
```

## 3. 返回数据结构

### 3.1 顶层结构

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "Version": "20260906120000",
        "Config": {
            "epp_config": {
                "cluster-a": {
                    "featureGates": ["flowControl"],
                    "plugins": [
                        { "name": "ep-discover", "type": "cluster-table-discovery", "parameters": { "clusterName": "cluster-a" } },
                        { "name": "util-filter", "type": "utilization-filter",
                          "parameters": { "conditions": [ { "metric": "kv-cache-utilization", "maxValue": 0.9 } ],
                                          "fallbackOnEmpty": false } },
                        { "name": "saturation-detector", "type": "utilization-detector",
                          "parameters": { "kvCacheUtilThreshold": 0.9, "queueDepthThreshold": 5,
                                          "stalenessPolicy": "ignore", "headroom": 0.0,
                                          "metricsStalenessThreshold": "200ms" } },
                        { "name": "kv-scorer", "type": "kv-cache-utilization-scorer", "parameters": {} },
                        { "name": "queue-scorer", "type": "queue-scorer", "parameters": {} },
                        { "name": "prefix-scorer", "type": "prefix-cache-scorer", "parameters": {} },
                        { "name": "max-score", "type": "max-score-picker", "parameters": {} },
                        { "name": "openai-parser", "type": "openai-parser", "parameters": {} }
                    ],
                    "schedulingProfiles": [
                        { "name": "default",
                          "plugins": [
                            { "pluginRef": "util-filter" },
                            { "pluginRef": "kv-scorer", "weight": 0.6 },
                            { "pluginRef": "queue-scorer", "weight": 0.6 },
                            { "pluginRef": "prefix-scorer", "weight": 0.6 },
                            { "pluginRef": "max-score" }
                          ] }
                    ],
                    "dataLayer": { "discovery": { "endpoints": { "pluginRef": "ep-discover" } } },
                    "flowControl": {
                        "defaultRequestTTL": "30s", "noEndpointRequestTTL": "10m0s",
                        "saturationDetector": { "pluginRef": "saturation-detector" },
                        "priorityBands": [ { "priority": 0, "maxRequests": "1000", "maxBytes": "5Gi" } ]
                    },
                    "requestHandler": { "parsers": [ { "pluginRef": "openai-parser" } ] }
                }
            },
            "assignment": {
                "cluster-a": { "primary": "epp-a", "standby": "epp-b" },
                "cluster-b": { "primary": "epp-b", "standby": "epp-a" },
                "cluster-c": { "primary": "epp-c", "standby": null }
            }
        }
    },
    "WorkMode": "ModeNormal"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| Version | string | 配置版本号，格式 `20060102150405` |
| Config | object | EPP 配置，含 `epp_config` 与 `assignment` 两段 |

### 3.2 Config.epp_config 段

`epp_config` 为 `map[cluster名]EndpointPickerConfig`，由 OpenAPI 写入的简化 `epp_config`（见 OpenAPI 接口定义 [clusters.md](../OpenAPI接口定义/clusters.md)）在导出时**确定性编译**而来，出厂即合法。

| 字段 | 类型 | 说明 |
|------|------|------|
| \<cluster名\> | object | 编译后的完整 `EndpointPickerConfig`，key 为集群名称 |

范围为全部 `balance_mode=EPP` 的 cluster。OpenAPI 校验保证 EPP 模式 cluster 的 `epp_config` 必填，因此导出结果中 EPP cluster 必然有配置。

**编译规则**：见 modifications 目录 `2026-09-29-optimize-epp-arguments/api-changes.md` §3.2（EPP 暴露参数优化后的现行规则）；历史规则见 `2026-09-08-epp-scheduling-integration/api-changes.md` §3.2.1。

> **结构稳定**：本次优化**不改变**本段的结构（键名、类型、`map[cluster名]EndpointPickerConfig` 形态）；仅**编译产物内容**发生如下变化：

- `plugins[]` 新增 `saturation-detector`（type `utilization-detector`），**始终下发**（即使用户未配 `flow_control`）；`queue-scorer` / `prefix-scorer` / `openai-parser` 亦为编译模板固定注入项。
- `util-filter.parameters` 新增 `fallbackOnEmpty`；`conditions` 在简化参数 `waiting_queue_max > 0` / `running_requests_max > 0` 时追加 `waiting-queue` / `running-requests` 条件（`kv-cache-utilization` 条件始终存在）。
- `schedulingProfiles[0].plugins` 中 `kv-scorer` / `queue-scorer` 权重随 `load_profile`（`queue-first` = (0.2, 1.0)、`balanced` = (0.6, 0.6)、`kv-first` = (1.0, 0.2)）；`prefix-scorer` / `session-scorer` 权重随 `affinity`（`off` / `low` / `medium` / `high` = 0 / 0.3 / 0.6 / 1.0，`off` 时不注入亲和 scorer）。
- `flowControl` 段**始终下发**（含 `saturationDetector.pluginRef` 与 priority band 0）；`featureGates: ["flowControl"]` 仅在简化配置含 `flow_control` 时追加；`enableEviction` 不再下发（恒缺省 `false`）。
- `no_endpoint_queue_ttl` 缺省时，`noEndpointRequestTTL` **显式展开为 `queue_ttl` 的值**；秒数统一转 Go duration（如 `30` → `"30s"`、`600` → `"10m0s"`）。
- `saturation-detector` 只出现在 `plugins[]` 与 `flowControl.saturationDetector`，**不写入 `schedulingProfiles[].plugins`**——llm-d 会自动将其作为过滤器生效（含"坏后端剔除"）。

> **兼容性**：`saturation-detector` 为 llm-d 内建 filter，向后兼容；EPP 侧需能消费 `flowControl.saturationDetector`（或确认忽略该字段不影响启动）。`assignment` 段不受本次优化影响。

### 3.3 Config.assignment 段（全量视图）

`assignment` 为 `map[cluster名]{primary, standby}`，**所有 EPP 实例返回完全相同的内容**（同一 version 快照）。

| 字段 | 类型 | 说明 |
|------|------|------|
| \<cluster名\>.primary | string | 主实例 id，为 `/epp-pool` 中配置的实例 id |
| \<cluster名\>.standby | string \| null | 备实例 id；单实例组（无备）时为 `null` |

**EPP 侧消费方式**：以自身 `-instance-id`（须与 `/epp-pool` 中某实例 id 一致）逐 cluster 匹配——`primary == 本实例 id` → 本实例为该 cluster 的 primary；`standby == 本实例 id` → standby；均未命中 → 跳过该 cluster。全量视图的附带收益：EPP 可获知同组 peer 实例 id（如备 Cell 建联、双活跃自检）。

**异常态**：`epp_config` 中存在但 `assignment` 中无条目的 cluster = 未分配。EPP 侧应本地告警、不为该 cluster 建 cell；api 侧导出 server_data_conf 时该 cluster 降级为 `WRR` 并输出 error 日志（见 [server-data-conf.md](./server-data-conf.md) §3.3）。

## 4. 配置未变化返回示例

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": null,
    "WorkMode": "ModeNormal"
}
```

---
