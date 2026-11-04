# 接口变更说明：`/entities`、`/entity-types` 列表恢复「无参返回全量」

## 1. 结论

- **无接口新增/删除，无请求字段与响应结构变化**；
- `GET /open-api/v1/entities` 与 `GET /open-api/v1/entity-types` 在**不传 `page` / `page_size`**
  （含空串 `?page=`）时恢复为「返回全部匹配记录」，响应 `pagination.page=1`、`page_size=total`
  （`total=0` 时为 0）；
- 显式传分页参数时行为不变（分页 + clamp + `total`=过滤后总条数）；
- `/api-keys` 保持 #303 之后的默认分页 1/20；`/providers` 不变。

## 2. 行为对照

| 请求 | #303 之后（当前） | 本次变更后 |
|---|---|---|
| `GET /entities`（无参） | `list`=前 20 条，`page=1, page_size=20` | **`list`=全部，`page=1, page_size=total`** |
| `GET /entity-types`（无参） | `list`=前 20 条，`page=1, page_size=20` | **`list`=全部，`page=1, page_size=total`** |
| `GET /entities?type=<t>`（无分页参数） | 前 20 条匹配项，`page_size=20` | **全部匹配项，`page=1, page_size=total`** |
| `GET /entity-types?level=2`（无分页参数） | 前 20 条匹配项 | **全部匹配项** |
| `GET …?page=`（空值） | 前 20 条（空串被绑成 0 后 clamp） | **全部匹配记录**（空串按未传处理） |
| `GET …?page=1&page_size=10` | 10 条，`page=1, page_size=10` | 不变 |
| `GET …?page=2&page_size=10` | 第 2 页，`page=2` | 不变 |
| `GET …?page_size=500` | 100 条，`page_size=100` | 不变（clamp） |
| `GET …?page=-1` | 前 20 条，`page=1` | 不变（clamp） |
| 空集合（无参） | `"list": null`、`page_size=20` | **`"list": []`**、`total=0`、`page_size=0`、`page=1`（顺带修正 nil→空切片，与 `/api-keys` 一致） |
| `GET /api-keys`（无参） | `page=1, page_size=20` | 不变 |
| `GET /providers`（无参） | `page=1, page_size=total` | 不变 |

（表中 `…` 同时代表 `/entities` 与 `/entity-types`。）

响应示例（无参，共 25 条）：

```json
{
  "ErrNum": 200,
  "ErrMsg": "success",
  "Data": {
    "list": [ /* ... 25 条 ... */ ],
    "pagination": { "page": 1, "page_size": 25, "total": 25 }
  }
}
```

## 3. `pagination` 语义（两个接口一致）

| 字段 | 无参模式 | 分页模式 |
|---|---|---|
| `page` | 固定 1 | 生效页码（`<=0` clamp 到 1） |
| `page_size` | `total`（表示本次已返回全量；空集合为 0） | 生效页大小（`<1 → 20`、`>100 → 100`） |
| `total` | `len(list)` = 匹配条数 | 满足过滤条件的总条数（与分页无关） |

> 无参模式的 `page_size=total` 与 `/providers` 一致：表示「这次一口气返回了全部」，
> 而不是「每页 20 条但只给了 1 页」。调用方据此可直接判断 `list` 是否已完整。

## 4. 兼容性

| 调用方 | 影响 |
|---|---|
| `ai-gateway-web` 的 4 处无参 `GET /entities`（Entity 列表页主数据、API-Key 编辑页 Entity 下拉、RouteTable/Clusters 名称映射） | **恢复可用**（重新拿到全量 Entity） |
| `ai-gateway-web` 的 3 处无参 `GET /entity-types`（Entity-Type 列表页主数据、Entity 列表页 type 映射、Entity 编辑页 type 下拉） | **恢复可用**（重新拿到全量 Entity-Type） |
| 已显式传分页参数的调用方 | 无影响 |
| 按 `page_size=20` 硬编码判断「是否还有下一页」的调用方 | 无参场景下 `page_size` 变为 `total`，请改用 `total` 与 `len(list)` 判断 |
| 依赖 `page=0` 的调用方（#303 之前的老实现） | 无兼容问题：无参一律 `page=1` |
| `/api-keys` 调用方（含 `Clusters/index.vue:441` 的无参调用） | 无变化：仍默认 20；需要全量 Key 需另立修订或改用翻页 |

## 5. 性能与容量提示

- 无参调用会一次性返回全部匹配记录，响应体大小随记录数线性增长；记录较多的部署建议调用方显式分页
  （`page`/`page_size`）；
- 服务端不做隐式截断（否则违背「完整列表」语义）；如需硬上限（例如 1000 条），应由产品裁定，
  届时同步修改本文与 `entities.md`、`entity-types.md`。

## 6. 文档同步点

| 文件 | 改动 |
|---|---|
| `entities.md` §2.2 | `page`/`page_size`：未传且另一个也未传时返回全部匹配记录；`pagination`：`page=1`、`page_size=total`；clamp 规则保留 |
| `entity-types.md` §2.2 | 同上 |
| `00-common.md#通用Query参数` | 「登记例外」扩为 `/providers`、`/entities`、`/entity-types`；重申其它接口默认 20 |
| `接口层设计文档` §4.4.6 | 补「可选分页（显式例外）」：`OptionalPagination` 的用途（含空串归一）、`page_size=total` 自洽要求、例外登记 |
| `providers.md` §2.2 | 不变（可交叉引用两个 Entity 列表，三者同语义） |

## 7. 验收对照

| 验收判据 | 落点 |
|---|---|
| 两个接口无参返回完整列表 | 无参不下推分页 + `page_size=total`；E-2-004 / ET-2-003 与新增 E-2-006 / ET-2-005（25 条不被截断）覆盖 |
| 过滤 + 无分页参数返回全部匹配 | E-2-005、ET-2-004 覆盖 |
| 显式分页 / clamp 不回归 | E-2-003、E-2-007、ET-2-002、ET-2-006 覆盖 |
| 空集合元数据自洽 | E-2-008 覆盖 |
| 其它列表接口不回归 | `tests/api_key/list` 等既有用例 |