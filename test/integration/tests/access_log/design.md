# Access Log Config 测试用例设计文档

## 1. 模块概述

Access Log Config（访问日志配置）是配合 BFE `mod_access_pb3` 模块的动态配置：
按白名单采集请求/响应头写入访问日志 proto 字段 `req_headers`（55）/`res_headers`
（158）（bfenetworks/bfe#1506）。OpenAPI 为**单例级全量读写**
（`GET/PUT /open-api/v1/access-log/config`，单行覆盖式存储，固定 `id=1` upsert，
不保存历史版本）；InnerAPI 导出 `mod_access_pb.data`（多段式 JSON，当前仅
`access_header` 段，Version 内嵌文件原样，ai-route 形态）。

关键语义：

- 头名称**大小写不敏感**：服务端归一为 HTTP 规范化形式（如 `business_type` →
  `Business_type`）存储、回显与下发；写入日志的 proto `key` 亦为规范化形式；
- **全量替换**：PUT 省略某字段 = 该字段按缺省清空（两列表均非必填，缺省/null
  按 `[]` 处理）——不存在 PATCH 式合并；
- **敏感头硬拒绝**（大小写不敏感）：`Authorization`/`Proxy-Authorization` →
  422 且不落库；BFE 数据面加载侧不限制白名单内容（管控只在控制面）；
- `Cookie`/`Set-Cookie` 允许，记 Warn 日志 + 操作审计；
- 校验（非空/≤128 字节/RFC 7230 token 字符/归一去重）镜像 BFE
  `mod_access_pb_data.go`，防"控制面放行、BFE reload 失败"穿透；
- 与 intent_config 的语义差异：**空白名单也导出**（空白名单是合法默认态，
  且需支持撤销已发布白名单）；未配置首拉 `Data` 为空白名单而非 null；
- 版本由导出链路数据签名承担（表无 `version` 列），同秒连续变更版本严格
  递增（+1s 抬升，issue #142 机制）。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| AL-1 | 全量更新访问日志配置 | PUT | `/open-api/v1/access-log/config` | 单行覆盖 upsert；响应同 GET 结构（无 version/id） |
| AL-2 | 查询访问日志配置 | GET | `/open-api/v1/access-log/config` | 当前生效配置；未配置返回空数组（非 404） |
| AL-E | 导出访问日志动态配置 | GET | `/inner-api/v1/configs/mod-access-pb` | 导出 `mod_access_pb.data`；版本未变返回 `Data: null`；空白名单也导出 |

## 3. 测试用例统计

| 接口 | 测试用例数 |
|------|-----------|
| 全量更新访问日志配置（PUT） | 7（AL-1-002/003/004/005/006/007/010，含失败审计） |
| 查询访问日志配置（GET） | 1（AL-1-001） |
| 防泄漏（跨 OpenAPI/InnerAPI） | 1（AL-1-008） |
| 成功审计（家族7） | 1（AL-1-009） |
| 并发（家族10，mysql tag） | 1（AL-1-011） |
| 导出访问日志动态配置（InnerAPI） | 5（AL-E-001~005） |
| **合计** | **16** |

> AL-1-011 为 `//go:build mysql` 并发用例（家族10），需 `AIAPI_MYSQL_DSN`，
> SQLite 环境不编译。

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。OpenAPI 权限
`FeatureAccessLog + ActionRead/ActionUpdate`，InnerAPI 权限
`FeatureRoute + ActionExport`，测试环境均跳过。

## 5. 缺陷家族覆盖对照（ai-gateway-api-integration-test-design skill）

| 家族 | 覆盖用例 |
|------|----------|
| 1 省略字段语义 | AL-1-007（省略 = 清空，全量替换语义锁定） |
| 3 4xx 回读零变更 | AL-1-003/004（拒绝后 GET 逐字段比对） |
| 4 非法值矩阵/归因/防泄漏 | AL-1-003/004/006；AL-1-008（被拒配置不得出现在导出产物） |
| 7 审计（成功/失败/精确键/身份快照） | AL-1-009/010 |
| 8 导出产物/版本单调 | AL-E-001~005（顶层键精确、版本未变 null、同秒递增 #142） |
| 10 并发（mysql 落盘） | AL-1-011 |
| 11 返回形状 | AL-1-001（空数组非 404）、AL-E-001（Data 非 null） |
| 12 合同一致性 | 全部用例断言依据 api-define/access-log.md 与 mod-access-pb.md |

## 6. 目录结构

```
access_log/
├── design.md                     # 本文档
├── access_log_test.go            # AL-1-001~008（OpenAPI + 防泄漏）
├── audit_test.go                 # AL-1-009/010（操作审计，家族7）
└── concurrency_mysql_test.go     # AL-1-011（家族10，//go:build mysql）
```
