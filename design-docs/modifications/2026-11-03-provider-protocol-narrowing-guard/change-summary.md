# provider 收窄 model_protocols 的引用完整性校验——变更摘要

## 1. 背景

集群的 `llm_config.ensure_reasoning_content`（协议名数组）在写入时会被校验为"元素 ⊆ 该 cluster 所用 provider 的 `model_protocols`"，且在导出时原样落到 bfe `AIConf.EnsureReasoningContent`。bfe 加载期 `AIConfCheck` 会再校验一次同样的约束——两份校验口径一致，所以"控制面放行的配置不会被 bfe 拒绝"这一不变量原本成立。

但 provider 的更新路径**没有**对引用它的 cluster 做级联校验，实测（`test/integration`，真实进程）：

| 步骤 | 实测结果 |
|---|---|
| 1. 双协议 provider（`["openai","anthropic"]`）下建 cluster，配 `ensure_reasoning_content=["anthropic"]` | 200 |
| 2. PATCH provider 收窄到 `["openai"]` | **200 success**（无校验） |
| 3. cluster 回读 | 仍为 `["anthropic"]` |
| 4. InnerAPI 导出 | `ModelProtocols=["openai"]` + `EnsureReasoningContent=["anthropic"]` —— 数据面必然拒绝的配置 |
| 5. 收窄后再 PATCH cluster 的无关字段 | 200（校验只在提交 `llm_config` 时触发） |

第 4 步的后果不是单个 cluster 失效：bfe `AIConfCheck` 报 `EnsureReasoningContent: protocol "anthropic" not in ModelProtocols`，**整份 `cluster_conf` 加载失败**——流量不中断（bfe 保留旧配置），但该导出主题版本停在旧版，conf-agent 持续告警，直到有人手工修配置。

## 2. 目标

1. provider 更新时做**引用完整性**校验：若收窄 `model_protocols` 会让某个引用它的 cluster 的 `ensure_reasoning_content` 变成非法，则拒绝本次更新（**409**），错误信息列出 provider 名、被移除的协议、以及引用它的 cluster 名；
2. 不改变正常路径：不收窄、或收窄后无 cluster 引用被移除协议时，行为与现在完全一致；
3. 与既有 `ProviderKeyRefChecker` / `ProviderModelRefChecker` 同构，避免为同一类问题引入第二套机制。

## 3. 范围

- **涉及面**：`model/iprovider`（hook 触发条件）、`model/icluster_conf`（新增 checker）、provider 更新端点（挂接 hook）；
- **不涉及面**：存储层、导出层、OpenAPI 字段、cluster 写入校验（保持不变）；
- **契约面**：provider 更新接口新增一种 409 冲突（引用完整性），需同步 `api-define`。

## 4. 关键决策

| 决策 | 说明 |
|------|------|
| 复用既有 hook 机制 | `UpdateProvider` 已有 `syncHooks`，`ProviderKeyRefChecker`/`ProviderModelRefChecker` 就是"provider 字段被删改时检查 cluster 引用"的同款；新增 `ProviderProtocolRefChecker` 即可，不新建机制 |
| 扩展 hook 触发条件 | 现有触发条件只覆盖 keys/models/instance_source/instance_pool，**协议变更不触发 hook**；需要把 `model_protocols` 变化纳入（hook 内部各自判断是否需要动作，未变化的会提前返回） |
| 409 而非 422 | 这是"请求本身合法，但与现有资源冲突"（引用完整性），与 model/key 引用检查一致；422 留给字段级非法值 |
| 只校验 `ensure_reasoning_content` | provider 自身的 `protocol_paths` 若含被移除协议，已由既有 `validateProviderProtocolPaths(effectivePaths, ...)` 在更新时拦截（参数错误）；cluster 侧另一处协议引用暂无其他字段 |
| 不做级联清理 | 静默改写用户配置（把别人的开关关掉）比拒绝更危险，也与"导出期静默裁剪"的取舍一致（不做） |

## 5. 关联文档

- 详细设计：`design-changes.md`
- 接口变更：`api-changes.md`
- 触发本变更的变更：`2026-11-03-cluster-reasoning-content-protocols/`（其 `design-changes.md` 风险 4 与 §6 路由表把该缺口登记为待决策项）
- bfe 侧加载期校验：`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` 的 `checkEnsureReasoningContent`

## 6. 实施阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | hook 触发条件纳入 `model_protocols` | ✅ 已完成 |
| 2 | `ClusterManager.ProviderProtocolRefChecker` + 端点挂接 | ✅ 已完成 |
| 3 | 单测（无引用放行 / 有引用 409 / 放宽放行） | ✅ 已完成 |
| 4 | 集成用例：CL-4-205 由 `t.Skip` 转正式断言，新增 CL-4-206 正向用例 | ✅ 已完成 |
| 5 | 文档同步（api-define 409 语义、clusters design.md、本目录） | ✅ 已完成 |

## 7. 实现结果

- provider 收窄 `model_protocols` 且被 cluster 的 `ensure_reasoning_content` 引用时返回 **409**，provider 与 cluster 回读零变更，导出保持合法；
- 放宽、收窄但无引用、其他字段更新均不受影响（既有 provider/update 用例全绿）；
- 单测 `TestProviderProtocolRefChecker` 6 个子用例 + `provider_test.go` 的 hook 触发用例覆盖；
- 集成 `CL-4-205`/`CL-4-206` 通过；
- 变更记录：`design-changes.md`（实现细节与取舍）、`api-changes.md`（409 语义与错误码）。