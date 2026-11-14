# InnerAPI 测试用例设计文档

## 1. 模块概述

InnerAPI 模块为 BFE/Conf Agent 提供只读配置导出接口，支持基于 `version` 的增量同步。导出类接口均为 GET 请求，返回值包含 `WorkMode` 字段；`/k8s_pools` 域（IN-11）为例外的资源读写接口（PUT/GET/DELETE），供 K8s 发现组件维护实例池，不支持 `version` 增量同步。v0.3.0 对应 OpenAPI 的模块精简不影响 InnerAPI 导出结构（底层表结构未变）。v0.3.0 起，路由规则导出（`/configs/tls_conf/server_data_conf`）中的 `ClusterConf` 会包含模型定价表（`AIConf.ModelTable`），用于 BFE 按 provider 匹配可用模型。

v0.5.0 起，模型定价表支持分时段定价：OpenAPI 在 `/providers` 上维护 `time_zone`/`tiers` 时段模板，在 `/model-prices` 上维护 `tier_prices`；InnerAPI 导出时把 provider 的时段模板与 model-prices 的分时价格拼接到 `AIConf.ModelTable` 中，BFE 按请求时刻匹配 tier 并取对应价格。

v0.0.7 起，Cluster 表导出（`/configs/gslb_data/cluster_table`）中的 `AIConf` 字段不再使用单 `Key`，而是输出 `Keys`（`[]{Name, Key, Weight}`）和 `KeyPolicy`（`{Strategy, MaxRetries, RetryBackoffInitial, RetryBackoffMax}`），以支持多 API-Key 路由。InnerAPI 应验证该导出结构与 OpenAPI 写入的多 Key 配置一致。

v0.6 起，`AIConf.KeyPolicy` 新增 `SessionAffinity`、`SessionAffinityTTL`、`SessionAffinityRedisPrefix`、`SessionAffinityPenaltyEnable` 字段，对应 OpenAPI `llm_config.key_affinity`，用于会话级 Key 亲和性。InnerAPI 应验证这些字段与 OpenAPI 写入的 `key_affinity` 配置一致（含默认值）。

v0.10 起，`[Security].EncryptExports=true` 时，本清单的 IN-1（`AIConf.Keys[].Key`）与 IN-6（`tokens` 外层键、内层 `key` 置空）以 `enc$v1$` 字段级密文导出（信封/marker 直通/确定性密文等契约见 `api-define/InnerAPI接口定义/mod-api-key.md` §3.5 与 `modifications/2026-10-05-export-config-field-encryption/api-changes.md`）；其余 topic 不受影响。加密导出专项用例见独立模块 `tests/export_encryption/`（前缀 EFE），本目录各 submodule 用例保持明文形态（默认开关关闭）。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| IN-1 | 导出 TLS/Server 配置 | GET | `/inner-api/v1/configs/tls_conf/server_data_conf` | version 可选 |
| IN-2 | 导出 GSLB 配置 | GET | `/inner-api/v1/configs/gslb_data/gslb` | 必填 bfe_cluster |
| IN-3 | 导出集群表配置 | GET | `/inner-api/v1/configs/gslb_data/cluster_table` | version 可选 |
| IN-4 | 导出证书配置 | GET | `/inner-api/v1/configs/protocol/server_cert_conf` | version 可选 |
| IN-5 | 导出额外文件 | GET | `/inner-api/v1/configs/extra_files/{filename}` | - |
| IN-6 | 导出 API-Key 配置 | GET | `/inner-api/v1/configs/mod-api-key` | version 可选 |
| IN-7 | 导出请求体处理配置 | GET | `/inner-api/v1/configs/mod-body-process` | version 可选 |
| IN-8 | 导出限流策略配置 | GET | `/inner-api/v1/configs/rate-limit-policy` | version 可选 |
| IN-9 | 导出 AI 路由配置 | GET | `/inner-api/v1/configs/ai-route` | version 可选 |
| IN-10 | 导出 EPP 配置 | GET | `/inner-api/v1/configs/epp_data/config` | version 可选 |
| IN-11 | K8s 实例池维护 | PUT/GET/DELETE | `/inner-api/v1/k8s_pools/{name}/instances`、`/inner-api/v1/k8s_pools/{name}`、`/inner-api/v1/k8s_pools` | 资源读写，无 version；发现组件写入通道 |
| IN-12 | 导出 AI 缓存规则配置 | GET | `/inner-api/v1/configs/ai-cache-rule` | version 可选；Data 形状 `{"Config": {"AI_product": [...]}, "Version": "..."}`；一期每条规则仅导出 cond/cacheKeyStrategy/cacheTTL/maxBodyBytes/maxValueBytes 5 个 tag |

## 3. 测试用例统计

| 接口 | 测试用例数 |
|------|-----------|
| 导出 TLS/Server 配置 | 6 |
| 导出 GSLB 配置 | 2 |
| 导出集群表配置 | 2 |
| 导出证书配置 | 1 |
| 导出额外文件 | 1 |
| 导出 API-Key 配置 | 3 |
| 导出请求体处理配置 | 1 |
| 导出限流策略配置 | 1 |
| 导出 AI 路由配置 | 2 |
| 导出 EPP 配置 | 5 |
| 导出 AI 缓存规则配置 | 6 |
| K8s 实例池维护 | 4 |
| **合计** | **33** |

## 4. 认证方式

InnerAPI 鉴权为 `McUserProbe`，测试环境需配置为可跳过或使用 Support Token。

## 5. 目录结构

```
innerapi/
├── design.md
├── tls_conf/
│   ├── tls_conf_test.go
│   └── tiered_pricing_test.go
├── gslb/
│   └── gslb_test.go
├── cluster_table/
│   └── cluster_table_test.go
├── server_cert_conf/
│   └── server_cert_conf_test.go
├── extra_files/
│   └── extra_files_test.go
├── mod_api_key/
│   └── mod_api_key_test.go
├── mod_body_process/
│   └── mod_body_process_test.go
├── rate_limit_policy/
│   └── rate_limit_policy_test.go
├── ai_route/
│   └── ai_route_test.go
├── ai_cache/
│   └── ai_cache_test.go
├── traffic_mirror/
│   └── traffic_mirror_test.go
└── epp_data/
│   └── epp_data_test.go
└── k8s_pools/
    ├── k8s_pools_test.go
    └── concurrency_mysql_test.go  # //go:build mysql，需 AIAPI_MYSQL_DSN
```

## 6. 导出 TLS/Server 配置

### 6.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 TLS/Server 配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/tls_conf/server_data_conf` |
| 说明 | 导出 TLS/Server 配置，支持 version 增量同步 |

### 6.2 接口参数说明

#### 6.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 6.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 配置数据，未变化时为 null |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

> 具体 TLS/Server 配置结构以 BFE 配置文件格式为准。

### 6.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-1-001 | 首次导出 TLS/Server 配置 | 正常参数 | 返回完整配置与 version |
| IN-1-002 | 导出 ClusterConf 含多 Key AIConf | 返回数据 | 验证 ClusterConf.AIConf.Keys/KeyPolicy 与 OpenAPI 写入一致 |
| IN-1-003 | 导出 ClusterConf 含模型定价表 | 返回数据 | 验证 ClusterConf.AIConf.ModelTable 与导入的 model prices 一致 |
| ~~IN-1-004~~ | ~~导出 ClusterConf 含 ModelProtocols~~ | 返回数据 | **编号合并**：同一断言已由 `IN-TLS-1-007` 实现（含 provider 更新后的导出收敛），本编号不再单独实现 |
| IN-TLS-1-009 | 同秒连续变更导出版本严格递增 | 并发语义 | create→export→delete→export 连续 6 轮，版本串严格递增；携带旧版本拉取返回新内容（fixes #142，版本楔死回归锚点） |
| IN-TIER-1-001 | 导出 ClusterConf 含分时段定价 | 返回数据 | 验证 ModelTable 携带 TimeZone/Tiers 与 ModelPrice 的 TierPrices |
| IN-TIER-1-002 | 未配置时段规则时导出固定价格 | 返回数据 | 验证 ModelTable.Tiers 为空，仍按默认 Prices 导出 |

### 6.4 测试场景详细设计

#### 6.4.1 IN-1-001：首次导出 TLS/Server 配置（正常参数）

##### 设计思路

验证不传 version 时返回完整 TLS/Server 配置与 version。

##### 前提数据准备

已创建证书或存在默认 TLS 配置。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/tls_conf/server_data_conf`。
2. 验证返回包含 `Data`、`version` 和 `WorkMode`。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | 非空对象 | IsObject |
| version | 非空字符串 | NotEmpty |
| WorkMode | 非空字符串 | NotEmpty |

---

#### 6.4.2 IN-1-002：导出 ClusterConf 含多 Key AIConf（返回数据）

##### 设计思路

验证通过 OpenAPI 创建的 Cluster（含 `llm_config.keys`/`key_policy`）经 InnerAPI `/configs/tls_conf/server_data_conf` 导出后，`ClusterConf.Config.{cluster_name}.AIConf` 中正确输出 `Keys` 和 `KeyPolicy`。

##### 前提数据准备

1. 通过 POST `/open-api/v1/clusters` 创建集群 `cluster_inner_multi_keys`，其 `llm_config` 包含：
   - `models`: ["deepseek-chat"]
   - `keys`: 2 个 Key（name/key/weight）
   - `key_policy`: strategy="weighted_random", max_retries=3, retry_backoff_initial=100, retry_backoff_max=5000

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/tls_conf/server_data_conf`。
2. 在返回的 `Data.ClusterConf.Config` 中找到目标集群。
3. 校验该集群的 `AIConf.Keys` 和 `AIConf.KeyPolicy` 与写入一致。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.Keys | 长度为 2 | Len=2 |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.Keys[0].Name | "primary" | Equals |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.Keys[0].Key | "sk-aaaaaaaaaaaa" | Equals |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.Keys[0].Weight | 70 | Equals |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.KeyPolicy.Strategy | "weighted_random" | Equals |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.KeyPolicy.MaxRetries | 3 | Equals |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.KeyPolicy.RetryBackoffInitial | 100 | Equals |
| Data.ClusterConf.Config.cluster_inner_multi_keys.AIConf.KeyPolicy.RetryBackoffMax | 5000 | Equals |

---

#### 6.4.3 IN-1-003：导出 ClusterConf 含模型定价表（返回数据）

##### 设计思路

验证通过 OpenAPI 导入模型定价后，`/configs/tls_conf/server_data_conf` 导出的 `ClusterConf.AIConf.ModelTable` 会按 cluster 的 `llm_config.provider` 匹配并输出对应模型列表。

##### 前提数据准备

1. 通过 POST `/open-api/v1/model-prices/import`（`mode=replace`）导入如下 `model-list.yaml`：

   ```yaml
   version: v1.0
   default_currency: RMB
   models:
     - provider: openai
       model: gpt-4o
       base_model: gpt-4o
       mode: chat
       prices:
         input_cost_per_token: 0.0001
     - provider: openai
       model: gpt-4o-mini
       base_model: gpt-4o-mini
       mode: chat
       prices:
         input_cost_per_token: 0.00001
   ```

2. 通过 POST `/open-api/v1/clusters` 创建集群 `cluster_inner_model_price`，其 `llm_config.provider="openai"`。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/tls_conf/server_data_conf`。
2. 在返回的 `Data.ClusterConf.Config` 中找到目标集群。
3. 校验该集群的 `AIConf.ModelTable` 与导入的模型定价一致。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data.ClusterConf.Config.cluster_inner_model_price.AIConf.ModelTable.Currency | "RMB" | Equals |
| Data.ClusterConf.Config.cluster_inner_model_price.AIConf.ModelTable.Models | 长度为 2 | Len=2 |
| Data.ClusterConf.Config.cluster_inner_model_price.AIConf.ModelTable.Models[*].Provider | 全部为 "openai" | Equals |
| Data.ClusterConf.Config.cluster_inner_model_price.AIConf.ModelTable.Models[*].Model | 包含 "gpt-4o" 和 "gpt-4o-mini" | Contains |

---

#### 6.4.4 IN-1-004：导出 ClusterConf 含 ModelProtocols（**已合并至 IN-TLS-1-007**）

本编号与 `IN-TLS-1-007`（`AIConf 包含 ModelProtocols`）的验证目标是同一个：Provider 写入的
`model_protocols` 经导出落到 `ClusterConf.Config.{cluster_name}.AIConf.ModelProtocols`。为避免同一断言
两处维护，`IN-1-004` 不再单独实现，改为在 `IN-TLS-1-007` 中覆盖，并在此基础上补充了**变更传播**断言：

1. Provider 初始 `model_protocols=["anthropic"]` → 导出 `AIConf.ModelProtocols == ["anthropic"]`；
2. PATCH 该 Provider 放宽为 `["openai","anthropic"]` → **再次导出**断言 `ModelProtocols == ["openai","anthropic"]`。

第 2 步是本次补强的部分：它证明导出是**读时解析 provider 记录**，而不是创建集群时的快照——若哪天
`ModelProtocols` 被改成落库快照，该断言会立刻失败（检查项 #5 变更传播）。

### 6.4.5 IN-TLS-1-009：同秒连续变更导出版本严格递增（并发语义）

##### 设计思路

导出版本号为 14 位定宽时间串（`yyyyMMddHHmmss`）。预修复实现下，同一墙钟秒内两次"内容已变"的导出计算出相同版本串，叠加 sign 短路后版本推进被楔死，按版本轮询收敛的数据面消费者无限挂起（issue #142，SC2101-TC018 六连失败的根因）。用例以 create→export→delete→export 连续 6 轮覆盖"同秒"窗口（亚秒级连发必然同秒），每轮断言第二次导出版本**严格大于**第一次；末尾追加收敛语义断言：携带旧版本号拉取，内容已变更时必须返回新内容而非 `Data=null`。

##### 执行步骤

1. 循环 6 轮：创建 cluster（unique 名）→ 导出取版本 v1 → 删除该 cluster → 导出取版本 v2 → 断言 `v2 > v1`（字符串比较）。
2. 再创建一个 cluster，携带 v2 作为 version 参数拉取导出，断言返回 `Data != null` 且 `Version > v2`。

---

### 6.5 IN-TIER-1-001：导出 ClusterConf 含分时段定价（返回数据）

##### 设计思路

验证 OpenAPI 设置 provider `time_zone`/`tiers`、导入 model-prices `tier_prices` 并创建 cluster 后，InnerAPI 导出的 `ClusterConf.AIConf.ModelTable` 能正确携带 `Currency`、`TimeZone`、`Tiers` 以及 `Models[*].TierPrices`。

##### 前提数据准备

1. 创建 Provider `tier_provider`，并通过 `PUT /providers/{provider_name}/pricing-tiers` 设置：
   - `time_zone`: `Asia/Shanghai`
   - `tiers`: `[{name:"peak", time_ranges:[{weekdays:[1,2,3,4,5], start:"09:00", end:"12:00"}, {weekdays:[1,2,3,4,5], start:"14:00", end:"18:00"}]}]`
2. 通过 `POST /open-api/v1/model-prices/import` 导入含 `tier_prices.peak` 的 model-list.yaml。
3. 创建 Cluster，其 `llm_config.provider=tier_provider`、`llm_config.models=["deepseek-chat"]`。

##### 执行步骤

1. GET `/inner-api/v1/configs/tls_conf/server_data_conf`。
2. 在 `Data.ClusterConf.Config` 中找到目标 cluster。
3. 校验 `AIConf.ModelTable`：
   - `Currency` = `"RMB"`
   - `TimeZone` = `"Asia/Shanghai"`
   - `Tiers` 长度为 1，`Tiers[0].Name` = `"peak"`
   - `Models` 长度为 1，`Models[0].Model` = `"deepseek-chat"`
   - `Models[0].Prices.input_cost_per_token` = `0.000002`
   - `Models[0].TierPrices.peak.input_cost_per_token` = `0.000004`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Currency | "RMB" | Equals |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.TimeZone | "Asia/Shanghai" | Equals |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Tiers | 长度为 1 | Len=1 |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Tiers[0].Name | "peak" | Equals |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Models[0].TierPrices.peak.input_cost_per_token | 0.000004 | Equals |

---

### 6.6 IN-TIER-1-002：未配置时段规则时导出固定价格（返回数据）

##### 设计思路

验证 Provider 未设置 `time_zone`/`tiers`、model-prices 未设置 `tier_prices` 时，导出的 `ModelTable` 仍只包含默认 `Prices`，`Tiers` 与 `TierPrices` 为空，保证向后兼容。

##### 前提数据准备

1. 创建 Provider，不设置 `time_zone`/`tiers`。
2. 导入只含 `prices` 的 model-list.yaml。
3. 创建引用该 Provider 的 Cluster。

##### 执行步骤

1. GET `/inner-api/v1/configs/tls_conf/server_data_conf`。
2. 找到目标 cluster 的 `AIConf.ModelTable`。
3. 校验 `Currency` = `"RMB"`，`Tiers` 为空，`Models[0].Prices` 存在，`Models[0].TierPrices` 为空。

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Currency | "RMB" | Equals |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Tiers | 空数组 | Len=0 / Empty |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Models[0].Prices.input_cost_per_token | 与导入一致 | Equals |
| Data.ClusterConf.Config.{cluster}.AIConf.ModelTable.Models[0].TierPrices | 空对象 | Empty |

---

## 7. 导出 GSLB 配置

### 7.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 GSLB 配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/gslb_data/gslb` |
| 说明 | 导出 GSLB 配置，`bfe_cluster` 必填 |

### 7.2 接口参数说明

#### 7.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| version | string | N | 上次返回的版本号 | - |
| bfe_cluster | string | Y | BFE 集群名称 | 必填 |

#### 7.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | GSLB 配置数据 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 7.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-2-001 | 导出 GSLB 缺少 bfe_cluster | 必填校验 | 验证 ErrNum=422 或 404 |
| IN-2-002 | 正常导出 GSLB | 正常参数 | 返回 GSLB 配置与 version |

### 7.4 测试场景详细设计

#### 7.4.1 IN-2-001：导出 GSLB 缺少 bfe_cluster（必填校验）

##### 设计思路

验证 `bfe_cluster` 为必填参数。

##### 前提数据准备

无

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/gslb_data/gslb`，不带 `bfe_cluster`。
2. 验证返回错误码。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：422 或 404  
**ErrMsg**：缺少 bfe_cluster 的错误信息  
**Data**：null

---

#### 7.4.2 IN-2-002：正常导出 GSLB（正常参数）

##### 设计思路

验证传入正确的 `bfe_cluster` 后返回 GSLB 配置与 version。

##### 前提数据准备

已创建 Cluster。

##### 执行步骤

1. 发送 GET 请求，`bfe_cluster=BFE-AI_product.szyf`。
2. 验证返回结构和 `version`。

##### 请求参数

```
bfe_cluster=BFE-AI_product.szyf
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | 非空对象 | IsObject |
| version | 非空字符串 | NotEmpty |
| WorkMode | 非空字符串 | NotEmpty |

---

## 8. 导出集群表配置

### 8.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出集群表配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/gslb_data/cluster_table` |
| 说明 | 导出集群表配置 |

### 8.2 接口参数说明

#### 8.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 8.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 集群表配置数据 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 8.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-3-001 | 导出 cluster_table | 正常参数 | 返回集群表配置 |

### 8.4 测试场景详细设计

#### 8.4.1 IN-3-001：导出 cluster_table（正常参数）

##### 设计思路

验证首次导出集群表配置成功。

##### 前提数据准备

已创建 Cluster。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/gslb_data/cluster_table`。
2. 验证返回结构和 `version`。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | 非空对象 | IsObject |
| version | 非空字符串 | NotEmpty |
| WorkMode | 非空字符串 | NotEmpty |

---

## 9. 导出证书配置

### 9.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出证书配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/protocol/server_cert_conf` |
| 说明 | 导出证书配置 |

### 9.2 接口参数说明

#### 9.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 9.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 证书配置数据 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 9.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-4-001 | 导出证书配置 | 正常参数 | 返回证书配置 |

### 9.4 测试场景详细设计

#### 9.4.1 IN-4-001：导出证书配置（正常参数）

##### 设计思路

验证首次导出证书配置成功。

##### 前提数据准备

已创建证书。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/protocol/server_cert_conf`。
2. 验证返回结构。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | 非空对象 | IsObject |
| version | 非空字符串 | NotEmpty |
| WorkMode | 非空字符串 | NotEmpty |

---

## 10. 导出额外文件

### 10.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出额外文件 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/extra_files/{filename}` |
| 说明 | 导出额外文件 |

### 10.2 接口参数说明

#### 10.2.1 请求参数

##### URI 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| filename | string | Y | 文件名 |

#### 10.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object/string | 文件内容 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 10.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-5-001 | 导出额外文件 | 正常参数 | 返回文件内容 |

### 10.4 测试场景详细设计

#### 10.4.1 IN-5-001：导出额外文件（正常参数）

##### 设计思路

验证导出额外文件成功。

##### 前提数据准备

存在额外文件 `example.data`。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/extra_files/example.data`。
2. 验证返回文件内容。

##### 请求参数

URI：`example.data`

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | 非空 | NotEmpty |
| WorkMode | 非空字符串 | NotEmpty |

---

## 11. 导出 API-Key 配置

### 11.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 API-Key 配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/mod-api-key` |
| 说明 | 导出 API-Key 及配额配置 |

### 11.2 接口参数说明

#### 11.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 11.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 配置数据，未变化时为 null |
| Data.version | string | 配置版本号 |
| Data.config | object | 按产品线分组的 API-Key 路由规则 |
| Data.QuotaPlans | object | 按产品线分组的配额计划定义 |
| Data.tokens | object | 按产品线分组的 Token 配置 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 11.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-6-001 | 首次导出 mod-api-key | 正常参数 | 返回 config/QuotaPlans/tokens |
| IN-6-002 | 增量同步未变化 | 正常参数 | Data=null |
| IN-6-003 | 增量同步配置变化 | 正常参数 | 返回新配置与新 version |

### 11.4 测试场景详细设计

#### 11.4.1 IN-6-001：首次导出 mod-api-key（正常参数）

##### 设计思路

验证不传 version 时返回完整 API-Key 配置。

##### 前提数据准备

已创建 API-Key。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/mod-api-key`。
2. 验证返回包含 `version`、`config`、`QuotaPlans`、`tokens`。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| version | 非空字符串 | NotEmpty |
| config | 对象 | IsObject |
| QuotaPlans | 对象 | IsObject |
| tokens | 对象 | IsObject |
| WorkMode | 非空字符串 | NotEmpty |

---

#### 11.4.2 IN-6-002：增量同步未变化（正常参数）

##### 设计思路

验证传入上次 version 且配置未变化时，返回 `Data=null`。

##### 前提数据准备

已获取 version=XXX 且未变更配置。

##### 执行步骤

1. 发送 GET 请求，`version=XXX`。
2. 验证返回 `Data=null`。

##### 请求参数

```
version=XXX
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Data | null | IsNull |
| WorkMode | 非空字符串 | NotEmpty |

---

#### 11.4.3 IN-6-003：增量同步配置变化（正常参数）

##### 设计思路

验证配置变化后传入旧 version，返回新配置与新 version。

##### 前提数据准备

已获取 version=XXX，随后创建新 API-Key。

##### 执行步骤

1. 创建新 API-Key。
2. 发送 GET 请求，`version=XXX`。
3. 验证返回新配置与新 version。

##### 请求参数

```
version=XXX
```

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| version | 非空字符串，且不等于 XXX | NotEmpty / NotEquals(XXX) |
| tokens | 对象 | IsObject |
| WorkMode | 非空字符串 | NotEmpty |

---

## 12. 导出请求体处理配置

### 12.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出请求体处理配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/mod-body-process` |
| 说明 | 导出请求体处理模块配置 |

### 12.2 接口参数说明

#### 12.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 12.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 配置数据，未变化时为 null |
| Data.Version | string | 配置版本号 |
| Data.Config | object | 按产品线组织的请求体处理配置 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 12.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-7-001 | 导出 mod-body-process | 正常参数 | 返回 Version、Config |

### 12.4 测试场景详细设计

#### 12.4.1 IN-7-001：导出 mod-body-process（正常参数）

##### 设计思路

验证首次导出请求体处理配置成功。

##### 前提数据准备

无

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/mod-body-process`。
2. 验证返回结构和字段。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Version | 非空字符串 | NotEmpty |
| Config | 对象 | IsObject |
| WorkMode | 非空字符串 | NotEmpty |

---

## 13. 导出限流策略配置

### 13.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出限流策略配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/rate-limit-policy` |
| 说明 | 导出限流策略配置 |

### 13.2 接口参数说明

#### 13.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 13.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 配置数据，未变化时为 null |
| Data.Config | object | 按产品线组织的路由规则 |
| Data.RateLimitPolicies | object | 限流策略定义 |
| Data.ApikeyRateLimitPolicyBindings | object | API-Key 到策略 ID 列表的绑定关系 |
| Data.Version | string | 配置版本号 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 13.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-8-001 | 导出 rate-limit-policy | 正常参数 | 返回 Config/RateLimitPolicies/Bindings/Version |

### 13.4 测试场景详细设计

#### 13.4.1 IN-8-001：导出 rate-limit-policy（正常参数）

##### 设计思路

验证首次导出限流策略配置成功。

##### 前提数据准备

已创建含限流策略的 API-Key 或 Entity。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/rate-limit-policy`。
2. 验证返回结构和字段。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Config | 对象 | IsObject |
| RateLimitPolicies | 对象 | IsObject |
| ApikeyRateLimitPolicyBindings | 对象 | IsObject |
| Version | 非空字符串 | NotEmpty |
| WorkMode | 非空字符串 | NotEmpty |

---

## 14. 导出 AI 路由配置

### 14.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 AI 路由配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/ai-route` |
| 说明 | 导出 AI 网关路由配置 |

### 14.2 接口参数说明

#### 14.2.1 请求参数

##### Query 参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 14.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| ErrNum | int | 返回码 |
| Data | object | 配置数据，未变化时为 null |
| Data.Version | string | 配置版本号 |
| Data.RouteRules | object | 所有路由表的集合 |
| Data.ApikeyRouteTableBindings | object | API-Key 到路由表查找顺序的映射 |
| ErrMsg | string | 文本消息 |
| WorkMode | string | 控制台工作模式 |

### 14.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-9-001 | 导出 ai-route | 正常参数 | 返回 Version/RouteRules/Bindings |
| IN-9-002 | ai-route 未启用路由表不导出 | 业务规则 | RouteRules 为空 |

### 14.4 测试场景详细设计

#### 14.4.1 IN-9-001：导出 ai-route（正常参数）

##### 设计思路

验证导出 AI 路由配置成功，包含启用路由规则的路由表。

##### 前提数据准备

已创建含启用路由规则的 API-Key/Entity/Global Route。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/ai-route`。
2. 验证返回结构和字段。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| Version | 非空字符串 | NotEmpty |
| RouteRules | 对象 | IsObject |
| ApikeyRouteTableBindings | 对象 | IsObject |
| WorkMode | 非空字符串 | NotEmpty |

---

#### 14.4.2 IN-9-002：ai-route 未启用路由表不导出（业务规则）

##### 设计思路

验证仅导出 `route_rules.enabled=true` 的路由表；全部禁用时 `RouteRules` 为空。

##### 前提数据准备

已创建但禁用所有路由规则（API-Key/Entity/Global Route 的 route_rules.enabled=false）。

##### 执行步骤

1. 发送 GET 请求到 `/inner-api/v1/configs/ai-route`。
2. 验证 `RouteRules` 为空对象。

##### 请求参数

无

##### 预期返回结果

**ErrNum**：200  
**ErrMsg**：success

**Data 字段校验**：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| RouteRules | 空对象 | IsEmpty |
| ApikeyRouteTableBindings | 对象（可能为空） | IsObject |
| WorkMode | 非空字符串 | NotEmpty |

---

## 15. 导出 EPP 配置

### 15.1 接口信息

| 项目 | 值 |
|-----|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 EPP 配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/epp_data/config` |
| 说明 | 导出 EPP 调度配置（编译后 `epp_config` 段 + `assignment` 分配段），支持 version 增量同步 |

### 15.2 接口参数说明

#### 15.2.1 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | N | 上次返回的版本号 |

#### 15.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| Data.Version | string | 配置版本号 |
| Data.Config.epp_config | object | `map[cluster名]EndpointPickerConfig`，由 OpenAPI 写入的简化 `epp_config` 确定性编译而来 |
| Data.Config.assignment | object | `map[cluster名]{primary, standby}`，EPP 实例分配（未分配 cluster 不出现在段内） |
| WorkMode | string | 控制台工作模式 |

### 15.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-EPP-001 | 空池创建 EPP 集群进入未分配态并降级导出 | 降级语义 | 空池上创建 EPP cluster 进入未分配态；`server_data_conf` 导出降级为 WRR、无 EPPAddr |
| IN-EPP-002 | 恢复容量后 reconciler 自动分配并回到 EPP | 自动分配 | PATCH `/epp-pool` 注入实例后 reconciler 自动选主；导出 BalanceMode=EPP 且 EPPAddr=[primary, standby] |
| IN-EPP-003 | epp_data 导出含编译后 epp_config 与 assignment 两段 | 返回数据 | 创建带完整 `epp_config`（含 `flow_control`）的 cluster，校验导出编译产物：flowControl 段各字段、**`priorityBands` 显式下发 band 0**（fixes #198）、插件链、**session-scorer `strategy=session_id` 显式下发**（fixes #181）、profile 权重、assignment 与分配视图一致 |
| IN-EPP-004 | epp_data 增量拉取同版本返回 Data null | 增量同步 | 携带当前 version 再拉取返回 `Data=null` |
| IN-EPP-005 | 手工覆写后版本推进且 assignment 更新 | 版本推进 | PUT `/epp-assignments/{cluster}` 覆写 primary 后版本 bump、assignment 更新 |

IN-EPP-003 中 `flow_control` 写入 `{"max_requests": 200, "queue_ttl": 45, "no_endpoint_queue_ttl": 120, "enable_eviction": true}`，导出 `flowControl` 段校验要点：

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| maxRequests | "200" | Equals |
| defaultRequestTTL | "45s" | Equals |
| noEndpointRequestTTL | "2m0s" | Equals |
| enableEviction | true | Equals |
| priorityBands | 长度 1，元素 `{"priority":0,"maxRequests":"200","maxBytes":"5Gi"}` | Len=1 / Equals（band0 与全局一致，锁"不静默截断"契约） |

---

## 16. 导出 AI 缓存规则配置

### 16.1 接口信息

| 项目 | 值 |
|-----|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 AI 缓存规则配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/ai-cache-rule` |
| 说明 | 导出 BFE `mod_ai_cache` 规则文件（`ai_cache.data`），支持 version 增量同步；一期每条规则仅导出 `cond`/`cacheKeyStrategy`/`cacheTTL`/`maxBodyBytes`/`maxValueBytes` 5 个 tag（大写驼峰，与 Open API 词汇不同） |

### 16.2 接口参数说明

#### 16.2.1 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | 否 | 上次返回的版本号；相同（内容未变）时 Data 为 null |

#### 16.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| Data.Config | object | 按产品线组织的缓存规则（key=`AI_product`，取自 `AIRouteInnerProductName`）；空集合导出空数组 `[]` 且 product 键 present |
| Data.Config.AI_product[] | object | 规则按 first-match-wins 匹配，数组顺序即优先级（= Open API PUT 提交顺序） |
| Data.Config.AI_product[].cond | string | BFE 条件表达式（必含） |
| Data.Config.AI_product[].cacheKeyStrategy | string | 缓存键策略（`disabled` 为缓存豁免） |
| Data.Config.AI_product[].cacheTTL | int | 缓存 TTL（秒），0 表示不过期 |
| Data.Config.AI_product[].maxBodyBytes | int64 | 请求体大小上限 |
| Data.Config.AI_product[].maxValueBytes | int64 | 缓存值大小上限 |
| Data.Version | string | 配置版本号（`yyyyMMddHHmmss` 14 位定宽） |
| WorkMode | string | 控制台工作模式 |

合同锁定：一期不得导出 `cacheKeyFrom`/`cacheValueFrom`/`cacheStreamValueFrom`/`cacheToolCallsFrom`/`responseTemplate`/`streamResponseTemplate`（二期/预留字段）。

### 16.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| AICE-1-001 | 首拉 | 正常参数 | PUT 2 条 → Config.AI_product 长度 2、顺序=提交顺序、5 个导出 tag 精确集合且值正确；对原始 body 文本断言 `"cacheKeyStrategy":"lastQuestion"` 等（家族8 文本形态） |
| AICE-1-002 | 二期字段缺席 | 合同一致性 | 导出 body 不得含 6 个二期/预留字段（家族8） |
| AICE-1-003 | 增量同步 | 正常参数 | 带当前 version 再拉 → Data null；不传 version 再拉 → 有数据且 Version 相同 |
| AICE-1-004 | 版本单调 | 并发语义 | 同一秒内连续两次不同 PUT → 轮询导出（500ms×10）直到版本严格大于旧版本且内容收敛为最新集合（家族8/#142） |
| AICE-1-005 | 清空导出 | 边界值 | PUT `{"rules":[]}` → Config.AI_product 为 `[]`（非 null，product 键 present）（家族8） |
| AICE-1-006 | 422 防泄漏 | 异常参数 | PUT 被拒 → 带旧 version 增量拉取 Data null、Version 不变且内容与拒绝前一致（家族4） |

> 逐用例明细（前置、步骤、请求、预期）见 `tests/ai_cache/design.md` 第 8 节。

---

## 17. 导出流量镜像规则配置

### 17.1 接口信息

| 项目 | 值 |
|-----|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出流量镜像规则配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/traffic-mirror-rule` |
| 说明 | 导出 BFE `mod_traffic_mirror` 规则文件（`mirror_rule.data`），支持 version 增量同步；每条规则**全字段 7 个 tag 恒输出**（`cond`/`mirrorCluster`/`percentage`/`removeHeaders`/`setHeaders`/`bodyRewrites`/`pathRewrite`，大写驼峰，与 Open API 词汇不同）；空值输出零值（`{}`/`[]`/`""`），不用 omitempty；`removeHeaders` 两层默认语义（缺省填默认黑名单 `["Authorization","Cookie","X-Api-Key"]`、显式 `[]` 不剔除） |

### 17.2 接口参数说明

#### 17.2.1 请求参数

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| version | string | 否 | 上次返回的版本号；相同（内容未变）时 Data 为 null |

#### 17.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| Data.Config | object | 按产品线组织的镜像规则（key=`AI_product`，取自 `AIRouteInnerProductName`）；空集合导出空数组 `[]` 且 product 键 present |
| Data.Config.AI_product[] | object | 规则按 first-match-wins 匹配，数组顺序即优先级（= Open API PUT 提交顺序）；每条规则恰含 7 个导出 tag |
| Data.Config.AI_product[].cond | string | BFE 条件表达式（控制面必填，不生成空 cond） |
| Data.Config.AI_product[].mirrorCluster | string | 镜像目标 cluster 名（必含、显式非空） |
| Data.Config.AI_product[].percentage | int | 镜像采样百分比（0-100） |
| Data.Config.AI_product[].removeHeaders | array | 剔除 Header 黑名单；**缺省填默认黑名单，显式 `[]` 导出不剔除（两层默认语义，BFE 自身缺省为空列表）** |
| Data.Config.AI_product[].setHeaders | object | 注入 Header（空输出 `{}`）；`X-Bfe-Mirror` 由 BFE 缺失时兜底注入，不随导出携带 |
| Data.Config.AI_product[].bodyRewrites | array | body 字段改写（空输出 `[]`；一期 `path` 仅 `"model"`） |
| Data.Config.AI_product[].pathRewrite | string | 镜像路径改写（空输出 `""`） |
| Data.Version | string | 配置版本号（`yyyyMMddHHmmss` 14 位定宽） |

### 17.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| TMIE-1-001 | 首拉与全字段文本形态 | 正常参数 | PUT 2 条（缺省 remove_headers + 显式全字段）→ 7 tag 精确集合与值；原始 body 文本断言 `"mirrorCluster"`、`"removeHeaders":["Authorization","Cookie","X-Api-Key"]`、`"setHeaders":{}`、`"bodyRewrites":[{"path":"model","value":"..."}]`、`"pathRewrite":""` 与 `"/v1/internal/chat/completions"`（家族8 文本形态/#102） |
| TMIE-1-002 | 显式空黑名单导出 | 正常参数 | 显式 `remove_headers:[]` 的规则导出 `"removeHeaders":[]`（与缺省填默认黑名单对照，锁两层默认语义，家族1） |
| TMIE-1-003 | 增量同步 | 正常参数 | 带当前 version 再拉 → Data null；不传 version 再拉 → 有数据且 Version 相同 |
| TMIE-1-004 | 版本单调 | 并发语义 | 同一秒内连续两次不同 PUT → 轮询导出（500ms×10）直到版本严格大于旧版本且内容收敛（家族8/#142） |
| TMIE-1-005 | 清空导出 | 边界值 | PUT `{"rules":[]}` → Config.AI_product 为 `[]`（非 null，product 键 present，body 文本 `"AI_product":[]`）（家族8） |
| TMIE-1-006 | 422 防泄漏 | 异常参数 | PUT 被拒 → 带旧 version 增量拉取 Data null、Version 不变且内容与拒绝前一致（家族4） |

> 逐用例明细（前置、步骤、请求、预期）见 `tests/traffic_mirror/design.md` 第 8 节。

---

## 18. K8s 实例池维护

### 16.1 接口信息

| 项目 | 值 |
|-----|-----|
| 模块 | InnerAPI |
| 接口名称 | K8s 实例池维护（`k8s_pools`） |
| 方法 | PUT（`/k8s_pools/{name}/instances`）、GET（`/k8s_pools/{name}`、`/k8s_pools`）、DELETE（`/k8s_pools/{name}`） |
| 说明 | 资源读写接口，供 K8s 发现组件写入发现的实例快照；不支持 `version` 增量同步；DELETE 无引用保护 |

### 16.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| Data.name | string | pool 名称 |
| Data.instances | array | 实例列表（元素 `{addr, port, weight}`，weight 缺省 100） |
| Data.instance_count | int | 实例数量 |
| Data.last_sync_time | int64 | 最近同步时间（unix 秒），取条目 `updated_at` |
| ErrNum | int | 200 成功；404 pool 不存在；422 参数非法 |
| WorkMode | string | 控制台工作模式 |

### 16.3 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-K8S-001 | PUT 幂等 upsert 全生命周期 | 正常参数 | 创建（weight 缺省 100）→ GET 单个 → 全量替换 → 重复 PUT 幂等 → GET 列表 → DELETE → GET 404 → 重复 DELETE 404 |
| IN-K8S-002 | 参数校验 | 必填/边界校验 | 路径名非法、缺 addr、addr/port 重复、weight 越界均 422；空数组（零实例）合法 |
| IN-K8S-003 | 共享池读路径与 last_sync_time | 返回数据 | N:1 读场景下 GET 单个返回完整实例列表；再次 PUT 后 `last_sync_time` 不回退 |
| IN-K8S-007 | 4xx 回读零变更 | 假事务/校验层回归 | 对已存在 pool 的非法 PUT（重复 addr/port）→ 422 后 pool 逐字段不变；非法路径名下 GET/DELETE 返回 422（区别于 404） |
| IN-K8S-008 | 并发全量替换（MySQL） | 并发语义（`//go:build mysql`，需 `AIAPI_MYSQL_DSN`） | 20 并发对同一 pool PUT 不同实例列表：全部 2xx、无 500/deadlock、最终读回与某一提交 payload 完全一致（last-write-wins） |

> `k8s_pools` 写入后的实时解析、cluster_table 导出跟随、provider 模式切换等跨域链路用例见 `tests/provider/k8s_pool/`（PV-K8S 系列）。PUT/DELETE 只写 `k8s_pools` 单表，不刷新 provider 镜像、不做派生池 fan-out。

---

## 19. 依赖与数据准备

1. 需要预先通过 OpenAPI 创建 API-Key、Entity、Cluster、证书、Global Route 等数据，才能验证导出内容非空；验证模型定价表时需先导入 model prices 并创建对应 provider 的 Cluster；验证分时段定价时需先设置 provider 的 `time_zone`/`tiers` 并导入含 `tier_prices` 的 model prices。
2. `/configs/gslb_data/gslb` 依赖正确的 `bfe_cluster` 参数，通常为 `BFE-AI_product.szyf`。
3. InnerAPI 鉴权为 `McUserProbe`，测试环境需配置为可跳过或使用 Support Token。

## 19. normalize_upstream_error 导出用例（2026-10-06 上游错误体归一）

### 19.1 测试场景总览

| 编号 | 场景 | 测试类型 | 简要说明 |
|------|------|---------|---------|
| IN-NUE-1-001 | 导出 AIConf.NormalizeUpstreamError 存在性与取值 | 返回数据 | 创建全字段配置集群 → 导出断言 Enabled/StreamEnabled/UnrecognizedAction/MaxBodyBytes/RedactSecrets 逐字段（检查项 #8） |
| IN-NUE-1-002 | redact_secrets 显式 false 导出 | 返回数据 | 验证显式 false 不被默认值污染（指针直通回归锚点） |
| IN-NUE-1-003 | 未配置集群导出为 null | 返回数据 | 验证 `AIConf.NormalizeUpstreamError == null`（BFE 关闭，合同见 api-changes §3） |

### 19.2 详细设计要点

- 与 IN-1-002 同模式：创建集群后拉取 `/inner-api/v1/configs/tls_conf/server_data_conf`，
  下钻 `ClusterConf.Config.<cluster>.AIConf`；
- 数值断言对反序列化值（MaxBodyBytes 无精度风险，int64 直比）。

## 20. EnsureReasoningContent 导出用例（2026-11-03 思考模式推理内容补齐）

### 20.1 测试场景总览

| 编号 | 验证点 | 检查项 |
|------|--------|--------|
| IN-ERC-1-001 | 配置 `ensure_reasoning_content=["openai","anthropic"]` → 导出 `AIConf.EnsureReasoningContent` 精确等于该数组（PascalCase 键）；snake_case 键不得出现 | #8 导出存在性 + 取值 + 映射方向 |
| IN-ERC-1-002 | 单协议 `["openai"]` 导出一致 | #8 |
| IN-ERC-1-003 | 未配置集群：键存在且为 `null`（导出结构体不带 `omitempty`，bfe 反序列化为 nil = 关闭） | 缺省语义（与 §19 NormalizeUpstreamError 同口径） |

### 20.2 设计要点

- 合同依据：`design-docs/api-define/InnerAPI接口定义/server-data-conf.md` 的 `AIConf.EnsureReasoningContent` 行，
  以及 `modifications/2026-11-03-cluster-reasoning-content-protocols/`；
- **snake_case 不泄漏**是本组用例的核心断言之一：OpenAPI 侧字段名是 `ensure_reasoning_content`，导出侧必须是
  `EnsureReasoningContent`；映射漏写时 bfe 会当成"未配置"静默关闭，只有这条断言能发现；
- **null 语义**：导出结构体不带 `omitempty`，未配置时键存在且为 `null`，与 `NormalizeUpstreamError` 实测一致；
  该断言同时防止后续加 `omitempty` 悄悄改变契约（bfe 侧虽等价，但契约变化需显式决策）；
- 集群 CRUD 侧的字段校验/回读/防泄漏覆盖在 `tests/clusters/`（CL-1-201~207、CL-4-201~206）；本目录只锁导出契约，
  两者按"写入语义"与"导出契约"分工，不重复。

## 21. 注意事项

1. InnerAPI 返回值仍包含 `WorkMode`（与 OpenAPI v0.3.0 不同，InnerAPI 未移除该字段）。
2. 配置未变化时 `Data=null`，不要断言为空对象 `{}`。
3. 测试用例应尽量验证 `version` 变化：首次非空、未变化时一致且 Data 为 null、变化后更新。
4. 增量同步测试对时间敏感，创建新资源后应立即再次拉取。
