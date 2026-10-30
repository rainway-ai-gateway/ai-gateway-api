# 缺价动作配置与形态标注（issue #1479）：ai-gateway-api 侧设计变更说明

> 日期：2026-10-30　　issue：[bfenetworks/bfe#1479](https://github.com/bfenetworks/bfe/issues/1479)
> 问题来源：v0.7.0.1 测试发现问题3"模型缺少某种接口的定价时，该接口的请求不扣费"
> 本文件展开 `change-summary.md` 的目标，给出逐文件改动、语义决策、兼容性与测试计划。
> 配套：bfe `v1.9.0-dev` commit `9f6ddd95`（`docs/zh_cn/modifications/2026-10-30-issue-1479-price-miss-control-and-observability/design-changes.md`）；`bfe-access-pb v0.3.17`（Release 已发布）。

## 1. 设计决策

1. **字段命名**：OpenAPI/存储层用 snake_case `price_miss_action`（与 `llm_config` 现有字段 `normalize_upstream_error`/`key_policy`/`match_prefix` 一致）；导出给 bfe 的 `AIConf` 用 PascalCase `PriceMissAction`（bfe 侧现有命名），由 AIConf 映射层转换。**不**为 API 层引入双键兼容（`pool.go` 的 `instanceCompat` 属存量数据兼容，不作为新字段先例）。
2. **缺省值**：bfe 缺省为 `fallback_chat`（已确认）；**控制台新建集群时显式写入** `fallback_chat`（表单默认选中、随创建请求提交，审计可追溯、与 bfe 版本解耦）；api 层**不对 `nil` 强制填充**——脚本/旧客户端省略时行为不变，存量集群保持 `nil`、不改写、无数据迁移。三种情形对 bfe 行为完全一致。
3. **不做配置期覆盖度校验**（已确认）：价格表由独立流程提供、导入顺序不可控（先建集群后定价、整体替换价目表均合法），且 provider 无条件能力声明，任何"应该有什么价"的判定都会误杀。缺价可见性由 bfe 的 `PRICE_MISS` 日志 + 计数器（`PriceMissZeroBilled`/`PriceFallbackUsed`/`PriceRejected`/`PriceFallbackSubItemMiss`）+ pb `ai_price_miss`/`ai_price_warn` 承担；本仓只做**报表列消费**。
4. **报表字段本次就做**（已确认）：`ai_price_miss`(bool, 763) 与 `ai_price_warn`(string, 766) 进报表明细，按 `ai_price_warn` 分组即"回退 / 按 0 / 拒绝 / 子项缺失"四类统计；控制台 UI 展示可后置。

## 2. 逐文件改动

### 2.1 `model/icluster_conf/cluster.go`：配置结构

`LLMConfig`（`:243`）新增字段（指针语义：`nil` = 未设置）：

```go
// PriceMissAction controls bfe's behavior when the (model, mode) price
// entry is absent: "fallback_chat" (bill at the model's chat-mode price,
// default when unset), "bill_zero" (bill 0) or "reject" (reject before
// forwarding). Nil keeps bfe's default (fallback_chat).
PriceMissAction *string `json:"price_miss_action"`
```

导出转换（`NormalizeUpstreamError` 映射块 `:1633` 旁）：

```go
aiConf.PriceMissAction = derefString(llmConfig.PriceMissAction, "")
```

空串在 bfe 侧 `AIConfCheck` 归一为 `fallback_chat`；导出保留"未设置"与"显式设置"的区别由 `nil → ""` 表达，bfe 行为一致。

### 2.2 `lib/validate/validate.go`：枚举校验

在 `normalize_upstream_error` 校验块（`:1148-1160`）附近新增：

```go
if c.PriceMissAction != nil {
    switch *c.PriceMissAction {
    case "", "fallback_chat", "bill_zero", "reject":
    default:
        // 与 bfe AIConfCheck 枚举保持同源
        return error: llm_config.price_miss_action must be fallback_chat, bill_zero or reject
    }
}
```

- 空串合法（等价未设置）；
- 非三值之一 → 400；
- 取值与 bfe `cluster_conf.PriceMissAction*` 常量同源，文档中注明"以 bfe 常量为准"，避免两边漂移。

### 2.3 `endpoints/openapi_v1/product_cluster/`：OpenAPI 入口

- `create.go`：`normalizeLLMConfig`（`:436`）增加字段透传；
- `update.go`：确认 update 路径复用同一 normalize/校验（若不复用，按 create 同步）；
- `list`/`one` 原样回显（结构体已有，无需额外处理）；
- 控制台新建集群显式提交 `fallback_chat`（web 侧改动，见 §1.2；api 不强制填充）。

### 2.4 报表链路（跨仓，本次就做）

已核验 `ai_price_miss` 目前**只存在于 pb 访问日志**：本仓报表 DDL（`db_ddl_report_mysql.sql`）、`ai-gateway-observability` 的 Doris/StarRocks/ClickHouse schema、`log-reader` 映射中均无该字段（grep 零命中）。链路改动：

1. **log-reader**：`field_mapper` 增加 `ai_price_miss`（bool）与 `ai_price_warn`（string）映射 → Kafka（顺带评估同批补 `ai_usage_source`/`ai_zero_cost_reason`，同区域字段一次改完更省事）；
2. **本仓**：`db_ddl_report_mysql.sql` 明细表加两列（含存量迁移 ALTER）+ `storage/mysqlreport/job.go` 入库 + 明细查询新增筛选/聚合（按 `ai_price_warn` 分组即四类统计）；
3. **ai-gateway-observability**：Doris / StarRocks / ClickHouse 三套明细 schema 加列（含升级脚本与回灌说明）+ 必要时分钟聚合；
4. **ai-gateway-web（可后置）**：请求明细对 `ai_price_miss=true` 打标并按 `ai_price_warn` 展示形态；"资源管理 → 模型定价"页可基于报表查询给出缺价提示。

每步独立可回滚：字段 optional，未映射/未加列时数据自然缺席。

## 3. 配置影响

| 项 | 说明 |
|----|------|
| OpenAPI | 集群 `llm_config.price_miss_action`（可选，枚举三值） |
| 导出 `cluster_conf.data` | `AIConf.PriceMissAction`（PascalCase）；空串 = 未设置 |
| 存储 | cluster 记录 JSON 直接承载字段，**无 DDL**（如 MySQL JSON 列校验存在，确认不限制新增键） |
| 报表 | 明细表新增 `ai_price_miss`/`ai_price_warn` 两列（DDL + 迁移） |
| 控制台 | 新建集群默认显式 `fallback_chat`；存量集群保持 `nil` 不改写 |

## 4. 兼容性

- **旧数据**：存量 cluster 无该字段 → `nil` → 导出不带 → bfe 走默认 `fallback_chat`，行为与现状一致；
- **旧 bfe**：不认识 `PriceMissAction` 会被忽略（AIConf JSON 反序列化忽略未知键），**本仓可先行发布**；
- **导出回放**（集群导入/导出、conf-agent 同步）：字段随 `cluster_conf.data` 往返，无 DDL；
- **报表列**：新增可选列，旧查询不受影响；未升级的 log-reader 不产出该列时，报表列为空。

## 5. 测试计划

### 单元测试

1. `lib/validate`：三值合法/非法/空值；错误消息断言；
2. `product_cluster`：create/update 字段透传、`nil` 保持 `nil`、导出 `cluster_conf.data` 中 `"PriceMissAction"` 的取值/缺省语义；
3. `model_price`：既有结构性校验（重复、枚举、数值范围）回归；**不做**覆盖度 lint（已确认）；
4. 报表：入库时两列写入、查询按 `ai_price_warn` 分组过滤。

### 集成测试

5. OpenAPI 集成：创建 cluster（`reject`）→ InnerAPI 导出 → 断言导出片段；更新为 `bill_zero` → 再次导出断言；
6. 回归：SC11（InnerAPI 导出）、SC13/SC14（token/配额继承，涉及 cluster 配置往返）、SC19（模型定价链路，integration-test 仓）；
7. bfe 侧验收锚点：bfe SC42（`scenario-SC42-price-miss-action`，4 TC）覆盖三态与 subitem 形态；SC03 覆盖 #1408 回退信号。

## 6. 发布顺序与回滚

1. `bfe-access-pb v0.3.17`（已发布）→ 2. `bfe`（已发布，commit `9f6ddd95`）→ 3. **本仓**（字段 + 校验 + 导出，旧 bfe 安全）→ 4. `log-reader`（字段映射）→ 5. `ai-gateway-observability`（三套 schema）→ 6. `web`（展示，可后置）。

回滚：本仓字段为可选，移除消费即回到旧行为；报表列保留不影响写入（字段为空）。
