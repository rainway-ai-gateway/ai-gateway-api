# k8s_pool 实时解析改造（方案 B：删除实例镜像与 fan-out）变更摘要

## 1. 背景

线上问题：`k8s_pools/k8s_default_vllm-backend_http_szyf` 已有 4 个实例，但 provider `yf-vllm-backends`（`instance_source=k8s_pool`）的 `k8s_instance_pool` 镜像为 `[]`（该 provider 创建于 pool PUT 之后一小时，期间没有任何 pool PUT），导致引用它的 cluster `yf-vllm-cluster` 派生池为空，`GET /inner-api/v1/configs/gslb_data/cluster_table` 返回 `"yf-vllm-cluster": {}`。

根因链：

| # | 环节 | 当前实现 | 问题 |
|---|---|---|---|
| 1 | 镜像写入 | `model/ik8s_pool/k8s_pool.go` `syncProviderMirrors` 是 `providers.k8s_instance_pool` 的唯一写入方 | 只在 `PUT/DELETE /k8s_pools` 时执行 |
| 2 | provider 创建 | `model/iprovider/provider.go` `CreateProvider` 不读 `k8s_pools`；storage 层把镜像固定写成 `[]` | 先有 pool、后建 provider 时镜像永远为空 |
| 3 | provider 切换 | `UpdateProvider` 不解析 `k8s_pools`；`EffectiveInstancePool` 直接读镜像 | `instance_pool -> k8s_pool` 时"有效池"为空；`k8s_pool_name A -> B` 时 hook 不触发 |
| 4 | cluster 快照 | `model/icluster_conf/cluster.go` 用 `EffectiveInstancePool(provider)` 建派生池 | 镜像为空 → 派生池为空 |
| 5 | 导出 | `model/icluster_conf/exporter.go` 跳过空实例的 subcluster | 输出 `"cluster": {}` |
| 6 | 自愈 | 无启动 backfill / 周期 reconcile | 只能等下一次 pool PUT，不保证发生 |

"fan-out 没执行"只说对了一半：fan-out 只在 pool PUT/DELETE 时触发，provider 直接以 k8s 模式创建时根本没有 hook。本问题的本质是**"有效池"被定义成镜像，而镜像的唯一写入时机与真实引用建立时机解耦**。只补 fan-out 或只修某一个入口，都会留下其他入口的同类空洞。

> 关联问题单：`20261026yf-vllm-cluster的实例列表是空.md`
> 关联评估：`20261027k8s-pool去掉fanout导出实时解析评估.md`

## 2. 目标

1. 修复"先有 pool、后建 provider、再建 cluster，导出为空"的问题。
2. k8s 模式下 provider / cluster / 导出 / batch 对实例的认知完全同源（`k8s_pools` 为唯一事实源）。
3. 删除 `providers.k8s_instance_pool` 镜像字段及所有读写路径。
4. 删除 k8s 模式的实例 fan-out、backfill 语义。
5. 保持手工来源（`instance_pool`）行为不变。
6. 保持 `/k8s_pools` InnerAPI 路径、鉴权、请求/响应结构不变。

## 3. 非目标

1. 不改变 BFE `cluster_table` 的格式与空条目语义。
2. 不改变 EPP 控制面（`epp_pool` 存储、指派、reconciler）。
3. 不新增 cluster 详情的实时实例字段（可选需求，后续独立评审）。
4. 不改变 controller 的 `EnsureK8sPool` 行为。
5. 不引入缓存（本期；导出器仅做请求级去重）。
6. 不把"provider 引用不存在的 pool"视为错误（仍按零实例处理）。

## 4. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api`（controller / web 仅回归，无代码改动） |
| 主要模块 | `model/iprovider`、`model/icluster_conf`、`model/ik8s_pool`、`model/ibatch`、`storage/rdb/{provider,k8s_pool}`、`stateful/container`、`endpoints/openapi_v1/provider` |
| 数据库 | P3 阶段从 DDL 删除 `providers.k8s_instance_pool` 列（P2 代码先不读不写，保留列便于回滚） |
| 接口契约 | provider 响应移除 `k8s_instance_pool`；字段已彻底删除（请求携带按未知字段静默忽略）；`/k8s_pools` PUT/DELETE 改为单表写 |
| 数据面 | `cluster_table` 导出格式不变；k8s cluster 实例改为实时解析 |

## 5. 关键决策

| 决策 | 结论 | 备选 |
|------|------|------|
| D1 k8s 模式 cluster 派生池 | 保留空占位行（schema 需要 subcluster → pool 引用） | 删除 pool 行（破坏引用） |
| D2 DELETE 被引用 pool | 保留行 + 清空实例（保持现有 404/200 契约与 `last_sync_time`） | 永远删行 |
| D3 导出读失败策略 | fail-close：整个导出报错，BFE 持最后成功版本 | fail-open 回退旧快照（掩盖故障） |
| D4 resolver 未注入 | 返回明确错误（不静默空池） | 回退快照（镜像已删除，无法回退） |
| D5 manual→k8s 时旧派生池 | 清空（维持占位不变量，防 `isDomainPool`/product_pool 视图被污染） | 不清空 |
| D6 k8s 实例应用到哪些 subcluster | 所有 subcluster（保持现有 fan-out 语义） | 仅 provider 派生 subcluster |
| D7 k8s 域名实例的 `isDomainPool` | 本期不支持，标注已知限制（发现组件只写 Pod IP） | AIConf 导出也接 resolver |
| D8 请求体 `k8s_instance_pool` | 已彻底删除：请求携带按未知字段静默忽略 | - |
| D9 发布方式 | P1 实时读 / P2 删写链路 / P3 DDL 收尾，三阶段分批发布 | 单 PR 一次完成 |
| 方案选型 | **方案 B：实时解析 + 删除镜像/fan-out**（单一事实源、无一致性窗口、手工来源零影响） | A 最小修复（保留镜像，同类 bug 面仍在）；C 周期重推（治标不治本） |

## 6. 影响与风险

| 项目 | 影响 |
|------|------|
| 向后兼容 | provider 响应移除 `k8s_instance_pool` 字段（破坏性契约变化，需同步前端回归）；`cluster_table` 格式不变 |
| 手工来源 | 所有读写路径与改造前完全一致（专项回归用例锁住） |
| 数据一致性 | 消除镜像与事实源的一致性窗口；"先 pool 后 provider"不再有空洞 |
| 导出可用性 | 导出新增 `k8s_pools` 读依赖，读取失败时导出 fail-close，BFE 保持最后成功版本 |
| 控制台视图 | k8s 模式下 product_pool / pool 视图派生池恒为空实例（占位），排障须看 `/k8s_pools` 与 provider |
| 风险 | 导出 DB 依赖（低，fail-close+告警）；手工来源误伤（中，显式分流+回归）；resolver 未注入（中，明确错误+装配测试） |

## 7. 关联文档

- 接口变更：本目录 `api-changes.md`
- 设计变更：本目录 `design-changes.md`
- OpenAPI provider 契约：`design-docs/api-define/OpenAPI接口定义/providers.md`（本文档已同步更新）
- InnerAPI k8s_pools 契约：`design-docs/api-define/InnerAPI接口定义/k8s-pools.md`（本文档已同步更新）
- OpenAPI k8s_pools 只读接口：`design-docs/api-define/OpenAPI接口定义/k8s-pools.md`（#290，无需变更）

## 8. 状态

- [x] 变更摘要、API 变更、设计变更文档
- [x] `api-define/OpenAPI接口定义/providers.md` 更新（删除镜像字段，有效池改为实时解析）
- [x] `api-define/InnerAPI接口定义/k8s-pools.md` 更新（PUT/DELETE 单表语义，删除 fan-out 描述）
- [x] 代码实现（P1 实时读路径）
- [x] 代码实现（P2 删除写链路）
- [x] DDL 收尾（P3：`k8s_instance_pool` 列已从 db_ddl/db_ddl_sqlite/升级脚本删除）
- [ ] 前端回归（ai-gateway-web，跨仓）
