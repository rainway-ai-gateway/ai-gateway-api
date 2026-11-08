# Issue #309 变更摘要：/model-prices 的 provider / model / base_model 补齐 1-255 长度校验

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#309](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/309)（SC1301-TC031：`/model-prices` 的 provider / model / base_model 未校验 1-255 长度上限）

## 1. 背景

### 1.1 问题现象

`POST /open-api/v1/model-prices`、`PUT /open-api/v1/model-prices/{id}`（及 `PUT /model-prices` 按查询更新）与整表导入（`POST /model-prices/import`）对 `provider`、`model`、`base_model` 仅校验"非空"。超过 255 字符的输入不会返回 422，而是进入存储层；`model_prices` 表三个字段均为 `VARCHAR(255)`，实际表现取决于数据库严格模式（MySQL 8 默认严格模式报 500 / `Data too long`；非严格模式可能截断落库），与契约要求的 `ErrNum=422` 不符。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

1. **共同校验入口缺长度检查**：`ValidateModelPrice`（`model/imodel_price/validate.go:195-207`）对三个字段只执行 `strings.TrimSpace(...) == ""`，无长度上限校验。已核实。
2. **无统一上限常量**：不存在 `MaxModelPriceFieldLength` 之类的常量，DB DDL 的 `VARCHAR(255)`（`db_ddl.sql:438-457`）成为事实上限，但错误形态不是 422。已核实。
3. **三条写入路径共用该入口**，一处修复即可全覆盖（`model/imodel_price/model_price.go` 直读核实）：
   - 创建：`Manager.CreateModelPrice`（:100）→ `ValidateModelPrice`
   - 更新：`Manager.UpdateModelPrice`（:116）→ `ValidateModelPrice`
   - 导入：`Manager.ImportModelPrices`（:191）逐条 `ValidateModelPrice`；YAML 解析后经 `ValidateImportFile`（`validate.go:291`）→ `ValidateModelPrice`

### 1.3 契约依据

- `design-docs/api-define/OpenAPI接口定义/model-prices.md` §1 数据模型（L47-L49）：
  - `provider`：必填；非空；**长度 1-255**；仅作为价格归集标识，不强制校验在 `/providers` 中存在
  - `model`：必填；非空；**长度 1-255**
  - `base_model`：必填；非空；**长度 1-255**
- 非法输入按统一信封返回 `ErrNum=422`（Param Illegal）。
- 规范性：以已发布的 `model-prices.md` 为准；DB 列长度不能替代 API 层校验。

## 2. 修复方案

### 2.1 方案：在 `ValidateModelPrice` 内补齐长度校验（模型层单点修复）

在 `model/imodel_price/validate.go` 定义统一常量并在 `ValidateModelPrice` 的非空校验旁增加长度校验，非法输入返回 `xerror.WrapParamErrorWithMsg`（→ HTTP 422）。

**选择模型层而非端点层的原因**：三条写入路径（create / update / import）与 YAML 导入的逐条校验全部汇聚于 `ValidateModelPrice`；端点层（`create.go`、`update.go`、`import.go`）不各自校验业务字段。单点修复无遗漏，也符合"校验随模型走"的既有分层（端点只做绑定，业务校验在 manager/model）。

**长度计量口径（假设，需产品 owner 确认）**：契约用词为"字符"，`ModelPrice` 的 `Provider/Model/BaseModel` 为业务标识（当前实际取值均为 ASCII）。MySQL `VARCHAR(255)` 的 255 是**字符数**而非字节数，因此校验按 **Unicode 字符数（`utf8.RuneCountInString`）≤255** 实现，与契约措辞和 DB 语义同时对齐；若产品裁定按字节计，仅需把计量函数改为 `len()`，其余不变。

### 2.2 具体改动

**改动 1：`model/imodel_price/validate.go`**

```go
// MaxModelPriceFieldLength bounds provider / model / base_model to the
// 1-255 character limit required by model-prices.md §1 and the
// VARCHAR(255) columns of model_prices (issue #309).
const MaxModelPriceFieldLength = 255

// validatePriceField checks one of provider / model / base_model: non-empty
// (already trimmed by callers) and within the length limit.
func validatePriceField(name, s string) error {
	if utf8.RuneCountInString(s) > MaxModelPriceFieldLength {
		return xerror.WrapParamErrorWithMsg("%s length must be between 1 and %d characters", name, MaxModelPriceFieldLength)
	}
	return nil
}
```

`ValidateModelPrice` 中三处非空校验各追加一次长度校验：

```go
if strings.TrimSpace(m.Provider) == "" {
	return xerror.WrapParamErrorWithMsg("provider is required")
}
if err := validatePriceField("provider", m.Provider); err != nil {
	return err
}
// model / base_model 同构
```

校验顺序保持在非空校验之后、`mode` 校验之前，错误信息风格与既有 `WrapParamErrorWithMsg` 一致（`%s length must be between 1 and %d`）。

**改动 2：不动端点与存储层。** 端点（create/update/import）与 DAO 无改动；无 DDL 变更（列已是 VARCHAR(255)，只是补 API 层前置拦截）。

### 2.3 修复后行为

| 输入 | 修复前 | 修复后 |
|------|--------|--------|
| `provider`/`model`/`base_model` 长度 256 | 进入存储层（严格模式 500 / 非严格截断落库） | **422** Param Illegal，不落库 |
| 长度恰好 255 | 200 | 200（不变） |
| 长度 0 / 全空白 | 422（已有非空校验） | 422（不变） |
| `mode` 非法 / 价格键非法等其他校验 | 422（不变） | 422（不变） |
| 导入 YAML 中含超长行 | 进入存储层 | 422，`ValidateImportFile` 包装为 `models[i]: ...` 错误形态 |

## 3. 测试计划

### 3.1 单元测试（`model/imodel_price/validate_test.go`）

扩展 `TestValidateModelPrice` 并新增边界用例：

| 用例 | 构造 | 期望 |
|------|------|------|
| 255 边界 | 三字段各 `strings.Repeat("a", 255)` | 通过（其余字段合法） |
| 256 拒绝 ×3 | 分别只让 provider / model / base_model 为 256 字符 | 422，错误信息含字段名 |
| 中文计量 | 三字段各 255 个汉字 | 通过（字符口径；若裁定字节口径则此用例转 422，见 §2.1 假设） |
| 空白仍拒 | `" "`（长度 1 但 TrimSpace 后为空） | 422（非空校验先行，行为不变） |
| 导入路径 | `ValidateImportFile` 中一条记录 `model` 超长 | 422，错误前缀 `models[i]:` |

`make test-model` 与 `make test-model-cover-gate`（model 层覆盖率 ≥70% 门槛）。

### 3.2 集成测试（`test/integration/tests/model_price/`）

在既有 `create` / `update` / `import` 场景中补用例（不新建场景目录，沿用现有组织）：

| 用例编号建议 | 路径 | 请求 | 期望 |
|--------------|------|------|------|
| create 场景追加 | `POST /model-prices` | `provider`=256 字符 | 422；随后列表查询确认无该记录 |
| update 场景追加 | `PUT /model-prices/{id}` | `model`=256 字符 | 422；再 GET 确认原记录未被修改 |
| import 场景追加 | `POST /model-prices/import` | YAML 单行 `base_model`=256 字符 | 422；确认存量记录未被 replace 清掉（断言导入前预置记录仍在） |
| 对照 | 同上三路径 | 字段=255 字符 | 200，落库值与提交值一致（GET 回读校验） |

后端：SQLite 与 MySQL 双后端均跑（MySQL 可复现修复前"严格模式 500"的实际损害形态；422 断言与后端无关）。

### 3.3 E2E / 验收判据

- SC1301-TC031 断言：`provider`/`model`/`base_model` 长度 256 → 422 Param Illegal，不落库；长度 255 → 200。
- 回归臂：既有 model_prices CRUD / import / batch_price / tier_prices 用例不回退。

## 4. 兼容性与发布

| 项 | 结论 |
|----|------|
| 行为变化 | 超长输入由"500 或截断落库"收紧为 422。契约早已声明 1-255，属纠偏非破坏；对正常调用方（实际取值远小于 255）无影响 |
| 数据迁移 | 无 DDL；无存量数据问题（严格模式下超长数据本就无法写入） |
| 导出 / 数据面 | `ModelTable` 导出来自已落库记录，不受校验收紧影响 |
| 下游依赖 | 仅管理面 OpenAPI；Web 前端无依赖 |

## 5. 关联风险与后续事项（不在本次范围）

1. **长度计量口径**：契约"字符"与 `VARCHAR(255)` 均指向字符数，本方案按 `utf8.RuneCountInString` 实现；若产品 owner 裁定按字节，改动收敛在 `validatePriceField` 一个函数。
2. **存量库中已被截断的记录**（非严格模式下产生）：不在本次清理范围；如需治理可另立数据修复任务。
3. **`mode` 等其余字段**：`mode` 列 `VARCHAR(50)` 且已有枚举校验，天然受控；`price_currency` 已有 `RMB` 固定值校验。无需额外处理。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 代码实现（`model/imodel_price/validate.go`：`MaxModelPriceFieldLength` 常量 + `validatePriceField`，三字段接入）
- [x] 单元测试（`TestValidateModelPrice_FieldLength`：255 边界/256 拒绝×3/中文计量/空白先行拒绝/错误信息含字段名；`TestValidateImportFile_FieldLength`：`models[i]` 包装形态；`go test ./model/...` 全量通过）
- [x] 集成测试（create MP-2-007/008、update MP-7-005、import MP-1-012；SQLite 与 MySQL 双后端 10 包全过，负向断言含"未落库/原记录未变/存量未被清"）
- [x] 文档同步：`sys-design/数据库设计文档.md` §6.6 校验边界说明（api-define/model-prices.md 契约原文已含 1-255，无需改动）
- [ ] E2E SC1301-TC031 转 PASS（QA 环境复跑）并关闭 issue #309

## 7. 实施校准记录（实现过程中确认的事实）

1. **长度计量口径**：按 Unicode 字符数（`utf8.RuneCountInString`）实现，与契约"长度 1-255"措辞及 MySQL `VARCHAR(255)` 字符语义对齐；单测含"255 个汉字通过"用例钉死该口径。若产品裁定按字节计，改动收敛在 `validatePriceField` 一个函数。
2. **校验顺序**：非空（TrimSpace）校验保持在长度校验之前——全空白值（长度合法但 TrimSpace 为空）仍由非空校验拒绝，行为与修复前一致。
3. **导入路径的保护语义**：`ImportModelPrices` 在任何删除前完成全部逐条校验（`manager_test.go` 行为），集成用例 MP-1-012 以"预置记录在非法 replace 导入后仍存活"钉死该顺序，防止未来回归为"先清表再校验"。
