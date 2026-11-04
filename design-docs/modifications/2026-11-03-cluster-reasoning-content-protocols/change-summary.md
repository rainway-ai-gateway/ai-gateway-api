# 思考模式推理内容补齐（ensure_reasoning_content）——变更摘要

## 1. 背景

部分思考模式上游对 chat 请求有单向校验：**"最后一条 `user` 消息之后"的 assistant 消息必须携带 `reasoning_content`**（缺字段或 `null` 即 400），而这些上游自身存在不产出 reasoning 文本的回合。客户端忠实镜像上游响应时，下一轮就会命中该 400；路由从一个 provider 切到另一个 provider 时更直接——当前轮 assistant 由上一个后端生成，根本不含该字段。

BFE 数据面已实现该问题的请求侧归一化：集群 `AIConf.EnsureReasoningContent`（协议名数组，空 = 关闭）在 AI 转发路径上为窗口内的 assistant 补空 `reasoning_content`（`openai`）或空 thinking 块（`anthropic`），定点写入、fail-open、自带四个指标（含 `ReasoningStill400` 作为上游契约变更的退出信号）。

本次变更是让 `ai-gateway-api` 控制面能够管理并下发该配置，替代"手工编辑 `cluster_conf.data` 且会被下一次导出覆盖"的临时路径。

## 2. 目标

1. cluster 数据模型 `llm_config` 新增 `ensure_reasoning_content` 可选数组字段（协议名列表）。
2. 导出 `cluster_conf.data` 时映射为 BFE `AIConf.EnsureReasoningContent`。
3. 校验规则与 BFE `AIConfCheck` 逐条对齐（值域收窄 + provider 协议交叉校验），防止"控制面放行、数据面加载失败"的配置穿透。
4. 向后兼容：字段缺省 = BFE 关闭该行为（历史逐字节透传）；无 DDL（复用 `clusters.llm_config` JSON 列）。

## 3. 范围

- **涉及面**：控制面数据模型与导出映射、两层校验、OpenAPI `/clusters` 字段、InnerAPI 导出内容、schema/单测/集成测试。
- **不涉及面**：BFE 数据面运行时逻辑（已完成）、conf-agent、数据库表结构、导出框架本身。
- **数据面影响**：BFE `AIConf` 已新增 `EnsureReasoningContent` 字段（见 `bfe` 仓 `docs/zh_cn/modifications/2026-11-03-ai-reasoning-content-normalization/design-changes.md`，commit `3b5d05a6`，分支 `v1.9.0-dev`）。

## 4. 关键决策

| 决策 | 说明 |
|------|------|
| 配置归属 cluster | BFE `AIConf` 为 per-cluster；同一 provider 可能有多个集群实例，按集群灰度才可控。挂 provider 会让同 provider 的所有集群一起变，无法分批 |
| 复用 `llm_config` JSON 列 | 无 DDL、零迁移，与 `key_affinity` / `normalize_upstream_error` 一致 |
| 数组而非 bool | 协议维度独立：同一 provider 可能同时提供 `openai` 与 `anthropic` 入口，注入落点不同；协议增加到 3 个以上时只需加元素 |
| 值域收窄到 `openai`/`anthropic` | `gemini` 在 `ValidModelProtocols` 内，但其请求体是 `contents/parts` 结构，没有 `reasoning_content` 字段可注入；接受该值等于放行一个"配了但什么都不做"的假开关，故**在控制面直接拒绝**，与 BFE `AIConfCheck` 同口径 |
| 默认值不下发 | 缺省字段不出现在导出里（nil 直通），由 BFE 侧零值即关闭处理——单一默认值来源 |
| 校验双层 | 值域（`lib/validate`，纯值检查）+ provider 交叉（`validateClusterLLMConfigAgainstProvider`，需要 provider 记录），与 BFE 加载期校验构成两道防线 |
| 依赖时序 | api 直接 import bfe 的 `cluster_conf` 包，新字段必须在依赖里可见才能编译：本地先用 `replace` 指向本地 bfe 验证，上游合并发版后再改回正常版本号 |

## 5. 关联文档

- 详细设计：`design-changes.md`
- 接口变更：`api-changes.md`
- BFE 侧修改说明：`bfe/docs/zh_cn/modifications/2026-11-03-ai-reasoning-content-normalization/design-changes.md`
- BFE 侧机制文档：`bfe/docs/zh_cn/sys_design/ai_reasoning_content_normalization.md`
- BFE 侧配置文档：`bfe/docs/zh_cn/configuration/server_data_conf/cluster_conf.data.md` §9.5
- BFE 侧集成场景：`bfe/tests/integration/测试设计文档/scenario-SC45-推理内容归一化/`

## 6. 实施阶段

| 阶段 | 内容 | 状态 | 关键文件 |
|------|------|------|----------|
| 1 | BFE 数据面实现（配置字段/`AIConfCheck`/转发路径归一化/四个 counter/单测/SC45） | ✅ 已完成（bfe `3b5d05a6`） | `bfe/bfe_config/.../cluster_conf_load.go`、`bfe/bfe_server/reverseproxy_ai_reasoning.go`、`bfe/bfe_server/proxy_state.go` |
| 2 | 控制面数据模型 + 导出映射（`llm_config.ensure_reasoning_content` → `AIConf.EnsureReasoningContent`） | ✅ 已完成 | `model/icluster_conf/cluster.go` |
| 3 | 两层校验（值域 + provider 交叉） | ✅ 已完成 | `lib/validate/validate.go`、`model/icluster_conf/cluster.go` |
| 4 | 端点透传（`normalizeLLMConfig`） | ✅ 已完成 | `endpoints/openapi_v1/product_cluster/create.go` |
| 5 | 依赖 bump（受 `bfe_tls` 下线阻塞，见 `design-changes.md` §6） | ⏳ 待外部条件 | `go.mod` |
| 6 | 文档与测试（api-define、sys-design、schema、单测） | ✅ 已完成 | `design-docs/`、`test/integration/tests/schema/` |

## 7. 实现结果

- **代码**：`model/icluster_conf/cluster.go`（`LLMConfig` 字段 + `newAIConf` 映射 + provider 交叉校验）、`lib/validate/validate.go`（值域校验）、`endpoints/openapi_v1/product_cluster/create.go`（`normalizeLLMConfig` 透传）；
- **文档**：`api-define/OpenAPI接口定义/clusters.md`、`api-define/InnerAPI接口定义/server-data-conf.md`、`sys-design/` 四篇（模型层/数据库/接口层/InnerAPI 导出细节）；
- **测试**：`model/icluster_conf/cluster_test.go`（导出映射 + provider 交叉）、`lib/validate/validate_test.go`（值域）、`endpoints/openapi_v1/product_cluster/create_test.go`（透传/nil）、`test/integration/tests/schema/{openapi,innerapi}`（schema 字段）；
- **验证**：`go build ./...`、`go vet ./...`、`go test ./...` 全通过；model 语句覆盖率 85.3%（门禁 70%）；`go.mod` 未改动（验证方式见 `design-changes.md` §6）；
- **遗留**：契约与全链路集成用例需在依赖 bump 后执行；`bfe_tls` 下线适配是 bump 的前置条件。