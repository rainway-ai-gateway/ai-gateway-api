# /clusters

## 1. 数据模型

```json
{
    "name": "my-cluster",
    "description": "示例集群",
    "basic": {
        "protocol": "http",
        "connection": {
            "max_idle_conn_per_rs": 0,
            "cancel_on_client_close": false
        },
        "retries": {
            "max_retry_in_cluster": 2
        },
        "buffers": {
            "req_write_buffer_size": 512
        },
        "timeouts": {
            "timeout_conn_serv": 50000,
            "timeout_response_header": 50000,
            "timeout_readbody_client": 30000,
            "timeout_read_client_again": 30000,
            "timeout_write_client": 60000
        }
    },
    "sticky_sessions": {
        "enabled": false,
        "hash_strategy": "CLIENT_IP_ONLY",
        "hash_header": ""
    },
    "passive_health_check": {
        "interval": 1000,
        "failnum": 3,
        "host": "",
        "uri": "/",
        "statuscode": 0
    },
    "llm_config": {
        "models": ["deepseek-chat", "deepseek-coder"],
        "model_mappings": [
            {"source_model": "gpt-4", "target_model": "deepseek-chat"}
        ],
        "keys": [
            {
                "name": "key-primary",
                "weight": 70
            },
            {
                "name": "key-secondary",
                "weight": 30
            }
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 3,
            "retry_backoff_initial": 500,
            "retry_backoff_max": 5000
        },
        "key_affinity": {
            "enabled": true,
            "ttl": 600,
            "redis_prefix": "bfe:ai:key_affinity",
            "penalty_enable": true
        },
        "provider": "deepseek",
        "match_prefix": "deepseek/",
        "strip_prefix": true
    },
    "balance_mode": "WRR",
    "epp_config": null
}
```

**数据模型示例（EPP 模式集群）**

```json
{
    "name": "cluster-a",
    "description": "EPP 调度集群",
    "balance_mode": "EPP",
    "epp_config": {
        "load_profile": "balanced",
        "affinity": "medium",
        "prefix_cache_affinity": true,
        "session_affinity_enabled": true,
        "session_affinity_header": "x-session-id",
        "kv_cache_utilization_max": 0.9,
        "waiting_queue_max": 0,
        "running_requests_max": 0,
        "fallback_on_empty": false,
        "metrics_staleness_threshold_ms": 200,
        "flow_control": {
            "max_requests": 1000,
            "queue_ttl": 30,
            "no_endpoint_queue_ttl": 600,
            "enable_eviction": false
        }
    },
    "llm_config": {
        "models": ["deepseek-chat"],
        "provider": "deepseek"
    }
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|----------|
| `name` | string | 集群名 | 全局唯一 | 必填；类型为 [ClusterName](./00-common.md#15-集群名称clustername) |
| `description` | string | 集群描述信息 | - | 非必填；若传入，长度 0-256 字符；不能包含控制字符 |
| `basic` | object | 基本参数 | 见下方 表：连接设置、表：重试设置、表：超时设置 | 非必填；未传时使用 AI 网关场景默认值 |
| `sticky_sessions` | object | 会话保持 | 见下方 表：会话保持 | 非必填；未传时使用默认值 |
| `passive_health_check` | object | 被动健康检查 | 见下方 表：被动健康检查 | 非必填；未传时使用默认值 |
| `llm_config` | object | AI LLM 服务配置 | 见下方 表：LLM配置 | 必填 |
| `balance_mode` | string | 集群均衡模式 | `WRR`、`EPP` | 非必填；默认值为 `WRR`；有效枚举：`WRR`、`EPP` |
| `epp_config` | object | EPP 调度配置（简化用户形态：实例负载画像 + 准入水位 + 排队，见下方 表：EPP调度配置）。仅 `balance_mode=EPP` 时生效；`WRR` 时保留但不生效（休眠），便于 EPP ↔ WRR 来回切换不丢配置 | - | 条件必填：`balance_mode=EPP` 时必填且通过字段校验；`balance_mode=WRR` 时可选（传入则保留并做格式校验，但不生效） |

**表：连接设置**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| max_idle_conn_per_rs| int | 连接池| N | 每个BFE实例，为集群中每个RS维持的空闲长连接数。一般情况下，无需特别维持，设置为0 。<br/>设置为非0时，可以提升转发性能 | 非必填；默认值为0；须为 >=0 的整数 |
| cancel_on_client_close| bool |  连接是否级联关闭 | N | 设置为true时，当客户端关闭连接后，BFE同时关闭对应RS的连接 <br/>设置为false时，当客户端关闭连接后，BFE按默认策略关闭对应RS的连接 | 非必填；默认值为 `false` |

**表：重试设置**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| max_retry_in_cluster| int |  同一个集群内重试次数 | N | 底层对应 `max_retry_in_subcluster`，**默认值为2** | 非必填；默认值为2；须为 >=0 的整数 |

**表：会话保持**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| enabled| bool |  是否开启会话保持 | N | **默认false**。为true时开启会话保持；为false时关闭 | 非必填；默认值为 `false` |
| hash_strategy| string |  会话保持策略  | N | CLIENT_IP_ONLY，根据client ip做会话保持(默认值) <br/>	CLIENT_ID_ONLY，根据请求中header做会话保持 <br>	CLIENT_ID_PERFERED，优先基于特定header，如果请求中没有对应header，则使用client ip| 非必填；默认值为 `CLIENT_IP_ONLY`；有效枚举：`CLIENT_IP_ONLY`、`CLIENT_ID_ONLY`、`CLIENT_ID_PREFERED` |
| hash_header| string |  指定CLIENT_ID使用的header | N | 当使用cookie作为会话保持的哈希key时，数据格式为Cookie:${key} | 非必填；默认值为空字符串；`enabled=true` 且 `hash_strategy` 为 `CLIENT_ID_ONLY` 或 `CLIENT_ID_PREFERED` 时必须非空 |

**表：超时设置**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| timeout_conn_serv| int |  连接后端超时(ms)| N |  | 非必填；默认值为50000；须为 >0 的整数 |
| timeout_response_header| int |  读后端响应头部超时(ms)| N |  | 非必填；默认值为50000；须为 >0 的整数 |
| timeout_readbody_client| int |  读请求body超时(ms)| N |  | 非必填；默认值为30000；须为 >0 的整数 |
| timeout_read_client_again| int |  与用户的长连接超时(ms) | N |  | 非必填；默认值为30000；须为 >0 的整数 |
| timeout_write_client| int |  写响应超时(ms)| N | - | 非必填；默认值为60000；须为 >0 的整数 |

**表：被动健康检查**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| failnum| int |  进入健康检查的失败次数阈值 | N | 连续转发失败多次后，BFE进入健康检查状态，对下游RS发起探活；**默认值为3** | 非必填；默认值为3；须为 >=0 的整数 |
| interval| int |  连续健康检查的时间间隔 | N | 单位ms；**默认值为1000** | 非必填；默认值为1000；须为 >=0 的整数 |
| host| string |  健康检查请求的域名| N | 为空时使用所属 provider 的 `instance_pool` 中第一个实例的 `addr` | 非必填；为空时使用 provider 首个实例的 `addr` |
| uri| string |  健康检查请求的URI  | N | **默认值为 `/`** | 非必填；默认值为 `/`；非空；须以 `/` 开头 |
| statuscode| int |  期望的健康检查返回码 | N | 如果需要忽略返回码，此处可以填0；**默认值为0** | 非必填；默认值为0；须为 `0` 或 `100-599` 的整数 |

**表：LLM配置**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| models| []string |  cluster 可转发的模型名称列表 | Y | 必须是所属 provider `models` 的子集 | 必填；至少1个元素；每个模型名非空；元素不能重复；每个元素必须存在于 provider 的 `models` 中 |
| model_mappings| []object |  模型名称映射 | N | 用于将用户请求的模型名映射为后端实际使用的模型名，具体字段见下方 表：模型映射 | 非必填；元素须满足 模型映射 结构约束 |
| keys| []ClusterKeyRef |  Key 引用与权重列表 | N | 一个 cluster 支持配置多个 Key，按权重做路由；通过 `name` 引用 provider 中定义的 key；为空数组表示不配置 API-Key | 非必填；默认值为空数组 `[]`；元素须满足 表：ClusterKeyRef 结构 |
| key_policy| object |  Key 路由策略 | N | 多 Key 时的选择策略、重试与退避配置 | 非必填；默认见下方 表：Key 路由策略；`strategy` 本版仅支持 `weighted_random` |
| key_affinity| object |  Key 亲和性配置 | N | 基于 Redis + `ClientKeyId` 的会话级 Key 亲和性 | 非必填；默认见下方 表：Key 亲和性配置 |
| provider| string |  所属 provider | Y | 用于关联 provider、解析后端实例池/key 明文/生成 `ModelTable` | 必填；非空；必须引用 `/providers` 中已存在的 provider |
| match_prefix| string |  需要匹配的 provider/model 前缀 | N | 例如 `openrouter/`；用于 OpenRouter 等聚合 provider 场景；必须以 `/` 结尾 | 非必填；`strip_prefix=true` 时必填 |
| strip_prefix| bool |  是否裁剪 `match_prefix` 指定前缀 | N | `true` 时转发给下游前会从请求 `model` 字段中去掉该前缀；`false` 时仅用于路由标识，不裁剪 | 非必填；默认 `false` |

> **注意：** `model_table` 不在 OpenAPI `/clusters` 端点中展示，仅通过 InnerAPI 根据 `provider` 自动填充后下发给 BFE。
>
> **注意：** `enable` 字段已移除，设置 `llm_config` 时默认开启 AI 网关能力。
>
> **注意：** cluster 不再返回 key 明文。`GET /clusters` 与 `GET /clusters/{name}` 仅返回 `keys[].name` 与 `keys[].weight`。

**表：ClusterKeyRef 结构（`llm_config.keys` 元素）**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| name | string | 引用 provider 中 key 的 name | Y | 用于标识引用的 provider key | 必填；非空；必须对应 provider `keys` 中存在的 name；同一 `keys` 数组内唯一 |
| weight | int | 权重 | Y | 用于加权随机选择，范围 `[0,100]` | 必填；取值范围 `[0,100]`；`0` 表示该 Key 不接收流量（等效于禁用） |

**表：Key 路由策略（`llm_config.key_policy`）**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| strategy | string | Key 选择策略 | N | 多 Key 时的选择算法 | 非必填；默认 `weighted_random`；本版仅支持 `weighted_random` |
| max_retries | int | 总额外重试次数 | N | 该 cluster 在当前请求内的总重试次数；不是单个 Key 的重试次数 | 非必填；默认 `0`；须为 `>=0` 的整数 |
| retry_backoff_initial | int (ms) | 初始退避时间 | N | 首次重试的退避时间，单位毫秒 | 非必填；默认 `500`；须为 `>=0` 的整数 |
| retry_backoff_max | int (ms) | 最大退避时间 | N | 退避时间上限，单位毫秒 | 非必填；默认 `5000`；须为 `>=0` 的整数，且须 `>= retry_backoff_initial` |

**表：Key 亲和性配置（`llm_config.key_affinity`）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| enabled | bool | 是否开启会话级 Key 亲和性 | N | 默认 `true`；为 `true` 时开启基于 Redis + `ClientKeyId` 的 Key 绑定 | 非必填；必须为 bool |
| ttl | int | 绑定空闲超时时间 | N | 单位秒，默认 `600`；命中绑定后 BFE 会刷新 TTL，持续请求则绑定保持 | 非必填；若传入，须为 `>0` 的整数 |
| redis_prefix | string | Redis key 前缀 | N | 默认 `"bfe:ai:key_affinity"` | 非必填；若传入，必须非空 |
| penalty_enable | bool | 是否开启 Key 惩罚 | N | 默认 `true`；为 `true` 时，近期返回 429/401/403 的 Key 会被跳过 | 非必填；必须为 bool |

**表：模型映射**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| source_model | string | 用户请求的模型名 | Y | - | 必填；非空；同一 `model_mappings` 内不能重复 |
| target_model | string | 映射后的实际模型名 | Y | - | 必填；非空 |

**表：EPP调度配置（`epp_config`）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| load_profile | string | **实例负载画像**：多个后端都可用时，偏向**排队短**还是 **KV 空闲**（仅调节 kv/queue 两个基础 scorer 的权重） | N | `queue-first`：偏向排队短（通常更低延迟）；`balanced`：均衡（默认）；`kv-first`：偏向 KV 空闲（通常更高吞吐） | 非必填；默认值为 `balanced`；有效枚举：`queue-first`、`balanced`、`kv-first` |
| affinity | string | **亲和强度**：把"相同前缀 / 同一会话"的请求往同一后端聚的力度（作用于 `prefix-cache-scorer` / `session-affinity-scorer` 的权重） | N | `off`：不注入亲和 scorer（关闭亲和，即使特性开关为 `true` 也不注入）；`low` / `medium` / `high`：亲和 scorer 权重分别为 `0.3` / `0.6` / `1.0` | 非必填；默认值为 `medium`；有效枚举：`off`、`low`、`medium`、`high` |
| prefix_cache_affinity | bool | 前缀缓存亲和（prefix-cache-scorer）：相同 prompt 前缀的请求经前缀哈希匹配**尽力收敛**到同一后端，提升 KV cache 复用；零参数、自主学习；**软亲和，不保证一定命中匹配后端** | N | 为 `true` 时注入亲和 scorer；为 `false` 时不注入；`affinity=off` 时同样不注入 | 非必填；默认值为 `true`；必须为 bool |
| session_affinity_enabled | bool | 会话亲和开关（session-affinity-scorer，session_id 策略）：开启后同一 session 的请求**尽力粘住**同一后端（有 binding 状态，亲和强度高于 prefix；但仍受利用率过滤与加权总分影响，非硬路由），绑定端点摘除后自动迁移重粘 | N | 为 `true` 时注入亲和 scorer（header 取 `session_affinity_header`）；为 `false` 时不注入；`affinity=off` 时同样不注入 | 非必填；默认值为 `false`；必须为 bool |
| session_affinity_header | string | session id 来源请求头（如 `x-session-id`），scorer 从该 header 解析 session id；`enabled=false` 时保留但不生效（休眠） | 条件必填 | 仅在 `session_affinity_enabled=true` 时生效 | `session_affinity_enabled=true` 时**必填**（非空 HTTP header 名）；`enabled=false`/缺省时可保留（非空 header 名，格式校验照常，不生效） |
| kv_cache_utilization_max | float | 端点过滤阈值：KV cache 利用率超过该值的端点被过滤；该值作为KV 阈值单一来源 | N | - | 非必填；默认值为 `0.9`；取值范围 `(0, 1]` |
| waiting_queue_max | number | 端点过滤阈值：后端排队长度超过该值的端点被过滤 | N | `0` = **不启用**该项过滤（非"上限为 0"）；`>0` 时，该值作为队列阈值单一来源 | 非必填；默认值为 `0`；须为 `>= 0` 的数 |
| running_requests_max | number | 端点过滤阈值：后端运行中请求数超过该值的端点被过滤 | N | `0` = **不启用**该项过滤（非"上限为 0"） | 非必填；默认值为 `0`；须为 `>= 0` 的数 |
| fallback_on_empty | bool | 所有端点都被过滤时的行为：`false` = 无端点可选、不派发；`true` = 回退派给"最不差"的端点 | N | 透传 `utilization-filter.fallbackOnEmpty` | 非必填；默认值为 `false`；必须为 bool |
| metrics_staleness_threshold_ms | int | **坏后端剔除**：后端指标超过该时长未更新，即从调度候选剔除，直到恢复抓取 | N | EPP 周期抓取各后端 `/metrics`（周期约 50ms）；默认 `200ms` ≈ 连续 4 次抓取失败；建议取值 100–500ms | 非必填；默认值为 `200`；须为 `> 0` 的整数（单位 ms） |
| flow_control | object | 流控参数；**编译产物始终包含流控段**（含检测器声明与 priority band 0），但仅当传入 `flow_control` 时才启用排队/背压 featureGate | N | 见下方 表：epp_config.flow_control | 非必填；若传入，须满足 flow_control 结构约束 |

**表：epp_config.flow_control**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| max_requests | int | 全局并发上限（跨全部优先级带）；`-1` 为显式"不限"（更新时用于明确恢复不限） | N | 缺省表示不限；`-1` 也表示不限 | 非必填；若传入，取值 `>0` 或 `-1` |
| queue_ttl | int | 池**有端点**时的排队预算（**单位：秒**），超期以可重试背压错误拒绝 | N | 缺省由 EPP 决定（llm-d 默认 60 秒）；`0` 为显式禁用驱逐 | 非必填；取值 ≥ 0 的整数 |
| no_endpoint_queue_ttl | int | 池**无端点**（冷启动扩容）时的排队预算（**单位：秒**），regime 切换时重新起算 | N | 默认值为跟随 `queue_ttl` | 非必填；同 `queue_ttl` |
| enable_eviction | bool | 需求驱动驱逐：高优先级被饱和阻塞时终止负优先级在飞请求回收容量 | N | **当前 EPP 未接线，请保持 `false`**；编译产物不下发该字段 | 非必填；默认值为 `false`；**建议仅允许 `false`（`true` 拒绝）** |

> **废弃字段（deprecated，兼容保留，勿用于新配置）**：
> - `scheduling_profile`（旧调度档位）：语义等价于 `load_profile`，旧值 `latency-first` → `queue-first`、`throughput-first` → `kv-first`、`balanced` 不变。同时存在时 `load_profile` 优先。
> - `cache_affinity`（旧 scorer 权重覆盖项，枚举 `low`/`medium`/`high`）：按 `low` → `queue-first`、`medium` → `balanced`、`high` → `kv-first` 映射为 `load_profile` 覆盖；**不再**影响新 `affinity`。二者同时存在时 `load_profile` 优先。


**约束**

- `name` 必填，类型为 [ClusterName](./00-common.md#15-集群名称clustername)，全局唯一。
- `description` 可选；若传入，长度 0-256 字符，不能包含控制字符。
- `llm_config` 必填；`provider` 必填且必须引用 `/providers` 中已存在的 provider。
- `llm_config.models` 至少包含 1 个非空模型名且不能重复；每个模型名必须存在于所属 provider 的 `models` 中。
- `llm_config.model_mappings` 中 `source_model` 不能重复。
- `llm_config.keys` 非必填，默认值为空数组 `[]`；若非空：
  - 每个元素 `name` 必填，且必须对应 provider `keys` 中存在的 name；
  - 同一 `keys` 数组内 `name` 唯一；
  - 每个元素 `weight` ∈ `[0,100]`；
  - 所有 Key 的 `weight` 之和必须等于 `100`。
- `llm_config.key_policy` 若传入：
  - `strategy` 仅允许 `weighted_random`；
  - `max_retries` 须为 `>=0` 的整数；
  - `retry_backoff_initial`、`retry_backoff_max` 须为 `>=0` 的整数，且 `retry_backoff_max >= retry_backoff_initial`。
- `llm_config.key_affinity` 若传入：
  - `enabled` 必须为 bool；
  - `ttl` 须为 `>0` 的整数；
  - `redis_prefix` 若传入须非空；
  - `penalty_enable` 必须为 bool。
- `llm_config.match_prefix` / `strip_prefix`：
  - `strip_prefix=true` 时，`match_prefix` 必填且非空；
  - `match_prefix` 若传入，必须以 `/` 结尾。
- `llm_config.model_table` 不在 OpenAPI `/clusters` 端点中展示，调用方传入时忽略或返回 `422`；该字段由 InnerAPI 根据 `provider` 自动填充后下发给 BFE。
- `balance_mode` 非必填，默认值为 `WRR`；有效枚举：`WRR`（BFE 本地加权轮询）、`EPP`（由 EPP 调度器接管后端选择）；`balance_mode` 是 EPP 模式的唯一判定来源。
- `epp_config` 条件必填：`balance_mode=EPP` 时必填且通过字段校验（简化字段校验：`load_profile`/`affinity` 枚举、`kv_cache_utilization_max` ∈ `(0,1]`、`waiting_queue_max`/`running_requests_max` ≥ 0、`metrics_staleness_threshold_ms` > 0、`flow_control` 秒数取值范围及 `enable_eviction=false`，全部 api 侧强制，不合法拒绝提交）；`balance_mode=WRR` 时可选——传入则保留并做格式校验，但不编译、不导出（休眠）。
- `balance_mode` 更新规则：允许 `WRR → EPP`（须同时提供合法 `epp_config`）；`EPP → WRR` 仅改 `balance_mode` 即可，`epp_config` 与分配记录保留（休眠，不编译/不导出），再切回 `EPP` 时原配置与原分配继续生效（分配如已悬空由 `/epp-pool` 变更时的自动修复处理）。**任一写入中 `epp_config` 非空即须通过字段校验，与 `balance_mode` 无关**（含 `EPP → WRR` 时随请求一并携带或休眠保留的配置）。
- 生效语义：`epp_config` 仅在 `balance_mode=EPP` 时生效；停用 EPP 调度 = 将 `balance_mode` 改回 `WRR`（无需清空 `epp_config`）。
- `basic`、`sticky_sessions`、`passive_health_check` 若未传则使用 AI 网关场景默认值。
- `basic.protocol` 有效值为 `http`、`https`。
- `basic.connection.max_idle_conn_per_rs` 须为 >=0 的整数。
- `basic.retries.max_retry_in_cluster` 须为 >=0 的整数。
- `basic.buffers.req_write_buffer_size` 须为 >0 的整数。
- `basic.timeouts.*` 各项均须为 >0 的整数。
- `sticky_sessions.hash_strategy` 有效值为 `CLIENT_IP_ONLY`、`CLIENT_ID_ONLY`、`CLIENT_ID_PREFERED`。
- `passive_health_check.failnum`、`interval` 须为 >=0 的整数；`uri` 非空且须以 `/` 开头；`statuscode` 须为 `0` 或 `100-599`。
- `sub_clusters` 与 `scheduler` 为系统内部自动生成数据，不再对外暴露；每个集群只包含一个子集群，调度设置固定为 `GSLB_BLACKHOLE=0`。
- `llm_config.enable` 字段已移除，设置 `llm_config` 时默认开启 AI 网关能力。
- 删除集群时自动级联清理关联的实例池和子集群。

## 2. 接口清单

### 2.1 创建集群

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 创建集群（通过引用 provider 自动创建实例池 + 子集群 + 绑定） | - |
| 端点 | /clusters | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| name| string |  集群名 | Y | 集群名必须全局唯一 | 必填；类型为 [ClusterName](./00-common.md#15-集群名称clustername) |
| description| string |  集群描述信息| N |  | 非必填；若传入，长度 0-256 字符；不能包含控制字符 |
| basic| object |  基本参数| N | AI网关场景默认推荐值：protocol=https；connection.max_idle_conn_per_rs=0、cancel_on_client_close=false；retries.max_retry_in_cluster=2；buffers.req_write_buffer_size=512；timeouts.timeout_conn_serv=50000、timeout_response_header=50000、timeout_readbody_client=30000、timeout_read_client_again=30000、timeout_write_client=60000 | 非必填；未传时使用 AI 网关场景默认值；子字段合法性见 1 中各表 |
| sticky_sessions| object |  会话保持| N | AI网关场景默认推荐值：enabled=false；若开启，hash_strategy=CLIENT_ID_ONLY、hash_header为空 | 非必填；未传时使用默认值；子字段合法性见 1 中各表 |
| passive_health_check| object |  被动健康检查| N | AI网关场景默认推荐值：failnum=3、interval=1000ms、host为空（使用provider首个实例addr）、uri="/"、statuscode=0 | 非必填；未传时使用默认值；子字段合法性见 1 中各表 |
| llm_config| object |  AI LLM服务配置| Y | 见 1 数据模型中表：LLM配置 | 必填；`provider` 必填且引用已存在 provider；`models` 至少1个非空元素且不能重复；子字段合法性见 1 中各表 |
| balance_mode| string |  集群均衡模式| N | `WRR`：BFE 本地加权轮询；`EPP`：由 EPP 调度器接管后端选择 | 非必填；默认值为 `WRR`；有效枚举：`WRR`、`EPP` |
| epp_config| object |  EPP 调度配置（简化用户形态：实例负载画像 + 准入水位 + 排队）| N | 见 1 数据模型中表：EPP调度配置 | 条件必填：`balance_mode=EPP` 时必填且通过字段校验；`balance_mode=WRR` 时可选（传入则保留并做格式校验，不生效）；子字段合法性见 1 中各表 |

**HTTP BODY参数示例**

```json
{
    "name": "my-cluster",
    "description": "示例集群",
    "basic": {
        "protocol": "http",
        "connection": {
            "max_idle_conn_per_rs": 0,
            "cancel_on_client_close": false
        },
        "retries": {
            "max_retry_in_cluster": 2
        },
        "buffers": {
            "req_write_buffer_size": 512
        },
        "timeouts": {
            "timeout_conn_serv": 50000,
            "timeout_response_header": 50000,
            "timeout_readbody_client": 30000,
            "timeout_read_client_again": 30000,
            "timeout_write_client": 60000
        }
    },
    "sticky_sessions": {
        "enabled": false,
        "hash_strategy": "CLIENT_IP_ONLY",
        "hash_header": ""
    },
    "passive_health_check": {
        "interval": 1000,
        "failnum": 3,
        "host": "",
        "uri": "/",
        "statuscode": 0
    },
    "llm_config": {
        "models": [
            "deepseek-chat",
            "deepseek-coder"
        ],
        "model_mappings": [
            {
                "source_model": "gpt-4",
                "target_model": "deepseek-chat"
            }
        ],
        "keys": [
            {
                "name": "key-prod-01",
                "weight": 70
            },
            {
                "name": "key-prod-02",
                "weight": 30
            }
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 3,
            "retry_backoff_initial": 500,
            "retry_backoff_max": 5000
        },
        "provider": "deepseek"
    }
}
```

**HTTP BODY参数示例（不配置 API-Key）**

```json
{
    "name": "internal-cluster",
    "description": "内部集群，无需 API-Key",
    "llm_config": {
        "models": ["deepseek-chat"],
        "model_mappings": [],
        "keys": [],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 0,
            "retry_backoff_initial": 500,
            "retry_backoff_max": 5000
        },
        "provider": "deepseek"
    }
}
```

**HTTP BODY参数示例（EPP 模式集群）**

```json
{
    "name": "cluster-a",
    "description": "EPP 调度集群",
    "llm_config": { "provider": "p1", "models": ["m1"] },
    "balance_mode": "EPP",
    "epp_config": {
        "load_profile": "balanced",
        "affinity": "medium",
        "prefix_cache_affinity": true,
        "session_affinity_enabled": true,
        "session_affinity_header": "x-session-id",
        "kv_cache_utilization_max": 0.9,
        "waiting_queue_max": 0,
        "running_requests_max": 0,
        "fallback_on_empty": false,
        "metrics_staleness_threshold_ms": 200,
        "flow_control": {
            "max_requests": 1000,
            "queue_ttl": 30,
            "no_endpoint_queue_ttl": 600,
            "enable_eviction": false
        }
    }
}
```

**执行逻辑**

创建集群时，系统自动执行以下步骤：

1. 校验请求参数（`name` 必填；`llm_config` 必填，且 `provider` 引用已存在的 provider；`llm_config.models` 为 provider `models` 的子集；`llm_config.keys` 引用 provider 中存在的 key name 等）
2. 若 `llm_config` 不为 nil，内部自动设置 `enable = true`
3. 创建集群
4. 根据 `llm_config.provider` 查找对应 provider，读取其 `instance_pool`，自动创建实例池（名称格式：`{product_name}.{cluster_name}`）
5. 自动创建子集群（名称：`{cluster_name}`，绑定实例池）
6. 自动绑定子集群到集群
7. 若 `balance_mode=EPP`：该集群的后端选择与流量调度由 EPP 调度器接管（实例分配见 [epp-assignments.md](./epp-assignments.md)）

**返回数据（Data内容）**

| 参数名 | 类型 |参数含义 |
| - | -  | - |
| name | string | 集群名 |
| description | string | 集群描述信息 |
| llm_config | object | LLM 配置 |
| basic | object | 基本参数 |
| sticky_sessions | object | 会话保持 |
| passive_health_check | object | 被动健康检查 |
| balance_mode | string | 集群均衡模式；默认 `WRR` |
| epp_config | object \| null | EPP 调度配置（简化用户形态：实例负载画像 + 准入水位 + 排队）；未配置过时为 `null`。**原样返回存储值**：`balance_mode=WRR` 时若创建/更新中传入过 `epp_config`，仍原样返回（休眠保留、不生效），切回 `EPP` 后继续生效 |

> **注意**：返回数据中不再包含 `instance_pool`；实例池信息通过 `llm_config.provider` 关联到 provider 获取。

**成功返回示例**
```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "my-cluster",
        "description": "示例集群",
        "llm_config": { "...": "..." },
        "basic": { "...": "..." },
        "sticky_sessions": { "...": "..." },
        "passive_health_check": { "...": "..." },
        "balance_mode": "WRR",
        "epp_config": null
    }
}
```

### 2.2 集群列表

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 所有集群列表 | - |
| 端点 | /clusters | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| provider | string | 按 Provider 过滤 | N | - | 须为已存在的 provider name |

**返回数据（Data内容）**

数组，单元素同创建接口。

### 2.3 集群详情

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 单个集群详情 | - |
| 端点 | /clusters/{cluster_name} | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| cluster_name | string | 集群名字 | Y | - | 必填；类型为 [ClusterName](./00-common.md#15-集群名称clustername)；必须引用已存在的集群 |

**返回数据（Data内容）**

同创建接口。

### 2.4 更新集群基本配置

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 更新集群基本信息 | 可编辑描述信息、Basic配置段、sticky_sessions配置段、healthcheck配置段、llm_config |
| 端点 | /clusters/{cluster_name} | - |
| 版本 | v1 | - |
| method | PATCH | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| cluster_name | string | 集群名字 | Y | - | 必填；类型为 [ClusterName](./00-common.md#15-集群名称clustername)；必须引用已存在的集群 |

**输入参数（Body）**

可修改字段含义同创建接口，但**输入参数不包括 `name`，即不能修改 cluster 的 name**（名称由 URI 中的 `cluster_name` 指定）。若请求体中仍包含 `name`，返回 422。不支持直接修改 `instance_pool`；如需调整后端实例，请更新对应 provider 的 `instance_pool`。

`balance_mode` / `epp_config` 更新规则：

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| balance_mode | string | 集群均衡模式 | N | 允许 `WRR → EPP`（须同时提供合法 `epp_config`）与 `EPP → WRR`（`epp_config` 与分配记录保留休眠） | 非必填；有效枚举：`WRR`、`EPP` |
| epp_config | object | EPP 调度配置（简化用户形态：实例负载画像 + 准入水位 + 排队） | N | `WRR → EPP` 时须同时提供合法 `epp_config`；`EPP → WRR` 无需清空（保留休眠）；变更后即时 bump server_data_conf version | 条件必填与字段校验同创建接口；`balance_mode=WRR` 时可选（传入保留、不生效）；**非空一律做字段校验，与 balance_mode 无关** |

> **注意：**
> - `sub_clusters` 与 `scheduler` 为系统内部自动生成，更新时不支持手动修改。
> - `llm_config.keys` 作为数组，按**全量替换**处理，即调用方需传入完整的最新 Key 引用列表。这与当前 `model_mappings` 的 PATCH 语义保持一致。

**HTTP BODY参数示例**

```json
{
    "description": "更新后的集群描述",
    "basic": {
        "protocol": "http",
        "connection": {
            "max_idle_conn_per_rs": 0,
            "cancel_on_client_close": false
        },
        "retries": {
            "max_retry_in_cluster": 2
        },
        "buffers": {
            "req_write_buffer_size": 512
        },
        "timeouts": {
            "timeout_conn_serv": 50000,
            "timeout_response_header": 50000,
            "timeout_readbody_client": 30000,
            "timeout_read_client_again": 30000,
            "timeout_write_client": 60000
        }
    },
    "sticky_sessions": {
        "enabled": false,
        "hash_strategy": "CLIENT_IP_ONLY",
        "hash_header": ""
    },
    "passive_health_check": {
        "interval": 1000,
        "failnum": 3,
        "host": "",
        "uri": "/",
        "statuscode": 0
    },
    "llm_config": {
        "models": ["deepseek-chat"],
        "keys": [
            {
                "name": "key-prod-01",
                "weight": 50
            },
            {
                "name": "key-prod-02",
                "weight": 50
            }
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 3,
            "retry_backoff_initial": 500,
            "retry_backoff_max": 5000
        },
        "provider": "deepseek"
    }
}
```

**返回数据（Data内容）**

同创建接口。

### 2.5 删除集群

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 删除集群（自动级联清理关联的实例池和子集群） | - |
| 端点 | /clusters/{cluster_name} | - |
| 版本 | v1 | - |
| method | DELETE | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| cluster_name | string | 集群名字 | Y | - | 必填；类型为 [ClusterName](./00-common.md#15-集群名称clustername)；必须引用已存在的集群 |

**执行逻辑**

删除集群时，系统先检查该集群是否被 **AI 路由规则**（global / entity / api-key 级别）引用；若被引用，删除失败。

通过引用检查后，系统自动执行以下级联清理：
1. 解绑集群关联的子集群
2. 删除子集群
3. 删除子集群关联的实例池
4. 删除集群

**返回数据（Data内容）**

同创建接口。

---
