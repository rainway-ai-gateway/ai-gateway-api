# Issue #318 变更摘要：同秒并发导出命中 uk_name_version 唯一约束时整单 500，应吸收冲突

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#318](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/318)（SC2101-TC072：同 Topic 同秒并发导出命中 `config_versions.uk_name_version` 唯一约束时整单 500）

## 1. 背景

### 1.1 问题现象

只读导出 `GET /inner-api/v1/configs/tls_conf/server_data_conf`（Topic `route_rule`）在「导出内容刚变更 + 多路调用方同墙钟秒并发拉取」时，3/4 路返回 HTTP 500（`ErrNum=500` / `Database Exception`），仅 1 路成功且版本为 `V0+1s`。两轮独立 run 复现（`20261109-100927` / `20261109-101151`，失败形态 `statuses=[500 500 200 500]`）。违反 `InnerAPI配置导出与版本控制.md` 第 95 行「命中唯一约束后重读最新行、版本 +1s 重试（上限 3 次），不再整单报错」与第 421 行「若并发请求已为相同内容建过版本则直接返回其版本号」。

### 1.2 根因核实结论（源码直读，与 issue 定位一致并补齐关键细节）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `model/iversion_control/version_control.go:87-108` | `ExportConfig` 在 `AtomExecute` 单事务内：生成内容 → 清版本 → 算签名 → `UpsertConfigLastExportedVersion` | ✅ 实锤（直读） |
| 2 | `storage/rdb/version_control/version_control.go:49-61` | 事务内读最新行；签名相同复用版本；否则候选版本 `bumpVersion(latest, now)` 后插入 | ✅ 实锤（直读） |
| 3 | `storage/rdb/version_control/version_control.go:63-90` | 命中唯一约束后的重试逻辑已存在：`DuplicateEntryError` 为真 → 重读最新行 → 同签名返回其版本 / 不同签名 `bumpVersion` 重试，上限 `maxDuplicateRetries=3`；为假 → `:72-73` 整单 `return "", err` | ✅ 实锤（直读）——**骨架是对的，两个入口都失效** |
| 4 | `lib/xdb.go:158-166` | `DuplicateEntryError` 的 MySQL 判定为子串 `"Error 1062: Duplicate entry"`（无 SQLSTATE 括号）；而 `go.mod:16` 的 `go-sql-driver/mysql v1.9.3` 的 `MySQLError.Error()` 输出 `Error 1062 (23000): Duplicate entry ...` → **判定恒为 false，重试从未被进入** | ✅ 实锤（直读）——本轮 500 的直接原因 |
| 5 | `lib/xdb.go:56-66` + `go.mod:20` | DAO 层以 `pkg/errors` `WithMessage`（v0.9.1，实现 `Unwrap()`）包装错误，stdlib `errors.As` 可穿透到 `*mysql.MySQLError` | ✅ 实锤（直读）——`errors.As` 方案可行的依据 |
| 6 | `storage/rdb/internal/dao/internal/curd.go:39-70` | DAO 查询经 gendry SelectBuilder 编译，**不支持 `FOR UPDATE` 后缀** | ✅ 实锤（直读）——重试重读必须走原始 SQL |
| 7 | `storage/rdb/version_control/version_control.go:79` | 重试路径的重读是事务内普通 SELECT；MySQL 默认 `REPEATABLE READ` 下读的是事务快照（同类说明见 `table_api_key_id_seq.go:70`），看不到并发赢家已提交的行 → `:89` `bumpVersion(旧latest, 已尝试版本)` 原样返回已尝试版本 → 再次插入同一版本再次 1062 → 3 次耗尽仍失败 | ✅ 源码+隔离级别推断——**即使修好 #4 仍不收敛**，需事务内并发单测实证（验收判据 5） |
| 8 | `storage/rdb/keyrotate/keyrotate.go:112` | 仓内已有先例：DAO 存储层直接 `Execer().QueryRowContext(... "FOR UPDATE")` 原始查询 | ✅ 实锤（直读）——修法有先例 |
| 9 | `storage/rdb/internal/dao/table_api_key_id_seq.go:136-140` | 仓内已有方言判定先例：`isMySQL(conn)` 经 `conn.Driver().(*mysql.MySQLDriver)` 类型断言 | ✅ 实锤（直读）——SQLite 不支持 FOR UPDATE 语法，必须方言分支 |
| 10 | `lib/xdb_test.go:113-119` | 现有测试只覆盖旧格式 `"Error 1062: Duplicate entry"`，无新格式 / 真实 `*mysql.MySQLError` 用例 | ✅ 实锤（直读）——测试放大器之一 |
| 11 | `storage/rdb/version_control/version_control_test.go:172-206` | 既有并发用例跑 SQLite `:memory:` + `SetMaxOpenConns(1)`，单连接无真实并发；SQLite 唯一冲突消息 `UNIQUE constraint failed` 又能被 `DuplicateEntryError` 命中 → MySQL 路径从未被真实执行 | ✅ 实锤（直读）——测试放大器之二 |
| 12 | `conf/i18n` 无关；驱动 `parseTime=true` 在测试 DSN（`testutil/mysql.go:194`、`table_api_key_id_seq_mysql_test.go:46`）均已确认 | FOR UPDATE 原始查询可直接 `Scan` `created_at/updated_at` 为 `time.Time` | ✅ 实锤（直读） |

**排除项**：
- 「导出接口整体不可用」——两轮基线串行导出均 2xx、内容正确；
- 「测试侧制造重复版本」——测试只走管理面 PATCH，导出行由产品自身写入；
- 「#142 修复引入的回归」——#142 的唯一索引 / `max(now, latest+1s)` 单调抬升 / `ORDER BY version DESC, id DESC` 三项机制均正常发挥作用（唯一约束确实拦截了后到者），本卡是 #142 文档中已声明的「并发残余竞态」吸收逻辑未生效，非回归。

### 1.3 契约依据与裁定需求

- `design-docs/sys-design/details/InnerAPI配置导出与版本控制.md`（normative）：
  - 第 95 行：「若插入命中唯一约束（并发导出交错等残余竞态），重读最新行、版本再 +1s 重试（上限 3 次），不再整单报错」；
  - 第 421 行：「并发导出竞争同一版本号 → 唯一约束拦截后到者，重读最新行 +1s 重试（上限 3 次）；**若并发请求已为相同内容建过版本则直接返回其版本号**」；
  - 第 13/88/97 行：版本号同 Topic 严格单调递增、`uk_name_version` 数据库层保证。
- **无需产品裁定**：条款对冲突吸收的要求已明确；issue 已给出修复方向（错误识别 + 重试收敛）。「耗尽后的错误形态」owner 已裁定采用 409 `Conflict`（见 §2.3）。

## 2. 修复方案

两个失效入口分别修，缺一不可（只修 #4 → 重试不收敛仍 500；只修 #7 → `DuplicateEntryError` 恒 false，根本进不了重试）：

### 2.1 改动 1：错误识别 —— `lib.DuplicateEntryError` 兼容驱动真实错误格式（`lib/xdb.go:158-166`）

```go
func DuplicateEntryError(err error) bool {
    if err == nil {
        return false
    }
    // go-sql-driver/mysql v1.9.3 的 MySQLError.Error() 输出
    // "Error 1062 (23000): Duplicate entry ..."，与旧子串
    // "Error 1062: Duplicate entry" 不兼容（issue #318）。
    var mysqlErr *mysql.MySQLError
    if errors.As(err, &mysqlErr) { // pkg/errors WithMessage 实现 Unwrap，可穿透 DAO 包装
        return mysqlErr.Number == 1062
    }
    msg := err.Error()
    return strings.Contains(msg, "UNIQUE constraint failed") || // SQLite
        strings.Contains(msg, "duplicate key value violates unique constraint") || // Postgres
        strings.Contains(msg, "Error 1062:") || // 旧版驱动格式，向后兼容
        strings.Contains(msg, "Error 1062 (") // 新版驱动格式（防御性，非 *MySQLError 来源时兜底）
}
```

要点：

- `github.com/go-sql-driver/mysql` 已是直接依赖（`go.mod:16`），在 `lib` 引入其类型不新增依赖；`lib` 与驱动解耦的顾虑由字符串兜底分支承担（驱动错误被进一步包装成普通 `error` 的极端场景）。
- `errors.As` 依赖 `pkg/errors v0.9.1` 的 `withMessage.Unwrap()`（已核实存在）；断言目标是具体错误号 1062，**不会**把其他 MySQL 错误误判为重复键。
- 保留 SQLite / Postgres / 旧驱动格式分支，其他部署形态零回归。
- **决策记录（有意识取代 #295 的表述）**：`design-docs/modifications/2026-11-03-issue-295-rule-set-replace-concurrency/design-changes.md:142` 在新增 `TransientLockError` 时声明 `lib`「沿用字符串判据风格，不引入驱动类型依赖」。本改动**取代**该表述，理由：
  1. 字符串匹配对消息格式无所有权——go-sql-driver 自 1.7.0 起把 `MySQLError.Error()` 改为含 SQLSTATE 的 `Error 1062 (23000): ...` 格式后，旧匹配串静默失效、无任何告警，issue #318 正是这一失效模式的后果；类型断言 `Number == 1062` 对消息格式完全免疫。
  2. 耦合代价极小：驱动已是 `go.mod` 直接依赖且 `stateful` 本就完成驱动注册，单二进制交付；仓内 dao 层已有驱动类型断言先例（`table_api_key_id_seq.go:138` 的 `isMySQL`），本改动把同一做法上移到 `lib` 的错误识别入口。
  3. 字符串兜底分支仍保留（错误链被无 `Unwrap` 的包装打断的极端场景），且 §3.1 用真实 `&mysql.MySQLError` 的用例可在驱动格式再变时把断裂变成单测红，而非生产 500。

  #295 的 modifications 文档保持原文不改（历史变更记录不改写），取代关系以本条为准。

### 2.2 改动 2：重试收敛 —— savepoint + 唯一键等值 `FOR UPDATE` pin（`storage/rdb/`）

问题：MySQL `REPEATABLE READ` 下事务内普通 SELECT 读快照，重读永远拿到旧 `latest`，`bumpVersion` 原样返回已撞库版本，循环空转。

改法（实现期经真实 MySQL 并发测试迭代定型，三轮演进理由见文末「实现期修正记录」）：

1. `storage/rdb/internal/dao/table_config_versions.go` 新增：

   ```go
   // MySQL 下使用 SELECT ... WHERE name = ? AND version = ? FOR UPDATE
   // （唯一键等值 → 仅记录锁，无间隙锁）；非 MySQL 退化为普通 QueryOne。
   func TConfigVersionOneForUpdate(dbCtx lib.DBContexter, name, version string) (*TConfigVersion, error)
   ```

   - 等值 pin 的是**碰撞的确切行**：1062 的唯一键错误本身就证明了 `(topic, 已尝试版本)` 行存在，等值查询可直接定位，无需 `ORDER BY ... LIMIT 1` 范围扫描（范围扫描会引入间隙锁，见修正记录）。
   - 方言判定复用 `table_api_key_id_seq.go:138` 的 `isMySQL(conn)` 模式，并导出为 `dao.IsMySQL(dbCtx)` 供 storager 分支使用。
   - **只有重试路径调用它**；首次读取保持普通快照读，不扩大锁范围、不影响无冲突热路径。

2. `storage/rdb/version_control/version_control.go` 重试循环：

   ```go
   for i := 0; i < maxDuplicateRetries; i++ {
       if useSavepoint { SAVEPOINT vc_version_dup_retry }
       INSERT (topic, sign, version)
       成功 → RELEASE SAVEPOINT，返回 version
       非重复键错误 → 原样返回
       if useSavepoint { ROLLBACK TO SAVEPOINT }        // 关键：释放失败 INSERT 持有的 S 锁
       winner = SELECT ... WHERE name=? AND version=? FOR UPDATE   // current read，等值记录锁
       winner 为 nil → 返回（既有 :83-85 行为）
       签名一致 → 返回 winner.Version（第 421 行短路）
       version = bumpVersion(winner.Version, version)   // 严格抬升（第 97 行）
   }
   ```

   - `useSavepoint = dbCtx.InTx() && dao.IsMySQL(dbCtx)`：savepoint 只用于 MySQL 事务内（autocommit 下语句锁随语句结束即释放；非 MySQL 无此锁语义）。`lib.DBContext` 相应新增 `InTx()`（`DBContexter` 接口扩展，全仓唯一实现为 `*DBContext`）。
   - 循环其余语义（同签名返回赢家版本、异签名 `bumpVersion` 严格抬升、上限 3 次）**一行不动**。

3. `model/iversion_control/version_control.go` 的 `ExportConfig` 事务体经 `itxn.ExecuteWithRetry` 执行（原文档列为"后续可选"，实现期提前接入，理由见修正记录 C）：瞬态锁错误（1213/1205/SQLite busy）整事务回放，耗尽返回 409。导出事务（生成内容只读 + 签名 + 幂等 upsert）回放安全。

收敛性论证（MySQL 下）：

- 同签名并发（本 issue 场景）：后到者 1062 → `ROLLBACK TO SAVEPOINT` 释放 S 锁 → `FOR UPDATE` pin 看到赢家行（current read）→ 签名一致 → 返回赢家版本。4 路同内容并发 = 1 行版本、4 路全 2xx，满足第 421 行与验收判据 1/3。
- 异签名并发：后到者 pin 赢家行（X 记录锁，多后到者在 pin 上串行）→ 签名不同 → `bumpVersion` 抬升 → 插入；撞库则再循环，pin 逐次锚定更高行，上限 3 次内收敛。
- 极端交错（pin 后又有人插入同版本）：再次 1062 → 循环内再 pin 再抬升，每轮锚定的行严格升高，必然收敛。

**否决的替代方案**：

- 「盲目对已尝试版本 +1s 重试」（不重读）：issue 修复方向已否决，本文档确认（同签名冗余版本、4 路异签名吃满上限）。
- 「`ORDER BY version DESC, id DESC LIMIT 1 FOR UPDATE` 范围扫描重读」（本文档初稿方案）：真实并发测试中出现间隙锁/S 锁升级两类 1213 死锁（见修正记录 A/B），否决。
- 「`FOR SHARE` pin」：同签名场景可行（S 锁兼容），但异签名时 S pin 无法串行化后到者，多个后到者同时插入同一抬升值，在聚簇索引尾上死锁（修正记录 B），否决。

#### 实现期修正记录（真实 MySQL 8.4 并发测试驱动）

- **A. 范围扫描 `FOR UPDATE` 死锁**：初稿按"重读最新行"设计 `ORDER BY ... FOR UPDATE`。4 路并发实测 1213。死锁图显示：失败 INSERT 的唯一键检查在 `REPEATABLE READ` 下把 **S next-key 锁持有到事务结束**，多个后到者同时在范围扫描的间隙锁/X 锁上互等。**教训：任何在失败 INSERT 之后请求更多锁的设计都必须先处理这个遗留 S 锁。**
- **B. 等值 `FOR UPDATE` pin 仍死锁（S→X 升级环）**：S 锁释放前直接 pin → 两个后到者各持 S、各等 X，互为死锁。改用 `FOR SHARE` 后同签名通过，但异签名在抬升值插入处死锁（多个事务同时插入同一新键，聚簇索引尾 X 间隙锁 + 插入意向锁互等——空表/同键并发插入的 InnoDB 固有行为）。最终方案：**savepoint 回滚释放 S 锁 + `FOR UPDATE` 等值 pin**。savepoint 释放行为经 CLI 实证（`ROLLBACK TO SAVEPOINT` 后他事务 `FOR UPDATE NOWAIT` 立即可得锁）。
- **C. 残余 1213 由模型层 ExecuteWithRetry 吸收**：即便 pin 方案，"N 路同时插入同一全新键"（首次导出空 topic 的极端场景）在 InnoDB 层仍可能 1213，这是单事务算法无法根除的。`ExportConfig` 因此接入 `itxn.ExecuteWithRetry`（#295 既有能力，原文档列为后续项）：受害事务整单回放后收敛。验收判据 1 的"4 路全 2xx"由「storager 吸收 1062（主路径）+ ExecuteWithRetry 回放 1213（残余路径）」共同保证。

### 2.3 改动 3：重试耗尽后的错误可区分（罕见安全网）

修复 1+2 后，3 次重试耗尽理论上仅在持续极端竞争下发生（每轮重读都基于 current read 严格抬升，实际不可达）。若耗尽，当前行为是原始 DAO 错误 → HTTP 500 `Database Exception`，与真实故障不可区分（验收判据 2 要求「错误必须可区分」）。

已确认（owner 裁定）：`model/iversion_control/version_control.go:102-105` 的 `ExportConfig` 对 `UpsertConfigLastExportedVersion` 返回的错误包一层 Conflict 错误 → ErrNum=409 `Conflict`，与 500 区分。模型层包装符合 `wrap.go` 的分层注释（`WrapConflictErrorWithMsg Just Service layout invoke`）。

实现要点：`xerror.WrapConflictErrorWithMsg` 本身只包装 `fmt.Errorf(tip, args...)`，会丢弃原始错误链。为保留底层错误供日志排查，将原始错误以 `%v` 形式带入消息：

```go
lrv.version, err = vcm.storager.UpsertConfigLastExportedVersion(ctx, lrv)
if err != nil {
    if lib.DuplicateEntryError(err) {
        return xerror.WrapConflictErrorWithMsg("export version conflict after retries: %v", err)
    }
    return err
}
```

注意只包装重试耗尽路径：需在包装前判定该错误已被识别为重复键冲突（如经 `lib.DuplicateEntryError` 判定，或约定 `UpsertConfigLastExportedVersion` 耗尽时返回可识别错误类型），非冲突类错误（连接失败、锁超时等）保持原有 500 语义不变。具体判定方式实现时定，原则：**只有「3 次重试耗尽」这一种理论路径从 500 → 409，其余错误码零变化**。

### 2.4 不改动的部分（#142 回归红线）

| 项 | 说明 |
|----|------|
| 唯一索引 `uk_name_version` / 表结构 | 不动，它是冲突拦截与收敛正确性的地基 |
| `bumpVersion` 单调抬升语义 | 不动（改动 2 只是让它拿到**真的**最新行） |
| 首次读取的 `ORDER BY version DESC, id DESC` 确定性读取 | 不动（重试 pin 改为唯一键等值定位，与该排序无关） |
| 内容签名不一致时绝不吞掉冲突返回旧版本 | 不动——否则重新引入 #142 版本楔死 |
| 版本串 14 位 `20060102150405` 格式 | 不动 |
| `ExportConfig` 事务边界、签名计算、内容生成 | 事务边界本身不动（仍由 `AtomExecute` 包裹）；唯一变化是外层经 `itxn.ExecuteWithRetry` 执行，瞬态锁错误整单回放（§2.2 修正记录 C） |

### 2.5 修复后行为对照

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 同内容 4 路同秒并发导出（TC072） | 3/4 路 500 | **4 路全 2xx，返回同一赢家版本，仅 1 行版本记录** |
| 异内容 N 路同秒并发导出 | 后到者 500 或（SQLite 下）重试不收敛 | 后到者 FOR UPDATE 串行后逐次 +1s，全部成功，版本严格递增 |
| 串行内容变更同秒导出 | 正常 +1s 抬升（#142 已修） | **不变**（回归守护点） |
| 内容未变导出 | 返回原版本，不新增行 | 不变 |
| 时钟回拨 / 库内未来版本 | 抬升至 latest+1s（#142 已修） | 不变 |
| 首次导出（无最新行） | 竞态时 500 | FOR UPDATE 空结果 → 重试循环按 `latest == nil` 返回错误路径处理（既有 `:83-85` 行为不变；空 topic 竞态由 1062 重试 + bump 吸收） |
| 重试 3 次耗尽（理论） | HTTP 500 Database Exception | **HTTP 409 Conflict（可区分）**（§2.3，已裁定） |

## 3. 测试计划

### 3.1 单元测试（离线必跑）

| 包 | 用例 | 断言 |
|----|------|------|
| `lib` | `TestDuplicateEntryError` 扩充（`xdb_test.go:113-119`） | ① 真实 `&mysql.MySQLError{Number: 1062, SQLState: [5]byte{'2','3','0','0','0'}, Message: "Duplicate entry ..."}` → true（`Error()` 输出 `Error 1062 (23000): ...`）；② 该错误经 `xerror.WrapDaoError` 包装后 → true（`errors.As` 穿透）；③ `&mysql.MySQLError{Number: 1146}` → false（不误判其他错误号）；④ 新格式裸串 `Error 1062 (23000): ...` → true；⑤ 旧格式 / SQLite / Postgres 串 → true（不回归）；⑥ 无关错误 / nil → false |
| `lib` | `TestTransientLockError`（`:121-140`） | 不改动即通过（旁证：同文件已用新驱动格式 `Error 1213 (40001)` 串，说明仓内只有 `DuplicateEntryError` 停留在旧格式） |
| `storage/rdb/version_control` | 新增 `TestUpsertConfigLastExportedVersion_RetryReturnsWinnerVersion`（文件 SQLite 库，第二条连接持**未提交**赢家行） | 用 blocker 连接插入 `(topic, sign-a, V0)` 不提交 → storager 首读不可见、候选版本必然撞库 → 阻塞至提交后命中 `UNIQUE constraint failed` → 重试 pin 看到赢家行 → **返回 V0 且不再新增行**（验证第 421 行短路 + 重试路径真实执行）。实现说明：单连接可见性下「预插赢家再调 Upsert」无法制造撞库（候选版本恒大于最新行），必须用未提交行制造快照不可见，秒级翻转用 3 次尝试兜底 |
| `storage/rdb/version_control` | 新增 `TestUpsertConfigLastExportedVersion_RetryBumpsPastWinner`（同上设置） | 赢家 `sign-a` / 请求 `sign-b` → 撞库后 pin → 签名不同 → 插入 `bumpVersion(V0, V0)=V0+1s`，**严格大于 V0**，最终 2 行且递增 |
| `storage/rdb/version_control` | 既有 `TestUpsertConfigLastExportedVersion_*` 全部（`:92-241`） | **不改动即通过**——#142 四项门禁（同秒单调 / 内容回弹 / 未来版本 / 并发）回归守护 |
| `storage/rdb/internal/dao` | 新增 `TConfigVersionOneForUpdate` 的 SQLite 退化分支用例 | 非 MySQL 连接下退化为普通 QueryOne，结果与 `TConfigVersionOne` 一致（FOR UPDATE SQL 只在 MySQL 分支生成，离线环境不触发语法错误） |

### 3.2 并发收敛测试（真实 MySQL，`//go:build mysql`）

新增 `storage/rdb/version_control/version_control_concurrency_mysql_test.go`（DSSN 约定同 `table_api_key_id_seq_mysql_test.go`：读 `~/mysql-data/root-password.txt`，实例不可用自动 Skip）：

| 用例 | 设置 | 断言（对应验收判据 5） |
|------|------|------------------------|
| `TestUpsertConfigLastExportedVersion_ConcurrentSameSign` | 每 worker 独立连接、`RDBTxnExecute` 包裹（复现 `ExportConfig` 事务内语义）+ `itxn.ExecuteWithRetry` 包装（同生产）；topic 先种 baseline 行，4 worker 3ms stagger 后同签名并发 | 全部成功；返回版本全部相同（= 赢家版本，> baseline）；该 topic 恰 2 行（baseline + winner）——**已实证：修复前代码上此用例必失败（1062 不可识别 → 非瞬态错误），修复后 30 连跑通过** |
| `UpsertConfigLastExportedVersion_ConcurrentDiffSign` | 同上 baseline；**暂存未提交赢家**（事务内插入后挂起，确定性制造撞库条件）+ 3 个异签名 loser 同时唤醒；worker 走 `ExecuteWithRetry` | 全部成功；返回版本 = base+1s..base+4s 各一（pin 串行抬升链）；恰 5 行——修复前同样必失败 |
| 隔离级别显式声明 | DSN 不设特殊参数（默认 `REPEATABLE-READ`） | 测试注释标明：前提即默认隔离级别下快照读不可见赢家行 |

实现说明（真实测试驱动的设计决策）：

- **baseline 种子行**：空 topic 上 N 路完全同步的首插入会在聚簇索引尾互持 X 间隙锁死锁（InnoDB 固有行为，见 §2.2 修正记录 B），生产由 `ExecuteWithRetry` 回放吸收；storager 级测试用 baseline 行 + stagger 让竞争落在「1062 → 重试」路径（issue 实际故障形态）。
- **`ExecuteWithRetry` 包装**：同生产栈一致（#318 修复后 `ExportConfig` 本身即如此），残余 1213 走整单回放收敛。
- **秒级翻转兜底**：候选版本为秒级时间串，测试用 3 次尝试 + 新 topic 重试消除翻转抖动。

### 3.3 E2E / 验收判据

对照 issue 七条验收判据：1/2/3 由 QA requeue `SC2101-TC072`（连续 3 轮 × 4 路并发）验证；4 由 §3.1 的 `lib` 用例覆盖；5 由 §3.2 覆盖；6 由 §3.1 既有用例 + requeue `SC2101-TC018`（#142 门禁）覆盖；7 由全量回归覆盖。落地动作：QA 环境跑 `SC2101-TC072` ×3 轮无 500 后关闭 issue。

**已落地的自动化 E2E**（本实现新增，`test/integration/tests/innerapi/tls_conf/concurrent_export_mysql_test.go`，`//go:build mysql`）：完整复刻 TC072 形态——真实 API 子进程（`testutil.StartServer`，MySQL 后端自动探测）+ OpenAPI 建/删 Cluster 变更内容 + 串行基线导出 + 4 路同秒并发 `GET server_data_conf`，3 轮断言全部 `ErrNum=200`、收敛同一新版本（> 基线）、`ErrMsg` 不泄漏驱动原文。**判别性已实证：修复前二进制上 3/4 路 500（原生症状复现），修复后二进制 3 连跑通过。** 非 MySQL 后端自动 Skip。运行前需 `go build -o ai-gateway-api.exe .`（testutil 要求项目根存在该二进制）。

## 4. 兼容性、迁移与风险

| 项 | 结论 |
|----|------|
| 正常路径行为 | 零变化：无冲突时首次读仍是快照读，不新增任何锁；签名相同短路、版本单调抬升语义不变 |
| 锁竞争风险 | savepoint + 等值记录锁将死锁面收敛到「N 路同时插入同一全新键」的 InnoDB 固有场景；该残余由 `ExportConfig` 已接入的 `itxn.ExecuteWithRetry` 整单回放吸收（§2.2 修正记录 C），耗尽返回 409 而非 500 |
| 数据库形态 | MySQL：current read 收敛；SQLite：FOR UPDATE 语法不支持，方言分支退化为普通读——SQLite 单写者串行 + 集成测试单连接模式下本就不存在该竞态，无回归；Postgres：现有字符串分支保留，未引入新的方言假设 |
| 错误码 | 仅「3 次重试耗尽」这一理论路径由 500 → 409（§2.3，已裁定）；其余错误码零变化 |
| 数据面 / 导出格式 / `config_versions` 表结构 | 零改动 |
| 回滚 | 三个改动相互独立，单 commit 还原即可；改动 1 单独回滚不会使修复 2 恶化（只是回到恒 500） |

## 5. 关联风险与后续事项

1. **同类匹配串排查**：`DuplicateEntryError` 是仓内唯一按「旧驱动格式」匹配 MySQL 错误的函数（`TransientLockError` 等已用前缀格式）；建议合入前全仓再 grep 一次 `"Error 1062"` / `"Error 1` 模式确认无同类残留。
2. **错误日志补证**：issue 提到「触发冲突的具体数据库错误号需 API 侧日志补证」——QA requeue 时可从 API 错误日志确认本轮 500 的底层错误确为 1062（20261030 原始轮次的 `ErrMsg` 已逐字暴露 `Error 1062 (23000)` 与 `uk_name_version`，证据链已闭环，此项仅作形式补证）。
3. **E2E 影响面**：所有以导出做收敛/残留探测的 TC 集合待 QA 确认是否受「同秒并发 500 → 全 2xx」行为翻转影响（应全部转好，无预期恶化项）。
4. **后续可选优化**（本卡不做）：`config_versions` 表按需清理历史版本行（当前只增不减）。

## 6. 状态

- [x] 修复方案设计（本文档，含实现期修正记录）
- [x] 根因源码直读核实（错误识别入口 + 重试快照读两环 + 方言/驱动/先例核查）
- [x] owner 确认 §2.3 耗尽错误的 409 语义（已裁定：409 Conflict）
- [x] 代码实现（`lib/xdb.go` + `lib.DBContext.InTx` + `table_config_versions.go` + `version_control.go` + `model/iversion_control/version_control.go` 五处）
- [x] 单元测试（`lib` 错误识别扩充 + `version_control` 重试路径两用例 + dao 方言退化用例 + model 409/重试用例）
- [x] MySQL 并发收敛测试（`//go:build mysql` 两用例，修复前必败 / 修复后 30 连跑通过已实证）
- [x] 离线全量 `go test ./...` + `-tags mysql` 相关包 + 既有 #142 用例验证全绿
- [ ] QA requeue SC2101-TC072（3 轮 × 4 路）+ SC2101-TC018（#142 门禁）转 PASS 并关闭 issue #318
