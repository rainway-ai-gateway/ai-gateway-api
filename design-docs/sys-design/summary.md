# sys-design 文档索引

本目录存放 `ai-gateway-api` 系统级设计文档，覆盖总体架构、接口层、模型层、存储层、数据库设计以及关键模块的细节设计。

---

## 一、总体与分层设计

| 文档名称 | 相对路径 | 摘要说明 |
|---------|---------|---------|
| 总体设计文档 | [总体设计文档.md](./总体设计文档.md) | 描述 `ai-gateway-api` 在 AI 网关中的定位、功能范围、三层架构（接口层/模型层/存储层）、组件交互关系、核心数据流以及 OpenAPI 与 InnerAPI 的职责划分。 |
| 接口层设计文档 | [接口层设计文档.md](./接口层设计文档.md) | 描述管理面 OpenAPI（`/open-api/v1`）与数据面 InnerAPI（`/inner-api/v1`）的路由组织、统一的 `xreq.Endpoint` 抽象、全局与业务中间件链、子包划分以及典型接口实现模式。 |
| 模型层设计文档 | [模型层设计文档.md](./模型层设计文档.md) | 描述 `model/` 层的子包职责、Manager + Storager 接口的分层模式、Param/Filter 设计、事务管理、典型业务流程以及各业务模型（API-Key、Entity、Provider、Cluster、Quota、RateLimit、Route 等）的交互方式。 |
| 存储层设计文档 | [存储层设计文档.md](./存储层设计文档.md) | 描述 `storage/rdb` 层的 DAO + Storage 两层结构、通用 DAO 设计模式、事务与连接管理、42 张表的 DAO 映射关系以及 Storage 实现如何向上暴露接口供模型层调用。 |
| 数据库设计文档 | [数据库设计文档.md](./数据库设计文档.md) | 描述 `ai-gateway-api` 当前实现中全部 42 张持久化表（2026-10-07 新增 `batch_files`/`batch_tasks` 后；原 36/38 的口径不一致已一并修正）的字段、约束、索引、JSON 字段结构以及表间逻辑关系，覆盖基础配置、集群、路由、证书、API-Key、Entity、配额、限流、AI 缓存、AI 上下文压缩、AI 意图、模型定价、批量任务、操作日志、安全（密钥静态加密）等模块。 |
| 报表库设计文档 | [报表库设计文档.md](./报表库设计文档.md) | 描述独立报表库（`bfe_report`）的两张表设计：`bfe_ai_request_log` 明细表（101 列，含 2026-09-27 缓存/镜像/意图 10 列与 2026-10-30 缺价信号 `ai_price_miss`/`ai_price_warn` 2 列；幂等键/空值语义/按天 RANGE 分区/索引）与 `bfe_ai_metrics_1m` 分钟聚合表（40 维 + 24 指标，含 `ai_cache_status`/`mirror_hit`/`ai_intent_answer` 3 个维度列，与 Doris 对齐、不设唯一键的理由）、聚合与分区管理语义、账号权限矩阵、容量建议与列演进规则；DDL 归 `db_ddl_report_mysql.sql`。 |

---

## 二、细节设计（details/）

| 文档名称 | 相对路径 | 摘要说明 |
|---------|---------|---------|
| 操作日志模块 | [details/操作日志模块.md](./details/操作日志模块.md) | 描述 `model/ioperlog` 操作日志设计：覆盖 entity / api-key / provider / cluster 等域的变更记录、成功与失败双路径日志、异步批量写入、敏感字段脱敏与 `GET /operation-logs` 查询接口；含嵌套资源（quota_plan / rate_limit_policy）写/审分层审计与 `resource_parent_id` 归属设计（issue #161）；列表查询采用延迟关联分页消除深页随 offset 线性退化（issue #275）；过滤列组合索引经增量脚本落到存量库、`pagination.total` 按过滤条件做世代失效的短 TTL 缓存（issue #280）。 |
| 报表查询模块 | [details/报表查询模块.md](./details/报表查询模块.md) | 描述 `/report/*` 报表查询设计：`model/ireport` Manager + `ReportStorager` 四后端（MySQL/Doris/ClickHouse/StarRocks，后两者为 2026-10-02 新增的纯查询后端：CH 聚合由物化视图维护、保留期由 TTL 自管，SR 聚合由异步物化视图维护、保留期由动态分区 + partition_ttl 自管）方言实现、`Capabilities()` 后端能力门控（新维度 Doris 返回 422）、`Databases` 数据源复用与 `[Report]` 装配、分钟聚合 JOB（DELETE+INSERT 事务幂等、GET_LOCK 防重）与分区管理 JOB（RANGE 分区/DELETE 降级）、两张报表表 DDL 归属（`db_ddl_report_mysql.sql`）与三处经 LR03 集成测试验证的修正、FeatureReport 鉴权与未装配 404 行为、缓存/镜像/意图字段消费（overview/logs/timeseries cache_tokens/新维度，2026-09-27）。 |
| 认证授权机制 | [details/认证授权机制.md](./details/认证授权机制.md) | 描述 `model/iauth` 的认证授权设计，包括用户与 Token 共用 `users` 表、`Visitor` 统一抽象、Scope 作用域（System / SystemReadonly / Support）、Feature-Action 权限模型、四种认证方式、只读角色与密钥脱敏、内置账号保护以及中间件集成与数据库表设计。 |
| 密钥静态加密 | [details/密钥静态加密.md](./details/密钥静态加密.md) | 描述 DB 落盘加密机制（2026-10-05 落地）：`enc$v1$` 信封（keyID 自描述选钥、marker 明文兼容）、keyring 文件唯一注入与 `/reload/security` 热加载、启动 fail-fast（`CheckSecretAtRest`）、DAO 透明层、`api_key_hash` 常量 pepper 查询索引、sweep 收敛任务（DB 任务表+单行锁、多实例互斥与失联接管、reencrypt/decrypt 双模式）、轮换 runbook 与实现校准记录。 |
| 下发配置加密 | [details/下发配置加密.md](./details/下发配置加密.md) | 描述下发配置文件敏感字段加密机制（2026-10-05 落地）：两个密钥类导出 topic（mod-api-key Tokens 外层键、server_data_conf `AIConf.Keys[].Key`）的导出生成点字段级加密；`enc$v1$` 信封的跨仓校准（原始 keyring 密钥无派生、确定性 nonce 保 `data_sign` 稳定）、`[Security]` 导出钥三字段与 fail-fast、`/reload/security` 双 keyring 热加载、免重启轮换 runbook（含"确定性密文下需内容变更推进版本"要点）、回滚与版本错配硬约束。 |
| 管理面访问控制 | [details/管理面访问控制.md](./details/管理面访问控制.md) | 描述管理面 IP 白名单机制（2026-10-04 落地）：服务端 toml `[AccessControl]` 配置（不走 DB 的鸡生蛋规避）、`McIPProbe` 中间件在全局链的位置约束（Logger 后/CORS 前，覆盖 Dashboard 静态分支）、可信代理 XFF 右起解析、`atomic.Value` 原子热加载（`/reload/access_control`，与启动共用 `CompileAccessControl`）、fail-open 与 Audit 观察模式、403 错误类型，以及 validator `dive` 不下钻等实现踩坑记录。 |
| InnerAPI 配置导出与版本控制 | [details/InnerAPI配置导出与版本控制.md](./details/InnerAPI配置导出与版本控制.md) | 描述面向 BFE/Conf Agent 的 InnerAPI 配置导出机制，包括 `VersionControlManager` 的 MD5 签名比对、版本号生成与同 Topic 严格单调递增保证（`uk_name_version` 唯一约束 + 版本 +1s 抬升 + 冲突重试，issue #142）、`config_versions` 表持久化、11 类配置导出主题、增量同步流程，以及 `mod-api-key` 的批量预加载 + 内存回溯性能优化。 |
| API-Key 与 Entity 关联及模型继承 | [details/API-Key与Entity关联及模型继承.md](./details/API-Key与Entity关联及模型继承.md) | 描述 API-Key 与 Entity 的挂载关系、Entity 层级树约束、模型白名单交集与黑名单继承、配额计划层级合并、限流策略与路由规则的层级收集，以及导出到 BFE 时的最终生效规则。 |
| 限流策略与导出 | [details/限流策略与导出.md](./details/限流策略与导出.md) | 描述限流策略的数据模型（TPM/RPM/并发数/批量限流维度 `batch_limits`）、JSON 配置结构、CRUD 校验、API-Key/Entity 引用关系、按 Entity 层级向上合并导出到 BFE 的流程，以及 BFE 侧预期行为与边界情况。 |
| AI 缓存规则与导出 | [details/AI缓存规则与导出.md](./details/AI缓存规则与导出.md) | 描述 AI 缓存规则集合资源（`ai_cache_rules`）：集合级全量读写（first-match-wins、无 `enabled`、无 `/{id}`）、Open API 事务重建语义、导出到 BFE `mod_ai_cache` 的字段 tag 契约（一期五字段 + 二期 `enableSemanticCache` 与顶层 `Semantic` 全局块）、单例语义设置资源 `ai_cache_semantic_settings`（upsert、空表=默认、恒导出）、`iversion_control` 生成器合并流程（设置变更驱动版本流）、Redis/embedding/向量连接经 `CopyFiles` 不下发，以及与 rate_limit_policy 导出的差异。 |
| 意图配置与导出 | [details/意图配置与导出.md](./details/意图配置与导出.md) | 描述 AI 意图配置单例资源（`intent_config`）：单行覆盖式全量读写（无 `enabled`、无历史版本）、空 `questions` 软开关停用意图分类、`intent_questions.data` 导出（Data = 文件内容原样，Version 内嵌文件，非 ai-cache 的 Config 包装形态）、路由 cond 直传消费意图条件（fail-safe 引用语义）、静态 `mod_ai_intent.conf` 经 `CopyFiles` 不下发，以及与 ai_cache_rules 导出的差异。 |
| 流量镜像规则与导出 | [details/流量镜像规则与导出.md](./details/流量镜像规则与导出.md) | 描述流量镜像规则集合资源（`traffic_mirror_rules`）：集合级全量读写（first-match-wins、无 `enabled`、无 `/{id}`、`cond` 必填且全匹配显式 `default_t()`）、`mirror_cluster` 存在性校验、`remove_headers` 缺省填默认黑名单的两层默认语义、Open API 事务重建语义、导出到 BFE `mod_traffic_mirror` 的字段 tag 契约（规则全字段恒输出）、`iversion_control` 生成器流程、模块调参经 `CopyFiles` 不下发，以及与 ai_cache_rules 导出的差异。 |
| 路由规则管理 | [details/路由规则管理.md](./details/路由规则管理.md) | 区分产品级 BFE 路由规则与 AI 路由规则，描述 `route_rules` 表的三级（Global/Entity/API-Key）管理、校验规则、与 API-Key/Entity 生命周期的一致性、导出到 BFE 的绑定顺序与文件格式。 |
| 配额余额同步机制 | [details/配额余额同步机制.md](./details/配额余额同步机制.md) | 描述 Redis 作为余额唯一真实来源的架构，包括 OpenAPI 查询余额直接读 Redis、`QuotaCache` 批量读取接口（`BatchGetRemaining` / `GetInt64Batch`）、`QuotaResetScheduler` 调度器（含 Redis 分布式锁 + 看门狗续期）、`BalanceSyncManager` 的过期重置（先认领——`last_reset_at` 条件更新且显式覆盖 NULL，后原子 `SET` Redis）、`QuotaPlanManager.ApplyQuotaPlanChange` 对 quota_plan 变更的差异化处理、自然周/月重置逻辑、Redis Key 生命周期以及 `last_reset_at` 上移至 `quota_plans` 表的设计；包含 RMB 配额定点精度设计与 `Clock` 时钟注入。 |
| Provider 与 Cluster 概念分离 | [details/provider与cluster概念分离.md](./details/provider与cluster概念分离.md) | 描述 Provider 与 Cluster 解耦后的数据模型、接口变化、BFE 配置生成转换、引用关系、迁移策略与风险。 |
| Claude 协议转发支持 | [details/Claude协议转发支持.md](./details/Claude协议转发支持.md) | 描述 ai-gateway-api 控制面为支持 BFE 转发 Claude Messages API 所需配合的修改，包括 `AIConf.ModelProtocols` 透传、模型发现解析器下沉与废弃配置清理。 |
| Redis Key 清理机制 | [details/Redis Key 清理机制.md](./details/Redis%20Key%20清理机制.md) | 描述 API-Key / Entity 删除及 rate-limit 规则变更时的 Redis Key 清理逻辑，包括 Quota Key 与 Rate-Limit Key 格式、触发场景、对 BFE 的影响以及控制面立即清理实现方案。 |
| 模型定价管理 | [api-define/OpenAPI接口定义/model-prices.md](../api-define/OpenAPI接口定义/model-prices.md) | 描述 `/model-prices` 管理接口、`model-list.yaml` 导入格式、`ModelPrice` 数据模型与校验规则，是 InnerAPI `AIConf.ModelTable` 的数据源。 |
| RMB 配额分时段定价 | [details/RMB配额分时段定价.md](./details/RMB配额分时段定价.md) | 描述 RMB 配额按 provider 时段模板与 model-prices 分时段价格进行计费的设计，包括 `time_zone` / `tiers` / `tier_prices` 数据模型、控制面导出逻辑、BFE 数据面时段匹配与成本计算。 |
| EPP 调度对接 | [details/EPP调度对接.md](./details/EPP调度对接.md) | 描述 EPP（llm-d 调度器）接入后控制面的四块能力：cluster `balance_mode`/`epp_config`（简化用户形态 + 确定性编译）、`/epp-pool` 静态实例池、分配器（贪心确定性算法、悬空修复、周期对账）、双向下发（server_data_conf 的 `BalanceMode`/`EPPAddr` 含降级语义、epp_data 单 topic 两段合并）。 |
| 部分更新语义与 DAO 的 nil-skip 约定 | [details/部分更新语义与DAO-nil-skip约定.md](./details/部分更新语义与DAO-nil-skip约定.md) | 描述 PATCH"仅传需修改字段"契约的实现机制（DAO `struct2map` nil-skip）与 storager 层约束（不得把省略抹平为默认值）；各资源省略字段行为对照表（API-Key `models`/`subnet`、Entity `allow_models`/`block_models`、Provider 五字段）；Create/PATCH 默认值语义差异与"显式空数组不区分"的已知限制（issue #151、#147）。 |
| 规则文件顶层全局块的合并导出模式 | [details/规则文件顶层全局块的合并导出模式.md](./details/规则文件顶层全局块的合并导出模式.md) | 沉淀 ai-cache `Semantic` 块与 ai-context `Defaults` 块两个落地实例，固化"单例设置表 + 生成器合并顶层键恒导出"模式：否决 imods 独立 topic 的三条理由、空表=默认（不预置默认行）、设置变更驱动 MD5 版本流、连接配置经 CopyFiles 不下发、二期字段前向兼容前提（数据面未知字段容忍），以及新模块复用清单与不适用边界。 |
| 批量任务与对账 | [details/批量任务与对账.md](./details/批量任务与对账.md) | 描述批量与异步任务支持（一期）的控制面机制：`batch_tasks` / `batch_files` 实体与 OpenAI 状态机、Redis `BATCH_*` 五类键结构与 TTL、预留-结算-释放三态金额语义（1e-8 定点 RMB、允许透支 + `over_reserved`）、对账 job 职责（Redis SCAN 同步、状态推进、终态产物补漏、兜底结算、对账巡检与日志补登、TTL 清理）、cancel 五步流程与两把幂等锁竞态收敛、多实例协调（named lock / 条件更新 / Redis Lua）、与报表后端解耦的理由（`[Report].Backend` 可插拔）及不出网部署的降级路径。 |
| 条件原语校验与 BFE 依赖版本对齐 | [details/条件原语校验与BFE依赖版本对齐.md](./details/条件原语校验与BFE依赖版本对齐.md) | 描述控制面条件表达式校验的"无白名单"机制（编译交由 BFE `bfe_basic/condition`，避免两处原语清单漂移）：3 类编译入口清单（6 处调用点）、`replace` 指向 fork 提交的版本对齐口径（伪版本计算、新提交的 sumdb 404 与 `GOPRIVATE` 绕过、go.sum 写入后即稳定）、一次 bump 拖入接口漂移的实例（`redis_client.Client.HGetAll` 与 ibatch mock 的配套）、上线顺序与时区类原语的 UTC 缺省提醒、新增条件原语的 checklist。 |

---

## 三、阅读建议

1. **快速建立全局认知**：先阅读《总体设计文档.md》。
2. **理解代码分层**：依次阅读《接口层设计文档.md》《模型层设计文档.md》《存储层设计文档.md》。
3. **查看具体表结构**：参考《数据库设计文档.md》。
4. **深入关键模块**：根据关注领域选择 `details/` 下的细节文档。
