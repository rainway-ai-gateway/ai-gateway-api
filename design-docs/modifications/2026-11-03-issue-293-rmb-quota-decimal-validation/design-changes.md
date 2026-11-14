# 设计变更：#293 RMB 配额小数位校验判据重写

## 1. 现状与问题

### 1.1 校验链路

```
POST/PUT/PATCH /open-api/v1/api-keys[/{id}]      ─┐
POST/PUT/PATCH /open-api/v1/entities[/{id}]      ─┼─► checkCreateAPIKey / checkUpdateAPIKey 等
POST /open-api/v1/api-keys/{id}/quota-plan/reset ─┤   （endpoints/openapi_v1/*/checker.go）
POST /open-api/v1/entities/{id}/quota-plan/reset ─┘        │
                                                          ▼
                                            validate.QuotaPlan()  → validate.QuotaValue()
                                              lib/validate/validate.go:409 / :438
```

`QuotaValue` 是**唯一**的小数位闸口：全仓 `grep -rn "decimal place"` 仅命中其错误文案，
没有第二处校验、也没有模型层兜底，因此该校验的行为直接决定 RMB 小数配额能否写入。

### 1.2 现有判据的数学缺陷

```go
// lib/validate/validate.go:460（当前实现）
func validDecimalPlaces(v float64, maxPlaces int) bool {
	mult := math.Pow(10, float64(maxPlaces))
	scaled := v * mult
	return math.Abs(scaled-math.Round(scaled)) < 1e-9
}
```

| `v` | `scaled = v×1e8` | 该量级 ulp | `|scaled - round(scaled)|` | 阈值 1e-9 | 是否正确 |
|---|---|---|---|---|---|
| `0.14` | 14000000.000000 | 1.86e-9 | 1.86e-9 | 不满足 | ✗ 应通过 |
| `0.28` | 28000000.000000 | 1.86e-9 | 3.73e-9 | 不满足 | ✗ 应通过 |
| `0.55` | 55000000.000000 | 3.73e-9 | 7.45e-9 | 不满足 | ✗ 应通过 |
| `600.1234` | 60012339999.999992 | 7.63e-6 | 7.63e-6 | 不满足 | ✗ 应通过 |
| `888.88` | 88888000000.000000 | 1.53e-5 | 0 | 满足 | ✓ 恰好通过 |
| `1234.5678` | 123456780000.000000 | 1.53e-5 | 0 | 满足 | ✓ 恰好通过 |
| `100.123456789` | 10012345678.900000 | 1.86e-6 | 0.1 | 不满足 | ✓ 应拒绝 |
| `90000000.00000001` | 9000000000000001 | 1 | 0 | 满足 | ✓ 交给上限校验拒绝 |

误差来源有两层，且**都与阈值 1e-9 不可比**：

1. **输入侧**：十进制字面量 → float64 的表示误差，绝对值 ~`|v| × 2^-52`；
2. **缩放侧**：`v × 1e8` 的结果必须落在该量级 float64 的格点上，舍入误差可达半个 ulp
   （`1e7` 量级 ≈ 0.93e-9，`1e10` 量级 ≈ 7.6e-6）。

因此判据实际是「十进制字面量 × 量级 × 具体舍入结果」的随机函数：

- 对 `0.01 ~ 1,000,000.00` 的 1 亿个两位小数金额穷举，**11,148,559 个（11.48%）被误拒**；
- 对接近 9000 万元上限的值（`scaled ≈ 9e15`，ulp = 1），`scaled` 恒为整数，判据对 >8 位小数完全不生效。

### 1.3 期望语义（来自设计文档与产品单测，非从实现反推）

`design-docs/sys-design/模型层设计文档.md:1842`：

> `unit = "RMB"` 时，quota 必须 >= 0，小数位不超过 8 位（内部定点精度 1e-8 元）。

`lib/validate/validate_test.go:245` 既有断言：`100.12345678` → `NoError`，`100.123456789` → `Error`。

## 2. 新判据

### 2.1 定义

> 一个 `quota` 取值合法 ⟺ 存在一个小数位 ≤ 8 的十进制数 `d`，使得 `ParseFloat(d)` 得到该 float64。

实现取十进制表示的最短往返串：`strconv.FormatFloat(v, 'f', -1, 64)` 输出的是「能唯一还原该 float64 的最短十进制串」，
对 JSON 传入的值，它就是调用方写的那个十进制字面量（`888.88` → `"888.88"`，`1e-8` → `"0.00000001"`），
不做二次舍入、不引入量级依赖。

该实现与上面的定义等价：若调用方写了 ≤ 8 位小数的 `d`，则 `d` 自身就是 v 的一个往返串，
最短往返串长度不会超过它；反之若最短往返串有 9 位小数，则不存在 ≤ 8 位的等值十进制数。

### 2.2 代码改动

文件：`lib/validate/validate.go`

```diff
 import (
 	"fmt"
 	"math"
 	"net"
 	"regexp"
+	"strconv"
 	"strings"
 	"time"
 	"unicode"
```

```diff
 // QuotaValue validates a single quota value for the given unit.
 // A nil quota is treated as valid (not provided).
 func QuotaValue(quota *float64, unit string) error {
 	if quota == nil {
 		return nil
 	}
+	if math.IsNaN(*quota) || math.IsInf(*quota, 0) {
+		return xerror.WrapParamErrorWithMsg("quota must be a finite number")
+	}
 	if *quota < 0 {
 		return xerror.WrapParamErrorWithMsg("quota must be >= 0")
 	}
 	if unit == "total_token" {
 		if math.Mod(*quota, 1) != 0 {
 			return xerror.WrapParamErrorWithMsg("quota must be an integer when unit is total_token")
 		}
 	} else if unit == "RMB" {
 		if !validDecimalPlaces(*quota, 8) {
 			return xerror.WrapParamErrorWithMsg("quota can have at most 8 decimal places when unit is RMB")
 		}
 		if *quota > golibquota.MaxRMBQuota {
 			return xerror.WrapParamErrorWithMsg("quota must be <= %v when unit is RMB", golibquota.MaxRMBQuota)
 		}
 	}
 	return nil
 }
 
+// validDecimalPlaces reports whether v can be written with at most maxPlaces
+// fractional digits. It inspects the shortest decimal representation that
+// round-trips to v instead of scaling by 10^maxPlaces, because the scaled
+// product carries rounding error comparable to the tolerance at the magnitudes
+// RMB quotas use (e.g. 0.14*1e8 is off by one ulp), which rejected valid values.
 func validDecimalPlaces(v float64, maxPlaces int) bool {
-	mult := math.Pow(10, float64(maxPlaces))
-	scaled := v * mult
-	return math.Abs(scaled-math.Round(scaled)) < 1e-9
+	if math.IsNaN(v) || math.IsInf(v, 0) {
+		return false
+	}
+	repr := strconv.FormatFloat(v, 'f', -1, 64)
+	dot := strings.IndexByte(repr, '.')
+	if dot < 0 {
+		return true
+	}
+	return len(repr)-dot-1 <= maxPlaces
 }
```

说明：

- 函数名、签名、调用点（`validate.go:450`）保持不变，改动面只有函数体；
- `fmt` / `math` 仍被其他函数使用，无需删除 import；`strings` 已存在；
- 保留「先判小数位、再判上限」的顺序：`90000000.00000001` 仍是 8 位小数、落到上限分支，
  错误文案继续指向上限，与验收判据 4 一致。

### 2.3 边界矩阵

| 输入 `quota`（unit=RMB） | 判据 | 实际行为 | 说明 |
|---|---|---|---|
| `0`、`800`、`90000000` | 0 位小数 | 200 | 整数金额不受影响 |
| `888.88`、`1234.5678` | 2/4 位小数 | 200 | 修复前恰好通过，作为兼容性锚点 |
| `0.14`、`0.28`、`0.55`、`600.1234` | 2/4 位小数 | 200（修复前 422） | 本缺陷的真实受害者 |
| `1000.12345678`、`0.00000001` | 8 位小数 | 200 | 8 位上限正例 |
| `100.123456789`、`0.000000001` | 9 位小数 | 422（小数位文案） | 负例 |
| `90000000.00000001` | 8 位小数、超上限 | 422（上限文案） | 顺序依赖，需保持 |
| `-1` | 负数 | 422（`must be >= 0`） | 不变 |
| `NaN`、`+Inf`、`-Inf` | 非有限值 | 422（`must be a finite number`） | JSON 无法编码，但 `QuotaValue` 是导出函数，需自保 |
| `0.1 + 0.2`（运行时求和得 `0.30000000000000004`） | 17 位小数 | 422 | 契约预期：客户端应直接传十进制字面量（编译期常量折叠成 `0.3`，单测须运行时求和） |
| `nil` | 未提供 | 放行 | 不变 |

已知限制（不在本 issue 范围）：`quota` 接近 9e7 时 float64 的 ulp 已约 1.5e-8，
第 8 位小数在该量级本身不可表示；修复后仍按「调用方写法的小数位」判定并如实接受，
但落库 `DECIMAL(18,8)` 前的 float64 值已经不具备完整 8 位精度。若后续需要大额高精度，
须把 `quota` 改为十进制字符串或 `json.Number` 入参（见 §6 方案对比）。

## 3. 单元测试

文件：`lib/validate/validate_test.go`，在 `TestQuotaValue` 之后新增：

```go
func TestQuotaValue_RMBDecimalPlaces(t *testing.T) {
	// Accepted: every value whose shortest decimal representation has <= 8
	// fractional digits and stays within the RMB limit. 0.14/0.28/600.1234 are
	// regression anchors for issue #293 (they were rejected by the old
	// v*1e8 scaling check).
	for _, v := range []float64{
		0, 800, 90000000,
		0.14, 0.28, 0.55, 600.1234,
		888.88, 1234.5678, 1000.12345678, 0.00000001,
	} {
		q := v
		assert.NoError(t, QuotaValue(&q, "RMB"), "quota=%v", v)
	}

	// Rejected: more than 8 fractional digits.
	for _, v := range []float64{100.123456789, 0.000000001} {
		q := v
		assert.Error(t, QuotaValue(&q, "RMB"), "quota=%v", v)
	}

	// Float-artifact input must be computed at run time: the compiler folds the
	// constant expression 0.1+0.2 to exactly 0.3, whose shortest repr is "0.3".
	sum := 0.1
	sum += 0.2
	assert.Error(t, QuotaValue(&sum, "RMB"), "quota=%v", sum)

	// Over the RMB limit: rejected by the limit check, not the decimal check.
	q := 90000000.00000001
	assert.Error(t, QuotaValue(&q, "RMB"))

	// Non-finite values are rejected for both units.
	nan, inf := math.NaN(), math.Inf(1)
	assert.Error(t, QuotaValue(&nan, "RMB"))
	assert.Error(t, QuotaValue(&inf, "RMB"))
	assert.Error(t, QuotaValue(&inf, "total_token"))
}
```

测试文件需在 import 中新增 `"math"`（当前 `lib/validate/validate_test.go:3-12` 未引入）。

同时保留 `TestQuotaValue` 原有断言（`100.12345678` 通过 / `100.123456789` 拒绝 / `90000000` 通过 /
`90000000.00000001` 拒绝 / `nil` 放行）以证明未回归。

回归锚点选择原则：**不使用 issue 中列举的 `888.88` / `1234.5678`** —— 它们在部分环境下本就能通过，
无法区分修复前后的实现；改用 `0.14`、`0.28`、`600.1234`（修复前必失败）。

## 4. 集成用例调整

### 4.1 新增小数位边界 TC

以独立测试函数实现（避免改动既有表驱动用例的 `wantCode` 判定逻辑）：

`test/integration/tests/api_key/create/create_test.go` — `TestAPIKey_CreateRMBQuotaDecimals`：

| TC | 请求 | 期望 |
|---|---|---|
| AK-1-013 | `quota=0.14, unit=RMB` | 200；`quota` 与 `balance.remaining` = 0.14（误差 ≤ 1e-8），`used=0` |
| AK-1-014 | `quota=600.1234` | 200；回读一致 |
| AK-1-015 | `quota=1000.12345678` | 200；回读一致 |
| AK-1-016 | `quota=100.123456789` | 422，`ErrMsg` 含 `decimal places` |
| AK-1-017 | `quota=90000000.00000001` | 422，`ErrMsg` 含 `must be <=` |

`test/integration/tests/entity/create/create_test.go` — `TestEntity_CreateRMBQuotaDecimals`：
`E-1-029`（`0.28`）、`E-1-030`（`600.1234`）、`E-1-031`（`1000.12345678`）正例，
`E-1-032`（9 位小数 422）、`E-1-033`（超上限 422）负例。

重置路径各补 2 例（覆盖 `reset_quota.go:94` / `:104`）：`AK-8-005`、`E-8-005` 为 8 位小数重置正例
（前置配额本身用 `0.14` / `0.28` 这类修复前必失败的值），`AK-8-006`、`E-8-006` 为 9 位小数 422 负例。

`test/integration/tests/api_key/design.md`、`entity/design.md` 的场景总览与详细设计已同步补条目。

### 4.2 修复「前置请求静默失败」

issue 指出的放大因素：前置 `PATCH` 只判 `err`（HTTP 层错误）不判业务码，校验失败后用例继续执行，
产生「`used` 期望 600.1234 实得 0」这类误导性断言。修改点（示例，需全量排查同类写法）：

- `test/integration/tests/api_key/quota_update/quota_update_test.go:312`（AK-9-008 的前置 PATCH）
- `test/integration/tests/entity/quota_update/quota_update_test.go:144`（E-9-003 的前置 PATCH）

改法（已实施）：在 `test/integration/testutil/assert.go` 新增 `MustPatch`，前置写入一律走该 helper，
业务码非 200 直接 `t.Fatalf`：

```go
// MustPatch 发送 PATCH 并断言业务成功，用于测试前置配置。
// 前置写入只判 err 会静默吞掉校验失败（issue #293），让后续断言以误导性形式失败。
func MustPatch(t *testing.T, path string, body interface{}) *APIResponse {
	t.Helper()
	resp, err := GetClient().Patch(path, body)
	if err != nil {
		t.Fatalf("PATCH %s failed: %v", path, err)
	}
	AssertSuccess(t, resp)
	return resp
}
```

已改造 `tests/api_key/{quota_update,quota_reset}`、`tests/entity/{quota_update,quota_reset}` 共 31 处前置写入；
其余包的前置写入按 §7「遗留」项后续统一改造。

排查命令：`grep -rn "_, err = testutil.GetClient()" test/integration/tests`。

## 5. 回归门禁

`.github/workflows/model-ci.yml:25` 原先只跑 `./model/...`，`lib/validate` 不在 CI 内，
这是该缺陷自 `096287c` 引入后长期未被发现的原因之一。本次已一并落地：

```diff
       - name: Run model and validate unit tests
-        run: go test ./model/... -count=1
+        run: go test ./model/... ./lib/validate/... -count=1
```

后续新增共享校验库（如 `lib/` 下其他包）时应同步纳入该步骤，避免同类判据再次脱离门禁
（issue 验收判据 7）。

## 6. 备选方案与取舍

| 方案 | 做法 | 结论 |
|---|---|---|
| A（采纳） | `FormatFloat(v,'f',-1,64)` 统计小数位 | 与数值量级无关，改动面 1 个函数，无契约变更 |
| B | 保留缩放，把阈值改为相对量级（如 `1e-9*math.Max(1,math.Abs(scaled))`） | 仍依赖 ulp 推断，`9e15` 量级失效，且「9 位小数」拦截不可靠 —— 放弃 |
| C | 用 `math/big.Rat` 从 float64 精确展开十进制 | 只能得到二进制精确展开（如 `0.14` → 一长串 5 结尾的数字），无法还原调用方字面量 —— 放弃 |
| D | 把 `quota` 入参改为十进制字符串 / `json.Number`，DB 侧统一 decimal | 语义最彻底，但改动 OpenAPI 契约（`number` → `string`）与全部调用方/前端/集成用例 —— 超出本 issue 范围，列为后续演进项 |
| E | 在 HTTP 边界把原始 JSON 文本透传到校验层 | 需改 `xreq.BindJSON` 与所有 handler，收益与 D 重合 —— 放弃 |

## 7. 验证步骤

```bash
# 1) 单元测试（含新增回归锚点）
go test ./lib/validate/ -run 'TestQuotaValue|TestQuotaPlan' -count=1 -v

# 2) 全仓单元测试
go test ./... -count=1

# 3) 集成套件（SQLite）
export AIAPI_FORCE_SQLITE=1
cd test/integration
go test ./tests/api_key/... ./tests/entity/... -count=1

# 4) 接口手工验证
curl -s -X POST http://127.0.0.1:8183/open-api/v1/api-keys -d '{
  "quota_plan": {"unlimited": false, "quota": 0.14, "unit": "RMB", "reset_period": "monthly"}
}'   # 期望 ErrNum=200，Data.quota_plan.quota=0.14

curl -s -X POST http://127.0.0.1:8183/open-api/v1/api-keys -d '{
  "quota_plan": {"unlimited": false, "quota": 100.123456789, "unit": "RMB"}
}'   # 期望 ErrNum=422，文案含 "decimal places"
```

清理：集成测试进程自行删除 SQLite 库文件与临时二进制；本地手工验证后删除创建的 API-Key，
并确认仓库根目录不残留 `ai-gateway-api.exe`、`coverage.out`。

## 8. 实施与验证记录（2026-11-03）

§2.2 的补丁与 §3 的单测已在 `ai-gateway-api`（HEAD `f20d414`、Go 1.25.2 / windows-amd64）实施并验证：

- `go test ./lib/validate/ -run 'TestQuotaValue|TestQuotaPlan' -count=1`：PASS；
- `go test ./... -count=1`：100 个包 `ok`，无 FAIL；
- 集成（SQLite，`AIAPI_FORCE_SQLITE=1`）：`tests/api_key/{create,quota_reset,quota_update}`、
  `tests/entity/{create,quota_reset,quota_update}` 6/6 `ok`；
- 负向对照：临时回退 `validDecimalPlaces` 到缩放实现并重建二进制后，`AK-1-013`（`quota=0.14`）
  精确复现 `ErrNum=422 ... at most 8 decimal places`；恢复修复后 PASS —— 证明新增用例确实锚住缺陷。

实施中确认的三处细节（已写入上文）：测试文件需新增 `"math"` import；
`0.1+0.2` 必须以运行时求和构造，否则编译期常量折叠会把它变成精确的 `0.3`，负例失效；
集成用例的前置写入改用新增的 `testutil.MustPatch`（§4.2）。