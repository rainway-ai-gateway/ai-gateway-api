# 接口变更说明：#293 RMB 配额小数位校验修复

## 1. 结论：无接口契约变更

本次修复不新增、不删除、不重命名任何接口或字段，`quota_plan.quota` 仍为 `number`，
错误码仍为 `422 Param Illegal`。变更内容只是让实现回到 `design-docs/api-define/` 与
`design-docs/sys-design/模型层设计文档.md:1842` 已定义的契约上：

> `unit = "RMB"` 时 `quota` 非负、小数位 ≤ 8、上限 9000 万元。

## 2. 受影响接口

| 方法 | 路径 | 校验入口 | 修复前 | 修复后 |
|---|---|---|---|---|
| POST | `/open-api/v1/api-keys` | `checkCreateAPIKey` → `validate.QuotaPlan` | 合法小数金额可能 422 | 200 |
| PUT/PATCH | `/open-api/v1/api-keys/{id}` | `checkUpdateAPIKey` → 同上 | 同上 | 200 |
| POST | `/open-api/v1/api-keys/{id}/quota-plan/reset` | `reset_quota.go:94` → `validate.QuotaValue` | 同上 | 200 |
| POST | `/open-api/v1/entities` | `checkCreateEntity` → 同上 | 同上 | 200 |
| PUT/PATCH | `/open-api/v1/entities/{id}` | `checkUpdateEntity` → 同上 | 同上 | 200 |
| POST | `/open-api/v1/entities/{id}/quota-plan/reset` | `reset_quota.go:104` → 同上 | 同上 | 200 |

只读取配额、不写入的 `GET .../quota-plan`、`POST .../quota-plan/reset` 的回读语义不变。

## 3. 行为对照（`unit=RMB`）

| 请求取值 | 修复前 | 修复后 | 说明 |
|---|---|---|---|
| `0`、`800`、`90000000` | 200 | 200 | 不变 |
| `888.88`、`1234.5678`、`1000.12345678` | 200（部分环境下 422） | 200 | 环境相关的误拒被消除 |
| `0.14`、`0.28`、`0.55`、`600.1234` | **422** | **200** | 本缺陷的主要受害值（合法金额） |
| `100.123456789`、`0.000000001` | 422 | 422 | 文案不变：`quota can have at most 8 decimal places when unit is RMB` |
| `90000000.00000001`、`90000001` | 422 | 422 | 文案不变：`quota must be <= 9e+07 when unit is RMB` |
| `0.30000000000000004`（浮点运算产物） | 422 | 422 | 契约预期：客户端应直接传十进制字面量 |
| `-1` | 422 | 422 | `quota must be >= 0` |
| `NaN` / `±Inf` | 422 | 422 | 文案由「不是整数/小数位超限」统一为 `quota must be a finite number`（JSON 无法传输这些值，仅防御性变更） |
| `quota` 缺省 / `null` | 放行 | 放行 | 不变 |
| `unit=total_token` 带小数 | 422 | 422 | 不变 |

## 4. 错误响应兼容性

- 错误码：仍为 `422`（`xerror.WrapParamError`）。
- 文案：小数位与上限两类文案保持原样，前端/脚本里基于文案的匹配不受影响。
- 唯一文案变化：非有限值（`NaN`/`±Inf`）从「must be an integer / at most 8 decimal places」改为
  `quota must be a finite number`。JSON 数字无法表达非有限值，正常调用路径不可达，不构成兼容性风险。

## 5. 文档同步

| 位置 | 动作 |
|---|---|
| `design-docs/api-define/OpenAPI接口定义/00-common.md:195` | 在 `quota` 说明后补一句「超过 8 位小数、超过 9000 万元或负数返回 422」 |
| `design-docs/sys-design/模型层设计文档.md:1842` | 无需改动（条款已是正确语义），可在 QuotaPlan 校验要点补「小数位按十进制表示统计」一句 |
| `design-docs/modifications/2026-11-03-issue-293-rmb-quota-decimal-validation/` | 本目录（change-summary / design-changes / api-changes） |

## 6. 调用方注意事项

1. 请直接提交十进制字面量（如 `0.14`、`1234.5678`），不要提交 `0.1+0.2` 之类浮点运算结果；
2. 超过 8 位小数的金额不会被静默四舍五入，而是返回 422 —— 需要更高精度请先自行按业务规则取整；
3. 金额接近 9000 万元时 float64 的有效精度约 `1.5e-8` 元，若业务要求大额 + 8 位小数，
   需按 `design-changes.md §6` 的方案 D 改为十进制字符串入参（另行立项）。