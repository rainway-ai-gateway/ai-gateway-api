# OpenAPI 单实例配置 + 视图去除 standby（内部实现保留）变更摘要

## 1. 背景

EPP 实例池（`/epp-pool`）当前允许每组 1~2 实例（1=仅主，2=主+备），且 OpenAPI `/epp-assignments` 视图将备实例（standby）作为 `standby` 字段**读时展开**下发。现决定将 **OpenAPI 对外契约**收敛：

1. 部署形态收敛为单实例组——`/epp-pool` 写入面不再接受主备双实例组；
2. OpenAPI 视图不再暴露 `standby` 字段。

**内部实现全部保留**：模型层 `expandPrimaryAndStandby` 与 `ClusterAssignmentView.Standby`、InnerAPI `epp_data` 导出中的 `standby`、server_data_conf `EPPAddr` 有序主备列表均不动——存量双实例组数据继续按原语义导出，新数据自然得到 `standby: null` / 单元素 `EPPAddr`（该形态在 `2026-09-14-epp-pool-unify-group-size` 中已定义并支持）。

## 2. 目标

1. `PATCH /epp-pool` 组规模校验：每组 1~2 实例 → **恰好 1 实例**（拒绝空组与 2 个及以上实例的组）。
2. `GET /epp-assignments`、`PUT /epp-assignments/{cluster}` 响应：删除 `standby` 字段（key 物理消失）。

## 3. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api` |
| 模型层 | `model/epp_pool/epp_pool.go`（仅 `validateGroupSize` 与一处注释） |
| 端点层 | `endpoints/openapi_v1/epp_assignments/data.go`（两个响应结构删 `Standby` 字段） |
| 测试 | 单测 3 文件、集成测试 4 文件改写 |
| 文档 | `epp-pool.md`、`epp-assignments.md`、2 处视图描述、CHANGELOG |
| 不在范围 | 模型层 standby 展开逻辑、`epp_data`/`EPPAddr` 导出契约、存储层、存量数据、EPP/BFE 仓库、前端 UI 文档（`static/docs/zh-cn/05-epp-schedule.md`） |

## 4. Breaking Changes

| 接口 | 变化 | 影响 |
|------|------|------|
| `PATCH /epp-pool` | 双实例组由合法变为 422 | 部署流程须按单实例组提交 |
| `GET /epp-assignments`、`PUT /epp-assignments/{cluster}` | 响应删除 `standby` key | OpenAPI 调用方（如前端"备实例"列）需适配 key 缺失 |
| `epp_data` / server_data_conf | **不变** | BFE/EPP 零影响 |

## 5. 集成测试同步（已完成）

以下集成测试原验证 `[主,备]` 导出行为、夹具需经 OpenAPI 构造双实例组，已随本次变更同步改写（2026-10-14）：

- `test/integration/tests/innerapi/epp_data/epp_data_test.go`（IN-EPP-002/003/005）：夹具收敛为单实例组；`EPPAddr` 断言改单元素；OpenAPI 视图断言改为无 `standby` key；`epp_data` 导出保留 `standby: null` 断言（InnerAPI 契约未变）。
- `test/integration/tests/schema/innerapi/innerapi_schema_test.go`（`testServerDataConfEppSchema`）：同上，`EPPAddr` 期望改单元素。

## 6. 发布与回滚

- 前端适配（`standby` 列移除）与 api 发版同步安排；EPP/BFE 无需发版。
- 回滚：重新部署旧镜像（无 schema 变更）。
- 内部 standby 彻底删除（含 `epp_data`/`EPPAddr` 契约收敛）待存量双实例组收敛后另行立项。
