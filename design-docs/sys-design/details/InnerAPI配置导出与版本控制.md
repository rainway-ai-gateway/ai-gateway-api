# InnerAPI 配置导出与版本控制

## 1. 概述

`ai-gateway-api` 除了面向管理员的 OpenAPI 外，还提供一组面向数据面的 **InnerAPI**（路由前缀 `/inner-api/v1`），供 BFE、Conf Agent 等下游组件拉取运行时配置。

由于 BFE 配置通常较大且变更不频繁，系统引入了 **版本控制机制**：

- 每次导出时计算配置数据的 **MD5 签名**；
- 与上一次导出的签名比较，若相同则返回 `Data: nil`，避免重复下发；
- 若不同则生成新的版本号（14 位秒级时间戳格式），并持久化到 `config_versions` 表。

版本号在同一 Topic 内**严格单调递增**：即使多次配置变更落在同一墙钟秒内，版本号也会逐次 +1 秒抬升，消费方按版本号轮询始终能感知变更（issue #142 修复，见 `design-docs/modifications/2026-09-07-issue-142-export-version-freeze/`）。

这样下游组件通过携带 `version` 查询参数即可实现 **增量同步**。

---

## 2. 核心抽象

### 2.1 `VersionControlManager`

```go
type VersionControlManager struct {
    storager VersionControlStorager
    txn itxn.TxnStorager
}

func (vcm *VersionControlManager) ExportConfig(
    ctx context.Context,
    configTopic string,
    generator ConfigGenerator,
) (*ExportData, error)
```

`ConfigGenerator` 是实际生成配置数据的回调函数：

```go
type ConfigGenerator func(ctx context.Context) (*ExportData, error)
```

### 2.2 `ExportData`

```go
type ExportData struct {
    Topic string
    DataWithoutVersion VersionValuable

    version string
    DataSignWithoutVersion string
}
```

- `Topic`：配置主题，对应 `config_versions.name`；
- `DataWithoutVersion`：最终返回给下游的配置结构体，需实现 `UpdateVersion(version string) error`；
- `DataSignWithoutVersion`：对配置数据（版本号置零后）计算的 MD5 签名；
- `version`：由版本控制管理器写入的实际版本号。

### 2.3 签名与版本号

```go
func Sign(data interface{}) (string, error) {
    bs, err := json.Marshal(data)
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("%x", md5.Sum(bs)), nil
}

func Version(t time.Time) string {
    return t.Format("20060102150405")
}

var ZeroVersion = Version(time.Time{})
```

生成签名前，会先把配置结构体中的版本号字段更新为 `ZeroVersion`（即 `"00010101000000"`），避免版本号变化影响签名。

### 2.4 存储层 `config_versions`

| 字段 | 说明 |
|------|------|
| `id` | 自增主键 |
| `name` | 配置主题（Topic） |
| `data_sign` | 配置数据 MD5 签名 |
| `version` | 对应版本号 |

同一 Topic 下可有多条记录，按版本号严格递增。表上具有 `UNIQUE(name, version)` 约束（`uk_name_version`），数据库层保证同一主题不会出现重复版本行。

`VersionControlStorager.UpsertConfigLastExportedVersion` 的行为：

1. 按 `ORDER BY version DESC, id DESC` 读取该 Topic 的最新一行（平局时取 `id` 最大即最晚插入的行）；
2. 若最新行 `data_sign` 与当前重建内容的签名一致，直接返回其版本号（内容未变，不新增记录）；
3. 否则计算候选版本 `max(当前秒, 最新版本 + 1s)` 并插入新记录；
4. 若插入命中唯一约束（并发导出交错等残余竞态），重读最新行、版本再 +1s 重试（上限 3 次），不再整单报错。

> 说明：第 3 步的 +1s 抬升使版本号不依赖墙钟前进——同秒连续变更、时钟回拨/偏移场景下，版本号仍保持同 Topic 内严格单调递增。修复前版本号直接取 `time.Now()` 的秒级时间串，同秒第二次"内容已变"的导出会产生同 `(name, version)` 重复行，且 sign 短路可能将版本推进永久楔死（issue #142）。

---

## 3. InnerAPI 接口清单

| 路径 | 对应 Manager | 配置主题 | 说明 |
|------|-------------|---------|------|
| `GET /configs/tls_conf/server_data_conf` | `RouteRuleManager` | `route_rule` | 域名、基础/高级路由规则、集群配置 |
| `GET /configs/gslb_data/gslb` | `ClusterManager` | `gslb.<bfe_cluster>` | GSLB 调度配置，需 `bfe_cluster` 参数 |
| `GET /configs/gslb_data/cluster_table` | `ClusterManager` | `cluster_table` | 集群表（后端实例列表） |
| `GET /configs/protocol/server_cert_conf` | `CertificateManager` | `certificate` | TLS 证书配置 |
| `GET /configs/extra_files/{filename}` | `ExtraFileManager` | 无 | 附加文件原始内容 |
| `GET /configs/mod-api-key` | `APIKeyRuleManager` | `mod_api_key_rule` | API-Key 校验规则与配额计划 |
| `GET /configs/mod-body-process` | `ModBodyProcessManager` | `mod_body_process` | 请求体处理模块配置 |
| `GET /configs/rate-limit-policy` | `RateLimitPolicyManager` | `mod_ai_rate_limit` | 限流策略配置 |
| `GET /configs/ai-cache-rule` | `AICacheManager` | `mod_ai_cache` | AI 缓存规则配置（BFE `mod_ai_cache` 数据源，产物 `ai_cache.data`） |
| `GET /configs/mod-ai-intent` | `IntentConfigManager` | `intent_config` | AI 意图配置（BFE `mod_ai_intent` 数据源，产物 `intent_questions.data`；Data 即文件内容原样，未发布/未变化返回 `Data: null`） |
| `GET /configs/ai-route` | `AIRouteExporter` | `ai_route` | AI 路由规则与绑定关系 |

---

## 4. 各配置导出详解

### 4.1 Server Data（`/configs/tls_conf/server_data_conf`）

由 `model/iroute_conf/exporter.go` 实现，导出三类 BFE 配置：

```go
type RouteRuleExportData struct {
    Version string
    HostTable *host_rule_conf.HostTableConf
    RouteTable *route_rule_conf.RouteTableFile
    ClusterConf *cluster_conf.BfeClusterConf
}
```

生成流程：

1. 查询所有 `domains`；
2. 查询所有 `clusters` 及子集群/实例池；
3. 查询所有 `products`；
4. 组装 HostTable、RouteTable、ClusterConf；
5. 调用 `VersionControlManager.ExportConfig`。

其中 `RouteTable` 包含产品级高级路由规则（来自 `route_advance_rules`），`ClusterConf` 包含集群转发参数。

> 说明：当管理面 Cluster 配置了 `llm_config` 时，导出后的 `ClusterConf.Config.<cluster_name>.AIConf` 会包含模型映射、认证密钥、模型定价表、前缀路由裁剪开关、协议风格列表、Key 亲和性策略以及上游错误体归一配置，供 BFE 侧 AI 转发模块使用。`AIConf` 由 `llm_config.models`/`model_mappings`/`keys`/`key_policy`/`key_affinity`/`normalize_upstream_error`/`price_miss_action`/`provider`/`match_prefix`/`strip_prefix` 以及 provider 的 `model_protocols` 转换而来，字段位置为 `ClusterConf.Config.<cluster_name>.AIConf`。`llm_config.price_miss_action`（snake_case）映射为 `AIConf.PriceMissAction`（PascalCase）；nil/空串不导出，bfe 侧 `AIConfCheck` 归一为 `fallback_chat`（issue #1479）。
>
> `AIConf.Provider` 对应 OpenAPI `llm_config.provider`，用于关联 `model_prices` 表；`AIConf.ModelTable` 由 InnerAPI 根据 `Provider` 查询 `model_prices` 自动填充，不在 OpenAPI `/clusters` 端点中展示。`AIConf.MatchPrefix`/`StripPrefix` 对应 OpenAPI `llm_config.match_prefix`/`strip_prefix`，由 BFE 在转发前决定是否裁剪请求 `model` 字段前缀。`AIConf.ModelProtocols` 对应 Provider 的 `model_protocols`，供 BFE 判断请求协议风格是否被当前集群支持，例如识别 `anthropic` 请求并注入 `x-api-key` 与 `anthropic-version` 头。`model_prices` 变更后不会同步触发 InnerAPI 推送，BFE/Conf Agent 按自身周期拉取配置并热加载。

### 4.2 GSLB（`/configs/gslb_data/gslb`）

由 `model/icluster_conf/exporter.go` 实现，根据请求的 `bfe_cluster` 参数返回对应 BFE 集群的 GSLB 配置：

```go
type GSLBConf struct {
    Version string
    gslb_conf.GslbConf
}
```

Topic 为 `gslb.<bfe_cluster_name>`，因此不同 BFE 集群有独立的版本线。

生成流程：

1. 查询所有 `clusters`；
2. 对每个集群，取其 `lb_matrices` 中对应 `bfe_cluster` 的调度矩阵；
3. 组装 `GslbConf.Clusters`。

### 4.3 Cluster Table（`/configs/gslb_data/cluster_table`）

同样由 `model/icluster_conf/exporter.go` 实现，导出所有集群的后端实例：

```go
type ClusterTableConf struct {
    cluster_table_conf.ClusterTableConf
}
```

生成流程：

1. 查询所有 `clusters`；
2. 对每个集群，遍历其 `sub_clusters`；
3. 对每个子集群，遍历其 `InstancePool.Instances`；
4. IPv6 地址自动加 `[]` 包裹；
5. 输出 BFE 标准的 `cluster_table_conf`。

> 说明：
> - 管理面 Cluster 对外模型精简（隐藏 `ready`、`sub_clusters`、`scheduler`）不影响 InnerAPI 导出。数据面仍从 `clusters`、`sub_clusters`、`pools`、`lb_matrices` 等存储模型生成 `cluster_table` 与 `gslb` 配置。
> - 实例 `Weight` 直接透传管理面配置：`Weight=0` 表示该实例不接收流量，不会被后端强制改为默认值。

### 4.4 Server Cert（`/configs/protocol/server_cert_conf`）

由 `model/iprotocol/exporter.go` 实现：

```go
type ServerCertConf struct {
    server_cert_conf.BfeServerCertConf
}
```

生成流程：

1. 查询所有 `certificates`；
2. 标记默认证书；
3. 将证书/密钥文件路径写入配置；
4. `UpdateVersion` 时会对文件路径做版本化替换（如 `tls_conf_<version>/...`），便于 BFE 按版本加载。

### 4.5 Extra Files（`/configs/extra_files/{filename}`）

特殊接口，不使用版本控制：

```go
var ExportExtraFileEndpoint = &xreq.Endpoint{
    RegisterHandler: func(router *mux.Router) *mux.Route {
        return router.PathPrefix("/configs/extra_files/").Methods(http.MethodGet)
    },
    Handler: xreq.RawConvert(ExportExtraFileAction),
}
```

直接从 `extra_files` 表按文件名读取 `content` 并原样返回（`application/octet-stream`）。

### 4.6 mod-api-key（`/configs/mod-api-key`）

由 `model/imods/mod_api_key_rule.go` 实现，是 BFE `mod_api_key` 模块的完整输入：

```go
type ModAPIKeyRuleConf struct {
    Version *string `json:"version"`
    Config map[string][]*TokenRuleFile `json:"config"`
    QuotaPlans map[string][]*QuotaPlan `json:"QuotaPlans"`
    Tokens map[string]map[string]*TokenFile `json:"tokens"`
}
```

生成流程：

1. 构造 AI 路由对应的 API-Key 规则（`buildAIRouteAPIKeyRules`）；
2. **批量预加载**：一次性加载全部 `api_keys`、`entities`、`quota_plans`、`entity_types` 到内存索引（Map），避免 N+1 查询；
3. 遍历所有 `api_keys`，为每个 key 生成 `TokenFile`；
4. 根据 API-Key 的 `enabled` 字段及 Entity 层级的 `allow_models` 交集结果，确定导出的 `enabled`（布尔值）；`expired`/`exhausted` 状态由 BFE 根据 `expired_time` 和实时 Redis 配额余额自行判断，不再由导出层计算；
5. 合并 Entity 层级的 `allow_models`（交集）与 `block_models`（并集），通过内存 Map 沿 `parent_id` 回溯，不再递归查询数据库；
6. 收集 API-Key 自身及 Entity 层级向上的配额计划，**跳过 `unlimited=true` 的配额计划**，同样通过内存 Map 查询；
7. 为每个 Entity 标签补充 `TagLevel`（取自内存 Map 中对应 `EntityType.Level`）；
8. 输出 `QuotaPlans`、`Tokens`、`Config`。

> 详见《API-Key 与 Entity 关联及模型继承.md》。
>
> 性能优化记录：参见 `design-docs/modifications/2026-08-24-mod-api-key-export-performance/design-changes.md`。

### 4.7 mod-body-process（`/configs/mod-body-process`）

由 `model/imods/mod_body_process.go` 实现，当前为占位实现：

```go
type ModBodyProcessConf struct {
    Version *string `json:"Version"`
    Config map[string][]string `json:"Config"`
}
```

仅输出一个空的配置列表，供 BFE 模块初始化使用。

### 4.8 rate-limit-policy（`/configs/rate-limit-policy`）

由 `model/rate_limit_policy/rate_limit_policy_manager.go` 实现：

```go
type ExportRateLimitPolicyConfig struct {
    Config map[string][]*ExportRouteRule `json:"Config"`
    RateLimitPolicies map[string]*ExportRateLimitPolicy `json:"RateLimitPolicies"`
    ApikeyRateLimitPolicyBindings map[string][]string `json:"ApikeyRateLimitPolicyBindings"`
    Version string `json:"Version"`
}
```

生成流程：

1. 查询所有 `rate_limit_policies`；
2. 遍历所有 `api_keys`；
3. 收集每个 API-Key 自身及其 Entity 层级向上的限流策略 ID；
4. **跳过 `enabled=false` 的策略**，不导出也不生成绑定；
5. 将启用的策略转换为 BFE 格式并输出绑定关系。

> 详见《限流策略与导出.md》。

### 4.9 ai-route（`/configs/ai-route`）

由 `model/imods/ai_route_exporter.go` 实现：

```go
type AiRouteDataExport struct {
    Version string `json:"Version"`
    RouteRules map[string]*RouteTableExport `json:"RouteRules"`
    ApikeyRouteTableBindings map[string][]string `json:"ApikeyRouteTableBindings"`
}
```

生成流程：

1. 查询所有 `api_keys` 和 `entities`；
2. 查询 Global 路由规则；
3. 对每个 API-Key，依次检查其 API-Key 级、**Entity 层级向上（沿 `parent_id` 遍历）**、Global 级路由规则；
4. 若规则启用，则生成对应 `RouteTable` 并建立绑定关系。

> AI 路由条件表达式的构造详见《AI 路由规则与条件表达式.md》（待补充）。

### 4.10 ai-cache-rule（`/configs/ai-cache-rule`）

由 `model/ai_cache/ai_cache_manager.go` 实现，是 BFE `mod_ai_cache` 模块的规则数据源：

```go
type AICacheRuleExport struct {
    Version string `json:"Version"`
    Config map[string][]*AICacheRuleConf `json:"Config"`
}
```

生成流程：

1. 查询全部 `ai_cache_rules`，按 `id` 升序（无 enabled 过滤——提交列表即生效集合）；
2. 组装 `Config[AIRouteInnerProductName] = 规则数组`（product 名取自运行时配置 `RunTime.AIRouteInnerProductName`，默认 `AI_product`，不硬编码）；
3. 空表时导出空数组（product 键仍 present，值为 `[]`）；
4. 一期每条规则仅导出 `cond` / `cacheKeyStrategy` / `cacheTTL` / `maxBodyBytes` / `maxValueBytes` 五字段，其余字段由 BFE 规则加载器 `setDefaults` 填默认。

> 详见《AI缓存规则与导出.md》。Redis 连接等静态配置不进本接口，由 BFE 静态 `mod_ai_cache.conf` 经 conf-agent `CopyFiles` 下发。

### 4.11 mod-ai-intent（`/configs/mod-ai-intent`）

由 `model/iintent_config/iintent_config_manager.go` 实现，是 BFE `mod_ai_intent` 模块的数据源：

```go
type IntentConfigDataExport struct {
    Version       string          `json:"Version"`
    MinConfidence float64         `json:"MinConfidence"`
    Questions     json.RawMessage `json:"Questions"`
}
```

生成流程：

1. `Fetch` 取 `intent_config` 当前行（全局单例）；无记录返回 nil（不导出，BFE 维持现状）；
2. 组装 `IntentConfigDataExport{Version, MinConfidence, Questions(RawMessage)}`——**Data 即 `intent_questions.data` 文件内容原样**（Version 内嵌文件，同 ai-route 端点形态；非 ai-cache 的 `Version` + `Config` 包装形态）；
3. 导出内容 version 与上次导出相同返回 nil（不触发 conf-agent 落盘与 BFE `/reload/mod_ai_intent`）；BFE 侧对"版本不变的热更"同样跳过——双端同语义。

> 详见《意图配置与导出.md》。`mod_ai_intent.conf`（决策服务地址/超时/缓存/熔断）等静态配置不进本接口，由 conf-agent `CopyFiles` 下发。

---

## 5. 增量同步机制

所有使用版本控制的导出接口都遵循统一的增量同步协议：

### 5.1 请求

```http
GET /inner-api/v1/configs/mod-api-key?version=20260101120000
Authorization: Token <token>
```

### 5.2 响应

- **配置未变化**：

  ```json
  {
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": null
  }
  ```

- **配置变化**：

  ```json
  {
    "ErrNum": 200,
    "ErrMsg": "success",
    "Data": {
      "version": "20260102120000",
      "config": {...},
      "QuotaPlans": {...},
      "tokens": {...}
    }
  }
  ```

### 5.3 下游使用建议

1. 首次拉取时不传 `version` 或传空，获取全量配置与当前版本号；
2. 后续定时拉取时携带上一次返回的 `version`；
3. 若返回 `Data == null`，跳过本次更新；
4. 若返回新配置，更新本地文件并触发 BFE 热加载。

---

## 6. 配置 Topic 设计

| Topic | 对应配置 | 是否依赖外部参数 |
|-------|---------|----------------|
| `route_rule` | TLS/Server/路由 | 否 |
| `cluster_table` | 集群实例表 | 否 |
| `gslb.<bfe_cluster>` | GSLB 调度 | 是（BFE 集群名） |
| `certificate` | TLS 证书 | 否 |
| `mod_api_key_rule` | API-Key 模块 | 否 |
| `mod_body_process` | Body 处理模块 | 否 |
| `mod_ai_rate_limit` | 限流策略模块 | 否 |
| `mod_ai_cache` | AI 缓存规则模块 | 否 |
| `intent_config` | AI 意图配置模块 | 否 |
| `ai_route` | AI 路由模块 | 否 |

> `gslb.<bfe_cluster>` 因为依赖参数，不同 BFE 集群的版本号线相互独立。

---

## 7. 边界情况与注意事项

| 场景 | 行为 |
|------|------|
| 首次导出（`config_versions` 无记录） | 生成新记录，返回全量配置 |
| 配置未变化 | 返回 `Data: nil`，不生成新记录 |
| 配置变化但版本号相同 | 不可能：候选版本取 `max(当前秒, 最新版本 + 1s)`，同秒连续变更也会逐次 +1 秒抬升（issue #142） |
| 并发导出竞争同一版本号 | 唯一约束拦截后到者，重读最新行 +1s 重试（上限 3 次）；若并发请求已为相同内容建过版本则直接返回其版本号 |
| 本地时钟回拨/慢于库内最新版本 | 版本抬升至 `最新版本 + 1s`，不产生重复或倒退版本 |
| `generator` 返回错误 | 直接返回错误，不更新 `config_versions` |
| `DataWithoutVersion.UpdateVersion` 失败 | 中断导出流程 |
| `extra_files` 不存在 | 返回 `Record Not Exist`（404） |
| GSLB 请求缺少 `bfe_cluster` | 参数校验失败，返回 422 |

---

## 8. 性能优化

1. **批量预加载 + 内存回溯**：`mod-api-key` 导出已实现一次性全量加载 `api_keys`、`entities`、`quota_plans`、`entity_types`，并在内存中沿 `parent_id` 回溯 Entity 层级，避免 N+1 查询。
2. **缓存热点配置**：`rate-limit-policy`、`ai-route` 每次导出仍全量读取所有 API-Key / Entity，数据量大时建议参考 `mod-api-key` 进行批量预加载或加缓存。
3. **异步签名计算**：复杂配置的 MD5 计算可异步化，避免阻塞请求。
4. **按产品线分片**：当前所有配置按默认产品 `AI_product` 聚合，若未来多产品线并行，可按产品线拆分 Topic。
5. **压缩传输**：InnerAPI 返回的 JSON 较大时，可启用 Gzip 压缩。

---

## 9. 相关文件索引

| 文件 | 说明 |
|------|------|
| `model/iversion_control/version_control.go` | 版本控制核心抽象 |
| `storage/rdb/version_control/version_control.go` | `config_versions` 存储实现 |
| `endpoints/innerapi_v1/endpoints.go` | InnerAPI 路由注册 |
| `endpoints/innerapi_v1/export_util/param.go` | `version` 参数解析 |
| `endpoints/innerapi_v1/server_data/export.go` | Server Data 导出端点 |
| `endpoints/innerapi_v1/gslb_data/*.go` | GSLB / Cluster Table 导出端点 |
| `endpoints/innerapi_v1/protocol/cert_export.go` | 证书导出端点 |
| `endpoints/innerapi_v1/extra_file/export.go` | 附加文件导出端点 |
| `endpoints/innerapi_v1/mod_api_key/export.go` | mod-api-key 导出端点 |
| `endpoints/innerapi_v1/mod_body_process/export.go` | mod-body-process 导出端点 |
| `endpoints/innerapi_v1/rate_limit_policy/export.go` | 限流策略导出端点 |
| `endpoints/innerapi_v1/ai_cache/export.go` | AI 缓存规则导出端点 |
| `endpoints/innerapi_v1/intent_config_export/export.go` | AI 意图配置导出端点 |
| `endpoints/innerapi_v1/ai_route/export.go` | AI 路由导出端点 |
| `model/iroute_conf/exporter.go` | Server Data 配置生成 |
| `model/icluster_conf/exporter.go` | GSLB / Cluster Table 配置生成；`AIConf.Provider` / `AIConf.ModelTable` 填充 |
| `model/icluster_conf/cluster.go` | `AIConf` 构造；将 provider `model_protocols` 透传为 `AIConf.ModelProtocols` |
| `model/imodel_price/model_price.go` | `model_prices` 查询，供 `AIConf.ModelTable` 使用 |
| `model/iprotocol/exporter.go` | 证书配置生成 |
| `model/imods/mod_api_key_rule.go` | mod-api-key 配置生成 |
| `model/imods/mod_body_process.go` | mod-body-process 配置生成 |
| `model/imods/ai_route_exporter.go` | AI 路由配置生成 |
| `model/rate_limit_policy/rate_limit_policy_manager.go` | 限流策略配置生成 |
| `model/ai_cache/ai_cache_manager.go` | AI 缓存规则配置生成（topic `mod_ai_cache`） |
| `model/iintent_config/iintent_config_manager.go` | AI 意图配置生成（topic `intent_config`） |
