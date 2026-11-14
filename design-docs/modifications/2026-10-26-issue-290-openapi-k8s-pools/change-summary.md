# OpenAPI 暴露 K8s 实例池只读查询接口（Issue #290）变更摘要

## 1. 背景

K8s 实例池（`k8s_pools`）目前只有 InnerAPI 读写接口，其中两个只读接口尚未在 OpenAPI 暴露：

- `GET /inner-api/v1/k8s_pools/{name}`：查询单个 pool 条目
- `GET /inner-api/v1/k8s_pools`：查询全部 pool 列表

控制台/Web 使用普通用户身份访问 OpenAPI，无法直接读取 K8s 实例池，导致：

1. provider 配置为 `instance_source=k8s_pool` 时，无法展示当前可选的 K8s pool 列表，只能手工输入 `k8s_pool_name`。
2. provider 详情页无法根据 `k8s_pool_name` 展示该池当前可用的实例列表。
3. K8s 实例池的实际状态只能通过 InnerAPI 查看，前端无法形成完整的配置/详情体验。

本变更将上述两个只读接口以 OpenAPI 形式暴露，供控制台消费；写入/删除仍只保留在 InnerAPI，由 K8s 发现组件使用。

> 关联 issue：[rainway-ai-gateway/ai-gateway-api#290](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/290)

## 2. 目标

1. 新增 `GET /open-api/v1/k8s_pools`，返回全部 K8s pool 的摘要列表。
2. 新增 `GET /open-api/v1/k8s_pools/{name}`，返回指定 pool 的实例列表、实例数量和最近同步时间。
3. 复用现有 `ik8s_pool.K8sPoolManager` / storage，不新增表、不改变 InnerAPI 写入协议。
4. 授权复用 `FeatureK8sPool` + `ActionRead`。
5. 更新 OpenAPI 契约文档、路由注册、handler、单元测试与集成测试。
6. 使前端可以完成：
   - provider 配置时的 K8s pool 下拉选择；
   - provider 详情页按 `k8s_pool_name` 展示实时实例。

## 3. 非目标

- 不在 OpenAPI 开放 K8s pool 的 `PUT` / `DELETE` 写接口。
- 不修改 InnerAPI `/k8s_pools` 的写入协议和实例快照语义。
- 不修改 provider 的请求/响应契约；本变更只新增 K8s pool 只读查询通道。
- 不修改 `k8s_pools` 表结构、存储格式和同步 fan-out 逻辑。
- 不引入分页；本期与 InnerAPI 一致，返回全部 pool。

## 4. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api` |
| 主要模块 | `endpoints/openapi_v1/k8s_pools/`（新增）、`endpoints/openapi_v1/endpoints.go`（路由注册） |
| 复用模块 | `model/ik8s_pool/`、`storage/rdb/k8s_pool/`、`stateful/container` |
| 接口契约 | OpenAPI 新增 `/open-api/v1/k8s_pools`、`/open-api/v1/k8s_pools/{name}` 两个只读 GET |
| 数据库 | 无 DDL、无数据迁移 |
| 数据面 | 无 BFE / conf-agent / EPP 影响 |
| 控制台 | 前端可用新接口完成 pool 选择与 provider 详情实例展示；本仓仅提供 API |
| 测试 | handler 单测 + OpenAPI 集成测试 + schema 契约测试 |

## 5. 关键决策

| 决策 | 说明 |
|------|------|
| 只暴露只读 GET | K8s 池成员唯一写入方是 K8s 发现组件；OpenAPI 不提供写入口，避免控制台与发现组件互相覆盖。 |
| 路径与 InnerAPI 对齐 | OpenAPI 使用 `/open-api/v1/k8s_pools` 和 `/open-api/v1/k8s_pools/{name}`，与 InnerAPI 的资源语义保持一致。 |
| 复用 manager/storage | handler 直接调用 `container.K8sPoolManager.FetchPoolList` / `FetchPool`，不新增模型层逻辑，保证读取结果与 InnerAPI 同源。 |
| 响应结构与 InnerAPI 对齐 | 列表返回 `{list:[{name,instance_count,last_sync_time}]}`；详情返回 `{name,instances,instance_count,last_sync_time}`。 |
| 详情暴露 `disable` 字段 | 现有 `iprovider.ProviderInstance` 序列化包含 `disable`；OpenAPI 响应保留该字段并标注为系统字段，避免与 InnerAPI 实际响应不一致。 |
| 不复用 InnerAPI handler DTO | OpenAPI package 定义自己的轻量响应类型，避免 `endpoints/openapi_v1` 依赖 `endpoints/innerapi_v1` 的 handler 包。 |
| 不分页 | 池数量受 K8s Service 数量约束，且 provider 下拉需要全量列表；本期与 InnerAPI 保持完全一致。 |
| 授权复用 `FeatureK8sPool` | 与 InnerAPI 读接口一致；`ScopeSystemReadonly` 已具备 `ActionRead`。若未来 provider 管理下放到 product scope，再评估授权映射。 |
| provider 契约不变 | 前端在 provider 详情中按 `k8s_pool_name` 调用新接口展示实例，不在 provider 响应中重复维护实例列表。 |

## 6. 影响与风险

| 项目 | 影响 |
|------|------|
| 向后兼容 | 纯新增端点，不影响既有 OpenAPI / InnerAPI 行为。 |
| 权限 | 仅具备 `FeatureK8sPool` Read 的角色可访问；当前 system admin / system readonly 已覆盖。 |
| 数据一致性 | 直接读 `k8s_pools` 当前值，反映最近一次发现组件同步结果；`last_sync_time` 可用于判断新鲜度。 |
| 敏感信息 | 响应包含后端实例地址；仅对有权限的用户暴露，与 InnerAPI 的读权限一致。 |
| 性能 | 列表读取全部 pool，无分页；当前池数量有限，后续如有规模问题再扩展分页。 |
| 前端联动 | ai-gateway-web 需要接入两个新接口；不接入时既有功能不受影响。 |

## 7. 关联文档

- 接口变更：本目录 `api-changes.md`
- 设计变更：本目录 `design-changes.md`
- InnerAPI 来源定义：`design-docs/api-define/InnerAPI接口定义/k8s-pools.md`
- 目标 OpenAPI 文档：`design-docs/api-define/OpenAPI接口定义/k8s-pools.md`（待新增）
- OpenAPI 通用约定：`design-docs/api-define/OpenAPI接口定义/00-common.md`
- 相关 issue：`#290`；provider 实例池镜像问题见 `#289`

## 8. 状态

- [x] 变更摘要、API 变更、设计变更文档
- [x] `api-define/OpenAPI接口定义/k8s-pools.md` 新增
- [x] `api-define/OpenAPI接口定义/README.md` 索引更新
- [x] `api-define/OpenAPI接口定义/providers.md` 补充 K8s pool 实例查询指引
- [x] `api-define/InnerAPI接口定义/k8s-pools.md` 补充 `disable` 系统只读字段说明
- [x] OpenAPI handler、路由实现（`endpoints/openapi_v1/k8s_pools/`）
- [x] 单元测试、集成测试、OpenAPI schema 契约测试
- [ ] 前端接入（ai-gateway-web，跨仓）
