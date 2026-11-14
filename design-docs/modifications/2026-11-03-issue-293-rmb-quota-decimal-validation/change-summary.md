# Issue #293：RMB 配额小数位校验系统性误判（合法金额被 422 拒绝）

## 1. 问题来源

[rainway-ai-gateway/ai-gateway-api#293](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/293)

> 调用方提交 `unit=RMB` 且带小数的 `quota_plan` 时，API-Key / Entity 的创建、更新与配额重置接口统一返回
> `422 Param Illegal: quota can have at most 8 decimal places when unit is RMB`，导致 RMB 配额小数能力在控制面实际不可用。

违反 `design-docs/sys-design/模型层设计文档.md:1842`「unit = "RMB" 时 quota 小数位不超过 8 位（内部定点精度 1e-8 元）」，
也与产品自带单测 `lib/validate/validate_test.go:245` 的既有断言冲突。

## 2. 根因

`lib/validate/validate.go:438` `QuotaValue()` 在 `unit == "RMB"` 分支调用
`lib/validate/validate.go:460` `validDecimalPlaces()`：

```go
func validDecimalPlaces(v float64, maxPlaces int) bool {
	mult := math.Pow(10, float64(maxPlaces))
	scaled := v * mult
	return math.Abs(scaled-math.Round(scaled)) < 1e-9
}
```

该判据把「小数位 ≤ 8」转换成「`v × 1e8` 与最近整数之差 < 1e-9」，但 **1e-9 这个绝对阈值没有量级概念**：

| `v`（元） | `scaled = v×1e8` | float64 在该量级的 ulp | 实测偏差 | 当前判定 |
|---|---|---|---|---|
| 0.14 | 14000000 | 1.86e-9 | 1.86e-9 | 拒绝 ✗（应为通过） |
| 0.28 | 28000000 | 1.86e-9 | 3.73e-9 | 拒绝 ✗ |
| 600.1234 | 60012340000 | 7.63e-6 | > 1e-9 | 拒绝 ✗ |
| 888.88 | 88888000000 | 1.53e-5 | 0 | 通过 |
| 1234.5678 | 123456780000 | 1.53e-5 | 0 | 通过 |

也就是说：

1. 阈值 1e-9 与 float64 在 `1e7` 量级的 ulp（1.86e-9）同阶，而 `1e8` 量级的 ulp 已到 2e-8 —— 判定结果完全由「具体值 × 量级」的舍入结果决定，**任何 8 位小数以内的合法值都可能被随机误判**；
2. `quota` 接近 9000 万元上限时 `scaled ≈ 9e15`，该量级 ulp 已达 1（超过 `2^53 ≈ 9.007e15` 后为 2），
   `scaled` 恒为整数，判据对「≥ 8 位小数」完全失效；
3. 反过来，判据对「刚好 9 位小数」的拦截也只是数值巧合（`100.123456789` 恰好通过），并非契约语义。

结论：缺陷是**判据本身的数值不稳定性**，不是某个具体金额的个例。

## 3. 复现与本机偏差说明

2026-11-03 在 `ai-gateway-api` HEAD（`f20d414`）、Go 1.25.2 / windows-amd64 上实测：

- `go test ./lib/validate/ -run 'TestQuotaValue|TestQuotaPlan'` 当前 **PASS**；
- 直接调用校验函数：`0.14`、`0.28`、`0.29`、`0.55`、`600.1234` 均返回
  `PARAM: quota can have at most 8 decimal places when unit is RMB`；
- 对 `0.01 ~ 1,000,000.00` 范围内全部 1 亿个两位小数金额做穷举：**11,148,559 个（11.48%）被误拒**，
  最小样本为 `0.14`；
- issue 中列举的样本 `888.88`、`1234.5678`、`1000.12345678` 在本机通过（`v*1e8` 恰好落在整数上），
  说明 issue 给出的失败样本与环境/取值相关，**不影响根因判断**（同一函数在 `0.14` 等值上稳定复现）。

因此本方案不针对任何具体金额修补，而是替换判据本身；验收用 `0.14`、`0.28`、`600.1234` 这类「当前必失败」的值做回归锚点。

## 4. 目标

1. `unit=RMB` 时，任何「十进制表示小数位 ≤ 8 且 0 ≤ `quota` ≤ 90000000」的取值必须被接受；
2. 小数位 > 8 的取值必须返回 422，且文案指向「小数位超限」；超过 9000 万元的取值返回 422 且文案指向上限；
3. `unit=total_token` 的整数约束、`quota = nil` 放行、负数拒绝、`NaN/±Inf` 拒绝等相邻行为不回归；
4. 不改动 OpenAPI / InnerAPI 契约：字段名、类型（`quota` 仍为 `number`）、错误码（422 `Param Illegal`）均不变；
5. 补齐回归门禁：`lib/validate` 纳入 CI，并把「前置请求静默失败」的集成用例改为断言响应。

## 5. 范围

| 范围 | 说明 |
|------|------|
| 涉及仓库 | `ai-gateway-api` |
| 代码改动 | `lib/validate/validate.go`（1 个函数重写 + `QuotaValue` 增加有限值检查）；`lib/validate/validate_test.go`（新增边界用例） |
| 文档改动 | `design-docs/api-define/OpenAPI接口定义/00-common.md`（补一句「超过 8 位小数返回 422」）、本目录 |
| 测试改动 | `test/integration/tests/{api_key,entity}/**` 新增小数位边界 TC + 前置 PATCH 断言 |
| CI | `.github/workflows/model-ci.yml` 追加 `./lib/validate/...` |
| 数据库 | 无需迁移；`quota DECIMAL(18,8)` 不变 |
| 数据面 | 无影响：拒绝发生在校验层、未落库，`bfe` / `conf-agent` 收到的配置不变 |
| 兼容性 | 行为放宽（此前被误拒的合法值现在返回 200），不放宽任何非法值 |

## 6. 方案概览

把「缩放后与整数比较」改为「直接统计十进制表示的小数位」：

```go
func validDecimalPlaces(v float64, maxPlaces int) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	s := strconv.FormatFloat(v, 'f', -1, 64) // 最短往返十进制表示
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return len(s)-i-1 <= maxPlaces
	}
	return true
}
```

要点：

- `FormatFloat(v, 'f', -1, 64)` 输出「能唯一还原该 float64 的最短十进制串」，对 `json` 传入的 8 位小数金额，
  输出的就是调用方写的那个十进制数（`888.88` → `"888.88"`，`1e-8` → `"0.00000001"`），不引入二次舍入；
- 判据与数值量级无关，彻底消除 ulp 依赖；
- `NaN/±Inf` 显式拒绝，避免 `FormatFloat` 输出 `"NaN"` / `"+Inf"` 被判为「0 位小数」而放行。

详细设计、补丁、测试矩阵见 [design-changes.md](./design-changes.md)；接口行为对照见 [api-changes.md](./api-changes.md)。

## 7. 风险

| 风险 | 评估 | 缓解 |
|------|------|------|
| 放宽后 9 位小数被放过 | 无：`FormatFloat` 对 9 位小数稳定输出 9 位 | 单测 `100.123456789`、`0.000000001` 断言 422 |
| 客户端用浮点运算产生 `0.30000000000000004` 这类值被拒 | 属契约预期（非 8 位内可表示金额），行为与修复前一致 | 错误文案已指向小数位；文档标注「请直接传十进制字面量」 |
| `FormatFloat` 性能 | 单次调用纳秒级，仅在写入路径执行 | 无需处理 |
| 上下游是否还有第二处小数位校验 | 全仓仅 `lib/validate/validate.go:460` 一处（`grep -rn "decimal place"` 仅命中该错误文案） | 修复点唯一 |

## 8. 上线前检查清单

1. `go test ./lib/validate/ -run 'TestQuotaValue|TestQuotaPlan' -count=1` 通过，且新增边界断言齐全
   （`0.14`/`0.28`/`600.1234`/`888.88`/`1234.5678`/`1000.12345678`/`0.00000001`/`800`/`90000000` 通过；
   `100.123456789`/`0.000000001`/`90000000.00000001` 拒绝）；
2. `make test-model` 与 `go test ./... -count=1` 通过；
3. 集成套件 `go test ./tests/api_key/... ./tests/entity/... -count=1` 通过（SQLite 与 MySQL 各一轮）；
4. `POST /open-api/v1/api-keys` 提交 `quota=0.14`、`quota=888.88`、`quota=1000.12345678` 返回 `ErrNum=200`，
   `GET .../quota-plan` 回读 `quota` / `balance.remaining` 与写入值误差 ≤ 1e-8；
5. `quota=100.123456789` 返回 422 且文案指向小数位；`quota=90000000.00000001` 返回 422 且文案指向上限；
6. `.github/workflows/model-ci.yml` 已包含 `./lib/validate/...`，PR 上能看到该包测试结果。

## 9. 实施记录（Step 5 / Step 6）

### 9.1 代码改动

| 文件 | 改动 |
|------|------|
| `lib/validate/validate.go` | `validDecimalPlaces()` 改为按最短十进制往返表示统计小数位；`QuotaValue()` 增加 `NaN/±Inf` 拒绝；新增 `strconv` 导入 |
| `lib/validate/validate_test.go` | 新增 `TestQuotaValue_RMBDecimalPlaces`（11 个正例含 `0.14`/`0.28`/`600.1234`，5 个负例含 9 位小数、超上限、运行时 `0.1+0.2`、`NaN/±Inf`）；新增 `math` 导入 |
| `.github/workflows/model-ci.yml` | 单测范围扩为 `./model/... ./lib/validate/...` |
| `test/integration/testutil/assert.go` | 新增 `MustPatch`：前置 PATCH 必须业务成功，失败即 `t.Fatalf` |
| `test/integration/tests/api_key/{quota_update,quota_reset}`、`tests/entity/{quota_update,quota_reset}` | 31 处前置配额写入改为 `MustPatch`（消除静默失败） |
| `test/integration/tests/api_key/create/create_test.go` | 新增 `TestAPIKey_CreateRMBQuotaDecimals`：AK-1-013 ~ AK-1-017 |
| `test/integration/tests/entity/create/create_test.go` | 新增 `TestEntity_CreateRMBQuotaDecimals`：E-1-029 ~ E-1-033 |
| `test/integration/tests/api_key/quota_reset/quota_reset_test.go` | 新增 AK-8-005（8 位小数重置）、AK-8-006（9 位小数 422） |
| `test/integration/tests/entity/quota_reset/quota_reset_test.go` | 新增 E-8-005、E-8-006 |

### 9.2 文档改动

| 文件 | 改动 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/00-common.md` | `quota` 合法性条件补 422；新增「关于 `quota` 小数位的说明」 |
| `design-docs/sys-design/模型层设计文档.md` | QuotaPlan 校验要点补小数位统计口径与 `NaN/±Inf` |
| `test/integration/tests/{api_key,entity}/design.md` | 场景总览与详细设计补新增 TC（AK-1-013~017、E-1-029~033、AK-8-004~006、E-8-004~006） |

### 9.3 验证结果（2026-11-03，Go 1.25.2 / windows-amd64）

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 校验层单测 | `go test ./lib/validate/ -run 'TestQuotaValue|TestQuotaPlan' -count=1` | PASS（含新增 `TestQuotaValue_RMBDecimalPlaces`） |
| 主模块全量单测 | `go test ./... -count=1` | 100 个包 `ok`，0 个 FAIL |
| 受影响集成包（SQLite） | `AIAPI_FORCE_SQLITE=1 go test -p 1 ./tests/api_key/{create,quota_reset,quota_update}/ ./tests/entity/{create,quota_reset,quota_update}/` | 6/6 `ok` |
| api_key + entity 集成全量（SQLite） | `AIAPI_FORCE_SQLITE=1 go test -p 1 ./tests/api_key/... ./tests/entity/...` | 20/20 包 `ok` |
| 新增 TC 实际执行 | `go test -run 'RMBQuotaDecimals' -v`、`-run QuotaReset -v` | AK-1-013~017、E-1-029~033、AK-8-005/006、E-8-005/006 全部 PASS |
| 负向对照（证明用例能捕获缺陷） | 临时回退 `validDecimalPlaces` → 重建二进制 → 执行 AK-1-013 | FAIL：`expected ErrNum=200, got ErrNum=422, ErrMsg=Param Illegal: quota can have at most 8 decimal places when unit is RMB`；恢复修复后 PASS |

### 9.4 沉淀判断（Step 6）

不新增 `sys-design/details/` 文档：本次是校验判据的实现修正，不引入新机制或新流程；
判据口径已写入《模型层设计文档》QuotaPlan 校验要点与 `00-common.md`，`sys-design/summary.md` 索引无需变更。

### 9.5 遗留与后续

1. 全仓仍有约 40 处集成用例的前置写入只判 `err` 不判业务码（`grep -rn "_, err = testutil.GetClient()" test/integration/tests`），
   建议后续按 `MustPatch` 统一改造，避免同类「前置静默失败」再次掩盖缺陷；
2. `quota` 接近 9000 万元时 float64 对第 8 位小数已无有效精度，若业务需要大额高精度，
   须按 `design-changes.md §6` 方案 D 改为十进制字符串入参（另行立项）。

## 10. 参考

- [Issue #293](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/293)
- `lib/validate/validate.go:438`（`QuotaValue`）、`lib/validate/validate.go:460`（`validDecimalPlaces`）
- `lib/validate/validate_test.go:245`（`TestQuotaValue`）
- `design-docs/sys-design/模型层设计文档.md:1842`
- `design-docs/api-define/OpenAPI接口定义/00-common.md:195`
- `.github/workflows/model-ci.yml:25`
- `go-lib/quota/quota.go:39`（`RmbPrecision = 1e8`）、`go-lib/quota/quota.go:49`（`MaxRMBQuota`）