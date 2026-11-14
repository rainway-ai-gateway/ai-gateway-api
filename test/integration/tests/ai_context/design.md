# AI Context Rules 测试用例设计文档

## 1. 模块概述

AI Context Rules（AI 上下文压缩规则）模块负责 AI 上下文压缩规则集合的管理，配合 BFE `mod_ai_context` 模块使用（一期：无损裁剪 L1–L2 + 规则改写 P2，OpenAI chat 协议）。该模块为**集合级全量读写**：不提供 `/{id}` 单条规则操作接口，无 `name` 字段（`cond` 即规则身份）。规则按数组顺序匹配（first-match-wins），数组顺序即优先级（= 导出到 BFE 的顺序）。`mode` 必填（`off`/`conservative`/`balanced`/`aggressive`）：BFE 对缺失/非法 mode 整文件拒载，控制面 422 前置拦截；`cond` 必填、非空且必须通过 BFE `condition.Build` 编译（照 ai-cache-rules 同款控制面编译校验）。规则 `id` 为内部排序字段，不出现在 API 请求与响应中；响应**不返回** `created_at`/`updated_at`（整组替换语义，逐规则时间戳无信息价值）。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| CTX-1 | 全量更新AI上下文压缩规则 | PUT | `/open-api/v1/ai-context-rules` | 全量替换（单事务 delete-all + insert-all）；`rules` 为 null/缺省按 `[]` 清空 |
| CTX-2 | 查询AI上下文压缩规则 | GET | `/open-api/v1/ai-context-rules` | 全量查询（无分页），空集合返回 `{"rules": []}` |
| CTXE-1 | 导出 AI 上下文压缩规则配置 | GET | `/inner-api/v1/configs/ai-context-rule` | 供 BFE `mod_ai_context` 消费；query `version` 可选（增量同步，Data 形状 `{"Version": ..., "Defaults": {...}, "Config": {"AI_product": [...]}}`） |

CTXE 用例（`innerapi/ai_context/ai_context_test.go`）明细：CTXE-1-001 首拉（Defaults 恒导出 + 8 冻结 tag 逐字 + 文本形态）；CTXE-1-002 PUT rules 导出顺序与 4 冻结 tag；CTXE-1-003 PUT settings 版本流（Defaults 反映新值、rules 不受影响）；CTXE-1-004 清空 rules 后 Defaults 仍在；CTXE-1-005 增量同步（version 再拉 Data=null）；CTXE-1-006 422 防泄漏（特征串不出现在导出产物、版本不推进，家族4/#172）；CTXE-1-007 rules PUT 版本流（同秒碰撞防护轮询、Defaults 不受影响，家族8/#142）。

## 3. 测试用例统计

| 接口 | 测试用例数 |
|------|-----------|
| 全量更新AI上下文压缩规则（PUT） | 5 |
| 查询AI上下文压缩规则（GET） | 3 |
| 导出 AI 上下文压缩规则配置（InnerAPI） | 7 |
| **合计** | **15** |

> CTX-1-002 为表驱动用例，含 12 个负向子用例（mode 4 + cond 4 + 数值 2 + 重复 cond 1 + null 元素 1；mode 含大小写敏感子用例）。
> CTX-1-005 / CTX-1-006 为 `//go:build mysql` 并发用例（家族10），需 `AIAPI_MYSQL_DSN`，SQLite 环境不编译。

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。OpenAPI 权限 `FeatureAIContext + ActionUpdate/ActionRead`，InnerAPI 权限 `FeatureAIContext + ActionExport`，测试环境均跳过。

## 5. 目录结构

```
ai_context/
├── design.md
├── update/
│   ├── update_test.go            # CTX-1-001 ~ CTX-1-004（PUT）
│   ├── concurrency_mysql_test.go # CTX-1-005（//go:build mysql，并发全量替换）
│   └── replace_conflict_mysql_test.go # CTX-1-006（//go:build mysql，409 冲突语义，issue #295）
└── get/
    └── get_test.go               # CTX-2-001 ~ CTX-2-003（GET）

innerapi/ai_context/
└── ai_context_test.go            # CTXE-1-001 ~ CTXE-1-007（导出）
```

## 6. 全量更新AI上下文压缩规则（PUT）

### 6.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | AI Context Rules |
| 接口名称 | 全量更新AI上下文压缩规则 |
| 方法 | PUT |
| 路径 | `/open-api/v1/ai-context-rules` |
| 说明 | 单事务内整体替换规则集合（先删除全部旧规则，再按数组顺序写入）；任一校验失败或事务失败则整体回滚，集合保持原状 |

### 6.2 接口参数说明

#### 6.2.1 请求参数

##### Body 参数

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| rules | array | Y | 规则列表，数组顺序即优先级 | 必填；`null`/缺省按 `[]` 处理（清空） |
| rules[].cond | string | Y | BFE 条件表达式 | 必填、非空、须通过 BFE `condition.Build` 编译（控制面 422 前置拦截） |
| rules[].mode | string | Y | 压缩档位 | `off` / `conservative` / `balanced` / `aggressive`；缺失或非法 422 |
| rules[].max_context_tokens | int | N | 预算上限覆盖（token），0=用模型表窗口 | ≥ 0，默认 0 |
| rules[].reserve_tokens | int | N | 预留输出 token，0=自动 clamp(窗口×15%,256,16000) | ≥ 0，默认 0 |

#### 6.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| rules | []Rule | 规则列表（可空字段回填 0） |
| rules[].cond | string | BFE 条件表达式 |
| rules[].mode | string | 压缩档位 |
| rules[].max_context_tokens | int | 预算上限覆盖 |
| rules[].reserve_tokens | int | 预留输出 token |

约束：响应**不得含** `id`/`name`/`enabled` 键，**不返回** `created_at`/`updated_at`（marshal 后 NotContains）。

### 6.3 测试场景总览

| 编号 | 场景 | 测试类型 | 缺陷家族 | 简要说明 |
|------|------|---------|---------|---------|
| CTX-1-001 | 正常两条规则 | 正常参数 | 家族1 | cond 含 `req_path_in(...)` 与 `default_t()` 兜底、mode=balanced/off、带预算字段 → 200，顺序=提交顺序、可空字段回填 0、响应无 id、无 created_at/updated_at；GET 回读一致 |
| CTX-1-002 | 422 负向矩阵 | 异常参数 | 家族3/4 | 表驱动 12 子用例：mode 缺失/空/非法枚举/大小写敏感、cond 缺失/空/`default_t(`/`unknown_func()`（编译校验）、max_context_tokens=-1、reserve_tokens=-1、重复 cond、null 元素 → 全 422 + 错误消息归因字段名（代表子用例）+ GET 回读零变更 |
| CTX-1-003 | 清空 | 边界值 | - | `{"rules":[]}` → 200 且 GET `rules==[]`（非 null） |
| CTX-1-004 | 操作审计 | 审计 | 家族7（#201/#155） | PUT 成功记 update 审计（diff_keys ElementsMatch 精确匹配、快照小写词汇无 id）；PUT 422 记失败审计且身份固定不依赖请求体 |
| CTX-1-005 | 并发全量替换（MySQL） | 并发 | 家族10 | 20 goroutine 并发 PUT 互不相同单规则集合 → 无 5xx，最终集合与某次提交完全相等（`//go:build mysql`，SQLite 不编译） |
| CTX-1-006 | 整体替换冲突返回 409（MySQL） | 并发 | 家族10（#295） | 外部会话占住命名锁 → PUT 返回 409 且 `ErrMsg` 不含驱动错误原文；释放后同一请求 200 |

### 6.4 测试场景详细设计

#### 6.4.1 CTX-1-001：正常两条规则（正常参数，家族1）

##### 设计思路

验证典型生产形态：第一条精确匹配 `/v1/chat/completions` 走 `balanced` 并显式预算（`max_context_tokens=64000`/`reserve_tokens=8192`），第二条 `default_t()` 兜底 `off`（灰度关闭）。断言 200、顺序=提交顺序、可空字段回填 0、规则元素键集合精确 4 键（`cond/mode/max_context_tokens/reserve_tokens`，无 `id`/`name`/`enabled`/`created_at`/`updated_at`），GET 回读与 PUT 响应逐字段一致。

##### 执行步骤

1. PUT `{"rules":[{cond: req_path_in(...), mode: balanced, max_context_tokens: 64000, reserve_tokens: 8192},{cond: default_t(), mode: off}]}`。
2. 断言 200；`rules` 长度 2；[0] 各字段 Equals 提交值；[1] 预算字段回填 0。
3. 规则元素键集合 `assert.ElementsMatch` 精确 `{cond, mode, max_context_tokens, reserve_tokens}`。
4. `json.Marshal(resp.Data)` 后 NotContains `created_at`/`updated_at`。
5. GET 回读与 PUT 响应 `rules` 深比较一致。

##### 预期返回结果

**ErrNum**：200；GET 回读零偏差；响应键集合合同锁定。

---

#### 6.4.2 CTX-1-002：422 负向矩阵（异常参数，家族3/4）

##### 设计思路

表驱动 12 个子用例覆盖 api-define 每条校验规则的负向路径；每个负向用例前置一份合法集合快照，422 后 GET 回读与快照逐字段一致（家族3 假事务：整体拒绝且集合不变）。

| 子用例 | 请求要点 | 预期 |
|--------|---------|------|
| mode 缺失 | `{"cond":...}`（无 mode） | 422 |
| mode 空串 | `mode:""` | 422 |
| mode 非法枚举 | `mode:"hyper"` | 422（四枚举大小写敏感） |
| mode 大小写敏感 | `mode:"Balanced"` | 422（枚举区分大小写） |
| cond 缺失 | `{"mode":"off"}`（无 cond） | 422 |
| cond 空串 | `cond:""` | 422 |
| cond 语法错 | `cond:"default_t("` | 422（condition.Build 编译失败） |
| cond 未知原语 | `cond:"unknown_func()"` | 422（编译失败） |
| max_context_tokens=-1 | 预算=-1 | 422 |
| reserve_tokens=-1 | 预留=-1 | 422 |
| 重复 cond | 两条同 cond 不同 mode | 422 |
| null 元素 | `rules:[{...}, null]` | 422 |

---

#### 6.4.3 CTX-1-003：清空（边界值）

##### 执行步骤

1. PUT 1 条规则后 PUT `{"rules":[]}` → 200；GET 断言顶层键含 `rules` 且为 `[]`（非 null）。

---

#### 6.4.4 CTX-1-004：操作审计（审计，家族7 / #201 / #155）

照 ai_cache AC-1-010 手法：成功路径轮询 `GET /operation-logs`（`resource_type=ai_context_rule, action=update, status=1`，按 change_summary 特征串匹配到本用例），断言 `change_summary` 键集合精确 `{before, after, diff_keys}`、`diff_keys` ElementsMatch 精确 `{rules}`、快照规则元素恰 4 合同键无 id；失败路径（mode 非法 422）断言 `status=2` 审计出现且 `resource_id=resource_name=ai_context_rules`（服务端固定身份，不依赖请求体 #155）。

#### 6.4.5.1 CTX-1-006：整体替换冲突返回 409（MySQL，家族10，issue #295）

外部会话先 `SELECT GET_LOCK('ai_gateway:replace:ai_context_rules', 0)` 占住集合级命名锁，再 PUT 集合：断言 `ErrNum=409`、`ErrMsg` 以 `Conflict` 开头且不含驱动错误原文；释放锁后同一请求返回 200。与 AC-1-013 一起确认三个集合资源各自使用独立命名锁。

#### 6.4.5 CTX-1-005：并发全量替换（MySQL，家族10）

`//go:build mysql` 隔离（SQLite 单写者结构性无法测并发）。20 goroutine 并发 PUT 互不相同单规则集合（cond 携带 worker 特征值）→ 全部 2xx 无 5xx/deadlock；结束后 GET 集合恰 1 条且与某次提交完全相等（无混合状态），可空字段回填 0。DSN 取自 `AIAPI_MYSQL_DSN`，未设置时 Skip。

---

## 7. 查询AI上下文压缩规则（GET）

### 7.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | AI Context Rules |
| 接口名称 | 查询AI上下文压缩规则 |
| 方法 | GET |
| 路径 | `/open-api/v1/ai-context-rules` |
| 说明 | 全量查询；空集合返回 `{"rules": []}`；按优先级升序返回（顺序同导出到 BFE 的顺序） |

### 7.2 测试场景总览

| 编号 | 场景 | 测试类型 | 缺陷家族 | 简要说明 |
|------|------|---------|---------|---------|
| CTX-2-001 | 空集合形状 | 边界值 | 家族11 | 顶层键含 `rules` 且为 `[]`（非 null） |
| CTX-2-002 | 顺序 | 返回数据 | - | PUT 2 条后 GET 顺序=提交顺序 |
| CTX-2-003 | 幂等 | 返回数据 | - | 连续两次 GET 响应逐字段一致 |

### 7.3 测试场景详细设计

#### 7.3.1 CTX-2-001：空集合形状（边界值，家族11）

按合同，空集合返回 `{"rules": []}` 而非 `Data=null`。断言顶层键集合含 `rules`，值为 `[]`（区分于 null/缺失）。

#### 7.3.2 CTX-2-002：顺序（返回数据）

PUT 2 条 cond 各不相同的规则，GET 返回的 `rules[*].cond` 序列严格等于提交序列。

#### 7.3.3 CTX-2-003：幂等（返回数据）

连续两次 GET，响应 Data 逐字段一致。

---

## 8. 依赖与数据准备

1. 本模块为全局单例集合，无跨资源引用（cond 做控制面编译校验，不引用 cluster/provider）。
2. 测试包各自独立数据库（PID 隔离），用例间以 PUT 全量替换自包含，不得依赖执行顺序（GET 包内 CTX-2-001 依赖源码顺序最先执行，其后用例先自行 PUT 构造集合）。
3. Inner 导出 product 键名取自测试环境配置 `AIRouteInnerProductName="AI_product"`。

## 9. 注意事项

1. PUT 为全量替换，任一用例不得依赖执行顺序。
2. 每个负向用例（422）都必须带"GET 回读零变更"断言（家族3）。
3. 响应合同锁定：规则元素恰 4 键，无 `id`/`name`/`enabled`/`created_at`/`updated_at`。
