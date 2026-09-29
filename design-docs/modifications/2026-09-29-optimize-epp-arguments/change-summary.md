# EPP 暴露参数优化（ai-gateway-api 控制面改造）变更摘要

## 1. 背景

ai-gateway-api 已在 EPP 调度对接（`../2026-09-08-epp-scheduling-integration/`）中落地 cluster 级 `balance_mode` 与**简化用户形态** `epp_config`：用户以"调度档位 + 少量调优参数"表达意图，api 在 `epp_data` 导出时**确定性编译**为完整 llm-d `EndpointPickerConfig`（见该目录 `api-changes.md` §3.2.1）。

上线复盘发现简化参数存在三方面问题，导致用户难以理解与调优：

1. **档位命名与 EPP 实际调度维度错位**：`scheduling_profile` 的 `latency-first` / `throughput-first` 未点明"偏向**排队短** / 偏向 **KV 空闲**"这一真实含义——用户看到 `latency-first` 会以为"总选最快的后端"，实际只是提高 queue-scorer 权重，语义与预期不符。
2. **亲和强度不可调、无法关闭**：`cache_affinity` 名为"KV cache 亲和"，实为 (kv, queue) scorer 权重覆盖项；而真正的亲和 scorer（`prefix-cache-scorer` / `session-affinity-scorer`）权重被硬编码为 `1.0`，既无法按需调节亲和力度，也没有"关闭亲和"的入口。
3. **准入与剔除能力缺口**：仅暴露 `kv_cache_utilization_max`（KV 水位）一个准入旋钮，缺少**排队水位**、**并发水位**、**过滤回退**（`fallbackOnEmpty`）以及**坏后端剔除时长**（指标过期阈值）的暴露，用户无法表达"防排队过长""防并发过高""极端负载下宁可派给超载后端也不拒绝""尽快摘除失联后端"等诉求。

本方案重新梳理 `epp_config` 暴露面，收敛为用户只需理解的 **3 组概念**——"**往哪调**（调度倾向）、**谁能接**（准入水位 + 坏后端剔除）、**最多等多久**（容量与排队）"，并补齐上述缺口。参数名与取值更贴合 EPP 实际调度维度（排队 / KV），同时保持旧字段兼容。

## 2. 目标

- 将调度档位 `scheduling_profile` 重命名为 **`load_profile`**（实例负载画像），取值 `latency-first` → **`queue-first`**（偏向排队短，通常更低延迟）、`throughput-first` → **`kv-first`**（偏向 KV 空闲，通常更高吞吐），`balanced` 保持不变。
- 将 `cache_affinity` 重命名为 **`affinity`**，语义由"scorer 权重覆盖"改为**亲和强度**，并新增 `off` 档（不注入亲和 scorer）；取值为 `off` / `low` / `medium` / `high`。
- 新增准入水位参数 **`waiting_queue_max`**、**`running_requests_max`**（`0` = 不启用该维度过滤）。
- 新增过滤回退开关 **`fallback_on_empty`**（所有后端被过滤时的行为）。
- 新增坏后端剔除参数 **`metrics_staleness_threshold_ms`**（后端指标超过该时长未更新即从候选剔除）。
- 编译产物新增 **`saturation-detector`**（type `utilization-detector`，**始终下发**，不入 profile）与 **`flowControl.saturationDetector`**；`kv_cache_utilization_max` 作为"过滤 + 背压"的**单一阈值来源**，同时驱动 `utilization-filter` 条件与 detector 的 `kvCacheUtilThreshold`。
- `flow_control.enable_eviction` 停止下发（当前 EPP 未接线）；`flow_control.no_endpoint_queue_ttl` 缺省时**显式展开为 `queue_ttl`**。
- 旧字段 `scheduling_profile` / `cache_affinity` 保留为 deprecated 别名，兼容期内可平迁、可回读。
- 增强的一致性保证：**detector 始终下发**（即使不配 `flow_control`），避免"没配却在用默认值过滤"。

## 3. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api`（**仅参数与编译模板**，无新增端点、无新增表） |
| 主要模块 | `model/epp_pool/epp_config.go`（字段/校验/有效值）、`model/epp_pool/compiler.go`（编译模板）、`model/epp_pool/epp_data.go`（导出，逻辑不变）、`endpoints/openapi_v1/product_cluster/`（透传，逻辑不变） |
| 接口契约 | OpenAPI `/clusters` 的 `epp_config` 做**字段级变更**（重命名 + 新增，无新端点）；InnerAPI `/configs/epp_data/config` 结构**不变**、`epp_config` 段**编译产物内容**变更 |
| 数据库 | 复用 `clusters.epp_config` JSON 列，**无 DDL 变更**；列内容形态变化（旧 JSON 兼容读取） |
| 编译产物 | 新增 `saturation-detector` 插件、`flowControl.saturationDetector`、`util-filter.fallbackOnEmpty` / `waiting-queue` / `running-requests` 条件；scorer 权重随 `load_profile` / `affinity` 变化 |
| BFE 影响 | 无（不涉及 server_data_conf / EPPAddr） |
| EPP 影响 | 需要 EPP 侧支持 `saturation-detector`（`utilization-detector`）与 `flowControl.saturationDetector`；亲和 scorer 权重由固定 1.0 变为随 `affinity` 配置 |

## 4. 关键决策

| 决策 | 说明 |
|------|------|
| 档位改名为 `load_profile`（实例负载画像） | `scheduling_profile` 的命名容易被理解为"整体调度策略"，而其实际作用只是调节 (kv, queue) 两个基础 scorer 的权重。改名为 `load_profile` 并明确取值 `queue-first` / `kv-first`，直接表达"该实例画像偏向排队短还是 KV 空闲"，与 EPP 的调度维度（排队长度、KV 利用率）对齐。 |
| `latency-first` → `queue-first`、`throughput-first` → `kv-first` | 取值为"偏向维度"而非"结果"：`queue-first` 优先排队短（**通常**带来更低延迟），`kv-first` 优先 KV 空闲（**通常**带来更高吞吐）。避免用结果承诺误导；`balanced` 语义中立保持不变。 |
| `cache_affinity` → `affinity`，语义改为"亲和强度" | 旧 `cache_affinity` 实际是 **scorer 权重覆盖**（`low`/`medium`/`high` → (kv,queue) 权重），与"亲和"无关；真正的亲和 scorer 权重固定 1.0。新 `affinity` 直接调节 **`prefix-cache-scorer` / `session-affinity-scorer` 的权重**（`off`/`low`/`medium`/`high` = 0 / 0.3 / 0.6 / 1.0），`off` 时不注入亲和 scorer，语义名实一致。 |
| 新增 `waiting_queue_max` / `running_requests_max` | 补齐准入水位维度：与 `kv_cache_utilization_max` 一起构成"谁能接"的多维过滤（KV / 排队 / 并发）。`0` = 不启用（而非"上限为 0"），保持与"未设置走默认"一致的直觉。 |
| 新增 `fallback_on_empty` | 水位过滤是"尽力而为"，当所有后端都被过滤时需明确行为：`false`（默认）= 无后端可选、不派发；`true` = 回退派给"最不差"的后端。把该权衡交给用户显式决定。 |
| 新增 `metrics_staleness_threshold_ms` | 坏后端剔除能力：EPP 周期抓取各后端 `/metrics`，某后端"最后一次成功更新"超过该阈值即视为失联并剔除。默认 `200ms`（抓取周期约 50ms ≈ 连续 4 次失败），抗抖动与摘除速度兼顾。 |
| **detector 始终下发** | `saturation-detector` 与 `flowControl.saturationDetector` 在每次编译产物中都显式声明（即使不配 `flow_control`）。原因：① 过滤与背压共用同一 detector，缺省声明可避免"用户在过滤、却在用隐藏默认值背压"；② 坏后端剔除依赖 detector，必须存在。 |
| `kv_cache_utilization_max` 单一阈值来源 | 同一用户参数同时驱动 `utilization-filter` 的 `kv-cache-utilization` 条件与 detector 的 `kvCacheUtilThreshold`，杜绝"设了 0.9 却按 0.8 生效"的双阈值漂移。 |
| `enable_eviction` 停止下发 | 当前 EPP 未接线，保留字段仅为兼容旧配置；本期编译恒不下发，并建议校验拒绝 `true`（避免用户误以为生效）。 |
| 旧字段保留 deprecated 别名 + 旧值映射 | 兼容存量配置与自动化脚本：`scheduling_profile` / `cache_affinity` 仍可写入并回读；旧枚举值 `latency-first` / `throughput-first` **必须映射**为 `queue-first` / `kv-first`（否则查表失败会静默回退 `balanced`）。 |
| 不改变接口结构 | 本次为**字段级**变更：`/clusters` 无新端点、`epp_data` 无结构变更、数据库无 DDL。降低对接与回归成本。 |

## 5. 分期落地

| 期 | 内容 |
|----|------|
| P0 | `epp_config` 字段重命名 + 新增 4 个参数；`load_profile` / `affinity` 权重表；编译模板新增 `saturation-detector` 与 `flowControl.saturationDetector`、`util-filter` 新条件与 `fallbackOnEmpty`、`no_endpoint_queue_ttl` 展开；校验与有效值映射；OpenAPI / InnerAPI 文档同步（约 3 天） |
| P1 | （预留；视 P0 运行情况定，如亲和 scorer 细粒度参数、`session` 绑定 TTL 等） |
| P2 | （预留；如"per-band 容量""prefill/decode 分离画像"等更细粒度暴露） |

## 6. 关联文档

- 源方案：《Epp暴露参数优化》（`yxy-note/explorations/20260928-aigateway-epp-conf/20260928Epp暴露参数优化.md`）
- 接口变更：本目录 `api-changes.md`
- 设计变更：本目录 `design-changes.md`
- 既有 EPP 对接变更：`../2026-09-08-epp-scheduling-integration/`（`change-summary.md` / `api-changes.md` / `design-changes.md`）
- 接口定义：`design-docs/api-define/OpenAPI接口定义/clusters.md`、`design-docs/api-define/InnerAPI接口定义/epp-data.md`
- EPP 侧配置定义：`ai-gateway-epp/docs/zh_cn/configuration/EPP配置定义说明-epp_config.md`
- 代码位置：`model/epp_pool/epp_config.go`（简化配置与校验）、`model/epp_pool/compiler.go`（简化配置 → `EndpointPickerConfig` 编译模板）