# /entities

## 1. 数据模型

```json
{
  "id": "ent-001",
  "name": "op",
  "description": "运营部，负责线上业务",
  "type": "dep",
  "parent_id": null,
  "allow_models": ["*"],
  "block_models": [],
  "quota_plan": {
    "unlimited": false,
    "pass_when_no_enough_quota": false,
    "quota": 100000000,
    "unit": "total_token",
    "reset_period": "monthly"
  },
  "rate_limit_policy": {
    "enabled": false,
    "rules": {
      "tpm": [
        {"name": "tpm_1min", "model": "*", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1}
      ],
      "rpm": [
        {"name": "rpm_1min", "model": "*", "window_minutes": 1, "max_requests": 100}
      ],
      "max_concurrency": -1
    }
  },
  "route_rules": {
    "enabled": false,
    "rules": []
  },
  "create_time": 1716883200,
  "update_time": 1716883200
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 |
|------|------|------|----------|
| `id` | string | Entity唯一标识 | 系统生成，如`ent-001` |
| `name` | string | Entity名称 | 在全局范围内唯一 | 必填；类型为 [EntityName](./00-common.md#17-entity-名称entityname)；全局唯一 |
| `description` | string | Entity描述 | 自定义 | 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符 |
| `type` | string | Entity类型 | 必须引用已定义的Entity-Type | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename)；必须引用已存在的 Entity-Type |
| `parent_id` | string | 父Entity ID | 为空表示根节点 |
| `allow_models` | []string | 允许访问的模型白名单 | 包含"*"表示允许访问所有模型，**默认值为允许访问所有模型** |
| `block_models` | []string | 禁止访问的模型黑名单 | 包含"*"表示禁止访问所有模型，**默认值为空数组**；若某模型同时出现在`allow_models`和`block_models`中，以`block_models`为准；`block_models` 中的模型名无需为已配置的 `AIModel`（不必出现在 `/clusters` 的 `llm_config.models` 中） |
| `quota_plan` | object | 对该Entity设置的配额计划 | 类型为 [QuotaPlan](./00-common.md#公共参数类型)，**不会为空** |
| `rate_limit_policy` | object | 对该Entity设置的限流策略 | 类型为 [RateLimitPolicy](./00-common.md#公共参数类型)，**不会为空** |
| `route_rules` | object | 对该Entity设置的路由规则 | 类型为 [RouteRules](./00-common.md#公共参数类型)，**不会为空** |
| `create_time` | int64 | 创建时间 | Unix时间戳（秒） |
| `update_time` | int64 | 更新时间 | Unix时间戳（秒） |

> **说明**：`quota_plan` 中的 `quota`、`balance.used`、`balance.remaining` 类型为 `number`，支持 `unit="total_token"`（整数）和 `unit="RMB"`（最多 4 位小数展示）。详见 [QuotaPlan](./00-common.md#公共参数类型)。

---

## 2. 接口清单

### 2.1 创建Entity

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 创建Entity | - |
| 端点 | /entities | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| name | string | Entity名称 | Y | 全局唯一 | 必填；类型为 [EntityName](./00-common.md#17-entity-名称entityname)；全局唯一 |
| description | string | Entity描述 | N | - | 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符 |
| type | string | Entity类型 | Y | 必须引用已定义的Entity-Type | 必填；类型为 [EntityTypeName](./00-common.md#16-entity-type-名称entitytypename)；必须引用已存在的 Entity-Type |
| parent_id | string | 父Entity ID | N | 为空表示根节点 | 若传入非空值，父 Entity 必须存在，且父 Entity 对应 Entity-Type 的 level 必须小于本 Entity 对应 Entity-Type 的 level |
| allow_models | []string | 允许访问的模型白名单 | N | 包含"*"表示允许访问所有模型，**默认值为允许访问所有模型** | 每个元素类型为 [AIModel](./00-common.md#5-ai-模型名称aimodel)；包含 `"*"` 时表示允许访问所有模型 |
| block_models | []string | 禁止访问的模型黑名单 | N | 包含"*"表示禁止访问所有模型，**默认值为空数组**；若某模型同时出现在`allow_models`和`block_models`中，以`block_models`为准 | 每个元素为非空字符串；包含 `"*"` 时表示禁止访问所有模型；元素无需为已配置的 `AIModel`（不必出现在 `/clusters` 的 `llm_config.models` 中） |
| quota_plan | object | 配额计划 | N | 同2.1中quota_plan结构（不含balance），若未设置则使用默认值 | 类型为 [QuotaPlan](./00-common.md#公共参数类型) |
| rate_limit_policy | object | 限流策略 | N | 同2.1中rate_limit_policy结构，若未设置则使用默认值（enabled=false） | 类型为 [RateLimitPolicy](./00-common.md#公共参数类型) |
| route_rules | object | 路由规则 | N | 同2.1中route_rules结构，若未设置则使用默认值（enabled=false, rules为空） | 类型为 [RouteRules](./00-common.md#公共参数类型) |

**约束**

- `type` 必须引用系统中已存在的Entity-Type
- `parent_id` 若不为空，该父Entity对应的Entity-Type的 `level` 必须**小于**本Entity对应的Entity-Type的 `level`（数字越小级别越高，父节点级别必须更高）
- `name` 必填；类型为 [EntityName](./00-common.md#17-entity-名称entityname)；全局唯一
- `description` 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符。
- `allow_models` 每个元素类型为 [AIModel](./00-common.md#5-ai-模型名称aimodel)；包含 `"*"` 时表示允许访问所有模型。
- `block_models` 每个元素为非空字符串；包含 `"*"` 时表示禁止访问所有模型；元素无需为已配置的 `AIModel`（不必出现在 `/clusters` 的 `llm_config.models` 中）。
- `quota_plan`、`rate_limit_policy`、`route_rules` 的字段及合法性条件分别见 [QuotaPlan](./00-common.md#公共参数类型)、[RateLimitPolicy](./00-common.md#公共参数类型)、[RouteRules](./00-common.md#公共参数类型) 公共类型定义。
- `rate_limit_policy.rules` 中的 `batch_limits` 段（批量任务限流维度）与独立策略资源一致生效：四维度均按发起请求的 API-Key 计数（与本 Entity 挂载的 API-Key 各自的活跃批量任务数比对）、与 `model` 无关，省略/`null` 表示该策略不参与批量限流，不配置不代表批量不限流（仍受数据面 `mod_ai_batch.data` 全局硬顶约束）；字段定义见 [RateLimitPolicy](./00-common.md#公共参数类型)。

**HTTP BODY参数示例**

```json
{
    "name": "op",
    "description": "运营部，负责线上业务",
    "type": "dep",
    "parent_id": null,
    "allow_models": ["*"],
    "block_models": [],
    "quota_plan": {
        "unlimited": false,
        "pass_when_no_enough_quota": false,
        "quota": 100000000,
        "unit": "total_token",
        "reset_period": "monthly"
    },
    "rate_limit_policy": {
        "enabled": true,
        "rules": {
            "tpm": [
                {"name": "tpm_1min", "model": "*", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1}
            ],
            "rpm": [
                {"name": "rpm_1min", "model": "*", "window_minutes": 1, "max_requests": 100}
            ],
            "max_concurrency": 50,
            "batch_limits": {
                "max_create_rpm": 60,
                "max_active_batches": 10,
                "max_file_bytes": 104857600,
                "max_file_lines": 1000000
            }
        }
    },
    "route_rules": {
        "enabled": true,
        "rules": [
            {
                "name": "entity-default",
                "cond": "default_t()",
                "targets": [
                    {
                        "cluster_name": "cluster_entity",
                        "model": "",
                        "weight": 100
                    }
                ],
                "fallbacks": []
            }
        ]
    }
}
```

**执行逻辑**

1. 校验参数合法性
2. 若未传入 `quota_plan`，使用默认值（unlimited=true, pass_when_no_enough_quota=false, quota=0, unit=total_token, reset_period=never）
3. 若未传入 `rate_limit_policy`，使用默认值（enabled=false, rules为空）
4. 若未传入 `route_rules`，使用默认值（enabled=false, rules为空）
5. 若传入 `quota_plan`，创建对应的QuotaBalance（remaining = quota，used = 0）
6. 若传入 `rate_limit_policy`，创建RateLimitPolicy
7. 若传入 `route_rules`，创建路由规则配置
8. 写入entity
9. 返回结果，含完整的嵌套结构（不含balance）

**返回数据（Data内容）**

字段同4.1数据模型，包含系统生成的id、create_time、update_time（不含balance）。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "id": "ent-001",
        "name": "op",
        "description": "运营部，负责线上业务",
        "type": "dep",
        "parent_id": null,
        "allow_models": ["*"],
        "block_models": [],
        "quota_plan": {
            "unlimited": false,
            "pass_when_no_enough_quota": false,
            "quota": 100000000,
            "unit": "total_token",
            "reset_period": "monthly"
        },
        "rate_limit_policy": {
            "enabled": true,
            "rules": {
                "tpm": [{"name": "tpm_1min", "model": "*", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1}],
                "rpm": [{"name": "rpm_1min", "model": "*", "window_minutes": 1, "max_requests": 100}],
                "max_concurrency": 50
            }
        },
        "route_rules": {
            "enabled": true,
            "rules": [
                {
                    "name": "entity-default",
                    "cond": "default_t()",
                    "targets": [
                        {
                            "cluster_name": "cluster_entity",
                            "model": "",
                            "weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        },
        "create_time": 1716883200,
        "update_time": 1716883200
    }
}
```

---

### 2.2 查询Entity列表

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询Entity列表 | - |
| 端点 | /entities | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| page | int | 页码 | N | 默认1 | 有效值 >0；<=0 按 1 处理 |
| page_size | int | 每页条数 | N | 默认20，最大100 | 有效范围 1-100；<1 按 20 处理，>100 按 100 处理 |
| id | string | 按Entity ID过滤 | N | - | - |
| name | string | 按Entity名称过滤 | N | - | - |
| type | string | 按类型过滤 | N | - | - |
| parent_id | string | 按父Entity过滤 | N | - | - |
| quota_plan_id | int64 | 按配额计划ID过滤 | N | - | - |
| route_rules_id | int64 | 按路由规则ID过滤 | N | - | - |


**返回数据（Data内容）**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| list | []object | Entity列表 | 包含完整Entity字段 |
| pagination | object | 分页信息 | - |
| pagination.page | int | 当前页码 | - |
| pagination.page_size | int | 每页条数 | - |
| pagination.total | int | 总条数 | 满足本次查询过滤条件的总条数（与分页无关） |

**list 对象字段说明**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| id | string | Entity唯一标识 | - |
| name | string | Entity名称 | - |
| description | string | Entity描述 | - |
| type | string | Entity类型 | - |
| parent_id | string | 父Entity ID | - |
| allow_models | []string | 允许访问的模型白名单 | - |
| block_models | []string | 禁止访问的模型黑名单 | - |
| quota_plan | object | 配额计划 | 不含balance字段 |
| rate_limit_policy | object | 限流策略 | - |
| route_rules | object | 路由规则 | 类型为 [RouteRules](./00-common.md#公共参数类型) |
| create_time | int64 | 创建时间 | Unix时间戳（秒） |
| update_time | int64 | 更新时间 | Unix时间戳（秒） |

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "list": [
            {
                "id": "ent-001",
                "name": "op",
                "description": "运营部，负责线上业务",
                "type": "dep",
                "parent_id": null,
                "allow_models": ["*"],
                "block_models": [],
                "quota_plan": {
                    "unlimited": false,
                    "pass_when_no_enough_quota": false,
                    "quota": 100000000,
                    "unit": "total_token",
                    "reset_period": "monthly"
                },
                "rate_limit_policy": {
                    "enabled": true,
                    "rules": {
                        "tpm": [{"name": "tpm_1min", "model": "*", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1}],
                        "rpm": [{"name": "rpm_1min", "model": "*", "window_minutes": 1, "max_requests": 100}],
                        "max_concurrency": 50
                    }
                },
                "route_rules": {
                    "enabled": true,
                    "rules": [
                        {
                            "name": "entity-default",
                            "cond": "default_t()",
                            "targets": [
                                {
                                    "cluster_name": "cluster_entity",
                                    "model": "",
                                    "weight": 100
                                }
                            ]
                        }
                    ]
                },
                "create_time": 1716883200,
                "update_time": 1716883200
            },
            {
                "id": "ent-bfe-001",
                "name": "bfe",
                "description": "BFE 网关团队",
                "type": "team",
                "parent_id": "ent-001",
                "allow_models": ["*"],
                "block_models": [],
                "quota_plan": {
                    "unlimited": true
                },
                "rate_limit_policy": {
                    "enabled": false
                },
                "route_rules": {
                    "enabled": false,
                    "rules": []
                },
                "create_time": 1716883200,
                "update_time": 1716883200
            }
        ],
        "pagination": {
            "page": 1,
            "page_size": 20,
            "total": 2
        }
    }
}
```

---

### 2.3 查询单个Entity

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询单个Entity | - |
| 端点 | /entities/{id} | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | Entity标识 | Y | - | 必填、非空 |

**返回数据（Data内容）**

字段同4.1数据模型，但`quota_plan`中的`balance`字段不返回（仅调用独立quota-plan查询接口时返回）。

---

### 2.4 全量更新Entity

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量更新Entity | - |
| 端点 | /entities/{id} | - |
| 版本 | v1 | - |
| method | PUT | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | Entity标识 | Y | - | 必填、非空 |

**输入参数（Body）**

同4.2.1创建Entity的Body参数。

**约束**

- `type` 不可修改（创建后固定）。
- 若修改 `parent_id`，新父Entity对应的Entity-Type的 `level` 必须**小于**本Entity对应的Entity-Type的 `level`。
- `name` 必填；类型为 [EntityName](./00-common.md#17-entity-名称entityname)；全局唯一，不可与其他Entity冲突。
- `description` 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符。**全量语义下省略 `description` 将清空已有描述（重置为空字符串）**。
- `quota_plan`、`rate_limit_policy`、`route_rules` 的字段及合法性条件分别见 [QuotaPlan](./00-common.md#公共参数类型)、[RateLimitPolicy](./00-common.md#公共参数类型)、[RouteRules](./00-common.md#公共参数类型) 公共类型定义。
- `rate_limit_policy.rules` 中的 `batch_limits` 段（批量任务限流维度）与独立策略资源一致生效：四维度均按发起请求的 API-Key 计数（与本 Entity 挂载的 API-Key 各自的活跃批量任务数比对）、与 `model` 无关，省略/`null` 表示该策略不参与批量限流，不配置不代表批量不限流（仍受数据面 `mod_ai_batch.data` 全局硬顶约束）；字段定义见 [RateLimitPolicy](./00-common.md#公共参数类型)。

**执行逻辑**

1. 若传入新的 `quota_plan`：
   - 若 `quota_plan.unlimited` 为 `false`，且 `quota` 的值发生了改变（单位不变）：保留 `balance.used`，按 `balance.remaining = max(0, 新quota - balance.used)` 调整余额，并同步调整 Redis 剩余量。
   - 若 `quota_plan.unit` 发生变化（如 `total_token` ↔ `RMB`）：由于新旧单位无法直接换算，对 balance 进行重置（remaining = 新的quota，used = 0），并将 Redis 剩余量重置为新的quota。
   - 若 `quota_plan.unlimited` 由 `true` 改为 `false`：创建新的 balance（remaining = quota，used = 0），并将 Redis 剩余量重置为 quota。
   - 若 `quota_plan.unlimited` 由 `false` 改为 `true`：将 balance 置为无限制状态（used = 0，remaining = 一个足够大的 sentinel 值），并将 Redis 剩余量重置为 sentinel。
   - 若仅 `quota_plan` 的 `pass_when_no_enough_quota`、`reset_period` 等字段变化，而 `quota`、`unit`、`unlimited` 均未变化：不调整 balance。
2. 若传入新的 `rate_limit_policy` 且 `enabled` 为 `true`：
   - 对于 `tpm` 和 `rpm` 规则：
     - 若规则的 `name` 与之前的规则相同，但其他参数发生变化，则重置该规则对应的计数器
     - 若规则的 `name` 是新的（之前不存在），则创建新的计数器
     - 若之前存在的规则 `name` 在新传入的规则中消失了，则删除该规则对应的计数器
   - 若 `enabled` 由 `true` 改为 `false`，则保留计数器数据但不执行限流检查
   - 若 `enabled` 由 `false` 改为 `true`，则根据当前规则创建/恢复计数器
3. 若传入新的 `route_rules`，替换对应的路由规则配置
4. 更新Entity其他字段

**返回数据（Data内容）**

同4.1数据模型（不含balance）。

---

### 2.5 部分更新Entity

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 部分更新Entity | - |
| 端点 | /entities/{id} | - |
| 版本 | v1 | - |
| method | PATCH | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | Entity标识 | Y | - | 必填、非空 |

**输入参数（Body）**

同4.2.1创建Entity的Body参数，仅传需修改字段。

**约束**

- `type` 不可修改。
- 若修改 `parent_id`，新父Entity对应的Entity-Type的 `level` 必须**小于**本Entity对应的Entity-Type的 `level`。
- `name` 必填；类型为 [EntityName](./00-common.md#17-entity-名称entityname)；全局唯一，不可与其他Entity冲突。
- `description` 非必填；若传入，长度 0-255 字节（按 UTF-8 字节计）；不能包含控制字符。省略时保持原值；显式传入空字符串 `""` 表示清空描述。
- `quota_plan`、`rate_limit_policy`、`route_rules` 的字段及合法性条件分别见 [QuotaPlan](./00-common.md#公共参数类型)、[RateLimitPolicy](./00-common.md#公共参数类型)、[RouteRules](./00-common.md#公共参数类型) 公共类型定义。
- `rate_limit_policy.rules` 中的 `batch_limits` 段（批量任务限流维度）与独立策略资源一致生效：四维度均按发起请求的 API-Key 计数（与本 Entity 挂载的 API-Key 各自的活跃批量任务数比对）、与 `model` 无关，省略/`null` 表示该策略不参与批量限流，不配置不代表批量不限流（仍受数据面 `mod_ai_batch.data` 全局硬顶约束）；字段定义见 [RateLimitPolicy](./00-common.md#公共参数类型)。
- 修改 `quota_plan.quota`（单位不变）时，保留 `balance.used`，按 `新quota - used` 重新计算 `balance.remaining`；修改 `quota_plan.unit` 或 `quota_plan.unlimited` 时，会重置 `balance.used = 0`；仅修改 `quota_plan` 其他字段不会调整 balance。
- 若修改 `route_rules`，视为全量替换该路由规则配置。

**执行逻辑**

1. 若传入新的 `quota_plan`：
   - 若 `quota_plan.unlimited` 为 `false`，且 `quota` 的值发生了改变（单位不变）：保留 `balance.used`，按 `balance.remaining = max(0, 新quota - balance.used)` 调整余额，并同步调整 Redis 剩余量。
   - 若 `quota_plan.unit` 发生变化（如 `total_token` ↔ `RMB`）：由于新旧单位无法直接换算，对 balance 进行重置（remaining = 新的quota，used = 0），并将 Redis 剩余量重置为新的quota。
   - 若 `quota_plan.unlimited` 由 `true` 改为 `false`：创建新的 balance（remaining = quota，used = 0），并将 Redis 剩余量重置为 quota。
   - 若 `quota_plan.unlimited` 由 `false` 改为 `true`：将 balance 置为无限制状态（used = 0，remaining = 一个足够大的 sentinel 值），并将 Redis 剩余量重置为 sentinel。
   - 若仅 `quota_plan` 的 `pass_when_no_enough_quota`、`reset_period` 等字段变化，而 `quota`、`unit`、`unlimited` 均未变化：不调整 balance。
2. 若传入新的 `rate_limit_policy` 且 `enabled` 为 `true`：
   - 对于 `tpm` 和 `rpm` 规则：
     - 若规则的 `name` 与之前的规则相同，但其他参数发生变化，则重置该规则对应的计数器
     - 若规则的 `name` 是新的（之前不存在），则创建新的计数器
     - 若之前存在的规则 `name` 在新传入的规则中消失了，则删除该规则对应的计数器
   - 若 `enabled` 由 `true` 改为 `false`，则保留计数器数据但不执行限流检查
   - 若 `enabled` 由 `false` 改为 `true`，则根据当前规则创建/恢复计数器
3. 若传入新的 `route_rules`，替换对应的路由规则配置
4. 更新Entity其他字段

**返回数据（Data内容）**

同4.1数据模型（不含balance）。

---

### 2.6 删除Entity

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 删除Entity | - |
| 端点 | /entities/{id} | - |
| 版本 | v1 | - |
| method | DELETE | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | Entity标识 | Y | - | 必填、非空 |

**返回数据（Data内容）**

Data为null。

**约束**：
- 若该Entity存在子Entity（有其他Entity的parent_id指向它），返回ErrNum=409
- 若该Entity已被任何API-Key挂载，返回ErrNum=409

**说明**

- 删除Entity时，级联删除其专属的quota_plan、rate_limit_policy、route_rules及底层资源（如果这些资源不被其他API-Key或Entity引用）

---

### 2.7 查询配额计划（含Balance）

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询Entity的配额计划（含实时余额） | - |
| 端点 | /entities/{id}/quota-plan | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | Entity标识 | Y | - | 必填、非空 |

**返回数据（Data内容）**

完整的quota_plan对象，包含`balance`字段（含`used`和`remaining`）。

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| unlimited | bool | 是否无限配额 | - |
| pass_when_no_enough_quota | bool | 配额不足时是否放行 | - |
| quota | number | 配额总量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| unit | string | 配额单位 | 可选值：`total_token`、`RMB` |
| reset_period | string | 配额重置周期 | - |
| balance | object | 余额状态（只读） | 包含used和remaining |

**balance结构**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| used | number | 已用量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| remaining | number | 剩余量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |

---

### 2.8 重置配额余额（QuotaBalance Reset）

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 重置Entity的配额余额 | - |
| 端点 | /entities/{id}/quota-plan/reset | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | Entity标识 | Y | - | 必填、非空 |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| quota | number | 重置后的配额总量 | N | 若传入则更新quota并同步重置balance；若不传则按当前quota重置 | 非负数；`unit=total_token` 时必须为整数；`unit=RMB` 时取值范围 0 ~ 90000000.00（9000 万元），小数位不超过 8 位 |
| reason | string | 重置原因 | N | 用于审计 | - |

**执行逻辑**

1. 若 Entity 不存在，返回 404；找到该 Entity 的 quota_plan：若未关联 quota plan 或 `unlimited=true`，返回 422（Param Illegal；不产生配额变更，并记录 `status=2` 的操作日志）；若传入 quota，更新 quota_plan.quota 为新的值
2. 触发balance的reset：
   - balance.remaining = 当前quota（或新的quota）
   - balance.used = 0

**失败响应说明**

| HTTP status | ErrNum | ErrMsg | 触发条件 |
|------|------|------|------|
| 404 | 404 | `Entity Record Not Exist` | 路径中的 Entity 不存在 |
| 422 | 422 | `Param Illegal: <原因>` | 未关联 quota plan；unlimited 配额计划（`cannot reset balance for unlimited quota`）；quota 参数非法 |
| 500 | 500 | `Unknown Exception: <原因>` | 内部故障 |

**返回数据（Data内容）**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| id | string | Entity标识 | - |
| previous_quota | number | 重置前配额 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| new_quota | number | 重置后配额 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| balance | object | 余额变更详情 | 见下方 |

**balance结构**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| previous_remaining | number | 重置前剩余量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| new_remaining | number | 重置后剩余量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| used | number | 当前已用量 | 重置后为0；`unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |

---
