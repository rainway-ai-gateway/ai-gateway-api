# Issue #316 变更摘要：被 API Key 挂载的 Entity 删除返回 422，应为 409

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#316](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/316)（SC1203-TC014：被 API Key 挂载的 Entity 删除返回 422，产品设计要求 409）

## 1. 背景

### 1.1 问题现象

`DELETE /open-api/v1/entities/{id}` 在目标 Entity 仍被 API Key 挂载时返回 **HTTP 422 / ErrNum=422**，而 `entities.md §2.6 删除Entity` 明确要求：

- 若该 Entity 存在子 Entity，返回 ErrNum=409；
- 若该 Entity 已被任何 API-Key 挂载，返回 ErrNum=409。

同一接口的两个引用保护分支返回码不一致：子 Entity 分支返回 409，API Key 挂载分支返回 422。调用方无法按 409 语义统一处理引用冲突，也无法区分「被 API Key 引用」与真正的参数非法。QA 两次运行（`20261106-195610.851466251-sc1203tc014` 与 `20261109-092737.441504175-sc1203tc014`）失败指纹一致，确定性可复现。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `endpoints/openapi_v1/entity/delete.go:71` | API Key 列表非空时调用 `xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("cannot delete entity with associated api-keys"))` → 参数错误类型 | ✅ 实锤（直读） |
| 2 | `endpoints/openapi_v1/entity/delete.go:61` | 子 Entity 分支调用 `xerror.WrapConflictErrorWithMsg("entity has children, cannot delete")` → 冲突错误类型 | ✅ 实锤（直读） |
| 3 | `lib/xerror/resolve.go:64-65,85-87` | `etParam` → ErrNo=422；`etConflict` → ErrNo=409 | ✅ 实锤（直读） |
| 4 | `lib/xerror/wrap.go:52-66,163-166` | `WrapParamErrorWithMsg` 以 `etParam` 包装；`WrapConflictErrorWithMsg` 以 `etConflict` 包装，两者只差错误类型常量 | ✅ 实锤（直读） |

**排除项**：
- 「所有删除都返回 422」——同一 run 中 root/middle（子 Entity 保护）删除返回 409，证明仅 API Key 挂载分支命中 422；
- 「Entity 实际未被挂载」——leaf 确实仍被 API Key 挂载，422 触发条件与 handler 分支一致；
- 「消息文案导致的问题」——错误消息本身与 i18n 映射（`conf/i18n/zh.toml:88`，"cannot delete entity with associated api-keys" = "无法删除已有API密钥关联的组织"）均正确，问题只在错误类型 → 错误码映射这一层。

### 1.3 契约依据与裁定需求

- `design-docs/api-define/OpenAPI接口定义/entities.md:516-517`（normative）：子 Entity / API-Key 挂载两种引用保护**均返回 ErrNum=409** → 无需产品裁定，条款已明确，只需修复实现并回归。
- `workflows.md:142` 工作流表同样以「挂载 API-Key 时禁止删除」描述该约束。

## 2. 修复方案

### 2.1 代码改动（仅 1 处，单行级别）

`endpoints/openapi_v1/entity/delete.go:70-72`：

```go
// 修复前
if len(apiKeys) > 0 {
    return nil, xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("cannot delete entity with associated api-keys"))
}

// 修复后
if len(apiKeys) > 0 {
    return nil, xerror.WrapConflictErrorWithMsg("cannot delete entity with associated api-keys")
}
```

要点：

1. **错误类型** `WrapParamErrorWithMsg` → `WrapConflictErrorWithMsg`，与子 Entity 分支（`delete.go:61`）对齐，经 `resolve.go` 映射后 ErrNum=422 → **409**。
2. **错误消息字符串保持逐字不变**（`"cannot delete entity with associated api-keys"`），因此：
   - i18n 翻译（`conf/i18n/zh.toml:88`、集成测试副本 `test/integration/conf/i18n/zh.toml:83`）无需任何改动；
   - 前端与调用方看到的 ErrMsg 文案不变，只有 ErrNum/HTTP 状态码变化。
3. **顺带清理冗余写法**：`WrapParamErrorWithMsg("%s", fmt.Sprintf(...))` 是多余的二次格式化；`WrapConflictErrorWithMsg` 直接接受格式串与参数。改动后若 `fmt` 包不再被引用，同步删除 `fmt` import。
4. **删除顺序与「不产生部分删除」语义不变**：两个引用保护检查（子 Entity → API Key）都在 `DeleteEntity` 之前返回，任何 409 分支都不会触发级联清理，满足验收判据 3 的「删除失败不得产生部分清理」。

### 2.2 不改动的部分

| 项 | 说明 |
|----|------|
| 子 Entity 分支 | 已是 `WrapConflictErrorWithMsg` → 409，不动（回归守护点） |
| Entity 不存在分支 | `WrapRecordNotExist` → 404，不动 |
| `DeleteEntity` 级联清理逻辑 | 在 `model/entity/`，未被触碰 |
| `lib/xerror` 错误类型体系 | 映射关系（param=422 / conflict=409）本就正确，不动 |
| `entities.md` 等 API 定义文档 | 条款本来就写的是 409，是代码违背文档，文档无需修订 |
| 前端 `ai-gateway-web` | grep 确认未对 Entity 删除硬编码 409/422 分支（409/422 仅出现在 BatchTasks、Security 模块），错误提示走通用 ErrMsg 展示，零改动 |
| i18n 词条 | 消息串不变，zh.toml / en 映射均不动 |

### 2.3 修复后行为对照

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 删除被 API Key 挂载的 Entity | HTTP 422 / ErrNum=422 | **HTTP 409 / ErrNum=409**，消息文案不变 |
| 删除有子 Entity 的 Entity | HTTP 409 | 不变 |
| 删除无引用 Entity | 成功，Data=null | 不变 |
| 删除不存在的 Entity | HTTP 404 | 不变 |
| 409 时是否产生部分删除 | 否 | 否（不变，两条保护分支均在删除前返回） |

## 3. 测试计划

### 3.1 单元测试（新增 `endpoints/openapi_v1/entity/delete_test.go`）

该目录目前仅有 `validator_test.go`，删除 handler 无测试覆盖。按本仓既有 handler 测试模式（参考 `endpoints/openapi_v1/k8s_pools/endpoints_test.go` 的 `fakeStorager` + `setupManager` 注入 `container.*Manager` 全局变量的写法）：

| 用例 | 前置 | 断言 |
|------|------|------|
| `TestEntityDeleteAction_MountedByAPIKey` | fake Entity storager 返回目标 Entity、无子 Entity；fake API Key storager 按 `EntityID` 过滤返回 1 条 Key | 错误经 `xerror.Resolve` 后 **ErrNo=409**（直接断言 `resolve.go` 映射结果，钉死回归方向）；`DeleteEntity` 未被调用（无部分删除） |
| `TestEntityDeleteAction_HasChildren` | fake 返回 2 条子 Entity | ErrNo=409（回归守护，与上同一路径） |
| `TestEntityDeleteAction_NotFound` | fake 返回 nil | 404 语义（`WrapRecordNotExist`） |
| `TestEntityDeleteAction_Success` | 无子 Entity、无 API Key | `DeleteEntity` 被调用一次，返回 nil |

构造 manager 所需签名：`entity.NewEntityManager(txn, storager, ...)`（`model/entity/entity_manager.go:44`）、`api_key.NewAPIKeyManager(txn, storager, ...)`（`model/api_key/api_key.go:142`），txn 传 nil 即可（删除前置检查路径不触达事务）。fake storager 实现 `EntityStorager` / `APIKeyStorager` 接口的 `Fetch`/`FetchList`/`Delete` 回调方法。

### 3.2 集成测试（修改 `test/integration/tests/entity/delete/delete_test.go`）

**结论：需要在 `ai-gateway-api/test` 中补充测试，且是本次修复的必要组成部分**（不是可选项）。理由：

- `test/integration/tests/entity/delete/delete_test.go` 已有 E-6-003「删除被 API-Key 挂载的 Entity」，但断言宽容（`ErrNum != 409 && ErrNum != 422`，`:102`），修复前后都通过，是 bug 漏网的直接原因；只改源码不改此断言，修复将没有集成层面的回归守护。
- 该套件走真实 HTTP server（`testutil.StartServer()`），经 `xreq.Convert` → `xerror.Resolve` 全链路验证 ErrNum，恰好覆盖单元测试 mock 不到的错误码映射层。

改动内容（保持既有用例编号与 `t.Cleanup` 清理模式不变）：

| 用例 | 位置 | 改动 |
|------|------|------|
| E-6-002 删除存在子节点的 Entity | `delete_test.go:79` | 断言由「409 或 422 均可」收紧为 **仅 `ErrNum=409`**（`testutil.AssertErrCode(t, resp, 409)`），与子 Entity 分支的既有正确行为对齐，作为回归守护点 |
| E-6-003 删除被 API-Key 挂载的 Entity | `delete_test.go:102` | 同上收紧为 **仅 `ErrNum=409`**；并在 409 之后追加 **GET `/entities/{id}` 仍返回 200** 的断言（验收判据 1：引用保护拒绝删除且资源未被部分删除） |
| E-6-001 / E-6-004 | 不动 | 成功路径（Data=null、404 回读、ID 不复用）与本 bug 无关 |

执行方式：该目录 `TestMain` 经 `testutil.StartServer()` 拉起服务（SQLite 默认，MySQL 由 CI/环境变量切换），跑 `go test ./test/integration/tests/entity/delete/...`。

### 3.3 E2E / 验收判据

对照 issue 三条验收判据，requeue SC1203-TC014：

1. 删除仍被 API Key 挂载的 Entity 返回 HTTP 409 / ErrNum=409，且 Entity 仍可 GET；
2. 解绑并删除所有 API Key 后，按 leaf→middle→root 顺序删除均成功，Entity-Type 释放后可删除；
3. 子 Entity 引用删除仍返回 409；删除失败时 quota_plan / rate_limit_policy / route_rules 保持可读（无部分清理）。

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

1. 同类问题排查：全仓 grep 确认 `cannot delete entity with associated api-keys` 仅 `delete.go` 一处使用，`WrapParamErrorWithMsg` 的其他调用点均为真正的参数校验场景，无同类「引用保护误用参数错误」残留。
2. **测试放大器（重要）**：`test/integration/tests/entity/delete/delete_test.go` 的 E-6-002（子 Entity，`:79`）与 E-6-003（API-Key 挂载，`:102`）断言均为 `resp.ErrNum != 409 && resp.ErrNum != 422`——**409 与 422 都判通过**，是本 bug 躲过集成测试网的原因。修复必须同步收紧为只认 409（见 §3.2），否则修复后回归仍无守护。
3. 集成测试副本 `test/integration/conf/i18n/zh.toml:83` 的词条随消息串不变而自然保持有效，无需同步修改。
4. QA requeue SC1203-TC014 转 PASS 后关闭 issue #316。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 根因源码直读核实（`delete.go` 双分支 + `wrap.go` + `resolve.go` 映射链）
- [x] 影响面排查（i18n / 前端 / xerror 其他调用点 / 级联删除语义）
- [x] 代码实现（`delete.go` 单行修改 + 冗余 fmt 清理）
- [x] 单元测试（新增 `endpoints/openapi_v1/entity/delete_test.go` 四用例）
- [x] 集成测试（收紧 `test/integration/tests/entity/delete/delete_test.go` E-6-002 / E-6-003 断言为仅 409，E-6-003 补「409 后 Entity 仍可 GET」断言）
- [x] 验证：`go test ./endpoints/openapi_v1/entity/...` 通过；集成测试 `go test ./tests/entity/delete/...`（重建 `ai-gateway-api.exe` 后）通过；全量 `go test ./...` 零失败
- [x] Step 3/4（api-define / sys-design）核对：条款本就承诺 409，文档无需修订；无新增可沉淀机制，不新建 `sys-design/details/` 文档（xerror 类型→错误码映射已有文档覆盖）
- [ ] QA requeue SC1203-TC014 转 PASS 并关闭 issue #316

## 7. 实施校准记录（实现过程中确认的事实与取舍）

1. **单元测试守护方向验证**：`TestEntityDeleteAction_MountedByAPIKey` 在修复前的 `delete.go` 上实测失败（预期 409 实际 422），修复后通过——证明用例确实钉住了 422→409 的回归方向，而非空转。
2. **集成测试的「旧二进制」插曲**：`test/integration` 是独立 module，走真实子进程（`testutil` 复用项目根目录的 `ai-gateway-api.exe`）。首次跑 tightened 断言时拿到的是修复前的旧 exe，E-6-003 如期报 `expected ErrNum=409, got ErrNum=422`——这恰好构成一次端到端的 bug 复现，确认 tightened 断言能拦住本 bug；`go build -o ai-gateway-api.exe .` 重建后全绿。后续跑集成测试前需保证根目录二进制为最新构建。
3. **单测 fake 的 nil 安全性确认**：`EntityManager.FetchEntity/FetchEntityList/DeleteEntity` 与 `APIKeyManager.FetchAPIKeyList` 均经 `txn.AtomExecute`（fake 直接执行回调）；`quotaCache` 与各类关联 storager 传 nil 时所有 populate / cleanup / audit / operation-log 路径均有 nil 保护，fake 只需实现两个 storager 接口即可，无需事务与 Redis。
4. **集成测试改动最小化**：E-6-002 / E-6-003 仅收紧断言并补一条 GET 回读，未改动用例编号、`t.Cleanup` 清理顺序与夹具构造，与既有 `TestEntity_Delete` 结构保持一致。
