# /providers

## 1. 数据模型

```json
{
    "name": "deepseek",
    "description": "DeepSeek 官方 API",
    "model_endpoint": {
        "schema": "https",
        "uri": "/v1/models"
    },
    "models": ["deepseek-chat", "deepseek-coder"],
    "keys": [
        {
            "name": "key-primary",
            "key": "sk-aaaaaaaaaaaa"
        },
        {
            "name": "key-secondary",
            "key": "sk-bbbbbbbbbbbb"
        }
    ],
    "instance_source": "instance_pool",
    "instance_pool": [
        {
            "addr": "api.deepseek.com",
            "weight": 100,
            "port": 443
        }
    ],
    "k8s_pool_name": null,
    "model_protocols": ["openai"],
    "protocol_paths": {"openai": "/v1"},
    "time_zone": "Asia/Shanghai",
    "tiers": [
        {
            "name": "peak",
            "time_ranges": [
                { "weekdays": [1, 2, 3, 4, 5], "start": "09:00", "end": "12:00" },
                { "weekdays": [1, 2, 3, 4, 5], "start": "14:00", "end": "18:00" }
            ]
        }
    ],
    "create_time": 1716883200,
    "update_time": 1716883200
}
```

> 上例为 `instance_source=instance_pool`（默认）形态：`k8s_pool_name` 为 `null`。`instance_source=k8s_pool` 形态见下方[模式响应示例](#instance_sourcek8s_pool-模式响应示例)——此时 `instance_pool` 休眠保留（可为 `[]`），`k8s_pool_name` 由用户填写；响应中不返回任何 K8s 实例数据（实时实例经 `GET /open-api/v1/k8s_pools/{k8s_pool_name}` 查询）。

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 | 合法性条件 |
|------|------|------|----------|----------|
| `name` | string | Provider 唯一标识 | 全局唯一 | 必填；类型为 [ProviderName](./00-common.md#17-provider-名称providername)；合法命名参考 [ClusterName](./00-common.md#15-集群名称clustername) |
| `description` | string | Provider 描述信息 | - | 非必填；若传入，长度 0-256 字节（按 UTF-8 字节计）；不能包含控制字符 |
| `model_endpoint` | object | 模型发现端点 | 用于调用第三方 AI 模型提供商的模型列表接口 | 非必填；未设置时默认 `schema=https`、`uri=/v1/models`；具体字段见下方 表：Endpoint |
| `models` | []string | 该 provider 支持的模型列表 | - | 必填；至少 1 个元素；元素非空且不可重复 |
| `keys` | []ProviderKey | 该 provider 可用的 API Key 明文 | - | 非必填；默认空数组 `[]`；元素须满足 表：ProviderKey 结构 |
| `instance_source` | string | 实例供给方式 | 枚举：`instance_pool`、`k8s_pool` | 非必填；默认 `instance_pool`；`instance_pool` = 人维护 `instance_pool`；`k8s_pool` = 实例由 K8s 发现组件经 InnerAPI `/k8s_pools` 维护 |
| `instance_pool` | []Instance | Provider 对应的后端实例池 | 系统自动据此创建实例池和子集群 | `instance_source=instance_pool` 时必填：至少 1 个元素；同一 provider 内 `(addr, port)` 组合不能重复；至少有一个实例 `weight > 0`；`instance_source=k8s_pool` 时不参与有效池、不做成员数校验（休眠保留，切回 `instance_pool` 模式时自动恢复生效，作为人工兜底） |
| `k8s_pool_name` | string | 引用的 K8s 实例池名称 | 引用 `/k8s_pools` 中的条目 | 非必填；`instance_source=k8s_pool` 时必填；`instance_source=instance_pool` 时保留原值但不生效（休眠，切回 `k8s_pool` 模式时自动恢复生效）；普通名称校验（长度 1-128 字符，仅允许字母、数字、`_`、`-`、`.`，不能以 `.`、`-`、`_` 开头或结尾，不能包含空白字符）；与 K8s Service 名无耦合（Service→池名映射由发现组件自行约定）；引用的 pool 无需预先存在（池不存在 ≡ 零实例）；`k8s_pools` 是该实例集合的唯一事实源，provider 上不存储实例数据，消费方（导出/batch）按此名称实时解析 |
| `model_protocols` | []string | 支持的模型访问协议 | 枚举：`openai`、`anthropic`、`gemini` | 必填；至少 1 个元素；元素不可重复；枚举值见下方 |
| `protocol_paths` | map[string]string | 按协议的上游 API 基路径（该协议 SDK `base_url` 的 path 部分）；BFE 转发时将命中的标准端点改写到该基路径（openai 兼容带/不带 `/v1` 的客户端入口） | 键：`openai`、`anthropic` | 非必填；缺省 = 不改写（请求路径原样转发）；键必须是 `model_protocols` 已声明协议的子集；值须以 `/` 开头、不以 `/` 结尾、不含 `..`/`?`/`#`、长度 ≤ 128；语义与参考值见下方 |
| `time_zone` | string | 计算时段所使用的时区 | 用于 tier 价格匹配 | 非必填；默认 `Asia/Shanghai`；须为合法 IANA 时区名 |
| `tiers` | []PricingTier | 时段 tier 定义列表 | 描述该 provider 在什么时段属于哪个 tier | 非必填；元素须满足 表：PricingTier 结构；**初期 `name` 只支持 `peak`** |
| `create_time` | int64 | 创建时间 | - | 系统生成 |
| `update_time` | int64 | 更新时间 | - | 系统生成 |

**表：PricingTier 结构（`tiers` 元素）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| `name` | string | Tier 名称 | Y | 用于与 `model-prices` 的 `tier_prices` 关联 | 必填；**初期只支持 `peak`** |
| `time_ranges` | []TimeRange | 时段范围列表 | Y | 命中任意一个即属于该 tier | 必填；至少 1 个元素；元素须满足 表：TimeRange 结构 |

**表：TimeRange 结构（`time_ranges` 元素）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| `weekdays` | []int | 星期几 | N | 0=周日，1=周一，...，6=周六 | 为空表示每天；元素须在 0-6 之间 |
| `start` | string | 开始时间 | Y | 格式 `HH:MM` | 必填；`HH:MM` 格式 |
| `end` | string | 结束时间 | Y | 格式 `HH:MM` | 必填；`HH:MM` 格式；`end` 必须大于 `start`；跨午夜请拆成两段 |

> **说明**：
> - 同一 tier 内的多个 `time_ranges` 为"或"关系；`tiers` 列表按顺序匹配，命中第一个即停止。
> - `start` / `end` 采用左闭右开语义（`start <= cur < end`），跨午夜请拆成两段。
> - `time_zone` / `tiers` 可通过 `PUT /providers/{provider_name}/pricing-tiers` 接口单独维护，创建 provider 时无需传入。

**表：Endpoint（`model_endpoint`）**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| schema| string |  请求协议 | N |  取值为 http、https；**默认值为 https** | 非必填；默认值为 `https`；有效值 `http`、`https` |
| uri| string |  请求URI | N |  **默认值为 `/v1/models`** | 非必填；默认值为 `/v1/models`；非空；须以 `/` 开头 |

> **说明**：不再允许配置 `headers.Authorization`。系统根据 `model_protocols` 自动决定调用模型发现接口时使用的认证头风格（如 `openai` 用 `Authorization: Bearer`，`anthropic` 用 `x-api-key`，`gemini` 用 `x-goog-api-key`）。

**表：ProviderKey 结构（`keys` 元素）**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| name | string | Key 名称/标识 | Y | 用于日志、监控、运维识别；在 `/clusters` 中通过该 name 引用 | 必填；长度 1-128 字符；同一 provider 内唯一 |
| key | string | API-Key 值 | Y | 实际用于后端认证的密钥。**角色可见性**：系统管理员返回明文；系统管理员（只读）返回掩码值（长度>8：前4+`********`+后4；4<长度≤8：前2+`****`+后2；长度≤4：全掩码），掩码值不可用于鉴权 | 必填；非空；长度 1-512 字符 |

**表：Instance 结构（`instance_pool` 元素）**

| 参数名 | 类型 |参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | -  | - | - | - | - |
| addr | string | 实例地址 | Y | 无 DNS 时可填写 IP 地址 | 必填；类型为 [Hostname](./00-common.md#1-主机名hostname) |
| weight | int | 实例权重，范围 [0,100] | Y | | 必填；取值范围 [0,100]；`0` 表示该实例不接收流量 |
| port | int | 实例端口 | Y | | 必填；类型为 [Port](./00-common.md#3-网络端口port) |

**有效池（effective pool）**

provider 对下游（cluster 引用快照、实例池同步、配置导出、batch 出网）暴露的实例集合称为**有效池**：

```
有效池 = instance_source == "k8s_pool" ? 实时查询 k8s_pools[k8s_pool_name].instances : instance_pool
```

- `instance_source=instance_pool`：有效池即 `instance_pool`（现状行为）。
- `instance_source=k8s_pool`：有效池 = 消费时按 `k8s_pool_name` 实时查询 `k8s_pools`（`k8s_pools` 是唯一事实源，provider 上不存实例数据）；`instance_pool` 休眠保留但不生效。
- k8s 模式下没有任何镜像同步：`PUT/DELETE /k8s_pools` 只写 `k8s_pools` 单表，下一次导出/batch 即时反映；cluster 派生池在 k8s 模式下为空占位行（仅满足 schema 引用，不承载实例数据）。
- 有效池为空时（池不存在 / 池名为空 / 池为空），引用 cluster 以空条目下发，该 cluster 请求将返回 500（`BK_NO_BACKEND`），须配合告警使用；手工来源的有效池变化仍会同步更新引用 cluster 的派生实例池。

**`instance_source=k8s_pool` 模式响应示例**

```json
{
    "name": "svc-a-provider",
    "description": "K8s 服务发现的推理后端",
    "instance_source": "k8s_pool",
    "k8s_pool_name": "svc-a",
    "instance_pool": [],
    "models": ["deepseek-chat"],
    "model_protocols": ["openai"],
    "create_time": 1716883200,
    "update_time": 1716883200
}
```

> K8s pool 的实时实例列表可通过 `GET /open-api/v1/k8s_pools/{k8s_pool_name}` 查询（见 [k8s-pools.md](./k8s-pools.md)）；provider 详情页可按 `k8s_pool_name` 调用该接口展示实例。

**`model_protocols` 枚举**

| 枚举值 | 说明 |
|--------|------|
| `openai` | OpenAI 兼容协议（含大多数国产兼容平台） |
| `anthropic` | Anthropic Claude Messages API |
| `gemini` | Google Gemini API（认证头 `x-goog-api-key`） |

> 一个 provider 可同时支持多种协议（如聚合平台），但 `model_protocols` 至少包含一个。

**`protocol_paths` 语义与常见 provider 参考值**

- 配置值 = 该协议官方 SDK `base_url` 的 path 部分：`openai` 含 `/v1` 尾（OpenAI SDK 向 base_url 拼 `/chat/completions` 等）；`anthropic` 不含 `/v1`（Anthropic SDK 自行拼接 `/v1/messages`）。
- 改写规则：anthropic 请求 `/v1/messages`（及子路径）被改写为 `{anthropic}/v1/messages`；openai 请求命中标准端点（`/chat/completions`、`/completions`、`/embeddings`、`/models`、`/responses` 等）时，先剥离可选的 `/v1` 前缀再拼接到 base——`/v1/chat/completions` 与 `/chat/completions` 均改写为 `{openai}/chat/completions`，即客户端入口带不带 `/v1` 不影响最终上游路径（兼容 Trae 等直连 base_url 的 OpenAI 兼容客户端）。
- 未配置（或对应协议无条目）时请求路径原样转发；未命中 openai 标准端点的路径（provider 原生路径、自定义路径、`/v10/xxx`、`/v1beta/...`）永不改写——客户端以 provider 原生路径访问的透传模式不受影响。`gemini` 协议不支持路径改写（其原生路径即标准路径，透传已可用）。

| provider | `protocol_paths` 参考值 |
|----------|-------------------------|
| 百炼 DashScope | `{"openai": "/compatible-mode/v1", "anthropic": "/apps/anthropic"}` |
| Kimi 开放平台（api.moonshot.cn） | `{"openai": "/v1", "anthropic": "/anthropic"}` |
| Kimi Code 会员（api.kimi.com） | `{"openai": "/coding/v1", "anthropic": "/coding"}` |
| DeepSeek | `{"openai": "/v1", "anthropic": "/anthropic"}` |
| 火山方舟·按量 | `{"openai": "/api/v3", "anthropic": "/api/compatible"}` |
| 火山方舟·Coding Plan | `{"openai": "/api/coding/v3", "anthropic": "/api/coding"}` |

## 2. 接口清单

### 2.1 创建 Provider

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 创建 Provider | - |
| 端点 | /providers | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（Body）**

字段同 [1. 数据模型](#1-数据模型)，但请求体中无需传入 `create_time`、`update_time`。

**HTTP BODY 参数示例**

```json
{
    "name": "deepseek",
    "description": "DeepSeek 官方 API",
    "model_endpoint": {
        "schema": "https",
        "uri": "/v1/models"
    },
    "models": ["deepseek-chat", "deepseek-coder"],
    "keys": [
        {
            "name": "key-primary",
            "key": "sk-aaaaaaaaaaaa"
        },
        {
            "name": "key-secondary",
            "key": "sk-bbbbbbbbbbbb"
        }
    ],
    "instance_pool": [
        {
            "addr": "api.deepseek.com",
            "weight": 100,
            "port": 443
        }
    ],
    "model_protocols": ["openai"],
    "protocol_paths": {"openai": "/v1"}
}
```

**执行逻辑**

1. 校验 `name` 全局唯一、`instance_source` 合法（缺省 `instance_pool`）、`instance_pool` 合法、`model_protocols` 合法、`protocol_paths` 合法（键 ⊆ `model_protocols` 且取值 ∈ {openai, anthropic}，值符合路径格式）。
   - `instance_source=instance_pool`：`instance_pool` 必填且至少 1 个元素；`k8s_pool_name` 若传入则保留（休眠，不生效）。
   - `instance_source=k8s_pool`：`k8s_pool_name` 必填；`instance_pool` 不参与有效池、不做成员数校验（可传空）。
   - 任意模式：请求体携带已删除的 `k8s_instance_pool` 字段被静默忽略（按未知字段处理）。
2. 若未传 `model_endpoint`，使用默认值 `{schema: "https", uri: "/v1/models"}`。
3. 若未传 `keys`，默认空数组。
4. 若未传 `time_zone`，默认 `Asia/Shanghai`。
5. 若请求中携带 `tiers`，按 表：PricingTier 结构 校验；**初期只支持 `name="peak"`**。
6. 校验 `models` 必填：至少 1 个元素，元素非空且不可重复，通过后直接保存。如需借助模型发现工具（`/providers/tools/discover-models`，无状态接口）确定模型列表，调用方需先调用该工具，再在创建请求中携带其结果。
7. 写入 provider 记录，返回完整对象。

**返回数据（Data内容）**

字段同 [1. 数据模型](#1-数据模型)，包含系统生成的 `create_time`、`update_time`。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "deepseek",
        "description": "DeepSeek 官方 API",
        "model_endpoint": {
            "schema": "https",
            "uri": "/v1/models"
        },
        "models": ["deepseek-chat", "deepseek-coder"],
        "keys": [
            {"name": "key-primary", "key": "sk-aaaaaaaaaaaa"},
            {"name": "key-secondary", "key": "sk-bbbbbbbbbbbb"}
        ],
        "instance_source": "instance_pool",
        "instance_pool": [
            {"addr": "api.deepseek.com", "weight": 100, "port": 443}
        ],
        "model_protocols": ["openai"],
        "protocol_paths": {"openai": "/v1"},
        "time_zone": "Asia/Shanghai",
        "tiers": [
            {
                "name": "peak",
                "time_ranges": [
                    {"weekdays": [1, 2, 3, 4, 5], "start": "09:00", "end": "12:00"},
                    {"weekdays": [1, 2, 3, 4, 5], "start": "14:00", "end": "18:00"}
                ]
            }
        ],
        "create_time": 1716883200,
        "update_time": 1716883200
    }
}
```

### 2.2 Provider 列表

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 所有 Provider 列表 | - |
| 端点 | /providers | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

通用列表参数见 [00-common.md](./00-common.md#通用query参数列表接口)。

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| page | int | 页码 | N | 未传且 `page_size` 也未传时，返回全部记录 | 大于 0 |
| page_size | int | 每页条数 | N | 未传且 `page` 也未传时，返回全部记录；最大 1000 | 大于 0 |
| model_protocol | string | 按协议过滤 | N | - | 须为 `model_protocols` 枚举值 |

**返回数据（Data内容）**

```json
{
  "list": [...],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 100
  }
}
```

- 未携带 `page`/`page_size` 时，`list` 返回全部匹配记录，`pagination.page=1`，`pagination.page_size=total`；
- 携带分页参数时，按对应页码返回。

数组元素同创建接口。

### 2.3 Provider 详情

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 单个 Provider 详情 | - |
| 端点 | /providers/{provider_name} | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| provider_name | string | Provider 名字 | Y | - | 必填；类型为 [ProviderName](./00-common.md#17-provider-名称providername)；必须引用已存在的 provider |

**返回数据（Data内容）**

同创建接口，包含完整的 Provider 数据模型，其中包括：

- `time_zone`：计算时段所使用的时区（创建时未传则默认 `Asia/Shanghai`）。
- `tiers`：该 provider 的高峰/闲时 tier 定义列表（若未设置则为空数组 `[]`）。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "deepseek",
        "description": "DeepSeek 官方 API",
        "model_endpoint": { "schema": "https", "uri": "/v1/models" },
        "models": ["deepseek-v4-pro", "deepseek-v4-flash"],
        "keys": [...],
        "instance_pool": [...],
        "model_protocols": ["openai"],
        "protocol_paths": {"openai": "/v1"},
        "time_zone": "Asia/Shanghai",
        "tiers": [
            {
                "name": "peak",
                "time_ranges": [
                    { "weekdays": [1, 2, 3, 4, 5], "start": "09:00", "end": "12:00" },
                    { "weekdays": [1, 2, 3, 4, 5], "start": "14:00", "end": "18:00" }
                ]
            }
        ],
        "create_time": 1716883200,
        "update_time": 1716883200
    }
}
```

> 说明：若 provider 未通过 `PUT /providers/{provider_name}/pricing-tiers` 设置过高峰/闲时模板，则 `tiers` 返回空数组，`time_zone` 返回默认值 `Asia/Shanghai`。

### 2.4 更新 Provider

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 更新 Provider 基本信息 | 可编辑描述信息、模型端点、模型列表、Key、实例池、协议等 |
| 端点 | /providers/{provider_name} | - |
| 版本 | v1 | - |
| method | PATCH | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| provider_name | string | Provider 名字 | Y | - | 必填；类型为 [ProviderName](./00-common.md#17-provider-名称providername)；必须引用已存在的 provider |

**输入参数（Body）**

可修改字段含义同创建接口，但**输入参数不包括 `name`，即不能修改 provider 的 name**（名称由 URI 中的 `provider_name` 指定）。若请求体中仍包含 `name`，返回 422。若传入 `instance_pool` 字段（且 `instance_source=instance_pool`），系统会自动同步更新被引用该 provider 的所有 cluster 所生成的实例池。

> **实例供给方式（`instance_source` / `k8s_pool_name`）更新语义**：
> - 两字段均可通过 PATCH 修改，包括**模式切换**（`instance_pool` ↔ `k8s_pool`）；模式切换时清空/写入引用 cluster 的派生实例池快照以维持占位不变量（manual → k8s 清空旧手工快照；k8s → manual 写入新 `instance_pool` 快照）。
> - 模式切换时点校验：`instance_source=instance_pool` 要求 `instance_pool` 非空；`instance_source=k8s_pool` 要求 `k8s_pool_name` 非空。不满足返回 422。
> - `instance_source=k8s_pool` 时 PATCH `instance_pool` 仅作休眠保留（不参与有效池、不触发同步），切回 `instance_pool` 模式时自动恢复生效；PATCH `k8s_pool_name`（A→B）只改引用，不触发任何派生池同步，消费方实时读 B。
> - 字段已彻底删除：请求体携带 `k8s_instance_pool` 被静默忽略（响应同样不含该字段）；k8s 模式的实时实例经 `GET /open-api/v1/k8s_pools/{k8s_pool_name}` 查询。

> **注意**：本接口为**部分更新**语义——请求体中未提供的字段（`description`、`model_endpoint`、`models`、`keys`、`time_zone`、`tiers`、`instance_source`、`k8s_pool_name` 等）保持原值不变。
> - 通用约定：对可选的 map / 数组字段（`keys`、`tiers`、`protocol_paths` 等），省略与传 `null` 等价，均保留原值；显式传入空集合（`[]` / `{}`）按全量替换处理，即清空该字段（仍须通过对应字段校验）。
> - `keys` 作为数组，**显式提供时按全量替换**处理，即调用方需传入完整的最新 Key 列表；省略时保留原值。Key 的 `name` 删除/重命名会校验无 cluster 仍引用旧 name；若被引用，返回 `409 Conflict`。
> - `models` 作为数组，**显式提供时按全量替换**处理；省略时保留原值。删除 model 会校验无 cluster 仍引用该 model；若被引用，返回 `409 Conflict`。
> - `model_protocols` 作为数组，**显式提供时按全量替换**处理。收窄（移除协议）会校验无 cluster 的 `llm_config.ensure_reasoning_content` 仍引用被移除的协议；若被引用，返回 `409 Conflict`（消息含 provider 名、协议名与引用它的 cluster 名）。放宽不受限制。
> - `tiers`、`time_zone`、`model_endpoint`：提供即更新，省略保留原值。`time_zone` 取值须为合法时区名（如 `Asia/Shanghai`、`UTC`）。
> - `protocol_paths`：提供即全量替换，**省略或传 `null` 保留原值；清空（禁用路径改写）须显式传入 `"protocol_paths": {}`**。键必须是**更新后** `model_protocols` 已声明协议的子集——同时调整 `model_protocols` 与 `protocol_paths` 时，须在同一个请求中给出合法组合。

**HTTP BODY 参数示例**

```json
{
    "description": "更新后的描述",
    "models": ["deepseek-chat", "deepseek-coder", "deepseek-reasoner"],
    "keys": [
        {"name": "key-primary", "key": "sk-aaaaaaaaaaaa"},
        {"name": "key-secondary", "key": "sk-bbbbbbbbbbbb"},
        {"name": "key-tertiary", "key": "sk-cccccccccccc"}
    ],
    "instance_pool": [
        {"name": "backend-1", "addr": "api.deepseek.com", "weight": 100, "port": 443}
    ],
    "model_protocols": ["openai"],
    "protocol_paths": {"openai": "/v1"}
}
```

清空 `protocol_paths`（禁用路径改写，恢复为请求路径原样转发）的请求体示例——注意须显式传空对象 `{}`，省略该字段或传 `null` 均保留原值、不清空：

```json
{
    "protocol_paths": {}
}
```

> 提示：部分客户端/序列化库会把空 map 归一化为 `null`，导致"传了空对象却没清空"且接口返回 200 无报错；如遇此情况，请先抓取实际请求体确认发出的是 `{}` 而非 `null`。

**返回数据（Data内容）**

同创建接口。

### 2.5 删除 Provider

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 删除 Provider | - |
| 端点 | /providers/{provider_name} | - |
| 版本 | v1 | - |
| method | DELETE | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| provider_name | string | Provider 名字 | Y | - | 必填；类型为 [ProviderName](./00-common.md#17-provider-名称providername)；必须引用已存在的 provider |

**执行逻辑**

1. 校验该 provider 未被任何 `/clusters` 引用；若被引用，返回 `409 Conflict`。
2. 删除 provider。 `/model-prices` 中的同名 `provider` 不再作为阻塞条件。

**返回数据（Data内容）**

Data 为 null。

### 2.6 触发模型发现

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 触发模型发现，返回模型名列表 | - |
| 端点 | /providers/tools/discover-models | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| model_protocol | string | 模型访问协议 | Y | - | 必填；枚举值：`openai`、`anthropic`、`gemini` |
| schema | string | 请求协议 | Y | - | 必填；有效值 `http`、`https` |
| addr | string | 目标实例地址 | Y | - | 必填；类型为 [Hostname](./00-common.md#1-主机名hostname) |
| port | int | 目标实例端口 | Y | - | 必填；类型为 [Port](./00-common.md#3-网络端口port) |
| `uri` | string | 模型列表接口 URI | N | 为空时按协议取默认值：`openai`/`anthropic` 为 `/v1/models`，`gemini` 为 `/v1beta/models` | 非空时须以 `/` 开头 |
| `apikey` | string | 调用模型列表接口的 API Key | N | - | 非空时长度 1-512 字符 |

**执行逻辑**

1. 若 `uri` 为空，按协议取默认值（`openai`/`anthropic` 为 `/v1/models`，`gemini` 为 `/v1beta/models`）；构造请求 URL：`{schema}://{addr}:{port}{uri}`。
2. 若 `apikey` 非空，根据 `model_protocol` 生成认证头：
   - `openai`：`Authorization: Bearer {apikey}`
   - `anthropic`：`x-api-key: {apikey}`
   - `gemini`：`x-goog-api-key: {apikey}`
3. 携带认证头（若有）调用第三方模型列表接口。
4. 根据 `model_protocol` 选择对应的响应解析器（如 `openai`、`anthropic`、`gemini`），提取模型名列表。gemini 响应从 `models[].name` 提取并剥离 `models/` 前缀（如 `models/gemini-2.5-pro` → `gemini-2.5-pro`）。
5. 返回模型名列表。

> **说明**：本接口为无状态工具接口，不读写任何 Provider 资源；如需将发现结果回填到 Provider，调用方需再调用 `PATCH /providers/{provider_name}`。

**返回数据（Data内容）**

| 参数名 | 类型 | 参数含义 |
| - | -  | - |
| models | []string | 发现到的模型名列表 |

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "models": ["deepseek-chat", "deepseek-coder", "deepseek-reasoner"]
    }
}
```

### 2.7 获取所有 Provider 名称列表

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 获取所有 Provider 名称列表 | 用于需要全量 provider 名称的场景，如下拉选择、自动补全 |
| 端点 | /providers/actions/get-provider-names | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数**

无。

**执行逻辑**

1. 查询所有 provider 的 `name` 字段。
2. 返回按字典序升序排列的名称列表。

> **说明**：本接口不返回 Provider 其他字段，仅用于获取全量名称；详细数据仍通过 `GET /providers` 分页查询。

**返回数据（Data内容）**

| 参数名 | 类型 | 参数含义 |
| - | -  | - |
| names | []string | 所有 Provider 名称列表 |

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "names": ["anthropic", "deepseek", "openai"]
    }
}
```

### 2.8 设置 Provider 高峰/闲时模板

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 设置/更新指定 provider 的高峰/闲时模板 | 支持 JSON body 或 YAML 文件；**初期 tier name 只支持 `peak`** |
| 端点 | /providers/{provider_name}/pricing-tiers | - |
| 版本 | v1 | - |
| method | PUT | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| `provider_name` | string | Provider 名称 | Y | - | 必填；类型为 [ProviderName](./00-common.md#17-provider-名称providername)；必须引用已存在的 provider |

**输入参数（Body）**

两种提交方式：

1. **JSON 格式**（`Content-Type: application/json`）：

```json
{
    "time_zone": "Asia/Shanghai",
    "tiers": [
        {
            "name": "peak",
            "time_ranges": [
                { "weekdays": [1, 2, 3, 4, 5], "start": "09:00", "end": "12:00" },
                { "weekdays": [1, 2, 3, 4, 5], "start": "14:00", "end": "18:00" }
            ]
        }
    ]
}
```

2. **YAML 文件格式**（`Content-Type: text/yaml` 或 `multipart/form-data` 上传文件）：

```yaml
time_zone: "Asia/Shanghai"
tiers:
  - name: "peak"
    time_ranges:
      - weekdays: [1, 2, 3, 4, 5]
        start: "09:00"
        end: "12:00"
      - weekdays: [1, 2, 3, 4, 5]
        start: "14:00"
        end: "18:00"
```

**执行逻辑**

1. 校验 `provider_name` 对应的 provider 存在。
2. 校验 `time_zone` 为合法 IANA 时区名；为空时默认 `Asia/Shanghai`。
3. 校验 `tiers` 中每个 tier 包含非空 `name` 和至少一个 `time_range`；**初期 `name` 只支持 `peak`**。
4. 校验 `time_ranges` 中 `weekdays` 元素在 0-6 之间；`start` / `end` 格式为 `HH:MM`，且 `end` > `start`；同一 tier 内部 `time_ranges` 不得重叠。
5. 更新 provider 的 `time_zone` / `tiers` 字段，并刷新 `update_time`。
6. 返回更新后的完整 provider 记录。

**返回数据（Data内容）**

同 `GET /providers/{provider_name}`，包含基础信息 + `time_zone` / `tiers`。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "name": "deepseek",
        "description": "DeepSeek 官方 API",
        "model_endpoint": { "schema": "https", "uri": "/v1/models" },
        "models": ["deepseek-v4-pro", "deepseek-v4-flash"],
        "keys": [...],
        "instance_pool": [...],
        "model_protocols": ["openai"],
        "protocol_paths": {"openai": "/v1"},
        "time_zone": "Asia/Shanghai",
        "tiers": [
            {
                "name": "peak",
                "time_ranges": [
                    { "weekdays": [1, 2, 3, 4, 5], "start": "09:00", "end": "12:00" },
                    { "weekdays": [1, 2, 3, 4, 5], "start": "14:00", "end": "18:00" }
                ]
            }
        ],
        "create_time": 1716883200,
        "update_time": 1716883200
    }
}
```

## 3. 校验规则

1. `name` 必填，类型为 [ProviderName](./00-common.md#17-provider-名称providername)，全局唯一。
2. `description` 可选；若传入，长度 0-256 字节（按 UTF-8 字节计），不能包含控制字符。
3. `instance_source` 非必填，缺省为 `instance_pool`；取值须为枚举值：`instance_pool`、`k8s_pool`。
4. `instance_pool` 校验按 `instance_source` 条件化：
   - `instance_source=instance_pool`：必填，至少包含 1 个实例；同一 provider 内 `(addr, port)` 组合不能重复；至少有一个实例 `weight > 0`（PATCH 部分更新时省略 `instance_pool` 表示保留原值，不视为违反必填；显式提供时必须满足本条款）。
   - `instance_source=k8s_pool`：不参与有效池、不做成员数校验（休眠保留，切回 `instance_pool` 模式时自动恢复生效）。
5. `k8s_pool_name` 校验按 `instance_source` 条件化：
   - `instance_source=k8s_pool`：必填，长度 1-128 字符，仅允许字母、数字、`_`、`-`、`.`，不能以 `.`、`-`、`_` 开头或结尾，不能包含空白字符；引用的 pool 无需预先存在（池不存在 ≡ 零实例）。
   - `instance_source=instance_pool`：不参与有效池，传入时保留原值（休眠，切回 `k8s_pool` 模式时自动恢复生效）。
6. `k8s_instance_pool` 字段已彻底删除：响应中不返回；请求体携带该字段按未知字段静默忽略。
7. 每个实例包含 `addr`、`weight`、`port`；`addr` 必填且类型为 [Hostname](./00-common.md#1-主机名hostname)；`weight` 取值范围 [0,100]；`port` 必填且类型为 [Port](./00-common.md#3-网络端口port)。
8. `model_endpoint.schema` 有效值为 `http`、`https`，未设置时默认 `https`；`uri` 非空且须以 `/` 开头。
9. `models` 必填，至少 1 个元素；元素非空且不可重复。（PATCH 部分更新时省略 `models` 表示保留原值，不视为违反必填；显式提供时必须满足本条款。）
10. `keys` 非必填，默认空数组 `[]`；若非空：
   - 每个元素 `name` 必填，长度 1-128，同一 provider 内唯一；
   - 每个元素 `key` 必填且非空，长度 1-512。
11. `model_protocols` 必填，至少 1 个元素，元素不可重复，取值须为枚举值：`openai`、`anthropic`、`gemini`。
12. `protocol_paths` 非必填，缺省 = 不改写（请求路径原样转发）；传入 `{}` 清空（恢复为原样转发），传 `null` 与省略等价、均保留原值；若传入非空对象：
   - 键必须是 `model_protocols` 已声明协议的子集，取值仅支持 `openai`、`anthropic`（`gemini` 不支持路径改写）；
   - 值须以 `/` 开头、不以 `/` 结尾、不含 `..`/`?`/`#`、长度 ≤ 128；
   - PATCH 更新时键还须是**更新后** `model_protocols` 的子集（与 `model_protocols` 同时调整须在同一个请求中给出合法组合）。
13. `time_zone` 非必填，为空时默认 `Asia/Shanghai`；若传入，须为合法 IANA 时区名。
14. `tiers` 非必填；若传入：
    - 每个 tier 必须包含非空 `name` 和至少一个 `time_range`；
    - **初期 `name` 只支持 `peak`**；
    - `time_ranges` 中 `weekdays` 元素须在 0-6 之间，为空表示每天；
    - `start` / `end` 格式为 `HH:MM`，且 `end` 必须大于 `start`；
    - 同一 tier 内部 `time_ranges` 不得重叠。
15. `PUT /providers/{provider_name}/pricing-tiers` 中，`time_zone` / `tiers` 的校验规则同上；YAML 文件格式须能正确解析为相同结构。
16. 删除 provider 前，须校验无 cluster 引用，否则返回 `409 Conflict`；`/model-prices` 记录不再作为阻塞条件。
17. 触发模型发现时，`model_protocol`、`schema`、`addr`、`port` 为必填，`uri` 和 `apikey` 为选填；各参数须满足对应合法性条件；`model_protocol` 不在枚举值范围内时返回 `422`。
