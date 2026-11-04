# 思考模式推理内容补齐（ensure_reasoning_content）：ai-gateway-api 控制面设计变更说明

## 1. 概述

### 1.1 变更背景

BFE 数据面新增集群 `AIConf.EnsureReasoningContent`（协议名数组，空 = 关闭）：在 AI 转发路径上，为"最后一条 `user` 消息之后"缺 `reasoning_content` 的 assistant 消息补空串（`openai`）或空 thinking 块（`anthropic`），定点写入、只增不改、异常 fail-open。数据面实现见 `bfe` 仓 commit `3b5d05a6`（分支 `v1.9.0-dev`）：`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go`（字段 + `AIConfCheck`）、`bfe/bfe_server/reverseproxy_ai_reasoning.go`（归一化实现）、`bfe/bfe_server/proxy_state.go`（四个指标）。

控制面此前无法管理该配置，只能手工编辑 `cluster_conf.data`——而该文件由 `route_rule` 导出主题生成，下一次导出就会覆盖，既无审计也无法回滚。

### 1.2 配置归属：cluster 级 `llm_config.ensure_reasoning_content`

| 维度 | 取值 | 理由 |
|------|------|------|
| 挂在 cluster 而非 provider | `LLMConfig` | 同一 provider 可能有多个集群实例；按集群才能分批灰度与单点回滚。挂 provider 会让同 provider 的全部集群同时改变 |
| 类型 | `[]string` | 协议维度独立：同一 provider 可能同时提供 openai 与 anthropic 入口，注入落点不同；协议增加到 3 个以上只需加元素，不必扩枚举 |
| 值域 | `openai`、`anthropic` | 只有这两种协议有注入实现；`gemini` 请求体为 `contents/parts`，无 `reasoning_content` 落点 |

### 1.3 变更范围

| 层 | 是否改动 | 说明 |
|----|----------|------|
| 数据模型 | ✅ | `model/icluster_conf/cluster.go`：`LLMConfig` 加字段 + 导出映射 |
| 校验 | ✅ | `lib/validate/validate.go`（值域）+ `ClusterManager.validateClusterLLMConfigAgainstProvider`（provider 交叉） |
| 端点透传 | ✅ | `endpoints/openapi_v1/product_cluster/create.go` 的 `normalizeLLMConfig` |
| 存储 | ❌ | `clusters.llm_config` 为 JSON 文本列（`storage/rdb/internal/dao/table_clusters.go:60`），无 DDL |
| 导出框架 | ❌ | `route_rule` 导出整体序列化 `cluster_conf.BfeClusterConf`（`model/iroute_conf/exporter.go:51`），新字段随之透传 |
| conf-agent / k8s | ❌ | 下发链路已通 |

### 1.4 兼容性

- **字段缺省**：不写该字段 → `LLMConfig.EnsureReasoningContent` 为 nil → 导出中不出现 → BFE 侧零值即关闭，请求与旧版本逐字节一致；
- **存量数据**：无需迁移（JSON 列反序列化后该字段为 nil）；
- **数据面版本**：老 BFE 不认识该键时按未知字段忽略，不会加载失败；但要使配置真正生效需部署含该字段的数据面版本（见 §6）；
- **API 兼容**：`PATCH` 的 `llm_config` 本就整体替换，新增字段不改变既有语义。

## 2. 数据模型改动

### 2.1 `model/icluster_conf/cluster.go`

**(a) `LLMConfig` 加字段**（`:245` 起，紧随 `NormalizeUpstreamError`）：

```go
type LLMConfig struct {
    ...
    NormalizeUpstreamError *NormalizeUpstreamError `json:"normalize_upstream_error"`
    // EnsureReasoningContent lists the model access protocols for which the
    // gateway fills an empty reasoning_content on the assistant messages that
    // sit after the last user message (the window the upstream validates).
    // Only "openai" and "anthropic" are accepted: gemini has no such field in
    // its contents/parts body. Empty/nil disables the behaviour.
    EnsureReasoningContent []string `json:"ensure_reasoning_content"` // 新增
    PriceMissAction        *string  `json:"price_miss_action"`
    ...
}
```

**(b) 不合成默认值**：nil 即未配置，与 `normalize_upstream_error` 同策略——默认语义由 BFE 侧决定（零值 = 关闭），避免控制面与数据面两处默认值来源。

### 2.2 存储层

**无需修改**。`clusters.llm_config` 是 `text` 列（`db_ddl.sql:90`），按 JSON 存取；`db_ddl.sql` / `db_ddl_sqlite.sql` 不变。

### 2.3 与 BFE 侧类型的映射

| 控制面（`LLMConfig`） | BFE（`cluster_conf.AIConf`） | 缺省语义 |
|------|------|------|
| `ensure_reasoning_content` (`[]string`) | `EnsureReasoningContent` (`[]string`) | nil → 导出中不出现 → BFE 零值关闭 |

两处唯一差别是 JSON 键命名风格：控制面 snake_case（与 `llm_config` 其余字段一致），BFE PascalCase（与 `ModelProtocols` 一致）。**这两个名字必须在文档与契约测试中成对出现**，避免旧问题重演（导出结构与加载器字段漂移）。

## 3. 校验规则

分两层，钩子均已存在：

### 3.1 值域（`lib/validate/validate.go` 的 `LLMConfig` 校验，与 `normalize_upstream_error` 同位置）

| 规则 | 错误 |
|------|------|
| 每个元素非空 | `llm_config.ensure_reasoning_content must not contain empty or duplicate entries` |
| 无重复项 | 同上 |
| 元素 ∈ `{openai, anthropic}` | `llm_config.ensure_reasoning_content contains unsupported protocol "x" (only openai, anthropic)` |

值域**刻意不复用** `iprovider.ValidModelProtocols`（后者含 `gemini`）：协议名合法性沿用其命名与查表方式，但本字段的允许集合更窄，且与 BFE `AIConfCheck` 的 `reasoningContentProtocols` 集合逐条一致。

### 3.2 provider 交叉（`ClusterManager.validateClusterLLMConfigAgainstProvider`，`cluster.go:654`）

该方法在 `CreateCluster`（`:500`）与 `UpdateCluster`（`:835`）路径上都会被调用，已有 provider 记录在手，新增分支：

| 规则 | 错误 |
|------|------|
| 每个元素 ∈ `provider.ModelProtocols`；`ModelProtocols` 为空时按 `["openai"]` 对待 | `ensure_reasoning_content "anthropic" not supported by provider deepseek (model_protocols=[openai])` |

> 空 `ModelProtocols` 按 `["openai"]` 处理的口径来自 BFE `modelprotocol.Supports`——控制面若按"空 = 允许任意值"校验，就会放行一个数据面必然拒绝的配置。

## 4. 配置导出逻辑改动

`model/icluster_conf/cluster.go` 的 `newAIConf`（`:1579`）在 `NormalizeUpstreamError` 映射块之后新增：

```go
// nil 直通：导出为 null（结构体不带 omitempty，与 NormalizeUpstreamError 实测一致），
// BFE 反序列化为 nil，零值即关闭
aiConf.EnsureReasoningContent = llmConfig.EnsureReasoningContent
```

导出链路（`newAIConf` → `ClusterConf` → `route_rule` 导出 → `cluster_conf.data` → conf-agent 热加载）对新字段无感知，无需改动。

## 5. 易漏的手工列举点（实施 checklist）

1. `model/icluster_conf/cluster.go`：`LLMConfig` 字段（§2.1）+ `newAIConf` 映射（§4）；
2. `lib/validate/validate.go`：值域校验（§3.1）；
3. `model/icluster_conf/cluster.go`：`validateClusterLLMConfigAgainstProvider` 交叉校验（§3.2）；
4. **`endpoints/openapi_v1/product_cluster/create.go` 的 `normalizeLLMConfig`（`:436`）：该函数手工重建 `LLMConfig` 结构体，`create` 与 `patch`（`update_basic.go:137`）共用此唯一咽喉点——漏加一行即字段被静默丢弃，且不报错。这是本变更最容易踩的坑（`normalize_upstream_error` 落地时踩过同款）**；
5. `test/integration/tests/schema/openapi/cluster.go`、`test/integration/tests/schema/innerapi/innerapi_schema_test.go`：schema 断言补字段；
6. `model/icluster_conf/cluster_test.go`：导出器单测（配置 → `cluster_conf.data` 字段存在且值正确；无配置 → 键不出现）；
7. `endpoints/openapi_v1/product_cluster/create_test.go`：`normalizeLLMConfig` 透传与 nil 保持；
8. 文档：`design-docs/api-define/OpenAPI接口定义/clusters.md` 的 `llm_config` 字段表、`design-docs/api-define/InnerAPI接口定义/` 中 `cluster_conf` 的结构说明、`design-docs/sys-design/` 三件套（模型层/接口层/数据库设计）的 `llm_config` 结构说明。

## 6. 依赖与发布顺序

api 直接 import bfe 的 `cluster_conf` 包（`model/icluster_conf/cluster.go:37`），当前 `go.mod:8` 钉在 `bfe v1.8.10-0.20261030144950-39c15e1079e9`——**不含**新字段，因此直接写映射代码编译不过（实测报错 `aiConf.EnsureReasoningContent undefined`）。

```
① BFE 数据面合入（已完成：commit 3b5d05a6，分支 v1.9.0-dev）
        ↓
② bfe 提供一个含该字段、且 api 可编译的版本 → go.mod bump
        ↓
③ 控制面合入本方案（字段 + 两层校验 + 导出；此时无 cluster 配置该字段，全链路无变化）
        ↓
④ PATCH 单个 DeepSeek 集群 ensure_reasoning_content=["openai"] → 灰度（数据面 SC45 判据）
        ↓
⑤ 其余 DeepSeek 集群逐个开启 → 全量
        ↓
⑥ 回滚 = PATCH 提交 []
```

**第 ② 步的障碍与当前处置**：

- 当前 `bfe` 工作分支（`v1.9.0-dev`）已按 TLS 改造下线了 `bfe_tls` 包，而 api 仍 import 它（`model/iprotocol/certificate.go:24`、`:174`）；把 bfe 指到该分支后 api 编译报 `no required module provides package github.com/bfenetworks/bfe/bfe_tls`；
- 同时该分支的依赖集已前进（`bfe_expr` → `expr-lang/expr`），`bfe_tls` 的 SM2 依赖 `tjfoc/gmsm` 随之消失；
- **api 侧的 `bfe_tls` 下线适配已完成**（见 `2026-11-03-drop-bfe-tls-import/`：证书 key pair 校验改用标准库 `crypto/tls`）；适配后 `replace` 到该分支，全仓 `go build ./...`、`go vet ./...`、`go test ./model/... ./lib/... ./endpoints/...` 与 `test/integration` 子模块构建全部通过；
- 因此 `go.mod` 目前**保留一条带 `TODO(dep-bump)` 注释的临时 `replace`**，指向 fork（`github.com/mileszhang2016/bfe`）上 `v1.9.0-dev` 提交 `3b5d05a6` 的伪版本 `v1.4.1-0.20261103140728-3b5d05a68931`（配套 `go mod tidy` 结果：`+ expr-lang/expr`、`- tjfoc/gmsm`）。**不依赖本地目录检出**，CI 可直接构建；上游发布含 `EnsureReasoningContent` 的版本后删除该行并改为正常版本 bump。

落地时的依赖路线（二选一，评审定）：

| 路线 | 内容 | 代价 |
|---|---|---|
| A（推荐，前置改造已完成） | 上游 bfe 合并并发版后 bump；api 侧 `bfe_tls` 下线适配已做完，不再阻塞 | 仅等上游节奏 |
| B（临时） | bfe 出一个仅含该字段的维护版本（如 v1.8.x + 字段），api bump 到它 | 依赖一个非主线版本，需跟踪；且仍需 `bfe_tls` 适配，故收益已不大 |

发布顺序本身无强耦合：控制面先上（字段全空）或数据面先上（不认识该键则忽略）都安全；唯一硬约束是第 ② 步的依赖可见性。

## 7. 测试计划与实测结果

| 层 | 内容 | 结果 |
|----|------|------|
| 单测 | `newAIConf` 映射三态：`["openai"]` / `["openai","anthropic"]` / nil（导出中键不存在） | ✅ `cluster_test.go` 新增 `ensure_reasoning_content export` 子测试 |
| 单测 | 值域校验：合法值通过；`gemini`、未知协议、空串、重复项报参数错误 | ✅ 新增 `TestLLMConfigEnsureReasoningContent` |
| 单测 | provider 交叉：`["openai","anthropic"]` 通过；`["openai"]` 下配 `anthropic` 报错；provider 空 `model_protocols` 配 `openai` 通过、配 `anthropic` 报错 | ✅ 新增 `TestValidateClusterLLMConfigEnsureReasoningContent` |
| 单测 | `normalizeLLMConfig`：字段透传、nil 保持 | ✅ `create_test.go` 新增两个子测试 |
| 契约 | 导出 `cluster_conf` 用 BFE 包反序列化 + `AIConfCheck` | ⏳ 依赖 bump 后在集成环境执行（当前 api 无法编译到含该字段的正式 bfe 版本，见 §6） |
| 集成 | `POST /clusters` 带字段 → `GET` 回读 → InnerAPI 导出可见 → `PATCH []` 后键消失 | ✅ `tests/clusters/create`（CL-1-201~207）、`tests/clusters/update`（CL-4-201~206） |
| 导出契约 | `AIConf.EnsureReasoningContent` 的键名方向（snake→Pascal）、取值、未配置时 `null`、snake_case 不泄漏 | ✅ `tests/innerapi/tls_conf` 的 `IN-ERC-1-001~003`（design.md §20） |
| 回归 | 未配置该字段的 cluster 导出结果不变 | ✅ 既有 `cluster_test.go` / `validate_test.go` / `product_cluster` 测试全绿 |

本地验证命令与结果：`go build ./...`、`go vet ./...`、`go test ./...`（覆盖 `model/`、`lib/`、`endpoints/`）全通过；model 语句覆盖率 **85.3%**（门禁 70%）。验证时 `go.mod` 用的是 §6 描述的临时 `replace => ../bfe`（见该节"当前处置"）。

## 8. 实施阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | BFE 数据面实现（字段 / `AIConfCheck` / 归一化 / 四指标 / 单测 / SC45 14 例） | ✅ 已完成（bfe `3b5d05a6`） |
| 2 | 依赖可见性（上游发版 bump；受 `bfe_tls` 下线阻塞，见 §6） | ⏳ 待外部条件 |
| 3 | 数据模型 + 导出映射（§2.1 / §4） | ✅ 已完成 |
| 4 | 两层校验（§3） | ✅ 已完成 |
| 5 | 端点透传（§5-4） | ✅ 已完成 |
| 6 | 文档同步 + schema/单测（§5、§7）；契约与全链路集成用例待阶段 2 | ✅ 已完成（除依赖阻塞项） |

预估 1.5–2 人日：没有建表、没有新端点、不改导出框架，主要成本在依赖 bump 与测试补齐。

## 10. 变更沉淀评估（Step 6）

本变更**不在 `sys-design/details/` 新增细节文档**，理由：它复用既有机制（cluster `llm_config` JSON 列 → `newAIConf` 映射 → `route_rule` 导出），没有引入新的数据流、状态存储或合并逻辑；结构说明已分别沉淀在 `sys-design/模型层设计文档.md`（`LLMConfig` / `AIConf` 结构、校验要点、导出步骤）、`sys-design/数据库设计文档.md`（`llm_config` 列说明）、`sys-design/接口层设计文档.md`（字段语义）与 `sys-design/details/InnerAPI配置导出与版本控制.md`（导出字段清单）。后续若出现"provider 级能力开关"的第二个同类字段，再考虑把它们合并成一篇细节文档。

## 9. 风险

| # | 风险 | 等级 | 处置 |
|---|---|---|---|
| 1 | `normalizeLLMConfig` 漏加字段导致静默丢弃 | 中 | 单测覆盖 create 与 patch 两条路径；代码评审 checklist 第 4 项 |
| 2 | 导出键名与 BFE 结构漂移 | 中 | 契约测试（导出片段用 BFE 包反序列化 + `AIConfCheck`） |
| 3 | 依赖 bump 时序错（api 先合入） | 中 | §6 顺序；CI 编译即暴露 |
| 3b | ~~依赖版本无法就地 bump：bfe 分支已下线 `bfe_tls`，api 仍 import 它~~ | ~~高~~ 已解除 | api 侧 `bfe_tls` 下线适配已完成（`2026-11-03-drop-bfe-tls-import/`），`replace` 到 bfe 分支后全仓可编译；剩余仅为等待上游发布含该字段的版本 |
| 4 | provider 收窄 `model_protocols` 后，既有 cluster 配置变成非法 | ~~中~~ 已修 | 已落地引用完整性校验（收窄且被引用 → 409），见 `2026-11-03-provider-protocol-narrowing-guard/`；集成用例 CL-4-205/206 |
| 5 | 控制面值域与 BFE `AIConfCheck` 口径不一致 | 低 | 允许集合逐条对齐（仅 openai/anthropic）、交叉校验对齐 `Supports` 的空默认语义；契约测试兜底 |

## 10. 非本仓登记点

| 位置 | 改动 | 状态 |
|---|---|---|
| `bfe` 数据面 | 字段 + `AIConfCheck` + 归一化 + 四指标 | ✅ 已完成（`3b5d05a6`） |
| conf-agent / k8s configmap | 无 | 零改动 |
| ai-gateway-web（dashboard） | cluster 表单增加协议多选（`ensure_reasoning_content`） | 一期走 OpenAPI；表单项可随本变更同期或二期 |
| integration-test 仓 | 全链路用例：PATCH → 导出 → conf-agent → BFE 生效 | 待落地 |