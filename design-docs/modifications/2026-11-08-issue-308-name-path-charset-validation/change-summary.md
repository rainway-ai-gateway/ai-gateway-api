# Issue #308 变更摘要：按名端点补齐名称字符集校验（非法名 422）

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#308](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/308)（SC1102-TC003：按名查询用户时尾随空白被接受并命中真实用户）

## 1. 背景

### 1.1 问题现象

已存在合法单字符用户 `u` 时：

```
GET /open-api/v1/auth/users/u%20        # user_name = "u "（尾随空格）
```

返回 HTTP 200 并返回用户 `u`，而不是按字符集条款拒绝 4xx。同族的 Token 按名端点与 DELETE 写路径存在同型缺口：非法名称可经 MySQL `PAD SPACE` 排序规则的尾空格不敏感比较，解析到真实用户/Token，DELETE/PATCH 可能作用于非精确目标。

违反的已发布契约（`design-docs/api-define/OpenAPI接口定义/00-common.md`）：

- §12 用户名（UserName）：长度 1-64；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾。空白不在允许集内，`"u "` 为非法名。
- §14 Token 名称（TokenName）：同上字符集，并明确「不能包含空白字符」。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

1. **入口缺校验**：`UserNameParam`（`endpoints/openapi_v1/auth/user_delete.go:27-29`）与 `TokenNameParam`（`endpoints/openapi_v1/auth/token_destroy.go:28-30`）只声明 `validate:"required,min=1"`，未调用 `validate.UserNameRef`（`lib/validate/validate.go:236`），也不存在 `TokenNameRef`。
2. **存储层比较不精确**：`users` 表 `name` 列无显式 collation，表级 `DEFAULT CHARSET=utf8`（`db_ddl.sql:271-284`），即 utf8mb3 + `utf8_general_ci`——**PAD SPACE（尾空格不敏感）且大小写不敏感**。用户与 Token 同表（`type=0` 用户 / `type=1` Token，`model/iauth/authentication.go:37-38`），唯一键 `name_uni(name, type)` 同样基于该 collation。等值查询 `WHERE name = 'u '` 命中 `'u'`。
3. **同族已修复端点**：`PATCH /auth/users/{user_name}/role` 与 `PATCH /auth/users/{user_name}/passwd` 在 issue #81 修复中已为各自参数结构实现 `Validate()` 并调用 `validate.UserNameRef`（`user_update_role.go:44-52`、`user_update_password.go:46`）。本次问题端点是同模式的漏网者。

### 1.3 受影响端点全量清单

| 端点 | 方法 | 参数结构 | 现状 |
|------|------|----------|------|
| `/auth/users/{user_name}` | GET | `UserNameParam` | **缺字符集校验** |
| `/auth/users/{user_name}` | DELETE | `UserNameParam` | **缺字符集校验**（写路径，可误删真实用户） |
| `/auth/users/{user_name}/products/{product_name}` | POST | `UserNameParam` | **仅定义未注册**（不在 `auth.Endpoints`，请求 404）；修复随 `UserNameParam` 一并覆盖 |
| `/auth/users/{user_name}/products/{product_name}` | DELETE | `UserNameParam` | **仅定义未注册**（同上） |
| `/auth/users/{user_name}/role` | PATCH | `UserUpdateRoleParam` | 已有 `UserNameRef`（#81 修复） |
| `/auth/users/{user_name}/passwd` | PATCH | `UserUpdatePasswordParam` | 已有 `UserNameRef`（#81 修复） |
| `/auth/tokens/{token_name}` | GET | `TokenNameParam` | **缺字符集校验** |
| `/auth/tokens/{token_name}` | DELETE | `TokenNameParam` | **缺字符集校验**（写路径，可误删真实 Token） |

> 注：`UserNameParam` 被 4 个端点复用，在参数结构上补一次 `Validate()` 即可全部覆盖；`TokenNameParam` 被 2 个端点复用。

### 1.4 非受影响面（已排查）

- `POST /auth/session-keys`（密码登录）：JSON 体的 `user_name` 走密码认证而非按名目标选择，尾随空格会因密码不匹配直接失败，不构成目标错绑；不属于本问题。
- `POST /auth/users`、`POST /auth/tokens`（创建路径）：已分别调用 `validate.UserName` / `validate.TokenName`（`user_create.go`、`token_create.go:42`），校验完备。

## 2. 修复方案

### 2.1 方案选择：入口校验（issue 中的方向 A）

在全部按名绑定点调用字符集校验，非法名一律返回 **HTTP 422**（`xerror.WrapParamError` → `ErrNo=422`，`lib/xerror/resolve.go:65`）。

选择理由（对方向 B「存储层改精确比较」的取舍）：

| 维度 | 入口校验（选定） | 存储层改 collation/NO PAD |
|------|------------------|---------------------------|
| 契约判据 | 非法名 422 可直接断言，E2E 可判 PASS | 非法名落到 404，同样是 4xx，但无法区分「格式非法」与「不存在」 |
| 一致性 | 与 PATCH 端点既有做法完全一致；MySQL/SQLite 双端行为统一（SQLite `=` 本为字节精确，两库行为本就不同） | 需 DDL 迁移，影响唯一键语义与所有依赖该 collation 的查询 |
| 契约冲突 | 无 | §12 明文要求用户名「全局唯一（大小写不敏感）」，`utf8_general_ci` 的大小写不敏感恰是契约实现；改二进制比较会破坏该契约 |
| 成本 | 两个文件加 `Validate()`，加一个校验函数 | 表迁移 + 唯一键重定义 + 历史数据核对 + 全回归 |

结论：入口校验足以闭合 issue 的断言（A-01/A-02：非法名必须 4xx，且建立可区分的 422 判据）；存储层精确比较作为后续加固项记录在 §5，不在本次实施。

### 2.2 具体改动

**改动 1：`lib/validate/validate.go` — 新增 `TokenNameRef`**

镜像 `UserName`/`UserNameRef` 的既有拆分模式（`validate.go:219-238`），将 `TokenName` 的格式部分提取为 `tokenNameFormat`，新增引用型校验：

```go
// tokenNameFormat validates the generic token_name format: length 1..64,
// charset [a-zA-Z0-9_.-], no leading/trailing '.', '-', '_', no whitespace
// (covered by the charset). It does not check reserved names.
func tokenNameFormat(s string) error { /* length + nameToken + edges */ }

// TokenNameRef validates a token_name that references an existing token:
// format only, reserved names are allowed — built-in tokens must remain
// operable through these endpoints. Do not add a reserved-name check here.
func TokenNameRef(s string) error { return tokenNameFormat(s) }
```

`TokenName` 改为调用 `tokenNameFormat` + 保留名检查，行为不变。

> 为什么引用型必须允许保留名：内置账号（如 `admin`）与内置 Token 必须能通过按名端点操作——`UserNameRef` 的文档注释（`validate.go:232-235`）已明确这一设计原则，Token 侧对齐。

**改动 2：`endpoints/openapi_v1/auth/user_delete.go` — `UserNameParam` 补 `Validate()`**

```go
// Validate performs centralized business validation on the request parameters.
func (p *UserNameParam) Validate() error {
	return validate.UserNameRef(*p.UserName)
}
```

`xreq.BindURI` → `validateData` 会在 struct tag 校验通过后调用 `Validate()`（`lib/xreq/param.go:126-137`、`lib/xreq/validate.go:33-46`）；`required` 保证指针非空，解引用安全——与 `user_update_role.go` 的既有模式逐字一致。一处改动覆盖 4 个按名端点。

**改动 3：`endpoints/openapi_v1/auth/token_destroy.go` — `TokenNameParam` 补 `Validate()`**

```go
// Validate performs centralized business validation on the request parameters.
func (p *TokenNameParam) Validate() error {
	return validate.TokenNameRef(*p.TokenName)
}
```

一处改动覆盖 Token 的 GET/DELETE 两个按名端点。

### 2.3 修复后行为

| 请求 | 修复前 | 修复后 |
|------|--------|--------|
| `GET /auth/users/u%20` | 200，返回用户 `u`（非法名命中真实用户） | **422** Param Illegal |
| `DELETE /auth/users/u%20` | 可能删除真实用户 `u` | **422** |
| `GET/DELETE /auth/tokens/t%20` | 同上型缺口 | **422** |
| `GET /auth/users/u` | 200 | 200（不受影响） |
| `GET /auth/users/admin` | 200 | 200（保留名引用不受影响） |
| `GET /auth/users/%20u`、`u%2B` | 404 | 422（由 404 收紧为 422，均为 4xx，符合断言） |
| 大小写变体（`GET /auth/users/U`） | 200（collation 大小写不敏感） | 200（**刻意保持**——§12 契约要求大小写不敏感唯一与匹配） |

## 3. 测试计划

### 3.1 单元测试

- `lib/validate/validate_test.go`：新增 `TestTokenNameRef`——合法名通过；尾随/前导空格、含 `+`/空格/中文、超长（65）、以 `.`/`-`/`_` 开头或结尾均报错；保留名 `admin`/`system`/`default` **通过**（引用型语义）。
- `endpoints/openapi_v1/auth/validator_test.go`：新增 `TestUserNameParamValidate`、`TestTokenNameParamValidate`，风格对齐既有 `TestTokenCreateParamValidate`（合法名、`"u "`、`" u"`、非法字符、边界字符）。

### 3.2 集成测试（`test/integration`，本修复的重点防线）

在 `test/integration/tests/auth/` 下新增场景目录 `invalid_name_paths/`（沿用该目录「一场景一目录 + `design.md` + `xxx_test.go`」的惯例，`TestMain` 用 `testutil.StartServer()` 起服务，`testutil.CreateUser/CreateToken` 造数、`t.Cleanup` 清理）：

```
test/integration/tests/auth/invalid_name_paths/
├── design.md                      # 场景设计：契约依据、用例矩阵、后端差异说明
└── invalid_name_paths_test.go     # 用例编号 AUTH-INV-xxx
```

用例矩阵（对 6 个按名端点逐一断言非法名 422、合法名不受影响）：

| 用例 | 端点 | 请求 | 期望 |
|------|------|------|------|
| AUTH-INV-001 | `GET /auth/users/{user_name}` | `users/u%20`（`url.PathEscape("u ")`） | **422**（修复前 MySQL 后端 200 命中 `u`，即 issue 复现臂） |
| AUTH-INV-002 | `GET /auth/users/{user_name}` | `users/%20u`、`users/u%2B`、`users/u.`、`users/-u` | 422 |
| AUTH-INV-003 | `DELETE /auth/users/{user_name}` | `users/u%20` | 422，且用户 `u` 未被删除（事后 `GET users/u` 仍 200） |
| AUTH-INV-004 | `GET /auth/tokens/{token_name}` | `tokens/t%20`、`tokens/%20t`、`tokens/t%2B` | 422 |
| AUTH-INV-005 | `DELETE /auth/tokens/{token_name}` | `tokens/t%20` | 422，且 Token `t` 未被删除（事后 `GET tokens/t` 仍 200） |
| AUTH-INV-006 | 对照臂 | `GET users/u`、`GET tokens/t`、`GET users/admin`（保留名引用） | 200，字段值精确等于请求名 |
| AUTH-INV-007 | 边界臂 | 合法极值名（64 字符、含 `.`/`-`/`_` 但不在首尾）创建后按名 GET/DELETE | 200 / 204，走正常流程 |

> 产品绑定/解绑端点（`POST/DELETE /auth/users/{user_name}/products/{product_name}`）仅定义、未注册进 `auth.Endpoints`（请求一律 404），集成测试不对其发请求；其参数结构与 `UserNameParam` 相同，修复已随 `UserNameParam.Validate` 一并覆盖。该事实为集成测试首轮运行时发现并已校准文档。

实现要点：

- 客户端直接拼接转义后路径（`testutil.Client.Get/Delete` 原样拼接、不二次转义），用例中以字面量写 `"/open-api/v1/auth/users/u%20"`，与 issue 复现的 `url.PathEscape("u ")` 结果一致。
- 断言统一用 `testutil.AssertErrCode(t, resp, 422)` / `AssertSuccess`，与既有 auth 场景同款。
- **后端差异**：已在本机 MySQL 8.4（3306，`AIAPI_MYSQL_DSN` 注入）与 SQLite 回退双后端跑通全部用例。本问题在 MySQL 上修复前表现为「200 命中真实用户」（issue 的失败指纹），在 SQLite 上修复前已是 404（SQLite `=` 字节精确）——422 断言在两个后端均成立、无需 `//go:build mysql` 构建标签。根因臂经 MySQL 直读验证：`SELECT 'u ' = 'u'` 返回 1（PAD SPACE）、`'U' = 'u'` 返回 1（大小写不敏感）。

### 3.3 E2E / 验收判据

- SC1102-TC003 断言 A-01：`GET /open-api/v1/auth/users/u%20` → 422（原判据为 4xx，422 满足并更可断言）。
- 断言 A-02（Token 同族）：`GET /open-api/v1/auth/tokens/{合法名}%20` → 422。
- 回归臂：`GET /auth/users/u` → 200 且 `Data.user_name="u"`；`GET /auth/users/admin` → 200；`PATCH /auth/users/{name}/role` 既有用例不回退。

## 4. 兼容性与发布

| 项 | 结论 |
|----|------|
| 行为变化 | 非法名请求由「200/404 且可能命中真实对象」收紧为 422。这些输入在契约中早已声明非法，属纠偏而非破坏性变更 |
| 数据迁移 | 无 DDL、无数据变更 |
| 凭据安全 | DELETE 写路径不再可能作用于非精确目标，消除 issue 标注的 P1 误操作风险 |
| 双 DB 后端 | MySQL 与 SQLite 行为在修复后一致（此前 SQLite 字节精确、MySQL PAD SPACE，行为分叉） |
| 下游依赖 | 仅管理面 OpenAPI；BFE 数据面、Conf Agent、Web 前端无依赖（Web 本身不传非法名） |

## 5. 关联风险与后续事项（不在本次范围）

1. **TokenName 大小写唯一性契约缺口**：§14 仅声明「全局唯一」，未像 §12 那样注明「大小写不敏感」，但 DB 唯一键 `name_uni` 按 `utf8_general_ci` 判重，实际为大小写不敏感唯一。契约文本与实现不一致，需产品裁定后单独修订契约或实现，建议另开 issue 跟踪。
2. **`users.name` 列长 255 vs 契约 64**：列长度无 DB 层约束，完全依赖入口校验；如需加固可缩短列或加 CHECK，属独立变更。
3. **存储层精确比较（issue 方向 B）**：若未来出现绕过 API 的写库路径，或产品裁定要求字节级精确匹配，需将 `users.name` 改为 `utf8mb4_bin`（或 `NO PAD` collation）并同步处理 §12 的大小写不敏感契约（如查询侧显式 `LOWER(name) = LOWER(?)`），迁移成本见 §2.1 对比表。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 代码实现（`lib/validate`、`endpoints/openapi_v1/auth`）
- [x] 单元测试（`TestTokenNameRef`、`TestUserNameParamValidate`、`TestTokenNameParamValidate`；`endpoints/...`、`lib/...`、`model/iauth/...` 全量通过）
- [x] 集成测试 `test/integration/tests/auth/invalid_name_paths/`（SQLite 与 MySQL 双后端均通过，7 用例；MySQL 后端下另以 `SELECT 'u ' = 'u'` 直读验证 PAD SPACE 根因臂）
- [x] 文档同步：`api-define/auth.md` 约束节、`sys-design/details/认证授权机制.md`、`sys-design/summary.md`
- [ ] E2E SC1102-TC003 转 PASS（QA 环境 `qa-system-test` 复跑）并关闭 issue #308

## 7. 实施校准记录（实现过程中发现并已同步文档的事实）

1. **产品绑定/解绑端点未注册**：`ProductUserBindEndpoint`/`ProductUserUnbindEndpoint` 不在 `endpoints/openapi_v1/auth/endpoints.go` 的 `Endpoints` 列表中，是仅定义未挂载的 dead code（请求一律 404）。修复仍通过在共享参数结构 `UserNameParam` 上补 `Validate()` 一并覆盖；集成测试不对其发请求。
2. **集成测试后端**：本地无可用 MySQL（3306 不可达），`invalid_name_paths` 在 SQLite 回退后端通过；由于校验在入口层，422 行为与后端无关，MySQL 专属的原缺陷复现臂（修复前 200 命中）留待 CI/QA 环境。
