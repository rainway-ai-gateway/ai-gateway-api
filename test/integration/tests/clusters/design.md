# Cluster 测试用例设计文档

## 1. 模块概述

Cluster 模块负责 AI 网关后端集群的管理，包括创建、查询、更新、删除。v0.3.0 是本次变更最大模块：删除 `ready`、`sub_clusters`、`scheduler` 等内部字段对外暴露；`Instance` 删除 `tags`；`llm_config` 必填；`llm_config.models` 为字符串数组；不再通过 OpenAPI 设置/获取 `DefaultAIClusterName`。

v0.0.7 起，`llm_config` 支持多 API-Key：
- 旧字段 `llm_config.key` 已移除，改为 `llm_config.keys`（`APIKey` 数组）+ `llm_config.key_policy`（Key 路由策略）。
- `keys` 非必填，默认值为空数组 `[]`；若传入，则所有元素的 `name`/`key`/`weight` 必填，同一数组内 `name` 唯一，且所有 `weight` 之和必须等于 100。
- `key_policy` 非必填；`strategy` 当前仅支持 `weighted_random`；`max_retries`、`retry_backoff_initial`、`retry_backoff_max` 必须 ≥0，且 `retry_backoff_max` ≥ `retry_backoff_initial`。
- 当 `model_endpoint.headers` 中包含 `${API_KEY}` 占位符时，`keys` 不能为空，否则返回 422。
- v0.4 起，`llm_config` 新增 `match_prefix` / `strip_prefix`：
  - `match_prefix` 非必填，用于声明该 cluster 匹配的前缀（如 `openrouter/`），若传入且非空则必须以 `/` 结尾；
  - `strip_prefix` 非必填，默认 `false`；为 `true` 时 `match_prefix` 必填且非空；
  - 两个字段透传到 InnerAPI 导出的 `AIConf.MatchPrefix` / `StripPrefix`，由 BFE 在转发前执行前缀裁剪。
- v0.6 起，`llm_config` 新增 `key_affinity`（会话级 Key 亲和性）：
  - 非必填，用于基于 Redis + `ClientKeyId` 保持同一客户端会话在一段时间内命中同一个后端 API-Key；
  - 包含 `enabled`（默认 `true`）、`ttl`（空闲超时秒数，默认 `600`）、`redis_prefix`（默认 `"bfe:ai:key_affinity"`）、`penalty_enable`（默认 `true`）；
  - 若传入 `ttl` 必须 `>0`；若传入 `redis_prefix` 必须非空；
  - 导出到 InnerAPI 的 `AIConf.KeyPolicy.SessionAffinity*` 字段。

另外：
- 删除集群时会扫描全部 `route_rules` 表中的全局、Entity、API-Key 路由规则（不经过分页）：若任意规则的 `targets` 或 `fallbacks` 引用了该集群，则删除被拒绝。
- 更新集群的 `llm_config.models` 时，会检查被移除的模型是否仍被 global/Entity/API-Key 路由规则的 `targets` 或 `fallbacks` 引用（匹配 `ClusterName` + `Model`），若存在引用则更新被拒绝。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| CL-1 | 创建集群 | POST | `/open-api/v1/clusters` | 一键创建集群、实例池、子集群 |
| CL-2 | 查询集群列表 | GET | `/open-api/v1/clusters` | 数组 |
| CL-3 | 查询集群详情 | GET | `/open-api/v1/clusters/{cluster_name}` | - |
| CL-4 | 更新集群 | PATCH | `/open-api/v1/clusters/{cluster_name}` | 可更新描述、实例池、各配置段 |
| CL-5 | 删除集群 | DELETE | `/open-api/v1/clusters/{cluster_name}` | 级联清理实例池、子集群；删除前检查 global/entity/apikey 路由规则引用 |

## 3. 测试用例统计

| 接口 | 测试用例数 |
|------|-----------|
| 创建集群 | 31 |
| 查询集群列表 | 1 |
| 查询集群详情 | 1 |
| 更新集群 | 15 |
| 删除集群 | 8 |
| **合计** | **56** |

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。

## 5. 目录结构

```
clusters/
├── design.md
├── create/
│   └── create_test.go
├── list/
│   └── list_test.go
├── detail/
│   └── detail_test.go
├── update/
│   └── update_test.go
└── delete/
    └── delete_test.go
```

## 6. 创建集群

### 6.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Cluster |
| 接口名称 | 创建集群 |
| 方法 | POST |
| 路径 | `/open-api/v1/clusters` |
| 说明 | 一键创建集群、实例池、子集群 |

### 6.2 接口参数说明

#### 6.2.1 请求参数

##### Body 参数

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| name | string | Y | 集群名，全局唯一 | 长度 1-64；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾；全局唯一；不能包含空白字符 |
| description | string | N | 集群描述 | 长度 ≤256 字节（按 UTF-8 字节计）；不能包含控制字符 |
| instance_pool | []Instance | Y | 实例列表 | ≥1 个元素；(hostname, ip) 组合唯一；至少一个实例 weight>0 |
| instance_pool[].hostname | string | Y | 实例主机名 | 符合 Hostname 类型 |
| instance_pool[].ip | string | Y | 实例 IP 地址 | 符合 IP Address 类型 |
| instance_pool[].weight | int | Y | 实例权重，范围 [0,100] | 0-100 |
| instance_pool[].ports | map[string]int | Y | 实例端口，至少包含 Default | ≥1 个键值对；必须包含 `Default`；端口值唯一 |
| basic | object | N | 基本参数 | `protocol` ∈ {http, https}；超时 >0；重试/连接数 ≥0 |
| sticky_sessions | object | N | 会话保持 | `hash_strategy` ∈ {CLIENT_IP_ONLY, CLIENT_ID_ONLY, CLIENT_ID_PREFERED} |
| passive_health_check | object | N | 被动健康检查 | `uri` 非空且以 `/` 开头；`statuscode` 为 0 或 100-599 |
| llm_config | object | Y | AI LLM 服务配置 | 必填；`models` ≥1 个非空唯一字符串；`model_endpoint.schema` ∈ {http, https}；`model_mappings.source_model` 唯一 |
| llm_config.model_endpoint | object | N | 模型列表端点配置 | `schema` ∈ {http, https} |
| llm_config.models | []string | Y | 支持的模型名称列表 | ≥1 个非空唯一字符串 |
| llm_config.model_mappings | []object | N | 模型名称映射 | `source_model`/`target_model` 必填；`source_model` 唯一 |
| llm_config.keys | []object | N | 多 API-Key 列表 | 非必填，默认 `[]`；非空时须满足下方“API-Key 结构” |
| llm_config.key_policy | object | N | Key 路由策略 | 非必填；`strategy` 仅支持 `weighted_random`；退避参数 ≥0 且 max ≥ initial |
| llm_config.provider_type | string | Y | AI 模型提供商类型 | 必填 |
| llm_config.match_prefix | string | N | provider/model 前缀 | 若传入且非空，必须以 `/` 结尾 |
| llm_config.strip_prefix | bool | N | 是否裁剪 `match_prefix` | 默认 `false`；为 `true` 时 `match_prefix` 必填且非空 |
| llm_config.key_affinity | object | N | 会话级 Key 亲和性配置 | 见下方“Key 亲和性配置” |

##### Key 亲和性配置（`llm_config.key_affinity`）

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| enabled | bool | N | 是否开启会话级 Key 亲和性 | 默认 `false` |
| ttl | int | N | 绑定空闲超时时间，单位秒 | 默认 `600`；若传入必须 `>0` |
| redis_prefix | string | N | Redis key 前缀 | 默认 `"bfe:ai:key_affinity"`；若传入必须非空 |
| penalty_enable | bool | N | 是否开启 Key 惩罚 | 默认 `true` |

##### API-Key 结构（`llm_config.keys` 元素）

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| name | string | Y | Key 名称/标识 | 长度 1-128；同一 `keys` 数组内唯一 |
| key | string | Y | 服务认证密钥 | 长度 1-512 |
| weight | int | Y | 权重 | 0-100；所有元素 weight 之和必须等于 100 |

##### Key 路由策略（`llm_config.key_policy`）

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| strategy | string | N | 选择策略 | 仅支持 `weighted_random` |
| max_retries | int | N | 最大重试次数 | ≥0 |
| retry_backoff_initial | int | N | 初始退避时间（毫秒） | ≥0 |
| retry_backoff_max | int | N | 最大退避时间（毫秒） | ≥0；且 ≥ `retry_backoff_initial` |

#### 6.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| name | string | 集群名 |
| description | string | 集群描述 |
| instance_pool | []Instance | 实例列表 |
| basic | object | 基本参数 |
| sticky_sessions | object | 会话保持 |
| passive_health_check | object | 被动健康检查 |
| llm_config | object | LLM 配置 |

> 返回不含 `ready`、`sub_clusters`、`scheduler`、`Instance.tags`。

### 6.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| CL-1-001 | 最小参数创建集群 | 正常参数 | 验证默认值填充 |
| CL-1-002 | 完整参数创建集群 | 正常参数 | 验证完整结构 |
| CL-1-003 | 缺少 llm_config | 必填校验 | 验证 ErrNum=422 |
| CL-1-004 | 缺少 instance_pool | 必填校验 | 验证 ErrNum=422 |
| CL-1-005 | 重复集群名 | 业务规则 | 验证 ErrNum=555/556 |
| CL-1-006 | instance_pool 为空数组 | 异常参数 | 验证 ErrNum=422 |
| CL-1-007 | 实例不含 Default 端口 | 异常参数 | 验证 ErrNum=422 |
| CL-1-008 | 非法 hostname | 合法性条件 | 验证 ErrNum=422 |
| CL-1-009 | 非法 IP | 合法性条件 | 验证 ErrNum=422 |
| CL-1-010 | weight 超过 100 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-011 | 重复实例 (hostname+ip) | 合法性条件 | 验证 ErrNum=422 |
| CL-1-012 | llm_config 模型重复 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-013 | 使用多 Key 创建集群 | 正常参数 | 验证 `keys`/`key_policy` 返回正确 |
| CL-1-014 | keys 权重和不为 100 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-015 | keys 中存在重复 name | 合法性条件 | 验证 ErrNum=422 |
| CL-1-016 | keys 元素缺少必填字段 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-017 | model_endpoint.headers 含 ${API_KEY} 但 keys 为空 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-018 | key_policy 非法 strategy | 合法性条件 | 验证 ErrNum=422 |
| CL-1-019 | key_policy 退避参数非法 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-020 | 合法前缀配置（strip_prefix=true） | 正常参数 | 验证 `match_prefix`/`strip_prefix` 返回正确 |
| CL-1-021 | strip_prefix=true 但 match_prefix 为空 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-022 | match_prefix 缺少尾部斜杠 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-023 | 仅 match_prefix、strip_prefix=false | 正常参数 | 验证仅用于路由标识 |
| CL-1-024 | 未配置 match_prefix / strip_prefix | 正常参数 | 验证默认值/字段不存在 |
| CL-1-025 | 非法 strip_prefix 类型 | 异常参数 | 验证 ErrNum=422 |
| CL-1-026 | 合法 key_affinity 配置 | 正常参数 | 验证创建成功且字段回显正确 |
| CL-1-027 | key_affinity.ttl ≤ 0 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-028 | key_affinity.redis_prefix 为空 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-029 | provider 不存在 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-030 | model 不在 provider 模型列表中 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-031 | key name 不在 provider 中 | 合法性条件 | 验证 ErrNum=422 |
| CL-1-032 | 被动健康检查 failnum 负值 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-1-033 | 被动健康检查 interval 负值 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-1-034 | 被动健康检查 statuscode 超范围 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-1-035 | 被动健康检查 uri 非 `/` 开头 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-1-036 | 被动健康检查 uri 显式空串 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-1-037 | 被动健康检查边界合法值 | 正常参数 | failnum=0/interval=0/statuscode=0 创建成功且回显正确 |
| CL-1-038 | 被动健康检查空对象 | 正常参数 | 验证默认值填充（3/1000/0/`/`） |
| CL-1-039 | basic.connection.max_idle_conn_per_rs 负值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-1-040 | basic.retries.max_retry_in_cluster 负值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-1-041 | basic.buffers.req_write_buffer_size 零值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-1-042 | basic.timeouts.timeout_conn_serv 零值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-1-043 | basic 边界合法值 | 正常参数 | max_idle=0/max_retry=0/buffer=1/timeout=1 创建成功且回显正确 |
| CL-1-044 | basic 空对象 | 正常参数 | 验证默认值填充（0/2/512/50000 等） |
| CL-1-045 | description 恰好 256 字节 | 边界值（issue #244 / SC2101-TC024） | 合法上界，创建成功且 GET 回读一致 |
| CL-1-046 | description 257 字节 | 合法性条件（issue #244 / SC2101-TC024） | 验证 ErrNum=422，报错含 `description length must be <= 256` |
| CL-1-047 | description 含控制字符 | 合法性条件（issue #244 / SC2101-TC024） | 验证 ErrNum=422，报错含 `control characters` |

### 6.4 测试场景详细设计

#### 6.4.1 CL-1-001：最小参数创建集群（正常参数）

##### 设计思路

验证最小参数创建集群时，`basic`、`sticky_sessions`、`passive_health_check` 使用默认值填充，且不包含内部字段。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求到 `/open-api/v1/clusters`，传入最小参数。
2. 验证响应状态码和返回结构。
3. 验证返回不含 `ready`、`sub_clusters`、`scheduler`。

##### 请求参数

```json
{
    "name": "cluster_min",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["deepseek-chat"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "cluster_min" | Equals |
| instance_pool | 长度为 1 | Len=1 |
| basic | 非空对象 | IsObject |
| sticky_sessions | 非空对象 | IsObject |
| passive_health_check | 非空对象 | IsObject |
| llm_config.models | ["deepseek-chat"] | Equals |
| llm_config.keys | 空数组 `[]` | Equals |
| llm_config.key_policy | 不存在或为 null | NotExists / IsNull |
| ready | 不存在 | NotExists |
| sub_clusters | 不存在 | NotExists |
| scheduler | 不存在 | NotExists |

---

#### 6.4.2 CL-1-002：完整参数创建集群（正常参数）

##### 设计思路

验证完整参数创建集群时返回结构与输入一致。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，传入完整参数。
2. 验证返回结构与输入一致。

##### 请求参数

```json
{
    "name": "cluster_full",
    "description": "完整集群",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 50,
            "ports": {
                "Default": 8080
            }
        },
        {
            "hostname": "backend-2",
            "ip": "10.0.0.2",
            "weight": 50,
            "ports": {
                "Default": 8080
            }
        }
    ],
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
        "model_endpoint": {
            "schema": "https",
            "uri": "/v1/models",
            "headers": {
                "Authorization": "Bearer ${API_KEY}"
            }
        },
        "models": ["deepseek-chat", "deepseek-coder"],
        "model_mappings": [
            {
                "key": "gpt-4",
                "value": "deepseek-chat"
            }
        ],
        "keys": [
            {
                "name": "primary",
                "key": "sk-aaaaaaaaaaaa",
                "weight": 70
            },
            {
                "name": "secondary",
                "key": "sk-bbbbbbbbbbbb",
                "weight": 30
            }
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 3,
            "retry_backoff_initial": 100,
            "retry_backoff_max": 5000
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "cluster_full" | Equals |
| description | "完整集群" | Equals |
| instance_pool | 长度为 2 | Len=2 |
| llm_config.models | ["deepseek-chat", "deepseek-coder"] | Equals |
| llm_config.keys | 长度为 2；元素字段与输入一致 | Len=2 / Equals |
| llm_config.key_policy.strategy | "weighted_random" | Equals |
| ready | 不存在 | NotExists |
| sub_clusters | 不存在 | NotExists |
| scheduler | 不存在 | NotExists |

---

#### 6.4.3 CL-1-003：缺少 llm_config（必填校验）

##### 设计思路

验证 `llm_config` 为必填字段，缺少时返回参数校验错误。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，Body 中缺少 `llm_config`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_no_llm",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "llm_config" 的错误信息  
**Data**：null

---

#### 6.4.4 CL-1-004：缺少 instance_pool（必填校验）

##### 设计思路

验证 `instance_pool` 为必填字段，缺少时返回参数校验错误。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，Body 中缺少 `instance_pool`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_no_pool",
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "instance_pool" 的错误信息  
**Data**：null

---

#### 6.4.5 CL-1-005：重复集群名（业务规则）

##### 设计思路

验证集群名全局唯一，重复创建时返回错误。

##### 前提数据准备

已创建 `cluster_dup`。

##### 执行步骤

1. 发送 POST 请求，使用重复集群名。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_dup",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：555 或 556  
**ErrMsg**：集群名已存在的错误信息  
**Data**：null

---

#### 6.4.6 CL-1-006：instance_pool 为空数组（异常参数）

##### 设计思路

验证 `instance_pool` 不能为空数组。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`instance_pool` 为空数组。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_empty_pool",
    "instance_pool": [],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "instance_pool" 或至少 1 个元素的错误信息  
**Data**：null

---

#### 6.4.7 CL-1-007：实例不含 Default 端口（异常参数）

##### 设计思路

验证实例端口必须包含 `Default` 键。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，实例 `ports` 中不包含 `Default`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_bad_port",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Other": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "Default" 或 "ports" 的错误信息  
**Data**：null

---

#### 6.4.8 CL-1-008：非法 hostname（合法性条件）

##### 设计思路

验证实例 `hostname` 必须符合 Hostname 类型（不能以 `-` 开头等）。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，实例 `hostname` 以 `-` 开头。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_bad_hostname",
    "instance_pool": [
        {
            "hostname": "-bad",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 hostname 非法的错误信息  
**Data**：null

---

#### 6.4.9 CL-1-009：非法 IP（合法性条件）

##### 设计思路

验证实例 `ip` 必须是合法 IPv4/IPv6 地址。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，实例 `ip` 为非法字符串。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_bad_ip",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "not-an-ip",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 ip 非法的错误信息  
**Data**：null

---

#### 6.4.10 CL-1-010：weight 超过 100（合法性条件）

##### 设计思路

验证实例 `weight` 取值范围为 0-100。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，实例 `weight=101`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_bad_weight",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 101,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 weight 非法的错误信息  
**Data**：null

---

#### 6.4.11 CL-1-011：重复实例 (hostname+ip)（合法性条件）

##### 设计思路

验证同一集群内 `(hostname, ip)` 组合不能重复。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`instance_pool` 包含两个相同 `(hostname, ip)` 的实例。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_dup_instance",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 50,
            "ports": {
                "Default": 8080
            }
        },
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 50,
            "ports": {
                "Default": 8081
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含重复实例的错误信息  
**Data**：null

---

#### 6.4.12 CL-1-012：llm_config 模型重复（合法性条件）

##### 设计思路

验证 `llm_config.models` 中的模型名称不能重复。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`llm_config.models` 包含重复模型名。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "cluster_dup_model",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m", "m"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含重复 model 的错误信息  
**Data**：null

---

#### 6.4.13 CL-1-013：使用多 Key 创建集群（正常参数）

##### 设计思路

验证使用 `llm_config.keys` 多 Key 列表和 `key_policy` 创建集群成功，返回结构与输入一致。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`llm_config.keys` 包含 2 个 Key，且 `key_policy` 配置完整。
2. 验证返回的 `llm_config.keys`/`llm_config.key_policy` 与输入一致。

##### 请求参数

```json
{
    "name": "cluster_multi_keys",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "model_endpoint": {
            "schema": "https",
            "uri": "/v1/models",
            "headers": {
                "Authorization": "Bearer ${API_KEY}"
            }
        },
        "models": ["deepseek-chat"],
        "keys": [
            {
                "name": "primary",
                "key": "sk-aaaaaaaaaaaa",
                "weight": 70
            },
            {
                "name": "secondary",
                "key": "sk-bbbbbbbbbbbb",
                "weight": 30
            }
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 3,
            "retry_backoff_initial": 100,
            "retry_backoff_max": 5000
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "cluster_multi_keys" | Equals |
| llm_config.keys | 长度为 2 | Len=2 |
| llm_config.keys[0].name | "primary" | Equals |
| llm_config.keys[0].key | "sk-aaaaaaaaaaaa" | Equals |
| llm_config.keys[0].weight | 70 | Equals |
| llm_config.keys[1].name | "secondary" | Equals |
| llm_config.key_policy.strategy | "weighted_random" | Equals |
| llm_config.key_policy.max_retries | 3 | Equals |

---

#### 6.4.14 CL-1-014：keys 权重和不为 100（合法性条件）

##### 设计思路

验证 `llm_config.keys` 所有元素 `weight` 之和必须等于 100，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`keys` 中两个 Key 的 weight 分别为 60 和 30（合计 90）。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_bad_key_weight",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "keys": [
            {"name": "k1", "key": "sk-1", "weight": 60},
            {"name": "k2", "key": "sk-2", "weight": 30}
        ],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "total weight" 或 "100" 的错误信息  
**Data**：null

---

#### 6.4.15 CL-1-015：keys 中存在重复 name（合法性条件）

##### 设计思路

验证同一 `keys` 数组内 `name` 必须唯一。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`keys` 中两个 Key 使用相同 `name`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_dup_key_name",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "keys": [
            {"name": "same", "key": "sk-1", "weight": 50},
            {"name": "same", "key": "sk-2", "weight": 50}
        ],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "duplicate name" 或 "keys" 的错误信息  
**Data**：null

---

#### 6.4.16 CL-1-016：keys 元素缺少必填字段（合法性条件）

##### 设计思路

验证 `keys` 元素中的 `name`/`key`/`weight` 均为必填字段，缺少任一字段返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`keys` 元素缺少 `weight`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_key_missing_field",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "keys": [
            {"name": "k1", "key": "sk-1"}
        ],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "weight is required" 或 "keys" 的错误信息  
**Data**：null

---

#### 6.4.17 CL-1-017：model_endpoint.headers 含 ${API_KEY} 但 keys 为空（合法性条件）

##### 设计思路

验证当 `model_endpoint.headers` 中出现 `${API_KEY}` 占位符时，`keys` 不能为空数组，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`model_endpoint.headers.Authorization` 包含 `${API_KEY}`，但 `keys` 为空数组。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_api_key_placeholder_no_keys",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "model_endpoint": {
            "schema": "https",
            "uri": "/v1/models",
            "headers": {
                "Authorization": "Bearer ${API_KEY}"
            }
        },
        "keys": [],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "API_KEY" 或 "keys is empty" 的错误信息  
**Data**：null

---

#### 6.4.18 CL-1-018：key_policy 非法 strategy（合法性条件）

##### 设计思路

验证 `key_policy.strategy` 仅支持 `weighted_random`，其他值返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`key_policy.strategy` 为 `round_robin`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_bad_key_policy_strategy",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "keys": [
            {"name": "k1", "key": "sk-1", "weight": 100}
        ],
        "key_policy": {
            "strategy": "round_robin"
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "weighted_random" 或 "strategy" 的错误信息  
**Data**：null

---

#### 6.4.19 CL-1-019：key_policy 退避参数非法（合法性条件）

##### 设计思路

验证 `key_policy.retry_backoff_max` 必须 ≥ `retry_backoff_initial`，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`retry_backoff_initial=1000`，`retry_backoff_max=500`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_bad_key_policy_backoff",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ],
    "llm_config": {
        "models": ["m"],
        "keys": [
            {"name": "k1", "key": "sk-1", "weight": 100}
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "retry_backoff_initial": 1000,
            "retry_backoff_max": 500
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "retry_backoff_max" 或 "retry_backoff_initial" 的错误信息  
**Data**：null

----

#### 6.4.20 CL-1-020：合法前缀配置（strip_prefix=true）（正常参数）

##### 设计思路

验证 `llm_config.match_prefix` / `strip_prefix` 合法组合可成功创建集群，且响应字段回显正确。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`match_prefix="openrouter/"`，`strip_prefix=true`。
2. 验证返回 200。
3. 验证响应 `llm_config.match_prefix == "openrouter/"`、`llm_config.strip_prefix == true`。

##### 请求参数

```json
{
    "name": "cluster_prefix_valid",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["openrouter/anthropic/claude-sonnet-4"],
        "match_prefix": "openrouter/",
        "strip_prefix": true,
        "provider_type": "openrouter"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**Data**：`llm_config.match_prefix == "openrouter/"`，`llm_config.strip_prefix == true`

----

#### 6.4.21 CL-1-021：strip_prefix=true 但 match_prefix 为空（合法性条件）

##### 设计思路

验证 `strip_prefix=true` 时，`match_prefix` 必须非空，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`strip_prefix=true`，不传入 `match_prefix`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_prefix_missing",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["m"],
        "strip_prefix": true,
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "match_prefix is required when strip_prefix is true"  
**Data**：null

----

#### 6.4.22 CL-1-022：match_prefix 缺少尾部斜杠（合法性条件）

##### 设计思路

验证非空 `match_prefix` 必须以 `/` 结尾，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`match_prefix="openrouter"`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_prefix_no_slash",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["m"],
        "match_prefix": "openrouter",
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "match_prefix must end with '/'"  
**Data**：null

----

#### 6.4.23 CL-1-023：仅 match_prefix、strip_prefix=false（正常参数）

##### 设计思路

验证 `strip_prefix=false` 时，`match_prefix` 可用于路由标识但不裁剪前缀，创建成功且字段回显正确。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`match_prefix="openrouter/"`，`strip_prefix=false`。
2. 验证返回 200，字段回显正确。

##### 请求参数

```json
{
    "name": "cluster_prefix_no_strip",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["openrouter/anthropic/claude-sonnet-4"],
        "match_prefix": "openrouter/",
        "strip_prefix": false,
        "provider_type": "openrouter"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**Data**：`llm_config.match_prefix == "openrouter/"`，`llm_config.strip_prefix == false`

----

#### 6.4.24 CL-1-024：未配置 match_prefix / strip_prefix（正常参数）

##### 设计思路

验证不配置这两个字段时，集群创建成功，响应中不存在相关字段或为默认值。

##### 前提数据准备

无

##### 执行步骤

1. 发送最小参数创建集群请求，不携带 `match_prefix` / `strip_prefix`。
2. 验证返回 200，响应中不包含 `match_prefix` 或 `strip_prefix` 字段（或 `strip_prefix` 为 false）。

##### 请求参数

```json
{
    "name": "cluster_no_prefix",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["deepseek-chat"],
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**Data**：`llm_config.match_prefix` 不存在、为 `null` 或为空字符串；`strip_prefix` 不存在或为 `false`

----

#### 6.4.25 CL-1-025：非法 strip_prefix 类型（异常参数）

##### 设计思路

验证 `strip_prefix` 必须为 bool 类型，传入字符串等非法类型时返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`strip_prefix="true"`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_prefix_bad_type",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["m"],
        "match_prefix": "openrouter/",
        "strip_prefix": "true",
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "strip_prefix" 或类型错误信息  
**Data**：null

---

#### 6.4.26 CL-1-026：合法 key_affinity 配置（正常参数）

##### 设计思路

验证 `llm_config.key_affinity` 合法配置可成功创建集群，且响应字段回显正确。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`key_affinity` 配置 `enabled=true`、`ttl=600`、`redis_prefix="bfe:ai:key_affinity"`、`penalty_enable=true`。
2. 验证返回 200，响应中 `llm_config.key_affinity` 与输入一致。

##### 请求参数

```json
{
    "name": "cluster_key_affinity_valid",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["deepseek-chat"],
        "key_affinity": {
            "enabled": true,
            "ttl": 600,
            "redis_prefix": "bfe:ai:key_affinity",
            "penalty_enable": true
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.key_affinity.enabled | true | Equals |
| llm_config.key_affinity.ttl | 600 | Equals |
| llm_config.key_affinity.redis_prefix | "bfe:ai:key_affinity" | Equals |
| llm_config.key_affinity.penalty_enable | true | Equals |

---

#### 6.4.27 CL-1-027：key_affinity.ttl ≤ 0（合法性条件）

##### 设计思路

验证 `key_affinity.ttl` 必须 `>0`，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`key_affinity.ttl=0`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_key_affinity_bad_ttl",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["m"],
        "key_affinity": {
            "enabled": true,
            "ttl": 0
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "ttl" 或 "key_affinity" 的错误信息  
**Data**：null

---

#### 6.4.28 CL-1-028：key_affinity.redis_prefix 为空（合法性条件）

##### 设计思路

验证 `key_affinity.redis_prefix` 若传入必须非空，否则返回 422。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`key_affinity.redis_prefix=""`。
2. 验证返回 422。

##### 请求参数

```json
{
    "name": "cluster_key_affinity_empty_prefix",
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "10.0.0.1",
            "weight": 100,
            "ports": {"Default": 8080}
        }
    ],
    "llm_config": {
        "models": ["m"],
        "key_affinity": {
            "redis_prefix": ""
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "redis_prefix" 或 "key_affinity" 的错误信息  
**Data**：null

---

#### 6.4.45 CL-1-045：description 恰好 256 字节（边界值，issue #244 / SC2101-TC024）

##### 设计思路

`clusters.md` §1/§2.1 规定 `description` 长度 0-256 字节。256 是合法上界，必须被接受并原样落库（与 CL-1-046 构成上界两侧）。

##### 前提数据准备

已存在可用 provider。

##### 执行步骤

1. POST 创建集群，`description` 为 256 个 ASCII 字符（= 256 字节）。
2. 验证返回 200 且 `Data.description` 与提交值一致。
3. GET 回读，验证 `description` 仍与提交值一致。

##### 请求参数

```json
{
    "name": "<随机唯一集群名>",
    "description": "<256 个字节的字符串>",
    "llm_config": { "models": ["deepseek-chat"], "provider": "<provider>" }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| description | 提交的 256 字节字符串 | Equals |
| description（GET 回读） | 提交的 256 字节字符串 | Equals |

---

#### 6.4.46 CL-1-046：description 257 字节（合法性条件，issue #244 / SC2101-TC024）

##### 设计思路

修复前 `/clusters` 对 `description` 无任何长度校验，257 字节会被接受并落库（`SC2101-TC024` 的失败摘要 `Cluster description length 257 code=200`）。本用例钉住修复后的拒绝行为。

##### 前提数据准备

已存在可用 provider。

##### 执行步骤

1. POST 创建集群，`description` 为 257 个 ASCII 字符。
2. 验证返回 422，且报错信息包含长度上限。

##### 请求参数

```json
{
    "name": "<随机唯一集群名>",
    "description": "<257 个字节的字符串>",
    "llm_config": { "models": ["deepseek-chat"], "provider": "<provider>" }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 `description length must be <= 256`  
**Data**：null

---

#### 6.4.47 CL-1-047：description 含控制字符（合法性条件，issue #244 / SC2101-TC024）

##### 设计思路

`clusters.md` §1 要求"不能包含控制字符"。修复前该规则在集群路径上完全缺失（`\x01` 会被接受），本用例钉住新增拦截。

##### 前提数据准备

已存在可用 provider。

##### 执行步骤

1. POST 创建集群，`description` 为 `"abc\u0001def"`。
2. 验证返回 422，且报错信息指向控制字符。

##### 请求参数

```json
{
    "name": "<随机唯一集群名>",
    "description": "abc\u0001def",
    "llm_config": { "models": ["deepseek-chat"], "provider": "<provider>" }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 `control characters`  
**Data**：null

---

## 7. 查询集群列表

### 7.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Cluster |
| 接口名称 | 查询集群列表 |
| 方法 | GET |
| 路径 | `/open-api/v1/clusters` |
| 说明 | 返回所有集群列表 |

### 7.2 接口参数说明

#### 7.2.1 请求参数

无

#### 7.2.2 返回数据字段

数组，单元素同创建接口返回字段。

### 7.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| CL-2-001 | 查询集群列表 | 正常参数 | 返回数组不含内部字段 |

### 7.4 测试场景详细设计

#### 7.4.1 CL-2-001：查询集群列表（正常参数）

##### 设计思路

验证列表接口返回所有集群，且元素不含内部字段。

##### 前提数据准备

已创建集群。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/clusters`。
2. 验证返回数组元素字段完整且不含 `ready`/`sub_clusters`/`scheduler`。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | 数组 | IsArray |
| Data[*].name | 非空字符串 | NotEmpty |
| Data[*].ready | 不存在 | NotExists |
| Data[*].sub_clusters | 不存在 | NotExists |
| Data[*].scheduler | 不存在 | NotExists |

---

## 8. 查询集群详情

### 8.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Cluster |
| 接口名称 | 查询集群详情 |
| 方法 | GET |
| 路径 | `/open-api/v1/clusters/{cluster_name}` |
| 说明 | 返回单个集群详情 |

### 8.2 接口参数说明

#### 8.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| cluster_name | string | Y | 集群名字 |

#### 8.2.2 返回数据字段

同创建接口。

### 8.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| CL-3-001 | 查询集群详情 | 正常参数 | 字段完整且无内部字段 |
| CL-3-002 | 查询单字符名称的集群 | 正常参数 | 单字符集群名可正常查询（回归 issue #130） |

### 8.4 测试场景详细设计

#### 8.4.1 CL-3-001：查询集群详情（正常参数）

##### 设计思路

验证详情接口返回完整集群字段，且不包含内部字段。

##### 前提数据准备

已创建集群。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/clusters/{cluster_name}`。
2. 验证返回字段完整且不含 `ready`/`sub_clusters`/`scheduler`。

##### 请求参数

URI：`cluster_full`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | 与目标集群一致 | Equals |
| instance_pool | 数组 | IsArray |
| llm_config | 非空对象 | IsObject |
| ready | 不存在 | NotExists |
| sub_clusters | 不存在 | NotExists |
| scheduler | 不存在 | NotExists |

#### 8.4.2 CL-3-002：查询单字符名称的集群（正常参数）

##### 设计思路

回归测试（issue #130）：创建集群允许名称长度为 1 个字符，但单查 URI 参数曾强校验 `min=2`，
导致单字符名称的集群"建得成、查不到"。验证单字符名称的集群可正常查询。

##### 前提数据准备

已创建名称为单字符（如 `c`）的集群。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/clusters/c`。
2. 验证 ErrNum=200，且返回的 `name` 与请求一致。

##### 请求参数

URI：`c`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | `c` | Equals |

---

## 9. 更新集群

### 9.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Cluster |
| 接口名称 | 更新集群 |
| 方法 | PATCH |
| 路径 | `/open-api/v1/clusters/{cluster_name}` |
| 说明 | 更新集群基本信息，可编辑描述、实例池、各配置段 |

### 9.2 接口参数说明

#### 9.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| cluster_name | string | Y | 集群名字 |

##### Body 参数

可修改字段含义同创建接口，但**输入参数不包括 `name`，即不能修改 cluster 的 name**（名称由 URI 中的 `cluster_name` 指定；若包含 `name` 返回 422）。若传入 `instance_pool` 字段，系统会自动同步更新对应的实例池。

#### 9.2.2 返回数据字段

同创建接口。

### 9.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| CL-4-001 | 更新实例池 | 正常参数 | instance_pool 更新，其余字段不变 |
| CL-4-002 | 更新 llm_config | 正常参数 | llm_config.models 更新 |
| CL-4-003 | 更新后查询一致性 | 返回数据 | PATCH 后立即 GET，验证数据一致 |
| CL-4-004 | 更新不存在的集群 | 异常参数 | 验证 ErrNum=404 |
| CL-4-005 | 更新非法 instance_pool（非法 IP） | 合法性条件 | 验证 ErrNum=422 |
| CL-4-006 | 更新 keys（全量替换） | 正常参数 | 验证 PATCH 后 keys 被完整替换 |
| CL-4-007 | 更新 key_policy | 正常参数 | 验证 PATCH 后 key_policy 更新生效 |
| CL-4-008 | 更新 match_prefix / strip_prefix | 正常参数 | 验证 PATCH 后前缀配置更新生效，InnerAPI 导出一致 |
| CL-4-009 | 删除被路由引用的模型 | 业务规则 | `llm_config.models` 移除仍被路由规则引用的模型，验证 ErrNum=409 |
| CL-4-010 | 清理路由引用后可删除模型 | 正常参数 | 移除 API-Key 路由规则引用后，可成功删除集群模型 |
| CL-4-011 | 更新 key_affinity | 正常参数 | 验证 PATCH 后 key_affinity 更新生效，InnerAPI 导出一致 |
| CL-4-012 | 请求体不包含 `name` | 正常参数 | 请求体不传 `name`，验证返回的 `name` 与 URI 一致 |
| CL-4-013 | 请求体包含 `name` | 异常参数 | 验证 ErrNum=422 |
| CL-4-014 | 更新 passive_health_check 合法值 | 正常参数 | 验证 PATCH 后回读与 InnerAPI `CheckConf` 导出一致（issue #172） |
| CL-4-015 | PATCH 被动健康检查 failnum 负值 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-4-016 | PATCH 被动健康检查 statuscode 超范围 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-4-017 | PATCH 被动健康检查 uri 非 `/` 开头 | 合法性条件（issue #172） | 验证 ErrNum=422 且错误可归因 |
| CL-4-018 | PATCH 非法被动健康检查不污染存量配置 | 业务规则（issue #172） | 非法 PATCH 被拒后 GET 回读，存量值保持不变 |
| CL-4-019 | PATCH basic.connection.max_idle_conn_per_rs 负值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-4-020 | PATCH basic.retries.max_retry_in_cluster 负值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-4-021 | PATCH basic.buffers.req_write_buffer_size 零值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-4-022 | PATCH basic.timeouts.timeout_conn_serv 零值 | 合法性条件（issue #173） | 验证 ErrNum=422 且错误可归因 |
| CL-4-023 | PATCH 非法 basic 不污染存量配置 | 业务规则（issue #173） | 非法 PATCH 被拒后 GET 回读，存量 basic 配置保持不变 |
| CL-4-024 | description 恰好 256 字节 | 边界值（issue #244 / SC2101-TC024） | 合法上界，返回 200 且 GET 回读一致 |
| CL-4-025 | description 257 字节 | 合法性条件（issue #244 / SC2101-TC024） | 验证 ErrNum=422（修复前为 200，SC2101-TC024 的失败臂） |
| CL-4-026 | description 含控制字符 | 合法性条件（issue #244 / SC2101-TC024） | 验证 ErrNum=422 且旧值保留 |
| CL-4-027 | PATCH 非法 description 不污染存量值 | 原子性 | 257 字节被拒后 GET 回读，存量 description 保持不变 |

### 9.4 测试场景详细设计

#### 9.4.1 CL-4-001：更新实例池（正常参数）

##### 设计思路

验证部分更新 `instance_pool` 成功，其余字段保持不变。

##### 前提数据准备

已创建集群。

##### 执行步骤

1. 发送 PATCH 请求到 `/open-api/v1/clusters/{cluster_name}`，传入新的 `instance_pool`。
2. 验证返回的 `instance_pool` 已更新。

##### 请求参数

```json
{
    "instance_pool": [
        {
            "hostname": "backend-2",
            "ip": "10.0.0.2",
            "weight": 100,
            "ports": {
                "Default": 9090
            }
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| instance_pool | 长度为 1 | Len=1 |
| instance_pool[0].hostname | "backend-2" | Equals |
| instance_pool[0].ip | "10.0.0.2" | Equals |
| instance_pool[0].ports.Default | 9090 | Equals |

---

#### 9.4.2 CL-4-002：更新 llm_config（正常参数）

##### 设计思路

验证部分更新 `llm_config` 成功。

##### 前提数据准备

已创建集群。

##### 执行步骤

1. 发送 PATCH 请求，传入新的 `llm_config`。
2. 验证返回的 `llm_config.models` 已更新。

##### 请求参数

```json
{
    "llm_config": {
        "models": ["qwen-turbo"],
        "provider_type": "qwen"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.models | ["qwen-turbo"] | Equals |
| llm_config.provider_type | "qwen" | Equals |
| llm_config.keys | 与更新前一致或为空数组 | Equals |

---

#### 9.4.3 CL-4-003：更新后查询一致性（返回数据）

##### 设计思路

验证 PATCH 更新成功后，立即通过 GET 查询，返回的集群数据与更新请求一致。

##### 前提数据准备

已创建集群。

##### 执行步骤

1. 发送 PATCH 请求更新集群描述。
2. 发送 GET 请求查询集群。
3. 对比两次返回的描述是否一致。

##### 请求参数

```json
{
    "description": "更新后的集群描述"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| description | "更新后的集群描述" | Equals |

---

#### 9.4.4 CL-4-004：更新不存在的集群（异常参数）

##### 设计思路

验证更新不存在的集群时返回 404。

##### 前提数据准备

无

##### 执行步骤

1. 发送 PATCH 请求到 `/open-api/v1/clusters/non_existent_cluster`。
2. 验证返回错误码。

##### 请求参数

URI：`non_existent_cluster`  
Body：
```json
{
    "description": "x"
}
```

##### 预期返回结果

**ErrNum**：404  
**ErrMsg**：集群不存在的错误信息  
**Data**：null

---

#### 9.4.5 CL-4-005：更新非法 instance_pool（非法 IP）

##### 设计思路

验证更新集群时传入的 `instance_pool` 同样受实例合法性条件约束。

##### 前提数据准备

已创建集群。

##### 执行步骤

1. 发送 PATCH 请求，`instance_pool` 中实例 `ip` 为非法字符串。
2. 验证返回错误码。

##### 请求参数

```json
{
    "instance_pool": [
        {
            "hostname": "backend-1",
            "ip": "not-an-ip",
            "weight": 100,
            "ports": {
                "Default": 8080
            }
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 ip 非法的错误信息  
**Data**：null

---

#### 9.4.6 CL-4-006：更新 keys（全量替换）

##### 设计思路

验证 `PATCH /open-api/v1/clusters/{cluster_name}` 对 `llm_config.keys` 执行全量替换：传入完整的新 Key 列表后，旧的 Key 被清空，新的 Key 生效。

##### 前提数据准备

已创建集群 `cluster_update_keys`，初始 `llm_config.keys` 包含一个 Key。

##### 执行步骤

1. 发送 PATCH 请求，传入新的 `llm_config.keys`（2 个 Key）和 `key_policy`。
2. 验证返回的 `keys` 与请求一致，原 Key 已不存在。
3. 发送 GET 请求查询集群，验证返回的 `keys`/`key_policy` 与 PATCH 一致。

##### 请求参数

URI：`cluster_update_keys`

```json
{
    "llm_config": {
        "models": ["deepseek-chat"],
        "keys": [
            {
                "name": "new-primary",
                "key": "sk-new-primary",
                "weight": 60
            },
            {
                "name": "new-secondary",
                "key": "sk-new-secondary",
                "weight": 40
            }
        ],
        "key_policy": {
            "strategy": "weighted_random",
            "max_retries": 5
        }
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.keys | 长度为 2 | Len=2 |
| llm_config.keys[0].name | "new-primary" | Equals |
| llm_config.keys[1].name | "new-secondary" | Equals |
| llm_config.key_policy.max_retries | 5 | Equals |

---

#### 9.4.7 CL-4-007：更新 key_policy（正常参数）

##### 设计思路

验证单独更新 `llm_config.key_policy` 不影响 `keys` 内容。

##### 前提数据准备

已创建集群 `cluster_update_policy`，初始 `llm_config.keys` 包含两个 Key。

##### 执行步骤

1. 发送 PATCH 请求，仅传入新的 `llm_config.key_policy`。
2. 验证返回的 `key_policy` 已更新，`keys` 保持原状。
3. 发送 GET 请求查询集群，验证一致性。

##### 请求参数

URI：`cluster_update_policy`

```json
{
    "llm_config": {
        "models": ["deepseek-chat"],
        "key_policy": {
            "strategy": "weighted_random",
            "retry_backoff_initial": 200,
            "retry_backoff_max": 2000
        }
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.keys | 与更新前一致 | Equals |
| llm_config.key_policy.retry_backoff_initial | 200 | Equals |
| llm_config.key_policy.retry_backoff_max | 2000 | Equals |

----

#### 9.4.8 CL-4-008：更新 match_prefix / strip_prefix（正常参数）

##### 设计思路

验证通过 PATCH 更新 `llm_config.match_prefix` / `strip_prefix` 后，OpenAPI 查询与 InnerAPI 导出均生效。

##### 前提数据准备

已创建集群 `cluster_update_prefix`，初始配置：
```json
{
    "llm_config": {
        "models": ["openrouter/anthropic/claude-sonnet-4"],
        "match_prefix": "openrouter/",
        "strip_prefix": true,
        "provider_type": "openrouter"
    }
}
```

##### 执行步骤

1. 发送 PATCH 请求，将 `match_prefix` 改为 `"deepseek/"`，`strip_prefix` 改为 `false`。
2. 验证返回 200，响应中字段已更新。
3. 发送 GET 请求查询集群，验证字段一致。
4. 发送 GET 请求到 `/inner-api/v1/configs/tls_conf/server_data_conf`，验证 `AIConf.MatchPrefix == "deepseek/"`、`AIConf.StripPrefix == false`。

##### 请求参数

URI：`cluster_update_prefix`

```json
{
    "llm_config": {
        "models": ["deepseek-chat"],
        "match_prefix": "deepseek/",
        "strip_prefix": false,
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.match_prefix | "deepseek/" | Equals |
| llm_config.strip_prefix | false | Equals |

---

#### 9.4.9 CL-4-009：删除被路由引用的模型（业务规则）

##### 设计思路

验证更新集群时，若 `llm_config.models` 中移除的模型仍被 global/Entity/API-Key 路由规则的 `targets` 或 `fallbacks` 引用（同时匹配 `ClusterName` 与 `Model`），则更新应被拒绝，返回 409 资源依赖冲突错误。

##### 前提数据准备

已创建集群 `cluster_ref_model`，`llm_config.models` 包含 `test-model`；已创建 API-Key `model-ref-key`，其 `route_rules` 中存在规则 `rule-ref-model`，`targets` 引用该集群的 `test-model`。

##### 执行步骤

1. 发送 PATCH 请求到 `/open-api/v1/clusters/{cluster_name}`，将 `llm_config.models` 从 `["test-model"]` 改为 `["other-model"]`。
2. 验证返回错误码与错误信息。

##### 请求参数

URI：`cluster_ref_model`

```json
{
    "llm_config": {
        "models": ["other-model"],
        "provider_type": "test-provider"
    }
}
```

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：包含 `Rule rule-ref-model Refer To Model test-model In Cluster`  
**Data**：null

---

#### 9.4.10 CL-4-010：清理路由引用后可删除模型（正常参数）

##### 设计思路

验证当被移除的模型不再被任何路由规则引用时，更新集群 `llm_config.models` 可以成功执行。

##### 前提数据准备

已完成 CL-4-009 的场景，即集群 `cluster_ref_model` 与 API-Key `model-ref-key` 均存在，且路由规则引用 `test-model`。

##### 执行步骤

1. 发送 PATCH 请求到 `/open-api/v1/api-keys/{api_key_id}`，将 API-Key 的 `route_rules` 清空或禁用，移除对 `test-model` 的引用。
2. 再次发送 PATCH 请求到 `/open-api/v1/clusters/{cluster_name}`，将 `llm_config.models` 改为 `["other-model"]`。
3. 验证返回 200，且 `llm_config.models` 已更新。

##### 请求参数

**步骤 1：清空 API-Key 路由规则**

URI：`model-ref-key` 的 id

```json
{
    "route_rules": {
        "enabled": false,
        "rules": []
    }
}
```

**步骤 2：更新集群模型列表**

URI：`cluster_ref_model`

```json
{
    "llm_config": {
        "models": ["other-model"],
        "provider_type": "test-provider"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.models | ["other-model"] | Equals |

---

#### 9.4.11 CL-4-011：更新 key_affinity（正常参数）

##### 设计思路

验证通过 PATCH 更新 `llm_config.key_affinity` 后，OpenAPI 查询与 InnerAPI 导出均生效。

##### 前提数据准备

已创建集群 `cluster_update_key_affinity`，初始配置：
```json
{
    "llm_config": {
        "models": ["deepseek-chat"],
        "key_affinity": {
            "enabled": false,
            "ttl": 600,
            "redis_prefix": "bfe:ai:key_affinity",
            "penalty_enable": true
        },
        "provider_type": "deepseek"
    }
}
```

##### 执行步骤

1. 发送 PATCH 请求，将 `key_affinity.enabled` 改为 `true`，`ttl` 改为 `1200`，`redis_prefix` 改为 `"bfe:ai:key_affinity:v2"`。
2. 验证返回 200，响应中字段已更新。
3. 发送 GET 请求查询集群，验证字段一致。
4. 发送 GET 请求到 `/inner-api/v1/configs/tls_conf/server_data_conf`，验证 `AIConf.KeyPolicy.SessionAffinity == true`、`SessionAffinityTTL == 1200`、`SessionAffinityRedisPrefix == "bfe:ai:key_affinity:v2"`。

##### 请求参数

URI：`cluster_update_key_affinity`

```json
{
    "llm_config": {
        "models": ["deepseek-chat"],
        "key_affinity": {
            "enabled": true,
            "ttl": 1200,
            "redis_prefix": "bfe:ai:key_affinity:v2",
            "penalty_enable": false
        },
        "provider_type": "deepseek"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| llm_config.key_affinity.enabled | true | Equals |
| llm_config.key_affinity.ttl | 1200 | Equals |
| llm_config.key_affinity.redis_prefix | "bfe:ai:key_affinity:v2" | Equals |
| llm_config.key_affinity.penalty_enable | false | Equals |

---

#### 9.4.24 CL-4-024：description 恰好 256 字节（边界值，issue #244 / SC2101-TC024）

##### 设计思路

`clusters.md` §1/§2.1 规定 `description` 长度 0-256 字节；256 是合法上界，PATCH 必须接受并原样落库（与 CL-4-025 构成上界两侧）。

##### 前提数据准备

已存在可用集群。

##### 执行步骤

1. PATCH `description` 为 256 个 ASCII 字符（= 256 字节），验证 200 且 `Data.description` 一致。
2. GET 回读，验证 `description` 仍一致。

##### 请求参数

```json
{ "description": "<256 个字节的字符串>" }
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| description | 提交的 256 字节字符串 | Equals |
| description（GET 回读） | 提交的 256 字节字符串 | Equals |

---

#### 9.4.25 CL-4-025：description 257 字节被拒绝（合法性条件，issue #244 / SC2101-TC024）

##### 设计思路

修复前 PATCH `description` 257 字节返回 200 并改写库值（`SC2101-TC024` 的失败臂）；修复后必须 422。

##### 前提数据准备

已存在可用集群。

##### 执行步骤

1. PATCH `description` 为 257 个 ASCII 字符。
2. 验证返回 422。

##### 请求参数

```json
{ "description": "<257 个字节的字符串>" }
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 `description length must be <= 256`  
**Data**：null

---

#### 9.4.26 CL-4-026：description 含控制字符被拒绝且旧值保留（合法性条件）

##### 设计思路

`clusters.md` §1 要求"不能包含控制字符"；修复前该路径无此校验。同时验证被拒请求不改写存量值。

##### 前提数据准备

已存在可用集群，并 PATCH 一个已知基线描述。

##### 执行步骤

1. PATCH 设置基线描述。
2. PATCH 提交含 `\u0001` 的 `description`，验证 422。
3. GET 回读，验证 `description` 仍为基线值。

##### 请求参数

```json
{ "description": "abc\u0001def" }
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 `control characters`  
**Data**：null

**GET 回读**：`description` == 基线值

---

#### 9.4.27 CL-4-027：PATCH 非法 description 不污染存量值（原子性）

##### 设计思路

校验发生在 `xreq.Bind` 阶段、早于 `UpdateCluster`；被拒请求不得改写库中既存描述（与 CL-4-104 同模式）。

##### 前提数据准备

已存在可用集群，并 PATCH 一个已知基线描述。

##### 执行步骤

1. PATCH 设置基线描述。
2. PATCH 提交 257 字节 `description`，验证 422。
3. GET 回读，验证 `description` 仍为基线值。

##### 请求参数

```json
{ "description": "<257 个字节的字符串>" }
```

##### 预期返回结果

**ErrNum**：422  
**Data**：null

**GET 回读**：`description` == 基线值

---

## 10. 删除集群

### 10.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Cluster |
| 接口名称 | 删除集群 |
| 方法 | DELETE |
| 路径 | `/open-api/v1/clusters/{cluster_name}` |
| 说明 | 删除集群，自动级联清理关联的实例池和子集群；删除前会扫描全部 global/entity/apikey 路由规则（不经过分页），若任意规则的 `targets` 或 `fallbacks` 引用了该集群，则拒绝删除 |

### 10.2 接口参数说明

#### 10.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| cluster_name | string | Y | 集群名字 |

#### 10.2.2 返回数据字段

同创建接口。

### 10.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| CL-5-001 | 删除集群 | 正常参数 | 级联清理，再次查询返回 404 |
| CL-5-002 | 删除不存在的集群 | 异常参数 | 验证 ErrNum=404 |
| CL-5-003 | 删除被 global 路由规则 target 引用的集群 | 业务规则 | 验证 ErrNum=409，错误信息包含规则名 |
| CL-5-004 | 删除被 global 路由规则 fallback 引用的集群 | 业务规则 | 验证 ErrNum=409，fallback 同样会拦截删除 |
| CL-5-005 | 删除被 entity 路由规则 target 引用的集群 | 业务规则 | 验证 ErrNum=409 |
| CL-5-006 | 删除被 apikey 路由规则 target 引用的集群 | 业务规则 | 验证 ErrNum=409 |
| CL-5-007 | 解除引用后可删除集群 | 正常参数 | 更新 global 路由规则移除引用后删除成功 |
| CL-5-008 | 路由规则引用其他集群时可删除 | 正常参数 | 规则引用 cluster_other，删除 cluster_unref 成功 |
| CL-5-009 | 删除单字符名称的集群 | 正常参数 | 创建单字符名称集群后删除成功，再次查询返回 404（回归 issue #130） |

### 10.4 测试场景详细设计

#### 10.4.1 CL-5-001：删除集群（正常参数）

##### 设计思路

验证删除集群成功，并级联清理实例池和子集群。

##### 前提数据准备

已创建集群 `cluster_to_del`。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/cluster_to_del`。
2. 验证返回被删除集群对象。
3. 再次查询集群，验证返回 404。

##### 请求参数

URI：`cluster_to_del`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "cluster_to_del" | Equals |

---

#### 10.4.2 CL-5-002：删除不存在的集群（异常参数）

##### 设计思路

验证删除不存在的集群时返回 404。

##### 前提数据准备

无

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/non_existent_cluster`。
2. 验证返回错误码。

##### 请求参数

URI：`non_existent_cluster`

##### 预期返回结果

**ErrNum**：404  
**ErrMsg**：集群不存在的错误信息  
**Data**：null

---

#### 10.4.3 CL-5-003：删除被 global 路由规则 target 引用的集群

##### 设计思路

验证 global 路由规则的 `targets` 引用集群时，删除该集群会被拒绝。

##### 前提数据准备

1. 创建集群 `cluster_global_ref`。
2. 创建另一集群 `cluster_other` 用于后续解除引用场景。
3. 通过 `PUT /open-api/v1/global-route-rules` 设置 global 路由规则，使其 `targets` 引用 `cluster_global_ref`。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/cluster_global_ref`。
2. 验证返回业务异常。

##### 请求参数

URI：`cluster_global_ref`

Global 路由规则准备请求：

```json
{
    "rules": [
        {
            "name": "global-ref",
            "Cond": "default_t()",
            "targets": [
                {"ClusterName": "cluster_global_ref", "Model": "", "Weight": 100}
            ],
            "fallbacks": []
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：包含 "Rule global-ref Refer To This Cluster" 或本地化的“集群被转发规则 global-ref 引用”  
**Data**：null

---

#### 10.4.4 CL-5-004：删除被 global 路由规则 fallback 引用的集群

##### 设计思路

验证 global 路由规则的 `fallbacks` 引用集群时，删除该集群同样会被拒绝。

##### 前提数据准备

1. 创建集群 `cluster_global_fb`。
2. 创建另一集群 `cluster_target`。
3. 通过 `PUT /open-api/v1/global-route-rules` 设置 global 路由规则，target 指向 `cluster_target`，fallback 指向 `cluster_global_fb`。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/cluster_global_fb`。
2. 验证返回业务异常。

##### 请求参数

URI：`cluster_global_fb`

Global 路由规则准备请求：

```json
{
    "rules": [
        {
            "name": "global-fb-ref",
            "Cond": "default_t()",
            "targets": [
                {"ClusterName": "cluster_target", "Model": "", "Weight": 100}
            ],
            "fallbacks": [
                {"ClusterName": "cluster_global_fb", "Model": ""}
            ]
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：包含 "Rule global-fb-ref Refer To This Cluster" 或本地化的“集群被转发规则 global-fb-ref 引用”  
**Data**：null

---

#### 10.4.5 CL-5-005：删除被 entity 路由规则 target 引用的集群

##### 设计思路

验证 Entity 级别的路由规则引用集群时，删除该集群会被拒绝。

##### 前提数据准备

1. 创建 Entity-Type `type_entity_ref`。
2. 创建 Entity `entity_ref`。
3. 创建集群 `cluster_entity_ref`。
4. 通过 `PATCH /open-api/v1/entities/{id}` 为 Entity 设置路由规则，target 引用 `cluster_entity_ref`。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/cluster_entity_ref`。
2. 验证返回业务异常。

##### 请求参数

URI：`cluster_entity_ref`

Entity 路由规则准备请求：

```json
{
    "route_rules": {
        "enabled": true,
        "rules": [
            {
                "name": "entity-ref",
                "Cond": "default_t()",
                "targets": [
                    {"ClusterName": "cluster_entity_ref", "Model": "", "Weight": 100}
                ],
                "fallbacks": []
            }
        ]
    }
}
```

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：包含 "Rule entity-ref Refer To This Cluster" 或本地化的“集群被转发规则 entity-ref 引用”  
**Data**：null

---

#### 10.4.6 CL-5-006：删除被 apikey 路由规则 target 引用的集群

##### 设计思路

验证 API-Key 级别的路由规则引用集群时，删除该集群会被拒绝。

##### 前提数据准备

1. 创建 API-Key `apikey_ref`（不绑定 Entity）。
2. 创建集群 `cluster_apikey_ref`。
3. 通过 `PATCH /open-api/v1/api-keys/{id}` 为 API-Key 设置路由规则，target 引用 `cluster_apikey_ref`。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/cluster_apikey_ref`。
2. 验证返回业务异常。

##### 请求参数

URI：`cluster_apikey_ref`

API-Key 路由规则准备请求：

```json
{
    "route_rules": {
        "enabled": true,
        "rules": [
            {
                "name": "apikey-ref",
                "Cond": "default_t()",
                "targets": [
                    {"ClusterName": "cluster_apikey_ref", "Model": "", "Weight": 100}
                ],
                "fallbacks": []
            }
        ]
    }
}
```

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：包含 "Rule apikey-ref Refer To This Cluster" 或本地化的“集群被转发规则 apikey-ref 引用”  
**Data**：null

---

#### 10.4.7 CL-5-007：解除引用后可删除集群

##### 设计思路

验证当路由规则不再引用目标集群后，删除可以成功。

##### 前提数据准备

1. 创建集群 `cluster_after_unref` 和 `cluster_other2`。
2. 通过 `PUT /open-api/v1/global-route-rules` 设置 global 路由规则引用 `cluster_after_unref`。
3. 确认删除 `cluster_after_unref` 被拒绝。
4. 更新 global 路由规则，将 target 改为引用 `cluster_other2`（或清空 rules）。

##### 执行步骤

1. 再次发送 DELETE 请求到 `/open-api/v1/clusters/cluster_after_unref`。
2. 验证返回 200，并再次查询返回 404。

##### 请求参数

URI：`cluster_after_unref`

更新后的 Global 路由规则（示例）：

```json
{
    "rules": [
        {
            "name": "global-ref",
            "Cond": "default_t()",
            "targets": [
                {"ClusterName": "cluster_other2", "Model": "", "Weight": 100}
            ],
            "fallbacks": []
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success  
**Data**：返回被删除集群对象

---

#### 10.4.8 CL-5-008：路由规则引用其他集群时可删除

##### 设计思路

验证路由规则引用的是其他集群时，不会误拦截当前集群的删除。

##### 前提数据准备

1. 创建集群 `cluster_unref` 和 `cluster_referred`。
2. 通过 `PUT /open-api/v1/global-route-rules` 设置 global 路由规则引用 `cluster_referred`。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/cluster_unref`。
2. 验证返回 200，并再次查询返回 404。

##### 请求参数

URI：`cluster_unref`

Global 路由规则准备请求：

```json
{
    "rules": [
        {
            "name": "global-other-ref",
            "Cond": "default_t()",
            "targets": [
                {"ClusterName": "cluster_referred", "Model": "", "Weight": 100}
            ],
            "fallbacks": []
        }
    ]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success  
**Data**：返回被删除集群对象

#### 10.4.9 CL-5-009：删除单字符名称的集群（正常参数）

##### 设计思路

回归测试（issue #130）：创建集群允许名称长度为 1 个字符，但删除/单查共用的 URI 参数曾强校验
`min=2`，导致单字符名称的集群"建得成、删不掉"。验证单字符名称的集群可正常删除，删除后查询返回 404。

##### 前提数据准备

已创建名称为单字符（如 `c`）的集群。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/clusters/c`。
2. 验证 ErrNum=200。
3. 再次发送 GET 请求到 `/open-api/v1/clusters/c`，验证 ErrNum=404。

##### 请求参数

URI：`c`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success  
**Data**：返回被删除集群对象

---

## 11. 依赖与数据准备

1. 模型协议 `provider_type` 取值参考 `providers.model_protocols` 枚举，或由 `/providers` 接口返回的 provider 对象中的 `model_protocols` 字段。
2. 创建集群会级联写入实例池、子集群、lb_matrices，测试后需清理。
3. 测试环境数据库需包含产品线初始化数据，以支持 `{product_name}.{cluster_name}` 实例池命名。
4. 涉及 global/entity/apikey 路由规则的用例，需先通过对应接口写入规则，测试结束后清理规则或清空 `rules` 数组，避免影响其他用例。

## 12. normalize_upstream_error 用例（2026-10-06 上游错误体归一）

### 12.1 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| CL-1-101 | 全字段创建 normalize_upstream_error | 正常参数 | 验证五字段写入与 GET 回读一致 |
| CL-1-102 | 部分字段创建（仅 enabled） | 正常参数 | 验证未配置字段回读为 null（全指针结构，与 key_affinity 一致） |
| CL-1-103 | unrecognized_action 非法值 | 合法性条件 | 验证 ErrNum=422 且消息归因该字段；导出产物防泄漏 |
| CL-1-104 | max_body_bytes 越界（-1 / 4MB+1） | 合法性条件 | 验证 ErrNum=422；边界 0 与 4MB 为合法（见 CL-1-105/106） |
| CL-1-105 | max_body_bytes=0（用默认） | 正常参数 | 验证 200 且导出 MaxBodyBytes=0（BFE 按 65536 处理） |
| CL-1-106 | max_body_bytes=4194304（上限边界） | 正常参数 | 验证 200 |
| CL-4-101 | PATCH 省略字段保留原值 | 更新语义 | 全字段创建后 PATCH 只改 description，GET 断言 normalize_upstream_error 五字段原值不变（检查项 #1） |
| CL-4-102 | PATCH 整体替换 nue 对象 | 更新语义 | 对齐 CL-4-011 key_affinity 模式：PATCH 提交完整 nue 对象（五字段全量、部分改值），GET 断言新值全量生效 |
| CL-4-103 | PATCH llm_config 省略 nue = 清空 | 更新语义 | `llm_config` PATCH 为整体替换（与 `keys` 全量替换同合同语义）：省略 `normalize_upstream_error` 即清空，GET 断言为 null |
| CL-4-104 | PATCH 非法值拒绝 + 回读零变更 | 合法性条件 | 非法 body PATCH → 422 且消息归因字段；GET 逐字段不变（检查项 #3）。注：`/clusters` 无 PUT 接口（405 实证），对称性检查项 #2 不适用 |

### 12.2 详细设计要点

- 合同依据：`design-docs/api-define/OpenAPI接口定义/clusters.md` 表"上游错误归一配置"
  与 `modifications/2026-10-06-upstream-error-normalization/api-changes.md`；
- 防泄漏（检查项 #4）：CL-1-103/104 的集群名不得出现在 InnerAPI 导出
  `ClusterConf.Config` 中（该集群未创建成功）；
- 审计（检查项 #7）：沿用 create/update 既有审计断言模式，本批用例不重复展开
  （字段随 llm_config 整体快照，无独立审计键）；
- 并发（检查项 #10）：本变更为纯配置透传，无 ID 生成/行锁路径，**不适用**，
  不新增 `//go:build mysql` 用例。

## 13. 缺价动作配置 `price_miss_action`（issue #1479）

合同依据：`design-docs/api-define/OpenAPI接口定义/clusters.md` LLM 配置表
`price_miss_action` 行与 `design-docs/modifications/2026-10-30-issue-1479-price-miss-action/api-changes.md`。

本批用例（create：CL-1-121/122/123；update：CL-4-121/122/123）覆盖：

| 编号 | 验证点 | 检查项 |
|------|--------|--------|
| CL-1-121 | 三值 `fallback_chat`/`bill_zero`/`reject` 均可创建，GET 回读一致，InnerAPI 导出 `AIConf.PriceMissAction`（snake→Pascal 映射）一致 | #12 导出断言 |
| CL-1-122 | 省略字段回读为 null，导出为空（bfe 归一为 `fallback_chat`） | 缺省语义 |
| CL-1-123 | 非法值（`reject_all`）4xx 且**不创建集群**（GET 回读零变更） | #2 4xx 零变更 |
| CL-4-121 | PATCH 只改无关字段时 `price_miss_action` 原值保留 | #1 省略字段保留 |
| CL-4-122 | PATCH 整体替换 `llm_config.price_miss_action` 生效 | #3 变更语义 |
| CL-4-123 | 非法值 PATCH 4xx 后回读零变更 | #2 4xx 零变更 |

### 13.2 设计要点

- 三值是枚举字符串，非法值由 `lib/validate` 拦截（与 bfe `AIConfCheck` 同源），不需要单独的错误码；
- 导出映射为 snake_case → PascalCase（`price_miss_action` → `PriceMissAction`），未设置为空串；bfe 侧空串归一为 `fallback_chat`（已由 bfe SC42 覆盖三态行为本身）；
- 纯配置透传：无 ID 生成/行锁路径，检查项 #10（MySQL 并发）不适用，不新增 `//go:build mysql` 用例；
- 字段随 `llm_config` 整体快照落审计，#7 不单独展开。

## 14. 注意事项

1. v0.3.0 已删除 `/clusters/{cluster_name}/ready`、`/model-providers`、`/models`。
2. 返回中不应出现 `ready`、`sub_clusters`、`scheduler`、`Instance.tags`、`llm_config.service_name`、`llm_config.group`。
3. `basic.retries.max_retry_in_cluster` 对应底层 `max_retry_in_subcluster`。
4. 更新时不支持修改 `sub_clusters`/`scheduler`，需通过 `instance_pool` 调整实例。
5. v0.0.7 起，`llm_config.key` 已移除，测试用例中应使用 `llm_config.keys` + `llm_config.key_policy`；`keys` 为全量替换语义，更新时需传入完整列表。
6. 测试环境 `SkipTokenValidate=true`，无需认证头。
