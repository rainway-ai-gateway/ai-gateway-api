# Issue #313 变更摘要：Entity-Type PATCH 补 level 1-5 校验与层级一致性保护

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#313](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/313)（SC1203-TC017：Entity-Type PATCH 接受越界 level（9/0）且改 level 后不检查已有 Entity 层级一致性）

## 1. 背景

### 1.1 问题现象

`PATCH /open-api/v1/entity-types/{type_name}` 接受 `level` 字段（Web 控制台"编辑类型"即如此提交，`EntityTypeUpsert.vue` / `EntityTypeList.vue`），但：

1. **范围校验缺失**：更新路径没有创建接口已有的 `1-5` 校验。实测 `{"level": 9}` 返回 200 并落库（2026-11-06 人工复核）；`artifacts/20261106-192332` 实测 `{"level": 1}` 返回 `HTTP 200`（合法值被接受证明字段可写，越界值同路径写入）。越界值落库后该类型脱离 `1-5` 语义，既不合法可作子级也不合法可作父级，**类型实质不可用但接口报告成功**。
2. **层级一致性无保护**：调高已投入使用类型的 `level` 后，其下已有 Entity 的父子关系（`父类型 level < 子类型 level`，`entities.md` §2.1/§2.5）可能静默变非法——存量关系不复查、不重写、不告警。
3. **文档缺口**：`entity-types.md` §2.4 Body 表只有 `description`，未声明 `level` 可修改——与 Web 实际能力和代码行为不一致。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `endpoints/openapi_v1/entity_type/update.go:58-71` | Body 绑定 `EntityTypeParam` 后仅校验 `type_name` 格式与 `description`，**无 `level` 范围校验** | ✅ 实锤（直读） |
| 2 | `model/entity/entity_type_manager.go:66-70` vs `:85-107` | `CreateEntityType` 有 `level` 必填 + `1-5` 校验；`UpdateEntityType` 只做存在性检查直接落库——创建/更新口径不一致 | ✅ 实锤（直读） |
| 3 | `storage/rdb/entity/entity_type.go:91-103` | 非 nil 的 `Level` 进入 UPDATE SET——越界值与任意层级真正写入 | ✅ 实锤（直读） |
| 4 | `model/entity/entity_manager.go:726-766` | `checkEntityLevel` 只在 **Entity** 创建/更新时校验；类型 level 变更后存量 Entity 不复查 | ✅ 实锤（直读） |
| 5 | `EntityTypeManager`（entity_type_manager.go:21-25） | 仅持有 `EntityTypeStorager`，**无 Entity 存储访问能力**，无法做层级一致性检查 | ✅ 实锤（直读） |
| 6 | `{"level": 9}` 成功 | 2026-11-06 用户手工复测 | ✅ 实锤（运行时，建议合入前补 envelope 留档） |
| 7 | `{"level": 1}` → 200 | `artifacts/20261106-192332/report.json` | ✅ 实锤（运行记录） |

**排除项**：与 #244 不混同（description 长度修复已生效，`A-03`/`A-04` PASSED）；非字段白名单问题（Web 实际提交 `level`，产品能力允许改 level，收敛为"允许改但没有校验"）。

### 1.3 契约依据

- `entity-types.md` §1：`level` 取值范围 1-5（数字越小级别越高）——**范围校验无需裁定**。
- `entities.md` §2.1/§2.5：父 Entity-Type 的 `level` 必须**严格小于**子 Entity-Type 的 `level`。
- `entity-types.md` §2.5 已有先例：类型被 Entity 引用时删除返回 409——**冲突类错误用 409 有文档先例**。
- **产品裁定（2026-11-08 通过）**：改 `level` 与已有子 Entity 冲突时的语义为 **409 拒绝、不做级联**；范围 `1-5` 本就无歧义。

## 2. 修复方案

### 2.1 改动 1：level 1-5 范围校验（无需裁定）

**位置**：`model/entity/entity_type_manager.go` `UpdateEntityType`（与 `CreateEntityType` 同层、对称）。

```go
if param.Level != nil && (*param.Level < 1 || *param.Level > 5) {
	err := xerror.WrapParamErrorWithMsg("level must be between 1 and 5")
	m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), param, entityTypeParamToMap(param), nil, err)
	return 0, err
}
```

选模型层的理由：与创建校验同层对称（创建校验就在 manager），PATCH 是唯一更新入口，单点覆盖；端点层（update.go）不加重复校验，保持"端点只做绑定"的分层惯例（现状端点 update.go 只校验 type_name/description 格式，范围语义归 manager——与创建路径一致）。

### 2.2 改动 2：层级一致性检查（409 拒绝，产品裁定已通过）

**裁定结论（2026-11-08，owner 接受建议方案）**：当 `level` 变更（`param.Level != nil` 且新值 ≠ 旧值）时，若该类型下已有 Entity 的存量父子关系在新 level 下违反 `父类型 level < 子类型 level`，**整个 PATCH 拒绝，返回 409 Conflict，不做任何部分写入、不做级联改写**。不采用级联/迁移策略。

错误形态：`409 Conflict`（`xerror.WrapConflictErrorWithMsg`，与 `entity-types.md` §2.5"被引用删除返回 409"同先例），消息含冲突关系数与示例，如：
`level change rejected: 2 existing entity relation(s) would violate the parent/child level constraint`。

**选 409 拒绝而非级联的理由**：
1. 文档先例：同资源"被引用时删除"已是 409 拒绝语义（`entity-types.md` §2.5），冲突拒绝是控制面一致风格；
2. 级联改写（提升/降低子类型或重写 parent_id）会静默改变计费/路由的层级继承语义，风险远大于收益；
3. 拒绝是幂等、可重试、可预期的；调用方收到 409 后可自行先调整子级或解除关系。

**备选方案（已否决，记录在案）**：级联/迁移策略——静默改写层级继承语义，风险不可控；如未来出现强需求，检查框架可复用，仅需替换冲突分支处理动作。

**裁定说明**：冲突语义已确定为 409 拒绝（见本节开头），级联方案已否决；实现按拒绝语义进行，文档同步写明。

**检查实现**（manager 层，镜像 `EntityManager.checkEntityLevel` 的依赖方向）：

`EntityTypeManager` 增加 Entity 存储依赖（现状只有 `EntityTypeStorager`，无法检查）。对称先例：`EntityManager` 已持有 `entityTypeStorager` 做反向检查（entity_manager.go:726）：

```go
type EntityTypeManager struct {
	txn                 itxn.TxnStorager
	storager            EntityTypeStorager
	entityStorager      EntityStorager   // 新增：层级一致性检查需要
	operationLogManager ioperlog.OperationLogRecorder
}

func NewEntityTypeManager(txn itxn.TxnStorager, storager EntityTypeStorager, entityStorager EntityStorager) *EntityTypeManager
```

检查逻辑（`UpdateEntityType` 内、存在性检查之后、落库之前）：

```go
if param.Level != nil && (oldEntityType.Level == nil || *param.Level != *oldEntityType.Level) {
	if err := m.checkLevelChangeConsistency(ctx, *oldEntityType.TypeName, *param.Level); err != nil {
		// 409，记操作日志（失败路径），不落库
		return 0, err
	}
}
```

`checkLevelChangeConsistency` 步骤（复用既有存储方法，全量语义已核实：storage `FetchEntityList` 忽略分页、返回全量）：

1. `entitiesOfType ← entityStorager.FetchEntityList({Type: T})`；
2. **作为子级**：对每个 `ParentID` 非空的 E，取父 Entity 及其类型 level，`父level >= newLevel` 即冲突；
3. **作为父级**：对每个 E，`children ← FetchEntityList({ParentID: E.ID})`，对每个 child 取其类型 level，`子level <= newLevel` 即冲突；
4. 任一冲突即汇总返回 409；类型 level 为 nil（脏数据）按冲突计并说明。

依赖注入连锁改动：`stateful/container/rdb/components.go:303` 的 `NewEntityTypeManager(...)` 调用补传 `container.EntityStorager`（其定义在 components.go 内已在 `NewEntityManager` 使用，无循环依赖问题）。

### 2.3 改动 3：文档修订（与代码同卡落地）

`entity-types.md` §2.4：

1. 基本信息"含义"：「更新Entity-Type描述」→「更新Entity-Type（描述与层级级别）」；
2. Body 表新增 `level` 行：`N | 类型描述……` 之后加 `| level | int | 层级级别 | N | - | 非必填；若传入，取值范围 1-5 的整数（数字越小级别越高）；若新值与已有 Entity 的父子层级约束冲突，返回 409 |`；
3. Body 表补充只读说明：请求体中的 `type_name` 与 `create_time` 不被改写（当前实现本就不写入，文档钉死该语义）；
4. **约束**节新增：`若修改 level 后该类型下已有 Entity 的父子关系（父类型 level 必须小于子类型 level）不再满足，返回 409 且不做任何修改`（或按 owner 裁定的级联语义表述）。

### 2.4 修复后行为对照

| 请求 | 修复前 | 修复后 |
|------|--------|--------|
| `PATCH {"level": 9}` / `{"level": 0}` / `{"level": -1}` | 200 落库（类型实质报废） | **422**，回读 level 保持原值 |
| `PATCH {"level": 3}`（1-5 内，无 Entity 引用该类型） | 200 | 200，回读 `level=3`，其余字段不变 |
| `PATCH {"level": 3}`（有存量父子关系，3 合法） | 200（可能静默非法） | **409**，无任何写入；解除冲突后可重试成功 |
| `PATCH {"description": "x"}`（不带 level） | 200 | 200（不触发一致性检查，`param.Level == nil`） |
| `PATCH {"level": <原值>}` | 200 | 200（新旧相等，跳过一致性检查） |
| `PATCH {"type_name": "...", ...}` | type_name 被校验但**不写入**（storage 只 SET Description/Level） | 行为不变，文档明示只读 |

## 3. 测试计划

### 3.1 单元测试（`model/entity/entity_type_manager_test.go` + `mocks_test.go`）

`NewEntityTypeManager` 签名变更需同步全部构造调用点（该文件内 5 处）；复用同包既有 `fakeEntityStorager`。

| 用例 | 构造 | 期望 |
|------|------|------|
| 越界 0/6/9/-1 | update param.Level 越界 | 422，storager.Update 未被调用（mock 断言） |
| 合法 1-5 且无引用 | update Level=3，无 Entity | 200，Update 被调用 |
| 调高 level 破坏子级关系 | 类型 T level 2→3；T 下 Entity E 有子 C（C 类型 level 3） | 409，Update 未被调用 |
| 调高 level 不破坏 | T level 2→3；子类型 level 4 | 200 |
| 作为子级被破坏 | T level 4→3；T 的 Entity 之父类型 level 3 | 409 |
| level 不变 | 新旧值相同，即使有理论上冲突的关系 | 200（跳过检查，幂等 PATCH 不拦） |
| 仅改 description | Level==nil | 200，不触发一致性检查 |
| 失败路径操作日志 | 409/422 用例 | `recordEntityTypeOperation` 带 err 被调用（对齐既有失败日志模式） |

`make test-model-cover-gate`（model ≥70% 门槛）。

### 3.2 集成测试（`test/integration/tests/entity_type/`）

在既有 update 场景追加（先查现有用例编号续编）：

| 用例 | 步骤 | 期望 |
|------|------|------|
| 越界 level → 422 且不变 | create level=3 → PATCH `{"level": 9}`、`{"level": 0}` → GET 回读 | 422，`level` 仍为 3 |
| 合法 level 修改 | PATCH `{"level": 2}` → GET 回读 | 200，`level=2`，`description`/`type_name`/`create_time` 不变 |
| 仅传 level 保其他 | PATCH 只带 `{"level": 1}` | description 保持原值（A-05 新语义） |
| 层级冲突 → 409 | 建 parent(类型 level 1)→child(类型 level 2) 的 Entity 链；PATCH 把类型 2 改为 1（或类型 1 改为 2）使存量关系非法 | 409，GET 回读 level 不变，Entity 关系不变 |
| 解除冲突后可改 | 删除子 Entity 后重发同一 PATCH | 200 |
| description 回归 | §3.4 既有边界（255/256 字节、控制字符） | 不回退（#244 成果守护） |

后端：SQLite + MySQL 双后端。

### 3.3 E2E / 验收判据

对照 issue 验收判据五条：越界 422 且回读不变 / 合法 1-5 成功且其他字段不变 / 冲突按已裁定语义（409 拒绝）且回读一致 / §2.4 文档补齐 / SC1203-TC017 `A-05` 按新语义 PASSED 且 TC011/TC015 不回归。

## 4. 兼容性与发布

| 项 | 结论 |
|----|------|
| 行为变化 | 越界 level 由"200 落库"收紧为 422（纠偏）；引入 409 拒绝新错误形态（文档同步写明） |
| 数据迁移 | 无 DDL；存量越界 level 数据（若 QA 环境存在 tc017 类临时记录）由 cleanup 处理，生产一般无 |
| Web 兼容 | Web 编辑弹窗提交 `level`（1-5 内）不受影响；提交越界值时前端可接 422 提示 |
| 下游依赖 | 仅管理面；BFE 数据面不直接消费 entity_types 表 |

## 5. 关联风险与后续事项

1. **~~【需产品裁定】冲突语义~~（已裁定：409 拒绝，2026-11-08）**：见 §2.2；级联方案已否决。
2. **`{"level": 9}` 的 envelope 留档**：issue 建议合入前补一次带 `ErrNum`/回读的最小复现记录（QA 环境 requeue TC017 时自然覆盖）。
3. **TC017 `A-05` 断言语义变更**：由"未声明字段必须拒绝"改为"`level` 可修改 + 范围校验 + 一致性保护 + `type_name`/`create_time` 只读"——测试设计文档（integration-test 仓）需同步，跨仓跟进。
4. **一致性检查成本**：检查为 O(T 类型实体数 + 其子级数)，控制面量级（实体数为运营规模，通常 < 千级）；如需优化可后续引入按 type 的索引查询，本次不做。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 产品裁定：层级冲突语义 = 409 拒绝、不做级联（2026-11-08）
- [x] 代码实现（`UpdateEntityType` 范围校验 + `EntityStorager` 注入 + `checkLevelChangeConsistency` 409 检查；container 装配同步）
- [x] 文档修订（`entity-types.md` §2.4 含义/level 行/只读字段/409 约束；sys-design《API-Key与Entity关联及模型继承》§3.3 层级守卫说明）
- [x] 单元测试（`TestEntityTypeManager_UpdateEntityType_LevelRange` 4 越界 + 通过臂；`TestEntityTypeManager_UpdateEntityType_LevelConsistency` 5 子用例含 mock 断言"Update 未到达 storager"与 `xerror.Resolve(err).ErrNo == 409` 精确断言；13 处构造调用点迁移；`go test ./model/...` 全量通过）
- [x] 集成测试（ET-4-008 越界 422 且回读不变、ET-4-009 合法修改且其余字段不变、`TestEntityType_Update_LevelConsistency` 409 + 解除冲突后成功；SQLite 与 MySQL 双后端 15 包全过）
- [ ] QA requeue SC1203-TC017 转 PASS 并关闭 issue #313
- [ ] 跨仓跟进：integration-test 仓 TC017 `A-05` 断言语义修订（"未声明字段拒绝"→"level 可改 + 校验 + 保护"）

## 7. 实施校准记录（实现过程中确认的事实）

1. **409 断言口径**：`WrapConflictErrorWithMsg` 的 message 不含 "409" 字面量，ErrNo 由 `xerror.Resolve` 映射；单测用 `xerror.Resolve(err).ErrNo == 409` 精确断言，不用字符串包含。
2. **子用例构造校准**：初版"父级提高"通过用例误用 3→2（实际等于父级 level 构成冲突），修正为子级 3→4（严格大于父级 level 的合法变更）——反向校验了检查逻辑确实双向生效。
3. **`level` 不变跳过检查的语义**：幂等 PATCH（新旧值相同）不触发一致性检查，集成/单测均有用例钉死——避免"重复提交合法值被存量脏关系误拦"。
