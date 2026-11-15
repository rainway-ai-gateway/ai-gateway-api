# 设计变更说明：访问日志 Header 白名单（控制面）

> 数据面（bfe）对应变更已实施并推送（`bfe` 仓库 `v1.10.0-dev` 提交 `c3ddeb18`，SC49 集成测试 8/8 通过：白名单采集/大小写不敏感/默认零采集回归/热加载/失败保留旧配置/未知段与缺段/2048 截断/凭证掩码）。**控制面（本仓库）已实施（2026-11-15）**：全仓 `go test ./...` 全绿、model 覆盖率 85.4%、集成测试（OpenAPI 5 用例 + InnerAPI 3 用例）通过；尚未提交/推送。实施期间一处路由登记修正：`endpoints/innerapi_v1/endpoints_test.go` 端点数断言 19 → 20。

## 1. 数据流

```
Web / API 客户端: PUT /access-log/config
  └─ endpoints/openapi_v1/access_log/          ← 参数绑定（Export 结构同构回显）
      └─ model/access_log (manager)
          ├─ 校验：格式（镜像 BFE normalizeHeaderNames）+ 敏感头硬拒绝 + Cookie Warn
          └─ DB: access_log_conf（单例行，JSON 文本列，itxn 事务）
conf-agent 轮询: GET /inner-api/v1/configs/mod-access-pb?version=<local>
  └─ endpoints/innerapi_v1/access_log/export.go（薄 handler）
      └─ model/imods/access_log_exporter.go
          └─ versionControlManager.ExportConfig(ctx, "mod_access_pb", generator)
              ├─ generator: 读 access_log_conf → ModAccessPbDataExport
              ├─ Sign() → UpsertConfigLastExportedVersion()（版本签名落库）
              └─ version 未变 → 返回 null（conf-agent 跳过）
                  └─ conf-agent 写 <conf>/mod_access_pb3/mod_access_pb.data，切符号链接
                      └─ BFE /reload/mod_access_pb3 → 锁内整份原子替换，热生效
```

全部环节复用既有框架，无框架改动：OpenAPI 模式同 `ai_cache`；导出器骨架同 `ai_route_exporter.go`；版本机制同 `iversion_control/version_control.go:86`。

## 2. 存储

### 2.1 表结构（`db_ddl.sql` 与 `db_ddl_sqlite.sql` 同步）

```sql
CREATE TABLE access_log_conf (
    id           INTEGER PRIMARY KEY,        -- 固定 1（单例行）
    req_headers  TEXT NOT NULL DEFAULT '[]', -- JSON 文本：请求头白名单（已归一存储）
    res_headers  TEXT NOT NULL DEFAULT '[]',
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

- 单行/单例模式（`id=1`，upsert 语义）：白名单是全局配置，与 `ai_cache` 规则列表不同，无多行/排序需求；
- 列表以 JSON 文本列存储（与仓内同类配置一致），读取时 `json.Unmarshal` + 归一校验；
- DAO `storage/rdb/access_log/` + manager `model/access_log/`（`itxn.TxnStorager`，依赖注入，覆盖率门禁 ≥70%）。

### 2.2 校验（`model/access_log/`，与 BFE 逐条对齐）

| 规则 | BFE 侧（`mod_access_pb_data.go`） | 控制面侧 |
|------|----------------------------------|----------|
| trim 后非空 | `normalizeHeaderNames` 报错 | 同（422） |
| 长度 ≤128 字节 | 同 | 同（422） |
| RFC 7230 token 字符 | `isValidHeaderName` | 同（422） |
| 归一为 `CanonicalHeaderKey` | 同 | 同（存储与导出均为规范化形式） |
| 归一后去重 | 报错 fail-fast | 同（422） |
| 敏感头硬拒绝 | **不限制**（数据面职责） | **额外做**：`Authorization`/`Proxy-Authorization`（大小写不敏感）→ 422 拒绝写入 |
| `Cookie`/`Set-Cookie` | 不限制 | 允许 + Warn 审计日志 |

> 敏感头管控**只在控制面做**是本方案明确决策（BFE 加载侧不限制白名单内容，避免限制逻辑分散两侧、误伤手工维护数据文件的部署）。控制面测试用例逐条镜像 BFE `mod_access_pb_data_test.go` 的 `TestNormalizeHeaderNames` 表，防双侧漂移。

## 3. 导出器（`model/imods/access_log_exporter.go`）

```go
const ConfigTopicProductAccessLog = "mod_access_pb"

// JSON tag 与 BFE 加载器、mod_access_pb.data 文件契约逐字一致
type ModAccessPbDataExport struct {
    Version      string             `json:"Version"`
    AccessHeader AccessHeaderExport `json:"access_header"`
}
type AccessHeaderExport struct {
    ReqHeaders []string `json:"req_headers"`
    ResHeaders []string `json:"res_headers"`
}

func (e *AccessLogExporter) ConfigExport(ctx context.Context, lastVersion string) (*ModAccessPbDataExport, error) {
    // versionControlManager.ExportConfig(ctx, ConfigTopicProductAccessLog, generator)
    // generator: 读 access_log_conf（无行 → 空白名单），组装导出结构
    // version == lastVersion → 返回 nil（conf-agent 收到 null 跳过）
}
```

- 版本机制零开发：`ExportConfig` 内置"数据签名 → `UpsertConfigLastExportedVersion` → 未变返回 null"（`version_control.go:86`）；
- DB 为空 → 导出空白名单（`req_headers: [], res_headers: []`）+ 签名版本：BFE 侧按空白名单运行（不采集），语义闭环；
- 注册：`stateful/container/components.go`（`AIRouteExporter` 同款位置）构造 `AccessLogExporter`。

## 4. 端点与鉴权

- OpenAPI：`endpoints/openapi_v1/access_log/endpoints.go`——`GET/PUT /access-log/config`，`Authorizer: iauth.FA(iauth.FeatureAccessLog, iauth.ActionRead/ActionUpdate)`；`iauth` 新增 Feature 枚举并挂 `operation_log`；
- InnerAPI：`endpoints/innerapi_v1/access_log/export.go`——`GET /configs/mod-access-pb`，`Authorizer: iauth.FA(iauth.FeatureRoute, iauth.ActionExport)`（同 `ai_route/export.go:31`）；路由注册 `endpoints/innerapi_v1/endpoints.go`。

## 5. 测试计划

1. **model 单测**（覆盖率门禁）：校验表驱动（镜像 BFE：空名/空白/超长/边界 128/非法字符空格与冒号/全 token 字符/归一重复/敏感头三形态/敏感头硬拒绝）；upsert 单行语义；并发读一致性；
2. **导出器单测**：mock storager——空表导出空数组、版本未变返回 nil、内容变化返回新 Version、JSON tag 逐字断言（marshal 后与契约字符串比对）；
3. **集成测试**（`test/integration/`）：PUT → GET 回显 → InnerAPI 导出契约 → 重复导出 null → 清空（PUT 空数组）→ 导出空数组；敏感头 PUT 422 且库中无记录；
4. **联调**（需 conf-agent 配置段，见 change-summary §5）：控制台改白名单 → conf-agent 拉取 → BFE 热生效（BFE 侧已由 SC49 覆盖）。

## 6. 实施清单

| 步骤 | 内容 | 涉及位置 |
|------|------|----------|
| 1 | DDL（MySQL + SQLite） | `db_ddl.sql`、`db_ddl_sqlite.sql` |
| 2 | DAO + manager（校验） | `storage/rdb/access_log/`、`model/access_log/`（新增） |
| 3 | OpenAPI 端点 + iauth Feature | `endpoints/openapi_v1/access_log/`、`endpoints/openapi_v1/endpoints.go`、`model/iauth/` |
| 4 | Exporter + container 注册 | `model/imods/access_log_exporter.go`、`stateful/container/components.go` |
| 5 | InnerAPI 端点 | `endpoints/innerapi_v1/access_log/export.go`、`endpoints/innerapi_v1/endpoints.go` |
| 6 | 单测/集成测试 | `model/access_log/*_test.go`、`test/integration/` |
| 7 | api-define 文档 | `design-docs/api-define/OpenAPI接口定义/`（新增 access-log.md 或并入系统配置域）、`InnerAPI接口定义/` |

依赖关系：1→2→（3、4、5 并行）→6→7。conf-agent 侧（`[Reloaders.mod_access_pb3]` 配置段，代码零改动）为独立变更，依赖本变更步骤 5 的路径定稿。

## 7. 风险与边界

1. **双侧校验漂移**：BFE 侧仅剩格式校验（token 字符/归一/去重/长度），控制面须与之一致——以镜像测试用例约束（§5.1）；敏感头规则只在控制面，无漂移问题；
2. **整份导出失败语义**：任一侧数据异常则整份 reload 失败、BFE 保留旧配置（BFE 侧原子性已由 SC49 TC-05 覆盖）；
3. **单行表并发**：多实例 ai-gateway-api 同时 PUT——以 `updated_at` 乐观覆盖即可（白名单无事务性要求），如需严格一致性可后续加版本列；
4. **web 控制台**：本期仅 API，UI 后续迭代，不影响本变更范围。
