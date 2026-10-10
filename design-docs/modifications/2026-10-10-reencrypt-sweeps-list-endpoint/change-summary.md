# reencrypt-sweeps 补充 GET 列表端点（任务历史翻查）

## 1. 背景与问题来源

2026-10-05 落库的密钥收敛任务接口（`security.md`）只定义了两个端点：POST 触发、
GET 按 `task_id` 查询。任务记录持久化于 `keyrotate_sweep_tasks` 且**历史保留无 TTL**，
但 `task_id` 只在触发响应中出现一次——之后无任何手段翻查历史：

- 运维/审计无法回答"上次收敛/回滚演练何时触发、谁触发、结果计数多少"；
- 任务 ID 未留存即丢失（如只看操作日志的结论行），后续无法按 ID 查询；
- 多实例部署下跨实例排查收敛历史同样无入口。

今日（2026-10-10）复核 `security.md` 确认缺少 **GET 列表端点**。现状代码亦如此：
`endpoints/openapi_v1/security/endpoints.go:58` 的 `Routes` 仅注册
`TriggerRoute`（POST）与 `GetRoute`（GET by task_id）。

## 2. 目标

| 项目 | 说明 |
|------|------|
| 变更日期 | 2026-10-10 |
| 涉及仓库 | `ai-gateway-api`（本仓） |
| 变更类型 | 新增 1 个 OpenAPI 端点（GET 列表）；任务对象响应新增 1 个字段（`created_by`）；`security.md` 三处描述对齐实现 |
| 产出 | `security.md` 契约更新 + endpoints/model/storage 列表查询实现 + 单测 |
| 兼容性 | 无既有端点结构变更（POST 响应不变；GET by task_id 仅新增 `created_by` 字段）；无 DDL 变更；`00-common.md` 错误码表无需新增 |

## 3. 关键决策

| # | 决策 | 结论 | 理由 |
|---|------|------|------|
| 1 | 分页方案 | **`page`/`page_size`/`sort_by`/`sort_order`**，遵循 `00-common.md` 全局列表约定 | 收敛任务集群级单例互斥、低频触发，历史量级小（百条级），offset 分页足够；与 `/operation-logs` 列表一致。批量任务（`/batches`）的主键游标分页针对高频写入场景，此处无此诉求 |
| 2 | 过滤维度 | `status` / `mode` / `scope` / `dry_run` 精确匹配 + `start_time`/`end_time` 按 `started_at` 闭区间过滤 | 全部对应 `keyrotate_sweep_tasks` 既有列；不加新索引（低频管理端接口，数据量小） |
| 3 | 默认排序 | `started_at` **倒序**（新→旧）；`sort_by` 有效字段仅 `started_at`、`task_id` | 运维翻查"最近一次收敛"是最高频场景；有效字段收紧，避免依赖任意列排序的实现口子 |
| 4 | `created_by` 对外暴露 | 任务对象（列表与详情）新增 `created_by`（触发者名称） | 库列（`created_by`）与模型字段（`keyrotate.SweepTask.CreatedBy`）已存在，仅未对外暴露；列表排障追溯"谁触发"必需。POST 触发响应**不回填**（保持触发响应结构不变，与 2026-10-05 契约一致） |
| 5 | 运行中任务的列表/详情取值 | **以库内已落库值为准**，文档如实描述，不改实现 | 现状：运行中仅更新 `scanned`/`rewritten` 总计与心跳（`storage/rdb/keyrotate/keyrotate.go:144-149`）；`summary`、`duration_ms` 仅在完成时落库（同文件 `:152-167`）；任务对象不暴露总计列。实时性靠轮询 GET 观察 `status` 翻转与完成后的 `summary` |
| 6 | 文档对齐（顺带修正，不改实现） | `security.md` 三处描述与实现对齐，证据见 §5 | 新增列表端点必须写清"运行中任务返回什么"，现有表述与之直接矛盾，不修正会留下自相矛盾的契约 |
| 7 | 权限与审计 | GET 列表用 FeatureSecurity `read`（与 GET by task_id 相同）；**列表查询不写操作日志** | 与 `/operation-logs` 列表等只读查询一致；审计留存由触发/完成时的操作日志承担 |

## 4. 范围

| 范围 | 说明 |
|------|------|
| 主要新增 | `endpoints/openapi_v1/security/`：`ListRoute`（GET 集合路由）+ list 响应结构；`model/keyrotate/manager.go`：`ListTasks`；`storage/rdb/keyrotate/keyrotate.go`：条件分页查询 |
| 主要修改 | `design-docs/api-define/OpenAPI接口定义/security.md`：新增列表端点章节、§1 模型加 `created_by`、三处描述对齐；`endpoints/openapi_v1/security/endpoints.go`：任务响应补 `created_by`（列表与 by-id 共用） |
| 明确不动 | 任务表 DDL（无新列/新索引）；POST 触发响应结构；触发/完成的操作日志行为；`scanned`/`rewritten` 总计列不进入任务对象响应（见决策 5）；批量任务游标分页不迁移 |
| 数据迁移 | 无 |

## 5. 文档对齐的三处不一致（决策 6 证据）

`security.md` 现有表述与实现不符，本次顺带修正：

| # | `security.md` 现有表述 | 实现行为（证据） |
|---|------------------------|------------------|
| 1 | §1：`duration_ms` "running 时为当前已耗时" | `duration_ms` 仅在 `FinishTask` 写入（`storage/rdb/keyrotate/keyrotate.go:166`），running 时为 0 |
| 2 | §3（GET by task_id）："含实时 `summary` 与 `duration_ms`" | `summary` 仅在完成时序列化落库（同文件 `:154-167`）；运行中仅总计计数与心跳更新（`:144-149`），且总计列不对外——running 时 `summary` 为空对象 |
| 3 | §1：`error` "仅 `failed` 时非空" | dry-run 成功任务 `error="dry_run"`（`model/keyrotate/manager.go:149-151`，写入 `error` 列），`status` 仍为 `succeeded` |

修正方向：文档如实改为"以库内已落库值为准"语义（`summary` 完成时按表分组写入；
`duration_ms` 完成时写入最终值，running 为 0；dry-run 任务 `error` 固定为 `dry_run`）。

**可选后续（不在本次范围）**：若需要运行中实时进度，把运行中已落库的
`scanned`/`rewritten` 总计提升进任务对象响应（或完成按表分组的运行中 summary 落库），
属实现增强，需单独评估。

## 6. 非本仓登记点（链路协同）

| 位置 | 改动 | 状态 |
|------|------|------|
| 本仓 `test/integration/tests/secret_at_rest/` | SAR-3-001~003 列表端点用例（`sweeps_list_test.go`）+ design.md 场景登记 | 已落地（见 §9） |
| `ai-gateway-web`（Dashboard） | 收敛任务历史列表页面/抽屉（调用 GET 列表） | 待排期 |
| `integration-test` 仓 | `secret_at_rest` 场景补充列表端点用例 | 待落地（本仓已覆盖，外仓按发布节奏同步） |

## 7. 文档配套

- 本文（change-summary）+ `api-changes.md`（GET 列表端点契约：Query 参数/响应/错误码/示例，
  及 `created_by` 字段与三处对齐的契约层面描述）。两份文档自包含。

## 8. 六步法核对

- [x] Step 1：变更目录 `2026-10-10-reencrypt-sweeps-list-endpoint`
- [x] Step 2：本摘要 + api-changes.md
- [x] Step 3：`api-define/OpenAPI接口定义/security.md` 已落地（intro、§1 模型加
      `created_by` 并修正 `duration_ms`/`summary`/`error` 三处表述、新增 §3 列表
      章节、原 GET by task_id 重编号为 §4、示例扩充含列表样例）
- [x] Step 4：sys-design `details/密钥静态加密.md` 端点清单与实现校准记录已更新
- [x] Step 5：代码实现完成（见 §9 实施记录）
- [ ] Step 6：落地后总结沉淀（跨仓登记点：`integration-test` 补充列表用例、
      `ai-gateway-web` 历史列表页面，均待排期）

## 9. 实施记录

实施内容与 §3 决策、api-changes.md 契约一致，已完成并验证（2026-10-10）：

- `model/keyrotate/types.go`：新增 `TaskFilter`（Status/Mode/Scope/DryRun/
  StartTime/EndTime/Page/PageSize/SortBy/SortOrder）；
- `model/keyrotate/manager.go`：`Storager` 接口与 `Manager` 新增 `ListTasks`；
- `storage/rdb/keyrotate/keyrotate.go`：`ListTasks` 条件分页查询（COUNT + 
  LIMIT/OFFSET，排序列白名单 `started_at`/`task_id`、`asc`/`desc`）；抽取
  `taskColumns` 常量与 `scanTask` 辅助函数，`GetTask`/`taskByID` 复用；
- `endpoints/openapi_v1/security/list.go`（新增）：`ListRoute`
  （GET `/security/reencrypt-sweeps`，FeatureSecurity `read`）+ `ListAction`
  （枚举过滤值 422、RFC3339 时间校验、`end_time` 早于 `start_time` 422、
  page≤0 回落 1、page_size 截断 100、非法 sort 值回落默认）；
- `endpoints/openapi_v1/security/endpoints.go`：`taskResponse` 新增 `created_by`，
  `GetAction` 抽取 `taskToResponse` 与列表共用，`Routes` 注册 `ListRoute`
  （`endpoints/openapi_v1/endpoints.go` 整体引用 `security.Routes`，自动生效）；
- 单测：`model/keyrotate/manager_test.go` 增 `TestListTasksDelegates`；
  `endpoints/openapi_v1/security/list_test.go`（新增）覆盖全参数映射、默认分页
  排序、归一化与 sort 回落、六类非法参数 422。
- 集成测试：`test/integration/tests/secret_at_rest/sweeps_list_test.go`（新增）
  SAR-3-001（列表过滤与字段：total/`created_by` 键/dry-run `error="dry_run"`/
  status·mode·dry_run·时间区间过滤）、SAR-3-002（分页翻页并集、超界空页、
  page_size 截断 100、task_id 升降序、非法 sort 回落默认）、SAR-3-003（六类
  非法参数 422 + 合法无匹配返回 200 空列表）；`design.md` 场景登记同步
  （计数 6→9）。
- Schema 测试：`test/integration/tests/schema/openapi/security.go`（新增
  SweepTriggerSchema/SweepTaskSchema/Summary/TableCount schema）+
  `openapi_schema_test.go` 注册 `security_reencrypt_sweeps` 子测试：三端点
  报文形状（触发响应 6 键、任务对象 12 键合同锁、列表+分页、带参 GET）、
  无 keyring 下 dry-run 空收敛必然 succeeded（transform skip 语义）。

验证结果：

- `go build ./...`、`go vet`（改动三包）通过；
- 单测：`go test ./model/keyrotate/... ./storage/rdb/keyrotate/... ./endpoints/openapi_v1/security/...` 全部 PASS；
- 回归：`go test ./endpoints/... ./storage/... ./lib/...` 全量 PASS；
- model 覆盖率门禁：84.8% ≥ 70%，通过（环境无 make，按 Makefile target 等价命令执行）；
- 集成测试：重建 `ai-gateway-api.exe` 后
  `go test -v -count=1 -timeout 900s ./tests/secret_at_rest/...`（test/integration）
  9/9 PASS（SAR-1/2 既有 6 例回归 + SAR-3 新增 3 例）；
- Schema 测试：`go test -v -count=1 -timeout 900s ./tests/schema/...` 全量 PASS
  （openapi 新增 security_reencrypt_sweeps 子测试 + innerapi/report 回归）；
- 文档核对：`security.md` 示例与实现字段一致（`created_by`/`summary`/`pagination`）。
