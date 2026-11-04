# 变更说明：`/entities`、`/entity-types` 列表恢复「无分页参数返回完整列表」（#303 后续修订）

## 1. 背景与诉求

issue #303 修复了「无参列表返回 `pagination.page=0/page_size=0`」的问题，同时把
`/api-keys`、`/entities`、`/entity-types` 三个列表接口统一改成**默认分页 1/20**。

使用方反馈：`GET /entities` 与 `GET /entity-types` 需要在**不传任何分页参数**时拿到**完整列表**
（前端一次性渲染 Entity 树/下拉与类型映射、脚本导出全量），改动后只能拿到前 20 条，
即使传 `page_size=100` 也有上限，无法一次调用取全量。

本次修订的目标：**对 `/entities` 与 `/entity-types` 恢复「无分页参数 = 返回全部匹配记录」的语义**
（分页元数据自洽，不再出现 `page=0/page_size=0`），其余列表接口维持 #303 的默认分页不变。

### 1.1 调用方现状（`ai-gateway-web` 只读排查，本次不改该仓）

**无参 `GET /entities` 并直接消费 `Data.list`**：

| 文件 | 用途 |
|---|---|
| `src/modules/Entity/components/EntityList.vue:414` | Entity 列表页主数据（与 `fetchEntityTypeList` 并行加载） |
| `src/modules/APIKey/components/Upsert.vue:1053` | API-Key 编辑页的 Entity 下拉 |
| `src/modules/RouteTable/index.vue:273` | 路由规则表 `entity:<id>` → 名称映射 |
| `src/modules/Clusters/index.vue:422` | 集群表格 `entity:<id>` → 名称映射 |

**无参 `GET /entity-types`**：

| 文件 | 用途 |
|---|---|
| `src/modules/Entity/components/EntityTypeList.vue:263` | Entity-Type 列表页主数据 |
| `src/modules/Entity/components/EntityList.vue:443` | Entity 列表页的 type 名称映射 |
| `src/modules/Entity/components/EntityUpsert.vue:979` | Entity 编辑页的 type 下拉 |

即：两个接口的「无参全量」都是前端既有依赖，#303 之后同时被截断到 20 条。

## 2. 现状（#303 之后）

```go
// endpoints/openapi_v1/entity/list.go:49-51
page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
filter.Page = &page
filter.PageSize = &pageSize

// endpoints/openapi_v1/entity_type/list.go:49-51（同一份逻辑）
page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
filter.Page = &page
filter.PageSize = &pageSize
```

两处都**无条件**下推分页，因此无参请求恒为前 20 条、`pagination.page=1/page_size=20`。

仓库内已有可借鉴的「文档化例外」：`GET /providers`（`provider/list.go:50-71`）——
未传分页参数时返回全部匹配记录，响应 `pagination.page=1`、`page_size=total`，
`providers.md` §2.2 明文约定。

## 3. 方案概览

把 `/entities` 与 `/entity-types` 都改成「**可选分页**」语义（与 `/providers` 对齐）：

| 请求形态 | 行为（两个接口一致） |
|---|---|
| 不传 `page` / `page_size`（可带过滤条件） | 不下推分页，返回**全部匹配记录**；`pagination.page=1`、`page_size=total`（`total=0` 时 `page_size=0`） |
| 传了 `page` 或 `page_size`（任一） | 按各自文档的 clamp 规则分页：`page<=0→1`、`page_size<1→20`、`>100→100`；`total`=过滤后总条数 |
| 其它过滤条件 | 两种模式下都生效；无参模式返回「匹配到的全部」 |

实现要点：

1. 在 `lib/xreq/pagination.go` 增加 `OptionalPagination(values url.Values, page, pageSize *int) (int, int, bool)`：
   用**原始 query 值**判断「是否显式要求分页」（`?page=` 这类空串视为未传——实测 `BindForm` 会把空串绑成 0，
   只看指针会误判），需要分页时复用 `ResolvePagination` 做默认值 + 100 截断；
2. 两个 handler：`paginated=false` 时**不设** `filter.Page/PageSize`（DAO 不加 `LIMIT`），
   fetch 后 `total = len(list)` 并回填 `page_size = total`；`paginated=true` 时保持现有分页 + count 查询；
3. `total` 口径沿用 #303 的修正（= 满足过滤条件的总条数）；无参模式下 `total == len(list)`，省掉二次查询。

范围与边界：

- **只改这两个接口**：`/api-keys` 保持默认 20（文本契约「必须 >0」的严格接口）；`/providers` 不动；
- 不引入新的 query 参数（不搞 `page_size=0 表示全部`），不改变过滤参数语义；
- 空集合：`total=0`、`page_size=0`、`list=[]`（与 `/providers` 一致）。

## 4. 为什么不采用其它做法

| 方案 | 结论 |
|---|---|
| A（采纳）无参 = 返回全部，`page_size=total`，两个接口一致 | 满足诉求；元数据自洽；与 `/providers` 先例一致 |
| B 只改 `/entities`，`/entity-types` 维持 20 | 前端 type 下拉/名称映射仍被截断，问题只解一半；本次按需求两个一起改 |
| C 保持默认 20，让调用方翻页取全量 | 无法满足「一次调用拿全量」，多轮往返且存在并发漂移 |
| D 把 `page_size` 上限提到 1000/10000 | 仍非完整列表，且放大单次响应体 |
| E 新增 `page_size=0`/`all=true` | 属契约新增（需产品确认 + 全接口推广），本次不引入 |

## 5. 风险

| 风险 | 评估 | 缓解 |
|---|---|---|
| 无界响应体（Entity/Entity-Type 数量大时内存/延迟/带宽） | **主要风险** | 仅限这两个接口且仅限「调用方显式不传分页」；文档提示大数据量走分页；后续可评估服务端上限（需产品确认） |
| 与 `/api-keys` 语义不一致 | 中 | `00-common` 的例外清单同时登记 `/providers`、`/entities`、`/entity-types`，其余接口仍默认 20 |
| 既有集成用例断言 `page_size=20` | 低 | 同步改写 `tests/entity/list` 的 E-2-004/E-2-005 与 `tests/entity_type/list` 的 ET-2-003，并补全量用例 |
| `api-keys` 的无参调用（`Clusters/index.vue:441`）仍受默认 20 影响 | 中 | 本次不改 `api-keys`；若前端同样需要全量 Key，应另立修订或改用翻页 |

## 6. 验收判据

1. 无参 `GET /open-api/v1/entities` → 200，`page=1`、`page_size=total`、`len(list)==total`（不受 20 条限制）；
2. 无参 `GET /open-api/v1/entity-types` → 同上；
3. 仅带过滤（`type`/`name`、`type_name`/`level` 等）→ 返回该过滤下的全部记录，`page_size=total`；
4. 显式分页 `page=1&page_size=10` → 仍分页，`page_size=10`，`total`=过滤后总条数；
   `page_size=500` → `page_size=100`；`page=-1` → `page=1`（clamp 不变）；
5. 空集合 → `list=[]`、`total=0`、`page_size=0`、`page=1`；
6. `/api-keys`、`/providers` 行为不变（回归）。

## 7. 上线前检查清单

1. `go test ./lib/xreq/ ./endpoints/openapi_v1/entity/ ./endpoints/openapi_v1/entity_type/ -count=1` 通过；
2. `AIAPI_FORCE_SQLITE=1 go test -count=1 -p 1 -run 'TestEntity_List|TestEntityType_List' ./tests/entity/list/ ./tests/entity_type/list/` 通过；
3. 手工：造 25 个 Entity 与 25 个 Entity-Type → 无参请求 `len(list)=25`、`page=1`、`page_size=25`；
   `?page=2&page_size=10` → 10 条、`page=2`；
4. 手工：`/api-keys` 无参仍为 `page_size=20`；
5. `go test ./...` 与集成全量回归（MySQL 后端）。

## 8. 参考

- 用户诉求（#303 后续修订）；[issue #303](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/303)
- `design-docs/modifications/2026-11-03-issue-303-default-pagination/`（本次修订的前序变更）
- `design-docs/api-define/OpenAPI接口定义/entities.md` §2.2、`entity-types.md` §2.2、`providers.md` §2.2
- `endpoints/openapi_v1/entity/list.go`、`entity_type/list.go`、`provider/list.go`（参考实现）
- `lib/xreq/pagination.go`（#303 引入的 `ResolvePagination` / `ValidatePagination`）
## 9. 实施记录（Step 5 / Step 6）

### 9.1 代码改动

| 文件 | 改动 |
|------|------|
| `lib/xreq/pagination.go` | 新增 `OptionalPagination(values url.Values, page, pageSize *int) (int, int, bool)`：以**原始 query 值**判定是否显式分页（空串算未传），需要分页时复用 `ResolvePagination`；新增 `net/url` 导入 |
| `endpoints/openapi_v1/entity/list.go` | 改用 `OptionalPagination`；`paginated=false` 时不下推分页、`total=len(list)`、`page_size=total`；空集合补 `list=[]`（此前为 `null`） |
| `endpoints/openapi_v1/entity_type/list.go` | 同上 |

### 9.2 文档改动

| 文件 | 改动 |
|------|------|
| `entities.md` §2.2 / `entity-types.md` §2.2 | `page`/`page_size` 补「未传分页参数时返回全部匹配记录」；`pagination.page/page_size` 补无参模式的取值 |
| `00-common.md#通用Query参数` | 例外清单扩为 `/providers`、`/entities`、`/entity-types`（无参返回全部、`page_size=total`） |
| `接口层设计文档` §4.4.6 | 新增第 6 条「可选分页的实现约定」（原始 query 判定、空串归一、汇总元数据回填） |
| 本目录 | 三份变更说明 |

### 9.3 测试改动

| 文件 | 内容 |
|------|------|
| `lib/xreq/pagination_test.go` | 新增 `TestOptionalPagination`（8 组：无参、空串、仅 page、仅 page_size、非法 clamp、超上限截断、显式值） |
| `tests/entity/list` | E-2-004/005 改写为「无参返回全量」；新增 E-2-006（25 条不被截断 + 显式分页仍生效）、E-2-007（clamp）、E-2-008（空集合元数据） |
| `tests/entity_type/list` | ET-2-003/004 改写；新增 ET-2-005（25 条不被截断）、ET-2-006（clamp） |

### 9.4 验证结果（2026-11-04，Go 1.25.2 / windows-amd64）

| 验证项 | 结果 |
|---|---|
| `go test ./lib/xreq/ -run 'TestOptionalPagination|TestResolvePagination|TestValidatePagination'` | PASS |
| `go test ./...` | **104 包 `ok`，0 FAIL** |
| 集成定向（SQLite，`-p 1`）：`tests/entity/list`、`tests/entity_type/list` | 2/2 包 PASS（E-2-001~008、ET-2-001~006） |
| 集成全量（MySQL 8.4.9） | 见 §9.5 |

### 9.5 全量回归

| 运行 | 后端 | 结果 |
|---|---|---|
| 单元 | — | 104 包 ok |
| 集成 | MySQL 8.4.9（默认并行，每包独立 `ai_gateway_it_*` 库） | **130 包 `ok`，0 FAIL** |

### 9.6 顺带修正与遗留

1. **空集合返回 `list: []`**：修复前 `/entities`、`/entity-types` 在无匹配记录时返回 `"list": null`
   （handler 未做 nil→空切片归一，`api-keys` 早已做了）。本次一并修正，与本变更的「空集合元数据自洽」
   目标一致，属响应形状的小幅规范化（数组字段不再出现 `null`）；
2. **`/api-keys` 无参仍为默认 20**（`Clusters/index.vue:441` 依赖无参全量 Key）：本次未改，
   如需要应另立修订；
3. **`ai-gateway-web` 未修改**（按用户要求只改本仓）：4 处无参 `/entities` 与 3 处无参 `/entity-types`
   调用在本修复后恢复可用，无需前端改动。

### 9.7 沉淀判断（Step 6）

不新增 `sys-design/details/` 文档：约定已写进《接口层设计文档》§4.4.6 与 `00-common.md`，
代码由 `OptionalPagination` 一个 helper 承载；`details/` 定位是配额同步、导出与版本控制这类复杂机制。
