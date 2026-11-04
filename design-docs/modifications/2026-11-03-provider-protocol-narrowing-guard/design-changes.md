# provider 收窄 model_protocols 的引用完整性校验：设计变更说明

## 1. 概述

### 1.1 问题

`llm_config.ensure_reasoning_content` 的元素被约束为"⊆ 该 cluster 所用 provider 的 `model_protocols`"，写入 cluster 时校验、bfe 加载期再校验一次，形成双重防线。但这道防线只有"写入 cluster"这一侧：provider 侧收窄 `model_protocols` 时没有任何级联检查（实测 5 步见 `change-summary.md` §1），于是控制面能产出 `ModelProtocols=["openai"]` + `EnsureReasoningContent=["anthropic"]` 的导出配置，bfe `AIConfCheck` 拒绝加载整份 `cluster_conf`，导致该导出主题版本停更、conf-agent 持续告警。

### 1.2 变更范围

| 项 | 是否改动 |
|----|----------|
| `model/iprovider/provider.go`：`UpdateProvider` 的 hook 触发条件 | ✅ 纳入 `model_protocols` 变化 |
| `model/icluster_conf/cluster.go`：新增 `ProviderProtocolRefChecker` | ✅ |
| `endpoints/openapi_v1/provider/update.go`：挂接 hook | ✅ |
| 存储层 / 导出层 / OpenAPI 字段 / cluster 写入校验 | ❌ |
| `api-define`（providers.md PATCH 行为说明） | ✅ 补一条 409 |

## 2. 详细改动

### 2.1 hook 触发条件（`model/iprovider/provider.go`）

`UpdateProvider` 只在 `clusterConfigChanged` 为真时调用 hooks，而该标记原本只由 keys/models/instance_source/instance_pool 的变化置位——**协议变化不会触发任何 hook**，这是本缺陷能被放过的直接原因。

```go
// 新增：协议变化同样可能让引用它的 cluster 配置失效
origProtocols := param.ModelProtocols
...
if origProtocols != nil && !providerProtocolsEqual(existing.ModelProtocols, origProtocols) {
    clusterConfigChanged = true
}
```

配套新增 `providerProtocolsEqual(a, b []string) bool`（与既有 `providerModelsEqual` 同形）。

> 说明：`ValidateProviderParam` 要求 `model_protocols` 非空，因此 update 路径该字段总是显式提供；不做 `nil` 还原（现有 `origInstancePool/Keys/Models` 的 nil 还原是为了让 `applyProviderUpdate` 保留原值，协议不适用）。
>
> 触发 hooks 的副作用可控：`ProviderInstancePoolSyncer` 在实例池未变化时提前返回，`ProviderKeyRefChecker`/`ProviderModelRefChecker` 各自判断关注点是否变化，均不会产生额外写入。

### 2.2 新增 checker（`model/icluster_conf/cluster.go`）

与 `ProviderModelRefChecker` 同构：

```go
// ProviderProtocolRefChecker rejects a model_protocols narrowing that would
// leave a referencing cluster's ensure_reasoning_content invalid: bfe's
// AIConfCheck requires every entry to be declared by ModelProtocols, and an
// invalid export makes the whole cluster_conf topic fail to reload (bfe keeps
// the previous config while conf-agent keeps alerting).
func (cm *ClusterManager) ProviderProtocolRefChecker(ctx context.Context,
    oldProvider, newProvider *iprovider.Provider) error {
    if newProvider == nil {
        return nil
    }
    declared := make(map[string]bool, len(newProvider.ModelProtocols))
    for _, p := range newProvider.ModelProtocols {
        declared[p] = true
    }

    clusters, err := cm.storager.FetchClusterList(ctx, nil)
    if err != nil {
        return err
    }
    for _, cluster := range clusters {
        if cluster.LLMConfig == nil || cluster.LLMConfig.Provider == nil {
            continue
        }
        if *cluster.LLMConfig.Provider != newProvider.Name {
            continue
        }
        for _, p := range cluster.LLMConfig.EnsureReasoningContent {
            if !declared[p] {
                return xerror.WrapConflictErrorWithMsg(
                    "provider %s model_protocols %s is referenced by cluster %s ensure_reasoning_content",
                    newProvider.Name, p, cluster.Name)
            }
        }
    }
    return nil
}
```

要点：

- **不比对 old/new 的全集差异**，直接对"引用它的 cluster 的每个协议"做存在性判断——放宽时天然放行，收窄时精确命中，也顺带覆盖"存量数据本来就不一致"的情形；
- 返回 `409 Conflict`（`xerror.WrapConflictErrorWithMsg`），与 key/model 引用冲突同族；
- 遍历 `FetchClusterList` 时按 provider 名过滤，避免误伤其他 provider 的集群。

### 2.3 端点挂接（`endpoints/openapi_v1/provider/update.go`）

```go
if err := container.ProviderManager.UpdateProvider(req.Context(), name, param,
    container.ClusterManager.ProviderInstancePoolSyncer,
    container.ClusterManager.ProviderKeyRefChecker,
    container.ClusterManager.ProviderModelRefChecker,
    container.ClusterManager.ProviderProtocolRefChecker, // 新增
); err != nil {
```

## 3. 为什么不是"级联清理"

另一种做法是 provider 收窄时自动删掉 cluster 上的相关协议（把用户的开关静默关掉）。不采纳：

- 静默改写用户配置比拒绝更危险——用户不知道自己的开关被关了，反而更难排查；
- 与本仓既有取舍一致：key/model 的引用冲突也是**拒绝**（409），而非级联清理；
- 与"导出期静默裁剪"同类，属于"配置与生效范围静默分叉"，在 `2026-11-03-cluster-reasoning-content-protocols/design-changes.md` §4 已明确不做。

## 4. 兼容性

- 无引用（或只放宽）时行为与现在逐字节一致；
- 存量数据若已处于非法状态（例如在本次修复前就收窄过），本次更新**不会**自动修好；此时任何一次带 `llm_config` 的 cluster PATCH 都会因既有校验被拒绝，需要人工修正——这一点写进 `api-changes.md` 与运维说明；
- 新增 409 只出现在"收窄且被引用"这一新场景，不改变既有错误码分布。

## 5. 测试

| 层 | 内容 |
|----|------|
| 单测 | `ProviderProtocolRefChecker`：无引用 cluster → nil；引用被移除协议 → 409；放宽 → nil；其他 provider 的 cluster 不受影响 |
| 单测 | `providerProtocolsEqual` 与 hook 触发条件（协议变化置位 `clusterConfigChanged`） |
| 集成 | 完整链路：双协议 provider + cluster 配 `["anthropic"]` → PATCH provider 收窄 → 409；provider 读回逐字段不变；cluster 读回不变；导出仍合法（`ModelProtocols` 仍含 `anthropic`） |
| 集成 | 正向：收窄到"仍覆盖所有被引用协议"→ 200；放宽 → 200 |
| 回归 | provider/update 既有用例全绿；clusters/update 中被挂起的 CL-4-205 转正式用例 |

## 6. 实施阶段

| 阶段 | 内容 | 状态 |
|------|------|------|
| 1 | hook 触发条件 + `providerProtocolsEqual` | ✅ 已完成 |
| 2 | `ProviderProtocolRefChecker` + 端点挂接 | ✅ 已完成 |
| 3 | 单测 | ✅ 已完成 |
| 4 | 集成用例（CL-4-205 转正式 + 新增 CL-4-206） | ✅ 已完成 |
| 5 | 文档：`api-define/providers.md`、`tests/clusters/design.md` §14、变更文档 | ✅ 已完成 |

## 7. 实现结果

- **代码**：`model/iprovider/provider.go`（`origProtocols` 捕获 + `clusterConfigChanged` 纳入协议变化 + `providerProtocolsEqual`）、`model/icluster_conf/cluster.go`（`ProviderProtocolRefChecker`）、`endpoints/openapi_v1/provider/update.go`（挂接 hook）；
- **单测**：`TestProviderProtocolRefChecker`（6 个子用例：无引用/收窄被引用 409/放宽/未引用协议/跳过无 llm_config 或 provider/nil）、`provider_test.go` 两个 hook 触发用例（协议变化触发、未变化不触发）；
- **集成**：CL-4-205（收窄被引用 → 409 + provider 与 cluster 回读零变更 + 导出仍合法）、CL-4-206（收窄但无引用 → 200）；
- **验证**：`go build ./...`、`go vet ./...`、`go test ./model/... ./lib/... ./endpoints/...` 全通过；`test/integration` 的 `./tests/clusters/...`、`./tests/provider/...`、`./tests/schema/...` 全通过（重建二进制后）；
- **文档**：`api-define/OpenAPI接口定义/providers.md` 补 409 条款；`tests/clusters/design.md` §14 的 CL-4-205/206 与设计要点同步；本变更的 `change-summary.md`/`api-changes.md` 已更新。

## 8. 变更沉淀评估（Step 6）

不新增 `sys-design/details/` 文档：guard 复用的是既有 hook 机制（`ProviderKeyRefChecker`/`ProviderModelRefChecker` 的同族），没有引入新流程或新状态；相关系统设计（provider 与 cluster 的引用关系）已在 `sys-design/模型层设计文档.md` 描述。本变更的价值在于把"provider 字段收窄 → 引用方失效"这一类问题在 provider 更新路径上闭环，属既有机制的补齐。