# /api-keys

## 1. 数据模型

```json
{
  "id": "apikey-001",
  "key": "ak-2v8x9k3m7p",
  "description": "BFE项目测试Key",
  "enabled": true,
  "create_time": 1716883200,
  "update_time": 1716883200,
  "expired_time": -1,
  "unlimited_quota": false,
  "models": ["*"],
  "subnet": ["*"],
  "quota_plan": {
    "unlimited": false,
    "pass_when_no_enough_quota": false,
    "quota": 100000000,
    "unit": "total_token",
    "reset_period": "monthly",
    "balance": {
      "used": 50000000,
      "remaining": 50000000
    }
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
    "enabled": true,
    "rules": [
      {
        "name": "apikey-default",
        "cond": "default_t()",
        "targets": [
          {
            "cluster_name": "cluster_apikey",
            "model": "",
            "weight": 100
          }
        ],
        "fallbacks": []
      }
    ]
  },
  "entity_id": "ent-zhangsan-001",
  "entity": {
    "id": "ent-zhangsan-001",
    "name": "zhangsan",
    "type": "person"
  }
}
```

**字段说明**

| 字段 | 类型 | 说明 | 可能取值 |
|------|------|------|----------|
| `id` | string | API-Key唯一标识（内部使用） | 系统生成，如`apikey-001` |
| `key` | string | API-Key值（用于请求头鉴权） | 系统生成，如`ak-2v8x9k3m7p`。**角色可见性**：系统管理员返回明文；系统管理员（只读）返回掩码值（长度>8：前4+`********`+后4；4<长度≤8：前2+`****`+后2；长度≤4：全掩码），掩码值不可用于鉴权 |
| `description` | string | 描述 | 自定义 |
| `enabled` | bool | 是否启用 | `true`（启用）、`false`（禁用） |
| `create_time` | int64 | 创建时间 | Unix时间戳（秒） |
| `update_time` | int64 | 更新时间 | Unix时间戳（秒） |
| `expired_time` | int64 | 过期时间 | `-1`表示永不过期；其他为Unix时间戳（秒） |
| `unlimited_quota` | bool | 是否无限配额 | `true`：不执行配额检查；`false`：执行配额控制；**默认值为false** |
| `models` | []string | 允许访问的模型白名单 | 包含"*"表示不限制，**默认值为不限制** |
| `subnet` | []string | 允许的客户端子网 | 包含"*"表示不限制，**默认值为不限制** |
| `quota_plan` | object | 配额计划 | 类型为 [QuotaPlan](./00-common.md#公共参数类型)，**不会为空** |
| `rate_limit_policy` | object | 限流策略 | 类型为 [RateLimitPolicy](./00-common.md#公共参数类型)，**不会为空** |
| `route_rules` | object | 路由规则 | 类型为 [RouteRules](./00-common.md#公共参数类型)，**不会为空** |
| `entity_id` | string | 挂载到的Entity ID | 为空表示未挂载到任何Entity |
| `entity` | object | 挂载的Entity摘要（只读） | 包含id、name、type |

> **说明**：`quota_plan` 中的 `quota`、`balance.used`、`balance.remaining` 类型为 `number`，支持 `unit="total_token"`（整数）和 `unit="RMB"`（最多 4 位小数展示）。详见 [QuotaPlan](./00-common.md#公共参数类型)。

**公共类型引用**

| 字段 | 公共类型 | 说明 |
|------|----------|------|
| `quota_plan` | [QuotaPlan](./00-common.md#公共参数类型) | 配额计划；作为输入时无需传入 `balance` |
| `rate_limit_policy` | [RateLimitPolicy](./00-common.md#公共参数类型) | 限流策略 |
| `route_rules` | [RouteRules](./00-common.md#公共参数类型) | 路由规则集；`rules` 每个元素类型为 [RouteRule](./00-common.md#公共参数类型) |

详细字段及合法性条件见 `00-common.md` 中对应公共类型定义。

---

## 2. 接口清单

### 2.1 创建API-Key

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 创建API-Key | - |
| 端点 | /api-keys | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| key | string | API-Key值 | N | 可选。若传入则使用该值作为API-Key；若不传则由后台生成。用于从其他系统导入API-Key | 若传入则必填、非空；长度 1-128 字符；仅允许大小写字母、数字、连字符(-)、下划线(_)；须全局唯一 |
| description | string | API-Key描述 | Y | - | 必填、非空；长度不超过 512 字符 |
| expired_time | int64 | 过期时间 | N | -1表示永不过期；其他为Unix时间戳（秒） | -1 或 Unix 时间戳秒；非 -1 时必须 >= 当前时间 |
| enabled | bool | 是否启用 | N | 默认true | - |
| unlimited_quota | bool | 是否无限配额 | N | 默认false | - |
| models | []string | 允许访问的模型白名单 | N | 包含"*"表示不限制，**默认值为不限制** | 每个元素类型为 [AIModel](./00-common.md#公共参数类型) |
| subnet | []string | 允许的客户端子网 | N | 包含"*"表示不限制，**默认值为不限制** | 每个元素类型为 [CIDR](./00-common.md#公共参数类型) |
| quota_plan | object | 配额计划 | N | 同2.1中quota_plan结构（不含balance），若未设置则使用默认值 | 类型为 [QuotaPlan](./00-common.md#公共参数类型) |
| rate_limit_policy | object | 限流策略 | N | 同2.1中rate_limit_policy结构，若未设置则使用默认值（enabled=false） | 类型为 [RateLimitPolicy](./00-common.md#公共参数类型) |
| route_rules | object | 路由规则 | N | 同2.1中route_rules结构，若未设置则使用默认值（enabled=false, rules为空） | 类型为 [RouteRules](./00-common.md#公共参数类型) |
| entity_id | string | 挂载的Entity ID | N | 为空表示不挂载 | 若传入非空值，该 Entity 必须存在 |

**约束**

- `quota_plan`、`rate_limit_policy`、`route_rules` 的字段及合法性条件分别见 [QuotaPlan](./00-common.md#公共参数类型)、[RateLimitPolicy](./00-common.md#公共参数类型)、[RouteRules](./00-common.md#公共参数类型) 公共类型定义。
- `rate_limit_policy.rules` 中的 `batch_limits` 段（批量任务限流维度）与独立策略资源一致生效：四维度均按发起请求的 API-Key 计数、与 `model` 无关，省略/`null` 表示该策略不参与批量限流，不配置不代表批量不限流（仍受数据面 `mod_ai_batch.data` 全局硬顶约束）；字段定义见 [RateLimitPolicy](./00-common.md#公共参数类型)。
- 若 `entity_id` 不为空，该Entity必须存在。
- 若传入 `key`，其值需在系统中全局唯一，长度 1-128 字符，且仅允许大小写字母、数字、连字符(-)、下划线(_)；若重复或格式非法，返回422。
- `description` 必填，长度不超过 512 字符。
- `expired_time` 为 -1 表示永不过期；其他值必须是不小于当前时间的 Unix 时间戳秒。
- `models` 每个元素类型为 [AIModel](./00-common.md#公共参数类型)；为 `"*"` 时表示不限制。
- `subnet` 每个元素类型为 [CIDR](./00-common.md#公共参数类型)；为 `"*"` 时表示不限制。

**HTTP BODY参数示例**

```json
{
    "description": "BFE项目测试Key",
    "expired_time": -1,
    "unlimited_quota": false,
    "models": ["*"],
    "subnet": ["*"],
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
                "name": "apikey-default",
                "cond": "default_t()",
                "targets": [
                    {
                        "cluster_name": "cluster_apikey",
                        "model": "",
                        "weight": 100
                    }
                ],
                "fallbacks": []
            }
        ]
    },
    "entity_id": "ent-zhangsan-001"
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
8. 若传入 `key`，使用输入的 `key` 作为API-Key；否则在后台生成新的API-Key，并绑定上述资源
9. 返回结果，含完整的嵌套结构（不含balance）

**返回数据（Data内容）**

字段同2.1数据模型（不含balance）。

**成功返回示例**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "id": "apikey-001",
        "key": "ak-2v8x9k3m7p",
        "description": "BFE项目测试Key",
        "enabled": true,
        "create_time": 1716883200,
        "update_time": 1716883200,
        "expired_time": -1,
        "unlimited_quota": false,
        "models": ["*"],
        "subnet": ["*"],
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
                    "name": "apikey-default",
                    "cond": "default_t()",
                    "targets": [
                        {
                            "cluster_name": "cluster_apikey",
                            "model": "",
                            "weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        },
        "entity_id": "ent-zhangsan-001",
        "entity": {
            "id": "ent-zhangsan-001",
            "name": "zhangsan",
            "type": "person"
        }
    }
}
```

---

### 2.2 查询API-Key列表

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询API-Key列表 | - |
| 端点 | /api-keys | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（Query）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| page | int | 页码 | N | 默认1 | 必须 >0 |
| page_size | int | 每页条数 | N | 默认20，最大100 | 取值范围 1-100 |
| enabled | bool | 是否启用过滤 | N | - | - |
| entity_id | string | 按挂载的Entity ID过滤 | N | - | 长度不超过 64 字符 |
| unlimited_quota | bool | 是否无限配额过滤 | N | - | - |


**返回数据（Data内容）**

返回分页结构：

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| list | []APIKey | API-Key列表 | 元素字段同2.1数据模型，`quota_plan`中包含`balance`字段（含`used`和`remaining`） |
| pagination | object | 分页信息 | 包含`page`、`page_size`、`total`；未传分页参数时为 `page=1`、`page_size=20`；`total` 为满足 `enabled`/`entity_id`/`unlimited_quota` 过滤条件的总条数 |

---

### 2.3 查询单个API-Key

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询单个API-Key | - |
| 端点 | /api-keys/{id} | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | API-Key标识 | Y | - | 必填、非空；长度不超过 255 字符 |

**返回数据（Data内容）**

字段同2.1数据模型，`quota_plan`中包含`balance`字段（含`used`和`remaining`）。

---

### 2.4 全量更新API-Key

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 全量更新API-Key | - |
| 端点 | /api-keys/{id} | - |
| 版本 | v1 | - |
| method | PUT | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | API-Key标识 | Y | - | 必填、非空；长度不超过 255 字符 |

**输入参数（Body）**

同2.2.1创建API-Key的Body参数。

注：`key` 字段仅在创建时生效，更新时会被忽略。

**约束**

- `quota_plan`、`rate_limit_policy`、`route_rules` 的字段及合法性条件分别见 [QuotaPlan](./00-common.md#公共参数类型)、[RateLimitPolicy](./00-common.md#公共参数类型)、[RouteRules](./00-common.md#公共参数类型) 公共类型定义。
- `rate_limit_policy.rules` 中的 `batch_limits` 段（批量任务限流维度）与独立策略资源一致生效：四维度均按发起请求的 API-Key 计数、与 `model` 无关，省略/`null` 表示该策略不参与批量限流，不配置不代表批量不限流（仍受数据面 `mod_ai_batch.data` 全局硬顶约束）；字段定义见 [RateLimitPolicy](./00-common.md#公共参数类型)。
- 若将 `entity_id` 修改为非空（挂载到新Entity），该Entity必须存在。
- 注：配额扣减遵循 [workflows.md](./workflows.md) §5——当 `unlimited_quota=false` 且 `quota_plan.unlimited=false` 时，Key 自身的 QuotaPlan 已计入扣减列表并扣减其自身 QuotaBalance，因此不要求新Entity或其祖先链上存在有效的Quota Plan。若 Key 自身余额不足，请求按 §5 step 6/8 拒绝（429002），与挂载目标的配额类型无关。

**执行逻辑**

1. 若传入新的 `quota_plan`：
   - 若 `quota_plan.unlimited` 为 `false`，且 `quota` 的值发生了改变（单位不变）：保留 `balance.used`，按 `balance.remaining = 新quota - balance.used` 调整余额，并同步调整 Redis 剩余量。**不再对结果钳零**（issue #1476）：`balance.used` 可大于旧 `quota`（欠费），此时 `remaining` 可为负——负值表示新配额仍不足以覆盖已用量，BFE 鉴权继续拒绝（剩余量对外展示仍为 0，见 [00-common.md](./00-common.md) balance 结构）。
   - 若 `quota_plan.unit` 发生变化（如 `total_token` ↔ `RMB`）：由于新旧单位无法直接换算，对 balance 进行重置（remaining = 新的quota，used = 0），并将 Redis 剩余量重置为新的quota。
   - 若 `quota_plan.unlimited` 由 `true` 改为 `false`：创建新的 balance（remaining = quota，used = 0），并将 Redis 剩余量重置为 quota。
   - 若 `quota_plan.unlimited` 由 `false` 改为 `true`：将 balance 置为无限制状态（used = 0，remaining = 一个足够大的 sentinel 值），并将 Redis 剩余量重置为 sentinel。
   - 若仅 `quota_plan` 的 `pass_when_no_enough_quota`、`reset_period` 等字段变化，而 `quota`、`unit`、`unlimited` 均未变化：不调整 balance。
2. 若传入新的 `rate_limit_policy` 且 `enabled` 为 `true`：
   - 对于 `tpm` 和 `rpm` 规则：
     - 若规则的 `name` 与之前的规则相同，但其他参数（如 `window_minutes`、`max_tokens`、`max_requests`、`step_minutes`）发生变化，则重置该规则对应的计数器
     - 若规则的 `name` 是新的（之前不存在），则创建新的计数器
     - 若之前存在的规则 `name` 在新传入的规则中消失了，则删除该规则对应的计数器
   - 若 `enabled` 由 `true` 改为 `false`，则保留计数器数据但不执行限流检查（下次启用时继续使用）
   - 若 `enabled` 由 `false` 改为 `true`，则根据当前规则创建/恢复计数器
3. 若传入新的 `route_rules`，替换对应的路由规则配置
4. 更新API-Key其他字段

**返回数据（Data内容）**

同2.1数据模型（不含balance）。

---

### 2.5 部分更新API-Key

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 部分更新API-Key | - |
| 端点 | /api-keys/{id} | - |
| 版本 | v1 | - |
| method | PATCH | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | API-Key标识 | Y | - | 必填、非空；长度不超过 255 字符 |

**输入参数（Body）**

同2.2.1创建API-Key的Body参数，仅传需修改字段。

注：`key` 字段仅在创建时生效，更新时会被忽略。

**约束**

- `quota_plan`、`rate_limit_policy`、`route_rules` 的字段及合法性条件分别见 [QuotaPlan](./00-common.md#公共参数类型)、[RateLimitPolicy](./00-common.md#公共参数类型)、[RouteRules](./00-common.md#公共参数类型) 公共类型定义。
- `rate_limit_policy.rules` 中的 `batch_limits` 段（批量任务限流维度）与独立策略资源一致生效：四维度均按发起请求的 API-Key 计数、与 `model` 无关，省略/`null` 表示该策略不参与批量限流，不配置不代表批量不限流（仍受数据面 `mod_ai_batch.data` 全局硬顶约束）；字段定义见 [RateLimitPolicy](./00-common.md#公共参数类型)。
- 若将 `entity_id` 修改为非空（挂载到新Entity），该Entity必须存在。不要求新Entity或其祖先链上存在有效的Quota Plan（依据 [workflows.md](./workflows.md) §5，Key 自身的 QuotaPlan 始终计入扣减列表）。
- 修改 `quota_plan.quota`（单位不变）时，保留 `balance.used`，按 `新quota - used` 重新计算 `balance.remaining`；修改 `quota_plan.unit` 或 `quota_plan.unlimited` 时，会重置 `balance.used = 0`；仅修改 `quota_plan` 其他字段不会调整 balance。
- 若修改 `route_rules`，视为全量替换该路由规则配置。

**返回数据（Data内容）**

同2.1数据模型（不含balance）。

---

### 2.6 删除API-Key

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 删除API-Key | - |
| 端点 | /api-keys/{id} | - |
| 版本 | v1 | - |
| method | DELETE | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | API-Key标识 | Y | - | 必填、非空；长度不超过 255 字符 |

**返回数据（Data内容）**

Data为null。

**说明**

- 删除API-Key时，级联删除其专属的quota_plan、rate_limit_policy、route_rules及底层资源（如果这些资源不被其他API-Key或Entity引用）
- 删除API-Key可能会影响正在处理中的请求

---

### 2.7 查询配额计划（含Balance）

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 查询API-Key的配额计划（含实时余额） | - |
| 端点 | /api-keys/{id}/quota-plan | - |
| 版本 | v1 | - |
| method | GET | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | API-Key标识 | Y | - | 必填、非空；长度不超过 255 字符 |

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

**成功返回示例（Token 配额）**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "unlimited": false,
        "pass_when_no_enough_quota": false,
        "quota": 100000000,
        "unit": "total_token",
        "reset_period": "monthly",
        "balance": {
            "used": 12345679,
            "remaining": 87654321
        }
    }
}
```

**成功返回示例（RMB 配额）**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "unlimited": false,
        "pass_when_no_enough_quota": false,
        "quota": 5000.00,
        "unit": "RMB",
        "reset_period": "monthly",
        "balance": {
            "used": 1234.56,
            "remaining": 3765.44
        }
    }
}
```

---

### 2.8 重置配额余额（QuotaBalance Reset）

**基本信息**

| 项目 | 值 | 说明 |
| - | - | - |
| 含义 | 重置API-Key的配额余额 | - |
| 端点 | /api-keys/{id}/quota-plan/reset | - |
| 版本 | v1 | - |
| method | POST | - |

**输入参数（URI）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| id | string | API-Key标识 | Y | - | 必填、非空；长度不超过 255 字符 |

**输入参数（Body）**

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| - | - | - | - | - | - |
| quota | number | 重置后的配额总量 | N | 若传入则更新quota并同步重置balance；若不传则按当前quota重置 | 非负数；`unit=total_token` 时必须为整数；`unit=RMB` 时取值范围 0 ~ 90000000.00（9000 万元），小数位不超过 8 位 |
| reason | string | 重置原因 | N | 用于审计 | - |

**执行逻辑**

1. 若 API-Key 不存在，返回 404；找到该 API-Key 的 quota_plan：若未关联 quota plan 或 `unlimited=true`，返回 422（Param Illegal；不产生配额变更，并记录 `status=2` 的操作日志）；若传入 quota，更新 quota_plan.quota 为新的值
2. 触发balance的reset：
   - balance.remaining = 当前quota（或新的quota）
   - balance.used = 0

**失败响应说明**

| HTTP status | ErrNum | ErrMsg | 触发条件 |
|------|------|------|------|
| 404 | 404 | `API-Key Record Not Exist` | 路径中的 API-Key 不存在 |
| 422 | 422 | `Param Illegal: <原因>` | 未关联 quota plan；unlimited 配额计划（`cannot reset balance for unlimited quota`）；quota 参数非法 |
| 500 | 500 | `Unknown Exception: <原因>` | 内部故障 |

**返回数据（Data内容）**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| id | string | API-Key标识 | - |
| previous_quota | number | 重置前配额 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| new_quota | number | 重置后配额 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| balance | object | 余额变更详情 | 见下方 |

**balance结构**

| 参数名 | 类型 | 参数含义 | 补充描述 |
| - | - | - | - |
| previous_remaining | number | 重置前剩余量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| new_remaining | number | 重置后剩余量 | `unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |
| used | number | 当前已用量 | 重置后为0；`unit=total_token` 时为整数；`unit=RMB` 时最多 4 位小数展示 |

**成功返回示例（Token 配额）**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "id": "apikey-001",
        "previous_quota": 100000000,
        "new_quota": 100000000,
        "balance": {
            "previous_remaining": 87654321,
            "new_remaining": 100000000,
            "used": 0
        }
    }
}
```

**成功返回示例（RMB 配额）**

```json
{
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
        "id": "apikey-001",
        "previous_quota": 5000.00,
        "new_quota": 5000.00,
        "balance": {
            "previous_remaining": 3765.44,
            "new_remaining": 5000.00,
            "used": 0
        }
    }
}
```

---

