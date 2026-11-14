# 配额超额全额记录（issue #1476）：ai-gateway-api 侧变更摘要

> 日期：2026-10-30　　issue：[bfenetworks/bfe#1476](https://github.com/bfenetworks/bfe/issues/1476)
> 上游背景：v0.7.0.1 测试发现问题4"越过配额边界的请求只记到配额上限"（下称"问题4"）
> 本目录产出：`change-summary.md`（本文）、`design-changes.md`（逐文件改动 / 语义决策 / 兼容性 / 测试计划）
> 配套仓库方案：`bfe/docs/zh_cn/modifications/2026-10-30-issue-1476-quota-overage-full-deduction/design-changes.md`（**已实施并推送**，bfe `v1.9.0-dev` commit `89787a17`：扣减 Lua 全额执行、余额可为负、新增 `QuotaOverage` 计数）

## 1. 背景

v0.7.0.1 测试发现问题4：API Key 额度快用完时，最后一次请求的实际成本超过剩余额度，请求照常完成，但已用额度只增加到配额上限，超出部分不记录；开启"额度不足时放行"（`pass_when_no_enough_quota`）后，已用额度一直停在配额上限，看不出实际消耗。

bfe 侧根因（已修复）是扣减 Lua 的 `math.min(current, amount)` 截断。**本仓（ai-gateway-api）是另一半根因**：所有"已用额度"都由 `quota − remaining` 派生，且派生路径上有 3 处"负值钳零"。即使 bfe 已让余额可为负，这 3 处钳制仍会让控制台已用额度显示 quota（表象不变），并在"欠费状态下调大配额"时凭空赠送额度。

| 问题 | 证据 |
|------|------|
| 展示钳零：负 remaining 被当 0，used 恒等于 quota | `model/api_key/api_key.go:327-330`、`model/entity/entity_manager.go:606-609`（两处 `fillQuotaBalance`，`if remaining < 0 { remaining = 0 }`） |
| 调额钳零：`targetRemaining < 0 → 0` 抹掉欠费 | `model/quota/quota_plan_manager.go:416-423`（`adjustQuota`：欠费状态调大配额时，欠费被悄悄清零、凭空赠送额度） |
| 读路径无独立用量累计可回退 | Redis `QUOTA_<owner>` 只有 remaining（bfe 侧 issue #1476 修复前还被截断在 0）；全仓无 overage/欠费字段 |

**影响**：控制台/OpenAPI 的 API Key、Entity 已用额度在超额场景下失真（放行模式下恒等于配额上限），影响对账与消费者可见的用量呈现。

## 2. 目标

| 项目 | 说明 |
|------|------|
| 变更日期 | 2026-10-30 |
| 涉及仓库 | `ai-gateway-api`（本仓：3 处钳制逻辑调整 + 测试）；配套 `bfe`（已完成：余额可为负） |
| 变更类型 | **纯读路径行为调整**：无 DB schema 变更、无 Redis key 变更、无 API 契约变更（`Balance.Used` 可能大于 `Quota`，`Balance.Remaining` 恒 ≥ 0） |
| 前置依赖 | bfe 侧 issue #1476 修复已上线（余额可为负）；本仓改动与 bfe 改动前后向兼容，可分开发布 |
| 产出 | `model/api_key`、`model/entity`、`model/quota` 三处改动 + 单测；`design-docs/sys-design/details/配额余额同步机制.md` 语义段同步 |
