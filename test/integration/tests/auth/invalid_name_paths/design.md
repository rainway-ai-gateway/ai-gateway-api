# invalid_name_paths：按名引用端点的非法名称校验（issue #308）

## 场景目的

`users` 表默认 `CHARSET=utf8`（collation `utf8_general_ci`，PAD SPACE）：尾空格不敏感且大小写不敏感。按名引用端点若只做 `required,min=1` 绑定，非法名（如尾随空白的 `"u "`）会命中真实对象 `u`，GET 泄露信息、DELETE/PATCH 可误操作（issue #308，SC1102-TC003）。

修复：按名引用端点对路径参数执行 `validate.UserNameRef` / `validate.TokenNameRef` 字符集校验，非法名一律 422，不依赖存储层比较的精确性。

## 契约依据

- `design-docs/api-define/OpenAPI接口定义/00-common.md` §12 用户名（UserName）：长度 1-64；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾。空白不在允许集内。
- `00-common.md` §14 Token 名称（TokenName）：同上字符集，且明确不能包含空白字符。
- `auth.md` 约束节：按名引用端点非法名返回 422（Param Illegal），不命中名称相近的用户/Token。

## 覆盖端点

| 端点 | 方法 | 参数结构 | 状态 |
|------|------|----------|------|
| `/auth/users/{user_name}` | GET / DELETE | `UserNameParam` | 已注册，本场景覆盖 |
| `/auth/tokens/{token_name}` | GET / DELETE | `TokenNameParam` | 已注册，本场景覆盖 |
| `/auth/users/{user_name}/products/{product_name}` | POST / DELETE | `UserNameParam` | **仅定义未注册**（不在 `endpoints/openapi_v1/auth/endpoints.go` 的 `Endpoints` 列表中，请求一律 404）；修复随 `UserNameParam.Validate` 一并覆盖，不发请求 |

`PATCH /auth/users/{user_name}/role`、`PATCH /auth/users/{user_name}/passwd` 在 issue #81 修复中已带校验，不在本场景重复覆盖（既有 update_role / reset_password 场景守护）。

## 用例矩阵

| 用例 | 请求（路径为字面量，未二次转义） | 期望 |
|------|------|------|
| AUTH-INV-001 | `GET /open-api/v1/auth/users/u%20` | 422（MySQL 后端修复前为 200 命中 `u`，即 issue 复现臂） |
| AUTH-INV-002 | `GET .../users/%20u`、`users/u%2B`、`users/u.`、`users/-u` | 422 |
| AUTH-INV-003 | `DELETE .../users/u%20` | 422，且 `GET .../users/u` 仍 200（未误删） |
| AUTH-INV-004 | `GET .../tokens/t%20`、`tokens/%20t`、`tokens/t%2B` | 422 |
| AUTH-INV-005 | `DELETE .../tokens/t%20` | 422，且 `GET .../tokens/t` 仍 200（未误删） |
| AUTH-INV-006 对照臂 | `GET .../users/u`、`GET .../tokens/t`、`GET .../users/admin` | 200；`user_name`/`name` 精确等于请求名 |
| AUTH-INV-007 边界臂 | 创建 64 字符合法名用户后按名 GET/DELETE | 200 / 200（Data null），正常走通 |

## 实现要点

- **前置数据**：`testutil.CreateUser("u", ...)`、`testutil.CreateToken("t", "System")`；`t.Cleanup` 清理。每包独立 server/DB（`TestMain` → `testutil.StartServer()`），与并行包的 `u`/`t` 不冲突。
- **路径转义**：`testutil.Client` 原样拼接路径串（`client.go` 不做二次转义），用例写字面量 `%20`/`%2B`，等价 issue 复现中的 `url.PathEscape("u ")`。
- **产品绑定端点不发请求**：`ProductUserBindEndpoint`/`ProductUserUnbindEndpoint` 未注册进 `auth.Endpoints`，请求一律 404；若未来注册，其 `UserNameParam` 校验已由本修复覆盖。
- **后端差异**：已在本机 MySQL 8.4（3306，`AIAPI_MYSQL_DSN` 注入）与 SQLite 回退双后端跑通。修复前 MySQL 上 AUTH-INV-001 为 200（失败指纹）；SQLite 上修复前即为 404（SQLite `=` 字节精确）。422 断言在两个后端均成立，无需 `//go:build mysql` 标签；根因臂经 MySQL 直读验证：`SELECT 'u ' = 'u'` 返回 1（PAD SPACE）、`'U' = 'u'` 返回 1（大小写不敏感）。
- **保留名对照**（AUTH-INV-007 的 `admin`）：`UserNameRef`/`TokenNameRef` 刻意不含保留名检查，内置 `admin` 必须可按名查询（与 `user_detail` 等既有场景一致）。

## 关联

- 代码变更：`design-docs/modifications/2026-11-08-issue-308-name-path-charset-validation/change-summary.md`
- 单测：`lib/validate`（`TestTokenNameRef`）、`endpoints/openapi_v1/auth`（`TestUserNameParamValidate`、`TestTokenNameParamValidate`）
