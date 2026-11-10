# Issue #321 变更摘要：Provider `instance_pool[].addr` 未做 Hostname 校验（非法地址静默持久化）

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#321](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/321)（SC2101-TC023：`addr="-bad"` 的 Provider 创建返回 2xx 并持久化，契约要求 400/422）

## 1. 背景

### 1.1 问题现象

`POST /open-api/v1/providers` 创建 `instance_source=instance_pool` 的 Provider 时，`instance_pool[0].addr="-bad"`（标签以连字符开头）返回 **2xx / code=200** 并被持久化（cleanup 需 DELETE 删除，residue 检查证实已落库）；同请求矩阵其余 10 组非法 payload（空池、全零权重、重复 (addr,port)、端口 0/65536、权重 -1/101、缺 addr/port）均被正确拒绝 400/422。后果：违反契约的非法后端地址可被静默写入实例池，若随 `cluster_table` 下发将进入数据面配置。同形态失败指纹跨 6 次 real attempt（09-16 ×4、10-09、11-09）复现，非瞬态、非新回归。

### 1.2 根因核实结论（源码直读，与 issue 定位一致）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `model/iprovider/provider.go:878-907` | `validateProviderInstancePool` 只校验：池非空（:879-881）、addr 去空格非空（:885-887）、port ∈ [1,65535]（:888-890）、weight ∈ [0,100]（:891-893）、(addr,port) 去重（:897-901）、至少一个 weight>0（:903-905）——**无 addr 的 Hostname 语法校验**，`"-bad"` 因此通过 | ✅ 实锤（直读） |
| 2 | `model/iprovider/provider.go:184,233` | `CreateProvider`/`UpdateProvider` 均调用 `ValidateProviderParam`（:491）→ `validateProviderInstanceSource`（:508）→ `validateProviderInstancePool`（:833）：校验缺口同时覆盖创建与更新路径 | ✅ 实锤（直读） |
| 3 | `lib/validate/validate.go:87-110` | `validate.Hostname` 已存在且与 00-common §1 逐条对齐：RFC 1123（标签 1-63、`[a-zA-Z0-9-]`、不得以 `-` 开头结尾）、总长 ≤255、**总长 ≥2（系统特殊要求）**、接受 IPv4/IPv6 字面量（`net.ParseIP` 短路） | ✅ 实锤（直读） |
| 4 | `lib/validate/validate.go:1265-1314` | 同仓既有 `validate.Instance`/`InstancePool`（cluster 后端池用）**已对每个 addr 调 `Hostname`**——校验族存在且已在 cluster 路径强制，唯 provider 路径未接入 | ✅ 实锤（直读） |
| 5 | `storage/rdb/provider/provider.go:43-56,171-226` | 校验通过后 `toDAOParam` 把 `instance_pool` 原样序列化落库（:195-198）→ 非法地址被持久化 | ✅ 实锤（直读） |

**排除项**：

- 「校验器不存在」——`validate.Hostname` 存在且规则与条款逐条对齐（含「≥2 字符」系统特殊要求），cluster 后端池路径（`validate.Instance`，validate.go:1272）已在用；
- 「环境/请求构造问题」——同矩阵 10 组对照臂全部正确 4xx；
- 「合法用例会受误伤」——现存所有单测/集成测试的 addr 均为多字符 hostname 或 IP（`api.deepseek.com`、`10.0.0.1` 等），`Hostname` 的 ≥2 字符下限不影响任何既有用例；
- 「k8s_pool 模式会误伤」——`validateProviderInstancePool` 仅在 `instance_source=instance_pool` 分支被调用（provider.go:832-835），k8s_pool 模式的 dormant instance_pool 不做成员校验（有既有单测钉死该行为，provider_instance_source_test.go:92-101），不受影响。

### 1.3 关键阻碍：直接复用 `validate.Hostname` 存在 import cycle

`model/iprovider` **不能直接 import `lib/validate`**，因为：

```
lib/validate ──imports──▶ model/icluster_conf（validate.Instance 的参数类型，validate.go:32）
model/icluster_conf ──imports──▶ model/iprovider（providerStorager，cluster.go:47）
model/iprovider ──(若 import lib/validate)──▶ 形成环，编译失败
```

这是本修复必须先解除的结构性阻碍，也是 issue 中「API 层已有 hostname 校验 helper，复用为唯一实现（待确认是否存在）」的确认结果：**helper 存在，但被依赖方向卡住**。

## 2. 修复方案

### 2.1 第一步：解除 lib/validate → model/icluster_conf 依赖（搬移 Instance/InstancePool 与 LLMConfig 校验）

**实施校准（对 §1.3 计划的勘误）**：实现时发现 `lib/validate` 依赖 `model/icluster_conf` 的函数有**两个**，不止方案设计时盘点的 `Instance`/`InstancePool`——`LLMConfig`（validate.go:1094-1258，校验 `*icluster_conf.LLMConfig`）同样依赖，且生产调用点 2 处（`product_cluster/create.go:131`、`product_cluster/checker.go:37`）。两个函数一并迁出后，`lib/validate` 对 `model/icluster_conf` 的依赖彻底解除（`bfe_basic` import 随之移除），`model/iprovider` 得以 import `lib/validate`。

改动：

1. **新建 `model/icluster_conf/instance_validate.go`**：移入 `MaxInstanceNameLength`、`Instance`、`InstancePool` 三个符号，**重命名**为 `ValidateInstance` / `ValidateInstancePool`（`icluster_conf` 包内已有 `type Instance struct`（pool.go:76），函数与类型不能同名）。内部对 `Hostname`/`Port` 的调用经新增的 `lib/validate` import 完成（方向：model → lib，合法）。
2. **新建 `model/icluster_conf/llm_config_validate.go`**：`LLMConfig` 迁入并改名 `ValidateLLMConfig`（包内已有 `type LLMConfig struct`（cluster.go:250），同名冲突）；函数体仅依赖 xerror/strings/bfe_basic，迁出时同步把 `bfe_basic` import 从 lib/validate 移除。
3. **`lib/validate/validate.go`**：删除上述全部符号与 `model/icluster_conf`、`bfe_basic` import（`model/shared` import 保留，无环）。`TestInstancePool`、`TestLLMConfig`、`TestLLMConfigEnsureReasoningContent` 同步迁往 `model/icluster_conf` 测试文件并适配新函数名/包内类型名。
4. **调用点更新**：`product_pool/create.go:94` → `icluster_conf.ValidateInstancePool(...)`；`product_cluster/create.go:131`、`checker.go:37` → `icluster_conf.ValidateLLMConfig(...)`（checker.go 的 `lib/validate` import 随之移除）。

搬移后的依赖图（无环，已编译验证）：

```
lib/validate ──▶ model/shared（叶子）           model/icluster_conf ──▶ lib/validate, model/iprovider
model/iprovider ──▶ lib/validate（新增，可行）
```

### 2.2 第二步：provider 校验接入 Hostname（issue 本体，单行级）

`model/iprovider/provider.go` `validateProviderInstancePool` 的 per-instance 循环内（现有 addr 非空检查处）：

```go
// 修复前
if strings.TrimSpace(inst.Addr) == "" {
    return xerror.WrapParamErrorWithMsg("instance_pool[%d]: instance addr is required", i)
}

// 修复后
if err := validate.Hostname(inst.Addr); err != nil {
    return xerror.WrapParamErrorWithMsg("instance_pool[%d]: instance addr: %v", i, err)
}
```

要点：

1. `validate.Hostname` 自带空串拒绝（`hostname is required`），原 TrimSpace 非空检查被完全覆盖（含纯空白 addr——`Hostname` 的标签正则会拒绝空白字符），**不保留双重检查**。
2. `Hostname` 接受 IPv4/IPv6 字面量（`net.ParseIP` 短路，validate.go:100-102），与 00-common §1「或有效的 IPv4/IPv6 地址」一致——`1.2.3.4`、`2001:db8::1` 形式的 addr 保持合法。
3. 错误消息格式与同函数其它检查一致（`instance_pool[%d]: ...`），经 `WrapParamError` → **HTTP 422**（issue 期望 400/422，本体系参数错误统一 422，与同矩阵其余 10 组的实际返回一致）。
4. 仅作用于 `instance_source=instance_pool` 分支；k8s_pool 模式的 dormant instance_pool 与 K8s 发现组件经 `/k8s_pools` InnerAPI 写入的实例（K8s Service 地址，单一事实源在 k8s_pools）均不经过此校验，行为不变。
5. 创建（`CreateProvider`）与更新（`UpdateProvider`）共用 `ValidateProviderParam`，一处修复两条路径全覆盖。

### 2.3 不改动的部分

| 项 | 说明 |
|----|------|
| `validate.Hostname` 规则本身 | 与 00-common §1 逐条对齐，不动 |
| cluster 后端池校验（`validate.Instance` 搬移后） | 逻辑逐字不变，只是换了包名/函数名 |
| k8s_pool 模式 / k8s_pools InnerAPI | 不经过本校验，不动 |
| `storage/rdb/provider` 序列化 | 校验在 model 层入口拦截，合法数据流不变 |
| `providers.md` / `00-common.md` | 条款本就要求 addr 为 Hostname + 创建时校验 instance_pool 合法——纯实现缺口，无契约修订 |
| 前端 | Provider 表单不构造非法 addr 场景，错误提示走通用 ErrMsg，零改动 |
| i18n | 复用 `Hostname` 既有错误消息（`hostname %q contains invalid label %q` 等），无新增词条需求 |

### 2.4 修复后行为对照

| 请求 | 修复前 | 修复后 |
|------|--------|--------|
| `addr="-bad"`（issue 复现值） | 2xx 并持久化 | **422**，不持久化 |
| `addr="bad-"` / `"b..d"`（空标签）/ 64 字符标签 / 总长 >255 | 2xx | **422** |
| `addr="a"`（单字符，违反系统 ≥2 要求） | 2xx | **422** |
| `addr="api.example.com"` / `"1.2.3.4"` / `"2001:db8::1"` | 2xx | 2xx（不变，对照臂） |
| 同矩阵其余 10 组非法 payload | 400/422 | 400/422（不变） |
| k8s_pool 模式 dormant instance_pool 成员非法 | 通过（dormant） | 通过（不变） |
| 合法 Provider 创建/回读/cluster_table 导出 | 正常 | 正常（不变） |

## 3. 测试计划

### 3.1 单元测试（model/iprovider）

在 `provider_instance_source_test.go` 新增 `TestValidateProviderParam_InstancePoolHostname`（表驱动，走 `ValidateProviderParam` 全链路）：

| 用例组 | addr | 期望 |
|--------|------|------|
| issue 复现值 | `-bad` | Error，消息含 `instance addr` |
| 同族非法 | `bad-`、`b..d`、64 字符标签、256 字符总长、`a`（单字符）、`bad addr`（空白）、`bad_addr`（下划线不在字符集） | Error |
| 对照臂（合法） | `api.example.com`、`a1`、`1.2.3.4`、`2001:db8::1` | NoError |

同时保留并确认既有用例不回退：`TestValidateProviderParam_InstanceSource` 全量（含「k8s_pool 模式不校验 dormant instance_pool 成员」：92-101）。

### 3.2 单元测试（搬移部分）

- `model/icluster_conf` 新增 `instance_validate_test.go`：迁入 `TestInstancePool` 原有用例，适配新函数名 `ValidateInstancePool`，并补 `ValidateInstance` 直接用例。
- `lib/validate/validate_test.go`：删除已迁走的用例；`TestHostname` 等保留不动。
- 方向性验证：迁移后的 cluster 后端池校验对非法 addr（如 `-bad`）仍拒绝（行为不变的守护）。

### 3.3 集成测试（`test/integration/tests/provider/create/create_test.go`）

在既有 case 矩阵（PV-1-004..PV-1-019 风格）追加，**不新建场景目录**：

| 用例 | body | 期望 |
|------|------|------|
| PV-1-022 instance addr 为非法 hostname | `minProviderBody` 基础上 `instance_pool[0].addr="-bad"`（双实例 weight 0/100，对齐 E2E 复现形态） | 422 |
| PV-1-023 instance addr 尾随连字符 | `addr="bad-"` | 422 |
| 非持久化守护 | PV-1-022 之后 `GET /open-api/v1/providers/{name}` | **404**（验收判据 1：非法 Provider 未持久化） |
| 对照臂 | 既有 PV-1-001/002 系（合法 addr → 200） | 不变 |

执行：重建 `ai-gateway-api.exe` 后 `go test ./tests/provider/create/...`（SQLite 默认；校验在 model 层入口，与后端无关）。更新路径与创建共用 `ValidateProviderParam`，由单测覆盖即可，不在集成重复建用例。

### 3.4 E2E / 验收判据

对照 issue 四条验收判据，requeue SC2101-TC023：

1. `addr="-bad"` 创建返回 4xx（实际 422）且未持久化（GET 404 / 列表无此项）；
2. 同族非法形态（尾随连字符、连续连字符导致的空标签、超长标签）同样 4xx；合法 hostname 与 IP 字面量仍 2xx；
3. 其余 10 组非法 payload 仍 4xx；合法 Provider 创建/回读/`cluster_table` 导出不回归；
4. SC2101-TC023 越过 create-guards，到达 name-default 与 `cluster_table` 导出断言，`ai-gateway-api#105` 的 `DEPLOYED_PROVIDER_INSTANCE_NAME_DEFAULT` deployment gate 可翻绿。

## 4. 兼容性、迁移与风险

| 项 | 结论 |
|----|------|
| 行为变化 | 非法 addr 的创建/更新由「2xx 并持久化」收紧为 422——契约早已声明 addr 为 Hostname，属纠偏；合法请求零影响 |
| 已持久化的非法数据 | 存量 Provider 若已含非法 addr，本次修复不影响其读取/导出（校验只在写入入口）；如需清理属运营动作，不在本修复范围 |
| 数据迁移 / DDL | 无 |
| 包结构变化 | `Instance`/`InstancePool` 校验从 lib/validate 迁至 model/icluster_conf 并改名——**对外（HTTP 契约）无感**，仅内部符号迁移，调用点仅 1 处已盘点 |
| 模型覆盖率门禁 | model/ 覆盖率 ≥70%：`icluster_conf` 迁入函数带测试随迁，iprovider 新增分支有表驱动用例覆盖，预期不回退 |
| 回滚 | 单 commit 还原即可 |

## 5. 关联风险与后续事项

1. **依赖方向规范化收益**：本次搬移后 `lib/validate` 不再依赖任何重 model 包（仅剩叶子 `model/shared`），后续任何 model 包均可直接复用 `Hostname`/`IPAddress`/`Port` 等基础校验器，避免再次遭遇 cycle。可在 `sys-design/details/` 沉淀一条 lib 分层约定（lib/validate 不得依赖 model/*），防止回退。
2. **单字符 addr 收紧的说明**：`Hostname` 的系统特殊要求「总长 ≥2」意味着 `addr="a"` 从合法变为非法——这与 00-common §1 条款一致（条款明载该要求），且无任何既有测试/集成用例使用单字符 addr；E2E 基线使用的受控后端服务地址均为合法 hostname/IP，不受影响。
3. **同类巡检**：provider 路径补齐后，「写入实例池类配置」的入口校验已全量覆盖（provider instance_pool ← 本次修复；cluster 后端池 ← 既有 `validate.Instance`；k8s_pools ← K8s 发现单一事实源）。issue 引用的同族先例 #172/#173（cluster passive_health_check / max_idle_conns 校验）已在各自修复中收口，无新增同型缺口。
4. QA requeue SC2101-TC023 转 PASS 后关闭 issue #321，并解锁 #105 的 deployment gate。

## 6. 状态

- [x] 修复方案设计（本文档）
- [x] 根因源码直读核实（`validateProviderInstancePool` 六规则 vs Hostname 缺口 + `validate.Hostname` 规则对照 + 落库链）
- [x] import cycle 结构性阻碍确认（lib/validate → icluster_conf → iprovider）与解除方案设计
- [x] 影响面排查（创建/更新双路径、k8s_pool dormant 语义、既有 addr 用例全量、cluster 后端池既有校验）
- [x] 代码实现（搬移 Instance/InstancePool + LLMConfig → icluster_conf + provider 接入 validate.Hostname）
- [x] 单元测试（iprovider hostname 表驱动用例 + icluster_conf 迁移测试 + lib/validate 清理）
- [x] 集成测试（provider/create 补 PV-1-022/023 + GET 404 非持久化守护）
- [x] 验证：`go test ./lib/validate/... ./model/... ./endpoints/...` 通过；集成测试（重建 `ai-gateway-api.exe` 后）`go test ./tests/provider/...` 12 个套件全过；model 覆盖率 85.3% ≥ 70% 门禁；全量 `go test ./...` 零失败
- [x] Step 3/4（api-define / sys-design）核对：条款本就要求 Hostname，文档无需修订；考虑沉淀 lib 分层约定文档
- [ ] QA requeue SC2101-TC023 转 PASS 并关闭 issue #321

## 7. 实施校准记录（实现过程中确认的事实与取舍）

1. **计划勘误——LLMConfig 也是依赖方**：方案设计时盘点「Instance/InstancePool 是 lib/validate 依赖 icluster_conf 的唯一原因」，编译验证时发现 `LLMConfig` 同样依赖（`validate.go:1094` 的参数类型 `*icluster_conf.LLMConfig`）。已按同一模式一并迁出（改名 `ValidateLLMConfig`，2 处调用点更新），lib/validate 对 icluster_conf 的依赖彻底解除、`bfe_basic` import 随之移除。教训：**盘点 lib→model 依赖应以编译器为准**（`go build` 立即暴露），不能只 grep import 声明后推断符号用途。
2. **单测守护方向验证**：`TestValidateProviderParam_InstancePoolHostname` 在修复前代码上实测失败——8 个非法 addr 用例全部「error expected but got nil」（即 issue 的静默接受），4 个合法对照臂通过；修复后全绿。回归方向确认无误。
3. **集成旧二进制复现**：临时还原 `provider.go` 并重建 exe 后，PV-1-022 报 `expected ErrNum=422, got ErrNum=200` 且回读 GET 200（**已持久化**）——与 issue 运行证据（2xx + cleanup 删除证实落库）完全一致；恢复修复重建后 `go test ./tests/provider/...` 12 个套件全部通过（含 k8s_pool、instance_pool_sync 等关联套件，证明搬移 LLMConfig/Instance 校验未影响 cluster 路径）。
4. **覆盖率确认**：`model/` 总覆盖率 85.3%（阈值 70%），迁入 icluster_conf 的函数带测试随迁，无回退。
