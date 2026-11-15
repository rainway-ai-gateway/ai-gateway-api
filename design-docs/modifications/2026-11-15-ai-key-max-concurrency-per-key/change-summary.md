# 变更摘要：cluster 级 `llm_config.max_concurrency_per_key`（每把上游 Key 的最大并发会话数）

> **日期**: 2026-11-15 ｜ **状态**: 代码 ✅ · api-define ✅ · sys-design ✅ · 单测/集成 ✅ ｜ **接口字段**: 新增 `llm_config.max_concurrency_per_key`（非必填，默认 `0` = 不限）
>
> 配套文档：接口变更明细见 [api-changes.md](./api-changes.md)；控制面实施设计（数据流/依赖提升/测试计划/实施清单）见 [design-changes.md](./design-changes.md)。

## 1. 背景

一个 cluster 的上游 provider 通常配置多把 API-Key（`llm_config.keys`，BFE 加权随机 + 会话亲和选择）。目前网关对单把上游 Key 没有任何并发维度保护：任意数量会话可同时压在一把 Key 上。部分上游账号按并发会话授权/计费（如并发 1 的测试账号、单会话订阅），需要"一把 Key 同时只服务有限个会话"的硬隔离能力。

语义要点（本变更 OpenAPI 侧只需承载配置与导出，语义由 BFE 数据面实现）：

| 决策点 | 约定 |
|--------|------|
| 配置位置 | cluster 级单值 `llm_config.max_concurrency_per_key`，对集群内每把 Key 统一生效（不逐 Key 配置） |
| 计数维度 | 会话（客户端 `ClientKeyId`），同一会话多个并发请求只计 1 |
| 占用区间 | 与 Key 会话亲和性绑定同生命周期：绑定即占用、空闲 TTL 超时（同 `key_affinity.ttl`，默认 600s）或解绑释放；**不按"最后一个在飞请求结束"释放**（否则会话在多把 Key 间 ping-pong，抵消亲和性的 prompt cache 收益） |
| 超限行为 | 新会话选 Key 时槽满 → 跳过轮换；全部 Key 不可用 → BFE 走既有合成 429 路径，正常触发 cluster fallback |
| 一致性 | BFE 侧 Redis 全局计数（复用 `[AIKeyAffinity]` 句柄，ZSet + Lua），第一期即支持多 BFE 实例；Redis 未配置/故障时 fail-open（限制不生效 + 指标告警） |

## 2. 变更

| 文件 | 变更 |
| - | - |
| `model/icluster_conf/cluster.go`（`LLMConfig` `:250` 起） | 新增 `MaxConcurrencyPerKey *int \`json:"max_concurrency_per_key"\``（指针形态，与既有可空字段一致；`nil`/0 = 不限） |
| `model/icluster_conf/llm_config_validate.go`（`ValidateLLMConfig`） | 新增校验：若传入须为 `>=0` 的整数（与 BFE `AIConfCheck` 逐条对齐，防配置穿透） |
| `model/icluster_conf/cluster.go`（`newAIConf` `:1654`） | 导出 `AIConf.MaxConcurrencyPerKey = derefInt(llmConfig.MaxConcurrencyPerKey, 0)`；该字段非敏感，不参与 `enc$v1$` 加密（沿用"仅 `Keys[].Key` 加密"规则） |
| `design-docs/api-define/OpenAPI接口定义/clusters.md` | 表：LLM配置 新增 `max_concurrency_per_key` 行（含语义/合法性）；数据模型与请求示例同步；约束区新增校验条目 |
| `design-docs/api-define/InnerAPI接口定义/server-data-conf.md` | `AIConf` 字段表新增 `MaxConcurrencyPerKey` 行 |
| 测试 | `llm_config_validate_test.go` 增非法值用例（负数/`>100000` 若采用该上限）；`cluster_test.go` / 导出相关测试增字段透传与缺省补 0 断言；`test/integration/tests/clusters/` 增字段校验与导出用例 |

## 3. 兼容性

- 存量数据字段缺省 → 按 `0`（不限）处理，导出行为与升级前完全一致（BFE 侧 `0` 不发起任何 Redis 调用）；
- `keys` 数组的全量替换语义不受影响；新增/删除 Key 不改变该值；
- `GET /clusters` / `GET /clusters/{name}` 在 `llm_config` 中返回该字段（非敏感，随配置原样返回）；
- 仅当值 `>0` 时，BFE 数据面才依赖 `bfe.conf [AIKeyAffinity]` Redis（运行时 fail-open 降级，不阻断业务）。

## 4. 验证

- `make test-model-cover-gate`：`ValidateLLMConfig` 新分支覆盖；
- 集成：创建/更新集群携带 `max_concurrency_per_key` 正反向用例；导出 `server_data_conf` 断言字段落位与缺省补 0；
- 待联调：配 `max_concurrency_per_key=1` 的双 BFE 实例 + 两个会话并发请求，验证第三会话拿到合成 429 并 fallback。

## 5. 后续

- BFE 数据面实现（`AIConf.MaxConcurrencyPerKey` + Redis ZSet/Lua 占用闸）为独立变更，与本变更通过 InnerAPI 导出格式解耦；
- `ai-gateway-web` 集群表单需增加该字段（默认 0/不限）；
- 若未来出现同集群内异构 Key 需求，再下沉 `keys[].max_concurrency_per_key`（缺省继承 cluster 级，向后兼容）。
