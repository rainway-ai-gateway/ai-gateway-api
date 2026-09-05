# Issue #135：Entity 名称字符集放开，允许 `@`

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#135](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/135)

## 1. 概述

### 1.1 问题现象

当前系统中用户名称统一采用 `用户名@项目名` 形式（如 `zhanghuzhenyu@default`），但 Entity 名称校验规则（Issue #81 收紧后）仅允许小写字母、数字、`_`、`-`，禁止 `@`，导致创建或编辑该类实体时表单验证失败。

### 1.2 变更目标

Entity 名称（`EntityName`）字符集在原有基础上放开 `@`，支持 `用户名@项目名` 形式；首尾不允许 `@`（与 `-`、`_` 一致）；其余规则（小写字母、长度 1-64、全局唯一、无空白字符）不变。

`EntityTypeName` 字符集保持不变（仍不允许 `@`）。

### 1.3 变更范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api` |
| 涉及模块 | `lib/validate` |
| 接口契约 | `POST /entities`、`PATCH/PUT /entities/{id}` 请求参数 `name` 的合法性条件放宽 |
| 数据迁移 | 无 |

## 2. 安全性评估（`@` 对各消费方的影响）

变更前逐项核查了 entity name 的全部消费方，结论：**安全**。

| 消费方 | 用法 | 结论 |
|--------|------|------|
| URL 路由 | 实体一律按 `{id}` 寻址；name 仅作列表接口 query 过滤参数 | 安全 |
| 配置导出（数据面） | name 拼为 `entity_<name>` 作为 **JSON** map key（`model/imods/ai_route_exporter.go:172-175`）；作为 API key tag 的 value（`model/imods/exporter.go:454-470`）。conf-agent 原样写 `.data` JSON，BFE 按 JSON 解析——**不会成为 TOML bare key**（`@` 在 TOML 裸键中非法，为主要风险点，已排除） | 安全 |
| API key 绑定 | 按 `entity_id` 引用，不经过 name 字符校验 | 安全 |
| 配额 Redis key | `QUOTA_<entity_id>`，name 不参与 | 安全 |
| 限流导出 | 使用 policy ID 与规则名（字符集未改），name 不参与 | 安全 |
| 数据库 | `entities.name VARCHAR(128)` + 唯一索引 | 安全 |
| 分隔符解析/二次正则 | 全仓无按分隔符切分或对 name 做二次正则匹配的代码 | 安全 |

已知行为影响（非缺陷）：实体改名会导致导出文件中 `entity_<name>` key 整体替换（同一快照内 key 与 binding 一致生成，无悬挂引用）；导出文件中会出现 `entity_zhanghuzhenyu@default` 形式的 key，JSON 中合法。

## 3. 详细设计

### 3.1 `lib/validate/validate.go` 修改

新增独立字符集正则（不再与 `EntityTypeName` 共用）：

```go
// entityNameToken matches the character set used by EntityName.
// '@' is allowed to support names in the form "user@project".
entityNameToken = regexp.MustCompile(`^[a-z0-9_@_-]+$`)
```

`EntityName` 改用 `entityNameToken`，首尾限制扩展为不能以 `-`、`_`、`@` 开头或结尾。`EntityTypeName` 仍使用 `entityTypeToken`（`^[a-z0-9_-]+$`），不受影响。

## 4. 涉及文件清单

| 文件 | 修改内容 |
|------|----------|
| `lib/validate/validate.go` | 新增 `entityNameToken` 正则；`EntityName` 改用该字符集并扩展首尾限制 |
| `lib/validate/validate_test.go` | `TestEntityName` 增加含 `@` 合法用例与首尾 `@`、`#` 非法用例 |
| `design-docs/api-define/OpenAPI接口定义/00-common.md` | §17 EntityName 合法性条件更新 |

## 5. 测试计划

| 用例 | 输入 | 期望 |
|------|------|------|
| 合法-含 `@` | `"dep@1"`、`"zhanghuzhenyu@default"` | 通过 |
| 非法-以 `@` 开头/结尾 | `"@dep"`、`"dep@"` | 失败 |
| 非法-其他特殊字符 | `"dep#1"`（替代原 `"dep@1"` 用例） | 失败 |
| 其余既有用例 | 不变 | 不变 |

回归：`go test ./lib/validate/...`、`go test ./endpoints/openapi_v1/entity/...`、`go build ./...`。

## 6. 实施状态

- [x] `lib/validate/validate.go`：`entityNameToken` + `EntityName` 首尾限制扩展；
- [x] `lib/validate/validate_test.go`：用例更新；
- [x] `design-docs/api-define/OpenAPI接口定义/00-common.md` §17 同步；
- [ ] 提交 PR 并关联 Issue #135。

## 7. 参考文档

- [Issue #135](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/135)
- `design-docs/modifications/2026-08-24-issue-81-entity-name-validation/`（EntityName 校验收紧的先例，本次为反向放宽）
- `ai-gateway-api/lib/validate/validate.go`
