# 同 cluster 多 API-Key 会话级亲和性——变更摘要

## 1. 背景

BFE 数据面已实现同 cluster 下多 API-Key 的会话级亲和性：当 cluster 绑定多个 API-Key 时，同一个 client_key_id 在 Redis 中绑定到固定的 API-Key，从而提升 provider 侧的缓存命中率，并避免因 Key 频繁切换导致的请求分散。

本次变更是为了让 `ai-gateway-api` 控制面能够将会话级 Key 亲和性配置下发到 BFE，使用户可以通过 OpenAPI `/clusters` 管理该能力。

## 2. 目标

1. 在 `ai-gateway-api` 控制面的 cluster 数据模型中增加 `llm_config.key_affinity` 配置。
2. 在 InnerAPI 导出 `server_data_conf` 时，将 `key_affinity` 映射为 BFE `AIKeyPolicy` 中的亲和性字段。
3. 保持向后兼容：`key_affinity` 为空或未传时，BFE 行为与之前一致（不开启亲和性）。
4. 不引入新的数据库表，复用 `clusters.llm_config` JSON 字段。

## 3. 范围

- **涉及面**：`ai-gateway-api` 控制面（OpenAPI `/clusters`、InnerAPI `/configs/tls_conf/server_data_conf`、配置导出）。
- **不涉及面**：BFE 数据面运行时逻辑、Redis 存储、Key 选择算法（已在 BFE 侧实现）。
- **数据面影响**：BFE `AIKeyPolicy` 新增 `SessionAffinity`、`SessionAffinityTTL`、`SessionAffinityRedisPrefix`、`SessionAffinityPenaltyEnable` 字段。

## 4. 关键决策

| 决策 | 说明 |
|------|------|
| 配置归属 cluster | 亲和性策略跟随 cluster 的 Key 策略，放在 `llm_config.key_affinity` 中。 |
| 复用 `llm_config` JSON 字段 | 不需要新增数据库列，降低迁移成本。 |
| 控制面默认值与 BFE 一致 | `enabled=true`、`ttl=600`、`redis_prefix="bfe:ai:key_affinity"`、`penalty_enable=true`。 |
| 仅控制开关与参数 | 运行时绑定关系由 BFE 维护，控制面只负责下发配置。 |

## 5. 关联文档

- 详细设计：`design-changes.md`
- 接口变更：`api-changes.md`
- BFE 侧修改说明：`bfe/docs/zh_cn/modifications/2026-08-26-ai-key-session-affinity/design-changes.md`
- BFE 侧设计文档：`bfe/docs/zh_cn/sys_design/multi_api_key.md`

## 6. 实施阶段

| 阶段 | 内容 | 状态 | 关键文件 |
|------|------|------|----------|
| 1 | BFE 数据面实现会话级 Key 亲和性 | ✅ 已完成 | `bfe/bfe_server/reverseproxy.go`、`bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go` |
| 2 | 控制面数据模型扩展（`llm_config.key_affinity`） | 🔄 待实现 | `ai-gateway-api/model/icluster_conf/cluster.go` |
| 3 | OpenAPI `/clusters` 校验与序列化适配 | 🔄 待实现 | `ai-gateway-api/endpoints/openapi_v1/product_cluster/*.go`、`ai-gateway-api/model/icluster_conf/validate.go` |
| 4 | InnerAPI 配置导出适配 | 🔄 待实现 | `ai-gateway-api/model/icluster_conf/cluster.go` 中 `newAIConf` |
| 5 | 测试与文档更新 | 🔄 待实现 | 单元测试、集成测试、OpenAPI/InnerAPI 接口定义文档 |

## 7. 实现结果

- 当前状态：BFE 数据面已完成实现，控制面尚未适配。
- 控制面适配后，用户创建/更新 cluster 时可通过 `llm_config.key_affinity` 开启会话级 Key 亲和性。
- InnerAPI 导出后，BFE `cluster_conf.data` 中对应 cluster 的 `AIConf.KeyPolicy` 将携带亲和性配置。
