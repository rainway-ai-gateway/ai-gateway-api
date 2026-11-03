# Issue #303：无分页参数的列表请求返回 `pagination.page=0 / page_size=0`

## 1. 问题来源

[rainway-ai-gateway/ai-gateway-api#303](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/303)

> 管理面 `GET /open-api/v1/api-keys` 在完全不传 `page` / `page_size` 时返回 `pagination.page=0`、`page_size=0`，
> 而设计规定默认 `page=1`、`page_size=20`。按分页元数据驱动的 UI / 脚本会把默认请求误判为「第 0 页、每页 0 条」。

## 2. 根因

`endpoints/openapi_v1/api_key/list.go:76`：

```go
var page, pageSize int                       // 零值 0/0
isPagination := listReq.Page != nil || listReq.PageSize != nil
if isPagination {                            // ← 无 query 参数时为 false，默认回填整体跳过
    page = 1
    if listReq.Page != nil && *listReq.Page > 0 { page = *listReq.Page }
    pageSize = 20
    if listReq.PageSize != nil && *listReq.PageSize > 0 { pageSize = *listReq.PageSize }
    if pageSize > 100 { pageSize = 100 }
}
...
resp.Pagination.Page = page                  // list.go:133-134 把 0/0 原样写进响应
resp.Pagination.PageSize = pageSize
```

即：「是否分页」被错误地用来决定「是否回填默认值」。默认值本应无条件生效，
`isPagination` 只应决定「请求里是否显式带了分页参数」（在修复后甚至不再需要这个分支）。

**同一模式共 4 处**（`grep -rn isPagination endpoints/`）：

| 位置 | 无参时 `pagination` | 判定 |
|---|---|---|
| `endpoints/openapi_v1/api_key/list.go:76` | `page=0, page_size=0` | **缺陷**（本 issue） |
| `endpoints/openapi_v1/entity/list.go:49` | `page=0, page_size=0` | **同缺陷**（entities.md 明确默认 20） |
| `endpoints/openapi_v1/entity_type/list.go:49` | `page=0, page_size=0` | **同缺陷**（entity-types.md 明确默认 20） |
| `endpoints/openapi_v1/provider/list.go:51` | `page=1, page_size=total` | **文档化例外**（见 providers.md:312） |

参考实现（一直正确）：`operation_log/list.go:84-105`、`security/list.go:111-131`、
`model_price/list.go:70` —— 都是「无条件取默认 1/20，显式值覆盖，>100 截断」。

## 3. 实测证据（2026-11-03，HEAD `c14bac7`，SQLite，真实服务，均不带任何 query 参数）

| 接口 | 状态 | `pagination` |
|---|---|---|
| `GET /open-api/v1/api-keys` | 200 | `page:0 page_size:0 total:0` |
| `GET /open-api/v1/entities` | 200 | `page:0 page_size:0 total:0` |
| `GET /open-api/v1/entity-types` | 200 | `page:0 page_size:0 total:0` |
| `GET /open-api/v1/providers` | 200 | `page:1 page_size:1 total:1`（例外语义：page_size=total） |
| `GET /open-api/v1/operation-logs` | 200 | `page:1 page_size:20 total:0`（参考实现） |

与 issue 中 qa 环境两次独立运行（2026-10-14、2026-11-03）的 `page=0/page_size=0` 一致；
本地基线库为空，所以 `total=0`，元数据缺陷与数据量无关。

另用「2 个 Key（1 个禁用）」的临时 fixture 验证了 `total` 口径问题（`GET /open-api/v1/api-keys`）：

| 请求 | `pagination` | `list` 长度 |
|---|---|---|
| 无参 | `page:0 page_size:0 total:2` | 2 |
| `enabled=false&page=1&page_size=20` | `page:1 page_size:20 **total:2**` | **1** |
| `enabled=true&page=1&page_size=20` | `page:1 page_size:20 **total:2**` | **1** |
| `page=1&page_size=1` | `page:1 page_size:1 total:2` | 1 |

即：带过滤且显式分页时，`total` 仍是**全量**（未随过滤条件变化），与 `list` 长度矛盾 —— 这就是
§6 里 `total` 口径修正的依据。

## 4. 契约基线（各文档口径）

| 来源 | 约定 |
|---|---|
| `00-common.md#通用Query参数` | `page` 默认 1，"必须 >0，否则使用默认值 1"；`page_size` 默认 20，最大 100，"超出时截断为 100" |
| `api-keys.md` §2.2 | `page` 默认 1，**必须 >0**；`page_size` 默认 20，**取值范围 1-100**（现实现：非法值 → 422） |
| `entities.md` / `entity-types.md` | `page` 默认 1，**<=0 按 1 处理**；`page_size` 默认 20，**<1 按 20、>100 按 100**（现实现：clamp，无 422） |
| `providers.md` §2.2 | **未传 page/page_size 时返回全部记录**，`pagination.page=1`、`page_size=total`；最大 1000 |

两处文档张力需要顺带澄清（本 issue 不改变现行为）：

1. 非法值的处理分两派：`api-keys` 严格 422（issue 验收判据 3 要求保持），`entities`/`entity-types` 按默认值 clamp；
   `00-common` 的「否则使用默认值 1」只描述了后者。
2. `pagination.total` 的语义（全量条数 vs 过滤后条数）在任何文档里都没有明确定义。

## 5. 目标

1. 无参 `GET /open-api/v1/api-keys` → `pagination.page=1`、`page_size=20`、`total` 为全量条数，
   且 `list` 长度 ≤ `page_size`（默认分页生效，而不是返回全部）；
2. `entity` / `entity-type` 列表同规则修复（同类缺陷，文档同样要求默认 20）；
3. 非法值校验不放松：`api-keys` 的 `page=0/-1`、`page_size=0/101` 仍 422；
   `entities`/`entity-types` 保持 clamp（`<=0 → 1`、`<1 → 20`、`>100 → 100`）；
4. `providers` 保持文档化的「无参返回全部」例外，但要在 `00-common` 中显式记录该例外；
5. 相邻行为不回归：显式分页、过滤条件、排序、响应结构不变。

## 6. 方案概览

新增共享 helper（`lib/xreq/pagination.go`），把「默认值 + 截断」收敛成一处，三个 handler 无条件调用：

```go
const (
    DefaultPage     = 1
    DefaultPageSize = 20
    MaxPageSize     = 100
)

// ResolvePagination returns the effective page/page_size for a list request:
// missing or non-positive values fall back to the documented defaults and
// page_size is capped at MaxPageSize.
func ResolvePagination(page, pageSize *int) (int, int) {
    p := DefaultPage
    if page != nil && *page > 0 { p = *page }
    ps := DefaultPageSize
    if pageSize != nil && *pageSize > 0 { ps = *pageSize }
    if ps > MaxPageSize { ps = MaxPageSize }
    return p, ps
}
```

handler 侧（以 api_key 为例）：

```go
if err := xreq.ValidatePagination(listReq.Page, listReq.PageSize); err != nil { // 仅严格接口使用，保留 422
    return nil, err
}
page, pageSize := xreq.ResolvePagination(listReq.Page, listReq.PageSize)
filter.Page, filter.PageSize = &page, &pageSize   // 无条件下推，DAO 层据此 LIMIT/OFFSET
```

同时修正 `total` 的统计口径（见 design-changes §3.4）：`total` 应等于**同一组过滤条件**下的总条数；
现 `api_key` 的 count 查询只带 `ProductName`、`entity` 的 count 查询漏了 `id/name/route_rules_id`。

详细设计、行为矩阵、测试与跨仓影响见 [design-changes.md](./design-changes.md)；
接口行为对照见 [api-changes.md](./api-changes.md)。

## 7. 范围与跨仓影响

| 范围 | 说明 |
|------|------|
| 本仓代码 | 新增 `lib/xreq/pagination.go`；`api_key/list.go`、`entity/list.go`、`entity_type/list.go` 接线；`provider/list.go` 不动 |
| 本仓测试 | `lib/xreq` 单测；`tests/{api_key,entity,entity_type}/list` 补默认分页/相邻页/非法值用例 |
| 文档 | `00-common.md`（非法值分派 + provider 例外 + `total` 定义）、`entities.md`/`entity-types.md`/`api-keys.md` 的 `total` 说明 |
| **跨仓（必须协同）** | `ai-gateway-web`：`src/modules/Clusters/index.vue:441` 目前 `GET /open-api/v1/api-keys` **不带分页参数**并用全量结果构建 id→name 映射；默认分页生效后只能拿到前 20 条，需同步改为「循环拉取所有页」或显式 `page_size=100`（>100 个 Key 时仍需翻页） |
| 数据面 | 无影响（纯查询接口） |

## 8. 风险

| 风险 | 评估 | 缓解 |
|------|------|------|
| 无参调用方拿不到全部数据（行为变化） | **高** | 这是契约要求的默认分页；已知消费方仅 `ai-gateway-web` 一处（已定位），必须同版本适配；脚本型调用方需显式 `page_size` |
| `total` 口径调整导致数字变化 | 中 | 明确 `total` = 过滤后总数并补测试；`AssertPagination` 只校验下界，不会误伤 |
| 严格/clamp 两派语义混淆 | 中 | 在 `00-common` 写清「默认值统一、非法值处理按接口定义」，helper 只做默认与截断，校验留给接口 |
| 三个 handler 各自漏改 | 中 | 用 `grep -rn isPagination` 作为收尾检查；helper 让「无条件回填」成为唯一写法 |

## 9. 上线前检查清单

1. `go test ./lib/xreq/ ./endpoints/ -count=1` 通过；
2. `AIAPI_FORCE_SQLITE=1 go test -count=1 -run 'TestListDefaultPagination' ./tests/api_key/list/ ./tests/entity/list/ ./tests/entity_type/list/` 通过；
3. 手工：无参 `GET /open-api/v1/api-keys` → `page=1,page_size=20,total=全量`，`list` ≤ 20；
4. 手工：`page=0`、`page=-1`、`page_size=0`、`page_size=101` 仍 422；
5. `GET /open-api/v1/providers`（无参）行为不变（`page=1,page_size=total`）；
6. `grep -rn "isPagination" endpoints/` 只剩 provider 一处（且注释说明为文档化例外）；
7. 全量集成回归 + `ai-gateway-web` 对应改造联调。

## 9. 实施记录（Step 5 / Step 6）

### 9.1 代码改动

| 文件 | 改动 |
|------|------|
| `lib/xreq/pagination.go`（新增） | `DefaultPage/DefaultPageSize/MaxPageSize` 常量、`ResolvePagination`（默认值 + 截断，缺失/非正数一律回落）、`ValidatePagination`（严格接口用，文案与现状逐字一致） |
| `endpoints/openapi_v1/api_key/list.go` | 删掉 `isPagination` 分支；改用 `ValidatePagination` + `ResolvePagination`；分页无条件下推；`total` 改为「同一组过滤条件、不分页」的条数 |
| `endpoints/openapi_v1/entity/list.go` | 同上（不调用 Validate，保持 clamp 语义）；count 改为复制 filter 后清空分页字段 |
| `endpoints/openapi_v1/entity_type/list.go` | 同上 |
| `endpoints/openapi_v1/provider/list.go` | 未改动（providers.md 文档化例外） |

### 9.2 文档改动

| 文件 | 改动 |
|------|------|
| `00-common.md#通用Query参数` | 拆出「默认值与截断（通用基线）」与「非法值处理（按各接口）」；登记 `/providers` 为例外；明确 `pagination.total` 语义 |
| `api-keys.md` / `entities.md` / `entity-types.md` | `pagination` 说明补默认值与 `total`（过滤后总条数）定义 |
| `接口层设计文档` | 新增 §4.4.6「列表分页约定」：默认值无条件生效、生效值必须下推 DAO、非法值分派、`total` 口径、provider 例外登记 |

### 9.3 测试改动

| 文件 | 内容 |
|------|------|
| `lib/xreq/pagination_test.go`（新增） | `ResolvePagination` 8 组表驱动；`ValidatePagination` 4 个非法值（含文案断言） |
| `tests/api_key/list/list_test.go` | AK-2-004 默认分页、AK-2-005 相邻页不重复、AK-2-006 非法值仍 422、AK-2-007 过滤 + `total` 一致 |
| `tests/entity/list/list_test.go` | E-2-004 默认分页与 clamp（`page_size=500 → 100`、`page=-1 → 1`）、E-2-005 过滤 `total` |
| `tests/entity_type/list/list_test.go` | ET-2-003 默认分页与 clamp、ET-2-004 过滤 `total` |

### 9.4 验证结果（2026-11-03，Go 1.25.2 / windows-amd64）

修复前后同一组无参请求（真实服务，本地基线库为空）：

| 接口 | 修复前 | 修复后 |
|---|---|---|
| `GET /open-api/v1/api-keys` | `page:0 page_size:0` | **`page:1 page_size:20`** |
| `GET /open-api/v1/entities` | `page:0 page_size:0` | **`page:1 page_size:20`** |
| `GET /open-api/v1/entity-types` | `page:0 page_size:0` | **`page:1 page_size:20`** |
| `GET /open-api/v1/providers` | `page:1 page_size:total` | 不变（例外） |
| `GET /open-api/v1/operation-logs` | `page:1 page_size:20` | 不变（参考实现） |

| 验证项 | 结果 |
|---|---|
| `go test ./lib/xreq/ -run 'TestResolvePagination|TestValidatePagination'` | PASS |
| `go test ./...` | 见 §9.5 |
| 集成定向（SQLite，`-p 1`）：`api_key/list`、`entity/list`、`entity_type/list` | 3/3 包 PASS（含新增 8 个子用例） |
| 集成全量（MySQL 8.4.9） | 见 §9.5 |

### 9.5 全量回归

| 运行 | 后端 | 结果 |
|---|---|---|
| 单元 | — | `go test ./...` → **104 包 `ok`，0 FAIL** |
| 集成 | MySQL 8.4.9（默认并行，每包独立 `ai_gateway_it_*` 库） | **130 包 `ok`，0 FAIL** |
| 集成定向 | SQLite（`-p 1`）：`api_key/list`、`entity/list`、`entity_type/list` | 3/3 PASS |

首轮「单元 + MySQL 集成并发跑」时 `storage/rdb/version_control` 的
`TestUpsertConfigLastExportedVersion_FirstCreate` 偶发失败（期望 `20261103193020`、实际 `20261103193019`，
即**秒边界**导致的既有 flake，与分页无关）；单独重跑该包通过，串行重跑全量单元 104 包全绿。

### 9.6 遗留与跨仓事项

1. **`ai-gateway-web` 未本次修改（按用户要求只改本仓）**：`src/modules/Clusters/index.vue:441`
   仍以**无参**方式 `GET /open-api/v1/api-keys` 并依赖全量结果构建 id→name 映射；本修复上线后该调用
   只能拿到前 20 个 Key，**Key 超过 20 个时 Clusters 页面会出现名称缺失**。web 仓需同版本改造
   （循环翻页拉全量，或显式 `page_size=100` 并接受上限）。该风险已在本目录 `design-changes.md §5` 记录；
2. `pagination.total` 目前通过「不分页再查一次」计算（O(n)），列表规模变大后建议改为
   `SELECT COUNT(*)`（新增 `Count*` storager + DAO，另案）；
3. `00-common` 的 `sort_by`/`sort_order` 仍未在列表 handler 中实现（本次未涉及）。

### 9.7 沉淀判断（Step 6）

不新增 `sys-design/details/` 文档：约定（默认值无条件生效、生效值必须下推 DAO、非法值分派、
`total` 口径、provider 例外）已完整写进《接口层设计文档》§4.4.6 与 `00-common.md`，
代码侧由一个 30 行的 helper 承载；`details/` 定位是配额同步、导出与版本控制这类复杂机制。

## 10. 参考

- [Issue #303](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/303)
- `design-docs/api-define/OpenAPI接口定义/00-common.md#通用Query参数`
- `design-docs/api-define/OpenAPI接口定义/{api-keys,entities,entity-types,providers}.md` 的列表查询小节
- 参考实现：`endpoints/openapi_v1/operation_log/list.go:84-105`、`security/list.go:111-131`、`model_price/list.go:70`
- 分页下推：`storage/rdb/api_key/api_key.go:120-122`（`Page && PageSize` 同时非空才 `LIMIT`）
- 消费方：`ai-gateway-web/src/modules/Clusters/index.vue:441`