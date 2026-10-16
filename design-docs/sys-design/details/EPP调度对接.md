# EPP 调度对接

## 1. 概述

EPP（Endpoint Picker，基于 llm-d 的 LLM 推理调度器）接入后，ai-gateway-api 需要承担其**控制面**职责：集群级调度配置管理、EPP 实例池管理、cluster→EPP 实例组分配，以及向 BFE 与 EPP 两侧的配置下发。

本组件覆盖四块能力：

- **cluster 均衡模式与调度配置**（`/clusters` 新增 `balance_mode` + `epp_config`）；
- **EPP 实例池管理**（`/epp-pool`，单例池 + 全量替换模式：`GET` 详情 + `PATCH` 全量替换）；
- **cluster→实例组分配**（分配器自动生成 + `/epp-assignments` 查询视图与手工覆写）；
- **双向下发**：server_data_conf 向 BFE 导出 `BalanceMode`/`EPPAddr`；epp_data（新 topic）向 EPP 实例统一下发编译后的调度配置 + assignment 全量视图。

> 完整变更说明见 `design-docs/modifications/2026-09-08-epp-scheduling-integration/`（change-summary / api-changes / design-changes）；接口契约见 `design-docs/api-define/` 下 `OpenAPI接口定义/epp-pool.md`、`epp-assignments.md`、`clusters.md` 与 `InnerAPI接口定义/epp-data.md`、`server-data-conf.md`。自动分配各场景的详细分析见《EPP实例自动分配-场景分析与方案》（document-ai-gateway 仓库）。

---

## 2. 数据模型

### 2.1 `clusters` 表新增列

| 列 | 类型 | 说明 |
|----|------|------|
| `balance_mode` | varchar，默认 `'WRR'` | 集群均衡模式：`WRR`（BFE 本地加权轮询，默认）/ `EPP`（EPP 调度器接管后端选择）。**唯一判定来源**（`getBalanceMode()` 直读该字段；pool `Role` 派生逻辑已删除） |
| `epp_config` | JSON text，可空 | EPP 调度配置，**简化用户形态**（见 §3）；仅 `balance_mode=EPP` 时生效，WRR 时保留不生效（休眠语义，见 §5） |

两列均可空/带默认值，存量数据无需迁移。`pools.epp_server` 列保留但已无消费方，后续版本清理。

> **EPP 模式 cluster 的实例池语义**：创建/更新时池**照常同步 provider `instance_pool` 实例**（`Role=EPP` 仅作类型标记）。这些实例服务于：① cluster_table 导出——EPP 的 `cluster-table-discovery` 经它发现推理后端（"EPP 从中获取 model server 实例列表，本方案不变更"）；② EPP 全挂时 BFE 回退本地 WRR 的兜底后端。`BalanceMode=EPP` 下 BFE 不做本地 WRR，实例不参与正常均衡。

### 2.2 `epp_instances` 表（新增）

EPP 实例池存储，一行一个实例：

```
id, host, port, group_name, 创建/更新时间
```

- `id`：实例 id，池内全局唯一；EPP 进程以 `-instance-id` 启动参数对应此值（StatefulSet 部署时缺省为 Pod hostname，同组副本共享相同启动参数）。
- `(host, port)`：`UNIQUE(host, port)` 池内地址唯一；host 为 Hostname 或 IP（IPv6 字面量不带括号），导出拼接 `host:port` 时经 `net.JoinHostPort` 自动加括号。
- **无 `status`/`last_heartbeat`**：存活感知在 BFE 侧（EPPAddr 连接滞回），api 侧不做存活标记。

### 2.3 `epp_assignments` 表（新增）

cluster→实例组分配，**只存主**：

```
cluster（唯一键）, group_name, primary_instance_id
```

- standby 不存储：读时展开为同组除 primary 外的实例，避免双写不一致。
- 分配由**分配器自动生成**（§4），`PUT /api/v1/epp-assignments/{cluster}` 手工覆写作为运维兜底。
- `EPP → WRR` 后分配记录**休眠保留**（不进导出），切回 EPP 继续生效。

---

## 3. `epp_config`：简化用户形态与确定性编译

`epp_config` 不直接暴露 llm-d `EndpointPickerConfig` 的插件声明细节，用户以"调度档位 + 少量一等公民调优参数"表达意图：

| 字段 | 默认 | 说明 |
|------|------|------|
| `scheduling_profile` | `balanced` | 调度档位：`latency-first` / `balanced` / `throughput-first`，映射 (kv, queue) scorer 权重 |
| `cache_affinity` | 缺省（跟随档位） | 显式 `low`/`medium`/`high` 时覆盖档位权重；存储保留原始 JSON 以区分"缺省"与"显式 medium" |
| `prefix_cache_affinity` | `true` | 前缀缓存亲和 scorer 开关（软亲和，权重固定 1.0；本地内存 LRU，无外部存储） |
| `session_affinity_enabled` / `session_affinity_header` | `false` / - | 会话亲和开关 + session id 来源请求头（enabled=true 时 header 必填；false 时 header 可休眠保留） |
| `kv_cache_utilization_max` | `0.9` | utilization-filter 阈值 `(0,1]` |
| `flow_control` | - | `max_requests`（缺省/-1 不限）、`queue_ttl`/`no_endpoint_queue_ttl`（int 秒，≥0，0=禁用驱逐；缺省由 EPP 默认 60s）、`enable_eviction` |

api 导出时把简化配置**确定性编译**为完整 `EndpointPickerConfig`（档位权重映射、亲和 scorer 条件注入、flowControl 段展开、featureGates 追加、固定插件注入），规则固化并单测覆盖；本期不提供原样透传的高级模式。秒数在编译时转为 Go duration 下发（`30` → `30s`）。

**休眠语义**：`epp_config` 非空即须通过字段校验（**与 `balance_mode` 无关**）；仅 `balance_mode=EPP` 时编译下发，WRR 时保留不导出；停用 EPP = 改回 WRR，无需清空。

---

## 4. EPP 实例池（`/epp-pool`）与分配器

### 4.1 实例池：静态配置替代自注册

- **单例池**，池名由配置项 `RunTime.DefaultEPPInstancePoolName` 提供（默认 `EPP.pool`）；`GET` 详情 + `PATCH` 全量替换。
- 实例列表是**部署事实**：部署流程在扩缩容/换机后 reconcile PATCH，天然幂等；api 侧无心跳续约、失联阈值等存活管理负担。
- 组规模校验：每组恰好 1 实例，拒绝空组与 2 个及以上实例的组（API 写入面不再接受主备双实例组）。

### 4.2 分配器：贪心 + 确定性 tie-break

```
输入：实例池（组→实例列表）、现有分配（epp_assignments）、目标 cluster
1. 候选组：池中全部组（PATCH 校验保证每组恰 1 实例，无空组），按组名排序
2. 选组：组负载 = 组内各实例"作为 primary 承担的 cluster 数"之和
        → 最小者优先，并列取组名字典序最小
3. 选主：组内"作为 primary 承担的 cluster 数最少"的实例
        → 并列取实例 id 字典序最小
4. upsert epp_assignments
```

全程确定性、可重放、可单测。互为主备是合法输出（同组两实例在不同 cluster 间互为主备）；不做组↔cluster 一对一约束。

### 4.3 触发时机

| 触发点 | 行为 |
|--------|------|
| cluster 创建为 EPP / `WRR → EPP` 变更 | 无分配记录 → 同步自动分配（创建请求内完成） |
| `/epp-pool` PATCH 后悬空修复 | ① primary 被移出、组仍在 → 同组剩余实例重选 primary（不换组）；② 组已不存在（或规模不满足要求）→ **跨组重分配**（对整个池重跑算法）；③ 池无可分配候选组 → 清除分配，进入未分配态 |
| 手工覆写指向的组被移除 | 同悬空修复② |
| 周期对账 reconciler（30s，可配） | 扫描全部 EPP cluster，无有效分配即自动补分配（漂移兜底；幂等，大部分轮次零写入） |

### 4.4 查询与手工覆写

- `GET /api/v1/epp-assignments`：分配全量视图——`epp_assignments` 与 `epp_instances` 的**读时 join**（不引入冗余存储），per-cluster 展开 `{group, primary}`（OpenAPI 视图不暴露 standby，内部读时展开保留）；不一致标记 `degraded`；汇总 `unassigned_clusters` 与 `idle_groups` 便于巡检。
- `PUT /api/v1/epp-assignments/{cluster}`：手工覆写 `{group_name, primary_instance_id}`（运维干预入口），触发 server_data_conf version bump。

---

## 5. 双向下发

### 5.1 server_data_conf → BFE（`BalanceMode` / `EPPAddr`）

- `balance_mode=EPP` 且有有效分配：`GslbBasic.BalanceMode=EPP`，`EPPAddr` 为**有序列表 `[主, 备]`**（主 `[0]`、备 `[1]`，单实例组仅主；host/port 经 `net.JoinHostPort` 拼接）。
- **无有效分配时降级导出**：该 cluster `BalanceMode` 置为 `WRR`、不生成 `EPPAddr`，同时输出 **error 级日志**（含 cluster 名与原因）。单 cluster 降级不阻塞整份下发；分配恢复后下轮导出自动回到 `EPP`。降级期间 BFE 以本地 WRR 调度该 cluster 池中的 provider 实例（请求仍可服务，仅失去 EPP 智能调度），靠 error 日志 + `unassigned_clusters` 告警发现。

### 5.2 epp_data → EPP（新 topic，两段合并单端点）

- **端点**：`GET /inner-api/v1/configs/epp_data/config?version=...`；topic `ConfigTopicEppData`，复用 `ExportConfig` 框架（MD5 签名、version 单调递增、未变化返回 `Data: null`）。
- **形状**：`{ "epp_config": { cluster → 编译后 EndpointPickerConfig }, "assignment": { cluster → {primary, standby} } }`。
- assignment 为**全量视图**：所有 EPP 实例返回完全相同内容，EPP 以自身 `-instance-id` 逐 cluster 匹配得出角色；未命中跳过。单 topic 保证两段配置同一 version 原子快照，消除角色/配置错配窗口。
- 实例池变更不直接 bump 该 topic（cluster→role 映射未变），仅分配变化时经 server_data_conf 导出间接体现。

---

## 6. 关键设计取舍

| 取舍 | 决策 | 理由 |
|------|------|------|
| 兼容性 | **不考虑兼容路径**：旧 EPPServer 派生逻辑直接删除 | 同版本发布，控制面与数据面一起升级 |
| 实例存活管理 | api 侧无心跳/存活标记 | 存活与 failover 由 BFE 侧 EPPAddr 连接滞回驱动；实例列表权威来源是部署流程 |
| 高级逃生门 | 本期不提供（epp_config 只暴露简化形态） | 自定义插件链需求出现时再开放 |
| 导出失败策略 | 无分配 → 单 cluster 降级 WRR + error 日志，而非整份拒绝 | 避免单 cluster 配置阻塞全局下发；显式降级可监控 |
| 均衡算法 | 贪心 + 字典序 tie-break，无随机 | 确定性可重放、可单测；组间再平衡/反亲和/失效翻转属 P2 |
| 亲和实现 | prefix/session 亲和均为 EPP 本地内存（LRU/binding），指标为周期抓取后端 `/metrics` | EPP 全链路对外依赖仅两类无状态拉取：控制面配置 + 数据面指标 |

---

## 7. 涉及代码位置

| 位置 | 职责 |
|------|------|
| `endpoints/openapi_v1/epp_pool/`、`endpoints/openapi_v1/epp_assignments/`（新增） | `/epp-pool`、`/epp-assignments` OpenAPI |
| `endpoints/innerapi_v1/epp_data/`（新增） | epp_data 导出 handler |
| `model/epp_pool/` + `storage/rdb/epp_pool/`（新增） | 实例池读写、分配器（含 reconciler）、epp_data generator、epp_config 编译模板 |
| `model/icluster_conf/cluster.go` | `balance_mode`/`epp_config` 字段、创建流程、EPPAddr 分配驱动生成 |
| `model/iversion_control/version_control.go` | 复用，新增 `ConfigTopicEppData` topic |
