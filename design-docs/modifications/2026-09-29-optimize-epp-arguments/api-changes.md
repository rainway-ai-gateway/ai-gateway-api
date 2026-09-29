# EPP 暴露参数优化：API 接口变更说明

## 1. 变更范围

| 接口类型 | 变更内容 |
|----------|----------|
| OpenAPI | 既有 `/clusters` 的 `epp_config` 做**字段级变更**：`scheduling_profile` → `load_profile`、`cache_affinity` → `affinity`（重命名，旧字段保留 deprecated 别名），新增 `waiting_queue_max` / `running_requests_max` / `fallback_on_empty` / `metrics_staleness_threshold_ms`。**无新增端点、无数据库变更** |
| InnerAPI | `/configs/epp_data/config` **结构不变**（仍为 `map[cluster名]EndpointPickerConfig` + `assignment`），但 `epp_config` 段的**编译产物内容**变更：新增 `saturation-detector`、`flowControl.saturationDetector`、`util-filter` 新条件与 `fallbackOnEmpty`，scorer 权重随 `load_profile` / `affinity` 调整 |
| 兼容性 | 旧字段 `scheduling_profile` / `cache_affinity` 保留为 deprecated 别名（可写可读）；旧枚举值 `latency-first` / `throughput-first` 自动映射为新值；存量 JSON 无需迁移即可继续使用。**旧字段去除计划：兼容期后另发文，本期不移除** |

**不涉及**：`balance_mode`、`/epp-pool`、`/epp-assignments`、`assignment` 段、server_data_conf / `EPPAddr`、数据库 DDL（`clusters.epp_config` 列复用）。

---

## 2. OpenAPI 变更：`/clusters` 的 `epp_config`

`POST /clusters`、集群更新接口（`PATCH /clusters/{cluster_name}`）请求体与 GET/List 响应中的 `epp_config` 段字段调整如下；创建/更新的条件必填规则、休眠语义、存储语义均**沿用**现有约定（见 `clusters.md` §1「表：EPP调度配置」及约束、`../2026-09-08-epp-scheduling-integration/api-changes.md` §3.2）。

### 2.1 字段变更清单

| 变更 | 字段 | 变更前 | 变更后 | 兼容 / 迁移 |
|------|------|--------|--------|-------------|
| 重命名 | `scheduling_profile` → **`load_profile`** | string，`latency-first` \| `balanced` \| `throughput-first`，默认 `balanced`；语义=调度档位 | string，`queue-first` \| `balanced` \| `kv-first`，默认 `balanced`；语义=**实例负载画像** | 旧字段 `scheduling_profile` 保留为 deprecated 别名；旧值 `latency-first` → `queue-first`、`throughput-first` → `kv-first`（**自动映射**） |
| 取值重命名 | `load_profile` 枚举 | `latency-first`、`throughput-first` | `queue-first`（偏向排队短，通常更低延迟）、`kv-first`（偏向 KV 空闲，通常更高吞吐） | `balanced` 不变 |
| 重命名 | `cache_affinity` → **`affinity`** | string，`low` \| `medium` \| `high`，默认 `medium`；语义=覆盖 (kv, queue) scorer 权重 | string，`off` \| `low` \| `medium` \| `high`，默认 `medium`；语义=**亲和强度**（作用于 `prefix-cache-scorer` / `session-affinity-scorer`），`off` = 不注入亲和 scorer | 旧字段 `cache_affinity` 保留为 deprecated 别名，按 `low` → `queue-first`、`medium` → `balanced`、`high` → `kv-first` 映射为 **`load_profile` 覆盖** |
| 新增 | **`waiting_queue_max`** | — | number，默认 `0`（不启用），`>= 0`；后端排队长度超过该值即不派新请求 | 新增准入过滤维度；`0` = 不按排队过滤 |
| 新增 | **`running_requests_max`** | — | number，默认 `0`（不启用），`>= 0`；后端运行中请求数超过该值即不派新请求 | 新增准入过滤维度；`0` = 不按并发过滤 |
| 新增 | **`fallback_on_empty`** | — | bool，默认 `false`；所有后端被过滤时 `false` = 无后端可选/不派发，`true` = 回退派给"最不差"的后端 | 透传 `utilization-filter.parameters.fallbackOnEmpty` |
| 新增 | **`metrics_staleness_threshold_ms`** | — | int，默认 `200`，`> 0`；后端指标超过该时长未更新即从候选剔除（坏后端剔除） | 透传 detector 的 `metricsStalenessThreshold`；抓取周期约 50ms，`200` ≈ 连续 4 次抓取失败 |
| 语义补充（取值不变） | `kv_cache_utilization_max` | float，`(0, 1]`，默认 `0.9`；端点过滤阈值 | 取值不变 | 现**同时**驱动 `utilization-filter` 条件与 detector 的 `kvCacheUtilThreshold`（**单一阈值来源**） |
| 语义明确（取值不变） | `flow_control.no_endpoint_queue_ttl` | int 秒，缺省跟随 `queue_ttl` | 不变 | 缺省时编译**显式展开**为 `queue_ttl` 的秒数 |
| 校验收紧 | `flow_control.enable_eviction` | bool，默认 `false` | 保留字段（兼容旧配置），建议仅允许 `false` | 当前 EPP 未接线，编译不下发；`true` 建议拒绝（422） |

> 未列出的字段（`prefix_cache_affinity`、`session_affinity_enabled`、`session_affinity_header`、`flow_control.max_requests` / `queue_ttl`）**语义与取值不变**。

### 2.2 合法性条件与约束（字段级校验，api 侧强制）

| 字段 | 合法性条件 |
|------|------------|
| `load_profile` | 枚举 `queue-first` / `balanced` / `kv-first` |
| `affinity` | 枚举 `off` / `low` / `medium` / `high` |
| `prefix_cache_affinity` | bool（不变） |
| `session_affinity_enabled` | bool（不变） |
| `session_affinity_header` | `session_affinity_enabled=true` 时必填（非空合法 HTTP header 名）；`false`/缺省时可保留（格式校验照常，不生效） |
| `kv_cache_utilization_max` | `(0, 1]` |
| `waiting_queue_max` | `>= 0`（`0` = 不启用） |
| `running_requests_max` | `>= 0`（`0` = 不启用） |
| `fallback_on_empty` | bool |
| `metrics_staleness_threshold_ms` | `> 0` |
| `flow_control.max_requests` | `> 0` 或 `-1`（不变） |
| `flow_control.queue_ttl` / `no_endpoint_queue_ttl` | `>= 0` 的整数（不变） |
| `flow_control.enable_eviction` | 建议仅允许 `false`（`true` 拒绝） |

> 校验时机不变：`epp_config` **非空即须通过字段校验，与 `balance_mode` 无关**（含 `WRR` 休眠保留与 `EPP → WRR` 随带场景）。

### 2.3 数据模型示例变更

`clusters.md` §1「数据模型示例（EPP 模式集群）」与 §2.1「HTTP BODY参数示例（EPP 模式集群）」中的 `epp_config` 需同步为：

```jsonc
// 变更前
"epp_config": {
    "scheduling_profile": "balanced",
    "cache_affinity": "medium",
    "prefix_cache_affinity": true,
    "session_affinity_enabled": true,
    "session_affinity_header": "x-session-id",
    "kv_cache_utilization_max": 0.9,
    "flow_control": {
        "max_requests": 1000,
        "queue_ttl": 30,
        "no_endpoint_queue_ttl": 600,
        "enable_eviction": false
    }
}

// 变更后
"epp_config": {
    "load_profile": "balanced",              // 原 scheduling_profile
    "affinity": "medium",                    // 原 cache_affinity
    "prefix_cache_affinity": true,
    "session_affinity_enabled": true,
    "session_affinity_header": "x-session-id",
    "kv_cache_utilization_max": 0.9,
    "waiting_queue_max": 0,                  // 新增
    "running_requests_max": 0,               // 新增
    "fallback_on_empty": false,              // 新增
    "metrics_staleness_threshold_ms": 200,   // 新增
    "flow_control": {
        "max_requests": 1000,
        "queue_ttl": 30,
        "no_endpoint_queue_ttl": 600,
        "enable_eviction": false
    }
}
```

### 2.4 兼容与迁移

- **旧字段别名**：`scheduling_profile`、`cache_affinity` 在结构体中保留（不删除），`ParseEppConfig` 仍严格解析；写入旧字段可正常存储与回读。
- **取值优先级**（`load_profile` 解析）：
  1. 显式 `load_profile`（新字段）；
  2. `scheduling_profile`（旧字段，旧值自动映射 `latency-first`→`queue-first`、`throughput-first`→`kv-first`）；
  3. `cache_affinity`（旧字段，`low`→`queue-first`、`medium`→`balanced`、`high`→`kv-first` 作为画像覆盖）；
  4. 默认 `balanced`。
- **`affinity` 与旧 `cache_affinity` 无关**：`affinity` 是新增的亲和强度字段，缺省为 `medium`；不会从旧 `cache_affinity` 推导（旧字段仅参与 `load_profile` 解析）。
- **存储不变**：`epp_config` 保留用户原始 JSON——未显式携带的字段不落盘、GET 回读与写入一致；默认值只体现在导出编译时。
- **迁移建议**：新增集群一律使用新字段；存量集群可在下次编辑时重写为新字段。旧字段去除计划兼容期后单独立项，本期不移除，避免破坏自动化脚本。

---

## 3. InnerAPI 变更：`epp_data` 的 `epp_config` 段

### 3.1 结构不变声明

`GET /inner-api/v1/configs/epp_data/config` 的返回结构**不变**：

- `Config.epp_config` 仍为 `map[cluster名]EndpointPickerConfig`（key 为集群名，范围仍为全部 `balance_mode=EPP` 的 cluster）；
- `Version` 增量同步机制不变（内容变化由 MD5 签名自然 bump，未变化返回 `Data: null`）；
- `Config.assignment` 段**不变**（全量视图，`primary` / `standby`）。

变化仅在 `epp_config` 段每个 cluster 的**编译产物内容**（见 3.2）。

### 3.2 编译产物变更清单

| 产物 | 变更 | 来源（编译规则） |
|------|------|------------------|
| `plugins[]` 新增 `saturation-detector`（type `utilization-detector`） | **始终下发**（即使用户未配 `flow_control`） | `compileSaturationDetectorPlugin` |
| └ `saturation-detector.kvCacheUtilThreshold` | 与 `kv_cache_utilization_max` 同源 | 单一阈值来源 |
| └ `saturation-detector.queueDepthThreshold` | 取 `waiting_queue_max`；未启用时默认 `5` | |
| └ `saturation-detector.metricsStalenessThreshold` | 取 `metrics_staleness_threshold_ms`（序列化为 `<n>ms`） | 坏后端剔除 |
| └ `saturation-detector.stalenessPolicy` / `headroom` | 固定 `"ignore"` / `0.0`（本期不暴露） | 防"监控抖动即全停" |
| `flowControl` 新增 `saturationDetector.pluginRef` | 恒为 `"saturation-detector"` | 过滤依赖该 detector |
| `util-filter.parameters.fallbackOnEmpty` | 新增，取 `fallback_on_empty` | |
| `util-filter.parameters.conditions` | 可新增 `waiting-queue`（`waiting_queue_max > 0`）、`running-requests`（`running_requests_max > 0`）条件；`kv-cache-utilization` 条件始终存在 | |
| `schedulingProfiles[0].plugins` scorer 权重 | `kv-scorer` / `queue-scorer` 权重随 `load_profile`；`prefix-scorer` / `session-scorer` 权重随 `affinity`（`affinity = off` 时不注入亲和 scorer） | |
| `flowControl` 段 | **始终下发**（含 `saturationDetector` 与 band0）；`featureGates: ["flowControl"]` 仅在存在 `flow_control` 参数时追加 | |
| `flowControl.noEndpointRequestTTL` | `no_endpoint_queue_ttl` 缺省时**显式展开为 `queue_ttl`** | |
| `flowControl.enableEviction` | **不再下发**（恒缺省 false） | 当前 EPP 未接线 |

`load_profile` / `affinity` 权重映射：

| `load_profile` | (kv-scorer, queue-scorer) 权重 |
|---|---|
| `queue-first` | (0.2, 1.0) |
| `balanced` | (0.6, 0.6) |
| `kv-first` | (1.0, 0.2) |

| `affinity` | 亲和 scorer（prefix / session）权重 |
|---|---|
| `off` | 0（**不注入**亲和 scorer） |
| `low` | 0.3 |
| `medium` | 0.6 |
| `high` | 1.0 |

### 3.3 变更后编译产物示例

以 `load_profile=balanced`、`affinity=medium`、`prefix_cache_affinity=true`、其余为默认（**未配 `flow_control`**）为例：

```jsonc
{
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
        { "name": "default", "plugins": [
            { "pluginRef": "util-filter" },
            { "pluginRef": "kv-scorer",     "weight": 0.6 },
            { "pluginRef": "queue-scorer",  "weight": 0.6 },
            { "pluginRef": "prefix-scorer", "weight": 0.6 },
            { "pluginRef": "max-score" }
        ] }
    ],
    "dataLayer": { "discovery": { "endpoints": { "pluginRef": "ep-discover" } } },
    "flowControl": {
        "saturationDetector": { "pluginRef": "saturation-detector" },
        "priorityBands": [ { "priority": 0, "maxRequests": "10000", "maxBytes": "5Gi" } ]
    },
    "requestHandler": { "parsers": [ { "pluginRef": "openai-parser" } ] }
}
```

> 说明：
> - 未配 `flow_control` 时 `featureGates` 缺省，但 `flowControl` 段仍下发（承载 `saturationDetector` 与 band0）。
> - `saturation-detector` 只出现在 `plugins[]` 与 `flowControl.saturationDetector`，**不写进 `schedulingProfiles[].plugins`**——llm-d 会自动把它作为过滤器生效（含坏后端剔除）。
> - 未加权的插件（`util-filter`、`max-score`）序列化为 `"weight": null`。

### 3.4 契约影响

| 项目 | 影响 |
|------|------|
| `Config.epp_config` 结构 | **不变** |
| `Version` / 增量同步 | `epp_config` 内容变化后由 MD5 签名自然 bump，下一次导出带出新产物；旧版本对比会出现内容差异（属预期） |
| `assignment` 段 | **不变** |
| EPP 侧兼容 | 新增 `saturation-detector` 为 llm-d 内建 filter，向后兼容；但 EPP 侧需能消费 `flowControl.saturationDetector`（否则应确认忽略该字段不影响启动） |
| 存量 cluster 产物变化 | 已有 EPP cluster 的编译产物会自动新增 detector / `flowControl.saturationDetector`、`prefix-scorer` 权重由 1.0 变为 `affinity` 默认 `medium` = 0.6；这是**行为变更**，需在发布说明中明示 |

---

## 4. 文档变更

| 位置 | 变更 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/clusters.md` | §1「表：EPP调度配置（`epp_config`）」替换 `scheduling_profile` / `cache_affinity` 行、新增 4 个字段行；§1「数据模型示例（EPP 模式集群）」、§1 约束、§2.1 输入参数表 / HTTP BODY 示例（EPP 模式）、§2.4 更新接口说明同步 |
| `design-docs/api-define/InnerAPI接口定义/epp-data.md` | §3.1 返回示例补 `saturation-detector` / `queue-scorer` / `openai-parser` / `flowControl.saturationDetector`；§3.2 说明补充"detector 始终下发"与"`epp_config` 段结构不变、内容随简化配置编译" |
| 本目录 | 新增 `change-summary.md`、`api-changes.md`、`design-changes.md` |

---

## 5. 边界语义

| 场景 | 行为 |
|------|------|
| 仅写旧字段 `scheduling_profile` | 正常解析；`scheduling_profile=latency-first` → 等价 `load_profile=queue-first`；`throughput-first` → `kv-first`；`balanced` 不变 |
| 仅写旧字段 `cache_affinity` | 作为 `load_profile` 覆盖：`low`→`queue-first`、`medium`→`balanced`、`high`→`kv-first`；不推导新 `affinity`（新 `affinity` 走默认 `medium`） |
| 同时写新旧字段 | 新字段优先（`load_profile` 覆盖 `scheduling_profile` / `cache_affinity`） |
| `affinity=off` | 不注入 `prefix-cache-scorer` / `session-affinity-scorer`；即使 `prefix_cache_affinity=true` / `session_affinity_enabled=true` 也不注入（亲和整体关闭） |
| `waiting_queue_max=0` / `running_requests_max=0` | 不启用对应过滤维度（非"上限 0"）；detector 的 `queueDepthThreshold` 走默认 `5` |
| `waiting_queue_max>0` | 生成 `util-filter` 的 `waiting-queue` 条件，并以同值作为 detector 的 `queueDepthThreshold` |
| 所有后端被过滤且 `fallback_on_empty=false` | 可能出现"无后端可选"，请求不被派发；设 `true` 则回退派给"最不差"的后端 |
| `metrics_staleness_threshold_ms` 超时 | 该后端从调度候选剔除，直到指标恢复更新；调小更快摘除（抗抖动弱），调大更容忍抖动（故障后端存活更久） |
| `flow_control.enable_eviction=true` | 建议拒绝（422）；即使接受也**不下发**到 EPP（恒 false） |
| 未配 `flow_control` | 仍下发 `flowControl` 段（含 `saturationDetector` + band0），但不追加 `featureGates: ["flowControl"]`（不排队、不背压，仅过滤） |