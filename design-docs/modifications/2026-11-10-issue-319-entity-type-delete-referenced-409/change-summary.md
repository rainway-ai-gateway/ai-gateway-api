# Issue #319 变更摘要：被 Entity 引用的 Entity-Type 删除返回 422，应为 409

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#319](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/319)（SC1203-TC011：被 Entity 引用的 Entity-Type 删除返回 HTTP 422 / ErrNum=422，产品设计要求 409）

## 1. 背景

### 1.1 问题现象

`DELETE /open-api/v1/entity-types/{type_name}` 在目标 Entity-Type 仍被至少一个 Entity 通过 `type` 字段引用时，返回 **HTTP 422 / ErrNum=422 / ErrMsg="Param Illegal: cannot delete entity type with associated entities"**，而 `entity-types.md §2.5 删除Entity-Type` 明确要求：

- **约束**：若该Entity-Type已被任何Entity引用，返回ErrNum=409。

引用保护本身生效（删除被拒后 Entity-Type 与 Entity 均保留，无悬空引用），但错误码语义与产品条款不符：依赖 409 区分「被引用不可删」与「请求参数非法」的调用方会把真实的引用冲突当成输入错误处理。该缺陷在 2026-10-21 的 SC1203-TC011 真跑中由断言 `A-06` 首次暴露（此前同 TC 的 `A-04` 因 issue #244 先失败、`reference-protection-and-delete` 阶段从未执行，故 422≠409 偏差长期被掩盖）。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `endpoints/openapi_v1/entity_type/delete.go:54-59` | 按 `type_name` 取 Entity-Type，用 `EntityManager.FetchEntityList({Type: type_name})` 统计引用数；`len(entities) > 0` 时进入引用保护分支——判定条件与设计一致 | ✅ 实锤（直读） |
| 2 | `endpoints/openapi_v1/entity_type/delete.go:61` | 引用保护分支调用 `xerror.WrapParamErrorWithMsg("cannot delete entity type with associated entities")` → 以「参数非法」包装器表达「资源冲突」 | ✅ 实锤（直读） |
| 3 | `lib/xerror/resolve.go:64-65,85-87` | `etParam` → ErrNo=422；`etConflict` → ErrNo=409 | ✅ 实锤（直读） |
| 4 | `lib/xerror/wrap.go:52-66,163-166` | `WrapParamErrorWithMsg` 以 `etParam` 包装；`WrapConflictErrorWithMsg` 以 `etConflict` 包装，两者只差错误类型常量 | ✅ 实锤（直读） |

**排除项**：

- 「错误码体系缺失 409 映射」——同一体系下 `etConflict` 已正确映射 409（`resolve.go:85-87`），缺陷只是该调用点选错了包装器，不是体系问题；
- 「Entity 实际未引用该类型」——issue 证据显示删除被拒后 Entity 的 `type` 仍指向该类型，422 触发条件与 handler 分支一致；
- 「消息文案导致的问题」——错误消息与 i18n 映射（`conf/i18n/zh.toml:87`，"cannot delete entity type with associated entities" = "无法删除已有组织关联的组织类型"）均正确，问题只在错误类型 → 错误码映射这一层；
- 「消息串存在多个调用点」——全仓 grep 确认该消息串仅 `entity_type/delete.go:61` 一处使用。

### 1.3 契约依据与裁定需求

- `design-docs/api-define/OpenAPI接口定义/entity-types.md:177`（normative，approved/effective/released）：引用保护**返回 ErrNum=409** → 无需产品裁定，条款已明确，只需修复实现并回归。
- 同族接口错误码一致性参照：Entity 删除的两个引用保护分支（子 Entity、API-Key 挂载，见 #316 修复）均返回 409；本次修复使 Entity-Type 删除与同族语义对齐。

## 2. 修复方案

### 2.1 代码改动（仅 1 处，单行级别）

`endpoints/openapi_v1/entity_type/delete.go:60-62`：

```go
// 修复前
if len(entities) > 0 {
    return nil, xerror.WrapParamErrorWithMsg("cannot delete entity type with associated entities")
}

// 修复后
if len(entities) > 0 {
    return nil, xerror.WrapConflictErrorWithMsg("cannot delete entity type with associated entities")
}
```

要点：

1. **错误类型** `WrapParamErrorWithMsg` → `WrapConflictErrorWithMsg`，经 `resolve.go` 映射后 ErrNum=422 → **409**，与 `entity-types.md §2.5` 条款及同族 Entity 删除（#316）语义对齐。
2. **错误消息字符串保持逐字不变**（`"cannot delete entity type with associated entities"`），因此：
   - i18n 翻译（`conf/i18n/zh.toml:87`、集成测试副本 `test/integration/conf/i18n/zh.toml:82`）无需任何改动；
   - 前端与调用方看到的 ErrMsg 文案不变，只有 ErrNum/HTTP 状态码变化。
3. **引用保护检查位置不变**：该检查在 `DeleteEntityType` 之前返回，409 分支不会触发任何删除，满足「删除失败不得产生部分删除 / 悬空引用」的验收语义。

### 2.2 不改动的部分

| 项 | 说明 |
|----|------|
| Entity-Type 不存在分支 | `WrapRecordNotExist` → 404（`delete.go:51`），不动 |
| 引用计数判定条件 | `FetchEntityList({Type: type_name})` + `len(entities) > 0`，与设计一致，不动 |
| `EntityTypeManager.DeleteEntityType` | 在 `model/entity/`，未被触碰 |
| `lib/xerror` 错误类型体系 | 映射关系（param=422 / conflict=409）本就正确，不动 |
| `entity-types.md` 等 API 定义文档 | 条款本来就写的是 409，是代码违背文档，文档无需修订 |
| 前端 `ai-gateway-web` | `EntityTypeList.vue` 删除仅判断 `data.status === 200` 成功分支（`:250`），失败提示走通用 ErrMsg 展示，未对 409/422 做硬编码分支，零改动 |
| i18n 词条 | 消息串不变，zh.toml 两处映射均不动 |

### 2.3 修复后行为对照

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 删除被 Entity 引用的 Entity-Type | HTTP 422 / ErrNum=422 | **HTTP 409 / ErrNum=409**，消息文案不变 |
| 删除被拒后 Entity-Type 是否保留 | 保留（GET 回读 200） | 保留（不变） |
| 删除被拒后 Entity 的 `type` 引用 | 保留 | 保留（不变，无悬空引用） |
| 删除无引用 Entity-Type | 成功，Data=null | 不变 |
| 删除不存在的 Entity-Type | HTTP 404 | 不变 |
| 删除不存在分支的 404 | 404 | 不变 |

## 3. 测试计划

### 3.1 单元测试（新增 `endpoints/openapi_v1/entity_type/delete_test.go`）

该目录目前仅有 `endpoints_test.go`（路由注册测试），删除 handler 无行为测试覆盖。按 #316 修复时已建立的 handler 测试模式（参考 `endpoints/openapi_v1/entity/delete_test.go` 的 fakeStorager + 注入 `container.*Manager` 全局变量的写法）：

| 用例 | 前置 | 断言 |
|------|------|------|
| `TestEntityTypeDeleteAction_ReferencedByEntity` | fake Entity-Type storager 返回目标类型；fake Entity storager 按 `Type` 过滤返回 1 条 Entity | 错误经 `xerror.Resolve` 后 **ErrNo=409**（直接断言映射结果，钉死回归方向）；`DeleteEntityType` 未被调用（无部分删除） |
| `TestEntityTypeDeleteAction_NotFound` | fake 返回 nil | 404 语义（`WrapRecordNotExist`） |
| `TestEntityTypeDeleteAction_Success` | Entity 列表为空 | `DeleteEntityType` 被调用一次，返回 nil |

### 3.2 集成测试（修改 `test/integration/tests/entity_type/delete/delete_test.go`）

**必须同步收紧断言，是本次修复的必要组成部分**（与 #316 相同的测试放大器问题）：

- 现有 ET-5-002「删除被 Entity 引用的 Entity-Type」（`:66`）断言宽容：`resp.ErrNum != 409 && resp.ErrNum != 422`——**409 与 422 都判通过**，是本 bug 躲过集成测试网的直接原因。只改源码不改此断言，修复将没有集成层面的回归守护。

改动内容（保持既有用例编号与 `t.Cleanup` 清理模式不变）：

| 用例 | 位置 | 改动 |
|------|------|------|
| ET-5-002 删除被 Entity 引用的 Entity-Type | `delete_test.go:66` | 断言由「409 或 422 均可」收紧为 **仅 `ErrNum=409`**（`testutil.AssertErrCode(t, resp, 409)`）；并在 409 之后追加 **GET `/open-api/v1/entity-types/{type_name}` 仍返回 200** 与 **Entity 回读 `type` 仍指向该类型** 的断言（验收判据：引用保护拒绝删除且资源保留、无悬空引用） |
| ET-5-001 | 不动 | 成功路径（删除后 404 回读）与本 bug 无关 |

执行方式：该目录 `TestMain` 经 `testutil.StartServer()` 拉起真实服务（复用项目根目录的 `ai-gateway-api.exe`，**跑之前需重新构建二进制**），跑 `go test ./test/integration/tests/entity_type/delete/...`。

### 3.3 E2E / 验收判据

对照 issue 验收判据，requeue SC1203-TC011：

1. 对被 Entity 引用的 Entity-Type 发起 DELETE，返回 HTTP 409 / ErrNum=409，且 Entity-Type 与 Entity 均保留、Entity 的 `type` 引用不悬空（对应断言 `A-06`）；
2. 删除 Entity 后，Entity-Type 可正常删除（对应 `cleanup-and-residue`，修复前已通过，保持不回归）；
3. 同批 `A-01..A-05` 全 PASS 不回归。

## 4. 兼容性、迁移与风险

| 项 | 结论 |
|----|------|
| 响应结构 | 无变化；仅 ErrNum 422→409、HTTP 状态码 422→409 |
| ErrMsg 文案 | **逐字不变**（i18n key 不变），前端/调用方提示语无感知 |
| 是否破坏性变更 | 属「恢复文档承诺语义」：文档从来承诺 409，422 是实现偏差。按 409 处理引用冲突的调用方（文档预期写法）从「失效」变为「生效」；曾对 422 做特殊适配的调用方需留意，但该适配本就是对着 bug 写的 |
| 数据面 / BFE / 导出 | 零改动 |
| 数据库 | 无 DDL、无迁移 |
| 回滚 | 单 commit 还原即可 |

## 5. 关联风险与后续事项

1. 同类问题排查：全仓 grep `WrapParamErrorWithMsg\("cannot delete` 仅此一处；`WrapParamErrorWithMsg` 的其他调用点均为真正的参数校验场景，无同类「引用保护误用参数错误」残留（#316 已修复 Entity 删除 API-Key 挂载分支，本修复收口 Entity-Type 删除分支）。
2. **测试放大器（重要）**：`test/integration/tests/entity_type/delete/delete_test.go:66` 的 ET-5-002 断言为 `resp.ErrNum != 409 && resp.ErrNum != 422`——**409 与 422 都判通过**，是本 bug 躲过集成测试网的原因。修复必须同步收紧为只认 409（见 §3.2），否则修复后回归仍无守护。
3. 集成测试副本 `test/integration/conf/i18n/zh.toml:82` 的词条随消息串不变而自然保持有效，无需同步修改。
4. QA requeue SC1203-TC011 转 PASS 后关闭 issue #319。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 根因源码直读核实（`delete.go` 引用保护分支 + `wrap.go` + `resolve.go` 映射链）
- [x] 影响面排查（i18n 两处 / 前端 EntityTypeList.vue / xerror 其他调用点 / 消息串全局唯一性）
- [x] 代码实现（`endpoints/openapi_v1/entity_type/delete.go:61` 单行修改）
- [x] 单元测试（新增 `endpoints/openapi_v1/entity_type/delete_test.go` 三用例）
- [x] 集成测试（收紧 `test/integration/tests/entity_type/delete/delete_test.go` ET-5-002 断言为仅 409，补 Entity-Type / Entity 回读及 `type` 引用断言）
- [x] 验证：`go test ./endpoints/openapi_v1/entity_type/...` 通过；集成测试（重建 `ai-gateway-api.exe` 后）`go test ./tests/entity_type/...` 全绿；全量 `go test ./...` 零失败
- [x] Step 3/4（api-define / sys-design）核对：条款本就承诺 409，文档无需修订；xerror 类型→错误码映射已有文档覆盖，不新建 `sys-design/details/` 文档
- [ ] QA requeue SC1203-TC011 转 PASS 并关闭 issue #319

## 7. 实施校准记录（实现过程中确认的事实与取舍）

1. **单元测试守护方向验证**：`TestEntityTypeDeleteAction_ReferencedByEntity` 在修复前的 `delete.go` 上实测失败（预期 409 实际 422），修复后通过——证明用例确实钉住了 422→409 的回归方向，而非空转。
2. **集成测试的旧二进制复现**：将 `delete.go` 临时还原为修复前版本并重建 `ai-gateway-api.exe` 后，收紧后的 ET-5-002 如期报 `expected ErrNum=409, got ErrNum=422`——构成一次端到端 bug 复现，确认 tightened 断言能拦住本 bug；恢复修复并重建后 `go test ./tests/entity_type/...` 全部通过。后续跑集成测试前需保证根目录二进制为最新构建。
3. **单测 fake 的 nil 安全性确认**：`EntityManager.FetchEntityList` 内部 `populateAssociatedData` 对 quotaPlan / rateLimitPolicy / routeRules storager 均有 nil 保护，`recordEntityTypeOperation` 对 nil operationLogManager 直接返回；fake 只需实现 `EntityTypeStorager` / `EntityStorager` 两个接口，txn 用直通 fake 即可，无需事务与 Redis。
