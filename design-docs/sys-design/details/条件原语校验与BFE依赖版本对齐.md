# 条件原语校验与 BFE 依赖版本对齐

## 1. 背景与适用范围

`ai-gateway-api` 在保存规则前会**真实编译** BFE 条件表达式——条件原语的词法/语法/原型校验与取值校验
全部由数据面侧的 `bfe_basic/condition` 包完成，控制面**不维护自己的原语清单**。因此"控制面支持某个条件原语"
这件事等价于"控制面的 BFE 依赖版本包含该原语"，而不是一段控制面代码。

本文说明该机制、变更时的操作口径与已知坑，供后续新增/修改条件原语时复用。

## 2. 机制：控制面为何不做原语白名单

- `lib/validate/validate.go:570-575` 的 `ConditionExpression` 直接调用 `condition.Build(cond)`；
- 该文件 `:811-816` 的函数注释明确：编译留在 BFE 侧，以**避免与原语版本耦合**（否则每新增一个原语
  都要两端同时改、且存在两份清单漂移的风险）；
- 依赖经 `go.mod` 的 `replace` 指向本 fork 的某次提交（见 §4），故控制面的实际编译能力 = 该提交携带的原语集。

## 3. 编译入口清单（全量，非测试代码）

| # | 位置 | 作用 | 触达入口 |
| --- | --- | --- | --- |
| 1 | `lib/validate/validate.go:570-575`（`ConditionExpression`） | 保存前校验条件表达式 | `RouteRules`（`:599`）、`AICacheRules`（`:729`）、`AIContextRules`（`:836`）、`TrafficMirrorRules`（`:972`） |
| 2 | `model/route_rules/route_rules.go:351`（`validateRouteRules`） | 路由表批量校验 | 路由表管理 |
| 3 | `model/iroute_conf/route_rule.go:226-229`（`ExpressionVerify`） | `/expression/verify` | OpenAPI `PATCH /expression/verify`（前端"校验表达式"） |

> AI 路由规则（`model/iai_route`）的 `Cond` **不在**上述清单内：其 `Cond` 由基本条件拼装生成，
> 保存阶段的校验覆盖情况与演进建议见 `details/路由规则管理.md`。

## 4. 依赖对齐操作口径

fork 的 go.mod 仍声明 `module github.com/bfenetworks/bfe`，与 `replace` 左侧一致，Go 允许该形态；
`require github.com/bfenetworks/bfe v1.9.0` 保持不变，由 `replace` 覆盖（v1.9.0 发布前的窗口期曾短暂移除 replace、直接 require 正式版；v1.10.0 发布前为获取 `AIConf.MaxConcurrencyPerKey` 重新指回 fork，发布后再度移除）。

```bash
cd ai-gateway-api

# 1) 把 bfe 侧目标提交解析为伪版本：v<基准tag>-0.<UTC yyyymmddhhmmss>-<12位sha>
#    基准 tag = 该提交上可达的最高 tag（本 fork 为 v1.4.1）；时间戳取提交时间（UTC）
#    当前指向：v1.10.0-dev 提交 de10ef5ea307（per-Key 并发会话限制，2026-11-15）
go mod edit -replace github.com/bfenetworks/bfe=github.com/mileszhang2016/bfe@v1.4.1-0.20261115044259-de10ef5ea307

# 2) 新推送的提交，sumdb 往往尚未收录 → 该模块改为直连 GitHub（跳过 sumdb）
GOPRIVATE=github.com/mileszhang2016/* go mod tidy
go mod tidy          # 再跑一次，确认已在无特殊环境下稳定

# 3) 本地联调可临时改用 go.mod:85 注释里的 ../bfe，勿提交
```

要点与坑：

- **sumdb 404**：刚推送的提交，`sum.golang.org`/代理 sumdb 可能报
  `reading .../sumdb/sum.golang.org/lookup/...: 404 Not Found`。`GOPRIVATE=<fork>/*` 可跳过校验；
  **go.sum 一旦写入哈希，后续开发者构建不再查 sumdb**，故该步骤只需做一次（由首次 bump 者完成并提交 go.sum）；
- `go mod tidy` 的失败容易被管道掩盖（`go mod tidy | tail` 会把 `$?` 变成 tail 的退出码）——
  判断是否成功要看 **go.sum 是否真的出现新伪版本条目**；
- 伪版本写错时 Go 会以 `invalid pseudo-version: ...` 报错并给出期望形态，可据此校正。

## 5. 一次 bump 会拖入什么：接口漂移

`replace` 指向的是 fork 的**某个提交**，从旧提交跳到新提交会带入区间内**所有**数据面改动，
可能包含与本次目的无关的接口变化。已知实例（2026-10-14 `time_weekday_in` bump）：

- `bfe_util/redis_client.Client` 新增 `HGetAll(key string) (map[string]string, error)`（新依赖 `client.go:48`，旧快照无此方法）；
- 控制面 `model/ibatch/redis_adapter_test.go` 的手写 mock `fakeRawRedisClient` 因此编译失败
  （`does not implement redis_client.Client (missing method HGetAll)`）→ 补一个空实现 stub 即可；
- **生产代码零改动**：`RedisClientAdapter.HGetAll(ctx, key)` 走 Lua 脚本（`luaAdapterHashGetAll`），
  不调用原始接口方法，故语义不变。

**结论**：bump 后必须全量 `go build ./... && go test ./...`；把"接口漂移"当作本次变更的必做配套，
并在变更说明中如实记录（不要声称"零代码改动"）。

## 6. 上线顺序与语义提醒

- 顺序：**BFE（含新原语）→ ai-gateway-api（bump 依赖）→ ai-gateway-web（面板按钮）**。
  颠倒会得到"控制面保存成功、BFE 热加载失败"的运维事件；先升 BFE 则只是"用户暂时保存不了该表达式"；
- **时区类原语的缺省语义必须写进面向用户的文案**：`time_weekday_in` 省略 `zone` 时按 UTC 判定，
  国内规则须显式写 `"H"`（UI 示例统一用两参形式）；
- 控制面**不**校验"用户是否写了 zone"——这是有意的（保持与数据面单一事实来源），靠文档与 UI 引导；
- 回滚：控制面回退依赖版本即可（回退期间该类表达式的**新增保存**会被拒，存量配置不受影响）；
  回退 BFE 镜像前须先清理引用新原语的规则配置。

## 7. 新增一个条件原语的 checklist

1. 数据面实现原语（`bfe_basic/condition/{primitive.go,build.go,parser/semant.go}`）并推送；
2. 控制面 `go mod edit -replace ...@<新提交伪版本>` + `go mod tidy`（必要时 `GOPRIVATE`）；
3. `go build ./...` 排除接口漂移，补必要的 mock 方法；
4. 在 `lib/validate/validate_test.go` 的 `TestConditionExpression` 增补该原语的合法/非法用例
   （锁定能力，防止后续依赖回退时无人察觉）；
5. `go test ./...`（等价 `make test`）与 `make test-model-cover-gate`（等价命令见 §4 所在仓库 Makefile）；
6. 更新本文（如入口清单变化）与 `modifications/<日期>-<主题>/change-summary.md`。