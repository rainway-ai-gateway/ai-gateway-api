# 接口变更说明：cluster 级 `llm_config.max_concurrency_per_key`（每把上游 Key 的最大并发会话数）

## 1. 结论

- **无接口新增/删除**，无路径或方法变化；改动集中在 `llm_config` 的请求/响应字段与 `server_data_conf` 导出字段；
- 新增 `max_concurrency_per_key`（int，单位：会话数，**非速率**），cluster 级参数，对集群内每把上游 Key 统一生效；`0`（缺省）= 不限（默认）；
- 校验：`>= 0` 的整数；负数返回 422，不落库；`100000` 为文档建议软上限（不做硬校验）；
- **行为变更**：默认无（缺省 = 不限，链路零改动）；`>0` 时数据面启用 Redis 全局并发会话占用（详见 bfe 变更记录），控制面只承载配置与导出。

## 2. OpenAPI：`llm_config`

### 2.1 字段

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| `max_concurrency_per_key` | int | 每把上游 Key 的最大并发会话数 | N | 对集群内每把 Key 统一生效：同一时刻允许绑定到**单把 Key** 的最大会话数；会话以客户端 `ClientKeyId` 标识，同一会话的多个并发请求只计 1；`0`（缺省）= 不限（默认）；`>0` 为固定上限（如 `1`）。占用与 `key_affinity` 绑定同生命周期（绑定即占用、空闲超时或解绑释放），跨 BFE 实例全局生效；Redis（`[AIKeyAffinity]`）未配置/故障期间限制不生效（fail-open） | 非必填；默认值为 `0`（不限）；若传入须为 `>=0` 的整数；建议 `<= 100000`（软上限，不硬校验） |

### 2.2 请求示例（创建 / PATCH 均适用）

```json
{
    "name": "my-cluster",
    "llm_config": {
        "models": ["deepseek-chat"],
        "provider": "deepseek",
        "keys": [
            {"name": "key-primary", "weight": 70},
            {"name": "key-secondary", "weight": 30}
        ],
        "max_concurrency_per_key": 1
    }
}
```

### 2.3 响应示例（读集群时回显）

```json
"llm_config": {
    "models": ["deepseek-chat"],
    "provider": "deepseek",
    "keys": [
        {"name": "key-primary", "weight": 70},
        {"name": "key-secondary", "weight": 30}
    ],
    "max_concurrency_per_key": 1
}
```

> 升级前创建、之后未再保存的集群，其 `llm_config` 中没有该键，GET 回显为 `null`。**`null` 等效 `0`（不限）**，导出到数据面时控制面显式补 `0`，数据面 `0` 不发起任何 Redis 调用，行为与升级前完全一致。

### 2.4 校验

- `max_concurrency_per_key` 传入时须为 `>=0` 的整数，负数返回 422；
- 错误码沿用统一参数错误（422），集群配置不落库；
- 校验在 `ValidateLLMConfig`（`model/icluster_conf/llm_config_validate.go`）执行，规则与 BFE `AIConfCheck` 逐条对齐（防"控制面放行、BFE 加载失败"）；
- 与 `key_affinity` 的区间校验不同，本字段 **`0` 是合法值（表示不限）**，不做"排除 0"处理。

## 3. InnerAPI：`server_data_conf` 导出

`ClusterConf.Config.<cluster>.AIConf` 新增整数字段，导出时恒为具体值（无 `omitempty`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `AIConf.MaxConcurrencyPerKey` | int | 对应 OpenAPI `llm_config.max_concurrency_per_key`；`0` = 不限（缺省） |

导出片段：

```json
"AIConf": {
    "Type": 0,
    "Keys": [
        {"Name": "key-primary", "Key": "sk-...", "Weight": 70},
        {"Name": "key-secondary", "Key": "sk-...", "Weight": 30}
    ],
    "KeyPolicy": { "...": "..." },
    "MaxConcurrencyPerKey": 1
}
```

该字段非敏感，不参与 `enc$v1$` 字段级加密（沿用"仅 `Keys[].Key` 加密"规则）。

## 4. 行为变更与错误码

| 场景 | 变更前 | 变更后 |
|------|--------|--------|
| 未配置（缺省/null/0） | 无并发限制 | 无并发限制（**行为完全不变**，零 Redis 调用） |
| 配置 `max_concurrency_per_key=1` | 字段不存在，被忽略 | 每把 Key 同时最多 1 个会话绑定；第 2 个会话轮换其它 Key，全部满则合成 429 并可触发 cluster fallback |
| 配置 `max_concurrency_per_key=-1` | —— | `422`，提示 `llm_config.max_concurrency_per_key must be >= 0` |
| 数据面 `[AIKeyAffinity]` Redis 未配置/故障 | —— | 限制不生效（fail-open），请求照常放行并记录告警指标 |

错误文案沿用仓库既有的 `must be >= 0` 风格，统一返回参数错误结构，不落库、不回显部分更新。

## 5. 使用约束（排查时注意）

- `PATCH /clusters/{cluster_name}` 的 `llm_config` 是**整体替换**：只发 `{"llm_config":{"max_concurrency_per_key":1}}` 会清空 `models` / `keys` / `provider` 等其余字段。验证或脚本修改时请先 GET 集群、改完整个 `llm_config` 再提交。
- 该值是**单实例精确、多实例全局**上限（数据面 Redis 全局计数），不存在"N × BFE 实例数"的放大问题。
- 占用按**会话**（`ClientKeyId`）计，不按请求数计；会话空闲期间仍占槽直至 `key_affinity.ttl`（默认 600s）超时——调小 `ttl` 可加速让槽。
- `max_concurrency_per_key > 0` 的生产部署必须配置数据面 `[AIKeyAffinity]` Redis 并保障其可用性（故障期间限制静默失效，有指标告警）。
