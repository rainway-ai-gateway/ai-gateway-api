# 缺价动作配置与形态标注（issue #1479）：OpenAPI 接口变更说明

## 1. 变更概述

| 项目 | 说明 |
|------|------|
| 变更日期 | 2026-10-30 |
| 影响模块 | `/clusters`（`llm_config` 新增字段）；`/report/logs` 明细行（新增两列） |
| 目标文档 | `ai-gateway-api/design-docs/api-define/OpenAPI接口定义/clusters.md`、`ai-gateway-api/design-docs/api-define/OpenAPI接口定义/report.md` |
| issue | [bfenetworks/bfe#1479](https://github.com/bfenetworks/bfe/issues/1479)（v0.7.0.1 测试问题3：模型缺少某种接口的定价时该接口的请求不扣费） |
| 变更性质 | **向后兼容的新增**：可选字段（无 DDL、无数据迁移）+ 报表可选列 |

本次变更为集群 `llm_config` 引入**缺价处置动作**配置，并让报表可见"命中缺定价接口"的请求：

- 新增 `price_miss_action`（三态）：`fallback_chat`（缺省，按 chat 价回退计费）/ `bill_zero`（按 0 计费）/ `reject`（转发前拒绝）；
- 报表明细新增 `ai_price_miss`（是否缺价）与 `ai_price_warn`（缺价形态）；
- **不新增**配置期价格覆盖度校验（价格表由独立流程提供，硬校验误杀风险高，已确认不做）。

---

## 2. `/clusters`：`llm_config` 字段变更

### 2.1 字段表

| 字段 | 变更类型 | 类型 | 说明 |
|------|----------|------|------|
| `price_miss_action` | **新增** | string | 请求的 `(model, 接口类型)` 无价格条目时的处置动作；可选；缺省 = `fallback_chat` |

取值：

| 取值 | 行为 | 说明 |
|------|------|------|
| `fallback_chat` | 非 chat 接口缺价时按同模型 chat 价计费 | **缺省值**；与 bfe issue #1408 历史行为一致 |
| `bill_zero` | 缺价按 0 计费 | 回到更早版本（#1382）行为；仍打缺价信号 |
| `reject` | **转发前**拒绝：HTTP 503 + `error.code=PRICE_NOT_CONFIGURED`，不调用上游、不计费 | 需求方"未定价则拒绝"选项 |

补充说明：

- `chat` 接口自身缺价时，`fallback_chat` 下仍按 0 计费（无回退基线），并计入缺价信号；
- `reject` 下集群价格表整体缺失同样视为"未定价"并拒绝；被拒请求不进入 cluster fallback 链之外的任何计费路径；
- 控制台新建集群时**显式写入** `fallback_chat`（表单默认选中）；脚本/旧客户端可省略，行为等价。

### 2.2 请求示例（新增行已标注）

```json
{
  "llm_config": {
    "models": ["gpt-5-codex"],
    "keys": [ {"name": "key-primary", "weight": 100} ],
    "key_policy": {"strategy": "weighted_random"},
    "normalize_upstream_error": {"enabled": true},
    "price_miss_action": "reject",        // ← 新增，可选
    "provider": "deepseek"
  }
}
```

### 2.3 合法性校验

| 规则 | 错误响应 |
|------|----------|
| 取值须为 `fallback_chat` / `bill_zero` / `reject`；省略或空串合法（等价缺省） | HTTP 400：`llm_config.price_miss_action must be fallback_chat, bill_zero or reject` |

校验实现位于 `lib/validate/validate.go`（与 bfe `AIConfCheck` 枚举同源，文档以 bfe 常量为准）。

### 2.4 导出与下发（InnerAPI）

- 导出的 `cluster_conf.data` 中对应字段为 `AIConf.PriceMissAction`（PascalCase，bfe 侧命名）；API 侧 snake_case 由映射层转换；
- 未设置（`nil`）时不导出该键，bfe 走缺省 `fallback_chat`；
- 旧版 bfe 忽略未知键，**本仓可先行发布**。

---

## 3. `/report/logs`：明细行新增字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `ai_price_miss` | bool | 该请求是否发生过 `(model, 接口类型)` 缺价（含 2xx 回退/按 0、`reject` 的 503、价格表缺失）。**非 2xx 行也会出现**（bfe 侧已把写出时机提升为请求级），筛选时建议叠加 `res_status_code` |
| `ai_price_warn` | string | 缺价形态：`no_entry`（按 0）、`no_baseline`（无 chat 基线按 0）、`fallback`（回退 chat 价计费）、`subitem_missing`（回退条目缺 cache 等子项价且请求有该维度用量）、`no_model_table`（集群价格表缺失）、`reject`（转发前拒绝）。多因同时命中时取最具体；未缺价时为空 |

- 明细行 SQL/语句可按 `ai_price_warn` 分组即"回退 / 按 0 / 拒绝 / 子项缺失"四类统计；
- 字段随报表后端（MySQL / Doris / ClickHouse / StarRocks）同名同列落库；
- 存量数据无该列值时返回空（`null`），不影响既有查询。

---

## 4. 兼容性说明

| 场景 | 影响 |
|------|------|
| 存量集群（无该字段） | `nil` → 导出不带 → bfe 默认 `fallback_chat`，行为与现状完全一致；**不改写存量数据** |
| 旧 bfe（< #1479） | 忽略 `PriceMissAction` 键，行为不变 |
| 旧客户端 | 不传新字段即可；新建集群由控制台显式落值 |
| 报表旧查询 | 新增列不影响既有 SELECT；未升级 log-reader/报表 schema 时列值为空 |
| 回滚 | 移除字段消费即回到旧行为；报表列保留为空不影响写入 |

---

## 5. 文档更新清单（实施时同步）

1. `design-docs/api-define/OpenAPI接口定义/clusters.md`：LLM 配置表新增 `price_miss_action` 行 + 请求示例 + `cluster_conf.data` 导出说明；
2. `design-docs/api-define/OpenAPI接口定义/report.md`：明细行字段表新增 `ai_price_miss` / `ai_price_warn`（含"非 2xx 行也可能出现"的提示）；
3. 控制台（ai-gateway-web）：新建集群表单默认 `fallback_chat`；明细页按 `ai_price_miss` 打标、按 `ai_price_warn` 展示形态（可后置）。

## 6. 测试关注点

- create/update 透传与 `nil` 保持；导出片段断言 `"PriceMissAction"` 取值/缺省；
- 校验 400 文案；
- 报表入库两列、按 `ai_price_warn` 分组与过滤；
- bfe 侧端到端锚点：bfe 集成 SC42（`scenario-SC42-price-miss-action`，4 个 TC 覆盖三态与 `subitem_missing`）。