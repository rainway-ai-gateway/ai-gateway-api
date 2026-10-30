# 缺价动作配置与形态标注（issue #1479）：ai-gateway-api 侧变更摘要

> 日期：2026-10-30　　issue：[bfenetworks/bfe#1479](https://github.com/bfenetworks/bfe/issues/1479)
> 上游背景：v0.7.0.1 测试发现问题3"模型缺少某种接口的定价时，该接口的请求不扣费"
> 本目录产出：`change-summary.md`（本文）、`design-changes.md`（逐文件改动 / 语义决策 / 兼容性 / 测试计划）
> 配套：bfe `v1.9.0-dev` commit `9f6ddd95`（已推送：`AIConf.PriceMissAction` 三态、`PRICE_MISS` 日志、计数器、`ai_price_miss`/`ai_price_warn` 写出）；`bfe-access-pb v0.3.17`（已发布：`ai_price_warn` field 766）

## 1. 背景

bfe 侧 #1408 已实现"非 chat mode 缺价回落 chat 价计费"，但缺价的**处置不可配置**（无法拒单）、**信号不可直读**（`ai_price_miss` 是布尔，不区分回退/按 0/拒绝/子项缺失）。#1479 在 bfe 侧补齐三态动作与形态化信号，其中"动作可配"与"控制台可见"需要本仓（控制台 API）参与：

| 需求 | 归属 | 本仓改动 |
|------|------|----------|
| 集群级"未定价则拒绝"选项 | 本仓 + bfe | `llm_config.price_miss_action`（OpenAPI/存储/校验/导出） |
| 控制台提示"有请求命中缺定价接口" | 本仓 + log-reader + observability + web | 报表字段 `ai_price_miss`/`ai_price_warn` 入库与查询（跨仓链路） |
| 配置期覆盖度校验 | **不做**（已确认） | 价格表由独立流程提供、顺序不可控，硬校验误杀风险高；缺价可见性由 bfe 日志/计数/pb 字段承担 |

## 2. 目标

| 项目 | 说明 |
|------|------|
| 变更日期 | 2026-10-30 |
| 涉及仓库 | `ai-gateway-api`（本仓：配置字段、校验、导出映射、报表列）；配套 `log-reader`（字段映射）、`ai-gateway-observability`（三套报表 schema）、bfe（已完成）、bfe-access-pb（已发布 v0.3.17） |
| 变更类型 | 新增可选配置字段（无 DB DDL、无数据迁移）；报表侧新增可选列（DDL 需迁移） |
| 兼容性 | 字段可选：旧数据/旧客户端省略即 `nil`，导出不带，bfe 走默认 `fallback_chat`；旧 bfe 忽略未知键，**本仓可先行发布** |
| 发布顺序 | bfe-access-pb（已发）→ bfe（已发）→ **本仓 + log-reader** → observability schema → web 展示（可后置） |
| 产出 | `model/icluster_conf`、`lib/validate`、`endpoints/openapi_v1/product_cluster`、报表 DDL/入库/查询 + 单测与集成断言 |
