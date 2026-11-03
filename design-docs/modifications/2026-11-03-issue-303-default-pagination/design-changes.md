# 设计变更：#303 列表接口默认分页回填

## 1. 现状

### 1.1 三处缺陷代码（同构）

`endpoints/openapi_v1/api_key/list.go:76-101,116-125,133-135`：

```go
var page, pageSize int                              // 零值 0/0
isPagination := listReq.Page != nil || listReq.PageSize != nil
if isPagination {                                   // ← 无参时整体跳过
    page = 1
    if listReq.Page != nil && *listReq.Page > 0 { page = *listReq.Page }
    pageSize = 20
    if listReq.PageSize != nil && *listReq.PageSize > 0 { pageSize = *listReq.PageSize }
    if pageSize > 100 { pageSize = 100 }
}
filter := &api_key.APIKeyFilter{...}
if isPagination { filter.Page, filter.PageSize = &page, &pageSize }   // ← 无参时不设分页 → DAO 不加 LIMIT
...
total := len(list)
if isPagination { ... 全量再查一次 ... total = len(allList) }
resp.Pagination.Page = page                         // 0
resp.Pagination.PageSize = pageSize                 // 0
```

`endpoints/openapi_v1/entity/list.go:49-66,82-93` 与 `entity_type/list.go:49-66,74-85` 是同一份代码的复制。

### 1.2 为什么 DAO 层没有兜底

`storage/rdb/api_key/api_key.go:120-122`：

```go
if filter.Page != nil && filter.PageSize != nil {
    offset := (*filter.Page - 1) * *filter.PageSize
    param.Limit = []uint{uint(offset), uint(*filter.PageSize)}
}
```

分页参数「同时非空」才下推，因此 handler 不设值时表现为「不分页、返回全量」。也就是说：
`isPagination=false` 时接口既返回了全量数据、又在元数据里声称 `page=0/page_size=0`，两头都不符合契约。

### 1.3 文档化例外与参考实现

- `provider/list.go:51-64,115-140`：无参 → `page=1`、`page_size=total`、上限 1000。**这是 providers.md §2.2 明文约定的**
  （「未携带 page/page_size 时返回全部匹配记录」），不是缺陷，但需要在 `00-common` 里登记例外。
- `operation_log/list.go:84-105`、`security/list.go:111-131`、`model_price/list.go:70`：无条件取默认 1/20、
  显式值覆盖、>100 截断 —— 正确的写法，本次修复向它们对齐。

## 2. 契约口径

| 接口 | `page` 非法值 | `page_size` 非法值 | 无参默认 | 来源 |
|---|---|---|---|---|
| api-keys | 422（`page must be > 0`） | 422（`page_size must be between 1 and 100`） | 1 / 20 | api-keys.md §2.2 + issue 验收 3 |
| entities | clamp 到 1 | `<1 → 20`、`>100 → 100` | 1 / 20 | entities.md:241-242 |
| entity-types | clamp 到 1 | 同上 | 1 / 20 | entity-types.md:82-83 |
| providers | clamp | `>1000 → 1000` | 1 / total | providers.md:295-296,312 |

结论：**默认值与截断是共性，非法值是否 422 由各接口自己的 Query 参数表决定**。
helper 只负责「默认值 + 截断」，严格校验保留在接口侧（api-keys 继续 422，不改现有对外行为）。

## 3. 方案

### 3.1 新增共享 helper（`lib/xreq/pagination.go`）

```go
// Pagination defaults shared by list endpoints; see
// design-docs/api-define/OpenAPI接口定义/00-common.md#通用Query参数.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ResolvePagination returns the effective page/page_size: missing or
// non-positive values fall back to the defaults, and page_size is capped.
func ResolvePagination(page, pageSize *int) (int, int) {
	p := DefaultPage
	if page != nil && *page > 0 {
		p = *page
	}
	ps := DefaultPageSize
	if pageSize != nil && *pageSize > 0 {
		ps = *pageSize
	}
	if ps > MaxPageSize {
		ps = MaxPageSize
	}
	return p, ps
}

// ValidatePagination enforces the strict contract used by endpoints whose
// design docs require 422 on invalid values (e.g. api-keys). Endpoints that
// clamp (entities/entity-types) must not call it.
func ValidatePagination(page, pageSize *int) error {
	if page != nil && *page <= 0 {
		return xerror.WrapParamErrorWithMsg("page must be > 0, got %d", *page)
	}
	if pageSize != nil && (*pageSize <= 0 || *pageSize > MaxPageSize) {
		return xerror.WrapParamErrorWithMsg("page_size must be between 1 and %d, got %d", MaxPageSize, *pageSize)
	}
	return nil
}
```

错误文案与现状逐字一致（`lib/xreq` 已依赖 `lib/xerror`），不会影响依赖文案的用例。

### 3.2 api_key 接线

```diff
 func ListAction(req *http.Request) (interface{}, error) {
 	listReq := &APIKeyListReq{}
 	if err := xreq.BindForm(req, listReq); err != nil {
 		return nil, err
 	}
 
-	if listReq.Page != nil && *listReq.Page <= 0 {
-		return nil, xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("page must be > 0, got %d", *listReq.Page))
-	}
-	if listReq.PageSize != nil && (*listReq.PageSize <= 0 || *listReq.PageSize > 100) {
-		return nil, xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("page_size must be between 1 and 100, got %d", *listReq.PageSize))
-	}
+	if err := xreq.ValidatePagination(listReq.Page, listReq.PageSize); err != nil {
+		return nil, err
+	}
 	...
-	var page, pageSize int
-	isPagination := listReq.Page != nil || listReq.PageSize != nil
-	if isPagination {
-		page = 1
-		...
-	}
+	page, pageSize := xreq.ResolvePagination(listReq.Page, listReq.PageSize)
 
 	filter := &api_key.APIKeyFilter{
 		ProductName:    &productName,
 		Enabled:        listReq.Enabled,
 		EntityID:       listReq.EntityID,
 		UnlimitedQuota: listReq.UnlimitedQuota,
+		Page:           &page,
+		PageSize:       &pageSize,
 	}
-	if isPagination {
-		filter.Page = &page
-		filter.PageSize = &pageSize
-	}
```

`total` 改为「同一组过滤条件下、不分页」的条数（§3.4）。

### 3.3 entity / entity_type 接线

两者删掉 `isPagination` 分支，改为：

```go
page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
filter.Page, filter.PageSize = &page, &pageSize
```

（这两处**不**调用 `ValidatePagination`：entities/entity-types 的文档是 clamp 语义，现状也没有 422。）

### 3.4 `total` 口径（同批修正）

实测（2026-11-03，HEAD `c14bac7`，2 个 Key、其中 1 个 `enabled=false`）：

| 请求 | `pagination` | `list` 长度 |
|---|---|---|
| `enabled=false&page=1&page_size=20` | `page:1 page_size:20 total:2` | 1 |
| `enabled=true&page=1&page_size=20` | `page:1 page_size:20 total:2` | 1 |

`total` 恒为全量 2，与 `list` 长度 1 矛盾。源码层面同样成立：

`total` 现在与过滤条件不一致：

- `api_key/list.go:116-125`：count 查询只带 `ProductName`，丢掉 `Enabled`/`EntityID`/`UnlimitedQuota`；
- `entity/list.go:82-93`：count 查询只带 `Type`/`ParentID`/`QuotaPlanID`，丢掉 `EntityID`/`Name`/`RouteRulesID`；
- `entity_type/list.go`：count 查询字段已完整。

改法（避免逐字段列举再次漏字段）：

```go
countFilter := *filter            // 复制当前过滤条件
countFilter.Page, countFilter.PageSize = nil, nil
allList, err := container.XxxManager.FetchXxxList(req.Context(), &countFilter)
total = len(allList)
```

并把 `pagination.total` 的定义写进文档：**满足查询过滤条件的总条数（与分页无关）**。

### 3.5 provider 不动，但登记例外

`provider/list.go` 保持「无参返回全部、`page_size=total`、上限 1000」，仅在 `00-common.md` 增加例外说明，
避免后续维护者按通用默认值「顺手改坏」。

## 4. 行为矩阵（修复后）

| 请求 | `list` | `pagination` |
|---|---|---|
| `GET /api-keys`（无参） | 前 20 条 | `page=1, page_size=20, total=全量` |
| `GET /api-keys?page=2&page_size=1` | 第 2 条 | `page=2, page_size=1, total=全量` |
| `GET /api-keys?enabled=false`（无分页参数） | 前 20 条匹配项 | `page=1, page_size=20, total=匹配条数` |
| `GET /api-keys?page=0` | — | `422 page must be > 0, got 0` |
| `GET /api-keys?page_size=101` | — | `422 page_size must be between 1 and 100, got 101` |
| `GET /entities?page_size=500` | 100 条 | `page=1, page_size=100`（clamp） |
| `GET /entities?page=-1` | 前 20 条 | `page=1`（clamp，不 422） |
| `GET /providers`（无参） | 全部 | `page=1, page_size=total`（例外，不变） |

## 5. 跨仓影响（必须协同）

`ai-gateway-web/src/modules/Clusters/index.vue:441`：

```js
this.$request({ url: 'api-keys', method: 'get', openapi: true, unneedTips: true })
  .then(res => { /* 用整个 list 构建 apikey:<id> → name 映射 */ })
```

该调用**不带分页参数**并依赖「返回全量」。默认分页生效后只能拿到前 20 条，Key 超过 20 个时映射缺失。
协同方案（在 web 仓实施）：

1. 首选：循环翻页拉全量（`page_size=100`，按 `pagination.total` 继续翻），封装成一个 `fetchAllApiKeys()`；
2. 或产品侧接受「下拉只显示前 100 个 Key」，则显式传 `page=1&page_size=100` 并在文档里写明上限。

> 本仓交付前必须确认该改造已排期，否则升级后 Clusters 页面会出现 Key 名称缺失。

## 6. 测试

### 6.1 单元测试

`lib/xreq/pagination_test.go`：

| 输入 | 期望 |
|---|---|
| `nil, nil` | `1, 20` |
| `2, nil` | `2, 20` |
| `nil, 5` | `1, 5` |
| `0, 0` | `1, 20`（clamp 语义） |
| `nil, 101` | `1, 100` |
| `nil, 100` | `1, 100` |

`ValidatePagination`：`nil/nil` → nil；`0/nil`、`-1`、`page_size=0`、`page_size=101` → 错误的文案与现状逐字一致。

### 6.2 集成测试

| 包 | 新增用例 | 断言 |
|---|---|---|
| `tests/api_key/list` | `AK-2-004 默认分页` | 无参：200，`page=1`、`page_size=20`、`total` = 全量，`len(list) ≤ 20` |
| | `AK-2-005 相邻页` | `page=1&page_size=1` 与 `page=2&page_size=1` 的 `list[0].id` 不同且都属于全量 |
| | `AK-2-006 非法值仍 422` | `page=0`、`page=-1`、`page_size=0`、`page_size=101` 均为 422，文案含字段名 |
| | `AK-2-007 过滤 + 默认分页` | `enabled=false`（无分页参数）：`page=1/page_size=20`，`total` = 该过滤下的总数 |
| `tests/entity/list`、`tests/entity_type/list` | `E-2-00x` / `ET-2-00x 默认分页与 clamp` | 无参：`page=1/page_size=20`；`page_size=500` → `100`；`page=-1` → `1` |
| `tests/entity/list` | 过滤后 `total` 一致性 | `name=<唯一名>` → `total=1` |

> 既有用例需复核：`tests/auth/readonly_admin`（无参拉 `api-keys`，当前 fixture 只有 2 个 Key，默认 20 不影响）、
> `tests/schema/openapi`（只校验 schema）。若后续 fixture 增多，这些无参调用要显式传 `page_size`。

## 7. 文档同步

| 文件 | 改动 |
|---|---|
| `00-common.md#通用Query参数` | 补一句：默认值/截断为通用基线，**显式非法值的处理按各接口 Query 参数表**（api-keys 422；entities/entity-types clamp）；`provider` 为「无参返回全部」的登记例外（上限 1000） |
| `api-keys.md` §2.2 / `entities.md` / `entity-types.md` | 在 `pagination` 字段说明里明确 `total` = 满足过滤条件的总条数 |
| `providers.md` §2.2 | 不动（已是例外语义），可在 `00-common` 交叉引用 |
| 本目录 | 三份变更说明 |

## 8. 验证步骤

```bash
# 1) 单测
go test ./lib/xreq/ -run 'TestResolvePagination|TestValidatePagination' -count=1 -v
go test ./... -count=1

# 2) 集成（SQLite 与 MySQL 各一轮）
go build -o ai-gateway-api.exe .
cd test/integration
AIAPI_FORCE_SQLITE=1 go test -count=1 -p 1 \
  ./tests/api_key/list/ ./tests/entity/list/ ./tests/entity_type/list/ -run 'DefaultPagination|AdjacentPages|InvalidPagination|Filter'

# 3) 手工核对
curl -s 'http://127.0.0.1:8183/open-api/v1/api-keys'                        # page=1,page_size=20,total=全量
curl -s 'http://127.0.0.1:8183/open-api/v1/api-keys?page_size=101'          # 422
curl -s 'http://127.0.0.1:8183/open-api/v1/entities?page_size=500'          # page_size=100
curl -s 'http://127.0.0.1:8183/open-api/v1/providers'                       # page=1,page_size=total（不变）
```

## 9. 备选方案

| 方案 | 做法 | 结论 |
|---|---|---|
| A（采纳） | helper 统一默认值/截断，三个 handler 无条件下推分页 | 与文档和参考实现一致，顺带消除 4 处复制粘贴 |
| B | 只修响应：`page==0 → 1`、`pageSize==0 → 20`，但仍返回全量 | 元数据与数据不一致（`list` 长度 > `page_size`），调用方按页取数会错乱；否决 |
| C | 全切成 provider 式「无参返回全部 + page_size=total」 | 与 api-keys/entities/entity-types 文档的默认 20 冲突；否决 |
| D | 新增 `page_size=0` 表示「全部」并保持无参=默认 20 | 可行但属契约新增（需要产品确认并改文档），本 issue 不引入 |

## 10. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 无参调用方数据变少（Web UI） | 已知消费方一处，跨仓同版本改造（§5）；脚本型调用方文档提示显式 `page_size` |
| `total` 计算全表扫描（现状已是如此） | 本次不新增 DAO；记录后续优化为 `SELECT COUNT(*)`（`Count*` storager + DAO），避免列表变大后 O(n) |
| 三个 handler 漏改 | `grep -rn isPagination endpoints/` 收尾，预期只剩 provider 一处 |
| 文案/行为回归 | `ValidatePagination` 文案与现状逐字对齐，严格/clamp 语义按接口区分，测试覆盖 |

回滚：把三个 handler 的 `ResolvePagination` 调用与其下推翻回原 `isPagination` 分支即可；
`lib/xreq` 的 helper 保留不影响其它接口。
## 11. 实施与验证记录（2026-11-03）

- `lib/xreq/pagination.go` + 三处 handler 接线已落地；`grep -rn isPagination endpoints/` 只剩
  `provider/list.go`（文档化例外）与 `entity/list.go`、`entity_type/list.go` 中的同名变量已删除；
- 无参请求实测：`api-keys`/`entities`/`entity-types` 由 `page:0 page_size:0` 变为 `page:1 page_size:20`；
  `providers`（`page:1 page_size:total`）与 `operation-logs`（`page:1 page_size:20`）保持不变；
- 单测：`lib/xreq` 新增 12 个断言组全 PASS；全量单元 104 包 ok；
- 集成：MySQL 8.4.9 后端全量 130 包 ok（0 FAIL）；SQLite 定向 3 包（含新增 8 个子用例）PASS；
- 已知跨仓事项：`ai-gateway-web` 的 Clusters 页面仍以无参方式拉取 `api-keys`，本次按要求未修改该仓，
  上线前必须确认其翻页改造已排期（§5）。
