# Issue #320 变更摘要：Entity-Type GET/DELETE URI 的 type_name 未接入 EntityTypeName 校验（非法名应 422）

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#320](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/320)（SC1203-TC020：`GET /entity-types/-a` 返回 404，契约要求 422）

## 1. 背景

### 1.1 问题现象

`GET /open-api/v1/entity-types/{type_name}` 传入非法名称 `-a`（EntityTypeName 条款禁止以 `-`/`_` 开头或结尾）时，未在 URI 入口做名称合法性校验，直接落到记录查询，无匹配后返回 HTTP 404 / ErrNum=404。由此：

- 非法名称与「合法但不存在的名称」不可区分（均为 404），错误处理、重试与排障语义失真；
- 名称条款在创建/更新入口被强制（`create.go:44`、`update.go:44` 均调用 `validate.EntityTypeName`），在详情/删除入口被绕过，同一参数在不同方法下错误码不一致；
- 同一失败最早记录于 2026-10-14（run `20261014-153757`），非本轮新回归。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `endpoints/openapi_v1/entity_type/one.go:34-36` | `OneReq.TypeName` 仅 `uri:"type_name" validate:"required"`，**无 `Validate()` 方法**（全包 grep 确认 entity_type 包无任何 `Validate()`） | ✅ 实锤（直读） |
| 2 | `endpoints/openapi_v1/entity_type/one.go:44-51` | 非法名通过绑定后进入 `FetchEntityType`，nil → `WrapRecordNotExist` → 404 | ✅ 实锤（直读） |
| 3 | `endpoints/openapi_v1/entity_type/delete.go:34-36` | `DeleteReq.TypeName` 同样仅 `required`，无校验——`DELETE /entity-types/-a` 同源风险（issue 预判，源码实锤，运行层待补测） | ✅ 实锤（直读） |
| 4 | `endpoints/openapi_v1/entity_type/create.go:44`、`update.go:44` | 创建/更新入口均显式调用 `validate.EntityTypeName`——校验器存在，详情/删除入口未接入 | ✅ 实锤（直读） |
| 5 | `lib/validate/validate.go:347-360` | `EntityTypeName`：长度 1-32、字符集 `[a-z0-9_-]+`、禁止首尾 `-`/`_`；非法时返回 `WrapParamErrorWithMsg` → 422 | ✅ 实锤（直读） |
| 6 | `lib/xreq/validate.go:27-44` | `xreq.Bind*` 在校验 struct tag 后自动调用参数结构实现的 `Validate()`，并以 `WrapParamError` 包装 → 422——补 `Validate()` 即可在绑定层拦截 | ✅ 实锤（直读） |

**排除项**：

- 「404 来自路由未命中」——`one.go` endpoint 已注册于 `/entity-types/{type_name}`，handler 正常执行，404 来自 `WrapRecordNotExist`；同一入口对合法名 `a` 读取正常；
- 「EntityTypeName 允许 `-a`」——`entityTypeToken = ^[a-z0-9_-]+$` 仅约束字符集，首尾边界由 `EntityTypeName` 单独拒绝（`validate.go:354-358`），`-a`/`a-`/`A`（大写不在字符集）均非法；
- 「修复会影响合法单字符名」——`a` 长度 1、字符集内、非首尾边界，校验通过，行为不变（集成测试 ET-3-001 即为回归守护）。

### 1.3 契约依据与裁定需求

- `design-docs/api-define/OpenAPI接口定义/entity-types.md:114`（§2.3 查询单个，GET）：`type_name` 合法性条件 = 「类型为 EntityTypeName；必须引用已存在的 Entity-Type」；
- `design-docs/api-define/OpenAPI接口定义/entity-types.md:171`（§2.5 删除，DELETE）：同款声明；
- `design-docs/api-define/OpenAPI接口定义/00-common.md:421-427`（§16 EntityTypeName，normative）：长度 1-32；仅小写字母、数字、`_`、`-`；不能以 `-`、`_` 开头或结尾；
- 接口契约约定：非法名称 URI 请求 → 422；仅格式合法但不存在的名称 → 404。
- **无需产品裁定**：条款已明确，属纯实现缺口（文档早已声明参数类型为 EntityTypeName，代码未接入校验）。

## 2. 修复方案

### 2.1 代码改动（2 处，均为参数结构补 `Validate()`，对齐 #311 既有模式）

**`endpoints/openapi_v1/entity_type/one.go`**（在 `OneReq` 定义后追加）：

```go
// Validate performs centralized business validation on the URI parameter.
func (p *OneReq) Validate() error {
	return validate.EntityTypeName(*p.TypeName)
}
```

**`endpoints/openapi_v1/entity_type/delete.go`**（在 `DeleteReq` 定义后追加）：

```go
// Validate performs centralized business validation on the URI parameter.
func (p *DeleteReq) Validate() error {
	return validate.EntityTypeName(*p.TypeName)
}
```

机制与要点：

1. `xreq.BindURI` → `validateData` 在 struct tag 校验（`required` 保证指针非空，解引用安全）通过后自动调用 `Validate()`（`lib/xreq/validate.go:38-42`），校验失败经 `WrapParamError` 包装 → **HTTP 422 / ErrNum=422**，与 `update.go:44` 显式调用 `validate.EntityTypeName` 的错误语义完全一致（`EntityTypeName` 内部已是 `WrapParamErrorWithMsg`，二次包装为幂等合并，不会产生双 `PARAM` 前缀）。
2. 模式与 #311（cluster `OneParam.Validate` → `validate.ClusterName`）、#308（auth `UserNameParam.Validate`）逐字同构，不引入新机制。
3. 校验在绑定层完成，先于任何存储查询：非法名请求**不会触达数据库**、不可能产生副作用（满足验收判据 3）。
4. 不改端点注册、不改存储层、无 DDL。

### 2.2 不改动的部分

| 项 | 说明 |
|----|------|
| `update.go` PATCH | 已显式调用 `validate.EntityTypeName`（`:44`），行为不变 |
| `create.go` POST | 已校验（`:44`），行为不变 |
| `list.go` | 无路径参数（查询参数走各自校验），不涉及 |
| `validate.EntityTypeName` 规则本身 | 与 00-common.md §16 逐条对齐，不动 |
| API 定义文档 | `entity-types.md:114/171` 参数行本就声明类型为 EntityTypeName，实现补齐后文档与代码完全对齐——**纯实现缺口，无契约修订** |
| 前端 `ai-gateway-web` | EntityTypeList 详情/删除走列表中已存在的合法 `type_name`，无影响 |
| i18n | 校验错误消息（`type_name cannot start or end with '-'` 等）均为既有词条路径，无新增消息串 |

### 2.3 修复后行为对照

| 请求 | 修复前 | 修复后 |
|------|--------|--------|
| `GET /entity-types/-a`（issue 复现值） | 404 | **422** |
| `GET /entity-types/a-` | 404 | **422** |
| `GET /entity-types/A`（大写不在字符集） | 404 | **422** |
| `DELETE /entity-types/-a` | 404（推断，运行待补测） | **422**，且不删除任何资源 |
| `GET /entity-types/does-not-exist`（合法不存在） | 404 | 404（**不变，对照臂**——区分「非法」与「不存在」的核心断言） |
| `GET /entity-types/a`（合法单字符，已创建） | 200 | 200（不变，回归守护） |
| `GET /entity-types/non_existent_type` | 404 | 404（不变，既有 ET-3-002） |
| PATCH/POST 非法名 | 422 | 422（不变） |
| 33 字符超长名 GET/DELETE | 404 | 422 |

## 3. 测试计划

### 3.1 单元测试（新增 `endpoints/openapi_v1/entity_type/validate_test.go`）

对 `OneReq.Validate()` 与 `DeleteReq.Validate()` 做表驱动测试（风格对齐 #311 的 `TestOneParamValidate` 与 auth 包 `TestUserNameParamValidate`）：

| 用例组 | 输入 | 期望 |
|--------|------|------|
| 合法名 | `"a"`、`"org"`、`"a-b_c1"` | 通过 |
| issue 复现值 | `"-a"`、`"a-"`、`"A"` | 422 语义（`xerror.Resolve` → ErrNo=422） |
| 首尾边界 | `"-a"`、`"_a"`、`"a-"`、`"a_"` | 422 |
| 字符集外 | `"A"`、`"Org"`、`"a.b"`、`"a b"`、`"a+b"` | 422 |
| 长度 | 33 字符（>32） | 422；32 字符 | 通过 |
| 空串 | `""` | 422（`required` tag 已拦，Validate 的 min=1 双重保障） |

同时用 handler 级单测（复用 issue #319 修复时已建的 fake manager 注入模式）断言：`EntityTypeOneAction` / `EntityTypeDeleteAction` 收到 `-a` 时返回 422，且 **storager 未被调用**（证明拦截发生在绑定层，无副作用）。

### 3.2 集成测试（修改既有套件，不新建场景目录）

**`test/integration/tests/entity_type/detail/detail_test.go`**：在 `TestEntityType_Detail` 中追加：

| 用例 | 请求 | 期望 |
|------|------|------|
| ET-3-003 非法名（连字符开头） | `GET /entity-types/-a` | 422 |
| ET-3-004 非法名（连字符结尾） | `GET /entity-types/a-` | 422 |
| ET-3-005 非法名（大写） | `GET /entity-types/A` | 422 |
| 对照臂 | `GET /entity-types/does-not-exist` | **404**（与 ET-3-002 共同守护「合法不存在仍 404」） |
| 副作用守护 | 非法名请求后 `GET /entity-types/{typeName}`（本次创建的合法类型）仍 200 | 无副作用 |

**`test/integration/tests/entity_type/delete/delete_test.go`**：追加：

| 用例 | 请求 | 期望 |
|------|------|------|
| ET-5-003 非法名删除 | `DELETE /entity-types/-a` | 422（此前该断言从未存在，与 #319 的 ET-5-002 放大器问题同款——必须新增，不能只改源码） |

执行：重建 `ai-gateway-api.exe` 后 `go test ./tests/entity_type/...`（SQLite 默认；校验在入口层，断言与后端无关）。

### 3.3 E2E / 验收判据

对照 issue 四条验收判据，requeue SC1203-TC020：

1. `GET /entity-types/-a`、`GET /entity-types/a-`、`GET /entity-types/A` 均返回 422（对应断言 `A-04`，phase `invalid-named-paths`）；
2. 合法但不存在名称（如 `does-not-exist`）仍返回 404；
3. 非法名请求不产生副作用：同 phase 创建的合法 `a` 仍可读取；
4. `DELETE /entity-types/-a` 返回 422 且不删除任何资源（issue 标注为待确认项，本次修复主动覆盖并在集成测试中钉死）。

## 4. 兼容性、迁移与风险

| 项 | 结论 |
|----|------|
| 行为变化 | 非法名请求由 404 收紧为 422——契约早已声明字符集与边界规则，属纠偏；合法请求（含合法不存在名 → 404）零影响 |
| 响应结构 | 无变化；仅非法名请求的 ErrNum/HTTP 状态码 404→422 |
| 数据迁移 | 无 DDL、无数据变更；非法名请求不再触达数据库 |
| 破坏性评估 | 曾对「非法名 → 404」做特殊适配的调用方需留意，但该适配本就是对着 bug 写的；按契约（非法名 422）处理的调用方从「失效」变为「生效」 |
| 回滚 | 单 commit 还原即可 |

## 5. 关联风险与后续事项

1. **#311 巡检结论的勘误**：#311 变更摘要 §5.1 曾记录「entity/entity_type 的 URI 参数结构均已有 Validate() 或自定义校验，未发现同型缺口」——本 issue 证明该结论对 entity_type 不成立（`OneReq`/`DeleteReq` 均无校验）。#311 的巡检以「包内存在任意校验」误判为「URI 参数已校验」，本次修复后建议按「URI 参数结构逐一核对」的标准重新巡检一遍其余按名端点。
2. **entity 包观察项（不在本次范围）**：`endpoints/openapi_v1/entity/one.go:35` 与 `delete.go:35` 的 `EntityID` 同样仅 `required` 无格式校验。但 Entity ID 是系统生成的资源标识（非用户命名空间），「非法格式 → 404」与「合法不存在 → 404」语义上可接受，契约亦未对 ID 路径参数声明命名规则——暂不改动，如需统一另行评估。
3. **跨仓测试设计修订**：integration-test 仓 `docs/test-design/SC1203/SC1203-TC020.md` 的 `A-04` 目前因第一个候选失败后即停止，建议补充 `a-`、`A`、`DELETE -a` 候选的独立断言，避免单点失败掩盖其余偏差。
4. QA requeue SC1203-TC020 转 PASS 后关闭 issue #320。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 根因源码直读核实（`one.go`/`delete.go` 参数结构 + `validate.EntityTypeName` 规则 + `xreq` Validate() 机制）
- [x] 影响面排查（文档参数行 / PATCH·POST 既有校验 / 前端 / i18n / #311 巡检勘误）
- [x] 代码实现（`one.go` + `delete.go` 各补一个 `Validate()`）
- [x] 单元测试（新增 `validate_test.go` 表驱动用例 + `one_test.go` handler 级「storager 未被调用」断言）
- [x] 集成测试（detail 套件补 ET-3-003/004/005 + 对照臂与副作用守护；delete 套件补 ET-5-003）
- [x] 验证：`go test ./endpoints/openapi_v1/entity_type/...` 通过；集成测试（重建 `ai-gateway-api.exe` 后）`go test ./tests/entity_type/...` 通过；全量 `go test ./...` 零失败
- [x] Step 3/4（api-define / sys-design）核对：文档本就声明 EntityTypeName 类型，无需修订；Validate() 机制已有 #308/#311 文档覆盖，不新建 `sys-design/details/` 文档
- [ ] QA requeue SC1203-TC020 转 PASS 并关闭 issue #320
- [ ] 跨仓跟进：integration-test 仓 `SC1203-TC020.md` 补 `a-`/`A`/`DELETE -a` 独立断言

## 7. 实施校准记录（实现过程中确认的事实与取舍）

1. **单测守护方向验证**：`TestEntityTypeOneAction_InvalidName` / `TestEntityTypeDeleteAction_InvalidName` 在修复前代码上实测失败——`expected: 422, actual: 404` 且 `fetchCalls=1`（旧代码落库查询），修复后通过且 `fetchCalls=0`——证明「绑定层拦截、零存储访问」的断言确实钉住了 404→422 的回归方向。
2. **fake 的名字感知修正**：初版 `fakeDeleteEntityTypeStorager.FetchEntityType` 不按名称过滤，旧代码下 GET `-a` 会拿到 fake 数据返回 200（「error expected but got nil」），失败方向不够锐利。修正为按 `filter.TypeName` 匹配后，旧代码失败形态与线上完全一致（404 Record Not Exist），且不影响 #319 既有用例。
3. **集成旧二进制复现**：将 `one.go`/`delete.go` 临时还原并重建 exe 后，ET-3-003 与 ET-5-003 均报 `expected ErrNum=422, got ErrNum=404, ErrMsg=Record Not Exist`——与 issue 运行证据逐字一致，确认 tightened 断言能拦住本 bug；恢复修复重建后 `go test ./tests/entity_type/...` 全部通过。
4. **二次包装安全性确认**：`xreq.validateData` 对 `Validate()` 返回值统一 `WrapParamError` 包装（`lib/xreq/validate.go:40`）；`validate.EntityTypeName` 内部已是 `WrapParamErrorWithMsg`（`unwrapMsg` 命中 `etParam` 时幂等返回，见 `lib/xerror/wrap.go:61-63`），不会形成双 `PARAM` 前缀，实测 ErrMsg 为单层 `Param Illegal: type_name cannot start or end with '-' or '_'...`。
