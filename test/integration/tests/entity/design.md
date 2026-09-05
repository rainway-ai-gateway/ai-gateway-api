# Entity 测试用例设计文档

## 1. 模块概述

Entity 模块用于管理组织架构实体（部门、团队、项目、个人等），支持层级关系、模型黑白名单、配额计划、限流策略、路由规则。v0.3.0 列表接口明确为分页结构 `{list, pagination}`，详情/创建/更新返回不含 `balance`。配额计划 `unit` 支持 `total_token` 与 `RMB`，金额型配额使用 `DECIMAL(18,8)` 存储，余额从 Redis 实时读取，精度为 1e8。

Entity ID 生成机制（issue #132）：未显式指定 `id` 时，系统从数据库序列表 `entity_id_seq` 原子分配序号，生成 `entity-{seq}` 形式的 ID。序号单调递增、分配即消耗，删除 Entity 后不回退，保证旧 ID 永不复用（避免 ABA 身份混淆）；显式传入 `id` 时保留唯一性查重，`uk_entity_id` 唯一索引作为最终一致性防线。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| E-1 | 创建 Entity | POST | `/open-api/v1/entities` | - |
| E-2 | 查询 Entity 列表 | GET | `/open-api/v1/entities` | 分页 |
| E-3 | 查询单个 Entity | GET | `/open-api/v1/entities/{id}` | - |
| E-4 | 全量更新 Entity | PUT | `/open-api/v1/entities/{id}` | type 不可改 |
| E-5 | 部分更新 Entity | PATCH | `/open-api/v1/entities/{id}` | - |
| E-6 | 删除 Entity | DELETE | `/open-api/v1/entities/{id}` | 有子节点或被挂载时禁止删除 |
| E-7 | 查询配额计划 | GET | `/open-api/v1/entities/{id}/quota-plan` | 含 balance |
| E-8 | 重置配额余额 | POST | `/open-api/v1/entities/{id}/quota-plan/reset` | - |
| E-9 | 更新配额计划（余额差异化调整） | PUT/PATCH | `/open-api/v1/entities/{id}` | 变更 quota_plan 时调整 Redis 余额：quota 变化保留 used，unit/unlimited 变化重置 |

## 3. 测试用例统计

| 接口 | 测试用例数 |
|------|-----------|
| 创建 Entity | 24 |
| 查询 Entity 列表 | 3 |
| 查询单个 Entity | 2 |
| 全量更新 Entity | 6 |
| 部分更新 Entity | 4 |
| 删除 Entity | 4 |
| 查询配额计划 | 2 |
| 重置配额余额 | 3 |
| 更新配额计划（余额差异化调整） | 6 |
| **合计** | **52** |

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。

## 5. 目录结构

```
entity/
├── design.md
├── create/
│   └── create_test.go
├── list/
│   └── list_test.go
├── detail/
│   └── detail_test.go
├── full_update/
│   └── full_update_test.go
├── partial_update/
│   └── partial_update_test.go
├── delete/
│   └── delete_test.go
├── quota_plan/
│   └── quota_plan_test.go
├── quota_reset/
│   └── quota_reset_test.go
└── quota_update/
    └── quota_update_test.go
```

## 6. 创建 Entity

### 6.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 创建 Entity |
| 方法 | POST |
| 路径 | `/open-api/v1/entities` |
| 说明 | 创建组织架构实体 |

### 6.2 接口参数说明

#### 6.2.1 请求参数

##### Body 参数

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| id | string | N | Entity 唯一标识；不传时系统自动生成 `entity-{seq}`（seq 从 `entity_id_seq` 序列表原子分配，单调不回退） | 全局唯一（`uk_entity_id` 约束），重复返回 422 |
| name | string | Y | Entity 名称，全局唯一 | 长度 1-64；不能包含控制字符；不能有首尾空白；全局唯一 |
| type | string | Y | Entity 类型，必须引用已定义的 Entity-Type | 必须为已存在的 EntityTypeName |
| parent_id | string | N | 父 Entity ID，为空表示根节点 | 若非空，父 Entity 必须存在，且其父类型的 level 必须小于当前类型的 level |
| allow_models | []string | N | 允许访问的模型白名单，默认 ["*"] | 每个元素为 AIModel |
| block_models | []string | N | 禁止访问的模型黑名单，默认 [] | 每个元素为非空字符串 |
| quota_plan | object | N | 配额计划，同 API-Key quota_plan 结构（不含 balance） | `quota` ≥0；`unit` ∈ {`total_token`, `RMB`}；`reset_period` ∈ {never, weekly, monthly} |
| rate_limit_policy | object | N | 限流策略 | 同 RateLimitPolicy 类型 |
| route_rules | object | N | 路由规则 | 同 RouteRules 类型 |

#### 6.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | Entity 唯一标识，格式 `entity-{seq}`（未显式传入时由系统从 `entity_id_seq` 序列表原子分配） |
| name | string | Entity 名称 |
| type | string | Entity 类型 |
| parent_id | string | 父 Entity ID |
| allow_models | []string | 允许访问的模型白名单 |
| block_models | []string | 禁止访问的模型黑名单 |
| quota_plan | object | 配额计划（不含 balance） |
| rate_limit_policy | object | 限流策略 |
| route_rules | object | 路由规则 |
| create_time | int64 | 创建时间 |
| update_time | int64 | 更新时间 |

### 6.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-1-001 | 创建 Entity（仅必填） | 正常参数 | 验证默认值 |
| E-1-002 | 创建 Entity（含 quota_plan） | 正常参数 | 验证嵌套结构 |
| E-1-003 | 缺少 name | 必填校验 | 验证 ErrNum=422 |
| E-1-004 | 缺少 type | 必填校验 | 验证 ErrNum=422 |
| E-1-005 | type 不存在 | 异常参数 | 验证 ErrNum=404 或 422 |
| E-1-006 | 重复 name | 业务规则 | 验证 ErrNum=555/556 |
| E-1-007 | 创建层级 Entity（合法 parent） | 正常参数 | 父 level 小于子 |
| E-1-008 | 创建层级 Entity（非法 parent level） | 异常参数 | 父 level 必须小于子 |
| E-1-009 | type 格式非法（含大写） | 合法性条件 | 验证 ErrNum=422 |
| E-1-010 | Entity name 包含首尾空白 | 合法性条件 | 验证 ErrNum=422 |
| E-1-019 | Entity name 含 `@`（`用户名@项目名` 形式） | 合法性条件 | 验证 ErrNum=200（Issue #135 放开 `@`） |
| E-1-020 | Entity name 以 `@` 开头 | 合法性条件 | 验证 ErrNum=422 |
| E-1-021 | Entity name 以 `@` 结尾 | 合法性条件 | 验证 ErrNum=422 |
| E-1-022 | Entity name 含 `@` 以外的特殊字符 | 合法性条件 | 验证 ErrNum=422 |
| E-1-101 | 自动生成 ID 格式为 entity-N | 返回数据 | 未传 id 时返回 `entity-{正整数}` 格式 ID |
| E-1-102 | 连续创建 Entity ID 单调递增 | 业务规则 | 串行创建 5 个，ID 序号严格递增 |

### 6.4 测试场景详细设计

#### 6.4.1 E-1-001：创建 Entity（仅必填）

##### 设计思路

验证仅传必填字段时，默认值正确填充，并返回系统生成的 id。

##### 前提数据准备

已创建 Entity-Type `department`。

##### 执行步骤

1. 发送 POST 请求到 `/open-api/v1/entities`。
2. 验证返回结构和默认值。

##### 请求参数

```json
{
    "name": "ent_root",
    "type": "department"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| id | 非空字符串 | NotEmpty |
| name | "ent_root" | Equals |
| type | "department" | Equals |
| parent_id | null 或空字符串 | IsNullOrEmpty |
| allow_models | ["*"] | Equals |
| block_models | [] | IsEmpty |
| quota_plan | 非空对象 | IsObject |
| rate_limit_policy | 非空对象 | IsObject |
| route_rules | 非空对象 | IsObject |
| quota_plan.balance | 不存在 | NotExists |

---

#### 6.4.2 E-1-002：创建 Entity（含 quota_plan）

##### 设计思路

验证传入完整嵌套结构时，返回的 `quota_plan` 与输入一致且不含 `balance`。

##### 前提数据准备

已创建 Entity-Type `department`。

##### 执行步骤

1. 发送 POST 请求，传入完整参数。
2. 验证返回结构和字段。

##### 请求参数

```json
{
    "name": "ent_quota",
    "type": "department",
    "quota_plan": {
        "unlimited": false,
        "quota": 1000000,
        "unit": "total_token",
        "reset_period": "monthly"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "ent_quota" | Equals |
| quota_plan.unlimited | false | Equals |
| quota_plan.quota | 1000000 | Equals |
| quota_plan.unit | "total_token" | Equals |
| quota_plan.reset_period | "monthly" | Equals |
| quota_plan.balance | 不存在 | NotExists |

---

#### 6.4.3 E-1-003：缺少 name（必填校验）

##### 设计思路

验证 `name` 为必填字段。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，Body 中缺少 `name`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "type": "department"
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "name" 的错误信息  
**Data**：null

---

#### 6.4.4 E-1-004：缺少 type（必填校验）

##### 设计思路

验证 `type` 为必填字段。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，Body 中缺少 `type`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "ent_notype"
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 "type" 的错误信息  
**Data**：null

---

#### 6.4.5 E-1-005：type 不存在（异常参数）

##### 设计思路

验证 `type` 必须引用已定义的 Entity-Type。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`type` 为不存在的类型。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "ent_bad_type",
    "type": "not_exist"
}
```

##### 预期返回结果

**ErrNum**：404 或 422  
**ErrMsg**：类型不存在的错误信息  
**Data**：null

---

#### 6.4.6 E-1-006：重复 name（业务规则）

##### 设计思路

验证 `name` 全局唯一。

##### 前提数据准备

已创建 `ent_dup`。

##### 执行步骤

1. 发送 POST 请求，使用重复的 `name`。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "ent_dup",
    "type": "department"
}
```

##### 预期返回结果

**ErrNum**：555 或 556  
**ErrMsg**：名称已存在的错误信息  
**Data**：null

---

#### 6.4.7 E-1-007：创建层级 Entity（合法 parent）

##### 设计思路

验证父 Entity 的 Entity-Type level 小于子 Entity 的 level 时，创建成功。

##### 前提数据准备

已创建 Entity-Type `department`(level=1)、`team`(level=2)，以及父 Entity。

##### 执行步骤

1. 发送 POST 请求，传入合法的 `parent_id`。
2. 验证返回的 `parent_id` 与输入一致。

##### 请求参数

```json
{
    "name": "ent_team",
    "type": "team",
    "parent_id": "<parent_id>"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "ent_team" | Equals |
| type | "team" | Equals |
| parent_id | "<parent_id>" | Equals |

---

#### 6.4.8 E-1-008：创建层级 Entity（非法 parent level）

##### 设计思路

验证父 Entity 的 Entity-Type level 不满足约束时返回错误。

##### 前提数据准备

已创建 level 较高子类型与 level 较低父类型（如 `department` level=1 的父 Entity 与 `team` level=2 的父 Entity）。

##### 执行步骤

1. 发送 POST 请求，将 `department` 类型 Entity 的 `parent_id` 指向 `team` 类型 Entity。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "ent_bad_parent",
    "type": "department",
    "parent_id": "<team_entity_id>"
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：父节点层级非法的错误信息  
**Data**：null

---

#### 6.4.9 E-1-009：type 格式非法（含大写）

##### 设计思路

验证 `type` 必须是已存在的 EntityTypeName（小写等格式约束）。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`type` 包含大写字母。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "ent_bad_type_fmt",
    "type": "BadType"
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 type 非法或类型不存在的错误信息  
**Data**：null

---

#### 6.4.10 E-1-010：Entity name 包含首尾空白（合法性条件）

##### 设计思路

验证 `name` 不能有首尾空白字符。

##### 前提数据准备

无

##### 执行步骤

1. 发送 POST 请求，`name` 包含首尾空格。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": " badname ",
    "type": "department"
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 name 非法的错误信息  
**Data**：null

---

#### 6.4.11 E-1-011：创建 Entity 并指定 RMB 配额（正常参数）

##### 设计思路

验证 Entity 创建时可指定 `quota_plan.unit=RMB` 与小数配额，返回结构正确。

##### 前提数据准备

已创建 Entity-Type `department`。

##### 执行步骤

1. 发送 POST 请求，`quota_plan.unit=RMB`、`quota=5555.5555`。
2. 验证返回 `quota_plan.unit=RMB`、`quota=5555.5555`、`quota_plan.balance` 不存在。
3. 调用 quota-plan 接口，验证 `balance.remaining=5555.5555`、`balance.used=0`。

##### 请求参数

```json
{
    "name": "ent_rmb_quota",
    "type": "department",
    "quota_plan": {
        "unlimited": false,
        "quota": 5555.5555,
        "unit": "RMB",
        "reset_period": "monthly"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "ent_rmb_quota" | Equals |
| quota_plan.unit | "RMB" | Equals |
| quota_plan.quota | 5555.5555 | Equals |
| quota_plan.balance | 不存在 | NotExists |

#### 6.4.12 E-1-101：自动生成 ID 格式为 entity-N（返回数据）

##### 设计思路

验证未显式传入 `id` 时，系统从 `entity_id_seq` 序列表分配序号并返回 `entity-{seq}` 格式的 ID（issue #132）。

##### 前提数据准备

已创建 Entity-Type `department`。

##### 执行步骤

1. 发送 POST 请求到 `/open-api/v1/entities`，Body 中只含 `name`、`type`。
2. 验证返回 `id` 以 `entity-` 为前缀，后缀为正整数。

##### 请求参数

```json
{
    "name": "ent_auto_id",
    "type": "department"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| id | 以 "entity-" 前缀、后缀为正整数 | Matches/GreaterThan(0) |

> 说明：并发分配的正确性由 DAO 层单元测试 `TestTEntityIDSeqAllocate_Concurrent`（50 goroutine）覆盖；集成测试环境为 SQLite 文件库，多连接并发写会触发 busy 等待，故此处不启用并发 HTTP 用例。

#### 6.4.13 E-1-102：连续创建 Entity ID 单调递增（业务规则）

##### 设计思路

验证序列表分配的 ID 严格单调递增，不会因删除或其他用例执行而复用旧序号。

##### 前提数据准备

已创建 Entity-Type `department`。

##### 执行步骤

1. 串行创建 5 个 Entity（每次仅传 `name`、`type`）。
2. 记录每次返回的 `id`，解析序号并断言严格递增。

##### 请求参数

```json
{
    "name": "ent_seq_<i>",
    "type": "department"
}
```

##### 预期返回结果

**ErrNum**：200（5 次均成功）

**断言**：5 个 ID 序号 `seq_i` 满足 `seq_1 < seq_2 < ... < seq_5`（GreaterThan 逐次比较）。

---

## 7. 查询 Entity 列表

### 7.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 查询 Entity 列表 |
| 方法 | GET |
| 路径 | `/open-api/v1/entities` |
| 说明 | 分页查询 Entity 列表 |

### 7.2 接口参数说明

#### 7.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| page | int | N | 页码，默认 1 |
| page_size | int | N | 每页条数，默认 20，最大 100 |
| id | string | N | 按 Entity ID 过滤 |
| name | string | N | 按 Entity 名称过滤 |
| type | string | N | 按类型过滤 |
| parent_id | string | N | 按父 Entity 过滤 |
| quota_plan_id | int64 | N | 按配额计划 ID 过滤 |
| route_rules_id | int64 | N | 按路由规则 ID 过滤 |

#### 7.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| list | []Entity | Entity 列表 |
| pagination | object | 分页信息 |
| pagination.page | int | 当前页码 |
| pagination.page_size | int | 每页条数 |
| pagination.total | int | 总条数 |

**list 对象字段说明**

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | Entity 唯一标识 |
| name | string | Entity 名称 |
| type | string | Entity 类型 |
| parent_id | string | 父 Entity ID |
| allow_models | []string | 允许访问的模型白名单 |
| block_models | []string | 禁止访问的模型黑名单 |
| quota_plan | object | 配额计划（不含 balance） |
| rate_limit_policy | object | 限流策略 |
| create_time | int64 | 创建时间 |
| update_time | int64 | 更新时间 |

### 7.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-2-001 | Entity 列表分页 | 正常参数 | 返回 {list, pagination} |
| E-2-002 | 按 type 过滤 | 正常参数 | 仅返回指定类型 |
| E-2-003 | 分页参数边界 | 边界值 | page=1&page_size=1 |

### 7.4 测试场景详细设计

#### 7.4.1 E-2-001：Entity 列表分页（正常参数）

##### 设计思路

验证列表接口返回分页结构，元素字段完整。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/entities`。
2. 验证返回结构和字段。

##### 请求参数

```
page=1&page_size=10
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 数组 | IsArray |
| list[0].id | 非空字符串 | NotEmpty |
| list[0].quota_plan | 非空对象 | IsObject |
| list[0].quota_plan.balance | 不存在 | NotExists |
| pagination.page | 1 | Equals |
| pagination.page_size | 10 | Equals |
| pagination.total | ≥ 1 | Gte |

---

#### 7.4.2 E-2-002：按 type 过滤（正常参数）

##### 设计思路

验证按 `type` 过滤后，列表中仅返回指定类型的 Entity。

##### 前提数据准备

已创建不同类型的 Entity。

##### 执行步骤

1. 发送 GET 请求，`type=department`。
2. 验证列表中所有元素的 `type` 均为 `department`。

##### 请求参数

```
type=department
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 数组 | IsArray |
| list[*].type | 全部为 "department" | Equals |

---

#### 7.4.3 E-2-003：分页参数边界（边界值）

##### 设计思路

验证分页参数边界，`page_size=1` 时返回单条记录。

##### 前提数据准备

已创建至少 2 个 Entity。

##### 执行步骤

1. 发送 GET 请求，`page=1&page_size=1`。
2. 验证 `list` 长度为 1，`pagination.total ≥ 2`。

##### 请求参数

```
page=1&page_size=1
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| list | 长度为 1 | Len=1 |
| pagination.page | 1 | Equals |
| pagination.page_size | 1 | Equals |
| pagination.total | ≥ 2 | Gte |

---

## 8. 查询单个 Entity

### 8.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 查询单个 Entity |
| 方法 | GET |
| 路径 | `/open-api/v1/entities/{id}` |
| 说明 | 按 Entity ID 查询单个 Entity |

### 8.2 接口参数说明

#### 8.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| id | string | Y | Entity 标识 |

#### 8.2.2 返回数据字段

同 6.2.2，`quota_plan` 不含 `balance`。

### 8.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-3-001 | 查询单个 Entity | 正常参数 | 字段完整，不含 balance |
| E-3-002 | 查询不存在的 Entity | 异常参数 | 验证 ErrNum=404 |

### 8.4 测试场景详细设计

#### 8.4.1 E-3-001：查询单个 Entity（正常参数）

##### 设计思路

验证按 ID 查询 Entity 的基本功能。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/entities/{id}`。
2. 验证返回字段完整且 `quota_plan` 不含 `balance`。

##### 请求参数

URI：`id`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| id | 非空字符串 | NotEmpty |
| name | 非空字符串 | NotEmpty |
| quota_plan | 非空对象 | IsObject |
| quota_plan.balance | 不存在 | NotExists |

---

#### 8.4.2 E-3-002：查询不存在的 Entity（异常参数）

##### 设计思路

验证查询不存在的 ID 时返回 404。

##### 前提数据准备

无

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/entities/non_existent_id`。
2. 验证返回错误码。

##### 请求参数

URI：`non_existent_id`

##### 预期返回结果

**ErrNum**：404  
**ErrMsg**：Entity 不存在的错误信息  
**Data**：null

---

## 9. 全量更新 Entity

### 9.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 全量更新 Entity |
| 方法 | PUT |
| 路径 | `/open-api/v1/entities/{id}` |
| 说明 | 全量更新 Entity，`type` 不可修改 |

### 9.2 接口参数说明

#### 9.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| id | string | Y | Entity 标识 |

##### Body 参数

同创建接口。

#### 9.2.2 返回数据字段

同创建接口。

### 9.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-4-001 | 全量更新 Entity name | 正常参数 | name 更新，其余不变 |
| E-4-002 | 全量更新后查询一致性 | 返回数据 | PUT 后立即 GET，验证数据一致 |
| E-4-003 | 全量更新冲突 name | 业务规则 | 验证名称唯一约束 |
| E-4-004 | 全量更新修改 type | 业务规则 | type 不可修改 |
| E-4-005 | 全量更新非法 name（含首尾空白） | 合法性条件 | 验证 ErrNum=422 |

### 9.4 测试场景详细设计

#### 9.4.1 E-4-001：全量更新 Entity name（正常参数）

##### 设计思路

验证全量更新 `name` 成功，其余字段保持不变。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 PUT 请求到 `/open-api/v1/entities/{id}`，传入完整 Body，`name` 改为新值。
2. 验证返回的 `name` 已更新，其余字段不变。

##### 请求参数

```json
{
    "name": "ent_updated",
    "type": "department",
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
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "ent_updated" | Equals |
| type | 与原 Entity 一致 | Equals |
| quota_plan.unlimited | true | Equals |

---

#### 9.4.2 E-4-002：全量更新后查询一致性（返回数据）

##### 设计思路

验证 PUT 更新成功后，立即通过 GET 查询，返回数据与更新请求一致。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 PUT 请求更新 Entity。
2. 发送 GET 请求查询该 Entity。
3. 对比两次返回的数据是否一致。

##### 请求参数

```json
{
    "name": "ent_consistency",
    "type": "department",
    "allow_models": ["gpt-4"],
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
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| name | "ent_consistency" | Equals |
| allow_models | ["gpt-4"] | Equals |

---

#### 9.4.3 E-4-003：全量更新冲突 name（业务规则）

##### 设计思路

验证 `name` 全局唯一，更新为已存在的 name 时返回错误。

##### 前提数据准备

已创建两个 Entity：Entity1 和 Entity2。

##### 执行步骤

1. 发送 PUT 请求，用 Entity1 的 id 更新 name 为 Entity2 的 name。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": "<entity2_name>",
    "type": "<entity1_type>",
    "quota_plan": {
        "unlimited": true
    },
    "rate_limit_policy": {
        "enabled": false
    },
    "route_rules": {
        "enabled": false,
        "rules": []
    }
}
```

##### 预期返回结果

**ErrNum**：555、556 或 500  
**ErrMsg**：名称冲突的错误信息  
**Data**：null

---

#### 9.4.4 E-4-004：全量更新修改 type（业务规则）

##### 设计思路

验证 `type` 字段不可修改。

##### 前提数据准备

已创建 Entity，类型为 `department`。

##### 执行步骤

1. 发送 PUT 请求，尝试将 `type` 修改为其他类型。
2. 验证返回 `type` 保持原值或返回错误。

##### 请求参数

```json
{
    "name": "<entity_name>",
    "type": "team",
    "quota_plan": {
        "unlimited": true
    },
    "rate_limit_policy": {
        "enabled": false
    },
    "route_rules": {
        "enabled": false,
        "rules": []
    }
}
```

##### 预期返回结果

**ErrNum**：200（type 保持不变）或 422  
**ErrMsg**：success 或 type 不可修改的错误信息

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| type | 原类型 | Equals |

---

#### 9.4.5 E-4-005：全量更新非法 name（含首尾空白）

##### 设计思路

验证全量更新时 `name` 同样受 EntityName 合法性条件约束。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 PUT 请求，`name` 包含首尾空格。
2. 验证返回错误码。

##### 请求参数

```json
{
    "name": " badname ",
    "type": "department",
    "quota_plan": {
        "unlimited": true
    },
    "rate_limit_policy": {
        "enabled": false
    },
    "route_rules": {
        "enabled": false,
        "rules": []
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含 name 非法的错误信息  
**Data**：null

---

#### 9.4.6 E-4-006：全量更新 quota_plan 切换为 RMB（正常参数）

##### 设计思路

验证全量更新可将 Entity 的 `quota_plan.unit` 切换为 `RMB`，并按新的金额配额重置余额。

##### 前提数据准备

已创建 `unit=total_token` 的 Entity。

##### 执行步骤

1. 发送 PUT 请求，`quota_plan.unit=RMB`、`quota=1234.56`。
2. 验证返回 `quota_plan.unit=RMB`、`quota=1234.56`。
3. 调用 quota-plan 接口，验证 `balance.remaining=1234.56`、`balance.used=0`。

##### 请求参数

```json
{
    "name": "ent_rmb_update",
    "type": "department",
    "allow_models": ["*"],
    "block_models": [],
    "quota_plan": {
        "unlimited": false,
        "quota": 1234.56,
        "unit": "RMB",
        "reset_period": "monthly"
    },
    "rate_limit_policy": {
        "enabled": false
    },
    "route_rules": {
        "enabled": false,
        "rules": []
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.unit | "RMB" | Equals |
| quota_plan.quota | 1234.56 | Equals |
| quota_plan.balance | 不存在 | NotExists |

---

## 10. 部分更新 Entity

### 10.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 部分更新 Entity |
| 方法 | PATCH |
| 路径 | `/open-api/v1/entities/{id}` |
| 说明 | 部分更新 Entity 字段 |

### 10.2 接口参数说明

#### 10.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| id | string | Y | Entity 标识 |

##### Body 参数

同创建接口，仅传需修改字段。

#### 10.2.2 返回数据字段

同创建接口。

### 10.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-5-001 | 部分更新 allow_models | 正常参数 | allow_models 更新 |
| E-5-002 | 部分更新后查询一致性 | 返回数据 | PATCH 后立即 GET，验证数据一致 |
| E-5-003 | 部分更新非法 route_rules（规则名重复） | 合法性条件 | 验证 ErrNum=422 |

### 10.4 测试场景详细设计

#### 10.4.1 E-5-001：部分更新 allow_models（正常参数）

##### 设计思路

验证部分更新 `allow_models` 成功，其余字段保持不变。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 PATCH 请求到 `/open-api/v1/entities/{id}`。
2. 验证返回的 `allow_models` 已更新。

##### 请求参数

```json
{
    "allow_models": ["gpt-4"]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| allow_models | ["gpt-4"] | Equals |
| type | 与原 Entity 一致 | Equals |

---

#### 10.4.2 E-5-002：部分更新后查询一致性（返回数据）

##### 设计思路

验证 PATCH 更新成功后，立即通过 GET 查询，返回数据与更新请求一致。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 PATCH 请求更新 `block_models`。
2. 发送 GET 请求查询该 Entity。
3. 对比两次返回的 `block_models`。

##### 请求参数

```json
{
    "block_models": ["gpt-4-32k"]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| block_models | ["gpt-4-32k"] | Equals |

---

#### 10.4.3 E-5-003：部分更新非法 route_rules（规则名重复）

##### 设计思路

验证部分更新 `route_rules` 时同样受 RouteRules 合法性条件约束。

##### 前提数据准备

已创建 Entity。

##### 执行步骤

1. 发送 PATCH 请求，`route_rules.rules` 包含同名规则。
2. 验证返回错误码。

##### 请求参数

```json
{
    "route_rules": {
        "enabled": true,
        "rules": [
            {
                "name": "dup",
                "Cond": "default_t()",
                "targets": [
                    {"ClusterName": "c1", "Weight": 100}
                ]
            },
            {
                "name": "dup",
                "Cond": "default_t()",
                "targets": [
                    {"ClusterName": "c2", "Weight": 100}
                ]
            }
        ]
    }
}
```

##### 预期返回结果

**ErrNum**：422  
**ErrMsg**：包含规则名称重复的错误信息  
**Data**：null

---

#### 10.4.4 E-5-004：部分更新 quota_plan 切换为 RMB（正常参数）

##### 设计思路

验证 PATCH 可单独修改 Entity 的 `quota_plan.unit` 与 `quota`，切换为金额型配额后余额同步重置。

##### 前提数据准备

已创建 `unit=total_token` 的 Entity。

##### 执行步骤

1. 发送 PATCH 请求，`quota_plan.unit=RMB`、`quota=777.7777`。
2. 验证返回 `quota_plan.unit=RMB`、`quota=777.7777`。
3. 调用 quota-plan 接口，验证 `balance.remaining=777.7777`、`balance.used=0`。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 777.7777,
        "unit": "RMB",
        "reset_period": "weekly"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.unit | "RMB" | Equals |
| quota_plan.quota | 777.7777 | Equals |
| quota_plan.balance | 不存在 | NotExists |

---

## 11. 删除 Entity

### 11.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 删除 Entity |
| 方法 | DELETE |
| 路径 | `/open-api/v1/entities/{id}` |
| 说明 | 删除 Entity，有子节点或被挂载时禁止删除 |

### 11.2 接口参数说明

#### 11.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| id | string | Y | Entity 标识 |

#### 11.2.2 返回数据字段

Data 为 null。

### 11.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-6-001 | 删除 Entity | 正常参数 | 删除成功，再次查询返回 404 |
| E-6-002 | 删除存在子节点的 Entity | 业务规则 | 验证 ErrNum=409 |
| E-6-003 | 删除被 API-Key 挂载的 Entity | 业务规则 | 验证 ErrNum=409 |
| E-6-004 | 删除最大编号 Entity 后新建不复用 ID | 业务规则 | 删除后重建的 ID 序号大于被删 ID |

### 11.4 测试场景详细设计

#### 11.4.1 E-6-001：删除 Entity（正常参数）

##### 设计思路

验证删除无子节点、未被挂载的 Entity 成功。

##### 前提数据准备

已创建无子节点、未被挂载的 Entity。

##### 执行步骤

1. 发送 DELETE 请求到 `/open-api/v1/entities/{id}`。
2. 验证返回成功。
3. 再次查询，验证返回 404。

##### 请求参数

URI：`id`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | null | IsNull |

---

#### 11.4.2 E-6-002：删除存在子节点的 Entity（业务规则）

##### 设计思路

验证存在子 Entity 的节点不可删除。

##### 前提数据准备

已创建父 Entity 及子 Entity。

##### 执行步骤

1. 发送 DELETE 请求到父 Entity id。
2. 验证返回错误码。

##### 请求参数

URI：父 Entity id

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：存在子节点无法删除的错误信息  
**Data**：null

---

#### 11.4.3 E-6-003：删除被 API-Key 挂载的 Entity（业务规则）

##### 设计思路

验证被 API-Key 挂载的 Entity 不可删除。

##### 前提数据准备

已创建 Entity 并被 API-Key 挂载。

##### 执行步骤

1. 发送 DELETE 请求到该 Entity id。
2. 验证返回错误码。

##### 请求参数

URI：Entity id

##### 预期返回结果

**ErrNum**：409  
**ErrMsg**：Entity 被挂载无法删除的错误信息  
**Data**：null

#### 11.4.4 E-6-004：删除最大编号 Entity 后新建不复用 ID（业务规则）

##### 设计思路

验证 Entity ID 序号分配单调不回退：删除当前最大编号的 Entity 后，新建 Entity 的 ID 序号必须大于被删 ID，不会复用（issue #132 的 ABA 场景）。

##### 前提数据准备

已创建 Entity-Type `department`；创建一个 Entity 并记录其自动分配的 `id`。

##### 执行步骤

1. 发送 DELETE 请求删除该 Entity，验证删除成功。
2. 再次创建 Entity（仅传 `name`、`type`），记录新 `id`。
3. 断言新 ID 不等于被删 ID，且其序号大于被删 ID 序号。

##### 请求参数

第一步 URI：被删 Entity id  
第三步 Body：

```json
{
    "name": "ent_recreated",
    "type": "department"
}
```

##### 预期返回结果

第一步：**ErrNum**：200，Data 为 null。  
第三步：**ErrNum**：200，返回 `id` 满足 `seq(new_id) > seq(deleted_id)` 且 `new_id != deleted_id`。

---

## 12. 查询配额计划

### 12.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 查询配额计划 |
| 方法 | GET |
| 路径 | `/open-api/v1/entities/{id}/quota-plan` |
| 说明 | 查询 Entity 的配额计划，含实时余额 |

### 12.2 接口参数说明

#### 12.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| id | string | Y | Entity 标识 |

#### 12.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| unlimited | bool | 是否无限配额 |
| pass_when_no_enough_quota | bool | 配额不足时是否放行 |
| quota | int64 | 配额总量 |
| unit | string | 配额单位 |
| reset_period | string | 配额重置周期 |
| balance | object | 余额状态，包含 used 和 remaining |

### 12.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-7-001 | 查询 Entity 配额计划 | 正常参数 | 返回完整 quota_plan 含 balance |
| E-7-002 | 查询 RMB 配额余额精度 | 正常参数 | balance 从 Redis 实时读取，精度 1e-8 |

### 12.4 测试场景详细设计

#### 12.4.1 E-7-001：查询 Entity 配额计划（正常参数）

##### 设计思路

验证独立 quota-plan 接口返回完整配额计划与余额，余额从 Redis 实时读取。

##### 前提数据准备

已创建非无限配额 Entity。

##### 执行步骤

1. 发送 GET 请求到 `/open-api/v1/entities/{id}/quota-plan`。
2. 验证返回包含 `balance`。

> 注：当前集成测试使用内存 Mock Redis，测试进程无法直接写入 Redis 构造非零 used。非零 used 场景由 `model/quota`、`model/entity` 单元测试覆盖。

##### 请求参数

URI：`id`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| unlimited | false | Equals |
| quota | 与创建时一致 | Equals |
| balance | 非空对象 | IsObject |
| balance.used | 0 | Equals |
| balance.remaining | 与 quota 一致 | Equals |

---

#### 12.4.2 E-7-002：查询 RMB 配额余额精度（正常参数）

##### 设计思路

验证 Entity 的 `unit=RMB` 配额余额支持 8 位小数精度，且余额从 Redis 实时读取。

##### 前提数据准备

已创建 `unit=RMB`、`quota=2000.12345678` 的 Entity。

##### 执行步骤

1. 创建 Entity 并 PATCH 为 `unit=RMB`、`quota=2000.12345678`。
2. 发送 GET 请求到 `/open-api/v1/entities/{id}/quota-plan`。
3. 验证返回 `unit=RMB`、`quota=2000.12345678`、`balance.used=0`、`balance.remaining=2000.12345678`。

> 注：当前集成测试使用内存 Mock Redis，测试进程无法直接写入 Redis 构造非零 used。RMB 精度场景由 `model/quota`、`model/entity` 单元测试覆盖。

##### 请求参数

URI：`id`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| unit | "RMB" | Equals |
| quota | 2000.12345678 | Equals |
| balance.used | 0 | Equals |
| balance.remaining | 2000.12345678 | Equals |

---

## 13. 重置配额余额

### 13.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 重置配额余额 |
| 方法 | POST |
| 路径 | `/open-api/v1/entities/{id}/quota-plan/reset` |
| 说明 | 重置 Entity 的配额余额 |

### 13.2 接口参数说明

#### 13.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| id | string | Y | Entity 标识 |

##### Body 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| quota | int64 | N | 重置后的配额总量 |
| reason | string | N | 重置原因 |

#### 13.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | Entity 标识 |
| previous_quota | int64 | 重置前配额 |
| new_quota | int64 | 重置后配额 |
| balance | object | 余额变更详情 |
| balance.previous_remaining | int64 | 重置前剩余量 |
| balance.new_remaining | int64 | 重置后剩余量 |
| balance.used | int64 | 当前已用量 |

### 13.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-8-001 | 重置配额余额 | 正常参数 | used=0，new_remaining=previous_quota |
| E-8-002 | 重置并修改 quota | 正常参数 | new_quota 和 new_remaining 同步更新 |

### 13.4 测试场景详细设计

#### 13.4.1 E-8-001：重置配额余额（正常参数）

##### 设计思路

验证不传 quota 时按当前 quota 重置余额。

##### 前提数据准备

已创建非无限配额 Entity。

##### 执行步骤

1. 发送 POST 请求到 `/open-api/v1/entities/{id}/quota-plan/reset`。
2. 验证返回的 `balance.used=0`，`new_remaining=previous_quota`。

##### 请求参数

```json
{
    "reason": "test reset"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| id | 与 URI 一致 | Equals |
| previous_quota | 与当前 quota 一致 | Equals |
| new_quota | 与 previous_quota 一致 | Equals |
| balance.used | 0 | Equals |
| balance.new_remaining | 与 new_quota 一致 | Equals |

---

#### 13.4.2 E-8-002：重置并修改 quota（正常参数）

##### 设计思路

验证传入 quota 时同步更新 quota 并重置余额。

##### 前提数据准备

已创建非无限配额 Entity。

##### 执行步骤

1. 发送 POST 请求，传入新的 `quota`。
2. 验证返回的 `new_quota` 和 `new_remaining` 均为新值，`used=0`。

##### 请求参数

```json
{
    "quota": 200000,
    "reason": "reset"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| new_quota | 200000 | Equals |
| balance.new_remaining | 200000 | Equals |
| balance.used | 0 | Equals |

---

#### 13.4.3 E-8-003：重置 RMB 配额余额（正常参数）

##### 设计思路

验证 Entity 的 `unit=RMB` 配额重置可指定小数新配额，余额精度保持 8 位小数。

##### 前提数据准备

已创建 `unit=RMB`、`quota=50.5` 的 Entity，并产生一定用量。

##### 执行步骤

1. 发送 POST 请求到 `/open-api/v1/entities/{id}/quota-plan/reset`，`quota=300.1234`。
2. 验证返回 `previous_quota=50.5`、`new_quota=300.1234`。
3. 验证 `balance.used=0`、`balance.new_remaining=300.1234`。

##### 请求参数

```json
{
    "quota": 300.1234,
    "reason": "adjust entity rmb quota"
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| previous_quota | 50.5 | Equals |
| new_quota | 300.1234 | Equals |
| balance.used | 0 | Equals |
| balance.new_remaining | 300.1234 | Equals |

---

## 14. 更新配额计划时的余额调整

### 14.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | Entity |
| 接口名称 | 更新配额计划（余额差异化调整） |
| 方法 | `PUT` / `PATCH` |
| 路径 | `/open-api/v1/entities/{id}` |
| 说明 | 当请求体中包含 `quota_plan` 且 `quota`/`unit`/`unlimited` 任一发生变化时，调整 Redis 余额：`quota` 变化且单位不变时保留 used，`unit` 变化或 `unlimited` 切换时重置；普通属性修改不触发余额变更。 |

### 14.2 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| E-9-001 | 仅修改 quota（total_token）保留 used | 正常参数 | 无使用量时 `remaining=新 quota`；非零 used 由单元测试覆盖 |
| E-9-002 | RMB 配额仅修改 quota 保留 used | 正常参数 | 无使用量时精度保持 8 位；非零 used 由单元测试覆盖 |
| E-9-003 | 修改 unit 重置 used 与 remaining | 正常参数 | `used=0`，`remaining=新 quota` |
| E-9-004 | unlimited false -> true 重置为 sentinel | 正常参数 | `used=0`，`remaining=100000000` |
| E-9-005 | unlimited true -> false 按新 quota 初始化 | 正常参数 | `used=0`，`remaining=新 quota` |
| E-9-006 | 普通属性修改不影响配额余额 | 正常参数 | 修改 `allow_models` 等，余额不变 |
| E-9-007 | 仅修改 quota（单位不变）时保留非零 used | 正常参数 | 预置 Redis 剩余 400（used=600），修改 quota 为 800 后 `used=600`、`remaining=200` |
| E-9-008 | RMB 配额仅修改 quota 时保留非零 used | 正常参数 | 预置 Redis 剩余 400.0000（used=600.1234），修改 quota 为 800 后 `used=600.1234`、`remaining=199.8766` |
| E-9-009 | 配额总量修改为 0 后剩余额度清零（回归 issue #136） | 正常参数 | 预置 Redis 剩余 400（used=600），修改 quota 为 0 后 `remaining=0`，且 Redis 余额同步为 0 |
| E-9-010 | RMB 配额总量修改为 0 后剩余额度清零 | 正常参数 | 预置 Redis 剩余 400.0000（used=600.1234），修改 quota 为 0 后 `remaining=0`，且 Redis 余额同步为 0 |

### 14.3 测试场景详细设计

> 说明：本组集成测试使用嵌入式 Redis（miniredis），测试进程可通过 `ServerManager.SetQuotaRemaining` / `GetQuotaRemaining` 直接读写 Redis，因此非零 used 路径（AK-9-007/008）与 quota 清零路径（AK-9-009/010）均在集成测试中覆盖。

#### 14.3.1 E-9-001：仅修改 quota（total_token）保留 used

##### 设计思路

验证单位不变、仅调额时，历史已用量 `used` 被保留，`remaining = max(0, 新 quota - used)`。

##### 前提数据准备

已创建 `unit=total_token`、`quota=1000` 的 Entity。

##### 执行步骤

1. 发送 PATCH 请求，仅修改 `quota_plan.quota=500`（同单位）。
2. 通过 GET `/entities/{id}/quota-plan` 查询余额。
3. 验证 `balance.used=0`、`balance.remaining=500`（无使用量时）。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 500,
        "unit": "total_token"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.quota | 500 | Equals |
| balance.used | 0 | Equals |
| balance.remaining | 500 | Equals |

---

#### 14.3.2 E-9-002：RMB 配额仅修改 quota 保留 used

##### 设计思路

验证 `unit=RMB` 时，仅调额同样保留 `used`，小数计算精度保持 8 位。

##### 前提数据准备

已创建 `unit=RMB`、`quota=1000.1234` 的 Entity。

##### 执行步骤

1. 发送 PATCH 请求，`quota_plan.quota=800.0000`。
2. 查询 quota-plan 接口。
3. 验证 `balance.used=0`、`balance.remaining=800.0000`（无使用量时）。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 800.0000,
        "unit": "RMB"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.quota | 800.0000 | InDelta(1e-5) |
| balance.used | 0 | InDelta(1e-5) |
| balance.remaining | 800.0000 | InDelta(1e-5) |

---

#### 14.3.3 E-9-003：修改 unit 重置 used 与 remaining

##### 设计思路

验证 `unit` 变化时，由于新旧单位无法直接换算，`used` 清零并按新 quota 重置。

##### 前提数据准备

已创建 `unit=total_token`、`quota=1000` 的 Entity。

##### 执行步骤

1. 发送 PATCH 请求，将 `unit` 改为 `RMB`，`quota=888.88`。
2. 查询 quota-plan 接口。
3. 验证 `balance.used=0`、`balance.remaining=888.88`。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 888.88,
        "unit": "RMB"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.unit | "RMB" | Equals |
| balance.used | 0 | Equals |
| balance.remaining | 888.88 | InDelta(1e-5) |

---

#### 14.3.4 E-9-004：unlimited 由 false 改为 true 重置为 sentinel

##### 设计思路

验证切换为无限配额时，`used` 清零，`remaining` 置为 sentinel 值 `100000000`。

##### 前提数据准备

已创建有限配额 Entity。

##### 执行步骤

1. 发送 PATCH 请求，`quota_plan.unlimited=true`。
2. 查询 quota-plan 接口。
3. 验证 `balance.used=0`、`balance.remaining=100000000`。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": true
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.unlimited | true | Equals |
| balance.used | 0 | Equals |
| balance.remaining | 100000000 | Equals |

---

#### 14.3.5 E-9-005：unlimited 由 true 改为 false 按新 quota 初始化

##### 设计思路

验证从无限配额切回有限配额时，按新 `quota` 初始化余额。

##### 前提数据准备

已创建 `unlimited=true` 的 Entity。

##### 执行步骤

1. 发送 PATCH 请求，`quota_plan.unlimited=false`、`quota=500`、`unit=total_token`。
2. 查询 quota-plan 接口。
3. 验证 `balance.used=0`、`balance.remaining=500`。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 500,
        "unit": "total_token"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| quota_plan.unlimited | false | Equals |
| balance.used | 0 | Equals |
| balance.remaining | 500 | Equals |

---

#### 14.3.6 E-9-006：普通属性修改不影响配额余额

##### 设计思路

验证仅修改 Entity 普通属性（如 `allow_models`）时，不触发 `ApplyQuotaPlanChange`，余额保持不变。

##### 前提数据准备

已创建有限配额 Entity。

##### 执行步骤

1. 发送 PATCH 请求，修改 `allow_models=["gpt-4"]`。
2. 查询 quota-plan 接口。
3. 验证 `balance.used=0`、`balance.remaining=1000`（未变更）。

##### 请求参数

```json
{
    "allow_models": ["gpt-4"]
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| allow_models | ["gpt-4"] | Equals |
| balance.used | 0 | Equals |
| balance.remaining | 1000 | Equals |

---


#### 14.3.7 E-9-007：仅修改 quota（total_token）保留非零 used

##### 设计思路

验证“仅修改 quota、单位不变”时保留已使用量：预置 Redis 剩余 400（即 used=600），将 quota 从 1000 修改为 800 后，`used` 保持 600，`remaining` 调整为 200。

##### 前提数据准备

已创建有限配额 Entity（total_token / RMB），并通过 `ServerManager.SetQuotaRemaining` 预置 Redis 余额。

##### 执行步骤

1. 发送 PATCH 请求修改 `quota_plan.quota`。
2. 查询 quota-plan 接口并校验 `balance`。
3. 通过 `ServerManager.GetQuotaRemaining` 校验 Redis 余额。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 800,
        "unit": "total_token"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| balance.used | 600 | Equals |
| balance.remaining | 200 | Equals |
| Redis 余额（`GetQuotaRemaining`） | 200 | Equals |

#### 14.3.8 E-9-008：RMB 配额仅修改 quota 保留非零 used

##### 设计思路

验证 RMB 配额“仅修改 quota、单位不变”时保留已使用量（精度 1e-8 元）：预置 Redis 剩余 400.0000（used=600.1234），将 quota 从 1000.1234 修改为 800.0000 后，`used` 保持 600.1234，`remaining` 调整为 199.8766。

##### 前提数据准备

已创建有限配额 Entity（total_token / RMB），并通过 `ServerManager.SetQuotaRemaining` 预置 Redis 余额。

##### 执行步骤

1. 发送 PATCH 请求修改 `quota_plan.quota`。
2. 查询 quota-plan 接口并校验 `balance`。
3. 通过 `ServerManager.GetQuotaRemaining` 校验 Redis 余额。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 800.0000,
        "unit": "RMB"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| balance.used | 600.1234 | Equals |
| balance.remaining | 199.8766 | Equals |
| Redis 余额（`GetQuotaRemaining`） | 199.8766 | Equals |

#### 14.3.9 E-9-009：配额总量修改为 0 后剩余额度清零（回归 issue #136）

##### 设计思路

回归 [ai-gateway-api#136](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/136)：预置 Redis 剩余 400（used=600），将 total_token 配额总量修改为 0 后，`remaining` 清零（`max(0, 0 - 600)`），且 Redis 余额被同步为 0，BFE 将立即以 QuotaExhausted 拒绝请求。

##### 前提数据准备

已创建有限配额 Entity（total_token / RMB），并通过 `ServerManager.SetQuotaRemaining` 预置 Redis 余额。

##### 执行步骤

1. 发送 PATCH 请求修改 `quota_plan.quota`。
2. 查询 quota-plan 接口并校验 `balance`。
3. 通过 `ServerManager.GetQuotaRemaining` 校验 Redis 余额。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 0,
        "unit": "total_token"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| balance.used | 0 | Equals |
| balance.remaining | 0 | Equals |
| Redis 余额（`GetQuotaRemaining`） | 0 | Equals |

#### 14.3.10 E-9-010：RMB 配额总量修改为 0 后剩余额度清零

##### 设计思路

RMB 配额的 quota 清零场景：预置 Redis 剩余 400.0000（used=600.1234），将 quota 从 1000.1234 修改为 0 后，`remaining` 清零，且 Redis 余额被同步为 0。

##### 前提数据准备

已创建有限配额 Entity（total_token / RMB），并通过 `ServerManager.SetQuotaRemaining` 预置 Redis 余额。

##### 执行步骤

1. 发送 PATCH 请求修改 `quota_plan.quota`。
2. 查询 quota-plan 接口并校验 `balance`。
3. 通过 `ServerManager.GetQuotaRemaining` 校验 Redis 余额。

##### 请求参数

```json
{
    "quota_plan": {
        "unlimited": false,
        "quota": 0,
        "unit": "RMB"
    }
}
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| balance.used | 0 | Equals |
| balance.remaining | 0 | Equals |
| Redis 余额（`GetQuotaRemaining`） | 0 | Equals |

## 15. 依赖与数据准备

1. 必须预先创建至少两种不同 `level` 的 Entity-Type 以验证层级约束。
2. 配额/余额用例依赖 Redis Mock。
3. 删除约束用例需要预先准备子 Entity 或挂载的 API-Key。

## 16. 注意事项

1. Entity 详情、创建、更新返回的 `quota_plan` 不含 `balance`，需通过独立 quota-plan 接口验证余额。
2. `name` 全局唯一，测试用例间注意清理。
3. 层级修改必须保证父节点 Entity-Type 的 `level` 小于当前节点。
4. 测试环境 `SkipTokenValidate=true`，无需认证头。
5. 自动生成的 Entity ID 为 `entity-{seq}`，序号来自 `entity_id_seq` 序列表且永不复用；用例间不能假设 ID 从 1 开始或连续，断言时应以相对大小（递增）而非绝对值为准。
6. 集成测试启动的是项目根目录预编译的 `ai-gateway-api.exe`，修改被测代码后必须先重新编译（`go build -o ai-gateway-api.exe .`），否则测试运行的是旧二进制。
7. 不要在集成测试中做多并发写请求：SQLite 文件库在多连接并发写下会触发 busy 等待直至请求超时（旧实现同样存在该限制），并发正确性由 DAO 层单元测试覆盖。
