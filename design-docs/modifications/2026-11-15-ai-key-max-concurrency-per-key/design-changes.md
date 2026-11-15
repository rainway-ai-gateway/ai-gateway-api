# 设计变更说明：cluster 级 `max_concurrency_per_key`（控制面）

> 数据面（bfe）对应变更已实施并推送（`bfe` 仓库 `v1.10.0-dev` 提交 `de10ef5e`，SC48 集成测试全过）。**控制面（本仓库）已实施（2026-11-15），实施记录见 §9。**

## 1. 数据流与默认值分层

```
Web / OpenAPI 请求（llm_config.max_concurrency_per_key）
  └─ POST/PATCH /open-api/v1/clusters
      └─ ValidateLLMConfig()        ← 校验层（model/icluster_conf）：显式值必须 >= 0（nil 放行）
          └─ DB: clusters.llm_config（JSON 文本列，无 DDL；nil 原样落库，无需入库前补默认）
              └─ newAIConf()        ← 导出层（derefInt 兜底：nil → 0）
                  └─ InnerAPI server_data_conf: AIConf.MaxConcurrencyPerKey（无 omitempty，恒显式）
                      └─ conf-agent → cluster_conf.data
                          └─ AIConfCheck()（bfe：< 0 报错；0 = 不限，零 Redis 调用）
```

与 #1448（惩罚 TTL）的关键差异：**本字段不需要"入库前补默认"这一层**——`nil` 与 `0` 语义完全相同（不限），控制面只需在校验层拦截负数、在导出层 `derefInt` 兜底为 0。

## 2. 模型与字段映射

### 2.1 `model/icluster_conf/cluster.go`（`LLMConfig` 结构体，`:250` 起）

```go
type LLMConfig struct {
    // ... 既有字段 ...
    // MaxConcurrencyPerKey caps the number of distinct client sessions
    // (ClientKeyId) per upstream Key, uniformly for every Key of this
    // cluster. nil / 0 = unlimited. Mirrored by the BFE AIConfCheck rule so
    // an accepted config never fails the data-plane load.
    MaxConcurrencyPerKey *int `json:"max_concurrency_per_key"`
}
```

指针形态与既有可空字段一致：`nil` = 未设置（导出按 0 处理）。

### 2.2 校验（`model/icluster_conf/llm_config_validate.go`，`ValidateLLMConfig`）

```go
if c.MaxConcurrencyPerKey != nil && *c.MaxConcurrencyPerKey < 0 {
    return xerror.WrapParamErrorWithMsg("llm_config.max_concurrency_per_key must be >= 0")
}
```

- 位置：`ValidateLLMConfig` 的 keys/key_policy 校验段附近；
- 与 bfe `AIConfCheck`（`MaxConcurrencyPerKey < 0` 报错）逐条对齐；
- `0` 是合法值（不限），**不做** `key_affinity` 那种"区间排除 0"的处理。

### 2.3 导出（`cluster.go`，`newAIConf` `:1654`）

```go
aiConf := &cluster_conf.AIConf{
    // ...
    MaxConcurrencyPerKey: derefInt(llmConfig.MaxConcurrencyPerKey, 0),
}
```

`cluster_conf.AIConf.MaxConcurrencyPerKey` 为 int 值类型、无 `omitempty`，导出 JSON 恒显式带值（0 或 N）。非敏感字段，不参与 `enc$v1$` 加密（加密逻辑只遍历 `Keys[].Key`，`cluster.go:1544-1556`）。

## 3. 依赖提升

`AIConf.MaxConcurrencyPerKey` 类型字段来自 BFE（`go.mod` 的 fork replace）。数据面已合入 fork `v1.10.0-dev` 提交 **`de10ef5e`**（已推送 origin）。

1. 把 `go.mod` 的 replace 指向该提交的伪版本（形如 `v1.4.1-0.<UTC>-de10ef5e`，由 `go mod tidy` 生成准确时间戳；sumdb 未收录时首次 tidy 用 `GOPRIVATE=github.com/mileszhang2016/*` 跳过，写 `go.sum` 后再普通 tidy 一次）；
2. 同步 `go.mod` 的 TODO 注释（对应分支与提交）；
3. 验证字段到位：

```bash
go doc github.com/bfenetworks/bfe/bfe_config/bfe_cluster_conf/cluster_conf.AIConf | grep MaxConcurrencyPerKey
```

本地联调可临时 `replace => ../bfe`，不要提交。`test/integration` 子模块已 `replace` 到本地 BFE 工作树，不需要单独提升。

## 4. 契约文档

| 文档 | 改动 |
|------|------|
| `design-docs/api-define/OpenAPI接口定义/clusters.md` | 数据模型示例的 `llm_config` 增 `"max_concurrency_per_key": 1`；表：LLM配置 增一行（语义/合法性/默认值 0）；约束区增校验条目（`>=0`，`0`=不限）；创建/更新请求示例同步 |
| `design-docs/api-define/InnerAPI接口定义/server-data-conf.md` | `AIConf` 字段表增 `MaxConcurrencyPerKey` 行（恒显式导出，0=不限） |

## 5. 测试

### 5.1 单元测试

| 文件 | 用例 |
|------|------|
| `model/icluster_conf/llm_config_validate_test.go` | `-1` 报错、`0` 通过、正数通过、nil 通过 |
| `model/icluster_conf/cluster_test.go`（导出相关） | `MaxConcurrencyPerKey` 显式值透传导出；nil 导出 `0` |

### 5.2 集成 schema 断言（`test/integration`，独立子模块）

| 文件 | 改动 |
|------|------|
| `tests/schema/openapi/cluster.go` | `LLMConfigSchema` 的 `Fields` 增 `max_concurrency_per_key`（TypeInt，非 Required） |
| `tests/schema/innerapi/schema.go` | `AIConfSchema` 的 `Fields` 增 `MaxConcurrencyPerKey`（TypeInt；导出无 `omitempty`，可 Required） |
| `tests/clusters/Create` / `Update` 行为测试 | 创建携带 `max_concurrency_per_key=1` 落库回读；`-1` 返回 422；InnerAPI 导出断言同值 |

### 5.3 本地验证命令

```bash
cd ai-gateway-api
go build ./...
make test-model                 # ./model/...
make test-model-cover-gate      # model/ 覆盖率闸门（≥70%）
cd test/integration && go test ./tests/schema/... ./tests/clusters/...
```

## 6. 携带约束：PATCH 的全量替换语义

`llm_config` 在 PATCH 中是整体替换而非字段合并：只发 `{"llm_config":{"max_concurrency_per_key":1}}` 会清空 `models`/`keys`/`provider` 等其余字段。该行为在本次改动前就存在，本变更不改变它；契约文档与联调脚本必须按「先 GET、改完整 `llm_config`、再提交」执行。

## 7. 兼容性与发布

| 组合 | 行为 |
|------|------|
| 新控制面 + 新 BFE（未配置） | 导出 `0`，零 Redis 调用，行为与历史一致 |
| 新控制面 + 新 BFE（配置 N>0） | 每把 Key 并发会话上限 N，热加载即时生效 |
| 新控制面 + 旧 BFE | 旧 BFE 忽略未知字段 → 限制不生效（等效 fail-open），其余配置不受影响 |
| 旧控制面 + 新 BFE | 缺字段解码为 `0` → 不限，行为一致 |

发布顺序：bfe（**已完成**，`de10ef5e`）→ 本仓库 → Web。发布说明写明「`>0` 依赖数据面 `[AIKeyAffinity]` Redis，故障期间限制 fail-open」。

回滚：控制面回滚后新字段被旧版本忽略（PATCH 会丢弃），集群其余配置不受影响；数据面回滚即回到无并发限制。

## 8. 实施清单

- [x] `go.mod` replace 指向 fork 提交 `de10ef5e` + `go mod tidy` + `go doc` 验证字段
- [x] `LLMConfig` 增 `MaxConcurrencyPerKey *int`
- [x] `ValidateLLMConfig` 增 `>=0` 校验（与 bfe `AIConfCheck` 对齐）
- [x] `newAIConf` 导出 `derefInt(..., 0)`
- [x] 单测（validate / 导出 / normalize 透传）
- [x] 集成 schema 断言 + clusters 行为测试
- [x] 契约文档（clusters.md、server-data-conf.md）
- [x] `go build ./...`、`go test ./model/...`、`go test ./...`、集成测试通过

## 9. 实施记录

**实施日期**：2026-11-15。基线 `v0.2.0-dev` @ `d7dd7a5`（先同步 upstream/v0.2.0-dev）。

### 9.1 依赖

- `go.mod` 末尾新增 replace：`github.com/bfenetworks/bfe => github.com/mileszhang2016/bfe v1.4.1-0.20261115044259-de10ef5ea307`（fork `v1.10.0-dev` 提交 `de10ef5e`，含 `AIConf.MaxConcurrencyPerKey`；附注释说明 v1.10.0 发布后移除）；
- 伪版本由 `go list -m ...@<full-sha>` 解析得出（手工按本地 tag 猜的 `v1.8.8.7-0...` 基准不被接受，**以 go 解析结果为准**）；
- 首次 `go mod tidy` 用 `GOPRIVATE=github.com/mileszhang2016/*`，随后普通 `go mod tidy` 稳定通过；
- `test/integration` 子模块 `go mod tidy` 一次（主模块 replace 变化传导）。

### 9.2 代码落点

| 文件 | 实施内容 |
|------|----------|
| `model/icluster_conf/cluster.go` | `LLMConfig` 增 `MaxConcurrencyPerKey *int`（注释说明 nil/0=不限、与 BFE 校验同源）；`newAIConf` 构造 `AIConf` 时 `derefInt(..., 0)` |
| `model/icluster_conf/llm_config_validate.go` | `ValidateLLMConfig` 增 `>=0` 校验（`<0` 返回 422 参数错误） |
| `endpoints/openapi_v1/product_cluster/create.go` | `normalizeLLMConfig` 增 `MaxConcurrencyPerKey: llm.MaxConcurrencyPerKey` 透传——**该函数逐字段重建结构体，漏加会静默丢字段**（集成测试首次失败即此因）；nil 保持 nil，无需写库前补默认 |

### 9.3 文档

- `design-docs/api-define/OpenAPI接口定义/clusters.md`：数据模型示例、创建/PATCH 示例、`llm_config` 字段表、约束区校验条目；
- `design-docs/api-define/InnerAPI接口定义/server-data-conf.md`：`AIConf` 示例与字段表增 `MaxConcurrencyPerKey`；
- `design-docs/sys-design/模型层设计文档.md`：`LLMConfig`/`AIConf` 结构体片段与校验要点；
- `design-docs/sys-design/details/条件原语校验与BFE依赖版本对齐.md`：依赖口径更新为 require v1.9.0 + replace 指 fork 现状。

### 9.4 测试

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 编译 | `go build ./...` | 通过 |
| 单测 | `go test ./...`（主模块全量） | 全部通过 |
| 覆盖率闸门 | `go test -coverprofile ./model/...` | **85.4%**（阈值 70%） |
| 集成 schema | `go test ./tests/schema/...` | openapi / innerapi / report 全过 |
| 行为测试 | `go test ./tests/clusters/...` | create/delete/detail/epp/list/update 全过 |

新增测试：

- `model/icluster_conf/llm_config_validate_test.go`：`-1` 报错、`0`/`1`/nil 通过；
- `model/icluster_conf/cluster_test.go`：导出透传 1；nil 导出 0（新增子测试）；
- `endpoints/openapi_v1/product_cluster/create_test.go`：`normalizeLLMConfig` 透传与 nil 保持；
- `test/integration` schema：`LLMConfigSchema` 增字段（Optional）；`AIConfSchema` 增 `MaxConcurrencyPerKey`（Required，导出无 omitempty）；创建携带 1 → InnerAPI 导出断言 1；
- `test/integration/tests/clusters/Create`：`TestClusters_Create_MaxConcurrencyPerKey`（显式回读 / 0 合法 / 缺省回显 null / 负数 422 + 错误消息归因字段名 + **拒绝后集群未落库**（GET 非 200））；
- `test/integration/tests/clusters/Update`：`TestClusters_Update_MaxConcurrencyPerKey`（回读与导出一致 / 负数 422 + 归因 + 回读与**导出**零变更 / **PATCH 只改 description 本字段保留**（检查项 #1 省略字段矩阵）/ 省略回落导出 0）；
- 用例已按 `ai-gateway-api-integration-test-design` skill 登记进 `test/integration/tests/clusters/design.md` §15（CL-1-301~304 / CL-4-301~304，含检查项家族标注）。

顺手修复（既有遗留，非本变更引入）：`tests/schema/openapi/openapi_schema_test.go:510` 的 EPP `timeout.call` 期望 `3s` 未随 90e62c2 改为 `11s`，已更新。

### 9.5 遗留事项

- 未提交、未推送（等待人工确认）；
- `go.mod` replace 为窗口期形态，bfe v1.10.0 正式发布后移除并升正式版（replace 注释已写明）；
- 全量集成套件中 `TestInnerAPI_ExportVersionMonotonic` 曾出现一次 SQLite `database operation failed`（delete cluster 500），单包重跑通过，判定为套件并发下的偶发锁冲突，与本变更无关；
- 发布顺序：bfe（已发布 `de10ef5e`）→ 本仓库 → Web；Web 表单需增加该字段（默认 0/不限）。
