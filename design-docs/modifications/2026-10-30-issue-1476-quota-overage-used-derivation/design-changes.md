# 配额超额全额记录（issue #1476）：ai-gateway-api 侧设计变更说明

> 日期：2026-10-30　　issue：[bfenetworks/bfe#1476](https://github.com/bfenetworks/bfe/issues/1476)
> 问题来源：v0.7.0.1 测试发现问题4"越过配额边界的请求只记到配额上限"
> 本文件展开 `change-summary.md` 的目标，给出逐文件改动、语义决策、兼容性与测试计划。
> 配套：`bfe/docs/zh_cn/modifications/2026-10-30-issue-1476-quota-overage-full-deduction/design-changes.md`（bfe 侧，已实施推送，`v1.9.0-dev` `89787a17`）

## 1. 设计决策

**跟随 bfe 侧"余额可为负"方案（方案A），不引入独立 used 计数器。**

- Redis 数据模型不变（单 key `QUOTA_<owner>` 存 remaining 定点整数），本仓 `used = quota − remaining` 派生公式不变，**只去掉派生路径上的负值钳制**。
- `used` 允许大于 `quota`（超出部分即欠费量 = `used − quota`）；`Balance.Remaining` 展示值恒 ≥ 0（Redis 里 −5 还是 −500 对消费者无意义）。
- 欠费状态下管理员调大配额：`targetRemaining = newQuota − used` 允许为负，负值 = 新配额仍不足以覆盖欠费，鉴权继续拒绝，**不凭空赠送额度**。

## 2. 逐文件改动

### 2.1 `model/api_key/api_key.go:322-339` `fillQuotaBalance`

变更前：

```go
if remaining < 0 {
    remaining = 0
}
used := *quotaPlan.Quota - remaining
if used < 0 {
    used = 0
}
...
quotaPlan.Balance.Used = &used
quotaPlan.Balance.Remaining = &remaining
```

变更后：

```go
// remaining 可为负（欠费，issue #1476）：used = quota - remaining 如实
// 反映实际消耗，允许超过 quota；对外展示的 Remaining 钳 0。
used := *quotaPlan.Quota - remaining
if used < 0 {
    used = 0 // 防御：数据异常（remaining > quota）时兜底
}
displayRemaining := remaining
if displayRemaining < 0 {
    displayRemaining = 0
}
...
quotaPlan.Balance.Used = &used
quotaPlan.Balance.Remaining = &displayRemaining
```

### 2.2 `model/entity/entity_manager.go:601-618` `fillQuotaBalance`

与 2.1 逐行同构的分身，做一模一样的修改。两处必须同步，否则 Entity 维度与 API-Key 维度的余额展示行为不一致。

### 2.3 `model/quota/quota_plan_manager.go:414-423` `adjustQuota`（quota 变化、单位不变分支）

变更前：

```go
used := oldQuotaValue - currentRemaining
if used < 0 {
    used = 0
}
targetRemaining := *newQuota - used
if targetRemaining < 0 {
    targetRemaining = 0   // ← 欠费下抹掉欠费、凭空赠送额度
}
```

变更后：

```go
used := oldQuotaValue - currentRemaining // remaining 可为负 → used 可大于 oldQuota
if used < 0 {
    used = 0 // 防御：数据异常时兜底
}
targetRemaining := *newQuota - used      // 允许为负：负值 = 新配额仍不足以覆盖欠费
quotaCache.SetRemaining(ctx, owner.key, &targetRemaining, planUnit)
```

`SetRemaining` 的 `GetInt64 + IncrBy(delta)` 实现（`model/quotacache/redis.go:88-107`）与定点换算 `golibquota.PtrToRedisValue/PtrFromRedisValue` 均为线性语义，负目标值与负值换算天然支持，**无需改动**。

## 3. 不需要改动的点（已核实）

| 位置 | 原因 |
|------|------|
| `model/api_key/api_key.go:199-201` `GetRemainingQuota` | 已有 `remain < 0 → 0` 钳制，展示语义正好符合本方案 |
| `model/quota/balance_sync.go:188/204` 周期/手动重置 | `ResetToQuotaAtomic` SET 为 quota，重置即清零欠费，符合"重置"语义（修复前后不变） |
| `endpoints/openapi_v1/**` | 余额均经 `fillQuotaBalance`/`GetRemainingQuota` 获取，随 2.1/2.2 自动生效 |
| `model/imods/exporter.go:603` bfe 配置导出 | 导出配额计划（quota/RedisKey），不涉及 remaining |
| `fillUnlimitedQuotaBalance`（两个文件） | 无限配额 sentinel 语义不变 |

## 4. 兼容性

1. **无 DB / Redis 迁移**：本侧纯读路径行为调整。
2. **先上本仓、后上 bfe**：bfe 仍截断扣减，remaining 不会变负，行为 = 现状，无回归。
3. **先上 bfe、后上本仓**：负 remaining 期间旧钳制让展示暂显 quota（现状表象），数据无损，升级后自动正确。
4. **web 前端**：`Balance.Used` 可能大于 `Quota`、`Balance.Remaining` 恒 ≥ 0。若前端有 `used ≤ quota` 隐含假设（进度条、百分比），需前端确认；本仓不改字段契约。

## 5. 文档同步

- `design-docs/sys-design/details/配额余额同步机制.md`：余额值域从 `[0, quota]` 扩为 `(-∞, quota]`，`used = quota − remaining` 不再钳零；`adjustQuota` 的 `targetRemaining` 允许为负。修改处标注 issue #1476。

## 6. 测试计划

### 单测

- `model/api_key/api_key_test.go`：`fillQuotaBalance` 用例——remaining 为负时 `Used = quota − remaining > quota`、`Remaining = 0`；remaining > quota 异常数据兜底 `Used = 0`。
- `model/entity/entity_manager_test.go`：同上（2.2 分身）。
- `model/quota/quota_plan_manager_test.go` `TestQuotaPlanManager_ApplyQuotaPlanChange`：
  - 欠费状态（mock remaining 为负）调大配额 → `SetRemaining` 收到 = `newQuota − used` 的负 targetRemaining；
  - 欠费状态调小配额 → targetRemaining 更负；
  - 余额充足场景回归：行为与修复前一致。
- `make test-model-cover-gate`（≥70% 语句覆盖门槛）通过。

### 集成测试（`test/integration`，已落地）

集成测试使用 miniredis，测试进程可通过 `ServerManager.SetQuotaRemaining` 直接预置负余额；运行前需重建根目录 `ai-gateway-api.exe`（`go build -o ai-gateway-api.exe .`）：

| 包 | 用例 | 验证点 |
|---|---|---|
| `tests/api_key/quota_update` | AK-9-011 | Redis 余额 -100 时查询：`used=1100`（> quota=1000）、`remaining=0` |
| 同上 | AK-9-012 | 欠费下调大配额至 1050：Redis **-50**、`used=1100`、`remaining=0`（欠费不被抹掉） |
| 同上 | AK-9-013 | 欠费下调大配额至 2000：Redis **900**、`used=1100`、`remaining=900`（鉴权恢复） |
| 同上 | AK-9-009/010（更新） | quota 改为 0：展示 `remaining=0`、`used` 保留，Redis 保留 **-600 / -600.1234** |
| `tests/entity/quota_update` | E-9-011/012/013 + E-9-009/010（更新） | Entity 维度与 API-Key 完全同构的断言 |

回归：`tests/api_key/quota_query`、`tests/api_key/quota_reset`、`tests/quota_period_reset` 全部通过（重置语义不变：重置即清零欠费）。

### 验收场景（跨仓，待发版后执行）

1. 额度 0.00002 元、放行=否：单次成本 0.0001486 → 已使用额度 = 0.0001486（> 配额），剩余额度显示 0；第二次请求 429。
2. 额度 0.00001 元、放行=是：3 次请求合计 0.0001348 → 已使用额度 = 0.0001348，不再停在配额上限。
3. 周期重置：重置后已用额度归零、剩余 = 配额（欠费清零，语义不变）。

## 7. 实施记录（2026-10-30）

六步法已按序执行完毕（Step 1/2 变更说明 → Step 3 api-define → Step 4 sys-design → Step 5 代码与单测 → Step 6 本文档回写）：

- **Step 3 api-define**：`00-common.md` balance 结构 `used` 行注明允许大于 `quota`（issue #1476，超出部分为超额用量）、`remaining` 行注明欠费时展示为 0；`api-keys.md` 更新接口执行逻辑第 1 条，调额公式由 `max(0, 新quota - balance.used)` 改为不钳零并说明负值语义。
- **Step 4 sys-design**：`details/配额余额同步机制.md` 三处同步——§8.1 派生步骤 3（不再钳零 + used 可超 quota）、§8.2 余额字段语义、§9.2 `ApplyQuotaPlanChange` 第 3 条。
- **Step 5 代码**（与 §2 逐文件对应）：
  - `model/api_key/api_key.go`、`model/entity/entity_manager.go` `fillQuotaBalance`：去掉 remaining 钳零；used 允许大于 quota；`Balance.Remaining` 展示钳 0；保留 remaining > quota 异常兜底。
  - `model/quota/quota_plan_manager.go` `adjustQuota`：去掉 `used` 与 `targetRemaining` 两处钳零；函数注释同步。
- **Step 5 测试**：`model/api_key`、`model/entity` 各新增 `TestFillQuotaBalance`（正常 / 负余额 / 异常兜底三臂）；`model/quota` `TestQuotaPlanManager_ApplyQuotaPlanChange` 更新"quota decreased"断言（targetRemaining 0 → −10）并新增"欠费状态调大配额"（→ 60）与"调大仍不足"（→ −20）两个子用例。
- **Step 5 集成测试**（补齐）：`tests/api_key/quota_update` 更新 AK-9-009/010 并新增 AK-9-011/012/013；`tests/entity/quota_update` 同构更新 E-9-009/010 并新增 E-9-011/012/013；两份 `design.md` 的用例表与 14.3 各节预期值同步。
- **验证**：`go test ./model/...`（等价 `make test-model`，本环境无 make）27 个包全部通过；`go vet ./model/...` 通过；集成测试 `tests/api_key/quota_update`、`tests/entity/quota_update`、`tests/api_key/quota_query`、`tests/api_key/quota_reset`、`tests/quota_period_reset` 全部通过。（注：仓库存在存量 gofmt 行尾告警，与本次改动无关。）

遗留：bfe 侧修复已在 `v1.9.0-dev`（commit `89787a17`）合入；两侧端到端验收（控制台已用额度口径，v0.7.0.1 原测试用例重跑）待本仓改动随版本发布后进行。
