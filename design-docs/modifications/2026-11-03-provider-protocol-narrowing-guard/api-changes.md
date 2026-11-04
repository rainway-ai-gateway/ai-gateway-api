# provider 收窄 model_protocols 的引用完整性校验——接口变更

## 1. 变更范围

只影响一个接口：

- `PATCH /open-api/v1/providers/{provider_name}`

无字段增删、无响应结构变化；新增一种**冲突结果**（409），与既有的 key/model 引用冲突同族。

## 2. 行为变化

### 2.1 新增 409 场景

当请求把 `model_protocols` 收窄（移除某些协议），且存在引用该 provider 的 cluster，其 `llm_config.ensure_reasoning_content` 里含有被移除的协议时，返回：

```
409 Conflict
provider <name> model_protocols <protocol> is referenced by cluster <cluster_name> ensure_reasoning_content
```

- 本次更新**整体不生效**（事务回滚），provider 自身读回与更新前逐字段一致；
- 引用它的 cluster 配置不变（不需要改，因为 provider 没变）。

### 2.2 不变的行为

| 场景 | 结果 |
|------|------|
| 放宽 `model_protocols`（增加协议） | 200，无额外校验 |
| 收窄但**没有** cluster 引用被移除的协议 | 200，行为与现在一致 |
| 收窄且 provider 自身 `protocol_paths` 含被移除协议 | 仍是既有的参数错误（`validateProviderProtocolPaths`），不进入本校验 |
| 其他字段更新（description/instance_pool/models/keys/…） | 行为不变 |
| `POST /open-api/v1/providers`（创建） | 行为不变 |

## 3. 合同条款（`design-docs/api-define/OpenAPI接口定义/providers.md`）

在 PATCH 的行为说明中补充一条，与既有 key/model 引用冲突并列：

> - `model_protocols` 作为数组，**显式提供时按全量替换**处理。收窄（移除协议）会校验无 cluster 的 `llm_config.ensure_reasoning_content` 仍引用被移除的协议；若被引用，返回 `409 Conflict`。放宽不受限制。

## 4. 错误码

| 场景 | HTTP | 说明 |
|------|------|------|
| 收窄 `model_protocols`，被 cluster 的 `ensure_reasoning_content` 引用 | 409 | 消息含 provider 名、协议名、引用它的 cluster 名 |
| `model_protocols` 为空或含非法值 | 422 | 既有校验，不变 |
| `protocol_paths` 的键不在（新）`model_protocols` 内 | 422 | 既有校验，不变 |