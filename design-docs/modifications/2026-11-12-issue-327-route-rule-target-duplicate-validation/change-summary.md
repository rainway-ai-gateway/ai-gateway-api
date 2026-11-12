# Issue #327 变更摘要：RouteRule 唯一性约束核实——非实现缺口，文档补口径 + 集成测试补钉

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#327](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/327)
>
> **核实结论：issue 描述的现象（「通过 OpenAPI 提交包含重复 (cluster_name, model) 的 targets 会创建成功」）在当前代码上不可复现。** OpenAPI 端点层自 2026-07-30（commit b8a8a93）起即强制全部 route rules 校验，含 targets 判重与 fallbacks 内部判重。本次变更**不含任何产品代码修改**（模型层修复方案曾实现、评审后回退，见 §5 校准记录），仅交付：契约文档补口径 + 3 条集成测试用例。

## 1. 核实过程与结论

### 1.1 issue 的指控

`00-common.md §6` 声明「同一 `targets` 数组内 `(cluster_name, model)` 组合不能重复」，issue 认为控制面实现未强制，并给出 `model/route_rules/route_rules.go` 的 `validateRouteRules` 代码位置作为证据。

### 1.2 双层核实结论

| # | 层 | 位置 | 事实 |
|---|----|------|------|
| 1 | 模型层 | `model/route_rules/route_rules.go` `validateRouteRules`（:333-378） | 确实**无** `(cluster_name, model)` 判重——issue 引用的代码证据属实，但该函数只是模型层基础校验 |
| 2 | 模型层 | `model/api_key/api_key.go:610,615,732`、`model/entity/entity_manager.go:143,372,377` | api-keys/entities 的模型层写路径**直调 storager**，不经过 `validateRouteRules` |
| 3 | **端点层** | `lib/validate/validate.go:631` `validate.RouteRules` | **自 2026-07-30（b8a8a93）起含完整校验**：规则名唯一、cond 合法性、targets 非空、单项权重 [0,100]、权重和 =100、ClusterName 必填与格式、Model 格式、**targets 内 (cluster_name, model) 判重（:686-690）**、**fallbacks 内 (cluster_name, model) 判重（:711-715）** |
| 4 | 端点层接入面 | `endpoints/openapi_v1/api_key/checker.go:52,89`、`endpoints/openapi_v1/entity/validator.go:61`、`endpoints/openapi_v1/global_route_rules/endpoints.go:67` | api-keys 创建/更新、entities 创建/更新、Global 路由表更新的**全部 OpenAPI 写路径均先经 `validate.RouteRules`**，非法即 422，请求不会到达模型层 |
| 5 | 旁证 | 既有集成用例 `GRR-1-008`（重复 target → 422）、`AK-1-010`（规则名重复 → 422） | 该行为一直在被测试守护 |
| 6 | 其他写路径 | inner API 不创建 api-key/entity 的路由规则；DAO 无校验 | 不存在绕过端点层的实际写路径 |

**结论**：issue 只核对了模型层函数（#1），未核查端点层校验（#3/#4），现象描述过时。OpenAPI 调用方视角下，契约与实际行为**一致**，无 bug。

### 1.3 本次仍交付的两项

1. **文档补口径**：`00-common.md §6` 此前未声明 fallbacks 的唯一性口径，而「fallbacks 内部不允许重复、targets↔fallbacks 跨组允许」自 2026-07-30 起已是既成行为。把现实行为写成契约条款（issue 明确要求「在文档中写明」）。
2. **集成测试补钉**：既有用例只覆盖部分场景，补 3 条契约守护用例（§3）。

## 2. 变更内容

### 2.1 文档（保留）

| 文档 | 改动 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/00-common.md §6` | `targets` 跨元素约束补判重口径（违反 422、`model` 按最终值比较、空串与未填写等价）；新增 `fallbacks` 跨元素约束与口径（内部不允许重复、与 targets 跨组允许） |
| `design-docs/sys-design/details/路由规则管理.md §5` | 重写为双层校验描述：端点层 `lib/validate.RouteRules` 完整规则清单（9 条）+ 模型层 `validateRouteRules` 基础规则清单；修正该节两处与实现脱节的旧描述 |
| `design-docs/sys-design/模型层设计文档.md`（`RouteRulesManager` 一节） | 同步双层校验描述 |

### 2.2 集成测试（保留）

| 套件 | 用例 | 覆盖 |
|------|------|------|
| `test/integration/tests/global_route_rules/update` | **GRR-1-010**（新增） | 重复 fallback → 422 |
| `test/integration/tests/api_key/create` | **AK-1-013**（新增） | route_rules 重复 target → 422 |
| `test/integration/tests/entity/full_update` | **E-4-012**（新增） | 携带重复 route rule target 全量更新 → 422 |

既有 GRR-1-008（重复 target）、AK-1-010（规则名重复）保持不变。

### 2.3 产品代码（无变更）

`model/`、`lib/`、`endpoints/`、`storage/` 均无任何改动。评审中曾实现并验证过的模型层修复（新子包 `route_rules/validation` + 4 处调用点收口 + 单测）**已整体回退**，理由见 §5 校准记录 1。

## 3. 验证

- 集成测试：三个相关套件（global_route_rules/update、api_key/create、entity/full_update）串行实测全过（新增用例与既有用例均通过）。注意同机并行跑多个集成套件会因共享 `test/integration/data/access.log` 的 Windows 文件锁失败，须串行执行——环境限制，与代码无关。
- 全量 `go test ./...` 零失败；model 覆盖率 85%+ 过 70% 门禁（回退后基线）。

## 4. 风险与兼容性

零风险：无产品代码变更，OpenAPI 行为修复前后完全一致；集成测试只增不改既有断言。

## 5. 实施校准记录（方案演进与回退理由）

1. **模型层修复方案已实现并验证，评审后回退**：初稿按 issue 字面结论在模型层补判重（`route_rules/validation` 子包、api_key/entity 4 处调用点、18 用例单测，全部实测通过）。随后在检查 `test/` 时反向发现端点层 `lib/validate.RouteRules` 自 2026-07-30 起已含全部判重（issue 现象不可复现），模型层改动对 OpenAPI 行为零影响，却引入与端点层的**双实现并存**（规则集、错误消息不一致的维护代价是现实成本，而「inner 写路径绕过」是它防御的、今天不存在的风险）。评审决定回退，与仓库既有模式一致（输入校验归端点层，如单项权重 [0,100]、ClusterName 格式校验本就只在端点层）。
2. **fallbacks 口径反转**：初稿裁定「fallbacks 内部允许重复、数据面去重兜底」，评审改为「内部不允许重复」——重复 fallback 在降级链按序尝试语义下无价值、浪费降级预算；且端点层实现自 2026-07-30 起本就拒绝（`lib/validate:711-715`），文档只是追认现实。`targets`↔`fallbacks` 跨组保持允许（fallback 指向主 target 属合理降级配置）。
3. **nil 元素 panic 观察项（未修）**：`lib/validate.RouteRules` 对 `targets`/`fallbacks` 数组内的 `[null]` 元素无防护（`target.Weight` 解引用 panic，经 OpenAPI 可达，表现为 500）。曾把 nil guard 加在模型层，随回退一并撤销——**正确的修复位置是 `lib/validate.RouteRules`**，建议另立 issue 处理，不混入本次。
4. **根因排查教训**：issue 给出精确的模型层代码位置，容易诱导「证据确凿、直接修模型层」的路径；本次先实现后发现的反转说明，**涉及「校验缺失」类 issue 时必须沿调用链核查每一层**（端点 checker/validator → 模型 manager → DAO），并以「端到端是否可复现」为准判定 bug 是否存在。

## 6. 状态

- [x] issue 指控核实（双层直读 + 引入版本定位 + 既有集成用例旁证：现象不可复现，非 bug）
- [x] 契约文档补口径（00-common.md §6；sys-design 两处同步并修正脱节描述）
- [x] 集成测试补钉（GRR-1-010 / AK-1-013 / E-4-012，三套件实测全过）
- [x] 模型层修复方案回退（代码零变更确认）
- [ ] 关闭 issue #327（建议注明：经核实 OpenAPI 层自 2026-07-30 起已强制判重，非实现缺口；本次已补文档口径与集成用例）
- [ ] 另立 issue：`lib/validate.RouteRules` 的 `[null]` 元素 panic（§5-3）
