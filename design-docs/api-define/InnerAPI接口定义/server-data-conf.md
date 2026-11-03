# server_data_conf 接口

## 1. 接口信息

| 项目 | 值 | 说明 |
|------|------|------|
| 含义 | 导出 BFE 路由/域名/集群综合配置 | 供 BFE `tls_conf/server_data_conf` 使用，包含 HostTable、RouteTable、ClusterConf 三部分 |
| 端点 | `/configs/tls_conf/server_data_conf` | - |
| Method | GET | - |
| 鉴权 | `FeatureRoute + ActionExport` | - |

## 2. 请求参数

**Query 参数**

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| version | string | 否 | 上次返回的版本号，用于增量同步 | 可选；无强制格式/长度校验；为空或未传时按首次拉取处理 |

**请求示例**

```shell
curl -X GET "http://api-server:port/inner-api/v1/configs/tls_conf/server_data_conf?version=00010101000000" \
  -H "Authorization:Token TOKEN_STRING"
```

## 3. 返回数据结构

### 3.1 顶层结构

与 BFE 动态配置文件 `server_data_conf` 格式保持一致：

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "Version": "00010101000000",
        "HostTable": {
            "Version": "00010101000000",
            "DefaultProduct": "AI_product",
            "Hosts": {
                "host-tag-1": ["api.example.com"]
            },
            "HostTags": {
                "AI_product": ["host-tag-1"]
            }
        },
        "RouteTable": {
            "Version": "00010101000000",
            "BasicRule": {},
            "ProductRule": {
                "AI_product": [
                    {
                        "Cond": "req_host_in(\"api.example.com\")",
                        "ClusterName": "my-cluster"
                    }
                ]
            }
        },
        "ClusterConf": {
            "Version": "00010101000000",
            "Config": {
                "my-cluster": {
                    "BackendConf": {
                        "Protocol": "http",
                        "TimeoutConnSrv": 50000,
                        "TimeoutResponseHeader": 50000,
                        "MaxIdleConnsPerHost": 0,
                        "MaxConnsPerHost": 0,
                        "CancelOnClientClose": false
                    },
                    "CheckConf": {
                        "Schem": "http",
                        "Uri": "/",
                        "Host": "",
                        "HostType": "",
                        "StatusCode": 0,
                        "FailNum": 3,
                        "CheckInterval": 1000
                    },
                    "GslbBasic": {
                        "CrossRetry": 0,
                        "RetryMax": 2,
                        "HashConf": {
                            "HashStrategy": 0,
                            "HashHeader": "Cookie:USERID",
                            "SessionSticky": false
                        },
                        "BalanceMode": "WRR",
                        "EPPAddr": null
                    },
                    "ClusterBasic": {
                        "TimeoutReadClient": 30000,
                        "TimeoutWriteClient": 60000,
                        "TimeoutReadClientAgain": 30000,
                        "ReqWriteBufferSize": 512,
                        "ReqFlushInterval": 0,
                        "ResFlushInterval": 0,
                        "CancelOnClientClose": false,
                        "DisableHostHeader": false,
                        "DisableHealthCheck": false
                    },
                    "HTTPSConf": null
                }
            }
        }
    },
    "WorkMode": "ModeNormal"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| Version | string | 配置版本号 |
| HostTable | object | BFE 域名路由表，映射 host-tag、product、hostname 关系 |
| RouteTable | object | BFE 路由规则表，包含 BasicRule 与 ProductRule |
| ClusterConf | object | BFE 集群配置，key 为集群名称 |

### 3.2 ClusterConf 中 AI 相关字段

当 OpenAPI 的集群配置了 `llm_config` 时，导出后的集群配置（`ClusterConf.Config.<cluster_name>`）中会包含 `AIConf`：

```json
{
    "ClusterConf": {
        "Version": "00010101000000",
        "Config": {
            "deepseek-cluster": {
                "BackendConf": { ... },
                "CheckConf": { ... },
                "GslbBasic": { ... },
                "ClusterBasic": { ... },
                "AIConf": {
                    "Type": 0,
                    "ModelMapping": {
                        "gpt-4": "deepseek-v3"
                    },
                    "Provider": "deepseek",
                    "Keys": [
                        {
                            "Name": "key-primary",
                            "Key": "sk-aaaaaaaaaaaa",
                            "Weight": 70
                        },
                        {
                            "Name": "key-secondary",
                            "Key": "sk-bbbbbbbbbbbb",
                            "Weight": 30
                        }
                    ],
                    "KeyPolicy": {
                        "Strategy": "weighted_random",
                        "MaxRetries": 3,
                        "RetryBackoffInitial": 500,
                        "RetryBackoffMax": 5000,
                        "SessionAffinity": true,
                        "SessionAffinityTTL": 600,
                        "SessionAffinityRedisPrefix": "bfe:ai:key_affinity",
                        "SessionAffinityPenaltyEnable": true
                    },
                    "MatchPrefix": "deepseek/",
                    "StripPrefix": true,
                    "ModelProtocols": ["openai"],
                    "EnsureReasoningContent": ["openai"],
                    "NormalizeUpstreamError": {
                        "Enabled": true,
                        "StreamEnabled": false,
                        "UnrecognizedAction": "passthrough",
                        "MaxBodyBytes": 65536,
                        "RedactSecrets": true
                    },
                    "ModelTable": {
                        "Currency": "RMB",
                        "TimeZone": "Asia/Shanghai",
                        "Tiers": [
                            {
                                "Name": "peak",
                                "TimeRanges": [
                                    { "Weekdays": [1, 2, 3, 4, 5], "Start": "09:00", "End": "12:00" },
                                    { "Weekdays": [1, 2, 3, 4, 5], "Start": "14:00", "End": "18:00" }
                                ]
                            }
                        ],
                        "Models": [
                            {
                                "Provider": "deepseek",
                                "Model": "deepseek-v3",
                                "BaseModel": "deepseek-v3",
                                "Mode": "chat",
                                "Capabilities": ["chat", "reasoning", "tools"],
                                "SupportedParameters": ["temperature", "max_tokens"],
                                "Limits": {
                                    "context_window": 128000,
                                    "max_input_tokens": 128000,
                                    "max_output_tokens": 8192
                                },
                                "Prices": {
                                    "input_cost_per_token": 0.000002,
                                    "output_cost_per_token": 0.000008,
                                    "cache_read_input_token_cost": 0.0000005
                                },
                                "TierPrices": {
                                    "peak": {
                                        "input_cost_per_token": 0.000004,
                                        "output_cost_per_token": 0.000016,
                                        "cache_read_input_token_cost": 0.000001
                                    }
                                }
                            }
                        ]
                    }
                }
            }
        }
    }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| Type | int | 固定为 0，保留字段 |
| ModelMapping | object | 模型名称映射，key 为请求模型名，value 为后端实际模型名 |
| Provider | string | 对应 OpenAPI `llm_config.provider`；默认空字符串 |
| Keys | array | API-Key 列表；为空数组时表示该 cluster 不配置 API-Key |
| Keys[].Name | string | Key 名称/标识（必填） |
| Keys[].Key | string | API-Key 值；**`[Security].EncryptExports=true` 时为 `enc$v1$` 字段级密文**（同 mod-api-key 信封规范），无此前缀按明文直通 |
| Keys[].Weight | int | 权重，范围 `[0,100]` |
| KeyPolicy | object | Key 路由策略 |
| KeyPolicy.Strategy | string | 本版仅支持 `weighted_random` |
| KeyPolicy.MaxRetries | int | 请求内总额外重试次数 |
| KeyPolicy.RetryBackoffInitial | int | 初始退避时间，单位毫秒 |
| KeyPolicy.RetryBackoffMax | int | 最大退避时间，单位毫秒 |
| KeyPolicy.SessionAffinity | bool | 是否开启会话级 Key 亲和性；对应 OpenAPI `llm_config.key_affinity.enabled`，默认 `true` |
| KeyPolicy.SessionAffinityTTL | int | 绑定空闲超时时间，单位秒；对应 OpenAPI `llm_config.key_affinity.ttl`，默认 `600`；命中后 BFE 会刷新 TTL |
| KeyPolicy.SessionAffinityRedisPrefix | string | Redis 绑定键前缀；对应 OpenAPI `llm_config.key_affinity.redis_prefix`，默认 `"bfe:ai:key_affinity"` |
| KeyPolicy.SessionAffinityPenaltyEnable | bool | 是否开启 Key 惩罚；对应 OpenAPI `llm_config.key_affinity.penalty_enable`，默认 `true` |
| MatchPrefix | string | 需要匹配的 provider/model 前缀；对应 OpenAPI `llm_config.match_prefix` |
| StripPrefix | bool | 是否裁剪 `MatchPrefix` 前缀；对应 OpenAPI `llm_config.strip_prefix` |
| ModelProtocols | []string | 该集群所属 provider 支持的模型访问协议；来源为 OpenAPI `/providers` 的 `model_protocols`。枚举值如 `openai`、`anthropic`；为空数组时 BFE 兜底为仅支持 `openai` |
| EnsureReasoningContent | []string | 需要对 assistant 消息补齐 `reasoning_content` 的协议列表；对应 OpenAPI `llm_config.ensure_reasoning_content`。**未配置时该字段为 `null`**（导出结构体不带 `omitempty`，BFE 反序列化为 nil = 关闭，请求逐字节透传）。取值仅 `openai`、`anthropic`，且须 ⊆ 本结构体的 `ModelProtocols`（为空按 `["openai"]`）；`gemini` 无注入落点，控制面与 BFE 加载期均拒绝 |
| NormalizeUpstreamError | object | 上游错误体归一配置；对应 OpenAPI `llm_config.normalize_upstream_error`；**未配置时整个字段不下发**（BFE 关闭归一，历史透传行为）；缺省字段不在导出中合成默认值，由 BFE 加载期 `Effective()` 合成（单一默认值来源） |
| NormalizeUpstreamError.Enabled | bool | 非流式归一开关；对应 `normalize_upstream_error.enabled`，默认 `false` |
| NormalizeUpstreamError.StreamEnabled | bool | 流式（SSE）归一开关；对应 `normalize_upstream_error.stream_enabled`，默认 `false` |
| NormalizeUpstreamError.UnrecognizedAction | string | 未识别错误体处理；对应 `normalize_upstream_error.unrecognized_action`；`""`/`passthrough`（默认）/ `rewrite_generic` |
| NormalizeUpstreamError.MaxBodyBytes | int64 | 错误响应体读取上限（字节）；对应 `normalize_upstream_error.max_body_bytes`；`0`（默认）= BFE 按 65536 处理；最大 4194304 |
| NormalizeUpstreamError.RedactSecrets | *bool | 凭证脱敏开关；对应 `normalize_upstream_error.redact_secrets`；未配置（nil）= BFE 按 `true` 处理；显式 `false` 关闭。**指针类型以区分"未配置"与"显式 false"** |
| ModelTable | object | 该 cluster 的成本定价表 |
| ModelTable.Currency | string | 价格货币；固定为 `"RMB"` |
| ModelTable.TimeZone | string | 计算时段所使用的时区；默认 `"Asia/Shanghai"` |
| ModelTable.Tiers | array | 时段 tier 定义列表；为空时按固定价格处理 |
| ModelTable.Tiers[].Name | string | Tier 名称；**初期只支持 `"peak"`** |
| ModelTable.Tiers[].TimeRanges | array | 时段范围列表；命中任意一个即属于该 tier |
| ModelTable.Tiers[].TimeRanges[].Weekdays | []int | 星期几；0=周日，1=周一，...，6=周六；为空表示每天 |
| ModelTable.Tiers[].TimeRanges[].Start | string | 开始时间，格式 `HH:MM` |
| ModelTable.Tiers[].TimeRanges[].End | string | 结束时间，格式 `HH:MM`；采用左闭右开语义 |
| ModelTable.Models | array | 模型定价条目列表 |
| ModelTable.Models[].Provider | string | Provider 名 |
| ModelTable.Models[].Model | string | 模型名，用于匹配请求中的 target_model |
| ModelTable.Models[].BaseModel | string | 归一化模型名 |
| ModelTable.Models[].Mode | string | 请求模式，默认 `"chat"`；枚举值同 OpenAPI `model_prices.mode` |
| ModelTable.Models[].Capabilities | []string | 能力列表；枚举值同 OpenAPI `model_prices.capabilities` |
| ModelTable.Models[].SupportedParameters | []string | 支持的请求参数列表；枚举值同 OpenAPI `model_prices.supported_parameters` |
| ModelTable.Models[].Limits | object | 限制对象；键名枚举值同 OpenAPI `model_prices.limits` |
| ModelTable.Models[].Prices | object | 默认价格对象；键名枚举值同 OpenAPI `model_prices.prices`；未命中 tier 时作为 fallback |
| ModelTable.Models[].TierPrices | object | 分时段价格对象；key 为 tier name，value 为价格对象；键名枚举值同 OpenAPI `model_prices.prices` |

> **说明**：`ModelTable` 由 InnerAPI 根据 `cluster.llm_config.provider` 查询 `/providers` 的 `time_zone` / `tiers` 与 `/model-prices` 的 `prices` / `tier_prices` 拼接后自动填充，不在 OpenAPI `/clusters` 端点中展示。`Currency` 固定为 `RMB`。

### 3.3 GslbBasic.EPPAddr 语义（分配组实例地址）

`GslbBasic.EPPAddr` 为分配的实例组实例地址列表，仅 `BalanceMode=EPP` 时有值（`BalanceMode=WRR` 时为 `null`）。当前单实例部署阶段**恒为单元素列表**（每组恰 1 实例）；bfe 侧按通用地址列表消费，failover 机制为未来多实例扩容预留。

- 由 `epp_assignments` 分配驱动生成（见 OpenAPI 接口定义 [epp-assignments.md](../OpenAPI接口定义/epp-assignments.md)）：按 cluster 的分配记录取分配的组，组内实例地址由 `epp_instances` 的 host/port 经 `net.JoinHostPort` 拼接为 `host:port`（IPv6 自动加括号）。
- EPP 模式 cluster 无有效分配记录时**降级导出**：该 cluster `BalanceMode` 置为 `WRR`、不生成 `EPPAddr`，同时 ai-gateway-api 输出 error 级日志（含 cluster 名与原因）；单 cluster 降级不阻塞整份 server_data_conf 下发，分配恢复后下轮导出自动回到 `EPP`。**降级时不导出任何 EPP 运行时字段**（见 §3.4）。
- 分配变更（cluster 进入 EPP 模式自动分配、`/epp-pool` 变更触发悬空修复、`PUT /api/v1/epp-assignments/{cluster}` 手工覆写）即时生效，下次导出自然带出。

### 3.4 GslbBasic EPP 运行时字段（EPPCheck / EPPTimeout / EPPTLS / EPPBreaker）

当 cluster `balance_mode=EPP` 且分配有效时，`ClusterConf.Config.<cluster>.GslbBasic` 会携带以下 EPP 运行时字段（来源为 OpenAPI `/clusters` 的 `epp_runtime`；固定值由 api 注入）：

```json
{
    "GslbBasic": {
        "BalanceMode": "EPP",
        "EPPAddr": ["10.0.0.1:8443"],
        "EPPTLS": { "Plaintext": true },
        "EPPCheck": {
            "Disabled": false,
            "CheckInterval": "2s",
            "FailThreshold": 3,
            "Cooldown": "45s",
            "SuccessThreshold": 2
        },
        "EPPTimeout": { "Connect": "500ms", "Call": "3s" },
        "EPPBreaker": { "Disabled": true }
    }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `EPPTLS` | object | **固定** `{ "Plaintext": true }`（统一明文接入 EPP）。取代 BFE legacy `{Insecure:true}` 默认，消除 BFE warning 并使 EPP 明文可达。`Insecure`/`CAFile` 恒为 false/空。 |
| `EPPCheck` | object | 来源 OpenAPI `epp_runtime.check`；**未配置 `epp_runtime` 时不下发**，由 BFE 加载期补默认 |
| `EPPCheck.Disabled` | bool | `check.disabled`，默认 `false` |
| `EPPCheck.CheckInterval` | string | `check.check_interval`，默认 `2s` |
| `EPPCheck.FailThreshold` | int | `check.fail_threshold`，默认 `3` |
| `EPPCheck.Cooldown` | string | `check.cooldown`，默认 `45s` |
| `EPPCheck.SuccessThreshold` | int | `check.success_threshold`，默认 `2` |
| `EPPTimeout` | object | 来源 OpenAPI `epp_runtime.timeout`；**未配置 `epp_runtime` 时不下发**，由 BFE 加载期补默认 |
| `EPPTimeout.Connect` | string | `timeout.connect`，默认 `500ms` |
| `EPPTimeout.Call` | string | `timeout.call`，默认 `3s` |
| `EPPBreaker` | object | **固定** `{ "Disabled": true }`（关闭熔断）；`WindowSize`/`MinVolume`/`ErrorRatePercent`/`OpenTimeout` 不下发 |

> **说明**：
> - 上述字段仅在 `BalanceMode=EPP`（分配有效）时输出。**未配置 `epp_runtime` 的 EPP cluster 也会输出固定 `EPPTLS` 与 `EPPBreaker`**（首次生效会产生一次 version bump / reload），但 `EPPCheck`/`EPPTimeout` 不下发。
> - `BalanceMode=WRR`（含 EPP 分配缺失降级）时**不输出**任何 EPP 运行时字段。
> - `EPPTLS` / `EPPBreaker` 由控制面固定注入，OpenAPI 不接受用户配置（见 [clusters.md](../OpenAPI接口定义/clusters.md) 表：EPP运行时配置）。

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
