# 设计变更：`/entities` 与 `/entity-types` 列表的「可选分页」语义

## 1. 现状与范式

### 1.1 当前实现（#303 之后）

```go
// endpoints/openapi_v1/entity/list.go:49-51
page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
filter.Page = &page                 // ← 无条件下推：DAO 一定加 LIMIT/OFFSET
filter.PageSize = &pageSize

// endpoints/openapi_v1/entity_type/list.go:49-51（同一份逻辑的复制）
page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
filter.Page = &page
filter.PageSize = &pageSize
```

行为：两个接口的无参请求都恒返回前 20 条、`pagination.page=1/page_size=20`。
诉求（使用方）：无分页参数时应返回**完整列表**（前端 4+3 处无参调用依赖全量，见 change-summary §1.1）。

### 1.2 现成的范式：`/providers`

```go
// endpoints/openapi_v1/provider/list.go:50-71
page, pageSize, isPagination := pageFilter(req)   // 无参 → isPagination=false
if isPagination {
    filter.Page = &page
    filter.PageSize = &pageSize
}
list, total, err := container.ProviderManager.FetchProviderList(req.Context(), filter)
if !isPagination {
    page = 1
    pageSize = int(total)      // ← page_size=total，元数据自洽
    if pageSize < 1 { pageSize = 0 }
}
```

`providers.md` §2.2 明文约定该行为。本次把两个 Entity 相关列表对齐到它，
并复用 `lib/xreq` 的默认值/截断逻辑。

## 2. 目标语义（两个接口一致）

| 请求 | 下推 DAO | `list` | `pagination` |
|---|---|---|---|
| 无 `page`/`page_size`（无过滤） | 否（无 LIMIT） | **全部记录** | `page=1`、`page_size=total`、`total`=全部条数 |
| 无分页参数 + 过滤（`type`/`name`、`type_name`/`level`…） | 否 | **全部匹配记录** | `page=1`、`page_size=total`（=匹配条数） |
| 只传 `page` | 是 | 第 page 页前 20 条 | `page`、`page_size=20`、`total`=匹配条数 |
| 只传 `page_size` | 是 | 前 `page_size` 条 | `page=1`、`page_size`=生效值、`total`=匹配条数 |
| `page_size=500` | 是 | 前 100 条 | `page_size=100`（clamp） |
| `page=-1` | 是 | 前 20 条 | `page=1`（clamp） |
| 空集合 | 否 | `[]` | `page=1`、`page_size=0`、`total=0`（与 `/providers` 一致） |
| `?page=` / `?page_size=`（空串） | 否 | **全部记录** | 空串按未传处理（见 §4） |

不变项：过滤条件语义、clamp 规则、`total` = 满足过滤条件的总条数（#303 的修正保留）、
响应结构 `{list, pagination{page, page_size, total}}`。

## 3. 实现方案

### 3.1 新增 helper（`lib/xreq/pagination.go`）

```go
// OptionalPagination resolves pagination for list endpoints whose contract is
// "no pagination parameters means return everything" (e.g. /entities,
// /entity-types, /providers).
//
// Blank values ("?page=") count as absent: xreq.BindForm binds them as 0, which
// would otherwise look like an explicit (invalid) page request.
//
// paginate == false means the caller must NOT push paging down and should report
// page=DefaultPage, page_size=total after fetching.
func OptionalPagination(values url.Values, page, pageSize *int) (int, int, bool) {
	if values.Get("page") == "" && values.Get("page_size") == "" {
		return DefaultPage, 0, false
	}
	effectivePage, effectivePageSize := ResolvePagination(page, pageSize)
	return effectivePage, effectivePageSize, true
}
```

保留 `ResolvePagination`（默认 1/20 + 100 截断，供已确认分页的分支使用）与
`ValidatePagination`（严格 422 接口用，如 `/api-keys`），二者语义不变。

> 说明：判断「是否显式分页」必须看**原始 query 值**（`url.Values`），不能只看 `*int` 是否为 nil ——
> 实测 `?page=` 会被 `BindForm` 绑成 0（非 nil），只看指针会把「空参数」误判成「显式分页」。
> 后续可把 `provider/list.go` 的私有 `pageFilter` 也收敛到该 helper（可选清理，不阻塞本次）。

### 3.2 `endpoints/openapi_v1/entity/list.go`

```diff
 	filter := &entity.EntityFilter{}
 	if err := xreq.BindForm(req, filter); err != nil {
 		return nil, err
 	}
 
-	page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
-	filter.Page = &page
-	filter.PageSize = &pageSize
+	// 无 page/page_size 时表示「返回全部匹配记录」（entities.md §2.2，语义同 /providers）：
+	// 不下推分页，并用 total 回填 page_size；空串参数（?page= / ?page_size=）按未传处理。
+	page, pageSize, paginated := xreq.OptionalPagination(req.URL.Query(), filter.Page, filter.PageSize)
+	if paginated {
+		filter.Page = &page
+		filter.PageSize = &pageSize
+	}
 
 	if filter.Type != nil && *filter.Type == "" { filter.Type = nil }
 	if filter.ParentID != nil && *filter.ParentID == "" { filter.ParentID = nil }
 
 	list, err := container.EntityManager.FetchEntityList(req.Context(), filter)
 	if err != nil { return nil, err }
 
-	// total counts the whole filtered collection; reuse the same filters so it
-	// never drifts from list (issue #303).
-	countFilter := *filter
-	countFilter.Page, countFilter.PageSize = nil, nil
-	allList, err := container.EntityManager.FetchEntityList(req.Context(), &countFilter)
-	if err != nil { return nil, err }
-	total := len(allList)
+	total := len(list)
+	if paginated {
+		// total counts the whole filtered collection; reuse the same filters so it
+		// never drifts from list (issue #303).
+		countFilter := *filter
+		countFilter.Page, countFilter.PageSize = nil, nil
+		allList, err := container.EntityManager.FetchEntityList(req.Context(), &countFilter)
+		if err != nil { return nil, err }
+		total = len(allList)
+	} else if total > 0 {
+		pageSize = total // page_size=total，与 /providers 一致；空集合保持 0
+	}
```

### 3.3 `endpoints/openapi_v1/entity_type/list.go`（同构，注意这里没有过滤条件归一）

```diff
-	page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
-	filter.Page = &page
-	filter.PageSize = &pageSize
+	// 无 page/page_size 时表示「返回全部匹配记录」（entity-types.md §2.2，语义同 /providers）
+	page, pageSize, paginated := xreq.OptionalPagination(req.URL.Query(), filter.Page, filter.PageSize)
+	if paginated {
+		filter.Page = &page
+		filter.PageSize = &pageSize
+	}
 
 	list, err := container.EntityTypeManager.FetchEntityTypeList(req.Context(), filter)
 	if err != nil { return nil, err }
 
-	countFilter := *filter
-	countFilter.Page, countFilter.PageSize = nil, nil
-	allList, err := container.EntityTypeManager.FetchEntityTypeList(req.Context(), &countFilter)
-	if err != nil { return nil, err }
-	total := len(allList)
+	total := len(list)
+	if paginated {
+		countFilter := *filter
+		countFilter.Page, countFilter.PageSize = nil, nil
+		allList, err := container.EntityTypeManager.FetchEntityTypeList(req.Context(), &countFilter)
+		if err != nil { return nil, err }
+		total = len(allList)
+	} else if total > 0 {
+		pageSize = total
+	}
```

### 3.4 DAO / 模型层

无需改动：`storage/rdb/{entity,entity_type}` 沿用既有约定——`filter.Page != nil && filter.PageSize != nil`
才写入 `Limit`（`[]uint{offset, size}`）。无参模式下两者均为 nil，即无 LIMIT。

## 4. 边界与细节

| 边界 | 处理 |
|---|---|
| `?page=` / `?page_size=`（空字符串） | 实测 `xreq.BindForm` 会把空串绑成 **0**（非 nil）；`OptionalPagination` 以原始 query 为空串判定「未传」→ 返回全量。单测固定 |
| 只传 `page`（无 `page_size`） | `paginated=true` → `page_size=20`（默认值）；避免"只给页码就拿全量"的歧义 |
| 只传 `page_size` | `paginated=true` → `page=1` |
| `page_size` 非法（0/负/>100） | clamp：`<1 → 20`、`>100 → 100`（两接口文档口径，非 422） |
| 空集合 | `list=[]`、`total=0`、`page_size=0`、`page=1` |
| 无界响应体 | 仅「显式不传分页」触发；文档提示大数据量走分页；如需硬上限（如 1000），另行产品裁定 |
| 与 `/api-keys` 的差异 | `/api-keys` 保持默认 20（严格接口）；在 `00-common` 例外清单中登记清楚 |

## 5. 测试

### 5.1 单元测试（`lib/xreq/pagination_test.go`）

| 用例 | 断言 |
|---|---|
| `OptionalPagination(空 query, nil, nil)` | `(1, 0, false)` |
| `OptionalPagination({"page":"2"}, P(2), nil)` | `(2, 20, true)` |
| `OptionalPagination({"page_size":"5"}, nil, P(5))` | `(1, 5, true)` |
| `OptionalPagination({"page":""}, P(0), nil)` | `(1, 0, false)`（空串算未传） |
| `OptionalPagination({"page":"0"}, P(0), nil)` | `(1, 20, true)`（显式非法 → clamp） |
| `OptionalPagination({"page_size":"101"}, nil, P(101))` | `(1, 100, true)` |

### 5.2 集成测试

**`tests/entity/list`**

| 用例 | 断言 |
|---|---|
| E-2-004（改写）无参返回全量 | `page=1`；`page_size == total`；`len(list) == total`（该包 fixture 至少 2 条） |
| E-2-005（改写）过滤 + 无分页参数 | `name=<ent1>` → `len(list)==1`、`total==1`、`page_size==1` |
| E-2-006（新增）无参不被 20 截断 | 造 25 个 Entity → `len(list)==25`、`page_size==25`；`?page=2&page_size=10` → 10 条、`page=2` |
| E-2-007（新增）clamp 保持 | `page_size=500` → `page_size=100` 且分页生效；`page=-1` → `page=1` |
| E-2-008（新增）空集合 | 新建一个 `type` 过滤（无 Entity）→ `list=[]`、`total=0`、`page_size=0` |

**`tests/entity_type/list`**

| 用例 | 断言 |
|---|---|
| ET-2-003（改写）无参返回全量 | `page=1`；`page_size == total`；`len(list) == total` |
| ET-2-004（改写）过滤 + 无分页参数 | `type_name=<t1>` → `len(list)==1`、`total==1` |
| ET-2-005（新增）无参不被 20 截断 | 造 25 个 Entity-Type → `len(list)==25`、`page_size==25` |
| ET-2-006（新增）clamp 保持 | `page_size=500` → `page_size=100`；`page=-1` → `page=1` |

> `tests/schema/openapi` 只校验 schema，不受影响；`tests/api_key/list` 的默认分页用例应保持通过。

### 5.3 回归面

`/api-keys`、`/providers`、`/operation-logs` 的无参/分页行为不变。

## 6. 文档同步

| 文件 | 改动 |
|---|---|
| `entities.md` §2.2 | `page`/`page_size`：未传且另一个也未传时返回全部匹配记录；`pagination`：`page=1`、`page_size=total`；clamp 规则保留 |
| `entity-types.md` §2.2 | 同上（`id`/`type_name`/`level` 过滤条件等价生效） |
| `00-common.md#通用Query参数` | 「登记例外」扩为 `/providers`、`/entities`、`/entity-types`（无参返回全部、`page_size=total`）；重申其它接口默认 20 |
| `接口层设计文档` §4.4.6 | 补「可选分页（显式例外）」：`OptionalPagination` 的用途（含空串归一）、`page_size=total` 自洽要求、例外登记 |
| 本目录 | 三份变更说明 |

## 7. 验证步骤

```bash
# 1) 单测
go test ./lib/xreq/ -run 'TestOptionalPagination|TestResolvePagination' -count=1 -v
go test ./...

# 2) 集成
go build -o ai-gateway-api.exe .
cd test/integration
AIAPI_FORCE_SQLITE=1 go test -count=1 -p 1 -run 'TestEntity_List|TestEntityType_List' \
  ./tests/entity/list/ ./tests/entity_type/list/ -v

# 3) 手工（各造 25 条）
curl -s 'http://127.0.0.1:8183/open-api/v1/entities'      | jq '.Data.pagination, (.Data.list|length)'
curl -s 'http://127.0.0.1:8183/open-api/v1/entity-types'  | jq '.Data.pagination, (.Data.list|length)'
#   期望 page=1、page_size=25、长度 25
curl -s 'http://127.0.0.1:8183/open-api/v1/api-keys'      | jq '.Data.pagination'
#   期望仍为 page=1、page_size=20
```

## 8. 备选方案

| 方案 | 结论 |
|---|---|
| A（采纳）两个接口无参 = 全部，`page_size=total` | 满足诉求、元数据自洽、与 `/providers` 一致 |
| B 只改 `/entities` | 前端 type 下拉/映射仍被截断，问题只解一半 |
| C 维持默认 20，改由前端/脚本翻页 | 违背诉求；多轮往返且数据变动时可能漏/重 |
| D 只把默认 `page_size` 提到 1000 | 仍非全量；只是把问题推后 |
| E 新增 `all=true` / `page_size=0` | 契约新增，需产品裁定并在全接口推广；本次不引入 |

## 9. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 无界响应（两个接口各一份） | 只在「未传分页参数」时触发；文档提示；后续可评估上限（需产品确认） |
| 误以为所有列表都不分页 | `00-common` 明确列出例外清单，其余接口保持默认 20 |
| 集成用例与旧断言冲突 | 明确列出需改写的用例（E-2-004/005、ET-2-003/004）并新增全量/clamp 用例 |
| `?page=` 空串被绑成 0 后误判为「显式分页」 | 已实测确认；`OptionalPagination` 依据原始 query 判空串，并有单测固定 |

回滚：把两个 handler 改回无条件下推 `ResolvePagination` 的三行即可恢复 #303 的默认分页行为。
## 10. 实施与验证记录（2026-11-04）

- 两个 handler 已改为 `OptionalPagination` 分支；`grep -n "ResolvePagination(" endpoints/openapi_v1/entity*/list.go`
  只剩「显式分页」分支内的调用；
- 实测（集成用例固定）：
  - 无参 `GET /entities`、`GET /entity-types` → `page=1`、`page_size=total`、`len(list)=total`；
  - 造 25 条后无参请求 `len(list)=25`（不再被 20 截断），`?page=2&page_size=10` 仍返回 10 条；
  - `?page_size=500` → `page_size=100`；`?page=-1` → `page=1`；`?page=`（空串）→ 全量；
  - 空集合 → `list=[]`、`total=0`、`page_size=0`、`page=1`；
- **实现中发现并一并修正**：两个接口此前在空集合时返回 `"list": null`（nil 切片未归一），
  与 `api-keys` 的处理不一致，也让「空集合元数据自洽」的断言无法成立；已补 `list = []*T{}` 归一；
- 单测：`lib/xreq` 新增 8 组 `OptionalPagination` 断言；全量单元 104 包 ok；
- 集成：SQLite 定向 2 包 PASS；MySQL 8.4.9 后端全量回归结果见 change-summary §9.5。
