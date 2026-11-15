# 现有接口清单

## 1. 接口总览

| 序号 | 接口路径 | 功能描述 | 特殊参数 | 文档 |
|------|----------|----------|----------|------|
| 1 | `/configs/tls_conf/server_data_conf` | 导出 TLS/Server 配置 | `version` | [server-data-conf.md](./server-data-conf.md) |
| 2 | `/configs/gslb_data/gslb` | 导出 GSLB 配置 | `version`、`bfe_cluster`(必填) | [gslb.md](./gslb.md) |
| 3 | `/configs/gslb_data/cluster_table` | 导出集群表配置 | `version` | [cluster-table.md](./cluster-table.md) |
| 4 | `/configs/protocol/server_cert_conf` | 导出证书配置 | `version` | [server-cert-conf.md](./server-cert-conf.md) |
| 5 | `/configs/extra_files/{filename}` | 导出额外文件 | - | [extra-files.md](./extra-files.md) |
| 6 | `/configs/mod-api-key` | 导出 API-Key 配置 | `version` | [mod-api-key.md](./mod-api-key.md) |
| 7 | `/configs/mod-body-process` | 导出请求体处理配置 | `version` | [mod-body-process.md](./mod-body-process.md) |
| 8 | `/configs/rate-limit-policy` | 导出限流策略配置 | `version` | [rate-limit-policy.md](./rate-limit-policy.md) |
| 9 | `/configs/ai-route` | 导出 AI 路由配置 | `version` | [ai-route.md](./ai-route.md) |
| 10 | `/configs/epp_data/config` | 导出 EPP 配置（epp_config + assignment 全量视图，合并单端点） | `version` | [epp-data.md](./epp-data.md) |
| 11 | `/configs/ai-cache-rule` | 导出 AI 缓存规则配置 | `version` | [ai-cache-rule.md](./ai-cache-rule.md) |
| 12 | `/configs/ai-context-rule` | 导出 AI 上下文压缩规则配置 | `version` | [ai-context-rule.md](./ai-context-rule.md) |
| 13 | `/configs/traffic-mirror-rule` | 导出流量镜像规则配置 | `version` | [traffic-mirror-rule.md](./traffic-mirror-rule.md) |
| 14 | `/configs/mod-ai-intent` | 导出 AI 意图配置（intent_questions.data） | `version` | [mod-ai-intent.md](./mod-ai-intent.md) |

## 2. k8s_pools 接口（资源读写）

K8s 实例池维护接口，供 K8s 发现组件写入发现的实例快照。与导出接口不同，不支持 `version` 增量同步参数。

| 序号 | 接口路径 | Method | 功能描述 | 文档 |
|------|----------|--------|----------|------|
| 1 | `/k8s_pools/{name}/instances` | PUT | 全量替换 pool 实例列表（幂等 upsert） | [k8s-pools.md](./k8s-pools.md) |
| 2 | `/k8s_pools/{name}` | GET | 查询单个 pool 条目 | [k8s-pools.md](./k8s-pools.md) |
| 3 | `/k8s_pools` | GET | 查询全部 pool 列表 | [k8s-pools.md](./k8s-pools.md) |
| 4 | `/k8s_pools/{name}` | DELETE | 删除 pool（无引用保护） | [k8s-pools.md](./k8s-pools.md) |
| 15 | `/configs/mod-access-pb` | 导出访问日志动态配置（Header 白名单） | `version` | [mod-access-pb.md](./mod-access-pb.md) |

## 3. 特殊参数说明

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| version | string | 否 | 所有导出接口通用，用于增量同步，首次拉取可省略或传空 | 可选；无强制格式/长度校验；为空或未传时按首次拉取处理 |
| bfe_cluster | string | 是 | 调用 `/configs/gslb_data/gslb` 时为必填参数，用于指定 BFE 集群名称，返回该集群对应的 GSLB 调度配置 | 必填；长度至少为 1；必须对应实际存在且在 AI 集群 GSLB 调度中被引用的 BFE 集群名称 |

---

