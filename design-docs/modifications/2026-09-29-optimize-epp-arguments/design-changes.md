# EPP 暴露参数优化：ai-gateway-api 控制面设计变更说明

> 本文档只描述 `ai-gateway-api` 控制面为 EPP 暴露参数优化所做的改造。接口契约见本目录 `api-changes.md`；摘要见本目录 `change-summary.md`。EPP 侧配置消费见 `ai-gateway-epp/docs/zh_cn/configuration/EPP配置定义说明-epp_config.md`；既有 EPP 调度对接见 `../2026-09-08-epp-scheduling-integration/design-changes.md`。

## 1. 概述

### 1.1 变更目标

在不改变接口结构（无新端点、无新表、无 DDL）的前提下，重新梳理 `clusters.epp_config` 简化用户形态的字段，使参数名与取值贴合 EPP 实际调度维度（排队 / KV），并补齐准入水位与坏后端剔除能力：

1. 调度档位 `scheduling_profile` → `load_profile`（实例负载画像），取值 `latency-first` / `throughput-first` → `queue-first` / `kv-first`，`balanced` 不变。
2. `cache_affinity` → `affinity`，语义由"scorer 权重覆盖"改为"亲和强度"（作用于 `prefix-cache-scorer` / `session-affinity-scorer`），新增 `off` 档。
3. 新增 `waiting_queue_max`、`running_requests_max`（准入水位）、`fallback_on_empty`（过滤回退）、`metrics_staleness_threshold_ms`（坏后端剔除）。
4. 编译产物新增 `saturation-detector`（**始终下发**）与 `flowControl.saturationDetector`；`kv_cache_utilization_max` 作为过滤与背压的**单一阈值来源**。
5. `flow_control.enable_eviction` 停止下发；`no_endpoint_queue_ttl` 缺省显式展开为 `queue_ttl`。
6. 旧字段保留 deprecated 别名，旧值自动映射，兼容存量配置。

### 1.2 可复用基础（代码事实，已核实）

| 能力 | 位置 | 说明 |
|------|------|------|
| 简化配置结构 | `model/epp_pool/epp_config.go:50-66` | `EppConfigSimplified` / `FlowControlSimplified`，全部字段用指针区分"未设置"与"显式值" |
| 严格解析 | `model/epp_pool/epp_config.go:70-84` | `ParseEppConfig` 用 `DisallowUnknownFields`，空串返回 `nil`；新增字段需同步加入结构体，否则旧数据解析报"unknown field" |
| 字段校验 | `model/epp_pool/epp_config.go:101-170` | `Validate()` 覆盖枚举 / 范围 / 必填联动；由 cluster 写入路径调用 |
| 有效值解析 | `model/epp_pool/epp_config.go:172-202` | `EffectiveSchedulingProfile` / `EffectiveKVCacheUtilizationMax` 等 |
| 编译模板 | `model/epp_pool/compiler.go:198-366` | `CompileEppConfig` 固定注入 discovery / util-filter / kv-scorer / queue-scorer / picker / parser；`scorerWeights` 权重表在 `:188-196` |
| 流控编译 | `model/epp_pool/compiler.go:338-366` | `compileFlowControl`；当前仅在 `conf.FlowControl != nil` 时下发 `flowControl` 段并开 featureGate（`:306-309`） |
| 导出接线 | `model/epp_pool/epp_data.go:82-91` | 逐 EPP cluster `ParseEppConfig` → `CompileEppConfig`（本方案不改其逻辑） |
| 校验入口 | `model/icluster_conf/epp.go:54-88` | `validateClusterBalanceConfig`：`epp_config` 非空即校验，`balance_mode=EPP` 时禁止空配置 |

### 1.3 现有缺口

| 缺口 | 现状 |
|------|------|
| 档位命名与实际含义错位 | 代码常量 `SchedulingProfileLatencyFirst` / `SchedulingProfileThroughputFirst`（`epp_config.go:29-31`）只映射 (kv, queue) 权重（`compiler.go:188-191`），但命名让用户误以为"整体策略"；`balanced` 权重为 `(1.0, 0.5)` |
| 亲和强度不可调 | `prefix-cache-scorer` / `session-affinity-scorer` 权重硬编码 `1.0`（`compiler.go:250, 269`），无开关档位；`cache_affinity` 名不副实（实为 (kv, queue) 权重覆盖，`compiler.go:317-330`） |
| 准入维度单一 | `util-filter` 仅生成 `kv-cache-utilization` 条件（`compiler.go:212-223`）；无排队 / 并发水位，无 `fallbackOnEmpty` |
| 无坏后端剔除 | 无 detector，无指标过期阈值；后端失联时依赖指标中性化退化，无显式剔除 |
| 无 detector 声明 | `flowControl` 段仅在配了 `flow_control` 时下发（`compiler.go:306-309`），不存在 `saturation-detector` / `saturationDetector` |
| `enable_eviction` 空转 | 当前原样透传（`compiler.go:360-362`），但 EPP 侧未接线，用户开启后无效果 |

---

## 2. 简化配置字段设计

### 2.1 字段与默认值

| 字段 | 类型 | 默认 | 语义 |
|------|------|------|------|
| `load_profile` | string | `balanced` | 实例负载画像：`queue-first` / `balanced` / `kv-first` |
| `affinity` | string | `medium` | 亲和强度：`off` / `low` / `medium` / `high` |
| `prefix_cache_affinity` | bool | `true` | 前缀缓存亲和开关（不变） |
| `session_affinity_enabled` | bool | `false` | 会话亲和开关（不变） |
| `session_affinity_header` | string | — | session id 请求头；`enabled=true` 时必填（不变） |
| `kv_cache_utilization_max` | float | `0.9` | KV 利用率准入水位，`(0, 1]`；过滤与背压单一来源 |
| `waiting_queue_max` | number | `0`（不启用） | 排队长度准入水位，`>= 0` |
| `running_requests_max` | number | `0`（不启用） | 运行中请求数准入水位，`>= 0` |
| `fallback_on_empty` | bool | `false` | 所有后端被过滤时的回退开关 |
| `metrics_staleness_threshold_ms` | int | `200` | 坏后端剔除阈值，`> 0`（抓取周期约 50ms → 4 次失败） |
| `flow_control` | object | — | 流控段（字段不变；`enable_eviction` 不再下发） |

### 2.2 `load_profile`（实例负载画像）

`load_profile` 只调节两个基础 scorer 的权重，取值语义与映射：

| `load_profile` | (kv-scorer, queue-scorer) 权重 | 含义 |
|---|---|---|
| `queue-first` | (0.2, 1.0) | 偏向**排队短**（通常更低延迟） |
| `balanced` | (0.6, 0.6) | 均衡 |
| `kv-first` | (1.0, 0.2) | 偏向 **KV 空闲**（通常更高吞吐） |

- 与旧 `scheduling_profile` 的映射：`latency-first` → `queue-first`（权重不变 (0.2, 1.0)）、`throughput-first` → `kv-first`（权重不变 (1.0, 0.2)）。
- **行为变更**：`balanced` 权重由当前 `(1.0, 0.5)` 调整为 `(0.6, 0.6)`——默认带宽下 kv/queue 等权，避免默认画像过度偏向 KV。发布说明需明示。
- 旧字段 `cache_affinity` 作为 deprecated 别名映射为画像覆盖（`low`→`queue-first`、`medium`→`balanced`、`high`→`kv-first`）。

### 2.3 `affinity`（亲和强度）

| `affinity` | 亲和 scorer 权重 | 注入行为 |
|---|---|---|
| `off` | 0 | **不注入** `prefix-cache-scorer` / `session-affinity-scorer` |
| `low` | 0.3 | 注入（若对应特性开关为 true） |
| `medium` | 0.6 | 注入（若对应特性开关为 true） |
| `high` | 1.0 | 注入（若对应特性开关为 true） |

- 亲和强度**正交**于 `prefix_cache_affinity` / `session_affinity_enabled`：特性开关决定"是否注入"，`affinity` 决定"注入权重"。当 `affinity=off` 时，即使特性开关为 true 也不注入（亲和整体关闭）。
- **行为变更**：当前亲和 scorer 权重固定 `1.0`；新默认 `medium` = `0.6`，即默认亲和力度下降，kv/queue 基础打分相对权重上升。发布说明需明示。
- 旧 `cache_affinity` **不再**驱动亲和强度（避免与其 deprecated 的画像覆盖语义冲突）；未写 `affinity` 时走默认 `medium`。

### 2.4 准入水位与坏后端剔除

三组准入维度共同构成 `utilization-filter` 的 `conditions`：

| 用户参数 | 过滤条件 | 条件始终存在？ |
|---|---|---|
| `kv_cache_utilization_max` | `{ "metric": "kv-cache-utilization", "maxValue": <v> }` | 是 |
| `waiting_queue_max`（`> 0`） | `{ "metric": "waiting-queue", "maxValue": <v> }` | 否（`0` 不加） |
| `running_requests_max`（`> 0`） | `{ "metric": "running-requests", "maxValue": <v> }` | 否（`0` 不加） |

- `fallback_on_empty` 直接透传 `utilization-filter.parameters.fallbackOnEmpty`：`false` = 所有后端被过滤时无后端可选；`true` = 回退派给"最不差"的后端。
- `metrics_staleness_threshold_ms` 透传 detector 的 `metricsStalenessThreshold`：某后端"最后一次成功更新指标"距今超过该阈值即从调度候选剔除，直到恢复抓取。

### 2.5 `flow_control`

字段本身不变（`max_requests` / `queue_ttl` / `no_endpoint_queue_ttl` / `enable_eviction`），编译语义调整：

- `no_endpoint_queue_ttl` 缺省时**显式展开为 `queue_ttl`**（此前缺省则不生成 `noEndpointRequestTTL`，由 EPP 自行决定）。
- `enable_eviction` **不再下发**（当前 EPP 未接线）；建议校验拒绝 `true`。
- `flowControl` 段**始终下发**（见 §3.4），`featureGates: ["flowControl"]` 仅在存在 `flow_control` 参数时追加（不排队、不背压，仅下发达成 detector / band0 声明）。

---

## 3. 编译规则（简化配置 → `EndpointPickerConfig`）

编译模板固化在 `model/epp_pool/compiler.go`，本次改造点如下。

### 3.1 常量与权重表（`epp_config.go` / `compiler.go`）

```go
// epp_config.go —— 档位常量重命名 + 亲和档位新增
const (
    LoadProfileQueueFirst = "queue-first"
    LoadProfileBalanced   = "balanced"
    LoadProfileKVFirst    = "kv-first"

    AffinityOff    = "off"
    AffinityLow    = "low"
    AffinityMedium = "medium"
    AffinityHigh   = "high"

    DefaultLoadProfile                 = LoadProfileBalanced
    DefaultAffinity                    = AffinityMedium
    DefaultPrefixCacheAffinity         = true
    DefaultKVCacheUtilizationMax       = 0.9
    DefaultWaitingQueueMax             = 0
    DefaultRunningRequestsMax          = 0
    DefaultFallbackOnEmpty             = false
    DefaultMetricsStalenessThresholdMs = 200
    DefaultSessionAffinityEnabled      = false

    // 检测器内部固定值（本期不暴露）
    defaultSaturationQueueDepthThreshold = 5
    stalenessPolicyIgnore                = "ignore"
)
```

```go
// compiler.go —— 权重表
var loadProfileWeights = map[string][2]float64{
    LoadProfileQueueFirst: {0.2, 1.0},
    LoadProfileBalanced:   {0.6, 0.6},
    LoadProfileKVFirst:    {1.0, 0.2},
}

var affinityWeights = map[string]float64{
    AffinityOff: 0, AffinityLow: 0.3, AffinityMedium: 0.6, AffinityHigh: 1.0,
}
```

### 3.2 有效值解析（`epp_config.go`）

```go
// EffectiveLoadProfile 解析画像：新字段优先，其次旧 scheduling_profile（旧值映射），
// 再次旧 cache_affinity（作为画像覆盖），最后默认 balanced。
func (c *EppConfigSimplified) EffectiveLoadProfile() string {
    if c == nil {
        return DefaultLoadProfile
    }
    if c.LoadProfile != nil {
        return *c.LoadProfile
    }
    if c.SchedulingProfile != nil { // 兼容旧 scheduling_profile
        switch *c.SchedulingProfile {
        case "latency-first":
            return LoadProfileQueueFirst
        case "throughput-first":
            return LoadProfileKVFirst
        default: // balanced
            return LoadProfileBalanced
        }
    }
    if c.CacheAffinity != nil { // 兼容旧 cache_affinity（画像覆盖）
        switch *c.CacheAffinity {
        case "low":
            return LoadProfileQueueFirst
        case "high":
            return LoadProfileKVFirst
        default:
            return LoadProfileBalanced
        }
    }
    return DefaultLoadProfile
}

// EffectiveAffinity / EffectiveWaitingQueueMax / EffectiveRunningRequestsMax /
// EffectiveFallbackOnEmpty / EffectiveMetricsStalenessThresholdMs 均为"未设置走默认"。
```

> 旧值映射是**必须**的：若直接返回 `scheduling_profile` 的原值（`latency-first`），`loadProfileWeights` 查表会失败并静默回退 `balanced`，使旧配置失效。

### 3.3 编译函数（`compiler.go`）

`CompileEppConfig` 的变更点：

1. `compileLoadProfileWeights(conf)` 取 (kv, queue) 权重（替代 `compileScorerWeights`，删除 `cache_affinity` 覆盖逻辑）。
2. `compileAffinityWeight(conf)` 取亲和权重；`affinity=off`（权重 0）时**不注入** `prefix-cache-scorer` / `session-affinity-scorer`。
3. 固定插件序列新增 `compileSaturationDetectorPlugin(conf)`（始终注入）。
4. `util-filter` 编译改为 `compileUtilFilterPlugin(conf)`：`conditions` 按 §2.4 组装，并加 `fallbackOnEmpty`。
5. `flowControl` 段与 `saturationDetector.pluginRef` 始终下发（见 §3.4）。

```go
func CompileEppConfig(clusterName string, conf *EppConfigSimplified) *EndpointPickerConfig {
    kvWeight, queueWeight := compileLoadProfileWeights(conf)
    affinityWeight := compileAffinityWeight(conf) // affinity=off → 0

    plugins := []*PluginConfig{
        {Name: pluginNameDiscovery, Type: pluginTypeDiscovery,
            Parameters: map[string]interface{}{"clusterName": clusterName}},
        compileUtilFilterPlugin(conf),         // 准入过滤（含 kv/waiting/running）
        compileSaturationDetectorPlugin(conf), // 水位检测器（含坏后端剔除时长）
        {Name: pluginNameKVScorer, Type: pluginTypeKVScorer, Parameters: map[string]interface{}{}},
        {Name: pluginNameQueueScorer, Type: pluginTypeQueueScorer, Parameters: map[string]interface{}{}},
    }
    // ... profile 组装：kv/queue 权重来自 load_profile；
    //     亲和 scorer 仅在 affinityWeight > 0 且对应特性开关为 true 时注入，权重 = affinityWeight。
    // ... 追加 max-score-picker / openai-parser。

    compiled := &EndpointPickerConfig{ /* plugins / schedulingProfiles / dataLayer / requestHandler */ }

    // 流控段始终下发（含 saturationDetector）；仅当存在真正的流控参数时才开 featureGate。
    compiled.FlowControl = compileFlowControl(conf)
    if conf.FlowControl != nil {
        compiled.FeatureGates = []string{featureGateFlowControl}
    }
    return compiled
}

func compileLoadProfileWeights(conf *EppConfigSimplified) (float64, float64) {
    w, ok := loadProfileWeights[conf.EffectiveLoadProfile()]
    if !ok {
        w = loadProfileWeights[DefaultLoadProfile]
    }
    return w[0], w[1]
}

func compileAffinityWeight(conf *EppConfigSimplified) float64 {
    w, ok := affinityWeights[conf.EffectiveAffinity()]
    if !ok {
        return affinityWeights[DefaultAffinity]
    }
    return w
}
```

准入过滤与检测器子编译函数：

```go
// 准入过滤器：kv 条件始终存在；队列/并发仅 >0 时加入。
func compileUtilFilterPlugin(conf *EppConfigSimplified) *PluginConfig {
    conditions := []map[string]interface{}{
        {"metric": metricKVCacheUtilization, "maxValue": conf.EffectiveKVCacheUtilizationMax()},
    }
    if q := conf.EffectiveWaitingQueueMax(); q > 0 {
        conditions = append(conditions, map[string]interface{}{"metric": metricWaitingQueue, "maxValue": q})
    }
    if r := conf.EffectiveRunningRequestsMax(); r > 0 {
        conditions = append(conditions, map[string]interface{}{"metric": metricRunningRequests, "maxValue": r})
    }
    return &PluginConfig{
        Name: pluginNameUtilFilter, Type: pluginTypeUtilFilter,
        Parameters: map[string]interface{}{
            "conditions":      conditions,
            "fallbackOnEmpty": conf.EffectiveFallbackOnEmpty(),
        },
    }
}

// 水位检测器：kv 阈值与过滤同源；队列阈值取 waiting_queue_max（未启用用默认 5）；
// 过期策略固定 ignore；headroom 固定 0；坏后端剔除时长取用户配置。
func compileSaturationDetectorPlugin(conf *EppConfigSimplified) *PluginConfig {
    qDepth := defaultSaturationQueueDepthThreshold
    if q := conf.EffectiveWaitingQueueMax(); q > 0 {
        qDepth = int(q)
    }
    return &PluginConfig{
        Name: pluginNameSaturationDetector, Type: pluginTypeSaturationDetector,
        Parameters: map[string]interface{}{
            "kvCacheUtilThreshold":      conf.EffectiveKVCacheUtilizationMax(),
            "queueDepthThreshold":       qDepth,
            "stalenessPolicy":           stalenessPolicyIgnore,
            "headroom":                  0.0,
            "metricsStalenessThreshold": fmt.Sprintf("%dms", conf.EffectiveMetricsStalenessThresholdMs()),
        },
    }
}
```

### 3.4 detector 始终下发与流控段

- **为什么 detector 始终下发**：① `saturation-detector` 同时服务"端点过滤"与"背压"，必须在过滤路径存在；② 坏后端剔除依赖 detector，无 detector 则无剔除；③ 显式声明避免"用户没配检测器、却在用隐藏默认阈值过滤/背压"的不可审计状态。
- `saturation-detector` 只出现在 `plugins[]` 与 `flowControl.saturationDetector`，**不写入 `schedulingProfiles[].plugins`**——由 llm-d 自动作为过滤器生效。
- `flowControl` 段始终下发（承载 `saturationDetector.pluginRef` 与 band0）；`featureGates: ["flowControl"]` 仅在 `conf.FlowControl != nil` 时追加，保持"是否启用排队/背压"的语义边界。

```go
// 流控段：始终含 saturationDetector（filter 依赖）；缺省时 noEndpointRequestTTL 显式展开为 queue_ttl。
func compileFlowControl(conf *EppConfigSimplified) *FlowControlConfig {
    compiled := &FlowControlConfig{}
    band := PriorityBandConfig{Priority: 0, MaxBytes: defaultPriorityBandMaxBytes}

    if fc := conf.FlowControl; fc != nil {
        if fc.MaxRequests != nil && *fc.MaxRequests > 0 {
            q := strconv.Itoa(*fc.MaxRequests)
            compiled.MaxRequests = q
            band.MaxRequests = q // band 0 联动全局
        } else {
            band.MaxRequests = defaultPriorityBandMaxRequests // -1/未设：band 显式兜底
        }
        if fc.QueueTTL != nil {
            compiled.DefaultRequestTTL = secondsToDuration(*fc.QueueTTL)
        }
        // 文档契约：no_endpoint_queue_ttl 缺省跟随 queue_ttl
        if fc.NoEndpointQueueTTL != nil {
            compiled.NoEndpointRequestTTL = secondsToDuration(*fc.NoEndpointQueueTTL)
        } else if fc.QueueTTL != nil {
            compiled.NoEndpointRequestTTL = secondsToDuration(*fc.QueueTTL)
        }
        // enable_eviction：当前不下发（EPP 未接线）
    } else {
        band.MaxRequests = defaultPriorityBandMaxRequests
    }

    compiled.PriorityBands = []PriorityBandConfig{band}
    compiled.SaturationDetector = &SaturationDetectorConfig{PluginRef: pluginNameSaturationDetector}
    return compiled
}
```

编译产物类型补充（`compiler.go`）：

```go
const (
    pluginNameSaturationDetector = "saturation-detector"
    pluginTypeSaturationDetector = "utilization-detector"
    metricWaitingQueue           = "waiting-queue"
    metricRunningRequests        = "running-requests"
)

type FlowControlConfig struct {
    MaxRequests          string                    `json:"maxRequests,omitempty"`
    DefaultRequestTTL    string                    `json:"defaultRequestTTL,omitempty"`
    NoEndpointRequestTTL string                    `json:"noEndpointRequestTTL,omitempty"`
    EnableEviction       bool                      `json:"enableEviction,omitempty"`
    SaturationDetector   *SaturationDetectorConfig `json:"saturationDetector,omitempty"` // 新增
    PriorityBands        []PriorityBandConfig      `json:"priorityBands,omitempty"`
}

type SaturationDetectorConfig struct {
    PluginRef string `json:"pluginRef"`
}
```

> `compiler.go` 需新增 `"fmt"` import（`metricsStalenessThreshold` 拼接）。

---

## 4. 兼容与迁移设计

- **结构体保留旧字段**：`EppConfigSimplified` 新增新字段的同时保留 `SchedulingProfile *string`、`CacheAffinity *string`（deprecated），否则 `ParseEppConfig` 的 `DisallowUnknownFields` 会让存量 JSON 解析报错。
- **旧值映射**：`latency-first` → `queue-first`、`throughput-first` → `kv-first`（§3.2）；不做映射将静默回退 `balanced`。
- **优先级**：`load_profile` > `scheduling_profile` > `cache_affinity`（画像解析）；`affinity` 独立，不从旧字段推导。
- **存储不变**：`clusters.epp_config` 仍存用户原始 JSON，未显式携带字段不落盘，GET 回读与写入一致；默认值只体现在编译时。
- **迁移建议**：新集群用新字段；存量集群编辑时重写为新字段。旧字段去除另立兼容期后事项，本期不移除。
- **行为变更清单（发布说明需明示）**：
  1. `balanced` 权重 `(1.0, 0.5)` → `(0.6, 0.6)`；
  2. 亲和 scorer 权重由固定 `1.0` → 随 `affinity`（默认 `medium` = `0.6`）；
  3. 已有 EPP cluster 编译产物新增 `saturation-detector` 与 `flowControl.saturationDetector`；
  4. `no_endpoint_queue_ttl` 缺省时由"EPP 决定"变为"显式 = `queue_ttl`"；
  5. `enable_eviction` 不再下发。

---

## 5. 校验设计（`epp_config.go` `Validate()`）

在现有校验基础上新增 / 调整：

```go
if c.LoadProfile != nil {
    switch *c.LoadProfile {
    case LoadProfileQueueFirst, LoadProfileBalanced, LoadProfileKVFirst:
    default:
        return xerror.WrapParamErrorWithMsg(
            "epp_config.load_profile must be one of %q/%q/%q, got %q",
            LoadProfileQueueFirst, LoadProfileBalanced, LoadProfileKVFirst, *c.LoadProfile)
    }
}
if c.Affinity != nil {
    switch *c.Affinity {
    case AffinityOff, AffinityLow, AffinityMedium, AffinityHigh:
    default:
        return xerror.WrapParamErrorWithMsg(
            "epp_config.affinity must be one of %q/%q/%q/%q, got %q",
            AffinityOff, AffinityLow, AffinityMedium, AffinityHigh, *c.Affinity)
    }
}
if c.KVCacheUtilizationMax != nil && (*c.KVCacheUtilizationMax <= 0 || *c.KVCacheUtilizationMax > 1) {
    return xerror.WrapParamErrorWithMsg("epp_config.kv_cache_utilization_max must be in (0, 1], got %v", *c.KVCacheUtilizationMax)
}
if c.WaitingQueueMax != nil && *c.WaitingQueueMax < 0 {
    return xerror.WrapParamErrorWithMsg("epp_config.waiting_queue_max must be >= 0, got %v", *c.WaitingQueueMax)
}
if c.RunningRequestsMax != nil && *c.RunningRequestsMax < 0 {
    return xerror.WrapParamErrorWithMsg("epp_config.running_requests_max must be >= 0, got %v", *c.RunningRequestsMax)
}
if c.MetricsStalenessThresholdMs != nil && *c.MetricsStalenessThresholdMs <= 0 {
    return xerror.WrapParamErrorWithMsg("epp_config.metrics_staleness_threshold_ms must be > 0, got %d", *c.MetricsStalenessThresholdMs)
}
// EnableEviction=true 建议拒绝（当前不支持）：
//   return xerror.WrapParamErrorWithMsg("epp_config.flow_control.enable_eviction is not supported, keep false")
```

其余沿用现状：`session_affinity_header` 规则、`flow_control` 的 `max_requests` / TTL 范围；旧字段 `scheduling_profile` / `cache_affinity` 仍在 `Validate()` 中做枚举校验（保证休眠保留的旧配置合法）。

---

## 6. 存储与数据库

- **无 DDL 变更**：复用 `clusters.epp_config`（JSON text，可空）。
- 列内容形态变化：新写入为"新字段名 + 新枚举值"；旧 JSON 通过别名与旧值映射兼容读取。默认值不落盘。
- 无需 migration：`ParseEppConfig` 严格解析的字段集合已包含新旧全部字段。

---

## 7. 涉及文件清单

| 文件 | 改造点 |
|------|--------|
| `model/epp_pool/epp_config.go` | 结构体新增 `LoadProfile` / `Affinity` / `WaitingQueueMax` / `RunningRequestsMax` / `FallbackOnEmpty` / `MetricsStalenessThresholdMs`，保留 deprecated `SchedulingProfile` / `CacheAffinity`；`IsEmpty()` 纳入新字段；`Validate()` 新增枚举/范围校验；新增 `Effective*` 访问器与旧值映射 |
| `model/epp_pool/compiler.go` | 权重表改为 `loadProfileWeights` / `affinityWeights`；`compileScorerWeights` → `compileLoadProfileWeights` + `compileAffinityWeight`；新增 `compileUtilFilterPlugin` / `compileSaturationDetectorPlugin`；`compileFlowControl` 改版（始终 `saturationDetector`、`no_endpoint` 展开、去掉 `enableEviction`）；新增 `SaturationDetectorConfig` 与插件/指标常量；新增 `"fmt"` import |
| `model/epp_pool/compiler_test.go` | 更新权重断言与产物断言；新增 detector / conditions / fallbackOnEmpty / no_endpoint 展开 / enable_eviction 不下发用例 |
| `model/epp_pool/epp_config_test.go` | 新增/调整字段校验、旧字段别名与旧值映射、`IsEmpty` 用例 |
| `model/epp_pool/epp_data.go` | **不改逻辑**（`ParseEppConfig` → `CompileEppConfig` 链路不变） |
| `model/icluster_conf/epp.go` | **不改逻辑**（校验入口 `validateClusterBalanceConfig` 不变，自动受益于 `Validate()` 增强） |
| `design-docs/api-define/OpenAPI接口定义/clusters.md` | `epp_config` 字段说明 / 示例 / 约束同步（见 `api-changes.md` §4） |
| `design-docs/api-define/InnerAPI接口定义/epp-data.md` | `epp_config` 段示例与说明同步（见 `api-changes.md` §4） |

---

## 8. 测试计划

### 8.1 验收标准（checkable）

1. `POST/PUT /clusters` 携带新字段（`load_profile` / `affinity` / `waiting_queue_max` / `running_requests_max` / `fallback_on_empty` / `metrics_staleness_threshold_ms`）可正常写入并校验；非法值返回 422 且错误信息含字段路径。
2. 仅写旧字段 `scheduling_profile=latency-first` 的存量配置，编译产物 `kv-scorer` / `queue-scorer` 权重等价于 `load_profile=queue-first`；`throughput-first` 等价 `kv-first`；`balanced` 不变。
3. `affinity=off` 时编译产物不含 `prefix-cache-scorer` / `session-affinity-scorer`（即使特性开关为 true）。
4. `waiting_queue_max>0` / `running_requests_max>0` 时 `util-filter.conditions` 出现对应条目，且 detector `queueDepthThreshold` 取 `waiting_queue_max`；为 `0` 时不出现且 detector 用默认 `5`。
5. 任意配置下编译产物均含 `saturation-detector` 与 `flowControl.saturationDetector`；未配 `flow_control` 时无 `featureGates`。
6. `metrics_staleness_threshold_ms=300` → detector `metricsStalenessThreshold="300ms"`。
7. `no_endpoint_queue_ttl` 缺省且 `queue_ttl=30` → `noEndpointRequestTTL="30s"`；显式 `600` → `"10m0s"`。
8. `enable_eviction=true` 写入被拒；`false`/缺省时产物不含 `enableEviction`。
9. `epp_data` 导出对全部 EPP cluster 生效，version 增量语义不变（重复拉取 `Data: null`）。

### 8.2 单元测试

- `EffectiveLoadProfile`：新字段优先、旧 `scheduling_profile` 旧值映射、旧 `cache_affinity` 画像覆盖、默认 `balanced`。
- `Validate`：新枚举/范围校验；`enable_eviction=true` 拒绝；旧字段枚举仍校验。
- `CompileEppConfig`：`load_profile` 三档权重；`affinity` 四档（含 `off` 不注入）；`util-filter` conditions / `fallbackOnEmpty`；detector 参数与默认值；`flowControl` 始终下发 + `saturationDetector`；`no_endpoint` 展开；`enable_eviction` 不下发。
- `ParseEppConfig`：含新旧字段的 JSON 均可解析；未知字段仍报错；空串返回 `nil`。

### 8.3 集成测试

- OpenAPI：`PUT /clusters`（`balance_mode=EPP` + 新形态 `epp_config`）→ `GET /clusters` 回读一致 → `GET /configs/epp_data/config` 校验 `epp_config` 段产物（detector / 权重 / conditions）。
- 兼容：旧形态 `epp_config` 写入的 cluster 导出产物等价于新形态映射结果。

---

## 9. 风险与注意事项

| 风险 | 说明 | 缓解措施 |
|------|------|----------|
| 默认行为变更 | `balanced` 权重与亲和 scorer 权重变化，存量集群调度结果可能改变 | 在发布说明中明示；单测锁定新权重；建议灰度观察 |
| 产物自动新增 detector | 存量 EPP cluster 的 `epp_config` 段内容变化，可能与 EPP 侧预期不一致 | 确认 EPP 侧支持 `saturation-detector` / `flowControl.saturationDetector`；否则先评估忽略该字段的影响 |
| 旧值映射遗漏 | 若 `EffectiveLoadProfile` 直接返回旧值，`latency-first` / `throughput-first` 会静默回退 `balanced` | 显式映射 + 单测覆盖；兼容期保留旧字段 |
| `cache_affinity` 语义重映射 | 旧 `cache_affinity` 由"权重覆盖"改为"画像覆盖"，用户预期可能变化 | 文档明确；仅影响旧字段，新字段 `affinity` 语义独立 |
| `enable_eviction=true` 被拒 | 存量若有人误开 `true`，升级后写入被拒 | 本期仅在写入校验拒绝；读取/导出仍兼容（不下发），不阻塞存量导出 |
| `flowControl` 始终下发 | 未配 `flow_control` 也下发流控段，可能被误读为"已启用排队" | `featureGates` 不开即不启用排队/背压；文档与示例明示 |
| `DisallowUnknownFields` 严格解析 | 旧代码若不认识新字段会解析失败（反向兼容） | 本方案为 api 侧单仓升级，无旧代码消费新 JSON 的场景 |