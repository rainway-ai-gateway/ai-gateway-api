# 变更摘要：访问日志 Header 白名单管理（`mod_access_pb.data` 控制面）

> **日期**: 2026-11-15 ｜ **状态**: 设计 ✅ · 代码 ✅ · api-define ✅ · sys-design ✅ · 单测/集成 ✅ ｜ **接口**: 新增 OpenAPI `GET/PUT /open-api/v1/access-log/config` + InnerAPI `GET /inner-api/v1/configs/mod-access-pb`
>
> 实施要点：存储为单行覆盖式 `access_log_conf`（无 version 列，版本由导出链路数据签名承担）；**空白名单也导出**（与 intent_config “未发布不导出” 语义不同——空白名单是合法默认态，且需支持撤销已发布白名单）；敏感头硬拒绝仅控制面做（校验表镜像 BFE `TestNormalizeHeaderNames` 防漂移）。
>
> 数据面（bfe）对应变更**已实施并推送**（`bfe` 仓库 `v1.10.0-dev` 提交 `c3ddeb18`，SC49 集成测试 8/8 通过）。配套文档：接口变更明细见 [api-changes.md](./api-changes.md)；控制面实施设计（数据流/存储/导出/测试计划/实施清单）见 [design-changes.md](./design-changes.md)。

## 1. 背景

`bfe-access-pb` 协议的 `RequestLog` 自发布起即预留 `req_headers`（字段 55）/`res_headers`（字段 158），但 BFE 数据面从未填充，访问日志写入 Doris 后两列恒空（Doris 明细表对应列早已存在）。业务需要按白名单采集请求头/响应头（如 `business_type`、`x-request-id`、`x-trace-id`）做流量分析与问题排查（需求来源：[bfenetworks/bfe#1506](https://github.com/bfenetworks/bfe/issues/1506)）。

数据面已实现：动态配置文件 `mod_access_pb.data`（多段式 JSON，首段 `access_header`）经 `/reload/mod_access_pb3` 热加载，BFE 加载侧不限制白名单内容（敏感头管控归控制面），白名单缺省为空 = 不采集（存量行为不变）。

本变更给 ai-gateway-api 增加白名单管理面：**OpenAPI 读写 + InnerAPI 按 conf-agent 协议导出 `mod_access_pb.data`**，配合 conf-agent 新增 reloader（仅配置）实现"控制台改白名单 → 全集群热生效"。

## 2. 变更

| 文件 | 变更 |
| - | - |
| `db_ddl.sql` / `db_ddl_sqlite.sql` | 新增表 `access_log_conf`（单例行：`req_headers`/`res_headers` JSON 文本列） |
| `storage/rdb/access_log/`（新增） | DAO：读写 `access_log_conf` |
| `model/access_log/`（新增） | manager：白名单读取/整体更新 + 校验（与 BFE `mod_access_pb_data.go` 格式校验逐条对齐，另加敏感头硬拒绝） |
| `endpoints/openapi_v1/access_log/`（新增） | `GET /access-log/config`、`PUT /access-log/config`；注册 `iauth.FeatureAccessLog`（读/更新鉴权 + 操作日志） |
| `model/imods/access_log_exporter.go`（新增） | Exporter：topic `mod_access_pb`，导出结构 JSON tag 与数据文件契约逐字一致（`Version`/`access_header`/`req_headers`/`res_headers`），复用 `versionControlManager.ExportConfig` 数据签名版本机制（未变更返回 null） |
| `endpoints/innerapi_v1/access_log/export.go`（新增） | `GET /inner-api/v1/configs/mod-access-pb`（薄 handler，模式同 `ai_route/export.go`） |
| `stateful/container/components.go` | 注册 `AccessLogExporter` |
| 测试 | `model/access_log/*_test.go`（校验镜像 BFE 用例）；导出/InnerAPI 集成用例（未变更返回 null、空白名单导出、签名变化返回新 Version） |

## 3. 兼容性

- 表为空（未配置过）→ 导出空白名单 `{Version, access_header:{req_headers:[],res_headers:[]}}` → BFE 按空白名单运行（不采集），语义闭环；
- InnerAPI 导出响应结构为既有 `{"ErrNum":200,"Data":...}` 包装；`Data=null` 表示版本未变（conf-agent `NormalFileTask` 既有语义，框架零改动）；
- 未升级 conf-agent 的部署：数据文件不下发，BFE 按缺省运行，无交叉影响；
- 未接入控制面的部署：可手工维护 `mod_access_pb.data` + reload 接口（数据面既有能力）。

## 4. 验证（已完成 2026-11-15）

- ✅ model 覆盖率 85.4%（门禁 70%）：manager 校验分支覆盖（镜像 BFE 校验表：空名/超长/非法字符/归一重复）；全仓 `go test ./...` 全绿；
- ✅ 集成（`test/integration/tests/access_log/` + `tests/innerapi/access_log/`）：`PUT` 白名单 → `GET` 回显规范化 → InnerAPI 导出断言 JSON 契约（顶层键精确 `{Version, access_header}`）→ 版本未变返回 null → 变更版本递增 → 清空白名单导出空数组；
- ✅ 敏感头：`PUT` 含 `Authorization`/`Proxy-Authorization`（任意大小写）返回 422 且不落库（集成覆盖）；`Cookie`/`Set-Cookie` 允许并记 Warn 日志 + 操作审计。

## 5. 后续

- **conf-agent**：`conf/conf-agent.toml` 新增 `[Reloaders.mod_access_pb3]` 段（`NormalFileTask` + `CopyFiles`，代码零改动）——独立小变更；
- **ai-gateway-web**：访问日志设置页读写 `/access-log/config`（本期仅 API）；
- BFE 侧未来动态配置新段（采样率、按 product 差异化白名单等）以新顶层 key 加入 `mod_access_pb.data`，控制面导出器追加字段即可（契约前向兼容：BFE 未知段忽略 + Warn）。
