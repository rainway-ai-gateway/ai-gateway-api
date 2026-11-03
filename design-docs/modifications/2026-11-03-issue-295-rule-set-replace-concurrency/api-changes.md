# 接口变更说明：#295 规则集合整体替换的并发冲突错误码

## 1. 结论

- **无新增/删除接口，无请求/响应字段变化**，三个 PUT 的成功语义（单事务整体替换、顺序即优先级）不变；
- 三个 PUT 新增一种错误响应 **`409 Conflict`**：并发整体替换取锁超时或瞬态锁冲突重试耗尽；
- 全局行为收紧（F-14）：`ErrNum=500 / Type=Database Exception` 的 `ErrMsg` 不再回填数据库驱动原文。

## 2. 受影响接口

| 方法 | 路径 | 接口定义 | 变更 |
|---|---|---|---|
| PUT | `/open-api/v1/ai-cache-rules` | `design-docs/api-define/OpenAPI接口定义/ai-cache-rules.md` §2.1 | 新增 409 |
| PUT | `/open-api/v1/ai-context-rules` | `.../ai-context-rules.md` 对应 PUT 小节 | 新增 409 |
| PUT | `/open-api/v1/traffic-mirror-rules` | `.../traffic-mirror-rules.md` 对应 PUT 小节 | 新增 409 |

GET 接口与其它资源不受影响。

## 3. 新增错误码：409 Conflict

| 项 | 值 |
|---|---|
| `ErrNum` | `409` |
| `ErrMsg`（英文默认） | 取锁超时：`Conflict: collection is being replaced by another request, retry later`<br>重试耗尽：`Conflict: collection is being replaced concurrently, retry later` |
| 触发条件 | ① 同一集合正被另一请求整体替换，10s 内未取到命名锁；② 事务遇到 MySQL 1213/1205 等瞬态锁冲突，3 次尝试后仍失败 |
| 语义 | **可重试**：客户端按指数退避重试即可；不代表数据不一致（事务已整体回滚，集合保持原状） |
| 响应体示例 | 见下 |

```json
{
    "ErrNum": 409,
    "ErrMsg": "Conflict: collection is being replaced by another request, retry later",
    "WorkMode": "release"
}
```

设计要点：

- 此前该场景返回 `500 Database Exception: Error 1213 ...`；改为 409 后满足「已知冲突不得 500」
  （`test/integration/doc/api-test-reorg/openapi-interface-testing-standard.md` 500 语义条款）；
- 文案不含表名、SQL、驱动错误、DSN（F-14）；
- 409 与既有 `409 Conflict` 用法一致（`lib/xerror/resolve.go:82-84` 已映射 `etConflict`），
  未引入新错误码段。

## 4. 500 文案收紧（F-14，全局）

| 场景 | 变更前 `ErrMsg` | 变更后 `ErrMsg` |
|---|---|---|
| DAO/驱动错误 | `Database Exception: Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction` | `Database Exception: database operation failed` |

- `ErrNum=500` 与 `Type=Database Exception` 保持不变（前端/脚本按码分支不受影响）；
- 驱动原文、SQL 与堆栈仍写日志（`ResolveResult.FullMsg()`），排障能力不变；
- 该变更作用于所有「DAO 错误 → 500」的接口，不只是本 issue 的三个接口；`test/integration` 中唯一相关断言
  （`tests/report/query/clickhouse_test.go:187`）匹配的是 `Type` 前缀，仍然通过。

## 5. i18n（`conf/i18n/zh.toml`）

新增/补充映射：

```toml
"Conflict" = "冲突"

"^collection is being replaced by another request, retry later$" = "规则集合正在被其他请求整体替换，请稍后重试"
"^collection is being replaced concurrently, retry later$"       = "规则集合并发更新冲突，请稍后重试"
"^database operation failed$"                                    = "数据库操作失败"
```

> `zh.toml` 目前没有 `Conflict` 的 Type 映射（`stateful/i18n.go:130-135` 命中不到即原样返回），
> 本 issue 顺带补齐，避免中文环境下出现 `Conflict: ...` 英文类型前缀。

## 6. 兼容性

| 方面 | 评估 |
|---|---|
| 成功响应结构 | 不变（`rules` 数组，无 `created_at`/`updated_at`） |
| 错误码 | 新增 409；422（校验失败）与 404（不存在资源）语义不变 |
| 客户端处理 | 原本需对 500 做重试；现在可对 409 做有界重试 + 退避，行为更可预期 |
| 数据面 | 无影响：失败整体回滚，`bfe`/`conf-agent` 读到的配置不会出现中间态 |
| 数据库 | 无 DDL 变更 |

## 7. api-define 文档同步点

| 文件 | 修改位置 | 内容 |
|---|---|---|
| `ai-cache-rules.md` | §2.1 执行逻辑第 3 步之后、`约束` 段 | 补一句：「整体替换按资源串行执行；若同一集合正被其他请求替换或遇瞬态锁冲突，接口在有限重试后返回 `409 Conflict`（可重试），集合保持原状，绝不返回 500 或驱动错误原文」 |
| 同上 | 成功/失败示例 | 增补 409 响应示例 |
| `ai-context-rules.md`、`traffic-mirror-rules.md` | 各自 PUT 小节「执行逻辑 / 约束」 | 同上，措辞一致 |
| `design-docs/sys-design/模型层设计文档.md` | 集合资源写入章节 | 新增「整组替换的并发写保护」：命名锁（MySQL）+ 有界重试 + 409 语义，并说明 SQLite 单写者无需该锁 |

## 8. 调用方建议

1. 保存规则集合时对 `409` 做**有界重试 + 随机退避**（例如 3 次，100ms/300ms/900ms），不要立即放弃；
2. 多管理员/自动化同时保存同一集合属于可预期场景，服务端已串行化，客户端只需正确处理 409；
3. 不要依赖 500 的 `ErrMsg` 文本做分支判断 —— 已脱敏且可能随版本变化，请使用 `ErrNum`。