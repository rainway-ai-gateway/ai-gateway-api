# 接口变更说明：Key 惩罚时长可配置（issue #1448）

## 1. 结论

- **无接口新增/删除**，无路径或方法变化；改动集中在 `llm_config.key_affinity` 的请求/响应字段与 `server_data_conf` 导出字段；
- 新增 `penalty_ttl_429`（默认 60）与 `penalty_ttl_auth`（默认 300），单位秒，均为整数；
- 新增范围校验：429 档 `[10, 600]`、auth 档 `[60, 1800]`；`0` / 负数 / 越界返回 422，不落库；
- **行为变更**：凭证档惩罚默认时长 3600 秒 → 300 秒（仅在新数据面上生效）。

## 2. OpenAPI：`llm_config.key_affinity`

### 2.1 字段

| 参数名 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
|--------|------|----------|------|----------|------------|
| `penalty_ttl_429` | int | 上游 429 时的惩罚时长 | N | 单位秒，默认 `60`；与 `ttl`（绑定空闲超时，默认 600）无关；`0` / 缺省不是「关闭」，缺省取默认值 | 非必填；若传入须为 `[10, 600]` 的整数 |
| `penalty_ttl_auth` | int | 上游 401/402/403 时的惩罚时长 | N | 单位秒，默认 `300`；语义同上 | 非必填；若传入须为 `[60, 1800]` 的整数 |

### 2.2 请求示例（创建 / PATCH 均适用）

```json
{
    "name": "kimi",
    "llm_config": {
        "models": ["kimi-k2"],
        "provider": "kimi",
        "key_affinity": {
            "enabled": true,
            "ttl": 600,
            "redis_prefix": "bfe:ai:key_affinity",
            "penalty_enable": true,
            "penalty_ttl_429": 30,
            "penalty_ttl_auth": 120
        }
    }
}
```

### 2.3 响应示例（读集群时回显）

```json
"key_affinity": {
    "enabled": true,
    "ttl": 600,
    "redis_prefix": "bfe:ai:key_affinity",
    "penalty_enable": true,
    "penalty_ttl_429": 30,
    "penalty_ttl_auth": 120
}
```

> 升级前创建、之后未再保存的集群，其 `key_affinity` 里没有这两个键，GET 会回显 `null`。**`null` 表示未配置，等效默认 60 / 300**，与既有四个字段的处理方式一致；导出到数据面时控制面会补默认值，因此「界面 null、实际 60」不代表不一致。

### 2.4 校验

- `penalty_ttl_429` 传入时须为 `[10, 600]` 的整数；
- `penalty_ttl_auth` 传入时须为 `[60, 1800]` 的整数；
- 上述范围已排除 `0` 与负数，错误码沿用统一参数错误（422），集群配置不落库；
- 校验在请求绑定阶段执行（`UpsertParam.Validate` → `validate.LLMConfig`），早于默认值填充，因此「缺省补默认」与「显式 0 被拒」同时成立。

## 3. InnerAPI：`server_data_conf` 导出

`ClusterConf.Config.<cluster>.AIConf.KeyPolicy` 新增两个整数字段，值恒为具体秒数（无 `omitempty`，不会缺省）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `KeyPolicy.SessionAffinityPenaltyTTL429` | int | 对应 OpenAPI `llm_config.key_affinity.penalty_ttl_429`，默认 `60` |
| `KeyPolicy.SessionAffinityPenaltyTTLAuth` | int | 对应 OpenAPI `llm_config.key_affinity.penalty_ttl_auth`，默认 `300` |

导出片段：

```json
"KeyPolicy": {
    "Strategy": "weighted_random",
    "MaxRetries": 3,
    "RetryBackoffInitial": 500,
    "RetryBackoffMax": 5000,
    "SessionAffinity": true,
    "SessionAffinityTTL": 600,
    "SessionAffinityRedisPrefix": "bfe:ai:key_affinity",
    "SessionAffinityPenaltyEnable": true,
    "SessionAffinityPenaltyTTL429": 60,
    "SessionAffinityPenaltyTTLAuth": 300
}
```

## 4. 行为变更与错误码

| 场景 | 变更前 | 变更后 |
|------|--------|--------|
| 未配置、上游 401 | 惩罚 3600 秒（数据面常量） | 惩罚 300 秒（控制面导出 300，数据面同值） |
| 未配置、上游 429 | 惩罚 60 秒 | 惩罚 60 秒（不变） |
| 配置 `penalty_ttl_auth=120` | 字段不存在，被忽略 | 惩罚 120 秒，改后无需重启 |
| 配置 `penalty_ttl_429=0` | —— | `422`，提示 `llm_config.key_affinity.penalty_ttl_429 must be between 10 and 600` |
| 配置 `penalty_ttl_auth=99999` | —— | `422`，提示 `llm_config.key_affinity.penalty_ttl_auth must be between 60 and 1800` |

错误文案沿用仓库既有的 `must be between X and Y` 风格，统一返回参数错误结构，不落库、不回显部分更新。

## 5. 使用约束（排查时注意）

- `PATCH /clusters/{cluster_name}` 的 `llm_config` 是**整体替换**：只发 `{"llm_config":{"key_affinity":{...}}}` 会清空 `models` / `keys` / `provider`，并把未传的 `key_affinity` 子字段按默认值重置。验证或脚本修改时请先 GET 集群、改完整个 `llm_config` 再提交。
- 惩罚时长只影响**新写入**的惩罚键；已存在的长惩罚需等过期或人工删除 Redis 键（`{prefix}:penalty:{cluster}:{key_name}`）。
- `penalty_enable=false` 时不写惩罚键，这两个字段不生效但仍可保存；单可用 Key 集群不参与惩罚过滤，字段无实际效果。