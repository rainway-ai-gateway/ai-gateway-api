# Issue #322 变更摘要：Provider PATCH 部分更新被创建式必填校验拒绝（先校验后合并）

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#322](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/322)（SC1601-TC003/TC009：`PATCH /providers/{name}` 只改 `description` 返回 422 `instance_pool is required`）

## 1. 背景

### 1.1 问题现象

`PATCH /open-api/v1/providers/{provider_name}` 提交部分字段（如仅 `{"description":"updated"}`）时，在**进入合并逻辑之前**就被当作完整创建体校验：

- 静态 Provider（`instance_source=instance_pool`）→ 422 `Param Illegal: instance_pool is required`；
- K8s Provider（`instance_source=k8s_pool`）→ 422 `Param Illegal: model_protocols is required`（携带未知已删字段 `k8s_instance_pool` 也不影响，该字段被 binder 静默忽略）。

任何不重复携带创建必填字段的 PATCH 都被拒绝，调用方必须回退到「提交完整创建体」的 workaround。违反 `providers.md §2.4 更新Provider` 的**部分更新**条款：「请求体中未提供的字段保持原值不变」。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `endpoints/openapi_v1/provider/update.go:52-58` | PATCH 请求体绑定到 `iprovider.ProviderParam`，仅 URI 补 `Name`，未提供字段保持 Go 零值（nil 切片/nil 指针） | ✅ 实锤（直读） |
| 2 | `model/iprovider/provider.go:233-237` | `UpdateProvider` 在 `FetchProvider`（:241）**之前**先执行 `ValidateProviderParam(param)`——部分体先过创建式必填校验 | ✅ 实锤（直读） |
| 3 | `model/iprovider/provider.go:880`（经 `:509`）| `validateProviderInstanceSource`：merged source 非 k8s_pool 时要求 `instance_pool` 非空——PATCH 省略时零值 nil → 422 | ✅ 实锤（直读） |
| 4 | `model/iprovider/provider.go:513` | `len(param.ModelProtocols) == 0` → 422——K8s 变体省略时的失败点 | ✅ 实锤（直读） |
| 5 | `model/iprovider/provider.go:1102-1147` | `applyProviderUpdate(existing, param)` 已实现正确的合并语义（nil = 保留原值、非 nil 含空切片 = 显式覆盖），且已被 hook 快照路径（:317）使用——**合并器现成，只是校验没用上** | ✅ 实锤（直读） |
| 6 | `model/iprovider/provider.go:250-260` | 既有「有效 protocol_paths 一致性」检查只把 paths 回落到存量值，protocols 仍取 param（可 nil）——若移除 :234 校验但保留本块，省略 protocols 的有 paths 存量 Provider 会被误判（paths 对 nil protocols 报 not declared）；本块必须一并被合并校验取代 | ✅ 实锤（直读，修复设计必要输入） |

**历史上下文**：#147（`2026-09-09-issue-147-provider-patch-partial-update`）修复了 PATCH 省略字段被**静默覆盖**的问题（time_zone/tiers/models/keys/model_endpoint），但当时明确把 `instance_pool`/`model_protocols` 排除在范围外——「`ValidateProviderParam` 强制非空，PATCH 必须提供，不在本次范围」。#322 即该遗留范围违反 `providers.md §2.4` 部分更新条款的收口：合约要求**所有**未提供字段保持原值（含这两个）。

**排除项**：

- 「资源未持久化必填字段」——issue 证据：同一 Provider GET 回读包含非空 `instance_pool`/`model_protocols`；
- 「权限/路由问题」——补回省略字段后同一 token PATCH 立即 200；
- 「合并器缺失」——`applyProviderUpdate` 现成且语义正确（nil vs 非 nil 区分与 DAO nil-skip 语义一致）。

### 1.3 契约依据与裁定需求

- `design-docs/api-define/OpenAPI接口定义/providers.md §2.4 更新Provider`（effective / normative，绑定 product-design-release=0.9.0）：接口为**部分更新**语义——「请求体中未提供的字段（description、model_endpoint、models、keys、time_zone、tiers、instance_source、k8s_pool_name 等）保持原值不变」；`model_protocols` 仅在**显式提供时**按全量替换处理；`instance_pool` 仅在显式提供且 `instance_source=instance_pool` 时同步集群实例池。
- **无需产品裁定**：条款已明确，是 #147 时代的实现范围决策与现行合约冲突，按合约修复。

## 2. 修复方案

### 2.1 核心思路：先合并、后校验（校验「有效参数体」而非「原始部分体」）

`ValidateProviderParam` 本身是创建/更新共用的正确校验器，**不需要区分模式**——只要让它作用于「PATCH 体 + 存量回退」合并出的有效参数体，创建必填约束自然变成「合并后仍必填」的语义：

- PATCH 省略 `instance_pool`（静态模式）→ 有效体回落到存量池 → 非空 → 通过；
- PATCH 显式 `instance_pool: []`（静态模式）→ 有效体为空 → 仍 422（**创建守卫不回归**）；
- 模式切换守卫、protocol_paths 一致性、各字段格式校验全部在有效体上成立。

### 2.2 代码改动（`model/iprovider/provider.go`，集中在一个函数 + 一个 helper）

**① 抽取 model_endpoint 默认值填充 helper**（保留既有副作用）：

```go
// fillProviderEndpointDefaults fills schema/URI defaults on a non-nil endpoint.
func fillProviderEndpointDefaults(ep *ProviderEndpoint) {
	if ep == nil {
		return
	}
	if ep.Schema == "" {
		ep.Schema = "https"
	}
	if ep.URI == "" {
		ep.URI = "/v1/models"
	}
}
```

`ValidateProviderParam` 内 :534-546 改为调用它（行为不变）。**必要性**：现行 `ValidateProviderParam` 会原地改写 `param.ModelEndpoint` 的缺省子字段，该副作用被 PATCH 路径依赖（`PATCH {"model_endpoint":{"uri":"/x"}}` 的 schema 缺省值会持久化）；合并校验作用于副本后，必须由 `UpdateProvider` 在合并前显式对 `param.ModelEndpoint` 调同一 helper，行为才不回退。

**② `UpdateProvider` 重排**（:230-334）：

```go
func (m *ProviderManager) UpdateProvider(ctx context.Context, name string,
	param *ProviderParam, syncHooks ...func(ctx context.Context, oldProvider, newProvider *Provider) error) error {

	var oldProvider *Provider
	err := m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		existing, err := m.storager.FetchProvider(ctx, &ProviderFilter{Name: &name})
		if err != nil {
			return err
		}
		if existing == nil {
			return xerror.WrapRecordNotExist("provider")
		}
		oldProvider = existing

		// Preserve the historical default-fill side effect for an explicitly
		// provided model_endpoint (previously applied by ValidateProviderParam
		// directly on param).
		fillProviderEndpointDefaults(param.ModelEndpoint)

		// Partial-update semantics (providers.md §2.4, issue #322): validate the
		// effective body — the PATCH param with omitted required/consistency
		// fields fallen back to the stored values — instead of the raw partial
		// body. Explicitly provided values (including empty slices) are still
		// validated as-is, so create-time guards keep firing.
		effective := *param // shallow copy; ValidateProviderParam's mutations
		                    // (endpoint defaults) already applied above
		if effective.InstanceSource == nil && existing.InstanceSource != "" {
			source := existing.InstanceSource
			effective.InstanceSource = &source
		}
		if effective.K8sPoolName == nil {
			effective.K8sPoolName = existing.K8sPoolName
		}
		if effective.ModelProtocols == nil {
			effective.ModelProtocols = existing.ModelProtocols
		}
		if effective.ProtocolPaths == nil {
			effective.ProtocolPaths = existing.ProtocolPaths
		}
		if effective.InstancePool == nil {
			effective.InstancePool = existing.InstancePool
		}
		if err := ValidateProviderParam(&effective); err != nil {
			return err
		}

		// ... 以下不变：origX 快照捕获（:262-269）、storager.UpdateProvider、
		//     hook 触发判定与快照（:271-323）
	})
	// ... 错误/成功 operation log 不变（:327-334）
}
```

要点：

1. **删除** :233-237 的预校验（先校验后合并的根因）与 :250-260 的旧「有效 paths」块（被合并校验完全取代，且修正其「paths 回落、protocols 不回落」的半吊子语义——这正是 :6 发现的隐藏陷阱）。
2. **只回落「必填/一致性」字段**（InstanceSource/K8sPoolName/ModelProtocols/ProtocolPaths/InstancePool）；`Models`/`Keys`/`Tiers` 等可选字段不回落——其校验（如 `models` 显式空数组拒绝，PV-4-016）仍只作用于显式提供的值，**存量为空的 legacy Provider（创建时未填 models，DB 存 `[]`）做 description-only PATCH 不会被新增 422**。
3. `InstanceSource` 回落仅在存量非空时进行：pre-`instance_source` 时代的存量行该字段为空字符串，留 nil 走 `validateProviderInstanceSource` 的缺省（instance_pool 分支）+ InstancePool 回落，行为正确。
4. 校验仍在 `AtomExecute` 事务内、storager 写入之前：校验失败即回滚，**无部分写入**（验收判据 4）；operation-log 失败记录路径（:327-330）不变。
5. hook 触发逻辑（origX 快照 + `clusterConfigChanged` 判定 + `applyProviderUpdate` 快照）**完全不动**——显式提供才触发同步的语义保留，模式切换校验（k8s_pool 缺 `k8s_pool_name` → 422）经有效体自然成立。
6. Create 路径（`CreateProvider` :184 直接调 `ValidateProviderParam`）**零改动**，POST 必填守卫不回归。

### 2.3 不改动的部分

| 项 | 说明 |
|----|------|
| `endpoints/openapi_v1/provider/update.go` | 绑定/拒绝 body `name`/URI 补名逻辑不动 |
| `applyProviderUpdate` / hook 快照 / `clusterConfigChanged` 判定 | 已正确，不动 |
| storager `toDAOParamForUpdate` / DAO nil-skip | #147 的成果，不动；写入仍按「显式字段」语义 |
| `ValidateProviderParam` 各字段规则 | 创建/更新共用校验器，规则不动 |
| `providers.md` 等 API 定义文档 | §2.4 条款本就承诺部分更新语义，纯实现缺口，无契约修订 |
| 前端 | Provider 编辑表单本就回显全量字段提交，合并语义下行为不变 |

### 2.4 修复后行为对照

| 请求 | 修复前 | 修复后 |
|------|--------|--------|
| PATCH 仅 `{"description":"updated"}`（静态） | 422 `instance_pool is required` | **200**，仅 description 变化，池/协议保持原值 |
| PATCH 仅 `{"description":"updated"}`（K8s） | 422 `model_protocols is required` | **200** |
| K8s PATCH 仅 `{instance_source, k8s_pool_name, k8s_instance_pool}`（已删字段） | 422 `model_protocols is required` | **200**，未知字段静默忽略，协议保持原值 |
| PATCH 显式 `instance_pool: []`（静态） | 422 | 422（**不变**，守卫回归臂） |
| PATCH 显式 `model_protocols: []` | 422 | 422（不变） |
| PATCH 显式空 `models: []` | 422（PV-4-016） | 422（不变） |
| PATCH 切 `instance_source=k8s_pool` 但缺 `k8s_pool_name` | 422 | 422（不变，模式切换守卫） |
| PATCH 提供 `protocol_paths` 但未声明协议 | 422 | 422（不变，对有效协议集判定） |
| 存量空 models 的 legacy Provider 做 description-only PATCH | 422（必须补全字段） | **200**（Models 不回落，校验跳过） |
| PATCH `{"model_endpoint":{"uri":"/v2/models"}}` | 200，schema 缺省 https 持久化 | 200，行为不变（副作用保留） |
| POST 创建缺 `instance_pool`/`model_protocols` | 422 | 422（不变） |

## 3. 测试计划

### 3.1 单元测试（`model/iprovider/provider_test.go` 新增 `TestProviderManager_UpdateProvider_PartialUpdate`）

fake storager（既有 `fakeProviderStorager` 模式）fetch 返回存量 Provider，update 回调记录收到的 param：

| 用例 | 存量 | PATCH param | 断言 |
|------|------|-------------|------|
| description-only（静态） | 完整静态 Provider | 仅 Description | NoError；storager 收到的 param.Models/Keys 为 nil（nil-skip 语义不被合并污染） |
| description-only（K8s） | k8s_pool Provider（含 protocols） | 仅 Description | NoError |
| legacy 空 models | Models 为空的存量 | 仅 Description | NoError（Models 不回落，不新增 422） |
| 显式空 instance_pool | 静态 | `InstancePool: []` + 全量其余 | Error，消息含 `instance_pool` |
| 显式空 model_protocols | 静态 | `ModelProtocols: []` + 其余 | Error |
| 模式切换缺名 | 静态 | `InstanceSource: k8s_pool`，无 K8sPoolName | Error 含 `k8s_pool_name` |
| endpoint 缺省副作用 | 静态 | ModelEndpoint 仅 URI | NoError；storager param.ModelEndpoint.Schema == "https" |
| paths 省略回归 | 存量带 ProtocolPaths + 对应 protocols | 仅 Description | NoError（旧 :254-260 半回落逻辑的陷阱用例） |
| paths 指向未声明协议 | 静态 protocols=[openai] | ProtocolPaths={anthropic: /x} + 全量其余 | Error 含 `not declared` |
| Provider 不存在 | — | 任意 | 404（WrapRecordNotExist 路径不变） |

同时回归运行既有 `TestProviderManager_UpdateProvider_InstanceSourceHook`（4 个 hook 用例）与 `operation_log_test.go` 的 Update 用例（该用例传全量字段，合并后校验应照常通过）。

### 3.2 集成测试（`test/integration/tests/provider/partial_update/partial_update_test.go`）

**移除测试放大器**：`requiredPatchBody()`（:42-47）是 #147 时代为绕过本 bug 打的补丁——每个 PATCH 都重复携带 `instance_pool`+`model_protocols`。修复后删除该 helper，全部用例改为真正的部分更新（请求体只含被测字段）：

| 用例 | 改动 |
|------|------|
| PV-4-007 省略字段保留原值 | body 只含 `{"description"}`；断言扩展：`instance_pool`/`model_protocols` 回读保持原值（本次修复的核心验收） |
| PV-4-008 只更新 time_zone | body 只含 `{"time_zone"}` |
| PV-4-009 显式空数组清空 keys/tiers | body 只含 `keys:[]`+`tiers:[]` |
| PV-4-010 非法 time_zone | body 只含非法 `time_zone` |
| PV-4-016 显式空 models 拒绝 | body 只含 `models:[]` |
| **新增 PV-4-017 K8s 部分更新** | 经 inner API `PUT /inner-api/v1/k8s_pools/{pool}/instances` 建池 → 创建 k8s_pool Provider（带 model_protocols）→ PATCH 仅 `{instance_source, k8s_pool_name, k8s_instance_pool:[...]}`（已删字段）→ **200**；回读 model_protocols 保持原值；cleanup 删 Provider+池 |

执行：重建 `ai-gateway-api.exe` 后 `go test ./tests/provider/partial_update/... ./tests/provider/update/... ./tests/provider/k8s_pool/...`（SQLite 默认；合并/校验在 model 层，与后端无关）。

### 3.3 E2E / 验收判据

对照 issue 五条验收判据，requeue SC1601-TC003 / SC1601-TC009：

1. 静态 Provider PATCH 仅 `{"description":"updated"}` → 200 且未提供字段保持原值（A：最小复现第 3 步）；
2. K8s Provider PATCH 仅 `{instance_source, k8s_pool_name, k8s_instance_pool}` → 200，model_protocols 保持原值，未知字段不生效（A-k8s-provider-crud）；
3. POST 必填守卫不回归（缺 instance_pool/model_protocols 仍 422——集成 PV-1-004/005 系既有守护）；
4. 模式切换守卫不回归（切空池/缺 k8s_pool_name 仍 422），失败无部分写入；
5. SC1601-TC003 `k8s-provider-crud` 与 SC1601-TC009 `provider-patch-no-mirror` 真跑 PASS，PATCH 后 `cluster_table` 无镜像实例（hook 语义未被合并校验扰动）。

## 4. 兼容性、迁移与风险

| 项 | 结论 |
|----|------|
| 行为变化 | PATCH 从「必须携带创建必填字段」放宽为「真部分更新」——**恢复 §2.4 承诺语义**；曾依赖「部分体 422」的调用方无正当场景（该行为即本 bug）， workaround（重复提交全量字段）在修复后行为不变、继续可用 |
| 响应结构 | 无变化 |
| 数据迁移 / DDL | 无；校验仍在写入前，失败不落库 |
| 存量数据 | 存量 Provider 无需迁移；校验只读存量做回退，不写回 |
| 与 #147 的关系 | #147 修「省略被覆盖」、遗留「必填逼传」；本次修后者，两者合起来才构成完整的 §2.4 部分更新语义。#147 文档的范围声明（「instance_pool/model_protocols 不在本次范围」）由本文档 supersede |
| 回滚 | 单 commit 还原即可 |

## 5. 关联风险与后续事项

1. **测试放大器清理是修复的必要组成**：`partial_update` 套件的 `requiredPatchBody()` 与本 bug 共生——不删除它，修复后集成层仍无「真部分更新」守护，回归无法被拦截（与 #316 ET-5-002 / #319 同型教训）。
2. **同模式巡检**：全仓 grep `ValidateProviderParam` 的直接调用方仅 Create/Update 两处（provider.go:184/233 改造点）；其余 PATCH 类接口（entity、cluster、api_key 等）的「先校验后合并」顺序值得按同标准巡检一轮，但无第二处实锤，列为观察项不阻塞本修复。
3. **`:250-260` 旧块的语义修正属顺带修复**：其「paths 回落、protocols 不回落」组合在「省略 protocols 且存量有 paths」场景下会产生误判；合并校验取代后该场景正确放行。此场景无独立 issue，由本修复的单测用例钉住。
4. QA requeue SC1601-TC003 / SC1601-TC009 转 PASS 后关闭 issue #322，并解除对下游 deployment gate 的阻塞。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 根因源码直读核实（预校验 :233 → instance_pool :880 / model_protocols :513 + 合并器 :1102 现成可用 + :250-260 半回落陷阱）
- [x] 影响面排查（#147 历史范围、ModelEndpoint 副作用、legacy 空 models、hook 语义、Create 路径）
- [x] 代码实现（`UpdateProvider` 重排 + `fillProviderEndpointDefaults` 抽取）
- [x] 单元测试（新增 `TestProviderManager_UpdateProvider_PartialUpdate` 10 用例 + 既有 hook/operation-log 用例回归）
- [x] 集成测试（删除 `requiredPatchBody()` 放大器，PV-4-007..016 改真部分更新，新增 PV-4-017 K8s 变体）
- [x] 验证：`go test ./model/iprovider/...` 通过；集成测试（重建 `ai-gateway-api.exe` 后）`go test -count=1 ./tests/provider/...` 12 个套件全过；`model` 覆盖率 85.4% 过 70% 门禁；全量 `go test ./...` 零失败
- [x] Step 3/4（api-define / sys-design）核对：§2.4 条款本就承诺部分更新，文档无需修订
- [ ] QA requeue SC1601-TC003 / SC1601-TC009 转 PASS 并关闭 issue #322

## 7. 实施校准记录（实现过程中确认的事实与取舍）

1. **单测守护方向验证**：`description_only_on_static_provider` / `description_only_on_k8s_pool_provider` 在修复前代码上实测失败（旧预校验对 partial body 抛 422），修复后 10/10 通过；`explicit_empty_*` 与 `mode_switch_*` 守卫臂在修复前后均拒绝，证明「合并校验」没有放松创建守卫。
2. **集成旧二进制复现**：临时还原 `provider.go` 并重建 exe 后，PV-4-007 报 `expected ErrNum=200, got ErrNum=422, ErrMsg=Param Illegal: instance_pool is required`——与 issue 最小复现逐字一致；恢复修复重建后 12 个 provider 套件（含 k8s_pool/instance_pool_sync，验证 hook 语义未被扰动）`-count=1` 强制重跑全过。注意：集成测试走子进程二进制，go test 缓存不感知 exe 变化，回归验证须 `-count=1` 或对相关套件强制重跑。
3. **`:250-260` 旧块移除的验证**：`omitted_paths_and_protocols_keep_stored_consistency` 用例（存量带 `ProtocolPaths` + PATCH 省略 protocols）在修复后通过——若保留旧块（paths 回落、protocols 取 nil）该用例会误报 `not declared`；合并校验取代后语义正确。
4. **副作用保留的验证**：`model_endpoint_default_fill_side_effect_preserved` 断言 storager 收到的 `param.ModelEndpoint.Schema == "https"`（PATCH 只给 URI 时）——证明 `fillProviderEndpointDefaults` 在合并前对 raw param 的调用完整保留了原 `ValidateProviderParam` 原地改写的持久化行为。
