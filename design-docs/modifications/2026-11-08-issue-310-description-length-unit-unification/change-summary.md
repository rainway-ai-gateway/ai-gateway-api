# Issue #310 变更摘要：description 长度单位口径统一为 UTF-8 字节（Provider 文档 / Certificate 下限）

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#310](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/310)（SC2101-TC005：description 长度单位口径不一致：Provider 按字节、Certificate 下限按字符）

## 1. 背景

### 1.1 问题现象

两个接口的 `description` 长度单位在文档与实现之间不一致，多字节（中文等）输入被误拒：

1. **Provider**：文档规定 0-256 **字符**，实现按 **UTF-8 字节**校验（200 个中文 = 600 字节 → 422，文档口径下合法）。
2. **Certificate**：文档规定 2-256 **字节（按 UTF-8 字节计）**，上限按字节校验，但下限由 `required,min=2` 的 struct tag 按**字符（runes）**校验（单个中文 = 1 字符/3 字节 → 被下限拒绝，文档口径下合法）。

### 1.2 根因核实结论（源码/文档直读，与 issue 定位一致）

| 项 | 文档（已发布） | 实现 | 核实 |
|----|--------------|------|------|
| Provider `description` 上限 | `providers.md:57`（数据模型）、`providers.md:661`（§3 校验规则）："长度 0-256 **字符**" | `model/iprovider/provider.go:806-810` `validateProviderDescription` 用 `len(s)`（**字节**），常量 `MaxProviderDescriptionLength = 256`（L34）；调用点 `ValidateProviderParam`（`provider.go:503`，create/update 共同入口） | ✅ 实锤 |
| Certificate `description` 下限 | `certificates.md:21`、`:54`："必填；长度 2-256 **字节（按 UTF-8 字节计）**" | `endpoints/openapi_v1/certificate/create.go:41` struct tag `validate:"required,min=2"`，go-playground/validator 的 `min` 按 **runes** 计 | ✅ 实锤 |
| Certificate `description` 上限 | 同上（字节） | `create.go:93` `validate.Description(..., MaxCertificateDescriptionLength=256, ...)` 用 `len(s)`（**字节**，`lib/validate/validate.go:365-368`） | ✅ 实锤（上限本无缺陷） |

**同族先例**（统一为字节口径，作为本方案的锚点）：

- `clusters.md` / `entities.md` / `entity-types.md` 的 description 均已明确写为"**字节（按 UTF-8 字节计）**"，实现经 `lib/validate.Description` 按字节校验（该函数注释明确"matching the 字节 wording of entity-types.md / entities.md / clusters.md"）。
- `lib/validate.Description` 的全部使用方（api_key 512 / entity 255 / entity_type 255 / cluster 256 / certificate 上限 256）均为字节口径——**唯一按字符计的点就是 Certificate 的 `min=2` tag，唯一按字节计但文档写字符的就是 Provider**。

**DB 层无冲突**：`providers.description` 与 `certificates.description` 均为 `VARCHAR(1024)`（utf8mb4 按字符计），远大于 256，字节口径校验不会触到 DB 上限。

### 1.3 契约依据与裁定需求

- Provider：`providers.md#1-数据模型` L57、`#3-校验规则` L661（"0-256 字符"）。
- Certificate：`certificates.md` L21、L54（"2-256 字节（按 UTF-8 字节计）"）。
- issue 明确要求产品确认每字段最终单位；**本方案采纳 issue 的建议方向：与 Cluster/Entity 统一为 UTF-8 字节**（理由见 §2.1）。该决策修订 providers.md 的已发布措辞（200 中文由"文档合法"变为"合法字节数内才合法"），需在合入前经产品 owner 点头（见 §5 后续事项 1）。

## 2. 修复方案

### 2.1 方案：统一为 UTF-8 字节口径

| 接口 | 改动侧 | 具体改动 |
|------|--------|----------|
| Provider | **文档**（实现不动） | `providers.md` L57、L661 "长度 0-256 字符" → "长度 0-256 字节（按 UTF-8 字节计）"。实现已是字节口径（`len(s)`），与改后文档一致 |
| Certificate | **实现**（文档不动） | `create.go:41` tag 去掉 `min=2`（保留 `required`）；下限改为字节口径校验，见改动 2 |

选择字节而非字符的理由：

1. **先例多数**：Cluster / Entity / Entity-Type 三处文档 + 全部 `lib/validate.Description` 使用方均已是字节口径，Certificate 上限也是字节——字节是既成事实的主流。
2. **改动面最小**：选字节只改 providers.md 两行 + certificate 一个 tag 与一处下限检查；选字符则需改 `lib/validate.Description`（5 个调用方）、`validateProviderDescription`、三处文档，且 Cluster/Entity 的已发布"字节"条款也要推翻。
3. **DB 语义中性**：utf8mb4 `VARCHAR(1024)` 对两种口径都够用，无存储层倒逼。

**备选方案（不采纳，记录在案）**：全部统一为字符——需同步修改 `lib/validate.Description`、`validateProviderDescription`、`providers.md`/`clusters.md`/`entities.md`/`entity-types.md` 四处文档措辞及各自单测；影响面约为字节方案的 4 倍，且推翻三处已按字节落地的已发布条款。

### 2.2 具体改动

**改动 1（文档）**：`design-docs/api-define/OpenAPI接口定义/providers.md`

- L57：`| \`description\` | string | Provider 描述信息 | - | 非必填；若传入，长度 0-256 字符；不能包含控制字符 |` → `... 长度 0-256 字节（按 UTF-8 字节计）；不能包含控制字符 |`
- L661（§3 校验规则第 2 条）：同改。

**改动 2（实现）**：`endpoints/openapi_v1/certificate/create.go`

```go
type CreateParam struct {
	Name *string `json:"cert_name" validate:"required,min=2"`

	// 下限不再由 min tag 承担（min 按 runes 计，与 certificates.md 的
	// 2-256 字节口径不符，issue #310）；下限字节检查在 CreateAction 中进行。
	Description *string `json:"description" validate:"required"`
	...
}
```

CreateAction 中把原 `validate.Description(*param.Description, ...)` 调用替换为带下限的字节校验：

```go
// lib/validate 新增（与 Description 共用 NoControlChars 逻辑）：
// DescriptionRange validates a description string: byte length within
// [minLen, maxLen] (UTF-8 bytes) and no control characters.
func DescriptionRange(s string, minLen, maxLen int, name string) error {
	if len(s) < minLen || len(s) > maxLen {
		return xerror.WrapParamErrorWithMsg("%s length must be between %d and %d bytes", name, minLen, maxLen)
	}
	return NoControlChars(s)
}
```

```go
// certificates.md §1/§2.1：description 必填、长度 2-256 字节（按 UTF-8 字节计）、
// 不能包含控制字符。下限与上限统一在本函数按字节校验（issue #310）；
// 校验位于 createActionProcess 之前，被拒请求不落库、不触发默认证书切换。
if err := validate.DescriptionRange(*param.Description, 2, validate.MaxCertificateDescriptionLength, "description"); err != nil {
	return nil, err
}
```

校验顺序：struct tag `required`（非空）→ `DescriptionRange`（字节下限/上限 + 控制字符）。被拒请求不落库、不触发默认证书切换的既有语义保持。

**不改动项**：`cert_name` 的 `min=2`（名称只允许 ASCII 字符集，` CertName` 正则 `^[a-zA-Z0-9_.-]+$`，runes==bytes，无歧义）；Certificate 更新路径（部分更新，`description` 省略即不改，无下限语义）；Provider 实现代码。

### 2.3 修复后行为

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| Provider description = 200 中文（600 字节/200 字符） | 422（字节超限，但文档写字符 → 口径争议） | 422（**文档与实现一致**：600 字节 > 256） |
| Provider description = 85 中文 + 1 ASCII（256 字节） | 422（误拒） | 200 |
| Provider description = 257 ASCII | 422 | 422（不变） |
| Certificate description = 单中文（1 字符/3 字节） | 422（`min=2` 按字符误拒） | **200**（3 字节 ∈ [2,256]） |
| Certificate description = 单 ASCII（1 字符/1 字节） | 422 | 422（1 字节 < 2，下限按字节） |
| Certificate description = 256 / 257 字节 | 200 / 422 | 200 / 422（不变） |
| Certificate description = 空白仅 1 空格 | 422（min=2） | 422（1 字节 < 2，行为等价） |

## 3. 测试计划

### 3.1 单元测试

- `lib/validate/validate_test.go`：新增 `TestDescriptionRange`——1/2/256/257 字节边界、控制字符拒绝、错误信息含上下限与单位（bytes）。
- `endpoints/openapi_v1/certificate/`：若有 CreateParam 相关测试文件则补字节下限用例（1 字节拒、2 字节过）；无则依赖集成测试。

### 3.2 集成测试（`test/integration/tests/`）

| 场景 | 追加用例 | 请求 | 期望 |
|------|----------|------|------|
| `provider/create`（或现有 create 场景） | description 多字节边界 | description = 85 中文 + "a"（256 字节） | 200，回读一致 |
| 同上 | 超限 | description = 200 中文（600 字节） | 422 |
| `certificate/create` | 下限字节口径（修复点） | description = 单中文（3 字节） | **200**，回读一致 |
| 同上 | 下限之下 | description = 单 ASCII（1 字节） | 422 |
| 同上 | 上限臂 | description = 256 字节 / 257 字节 | 200 / 422 |

用例编号沿用各场景现有 MP/PR/CT 前缀递增。后端：SQLite + MySQL 双后端跑（校验在入口层，与后端无关；422 断言双端一致）。

### 3.3 测试设计文档（跨仓跟进）

issue 证据指出 `docs/test-design/SC2101/SC2101-TC005.md`（Provider description 矩阵仅 257 ASCII，未覆盖多字节）与 `SC3201/SC3201-TC012.md`（证书名称/描述写成"字符"，与 certificates.md"字节"不一致）——该目录属集成测试仓（`integration-test/`），**不在本仓改动范围**，列为跨仓跟进项（见 §5）。

### 3.4 E2E / 验收判据

- SC2101-TC005 及 SC1301 同族断言按"字节口径 oracle"重判后转 PASS。
- 回归臂：既有 provider/certificate 场景全部用例不回退。

## 4. 兼容性与发布

| 项 | 结论 |
|----|------|
| 行为变化 | Certificate 单多字节字符 description 由 422 放宽为 200（对齐已发布文档，属纠偏）；Provider 多字节超长由"文档合法实现拒绝"变为"文档实现一致拒绝"（文档条款修订，需产品确认，见 §5.1） |
| 数据迁移 | 无 DDL；存量数据无影响（DB 列 VARCHAR(1024) 远大于 256 字节） |
| API 兼容 | 请求/响应字段无增删；仅 description 多字节边界语义变化 |
| 下游依赖 | 仅管理面 OpenAPI；Web 前端按字符提示长度的话需同步提示口径（跟进项） |

## 5. 关联风险与后续事项

1. **【需产品裁定】providers.md "字符→字节"为已发布条款修订**：200 中文 description 由"文档合法"变为"非法"。本方案按 issue 建议与 Cluster/Entity 先例取字节口径；若产品裁定取字符口径，应改走备选方案（§2.1），本方案的 Certificate 改动方向不变（其文档本来就是字节）。
2. **跨仓测试设计修订**：integration-test 仓 `SC2101-TC005.md`、`SC3201-TC012.md` 的多字节矩阵与"字符"表述需同步修订（本仓无法完成）。
3. **Web 前端提示口径**：若前端对 description 做实时长度提示，需确认其计数口径与字节口径一致。
4. **其他域 description 巡检结论**：api_key（512 字节）/entity（255 字节）/entity_type（255 字节）/cluster（256 字节）文档与实现均已按字节对齐，无同类缺陷；本修复后全域统一。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 单位口径决策：UTF-8 字节（与 Cluster/Entity/Entity-Type 先例及 `lib/validate.Description` 全部使用方对齐；§5.1 的产品裁定项随合入走评审）
- [x] 代码实现（`lib/validate` 新增 `DescriptionRange`；`certificate/create.go` 去 `min=2` tag 改字节下限）
- [x] 文档修订（`providers.md` L57/L661："0-256 字符"→"0-256 字节（按 UTF-8 字节计）"；sys-design 巡检无单位表述无需改动）
- [x] 单元测试（`TestDescriptionRange`：2/256 字节边界、单汉字过下限、控制字符、错误信息含单位）；`lib/validate`、certificate/product_cluster/entity 端点包全过
- [x] 集成测试（certificate `CERT-1-017` 注释校准 + `CERT-1-018` 单汉字合法；provider `PV-1-020` 256 字节多字节边界合法、`PV-1-021` 600 字节拒绝；SQLite 与 MySQL 双后端 19 包全过）
- [ ] E2E 相关 TC 按字节口径 oracle 重判转 PASS 并关闭 issue #310
- [ ] 跨仓跟进：integration-test 仓 `SC2101-TC005.md`、`SC3201-TC012.md` 多字节矩阵与表述修订

## 7. 实施校准记录（实现过程中确认的事实）

1. **CERT-1-017 注释校准**：原用例注释"（tag min=2）"在实现改动后失效（该 tag 已移除），行为断言不变（1 ASCII 字节仍 422），注释改为"长度为 1 字节被拒绝"。
2. **`k8s_pool` 集成测试在并发全量跑时出现 1 次偶发失败**（`cluster_table Config should be an object`）：经 stash 对照实验证实与本次改动无关（干净 HEAD 单跑通过、带改动单跑通过、重跑全量通过），定性为并发负载下的既有偶发，未处理。
3. **受影响面确认**：`lib/validate.Description` 的其余使用方（api_key/entity/entity_type/cluster）行为零变化（新函数为纯新增，`Description` 未动）。
