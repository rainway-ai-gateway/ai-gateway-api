# 设计变更：#295 规则集合全量替换的并发写保护

## 1. 现状与缺陷边界

### 1.1 写模式

三个集合资源共用同一写模式（`storage/rdb/*/` + `storage/rdb/internal/dao/table_*_rules.go`）：

```sql
-- 同一事务内
DELETE FROM <rules>;                     -- 无 WHERE，全表
INSERT INTO <rules> (...) VALUES (...);  -- 按数组顺序逐条
```

语义要求（`design-docs/api-define/OpenAPI接口定义/ai-cache-rules.md` §2.1 执行逻辑 3）：

> 单事务内整体替换规则集合（先删除全部旧规则，再按数组顺序写入）；任一校验失败或事务失败则整体回滚，集合保持原状。

### 1.2 缺陷边界

| 事实 | 结论 |
|---|---|
| 死锁只发生在「删全表 + 插入」并发时 | 单并发、校验失败路径正常 |
| `intent_config`（单行 upsert）、`batch`（`GET_LOCK` 串行化）同批并发用例通过 | 缺少保护的只有这三个「整组替换」资源 |
| 事务失败已回滚，最终一致性断言通过 | 无数据损坏，缺陷是**失败处理语义 + 错误卫生** |
| 三个模块各自 2/2 复现 | 同根因，不是测试构造问题 |

因此修复目标不是「让 SQL 不死锁」，而是「**并发写必须串行化到一个可预期的结果，且失败不得以 500 + 驱动原文上抛**」。

## 2. 方案

### 2.1 L1：按资源串行化（主手段，消除死锁）

新增 `lib/xlock`，MySQL 上用命名锁（`GET_LOCK`）串行化同一集合的整体替换；其它驱动 no-op。

**为什么用命名锁而不是表/行锁**：

| 方案 | 结论 |
|---|---|
| `GET_LOCK(name, timeout)`（采纳） | 跨副本有效、无需 DDL、无「行的存在性」依赖、会话结束自动释放；仓库已有先例（`model/ibatch/job.go:196`） |
| 哨兵行 `SELECT ... FOR UPDATE` | 需新建锁表 + 种子行；空表/首启竞态要 upsert 兜底（upsert 自身也会产生插入意向锁）；且把「规则集合写」与 settings 行耦合 |
| `LOCK TABLES` | 隐式提交，破坏单事务整体替换语义 |
| 进程内 `sync.Mutex` | 只在单副本内有效，多副本仍死锁，无法作为主手段 |
| 隔离级降为 READ COMMITTED | 减少间隙锁但不消除死锁，且改变全局事务语义 |

**代码（新增 `lib/xlock/xlock.go`）**

```go
// Package xlock serializes whole-collection replacements across api replicas.
// MySQL exposes named locks (GET_LOCK); SQLite serializes writers itself, so the
// helper degrades to a no-op there.
package xlock

// supportsNamedLock reports whether db speaks the MySQL named-lock protocol.
// Overridden in tests.
var supportsNamedLock = func(db *sql.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Driver().(*mysql.MySQLDriver)
	return ok
}

// Acquire takes the session-scoped named lock `name`. The returned release is
// never nil. A lock that cannot be taken within timeout yields a 409 conflict
// business error (never a 500); driver failures are wrapped as DAO errors.
func Acquire(ctx context.Context, db *sql.DB, name string, timeout time.Duration) (func(), error) {
	if !supportsNamedLock(db) {
		return func() {}, nil
	}

	conn, err := db.Conn(ctx) // dedicated: a named lock is bound to its session
	if err != nil {
		return nil, xerror.WrapDaoError(err)
	}

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, int(timeout/time.Second)).Scan(&got); err != nil {
		conn.Close()
		return nil, xerror.WrapDaoError(err)
	}
	if !got.Valid || got.Int64 != 1 {
		conn.Close()
		return nil, xerror.WrapConflictErrorWithMsg(
			"collection %q is being replaced by another request, retry later", name)
	}

	return func() {
		if _, err := conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", name); err != nil {
			// The lock lives as long as its session: discarding the physical
			// connection is what actually frees it. Returning it to the pool
			// would strand the lock until the connection is recycled.
			_ = conn.Raw(func(interface{}) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}
```

**调用点（三个 storager，各 4 行）**

```go
// storage/rdb/ai_cache/ai_cache.go:62
const replaceLockName = "ai_gateway:replace:ai_cache_rules"

func (s *AICacheStorager) ReplaceAll(ctx context.Context, rules []*ai_cache.AICacheRuleParam) error {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	// delete-all + insert-all takes gap/next-key locks on the whole table; two
	// concurrent replacements deadlock under InnoDB REPEATABLE READ (issue #295).
	release, err := xlock.Acquire(ctx, dbCtx.Conn(), replaceLockName, replaceLockTimeout)
	if err != nil {
		return err
	}
	defer release()

	// ... 原 dao.TAICacheRuleReplaceAll 调用不变
}
```

锁名：`ai_gateway:replace:{ai_cache_rules|ai_context_rules|traffic_mirror_rules}`（≤64 字符，含资源名便于
`performance_schema.metadata_locks` / `SHOW PROCESSLIST` 排查）。`replaceLockTimeout = 10 * time.Second`。

### 2.2 锁的作用域与安全性论证

锁在 `ReplaceAll` 内获取、在返回前释放，即**事务提交前释放**（事务由 manager 的 `AtomExecute` 持有）。为什么安全：

1. 任一时刻只有一个持锁者可进入 delete-all 阶段 → **不存在两个事务互等**（死锁的必要条件被消除）；
2. 持锁者释放后仍在提交，下一个持锁者的事务会**阻塞等待**这些行锁（W1 → W2 单向等待），
   提交在毫秒级完成，不构成环；
3. 若要严格「锁覆盖到 COMMIT」，需把获取点提到 manager（见 §2.5 备选 B），代价是 storager 接口新增方法，
   收益是消除「释放到提交之间」的毫秒级窗口 —— 本方案选择更小的改动面，并在 §6 记录该权衡。

### 2.3 L2：有界重试（纵深防御）

串行化已消除 1213，但仍可能遇到 `1205 Lock wait timeout`（同表其它写入者、慢查询）等瞬态锁冲突，
因此对**幂等**的整体替换事务增加有界重试。

`lib/xdb.go`（沿用既有 `DuplicateEntryError` 的字符串判据风格，不引入驱动类型依赖）：

```go
// TransientLockError reports whether err is a retryable lock conflict:
// MySQL 1213 deadlock, MySQL 1205 lock wait timeout, or SQLite busy.
func TransientLockError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, s := range []string{
		"Error 1213", "Deadlock found",
		"Error 1205", "Lock wait timeout exceeded",
		"database is locked", "database table is locked",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
```

`model/itxn/retry.go`（放在 itxn 内，manager 已持有 `m.txn`，无需新增 import）：

```go
const replaceTxAttempts = 3 // 首次 + 2 次重试

// ExecuteWithRetry runs do inside a transaction, retrying transient lock
// conflicts with jittered backoff. The closure may run more than once and must
// therefore be idempotent and free of non-DB side effects -- it is meant for
// whole-collection replacement (delete-all + insert-all), not for general use.
func ExecuteWithRetry(ctx context.Context, txn TxnStorager, do func(context.Context) error) error {
	var err error
	for attempt := 1; ; attempt++ {
		if err = txn.AtomExecute(ctx, do); err == nil {
			return nil
		}
		if attempt >= replaceTxAttempts || !lib.TransientLockError(err) {
			break
		}
		delay := time.Duration(attempt) * 20 * time.Millisecond
		jitter := time.Duration(rand.Int63n(int64(delay/2) + 1))
		select {
		case <-ctx.Done():
			return err // 交给上层按原错误处理（客户端已断开）
		case <-time.After(delay + jitter):
		}
	}
	if lib.TransientLockError(err) {
		return xerror.WrapConflictErrorWithMsg("collection is being replaced concurrently, retry later")
	}
	return err
}
```

manager 侧改动（三个文件各 1 行）：

```go
// model/ai_cache/ai_cache_manager.go:130
err := itxn.ExecuteWithRetry(ctx, m.txn, func(ctx context.Context) error {
	return m.storager.ReplaceAll(ctx, replaceRules)
})
```

**幂等前提**：闭包只做「delete-all + insert-all」，重跑得到同样的终态；`before/after` 审计快照在事务外，
失败只记录一条终态失败日志，不会因重试产生多条。

### 2.4 L3：错误契约与 F-14 收敛

1. **409 Conflict**：取锁超时与重试耗尽统一返回 `xerror.WrapConflictErrorWithMsg(...)` →
   `lib/xerror/resolve.go:82-84` 映射 `ErrNum=409`、`Type=Conflict`，文案为业务语义，不含驱动原文。
2. **全局 DAO 文案脱敏**（F-14）：`lib/xerror/resolve.go:55-58` 当前 `Msg = errors.Cause(err)`，即驱动原文。
   改为：

```go
rr := &ResolveResult{ErrNo: 500, err: err}
...
case etDao:
	rr.Type = "Database Exception"
	rr.Msg = "database operation failed" // 详情见 FullMsg() 日志，不回传客户端（F-14）
```

`ResolveResult.FullMsg()`（`resolve.go:42-48`）仍输出完整驱动错误与堆栈，排障信息不丢。
现有断言检查：全仓仅 `test/integration/tests/report/query/clickhouse_test.go:187` 断言
`ErrMsg` 含 `Database Exception`（**Type 前缀**，脱敏后仍满足），无测试断言驱动原文。

### 2.5 备选方案与取舍

| 方案 | 做法 | 结论 |
|---|---|---|
| A（采纳） | L1 命名锁 + L2 有界重试 + L3 409/脱敏 | 死锁面消除，跨副本有效，无 DDL |
| B | 把取锁点提到 manager，锁覆盖到 COMMIT | 语义更闭合，但需给三个 storager 接口新增 `LockReplace(ctx)` 并改 3 个接口 + fake 实现；本方案 2.2 已论证提交前释放无环，暂不做，保留为后续收紧项 |
| C | 只做重试（不串行化） | 20 并发下持续互杀，请求最长可等 `innodb_lock_wait_timeout`（50s），尾延迟与 409 概率不可控 |
| D | 改成按 id/name 增量 diff 写 | 会改变「每次 PUT 全删重建、id 重排」的既有契约（`ai-cache-rules.md` 已明确），属契约变更，超出本 issue |
| E | 进程内 `sync.Mutex` + 重试 | 单副本确定但不解决多副本；与 C 同样存在尾延迟，只作为 D 之前的临时手段 |

### 2.6 可选同批项（低优先）

`ai_cache_semantic_settings` / `ai_context_settings` 也是 `ReplaceAll` 单行替换
（`storage/rdb/ai_cache/settings_storager.go:71`、`storage/rdb/ai_context/settings_storager.go:71`），
同一 helper 各 4 行即可纳入（锁名分别 `...:ai_cache_semantic_settings` / `...:ai_context_settings`）。
它们并发度更低、单行删除插入的间隙冲突概率小，可作为同 PR 的可选收尾或另案。

## 3. SQLite / 单实例行为

| 场景 | 行为 |
|---|---|
| 驱动为 `glebarez/go-sqlite` 或 `sqlite-strip` | `xlock.Acquire` 直接返回 no-op release（不产生 SQL）；SQLite 单写者本身串行化 |
| 现有 `AIAPI_FORCE_SQLITE=1` 集成用例 | 行为不变，仅多一次类型断言 |
| 多副本 + MySQL | 命名锁跨副本互斥，锁名与资源一一对应 |

## 4. 关键边界

| 边界 | 处理 |
|---|---|
| 持锁进程崩溃 / 连接被 kill | 会话结束，MySQL 自动释放命名锁，无残留（对比：行锁方案同样自动释放，但需表与种子行） |
| `RELEASE_LOCK` 失败（ctx 取消、连接坏掉） | `conn.Raw` 返回 `driver.ErrBadConn` 丢弃物理连接 → 会话结束即释放，避免锁随连接回到池中永久滞留 |
| 取锁超时 | 返回 409（10s 上限），不阻塞请求链路 50s |
| 空集合替换 `{"rules":[]}` | 仍走 delete-all，也同样需要串行化（保持一致） |
| 校验失败（重复 cond、非法字段） | 在 `ReplaceAll` 之前返回 422，不取锁、不重试（不变） |
| 事务重试与审计日志 | 只在最终结果记录一次；`before` 快照在事务外抓取（不变） |
| 锁粒度 | 只锁同一集合；不同集合（三张表）互不阻塞 |

## 5. 单元测试设计

| 目标包 | 用例 | 断言 |
|---|---|---|
| `lib/xlock`（新增 `xlock_test.go`） | 非 MySQL 驱动（真实 sqlite 内存库） | 返回非 nil release、`nil` 错误、不执行任何 SQL |
| | sqlmock + `supportsNamedLock` 覆写为 true，`GET_LOCK` 返回 1 | 取锁成功；release 执行 `RELEASE_LOCK` |
| | `GET_LOCK` 返回 0 | 错误经 `xerror.Resolve` 得到 `ErrNo=409`、`Type=Conflict`、消息不含 SQL 原文 |
| | `GET_LOCK` 返回驱动错误 | `ErrNo=500`、`Type=Database Exception`、`Msg` 为脱敏文案 |
| | `RELEASE_LOCK` 失败 | 触发 `conn.Raw`（sqlmock `ExpectClose`），不 panic |
| `lib`（`xdb_test.go` 追加） | `TransientLockError` 表驱动 | 1213/1205/deadlock 文案/busy → true；`Error 1062`、普通错误 → false |
| `model/itxn`（新增 `retry_test.go`） | 首次成功 | 只调用 1 次 |
| | 前两次 1213、第三次成功 | 调用 3 次并返回 nil |
| | 三次全失败 | 返回 409 `Conflict` |
| | 非瞬态错误 | 不重试，原样返回 |
| `model/{ai_cache,ai_context,traffic_mirror}` | 既有 fake txn 用例 | 全部保持通过（`ExecuteWithRetry` 对 fake 等同单次 `AtomExecute`） |
| | 新增：`ReplaceAll` 返回 1213 三次 | manager 返回 409，且记录一条失败操作日志 |
| `lib/xerror`（新增 `resolve_test.go`） | `WrapDaoError(errors.New("Error 1213 ..."))` | `ErrNo=500`、`Type=Database Exception`、`Msg` 不含 `1213`；`FullMsg()` 含 |

## 6. 集成测试设计

三个既有并发用例（`AC-1-012` / `CTX-1-005` / `TM-1-015`）保留原断言，并补齐 F-14 断言：

```go
// 现状：只在 5xx 时报错
if resp.ErrNum >= 500 { ... }
// 补：任何非 2xx 的响应都不得携带驱动错误原文
if resp.ErrNum != 200 {
	assert.NotContains(t, resp.ErrMsg, "Database Exception")
	assert.NotContains(t, resp.ErrMsg, "Error 1213")
	assert.NotContains(t, resp.ErrMsg, "Deadlock")
}
```

新增（可选，同批）：`--run 'ConcurrentMySQL'` 覆盖「20 并发 + 每次提交 3 条规则」的变体，验证串行化下
集合顺序与最终状态一致（顺序即优先级，重试不得打乱顺序）。

## 7. 验证步骤

```bash
# 1) 单测
go test ./lib/... ./model/itxn/ ./model/ai_cache/ ./model/ai_context/ ./model/traffic_mirror/ -count=1
make test-model-cover-gate

# 2) MySQL 并发回归（本机 MySQL 8.4 需先启动：net start MySQL84）
export AIAPI_MYSQL_DSN='root@tcp(127.0.0.1:3306)/'
cd test/integration
go test -tags mysql -count=1 -v -run 'ConcurrentMySQL' \
  ./tests/ai_cache/update/ ./tests/ai_context/update/ ./tests/traffic_mirror/update/

# 3) SQLite 回归（确认 no-op 分支）
export AIAPI_FORCE_SQLITE=1
go test -count=1 ./tests/ai_cache/... ./tests/ai_context/... ./tests/traffic_mirror/...

# 4) 残留检查
mysql -h 127.0.0.1 -u root -e \
  "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'ai_gateway_it_%';"
```

判据：第 2 步三包 PASS（20 并发全 2xx）；第 3 步全通过；第 4 步 0 行。

## 8. 风险与回滚

| 风险 | 缓解 |
|---|---|
| 命名锁把并发写串行化后，长事务会拉长其他请求等待 | 锁只覆盖很小的 delete+insert，且有 10s 取锁上限；超时返回 409 而非挂死 |
| `GET_LOCK` 在极老 MySQL（<5.7.5）语义差异 | 仓库最低支持 MySQL 8.0（部署文档），`GET_LOCK` 自 5.7.5 起支持多锁 |
| 全局面包脱敏影响客户端解析 | 仅 DAO 500 文案变化；`Type` 前缀与错误码不变；F-14 明确要求 |
| 重试导致偶发重复执行 | 闭包为纯 DB 覆盖写，终态一致；契约写入 `ExecuteWithRetry` 注释 |

回滚：删除三个 `ReplaceAll` 中的取锁块即可回到当前行为（`lib/xlock`、`ExecuteWithRetry`、
脱敏均为独立改动，可按需单独回退）。

## 9. 实施与验证记录（2026-11-03）

| 项 | 结果 |
|---|---|
| 主模块单测 `go test ./...` | 103 包 `ok`，0 FAIL |
| `lib/xlock` / `model/itxn` / `lib/xerror` 新增单测 | 全 PASS（含取锁超时映射 409、释放失败丢弃连接、重试耗尽 409、DAO 文案脱敏） |
| MySQL 并发用例 AC-1-012 / CTX-1-005 / TM-1-015 | 3/3 PASS（20 并发全 2xx，最终集合等于某一次提交） |
| MySQL 409 端到端 AC-1-019 / CTX-1-006（新增 `replace_conflict_mysql_test.go`） | PASS：外部会话占锁 → PUT 返回 409 且 `ErrMsg` 无驱动原文；释放后 200 |
| MySQL 三模块全量 | 6/6 包 `ok` |
| SQLite 三模块 get/update | 6/6 包 `ok` |

**复现结论**：issue 记录的死锁在本机（MySQL 8.4.9）未复现 —— 不论空集合还是先灌 300 条规则，
临时移除取锁与重试后 20 并发仍全部 200；issue 环境为 MySQL 8.4.11，版本差异是可能原因但未定论。
修复的正当性来自 issue 的三模块 2/2 证据 + 源码链路；本地能验证的部分（锁生效、409、脱敏、no-op、无回归）均已覆盖，
其中 AC-1-013 用「外部会话持锁」直接证明命名锁在真实请求路径生效（若 `supportsNamedLock` 判定失败，
该用例会拿到 200 而失败）。