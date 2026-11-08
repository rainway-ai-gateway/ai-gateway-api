# Issue #311 变更摘要：Cluster GET/DELETE 按名端点补齐 ClusterName 校验（非法名 422）

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#311](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/311)（SC2101-TC024：Cluster GET/DELETE 的 `cluster_name` 未按 1-64 校验，错误码与 PATCH 不一致）

## 1. 背景

### 1.1 问题现象

`GET /open-api/v1/clusters/{cluster_name}` 与 `DELETE /open-api/v1/clusters/{cluster_name}`（及 `GET /clusters/{cluster_name}/ready`）的 URI 参数只声明 `required,min=1`，未复用公共类型 `ClusterName`（1-64 字符）校验。超长/非法名称直接落到记录查询，返回 404 `Record Not Exist`；同一参数在 PATCH/POST 上返回 422 `Param Illegal`——同一个 `ClusterName` 类型在不同方法下错误码不一致，调用方无法区分"名称非法"与"记录不存在"。

无写入风险（不存在的超长名称不会被创建或删除），属契约一致性问题。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

1. **缺校验的参数结构**：`OneParam`（`endpoints/openapi_v1/product_cluster/one.go:85-87`）仅 `uri:"cluster_name" validate:"required,min=1"`，无 `Validate()`。已核实。
2. **三端点复用该结构**：`GET /clusters/{cluster_name}`（one.go:196）、`DELETE /clusters/{cluster_name}`（delete.go:36,66）、`GET /clusters/{cluster_name}/ready`（ready.go:37,44）全部经 `OneParam`——**补一处 `Validate()` 即可全覆盖**（issue 建议中提到的 ready.go 已在核实时确认复用同一结构）。已核实。
3. **同包正确先例**：PATCH/POST 走 `UpsertParam.Validate()` → `validate.ClusterName`（create.go:110-113）返回 422；`update_subcluster.go:31-37` 的 URI 参数结构也已有同款 `Validate()` 调 `validate.ClusterName` 的写法——本修复逐字对齐该模式。已核实。
4. **存储层背景（与 issue #308 同族）**：`clusters` 表 `name` 列默认 `CHARSET=utf8`（`db_ddl.sql` L10-20），即 `utf8_general_ci`——PAD SPACE + 大小写不敏感。入口校验缺失时非法名同样可能在比较层命中相近名称；虽然 cluster 名不存在写入风险，入口校验仍是精确性的正确来源。

### 1.3 契约依据

- `00-common.md#15-集群名称clustername`（L419）：长度 1-64 字符；仅允许字母、数字、`_`、`-`、`.`；不能以 `.`、`-`、`_` 开头或结尾；不能包含空白字符。
- `clusters.md` 的 GET/DELETE 路径参数引用该公共类型，未设例外。
- 非法参数返回 422；仅格式合法但不存在的名称返回 404。

## 2. 修复方案

### 2.1 方案：为 `OneParam` 补 `Validate()`（对齐同包既有模式）

在 `endpoints/openapi_v1/product_cluster/one.go` 为 `OneParam` 增加：

```go
// Validate performs centralized business validation on the request parameters.
func (p *OneParam) Validate() error {
	return validate.ClusterName(*p.Name)
}
```

机制：`xreq.BindURI` → `validateData` 在 struct tag 校验通过后自动调用 `Validate()`（`lib/xreq/validate.go:33-46`）；`required` 保证指针非空，解引用安全。与 `update_subcluster.go:31-37` 的既有写法逐字同模式，也与 issue #308 修复的 auth 包 `UserNameParam.Validate()` 同构。

**不改端点、不改存储层、无 DDL 变更。** 三端点（GET/DELETE/ready）一处修复全部覆盖。

### 2.2 修复后行为

| 请求 | 修复前 | 修复后 |
|------|--------|--------|
| `GET /clusters/{257字符}` | 404 | **422**（同 PATCH/POST 语义） |
| `DELETE /clusters/{257字符}` | 404 | **422** |
| `GET /clusters/{cluster_name}/ready`（非法名） | 404 | **422**（同一参数结构，行为随之一致） |
| `PATCH /clusters/{257字符}` | 422 | 422（不变） |
| `GET /clusters/{合法但不存在}` | 404 | 404（不变，对照臂） |
| `GET /clusters/{65字符}` | 404 | 422（65 > 64 上限） |
| `GET /clusters/{首尾-、.、_}` | 404 | 422（边界字符集外） |
| `GET /clusters/ABC`（合法名大小写变体） | 200（collation 大小写不敏感） | 200（**刻意保持**——存储层语义，契约未禁止） |

> 说明：`ready` 端点 issue 建议中列为"等复用端点"，核实时确认它复用 `OneParam`（ready.go:44 `newOneParam4Ready` 绑定同一结构），自动纳入修复，无需单独改动。

## 3. 测试计划

### 3.1 单元测试（`endpoints/openapi_v1/product_cluster/`）

新增 `TestOneParamValidate`（风格对齐 auth 包 `TestUserNameParamValidate`）：

| 用例 | 构造 | 期望 |
|------|------|------|
| 合法名 | `"cluster-1"` | 通过 |
| 65 字符 | 超长 | 422 |
| 257 字符 | issue 复现值 | 422 |
| 非法字符 | `"cluster+1"`、含空白 | 422 |
| 首尾边界 | `"-c"`、`"c-"`、`".c"`、`"_c"` | 422 |
| 空/纯空白 | `""`、`" "` | 422（required/ClusterName 双重拦截） |

### 3.2 集成测试（`test/integration/tests/clusters/`）

在既有 `detail` / `delete` 场景追加（不新建场景目录）：

| 用例建议 | 请求 | 期望 |
|----------|------|------|
| detail 场景追加 | `GET /clusters/{257字符}` | 422 |
| detail 场景追加 | `GET /clusters/{65字符}`、`{非法字符}`、`{-leading}` | 422 |
| delete 场景追加 | `DELETE /clusters/{257字符}` | 422 |
| detail 场景追加（对照臂） | `GET /clusters/{合法但不存在}` | 404（区分"非法"与"不存在"的核心断言） |

后端：SQLite + MySQL 双后端（校验在入口层，断言与后端无关）。

### 3.3 测试设计文档（跨仓跟进）

`docs/test-design/SC2101/SC2101-TC024.md` 在 integration-test 仓，需补充 GET/DELETE 超长路径参数断言（本仓无法完成，列为跨仓跟进项）。

### 3.4 E2E / 验收判据

- SC2101-TC024 补充断言：GET/DELETE 超长路径参数 → 422；PATCH 422 不变；合法不存在名 → 404。
- 回归臂：既有 clusters create/update/detail/delete/list/epp 场景不回退。

## 4. 兼容性与发布

| 项 | 结论 |
|----|------|
| 行为变化 | 非法名请求由 404 收紧为 422（契约早已声明字符集，属纠偏）；合法请求零影响 |
| 数据迁移 | 无 DDL、无数据变更 |
| 错误码一致性 | GET/DELETE/ready 与 PATCH/POST 的 ClusterName 校验语义自此一致 |
| 下游依赖 | 仅管理面 OpenAPI；Web 前端传非法名的场景本就不存在 |

## 5. 关联风险与后续事项（不在本次范围）

1. **同类巡检结论**：auth 包（#308 已修）、product_cluster 包（本次）之外，`endpoints/openapi_v1/` 其余按名端点已逐一巡检——`entity`/`entity_type`/`api_key`/`provider`/`certificate`/`model_price` 的 URI 参数结构均已有 `Validate()` 或自定义校验，未发现同型缺口。（如后续发现遗漏，按 #308 同模式处理。）
2. **跨仓测试设计修订**：integration-test 仓 `SC2101-TC024.md` 补充 GET/DELETE 断言。
3. **`ready` 端点的语义说明**：其 422 行为变化是同参数结构修复的自然结果；若产品认为 ready 应保持宽松，需为 ready 单独定义参数结构——但按"同一公共类型同一校验"原则，本方案不拆分。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 代码实现（`product_cluster/one.go` `OneParam.Validate` → `validate.ClusterName`，一处覆盖 GET/DELETE/ready 三端点）
- [x] 单元测试（`TestOneParamValidate` 7 用例：257 复现值/65 上限/非法字符/尾随空白/首尾边界；端点包全量通过）
- [x] 集成测试（detail 场景新增 `TestClusters_Detail_InvalidName`：257/65/非法字符 → 422 + 合法不存在 → 404 对照臂；delete 场景 `CL-5-010/011`；SQLite 与 MySQL 双后端 6 包全过）
- [x] 文档 review：`clusters.md` GET/DELETE 参数行本就已引用 ClusterName 类型（无需改动）；sys-design 无单位表述（无需改动）
- [ ] E2E SC2101-TC024 补充断言转 PASS 并关闭 issue #311
- [ ] 跨仓跟进：integration-test 仓 `SC2101-TC024.md` 补充 GET/DELETE 超长路径参数断言

## 7. 实施校准记录（实现过程中确认的事实）

1. **ready 端点自动纳入**：`GET /clusters/{cluster_name}/ready`（ready.go:44）复用 `OneParam`，修复后非法名同样 422——与 issue 建议一致，无需单独改动。
2. **api-define 零改动**：`clusters.md` 详情（L592-600）与删除（L711-719）参数行的合法性条件本就写作"类型为 ClusterName"，实现补齐后文档与代码完全对齐——本修复是纯实现缺口，无契约修订。
3. **单字符名称回归守护**：既有 `TestClusters_Detail_SingleCharName` / `CL-5-009`（单字符名集群可查可删）在本修复下仍通过，`ClusterName` 的 1-64 下限（min=1）语义未受影响。
