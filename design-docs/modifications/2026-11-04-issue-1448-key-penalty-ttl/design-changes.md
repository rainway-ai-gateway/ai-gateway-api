# 设计变更说明：Key 惩罚时长可配置（控制面）

## 1. 数据流与默认值分层

```
Web / OpenAPI 请求（penalty_ttl_429 / penalty_ttl_auth）
  └─ POST/PATCH /open-api/v1/clusters
      └─ UpsertParam.Validate()            ← 校验层：显式值必须落在区间内（nil 放行）
          └─ normalizeKeyAffinity()        ← 默认值层 1（控制面入库前，nil → 60/300）
              └─ DB: clusters.llm_config（JSON 文本列，无 DDL）
                  └─ newAIConf()           ← 默认值层 2（控制面导出前，derefInt 兜底）
                      └─ InnerAPI server_data_conf: KeyPolicy.SessionAffinityPenaltyTTL{429,Auth}
                          └─ conf-agent → cluster_conf.data
                              └─ AIKeyPolicyCheck()   ← 数据面层（bfe，0 → 60/300）
                                  └─ defaultAIKeyPolicy()（bfe，KeyPolicy == nil 时的兜底）
```

控制面两层必须同值（60/300），且与数据面默认值一致；任何一层漏改都会出现「界面显示 300、实际 3600」或「写 0 = 永不过期」。

## 2. 模型与字段映射

### 2.1 `model/icluster_conf/cluster.go:223-228`

```go
type KeyAffinity struct {
	Enabled       *bool   `json:"enabled"`        // default true
	TTL           *int    `json:"ttl"`            // idle timeout in seconds, default 600
	RedisPrefix   *string `json:"redis_prefix"`   // default "bfe:ai:key_affinity"
	PenaltyEnable *bool   `json:"penalty_enable"` // default true
	// Penalty TTL (seconds) written when an upstream key becomes unusable.
	// nil means "unset" and is filled with the default; an explicit value out of
	// range is rejected by validation. 0 is NOT "disabled".
	PenaltyTTL429  *int `json:"penalty_ttl_429"`  // upstream 429, default 60
	PenaltyTTLAuth *int `json:"penalty_ttl_auth"` // upstream 401/402/403, default 300
}
```

### 2.2 导出默认值字面量（`cluster.go:1685-1694`，`newAIConf` 内）

```go
	aiConf.KeyPolicy = &cluster_conf.AIKeyPolicy{
		...
		SessionAffinityPenaltyEnable:  true,
		SessionAffinityPenaltyTTL429:  60,
		SessionAffinityPenaltyTTLAuth: 300,
	}
```

作用：`llm_config.key_affinity == nil` 的存量集群也导出具体值，不把 0 传给数据面。

### 2.3 导出映射（`cluster.go:1701-1706`）

```go
	if llmConfig.KeyAffinity != nil {
		...
		aiConf.KeyPolicy.SessionAffinityPenaltyTTL429 = derefInt(llmConfig.KeyAffinity.PenaltyTTL429, 60)
		aiConf.KeyPolicy.SessionAffinityPenaltyTTLAuth = derefInt(llmConfig.KeyAffinity.PenaltyTTLAuth, 300)
	}
```

`derefInt` 见 `cluster.go:1735-1740`。字段值来自 BFE 类型 `cluster_conf.AIKeyPolicy`，因此需先提升依赖（§3）。

### 2.4 入库前默认值（`endpoints/openapi_v1/product_cluster/create.go:461-483`）

```go
	if affinity != nil {
		...
		rst.PenaltyEnable = affinity.PenaltyEnable
		rst.PenaltyTTL429 = affinity.PenaltyTTL429
		rst.PenaltyTTLAuth = affinity.PenaltyTTLAuth
	}
	...
	if rst.PenaltyTTL429 == nil {
		rst.PenaltyTTL429 = lib.PInt(60)
	}
	if rst.PenaltyTTLAuth == nil {
		rst.PenaltyTTLAuth = lib.PInt(300)
	}
```

`normalizeLLMConfig`（`create.go:436-459`）把 `KeyAffinity` 整体交给该函数（`:446`），无需加白名单；更新路径 `update_basic.go:138` 复用同一份归一逻辑，也不用重复改。**注意该函数逐字段重建结构体，新字段不加在这里会被静默丢弃。**

### 2.5 校验（`lib/validate/validate.go:1153-1161`）

```go
	if c.KeyAffinity != nil {
		...
		if c.KeyAffinity.PenaltyTTL429 != nil &&
			(*c.KeyAffinity.PenaltyTTL429 < 10 || *c.KeyAffinity.PenaltyTTL429 > 600) {
			return xerror.WrapParamErrorWithMsg("llm_config.key_affinity.penalty_ttl_429 must be between 10 and 600")
		}
		if c.KeyAffinity.PenaltyTTLAuth != nil &&
			(*c.KeyAffinity.PenaltyTTLAuth < 60 || *c.KeyAffinity.PenaltyTTLAuth > 1800) {
			return xerror.WrapParamErrorWithMsg("llm_config.key_affinity.penalty_ttl_auth must be between 60 and 1800")
		}
	}
```

`UpsertParam.Validate`（`create.go:109-135`）在 `xreq.Bind*` 阶段执行，早于 `normalizeLLMConfig`，因此：

- 字段缺省（nil）→ 校验放行 → 入库前补默认；
- 显式 0 / 负数 / 越界 → 422，不落库。

区间已隐含拒绝 0，不再单写 `< 0` 分支。

## 3. 依赖提升

`AIKeyPolicy` 类型来自 BFE（`go.mod:92` 的 fork replace）。数据面已把两个字段合入 `v1.9.0-dev`，fork 提交 **`904ceab1`**。

1. 把 `go.mod:92` 的 replace 指向该提交的伪版本（形如 `v1.4.1-0.<UTC>-904ceab1`，由 `go mod tidy` 生成准确时间戳）；
2. `go mod tidy` 更新 `go.sum`；
3. 同步 `go.mod:83-91` 的 TODO 注释（说明对应分支与提交）；
4. 验证字段到位：

```bash
go doc github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf.AIKeyPolicy | grep SessionAffinityPenaltyTTL
```

本地联调可临时 `replace => ../bfe`，不要提交。`test/integration` 子模块已 `replace` 到本地 BFE 工作树，不需要单独提升。

## 4. 契约文档

| 文档 | 改动 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/clusters.md` | 创建示例 `:62-67` 补两字段；Key 亲和性字段表 `:232` 之后增两行；校验条款 `:316-322` 增两条；PATCH 说明 `:631` 附近补「`llm_config.key_affinity` 同样整体替换，未传子字段按默认值补齐」 |
| `design-docs/api-define/InnerAPI接口定义/server-data-conf.md` | 示例 `:152-161` 补两字段；字段表 `:237` 之后增两行 |
| 过期的历史记录与设计文档（顺手修正） | `design-docs/modifications/2026-08-26-ai-key-session-affinity/change-summary.md:28` 把默认值写成 `enabled=false`，实际代码是 `true`；`design-docs/sys-design/模型层设计文档.md` 的 `KeyAffinity` 片段同样写错并缺两个新字段，一并改正 |

## 5. 测试

### 5.1 单元测试

| 文件 | 用例 |
|------|------|
| `model/icluster_conf/cluster_test.go` | `newTestClusterLLM()` 的 `KeyAffinity` 显式写 `PenaltyTTL429: 30, PenaltyTTLAuth: 120`，在 `TestNewBfeClusterConf` 的 LLM 子测试断言导出为 30/120；另补一条 `KeyAffinity == nil` 导出 60/300 |
| `endpoints/openapi_v1/product_cluster/create_test.go` | `copies key_affinity` 补两字段传递断言；`key_affinity fills defaults when empty object` 与 `... when nil` 各补 60/300 断言 |
| `lib/validate/validate_test.go` | `penalty_ttl_429` 边界 9/10/600/601；`penalty_ttl_auth` 边界 59/60/1800/1801；0 与负数报错；nil 通过 |

### 5.2 集成 schema 断言（`test/integration`，独立子模块）

| 文件 | 改动 |
|------|------|
| `tests/schema/openapi/cluster.go:133-142` | `KeyAffinitySchema` 的 `Required` 与 `Fields` 增两字段（`TypeInt`） |
| `tests/schema/innerapi/schema.go:72-87` | `AIKeyPolicySchema` 的 `Required`/`Fields` 增两字段（导出无 `omitempty`，可 Required） |
| `tests/schema/innerapi/innerapi_schema_test.go:135-140`、`:164-195` | 创建集群的 `key_affinity` 入参补 30/120，并在 `assertServerDataConfKeyAffinity` 断言导出值 |

### 5.3 本地验证命令

```bash
cd ai-gateway-api
go build ./...
make test-model                 # ./model/...
go test ./endpoints/openapi_v1/product_cluster/... ./lib/validate/...
make test-model-cover-gate      # model/ 覆盖率闸门（≥70%）
cd test/integration && go test ./tests/schema/...
```

## 6. 携带约束：PATCH 的全量替换语义

`llm_config` 在 PATCH 中是整体替换而非字段合并：

- `update_basic.go:138` 只按请求体重建 `LLMConfig`；
- `storage/rdb/cluster_conf/cluster.go:77` 把重建后的对象序列化覆盖 `llm_config` 列；
- `normalizeKeyAffinity` 对未传的子字段一律补默认值。

因此只发 `{"llm_config":{"key_affinity":{"penalty_ttl_auth":120}}}` 会清空 `models`/`keys`/`provider`，并把 `key_affinity` 其余字段重置为默认。该行为在本次改动前就存在（四个既有字段相同），本变更不改变它，但契约文档与联调脚本必须按「先 GET、改完整 `llm_config`、再提交」执行。

## 7. 兼容性与发布

| 组合 | 行为 |
|------|------|
| 新控制面 + 新 BFE（未配置） | 导出 60/300 |
| 新控制面 + 新 BFE（已配置） | 按配置生效，无需重启 |
| 新控制面 + 旧 BFE | 旧 BFE 忽略未知字段 → 回退 60/3600，配置静默失效 |
| 旧控制面 + 新 BFE | 缺字段解码为 0 → BFE 加载期补 60/300 |
| 升级前的存量集群 | GET 回显 `null`；导出 60/300；Web 回填默认值后保存即补齐 |

发布顺序：bfe → 本仓库 → Web。发布说明需写明「凭证档默认 3600s → 300s」与「旧 BFE 忽略新字段」。

回滚：控制面回滚后新字段被旧版本忽略（PATCH 会丢弃），集群其余配置不受影响；数据面回滚即回到常量 60/3600。

## 8. 实施清单

- [ ] `go.mod` replace 指向 fork 提交 `904ceab1` + `go mod tidy` + `go doc` 验证字段
- [ ] `KeyAffinity` 增两字段
- [ ] `newAIConf` 字面量与 `derefInt` 映射
- [ ] `normalizeKeyAffinity` 传递 + 补默认
- [ ] `validate.LLMConfig` 双区间校验
- [ ] 单测（model / create / validate）
- [ ] 集成 schema 断言
- [ ] 契约文档（clusters.md、server-data-conf.md）+ 顺手修正 2026-08-26 记录的 `enabled` 默认值
- [ ] `go build ./...`、`make test-model`、`make test-model-cover-gate`、`test/integration` schema 测试通过

## 9. 实施记录

**实施日期**：2026-11-04。基线 `v0.1.0-dev` @ `03fdba4`。

### 9.1 依赖

- `go.mod:92` replace 指向 `github.com/mileszhang2016/bfe v1.4.1-0.20261104035437-904ceab15397`（fork 上 `v1.9.0-dev` 的提交 `904ceab1`，含 `AIKeyPolicy.SessionAffinityPenaltyTTL{429,Auth}`）；
- 伪版本按 fork 可达最高 tag `v1.4.0` 计算，时间戳取提交时间 UTC（2026-11-04T11:54:37+08:00 → `20261104035437`）；
- 新提交 sumdb 尚未收录，首次 `go mod tidy` 用 `GOPRIVATE=github.com/mileszhang2016/*` 跳过校验；`go.sum` 写入后，再跑一次普通 `go mod tidy` 稳定通过；
- `go.mod:83-90` 的 TODO 注释同步更新为本次提交与两个字段；
- `test/integration` 子模块自身 `replace` 到本地 `../../../bfe` 工作树，无需改动。

### 9.2 代码落点

| 文件 | 实施内容 |
|------|----------|
| `model/icluster_conf/cluster.go` | `KeyAffinity` 增 `PenaltyTTL429` / `PenaltyTTLAuth`（`json:"penalty_ttl_429"` / `"penalty_ttl_auth"`，注释写明「0 不是关闭」）；`newAIConf` 默认字面量补 60/300；`KeyAffinity` 分支增两条 `derefInt(..., 60/300)` |
| `endpoints/openapi_v1/product_cluster/create.go` | `normalizeKeyAffinity` 传递两字段并在 nil 时补 60/300（`normalizeLLMConfig` 与更新路径自动复用） |
| `lib/validate/validate.go` | `key_affinity` 块增双区间校验（429: [10,600]、auth: [60,1800]），注释说明区间有意排除 0 |

`normalizeKeyAffinity` 与 `newAIConf` 的两处默认值均为 60/300，与数据面 `AIKeyPolicyCheck` 同值。

### 9.3 文档

- `design-docs/api-define/OpenAPI接口定义/clusters.md`：创建示例补两字段；字段表增两行；校验条款增两条；PATCH 说明补「`key_affinity` 同样整体替换，未传子字段按默认值补齐」；
- `design-docs/api-define/InnerAPI接口定义/server-data-conf.md`：`KeyPolicy` 示例与字段表增两字段；
- `design-docs/sys-design/模型层设计文档.md`：`KeyAffinity` 片段补两字段，并修正 `enabled` 默认值注释（`false` → `true`）；
- `design-docs/modifications/2026-08-26-ai-key-session-affinity/change-summary.md:28`：默认值 `enabled=false` 修正为 `true`（该目录 `design-changes.md:118` 的「当 enabled=false 时…」是语义描述，本来正确，未改）。

### 9.4 测试

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 编译 | `go build ./...` | 通过 |
| 单测 | `go test ./model/... ./endpoints/... ./lib/...` | 83 个包全部通过 |
| 覆盖率闸门 | `go test -coverprofile=... ./model/...` → `go tool cover -func` | **85.3%**（阈值 70%，通过） |
| 集成 schema | 构建 `ai-gateway-api.exe` 后 `cd test/integration && go test ./tests/schema/innerapi/ ./tests/schema/openapi/ -run 'TestInnerAPI_Schema\|TestOpenAPI_Schema'` | 两个包均通过 |
| HTTP 行为测试 | `cd test/integration && go test ./tests/clusters/... ./tests/schema/...` | 9 个包全部通过 |

新增/更新的单测：

- `model/icluster_conf/cluster_test.go`：`newTestClusterLLM()` 显式写 30/120，`TestNewBfeClusterConf` 断言导出 30/120；新增子测试「`KeyAffinity == nil` 时导出 60/300」；
- `endpoints/openapi_v1/product_cluster/create_test.go`：`copies key_affinity` 补两字段传递断言；`fills defaults when empty object` / `when nil` 补 60/300；
- `lib/validate/validate_test.go`：`penalty_ttl_429` 取 10/600 通过、-1/0/9/601 报错；`penalty_ttl_auth` 取 60/1800 通过、-1/0/59/1801 报错；nil 通过。

集成 schema 断言：`KeyAffinitySchema` 与 `AIKeyPolicySchema` 增两字段（innerapi 为 `Required`，导出结构体无 `omitempty`；openapi 同既有四字段一并 `Required`），并在 `assertServerDataConfKeyAffinity` 断言导出 30/120。

新增 HTTP 行为测试（`test/integration/tests/clusters/`）：

- `Create/create_test.go` 的 `TestClusters_Create_KeyPenaltyTTL`：显式 30/120 落库并回读；只传 `enabled` 时回读 60/300；8 组非法值（429 取 -1/0/9/601、auth 取 -1/0/59/1801）返回 422；
- `Update/update_test.go` 的 `TestClusters_Update_KeyPenaltyTTL`：更新为 30/120 后回读一致且 InnerAPI `KeyPolicy` 导出同值；`penalty_ttl_auth=1801` 返回 422 且回读零变更；只传 `penalty_ttl_auth` 时 `penalty_ttl_429` 回落默认 60（验证 `key_affinity` 整体替换语义）。

### 9.5 遗留事项

- 未提交、未推送；
- 构建产物 `ai-gateway-api.exe`、覆盖率 profile 已在验证后删除；
- 上线顺序仍为 bfe → 本仓库 → Web；本仓库发布说明需写明「凭证档默认 3600s → 300s」与「旧 BFE 忽略新字段」。