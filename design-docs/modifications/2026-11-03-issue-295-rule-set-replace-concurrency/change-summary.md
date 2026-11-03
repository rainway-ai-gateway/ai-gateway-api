# Issue #295：规则集合全量替换在并发下把 InnoDB 死锁上抛为 500（泄露驱动错误）

## 1. 问题来源

[rainway-ai-gateway/ai-gateway-api#295](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/295)

> 20 个并发请求分别调用 `PUT /open-api/v1/ai-cache-rules`（及同构的 `/ai-context-rules`、`/traffic-mirror-rules`）
> 全量替换规则集合时，服务端把 InnoDB 死锁（Error 1213）直接上抛为
> `ErrNum=500 Database Exception: Error 1213 (40001): Deadlock found when trying to get lock`，
> 违反「单事务整体替换、事务失败整体回滚」的失败处理语义与 500 卫生要求。

## 2. 根因

### 2.1 调用链（源码直读）

```
PUT /open-api/v1/ai-cache-rules
  endpoints/openapi_v1/ai_cache/endpoints.go:81   container.AICacheManager.SetAICacheRules
    model/ai_cache/ai_cache_manager.go:130        m.txn.AtomExecute(...)            ← 单事务
      storage/rdb/ai_cache/ai_cache.go:62         ReplaceAll
        storage/rdb/internal/dao/table_ai_cache_rules.go:115  TAICacheRuleReplaceAll
          内部：internal.Delete(全表) + TAICacheRuleCreate(逐条 INSERT)
```

同构实现：

| 模块 | manager | storager | DAO |
|---|---|---|---|
| ai_cache | `model/ai_cache/ai_cache_manager.go:130` | `storage/rdb/ai_cache/ai_cache.go:62` | `table_ai_cache_rules.go:115` |
| ai_context | `model/ai_context/ai_context_manager.go:126` | `storage/rdb/ai_context/ai_context.go:62` | `table_ai_context_rules.go:109` |
| traffic_mirror | `model/traffic_mirror/traffic_mirror_manager.go:135` | `storage/rdb/traffic_mirror/traffic_mirror.go:68` | `table_traffic_mirror_rules.go:127` |

### 2.2 死锁机理

InnoDB `REPEATABLE READ` 下，`DELETE FROM <rules>`（无 WHERE，全表）会对扫描到的记录与索引间隙加
next-key 锁；随后的 `INSERT` 又需要在已删除记录所在间隙上取插入意向锁。N 个并发事务同时执行
「删全表 + 按序插入」，其中一方持有 A 间隙锁、等待 B 记录锁，另一方相反 → InnoDB 死锁检测介入，
牺牲一方并抛出 `Error 1213 (40001)`。这不是脏数据或构造问题，而是**写模式与并发度**的固有冲突。

### 2.3 为什么变成 500 + 驱动原文

```
internal/curd.go:141    xerror.WrapDaoError(err)      // Exec 失败即打 etDao 标记
lib/xerror/resolve.go:56-57  ErrNo=500, Msg = errors.Cause(err)  // ← 驱动原文直接成为 Msg
lib/xreq/result.go:143-146   ErrMsg = "Database Exception" + ": " + Msg
```

全仓无任何 1213/1205 重试或串行化（`grep -rn "deadlock|1213|retry" lib storage model/shared` 无命中），
`storage/rdb/txn/txn.go:36 AtomExecute` 也只是 `Begin → handler → commitOrRollback`，失败即上抛。

### 2.4 为什么同类资源没暴露

`intent_config` 是单行 upsert（`storage/rdb/iintent_config/iintent_config.go:62`，`INSERT ... ON DUPLICATE KEY`），
没有「删全表」这一步，因此同样的并发不产生间隙锁冲突；`batch` 走 `GET_LOCK` 串行化
（`model/ibatch/job.go:126-141`）。即：**只有「整组替换」这三个集合资源缺少并发写保护**。

## 3. 复现与环境说明

2026-11-03 本机环境读取结果：

- 仓库 HEAD `9e64e68`（含 issue #293 修复），Go 1.25.2 / windows-amd64；
- 本机已安装 MySQL 8.4（`mysqld.exe` 在 PATH），但**当前 3306 未启动**（`mysql -h 127.0.0.1 -u root` 返回
  `ERROR 2003 ... (10061)`），因此 issue 中的 `-tags mysql` 并发用例本轮未能直接复跑；
- 复现前提与启动方式见 `environment/mysql-installation.md`：`net start MySQL84`（或手工 `mysqld --defaults-file=...`），
  然后 `AIAPI_MYSQL_DSN='root@tcp(127.0.0.1:3306)/'`。

issue 记录的 2/20 worker 失败与三模块独立复现，配合上述源码链路，足以认定 `FAIL_PRODUCT`；
本方案不依赖复跑即可论证，但实施阶段必须按 §5 的判据在 MySQL 上验证一次（见 `design-changes.md §9`）。

## 4. 目标

1. 三个集合资源的并发全量替换不再出现 5xx：20 并发 PUT 全部 2xx（或明确的 4xx 业务冲突，绝不 500）；
2. `ErrMsg` 不出现驱动错误原文（`Error 1213` / `Deadlock` / `Database Exception: ...`）；
3. 单事务整体替换语义不回归：校验失败仍 422 且集合保持原状；最终读回集合等于某一次提交；
4. 不改动请求/响应数据结构，不新增接口；仅新增「并发冲突」这一类错误码（409）；
5. 保持 SQLite / 单实例部署行为不变（SQLite 单写者，无需额外串行化）。

## 5. 方案概览（三层，自下而上是主要手段）

### L1 按资源串行化（消除死锁，主手段）

新增 `lib/xlock`：MySQL 用命名锁 `GET_LOCK(name, timeout)` 在**专用连接**上串行化同一集合的整体替换；
非 MySQL 驱动（SQLite / sqlite-strip）退化为 no-op。三个 `ReplaceAll` 在 delete-all 之前取锁：

```go
release, err := xlock.Acquire(ctx, dbCtx.Conn(), "ai_gateway:ai_cache_rules:replace", 10*time.Second)
if err != nil {
    return err                       // 取锁超时 → 409（不 500）
}
defer release()
```

特性：跨 api 副本有效（命名锁是 MySQL 会话级全局互斥）、无需 DDL、进程崩溃时连接断开即自动释放。

### L2 有界重试（纵深防御）

`lib/xdb.go` 新增 `TransientLockError(err)`（1213 / 1205 / SQLite busy，沿用既有 `DuplicateEntryError`
的字符串判据风格），`model/itxn` 新增 `ExecuteWithRetry(ctx, txn, fn)`：对可重试错误做 3 次尝试、
20ms 起步指数退避 + 抖动；每次重试重新开启事务。耗尽后抛 409 业务冲突。仅在三个 manager 的替换事务上使用
（**不**下沉到通用 `AtomExecute`，避免重跑带非 DB 副作用的闭包）。

### L3 错误契约与 F-14 收敛

- 三个 PUT 接口新增 `409 Conflict`：语义为「该规则集合正被另一请求整体替换 / 并发冲突重试耗尽」，
  文案不含驱动原文；
- `lib/xerror/resolve.go`：`etDao` 的 `Msg` 不再回填 `errors.Cause(err)` 的驱动原文，改为固定文案
  （详情仍通过 `ResolveResult.FullMsg()` 进日志），从根上满足 F-14；
- 顺带把 `ai_cache_semantic_settings` / `ai_context_settings` 的单行替换纳入同一 helper（可选，见
  `design-changes.md §2.3`）。

详细设计、补丁、边界与测试矩阵见 [design-changes.md](./design-changes.md)；
错误码与接口行为对照见 [api-changes.md](./api-changes.md)。

## 6. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api`（数据面 `bfe`、`conf-agent` 不受影响：写失败整体回滚，导出配置不变） |
| 代码 | 新增 `lib/xlock`；`storage/rdb/{ai_cache,ai_context,traffic_mirror}` 的 `ReplaceAll`；`lib/xdb.go` 判据；`model/itxn` 重试 helper；三个 manager；`lib/xerror/resolve.go`；`conf/i18n/zh.toml` |
| 文档 | `design-docs/api-define/OpenAPI接口定义/{ai-cache-rules,ai-context-rules,traffic-mirror-rules}.md` 增 409 语义；`sys-design/模型层设计文档.md` 增「集合整体替换的并发写保护」；本目录 |
| 数据库 | 无 DDL 变更（命名锁不落表） |
| 测试 | 新增 `lib/xlock`、`lib/xdb`、`model/shared` 单测；三个 manager 单测补「重试/冲突」用例；三个集成并发用例补 `ErrMsg` 断言 |
| 兼容性 | 新增 409 属新增错误响应（此前该场景返回 500）；成功路径 200 与数据结构不变 |

## 7. 风险

| 风险 | 评估 | 缓解 |
|------|------|------|
| 命名锁泄漏（会话级锁随连接回池） | 中 | 取锁用专用 `*sql.Conn`；释放失败时用 `conn.Raw` 返回 `driver.ErrBadConn` 丢弃物理连接，连接关闭即释放锁 |
| 串行化降低写吞吐 | 低 | 管理面配置写，QPS 极低；串行段只含 delete-all + 少量 insert |
| 每请求多占一条连接 | 低 | 主库 `MaxOpenConns=500`（`conf/ai_gateway_api.toml:59`），最坏 +20 连接 |
| 重试重跑闭包带来重复副作用 | 中 | 仅对三个「纯 DB delete+insert」闭包启用；契约写入 `ExecuteReplaceTx` 注释与设计文档 |
| 409 引入新的客户端分支 | 低 | 文案明确「重试即可」；客户端原本对 500 也需重试 |
| 全局 DAO 文案脱敏影响排障 | 低 | `FullMsg()` 仍记录完整驱动错误与堆栈，仅响应体脱敏 |

## 8. 上线前检查清单

1. `go test ./lib/... ./model/ai_cache/ ./model/ai_context/ ./model/traffic_mirror/ -count=1` 通过；
2. `make test-model-cover-gate` 通过（model 覆盖率 ≥ 70%）；
3. `net start MySQL84` 后：
   `AIAPI_MYSQL_DSN='root@tcp(127.0.0.1:3306)/' go test -tags mysql -count=1 -v ./tests/ai_cache/update/ ./tests/ai_context/update/ ./tests/traffic_mirror/update/`
   —— AC-1-012 / CTX-1-005 / TM-1-015 全部 PASS（20 并发全 2xx）；
4. 单并发回归：三个模块既有 PUT 用例全通过（校验失败 422、集合保持原状）；
5. `AIAPI_FORCE_SQLITE=1` 下三个模块用例全通过（确认 no-op 分支无回归）；
6. 运行后 `SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'ai_gateway_it_%'` 无残留。

## 9. 实施记录（Step 5 / Step 6）

### 9.1 代码改动

| 文件 | 改动 |
|------|------|
| `lib/xlock/xlock.go`（新增） | `Acquire(ctx, db, name, timeout)`：MySQL 用 `GET_LOCK` 在专用连接上取命名锁；非 MySQL 驱动 no-op；取锁超时 → `409 Conflict`；`RELEASE_LOCK` 失败时 `conn.Raw` 返回 `driver.ErrBadConn` 丢弃物理连接 |
| `lib/xdb.go` | 新增 `TransientLockError`（1213 / 1205 / deadlock / lock wait timeout / SQLite busy） |
| `model/itxn/retry.go`（新增） | `ExecuteWithRetry`：3 次尝试、20ms 起步指数退避 + 抖动；耗尽后返回 409 业务冲突；非瞬态错误不重试 |
| `storage/rdb/{ai_cache,ai_context,traffic_mirror}` | `ReplaceAll` 在 delete-all 前取命名锁（名称 `ai_gateway:replace:<resource>`，超时 10s） |
| `model/{ai_cache,ai_context,traffic_mirror}` manager | 整组替换事务改用 `itxn.ExecuteWithRetry` |
| `lib/xerror/resolve.go` | `etDao` 的 `Msg` 固定为 `database operation failed`（F-14），详情留 `FullMsg()` |
| `conf/i18n/zh.toml` | 补 `Conflict` 类型映射与三条新文案映射 |
| `test/integration/testutil/assert.go` | 新增 `LeaksDriverError` |
| 三个并发用例 | 补 `ErrMsg` 不泄露驱动原文的断言；`traffic_mirror` setup 缺陷修复（provider 契约字段 + 断言业务码 + 用本实例 client） |
| `test/integration/tests/ai_cache/update/replace_conflict_mysql_test.go`（新增，AC-1-019）、`tests/ai_context/update/replace_conflict_mysql_test.go`（新增，CTX-1-006） | 外部会话占住命名锁 → PUT 返回 409 且无驱动原文；释放后同一请求 200 |

单测新增：`lib/xlock/xlock_test.go`（5 例）、`model/itxn/retry_test.go`（5 例）、`lib/xerror/resolve_test.go`（3 例）、
`lib/xdb_test.go` 的 `TestTransientLockError`、`model/ai_cache` 的「重试后 409 且只记一条失败日志」用例。

### 9.2 文档改动

`design-docs/api-define/OpenAPI接口定义/{ai-cache-rules,ai-context-rules,traffic-mirror-rules}.md`（执行逻辑 + 约束 + 409 示例）、
`design-docs/sys-design/模型层设计文档.md`（新增 §3.3.1 整组替换的并发写保护）、本目录三份文档。

### 9.3 验证结果（2026-11-03，Go 1.25.2 / windows-amd64；MySQL 8.4.9 本机）

| 验证项 | 结果 |
|--------|------|
| 主模块单测 `go test ./...` | 103 个包 `ok`，0 FAIL |
| `lib` + `model` 全量 | 36 个包 `ok` |
| MySQL 并发用例（AC-1-012 / CTX-1-005 / TM-1-015） | 3/3 PASS（20 并发全 2xx，最终集合等于某一次提交） |
| MySQL 409 端到端 AC-1-019 / CTX-1-006（新增） | PASS：外部占锁时 PUT 返回 `409 Conflict`（文案 `collection "ai_gateway:replace:ai_cache_rules" is being replaced by another request, retry later`），释放后同一请求 200 —— 同时证明命名锁在真实请求路径上确实生效，而非 no-op |
| MySQL 三模块全量用例 | 6/6 包 `ok` |
| SQLite 回归（三模块 get/update） | 6/6 包 `ok` |

### 9.4 复现结论（重要）

issue 记录的 InnoDB 死锁在本机 **未能复现**，即便临时移除取锁与重试：

- 20 并发 PUT（空集合）：全部 200；
- 先灌 300 条规则再 20 并发替换（临时探针，验证后删除）：全部 200；
- 本机 MySQL 8.4.9，issue 环境为 8.4.11 —— 版本差异（InnoDB 间隙锁/死锁检测行为调整）是可能原因，未进一步定论。

因此本次修复的正当性来自「issue 的三模块 2/2 复现证据 + 源码链路（delete-all+insert-all 无任何并发保护）」，
而不是本地复现；本地可验证的部分（命名锁生效、409 语义、文案脱敏、SQLite no-op、既有用例不回归）均已覆盖。
实施中确实发现并修复了一个可复现的真实缺陷：`traffic_mirror` 并发用例的前置 provider 创建缺契约字段且未断言业务码
（`Param Illegal: provider xxx not exist` 被吞掉，后续 cluster 创建失败伪装成并发缺陷）。

### 9.5 沉淀判断（Step 6）

不新增 `sys-design/details/` 文档：机制本身体量小（约 60 行 helper + 3 处 4 行调用），已完整写入
《模型层设计文档》§3.3.1，并与 api-define 的 409 语义互相引用；`details/` 目录定位是配额同步、层级合并、
导出与版本控制这类复杂机制，再拆一份会造成双份维护。`sys-design/summary.md` 索引无需变更。

### 9.6 遗留

1. 多副本场景下若同一集合被多个副本并发替换，命名锁仍保证串行（跨会话全局），无需额外处理；
   但 `MaxOpenConns` 配置很小（< 2×并发）时，每请求额外占一条锁连接可能排队——已评估无死锁环，
   仅表现为等待，必要时可把取锁点提到 manager 以复用事务连接（design-changes §2.5 方案 B）；
2. `ai_cache_semantic_settings` / `ai_context_settings` 的单行替换未纳入本次改造（可选同批项，见 design-changes §2.6）。

## 10. 参考

- [Issue #295](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/295)
- `design-docs/api-define/OpenAPI接口定义/ai-cache-rules.md`（§2.1 执行逻辑 3「单事务内整体替换……」）
- `test/integration/doc/api-test-reorg/openapi-interface-testing-standard.md`（500 语义、F-14 错误消息卫生）
- `storage/rdb/internal/dao/internal/curd.go:123-141`（DAO wrap）、`lib/xerror/resolve.go:50-106`、`lib/xreq/result.go:137-150`
- `model/ibatch/job.go:196-207`（既有 `GET_LOCK` 用法）、`storage/rdb/internal/dao/table_api_key_id_seq.go:138`（`isMySQL`）
- `test/integration/tests/{ai_cache,ai_context,traffic_mirror}/update/concurrency_mysql_test.go`（AC-1-012 / CTX-1-005 / TM-1-015）
- `environment/mysql-installation.md`（本机 MySQL 8.4 启动方式）