# API-Key 与 Entity 关联及模型继承

## 1. 概述

在 AI 网关的权限与配额模型中，**API-Key** 是请求链路的直接凭证，而 **Entity** 是业务组织单元（如部门、项目、用户组）。API-Key 可以挂载到一个 Entity 上，从而继承该 Entity 及其父级 Entity 的：

- 模型白名单 / 黑名单；
- 配额计划；
- 限流策略；
- 路由规则。

这种设计允许管理员在组织层面统一管理策略，而 API-Key 只作为最终的使用凭证。

---

## 2. 核心概念

| 概念 | 说明 | 数据库表 |
|------|------|---------|
| API-Key | 实际的请求凭证，包含 key 值、状态、过期时间等 | `api_keys` |
| Entity | 业务组织单元，支持层级结构 | `entities` |
| Entity-Type | Entity 类型定义，包含层级级别（1-5） | `entity_types` |
| QuotaPlan | 配额计划 | `quota_plans` |
| RateLimitPolicy | 限流策略 | `rate_limit_policies` |
| RouteRules | 路由规则（API-Key / Entity / Global 三级） | `route_rules` |

API-Key 通过 `api_keys.entity_id` 字段与 Entity 关联；Entity 通过 `entities.parent_id` 字段形成层级树。

---

## 3. 数据模型

### 3.1 `api_keys` 表关联字段

```sql
`entity_id` varchar(64) DEFAULT NULL COMMENT '挂载的Entity ID'
```

### 3.2 `entities` 表关联字段

```sql
`parent_id` varchar(64) DEFAULT NULL COMMENT '父Entity ID'
`allow_models` TEXT COMMENT '允许访问的模型白名单（JSON数组）'
`block_models` TEXT COMMENT '禁止访问的模型黑名单（JSON数组）'
`quota_plan_id` BIGINT DEFAULT NULL COMMENT '配额计划ID'
`rate_limit_policy_id` BIGINT DEFAULT NULL COMMENT '限流策略ID'
`route_rules_id` BIGINT DEFAULT NULL COMMENT '路由规则ID'
```

### 3.3 层级约束

创建或更新 Entity 时，必须满足：

- 父 Entity 必须存在；
- 父 Entity 的 `EntityType.Level` 必须 **小于** 当前 Entity 的 `Level`（数字越小级别越高）。

即：层级只能从高级别指向低级别，不能同级或反向。

**Entity-Type 侧的层级守卫**（issue #313）：`PATCH /entity-types/{type_name}` 修改 `level` 时，模型层在落库前全量复查该类型下已有 Entity 的存量父子关系（作为子级：父类型 level 必须小于新值；作为父级：子类型 level 必须大于新值），任一处冲突即返回 409 拒绝且不做任何修改；越界值（非 1-5）返回 422。`level` 取值不变或仅改 `description` 时不触发复查。

### 3.4 不可变字段

`type` 在 Entity 创建后固定，更新接口（PUT/PATCH）不得改写：

- 请求体携带的 `type` 与库中值**不同** → 拒绝（422 PARAM），库中值保持不变；
- 省略或与原值相同 → 放行（PATCH 省略保持原值；PUT 回传原值是正常回环场景）。

守卫分两层：接口层 fail-fast（`endpoints/openapi_v1/entity/validator.go`），模型层权威校验并在拒绝时记录操作日志（`model/entity/entity_manager.go` `UpdateEntity`，issue #178）。`type` 决定层级语义（Entity-Type 的 `level`），允许改写会破坏 §3.3 的层级不变量。

---

## 4. API-Key 与 Entity 的关联

### 4.1 关联方式

API-Key 创建或更新时，可通过 `entity_id` 字段挂载到某个 Entity。创建时还可传入 `key` 字段导入外部 API-Key（全局唯一，更新时忽略）：

```go
if param.EntityID != nil && *param.EntityID != "" {
    entity, err := rppm.entityStorager.FetchEntity(ctx, &shared.EntityFilter{EntityID: param.EntityID})
    if entity == nil {
        return xerror.WrapParamErrorWithMsg(fmt.Sprintf("Entity not found: %s", *param.EntityID))
    }
}
```

存在性守卫分两层：接口层 fail-fast（create / PUT / PATCH 均在写库前校验，`endpoints/openapi_v1/api_key/`），模型层权威校验（`CreateAPIKey` / `UpdateAPIKey`，issue #199）。`entity_id` 为空字符串表示解绑，不触发校验。

### 4.2 关联后的效果

挂载后，该 API-Key 在导出到 BFE 时会：

1. 合并 API-Key 自身与 Entity 层级的模型白名单；
2. 合并 Entity 层级的模型黑名单；
3. 收集 API-Key 自身及 Entity 层级向上的所有**非无限**配额计划；
4. 收集 API-Key 自身及 Entity 层级向上的所有**启用**限流策略；
5. 按优先级绑定 API-Key 级、**Entity 层级向上**、Global 级路由规则。

---

## 5. 模型继承规则

### 5.1 `allow_models`：交集继承

Entity 层级中，所有非空且不含 `*` 的 `allow_models` 会取**交集**。

```go
func intersectAllowModels(allAllowModels [][]string) []string {
    if len(allAllowModels) == 0 {
        return nil
    }
    result := make([]string, len(allAllowModels[0]))
    copy(result, allAllowModels[0])
    for _, models := range allAllowModels[1:] {
        result = intersectSlices(result, models)
    }
    return result
}
```

示例：

| Entity | allow_models |
|--------|-------------|
| 公司根 | `["*"]` |
| 部门 A | `["gpt-4", "gpt-3.5-turbo"]` |
| 项目 X | `["gpt-4", "claude-3"]` |
| API-Key | `[]`（未设置） |

最终允许的模型为部门 A 与项目 X 的交集：`["gpt-4"]`。

### 5.2 `block_models`：并集继承

Entity 层级中，所有非空且不含 `*` 的 `block_models` 会取**并集**。

示例：

| Entity | block_models |
|--------|-------------|
| 部门 A | `["gpt-4-32k"]` |
| 项目 X | `["davinci"]` |

最终禁止的模型为：`["gpt-4-32k", "davinci"]`。

### 5.3 API-Key 自身模型的优先级

API-Key 自身的 `models`（`allowed_models`）也会参与交集计算：

```go
var finalAllowModels []string
if len(apiKeyAllowModels) == 0 && len(entityAllowModels) == 0 {
    // 两者都为空或 * → 允许所有
} else if len(apiKeyAllowModels) == 0 {
    finalAllowModels = entityAllowModels
} else if len(entityAllowModels) == 0 {
    finalAllowModels = apiKeyAllowModels
} else {
    finalAllowModels = intersectSlices(apiKeyAllowModels, entityAllowModels)
}

if len(finalAllowModels) == 0 {
    // 交集为空 → 禁用该 Token，models 为空
}
```

**关键规则**：

- 空数组或 `["*"]` 表示不限制；
- API-Key 自身模型与 Entity 继承模型取交集；
- 若交集为空且双方都有非空非 `*` 配置，则该 API-Key 被禁用。

---

## 6. 配额计划层级合并

### 6.1 收集逻辑

API-Key 导出到 BFE 时，会收集 API-Key 自身及 Entity 层级向上的所有配额计划，**`unlimited=true` 的计划不会加入导出结果**：

```go
func (rlm *APIKeyRuleManager) fetchQuotaPlansWithEntityHierarchy(
    ctx context.Context,
    apiKey *api_key.APIKeyParam,
    productName string,
) ([]string, []ApikeyTag, error) {
    // 1. API-Key 自身配额计划（跳过 unlimited）
    if apiKey.QuotaPlanID != nil { ... }

    // 2. Entity 层级向上的配额计划（跳过 unlimited）
    if apiKey.EntityID != nil && *apiKey.EntityID != "" {
        entity, err := rlm.entityStorager.FetchEntity(...)
        if entity != nil {
            entityQuotaPlanIDs, entityTags, err := rlm.fetchEntityQuotaPlanHierarchy(ctx, entity, productName)
            quotaPlanIDs = append(quotaPlanIDs, entityQuotaPlanIDs...)
            tags = append(tags, entityTags...)
        }
    }
    return quotaPlanIDs, tags, nil
}
```

### 6.2 Entity 层级递归

```go
func (rlm *APIKeyRuleManager) fetchEntityQuotaPlanHierarchy(
    ctx context.Context,
    entity *entity.EntityParam,
    productName string,
) ([]string, []ApikeyTag, error) {
    // 当前 Entity 的 QuotaPlan（跳过 unlimited）
    if entity.QuotaPlanID != nil { ... }

    // 递归父 Entity
    if entity.ParentID != nil && *entity.ParentID != "" {
        parentEntity, err := rlm.entityStorager.FetchEntity(...)
        if parentEntity != nil {
            parentQuotaPlanIDs, parentTags, _ := rlm.fetchEntityQuotaPlanHierarchy(ctx, parentEntity, productName)
            quotaPlanIDs = append(quotaPlanIDs, parentQuotaPlanIDs...)
            tags = append(tags, parentTags...)
        }
    }
    return quotaPlanIDs, tags, nil
}
```

无限配额计划虽然不参与导出，但仍会影响 Entity 层级的标签收集。

在收集标签时，会根据 `entity.Type` 查询 `EntityType`，取其 `Level` 填充到 `ApikeyTag.TagLevel`；若对应 EntityType 不存在或 Level 无效，则导出失败并返回错误。

### 6.3 导出格式

每个配额计划导出为 BFE 的 `QuotaPlan`：

```go
type QuotaPlan struct {
    Id string
    Unlimited bool
    PassNoQuota bool
    RedisKey string
    ExpiredTime int64 // -1 表示永不过期
    Quota int64
    Unit string
}
```

- API-Key 自身配额计划的 `RedisKey` 为 `QUOTA_<api_key_value>`；
- Entity 配额计划的 `RedisKey` 为 `QUOTA_<entity_id>`。

这意味着 API-Key 可能同时受多个 Redis Key 的配额控制。

### 6.4 Tags

导出时还会为 API-Key 打上 Entity 层级的标签：

```go
type ApikeyTag struct {
    TagName string // 如 entity.type
    TagValue string // 如 entity.name
    TagLevel int    // 对应 EntityType.Level，取值为 1~5 的整数
}
```

每个 Entity 会生成一个 `TagName = Entity.Type`、`TagValue = Entity.Name` 的标签，并依据该 Entity Type 对应的 `EntityType.Level` 填充 `TagLevel`。

---

## 7. 限流策略层级合并

限流策略的收集逻辑与配额计划类似：

```go
func (m *RateLimitPolicyManager) fetchRateLimitPolicyIDsWithEntityHierarchy(
    ctx context.Context,
    apiKey *api_key.APIKeyParam,
) ([]int64, error) {
    policyIDs := make([]int64, 0)

    // API-Key 自身策略
    if apiKey.RateLimitPolicyID != nil {
        policyIDs = append(policyIDs, *apiKey.RateLimitPolicyID)
    }

    // Entity 层级向上递归
    if apiKey.EntityID != nil && *apiKey.EntityID != "" {
        entity, err := m.entityStorager.FetchEntity(ctx, &EntityFilter{EntityID: apiKey.EntityID})
        if entity != nil {
            entityPolicyIDs, _ := m.fetchEntityRateLimitPolicyIDs(ctx, entity)
            policyIDs = append(policyIDs, entityPolicyIDs...)
        }
    }
    return policyIDs, nil
}
```

导出时每个策略生成 `rlp-<policy_id>`，并与 API-Key 绑定。

> 详见《限流策略与导出.md》。

---

## 8. 路由规则层级

AI 路由导出时，API-Key 会按以下优先级绑定路由表：

```go
for _, apiKey := range apiKeys {
    var bindingList []string

    // 1. API-Key 级路由规则
    if apiKey.RouteRulesID != nil { ... }

    // 2. Entity 层级路由规则（自底向上遍历）
    currentEntityID := apiKey.EntityID
    for currentEntityID != nil && *currentEntityID != "" {
        entity, exists := entityMap[*currentEntityID]
        if !exists {
            break
        }
        if entity.RouteRulesID != nil {
            // 绑定 entity_<name>
        }
        currentEntityID = entity.ParentID
    }

    // 3. Global 级路由规则
    if globalRouteRules != nil { ... }
}
```

绑定顺序为：`apikey_xxx` → `entity_xxx`（直接 Entity）→ `entity_<parent>` → …… → `global_default`。BFE 按此顺序匹配时，通常先命中 API-Key 级规则，再依次命中各级 Entity 规则，最后命中 Global 兜底规则。

---

## 9. OpenAPI 查询时的表现

### 9.1 查询 API-Key 详情

`APIKeyManager.populateAssociatedData` 会自动填充：

- `QuotaPlan`（含 Balance）；
- `RateLimitPolicy`；
- `RouteRules`；
- `Entity`（仅摘要：id、name、type）。

### 9.2 查询 Entity 详情

`EntityManager.populateAssociatedData` 会填充：

- `QuotaPlan`（含 Balance）；
- `RateLimitPolicy`；
- `RouteRules`。

### 9.3 查询配额计划

- `/api-keys/{id}/quota-plan`：返回 API-Key 自身配额计划（含 `balance`）；
- `/entities/{id}/quota-plan`：返回 Entity 自身配额计划（含 `balance`）。

`GET /api-keys` 与 `GET /api-keys/{id}` 的返回中，`quota_plan` 同样包含 `balance`，由 `populateAssociatedData` 通过 `quotaCache` 实时读取 Redis 填充；无限配额返回 sentinel balance（`used=0`, `remaining=100000000`）。

---

## 10. 创建与删除时的一致性

### 10.1 创建 Entity

`EntityManager.CreateEntity`：

1. 校验 Entity-Type 和父 Entity 层级；
2. 创建 QuotaPlan、RateLimitPolicy、RouteRules（如有）；
3. 创建 Entity；
4. 事务提交后，若配额有限，通过 `quotaCache.SetRemaining` 向 Redis 写入初始剩余量。

### 10.2 删除 Entity

`EntityManager.DeleteEntity`：

1. 检查是否有子 Entity；
2. 级联删除 `quota_plan`、`rate_limit_policy`、`route_rules`；
3. 删除 Entity；
4. 事务提交后，调用 `quotaCache.DeleteKeys` 删除对应 Redis Key。

### 10.3 删除 API-Key

`APIKeyManager.DeleteAPIKey`：

1. 级联删除 API-Key 自身关联的 `quota_plan`、`rate_limit_policy`、`route_rules`；
2. 删除 API-Key；
3. 事务提交后，调用 `quotaCache.DeleteKeys` 删除对应 Redis Key。

---

## 11. 边界情况与注意事项

| 场景 | 行为 |
|------|------|
| API-Key 未挂载 Entity | 仅使用自身配置，不继承任何 Entity 策略 |
| Entity 未设置 allow_models | 不参与交集计算，视为不限制 |
| Entity allow_models 含 `*` | 跳过该层级的交集计算 |
| API-Key 与 Entity allow_models 交集为空 | 导出时禁用该 Token（`Enabled=false`） |
| Entity 层级成环 | 创建/更新时通过层级校验阻止 |
| 父 Entity 被删除 | 需先删除所有子 Entity |
| 多个配额计划 | API-Key 同时受多个 Redis Key 控制，BFE 侧需支持多配额校验 |
| API-Key `unlimited_quota=false` 但所有相关 quota plan 均为 `unlimited=true` | 导出到 BFE 的 `QuotaPlans` 为空，BFE 视 `UnlimitedQuota=false & QuotaPlans=[]` 为非法；导出层会兜底将该 token 的 `UnlimitedQuota` 修正为 `true`，保证配置可加载。该兜底**不修改**数据库中 `api_keys.unlimited_quota` 的原始值 |
| 路由规则未启用 | 不加入绑定列表 |

---

## 12. 设计建议

1. **模型继承可视化**：Dashboard 应展示每个 API-Key 最终生效的 allow_models / block_models，便于排查权限问题。
2. **配额层级冲突处理**：当前多个配额计划直接叠加，建议明确优先级或累加规则。
3. **Entity 删除保护**：删除有子 Entity 或有关联 API-Key 的 Entity 时应给出警告或强制解绑。
4. **Redis Key 清理**：API-Key / Entity 删除时建议主动清理对应 Redis Key，避免残留。

---

## 13. 相关文件索引

| 文件 | 说明 |
|------|------|
| `model/entity/entity.go` | Entity 模型定义 |
| `model/entity/entity_manager.go` | Entity 创建/更新/删除、关联填充 |
| `model/entity/entity_type.go` | EntityType 模型与层级校验 |
| `model/api_key/api_key.go` | API-Key 模型、GetRemainingQuota、级联删除 |
| `model/imods/exporter.go` | mod-api-key 导出：模型继承、配额计划层级合并 |
| `model/imods/mod_api_key_rule.go` | APIKeyRuleManager 定义 |
| `model/imods/ai_route_exporter.go` | AI 路由绑定关系导出 |
| `model/rate_limit_policy/rate_limit_policy_manager.go` | 限流策略层级收集与导出 |
| `endpoints/openapi_v1/api_key/*.go` | API-Key 端点 |
| `endpoints/openapi_v1/entity/*.go` | Entity 端点 |
| `endpoints/openapi_v1/entity_type/*.go` | Entity-Type 端点 |
