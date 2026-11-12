# 变更摘要：`lib/validate.RouteRules` 对 `[null]` 元素的 panic 防护

> 问题来源：issue #327 修复过程（见 `2026-11-12-issue-327-route-rule-target-duplicate-validation/change-summary.md` §5-3）中发现的遗留缺陷——`lib/validate.RouteRules` 对 `rules`/`targets`/`fallbacks` 数组内的 JSON `null` 元素无防护，解引用 nil 指针触发 panic，经 OpenAPI 可达。

## 1. 背景与根因核实

### 1.1 问题现象

向任一接受 route rules 的 OpenAPI 写接口提交含 `null` 元素的数组，服务端 panic（被 recovery 中间件捕获后返回 **500 Internal Error**），而非契约要求的 422 `Param Illegal`：

```json
PUT /open-api/v1/global-route-rules
{"rules":[{"name":"r1","cond":"default_t()","targets":[{"cluster_name":"c1","weight":100}, null]}]}
```

### 1.2 根因（源码直读，`lib/validate/validate.go` `RouteRules`，:631-728）

| # | 位置 | 解引用 | 触发条件 |
|---|------|--------|---------|
| 1 | :641 `rule.Name` | nil rule 元素 | `"rules":[null]` |
| 2 | :663 `target.Weight` | nil target 元素 | `"targets":[null]` 或含 `null` 元素 |
| 3 | :702 `fallback.ClusterName` | nil fallback 元素 | `"fallbacks":[null]` 或含 `null` 元素 |

- JSON 绑定（`encoding/json`）把数组中的 `null` 反序列化为 nil 指针元素，绑定层与 checker 层均无前置拦截；
- 同函数内对字段级 nil（如 `target.Weight == nil`）有校验，唯独漏了**元素级 nil**；
- `endpoints/middleware/recovery.go` 的 recover 使 panic 不扩散为进程崩溃，但对调用方表现为 **500**——把客户端输入错误报告成服务端错误，违反 `00-common.md` 的 422 约定，且 500 会触发告警/监控噪音。

### 1.3 影响面

所有调用 `validate.RouteRules` 的 OpenAPI 写路径同样受影响（`api_key/checker.go:52,89`、`entity/validator.go:61`、`global_route_rules/endpoints.go:67`）。全仓 grep 确认 `lib/validate` 内仅 `RouteRules` 存在多层切片元素解引用模式；同文件其他函数（如 `RateLimitPolicy` 等）若后续核实有同型问题可一并处理，本方案以 `RouteRules` 为范围。

## 2. 修复方案

### 2.1 核心思路

在 `RouteRules` 的三个循环入口对 nil 元素**直接拒绝**（422 `Param Illegal`），而非跳过——实施期从初稿「跳过」语义修正为「拒绝」，理由（见 §5 校准记录 1）：跳过的 null 元素会流入模型层（Global 路由表路径的 `validateRouteRules` 同样无防护，panic 只是搬家）、存储层与导出配置，形成脏数据；拒绝则把非法输入挡在端点层，三个数组语义统一。

### 2.2 代码改动（`lib/validate/validate.go`，仅 `RouteRules` 一个函数）

```go
nameSet := map[string]struct{}{}
for _, rule := range p.Rules {
    if rule == nil {
        return xerror.WrapParamErrorWithMsg("route rule element cannot be null")
    }
    // ... 原有逻辑不变
}

totalWeight := 0
targetKeySet := map[string]struct{}{}
for _, target := range rule.Targets {
    if target == nil {
        return xerror.WrapParamErrorWithMsg("target element cannot be null")
    }
    // ... 原有逻辑不变
}

fallbackKeySet := map[string]struct{}{}
for _, fallback := range rule.Fallbacks {
    if fallback == nil {
        return xerror.WrapParamErrorWithMsg("fallback element cannot be null")
    }
    // ... 原有逻辑不变
}
```

要点：

1. **只加 3 处 `if x == nil { return ... }`**，其余逻辑、校验顺序一律不动，最小 diff；既有错误消息无变化；
2. 语义统一：三个数组的 `null` 元素一律 422（`route rule/target/fallback element cannot be null`），null 元素不流入模型层、存储与导出；
3. 模型层 `RouteRulesManager.validateRouteRules`（`model/route_rules/route_rules.go:340+`）**同样无元素级 nil 防护**（`:361` `target.Weight`、`:378` `fallback.ClusterName` 解引用），但端点层拒绝后实际不可达；按 issue #327 的评审结论（输入校验归端点层），本次不动，列为观察项（§4）。

### 2.3 不改动的部分

| 项 | 说明 |
|----|------|
| 错误消息与校验规则集 | 不动 |
| 绑定层 / checker / validator | 不动，nil 元素流入 `RouteRules` 后自然处理 |
| 模型层 / DAO | 不动 |

## 3. 测试计划

### 3.1 单元测试（`lib/validate/validate_test.go` 的 `TestRouteRules` 追加子用例）

| 用例 | 输入 | 断言 |
|------|------|------|
| nil rule 元素 | `Rules: [nil]`、`[合法rule, nil]` | `NotPanics` + Error（`route rule element cannot be null`） |
| nil target 元素 | targets 含 nil（部分/全部） | `NotPanics` + Error（`target element cannot be null`） |
| nil fallback 元素 | fallbacks 含 nil（部分/全部） | `NotPanics` + Error（`fallback element cannot be null`） |

关键断言方式：修复前这些输入触发 panic（集成层实测旧二进制返回 500），修复后以 `assert.NotPanics` + 具体错误断言守护。

### 3.2 集成测试（已确认实施）

`global_route_rules/update` 补 1 条用例：`"targets": [{...合法元素...}, null]` → 期望 **422**（而非 500），端到端钉死「null 元素不再 panic、按输入错误处理」。testutil 的请求体走 `map[string]interface{}` 序列化，nil 值天然可构造，无技术障碍。

## 4. 兼容性、风险与后续事项

| 项 | 结论 |
|----|------|
| 行为变化 | 500 → 422（本就该 422），属缺陷修复；无合法请求受影响（合法请求不含 null 元素） |
| 数据迁移 | 无 |
| 同型巡检 | `lib/validate` 其他多切片函数（RateLimitPolicy 等）建议按同标准巡检 nil 元素解引用，无实锤前列观察项 |
| 模型层 | 模型层 `validateRouteRules` 同样无元素级 nil 防护（端点层先行校验故不可达）；若追求对称可顺手补，但按 issue #327 的评审结论（输入校验归端点层），建议不动 |

## 5. 实施校准记录

1. **「跳过」语义在实现中被否决**：初稿设计为 nil 元素 continue（null 视为省略）。实现时发现两个漏洞：(a) Global 路由表路径在端点层跳过后，模型层 `validateRouteRules`（无防护）会对同一 null 元素再次解引用——panic 只是从端点层搬到模型层；(b) api_key/entity 路径模型层直调 storager，跳过的 null 元素会被持久化并进入导出配置，形成脏数据。遂改为三处统一 **拒绝**（422 `element cannot be null`），非法输入不越过端点层。
2. **集成测试走子进程二进制**：`testutil.StartServer` 启动 `ai-gateway-api.exe` 子进程，源码修改必须重建 exe 才生效。用旧 exe 实测 GRR-1-011 返回 500（忠实复现 panic 路径），重建后返回 422 通过——回归验证须 `go build -o ai-gateway-api.exe .` 后 `-count=1` 重跑。

## 6. 状态

- [x] 根因源码直读核实（3 处 nil 解引用点定位；recovery 中间件确认 panic→500 路径）
- [x] 修复方案设计（本文档；实施期由「跳过」修正为「拒绝」，见 §5-1）
- [x] 代码实现（`lib/validate/validate.go` 3 处 null 拒绝 guard）
- [x] 单元测试（`TestRouteRules_NilElements`：5 组用例，`NotPanics` + 错误断言）
- [x] 集成测试（GRR-1-011 `targets` 含 null 元素 → 422；旧二进制实测 500 复现、新二进制通过）
- [x] 验证：`go test ./lib/validate/...`、集成套件、全量 `go test ./...` 零失败
