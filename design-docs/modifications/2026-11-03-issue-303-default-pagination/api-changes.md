# 接口变更说明：#303 列表接口默认分页

## 1. 结论

- **无接口新增/删除，无请求字段与响应结构变化**（`list` / `pagination` 结构不变）；
- 行为变化：**不传 `page`/`page_size` 的列表请求，返回默认第 1 页、每页 20 条**，
  响应 `pagination.page=1`、`page_size=20`、`total` 为匹配条数；此前该路径返回**全量数据**且
  `pagination.page=0`、`page_size=0`；
- `pagination.total` 的口径明确为「满足查询过滤条件的总条数」（此前 `api-keys` 与 `entities`
  的 total 忽略了部分过滤条件）；
- 非法值语义不变：`api-keys` 仍 422；`entities`/`entity-types` 仍按默认值 clamp；
- `providers` 保持文档化例外（无参返回全部、`page_size=total`），本次不改。

## 2. 受影响接口与行为对照

| 接口 | 场景 | 变更前 | 变更后 |
|---|---|---|---|
| `GET /open-api/v1/api-keys` | 无参 | `list`=全部，`page=0, page_size=0` | `list`=前 20 条，`page=1, page_size=20` |
| 同上 | `page=2&page_size=1` | 第 2 条，`page=2, page_size=1` | 不变 |
| 同上 | `enabled=false`（无分页参数） | 全部匹配项，`page=0/page_size=0` | 前 20 匹配项，`page=1/page_size=20`，`total`=匹配条数 |
| 同上 | `page=0`、`page=-1`、`page_size=0`、`page_size=101` | 422 | 422（文案不变） |
| `GET /open-api/v1/entities` | 无参 | 全部，`page=0/page_size=0` | 前 20 条，`page=1/page_size=20` |
| 同上 | `page_size=500` | 100 条，`page_size=100` | 不变（clamp） |
| 同上 | `page=-1` | 全部（分页参数被忽略，响应 `page=0`） | 前 20 条，`page=1`（clamp） |
| `GET /open-api/v1/entity-types` | 无参 / 非法值 | 同 entities | 同 entities |
| `GET /open-api/v1/providers` | 无参 | 全部，`page=1, page_size=total` | **不变**（providers.md 明文例外） |

响应示例（无参）：

```json
{
  "ErrNum": 200,
  "ErrMsg": "success",
  "Data": {
    "list": [ /* ... */ ],
    "pagination": { "page": 1, "page_size": 20, "total": 37 }
  }
}
```

## 3. `pagination.total` 语义

| 项 | 定义 |
|---|---|
| 含义 | 满足本次查询**过滤条件**、与分页无关的总条数 |
| 用途 | 计算总页数 `ceil(total / page_size)`、判断是否需要下一页 |
| 变更点 | `api-keys` 的 total 之前只按 `ProductName` 统计（忽略 `enabled`/`entity_id`/`unlimited_quota`）；`entities` 忽略 `id`/`name`/`route_rules_id`。修复后与过滤条件一致 |

## 4. 兼容性与调用方影响

| 调用方 | 影响 | 建议 |
|---|---|---|
| `ai-gateway-web/src/modules/Clusters/index.vue:441`（无参拉 `api-keys` 构建 id→name 映射） | **受影响**：默认分页后只能拿到前 20 条 | 同版本改造：循环翻页拉全量（`page_size=100` + 按 `total` 翻页），或接受上限并显式传 `page_size` |
| 已显式传分页参数的客户端 | 无影响 | — |
| 脚本 / 集成（无参依赖全量） | 受影响：数据被截断为 20 条 | 显式传 `page_size`（≤100）或翻页 |
| 依赖 `pagination.page=0` 做特殊判断的调用方 | 属于依赖缺陷行为 | 应删除该分支；正常分页应从 1 开始 |
| 依赖 422 文案的调用方 | 无变化（文案逐字保留） | — |

## 5. 文档同步点

| 文件 | 改动 |
|---|---|
| `00-common.md#通用Query参数` | 说明「默认值/截断是通用基线，显式非法值的处理按各接口 Query 参数表」；登记 `provider` 例外（无参全部、上限 1000） |
| `api-keys.md` §2.2 | `pagination` 说明补 `total` = 满足过滤条件的总条数 |
| `entities.md` / `entity-types.md` | 同上补 `total` 定义（并把「无参返回全部」的旧表述如有则删除） |
| `providers.md` §2.2 | 保持例外描述不动，可在 `00-common` 交叉引用 |

## 6. 验收对照

| Issue 验收判据 | 本方案 |
|---|---|
| 1. 无参 `GET /api-keys` → `page=1`、`page_size=20`、`total`=全量 | 默认值无条件回填 + 分页下推；`list` 同时被截断为 20 |
| 2. `page=1&page_size=1` 与 `page=2&page_size=1` 返回相邻页、不重复 | 分页偏移由 DAO 计算（`offset=(page-1)*page_size`），既有逻辑不变；新增集成用例覆盖 |
| 3. 非法值仍 4xx/422，不因默认回填放宽 | `ValidatePagination` 保留逐字文案，api-keys 路径不变 |