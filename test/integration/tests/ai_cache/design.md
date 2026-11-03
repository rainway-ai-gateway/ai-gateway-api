# AI Cache Rules 测试用例设计文档

## 1. 模块概述

AI Cache Rules（AI 缓存规则）模块负责 AI 缓存规则集合的管理，配合 BFE `mod_ai_cache` 模块使用（一期：仅 Redis 精确匹配）。该模块为**集合级全量读写**：不提供 `/{id}` 单条规则操作接口。规则按数组顺序匹配（first-match-wins），数组顺序即优先级（= 导出到 BFE 的顺序）。`cache_key_strategy=disabled` 为缓存豁免语义（命中即不读不写缓存并阻断后续规则），与"从列表移除"不同。规则 `id` 为内部排序字段，不出现在 API 请求与响应中。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| AC-1 | 全量更新AI缓存规则 | PUT | `/open-api/v1/ai-cache-rules` | 全量替换（单事务 delete-all + insert-all）；`rules` 为 null/缺省按 `[]` 清空 |
| AC-2 | 查询AI缓存规则 | GET | `/open-api/v1/ai-cache-rules` | 全量查询（无分页），空集合返回 `{"rules": []}` |
| AICE-1 | 导出 AI 缓存规则配置 | GET | `/inner-api/v1/configs/ai-cache-rule` | 供 BFE `mod_ai_cache` 消费；query `version` 可选（增量同步，Data 形状 `{"Config": {"AI_product": [...]}, "Version": "..."}`） |

## 3. 测试用例统计

| 接口 | 测试用例数 |
|------|-----------|
| 全量更新AI缓存规则（PUT） | 18 |
| 查询AI缓存规则（GET） | 3 |
| 导出 AI 缓存规则配置（InnerAPI） | 6 |
| **合计** | **27** |

> AC-1-012 / AC-1-019 为 `//go:build mysql` 并发用例（家族10），需 `AIAPI_MYSQL_DSN`，SQLite 环境不编译。

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。OpenAPI 权限 `FeatureAICache + ActionUpdate/ActionRead`，InnerAPI 权限 `FeatureAICache + ActionExport`，测试环境均跳过。

## 5. 目录结构

```
ai_cache/
├── design.md
├── update/
│   ├── update_test.go            # AC-1-001 ~ AC-1-018（PUT）
│   ├── concurrency_mysql_test.go # AC-1-012（//go:build mysql，并发全量替换）
│   └── replace_conflict_mysql_test.go # AC-1-019（//go:build mysql，409 冲突语义，issue #295）
└── get/
    └── get_test.go               # AC-2-001 ~ AC-2-003（GET）

innerapi/ai_cache/
└── ai_cache_test.go              # AICE-1-001 ~ AICE-1-006（导出）
```

## 6. 全量更新AI缓存规则（PUT）

### 6.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | AI Cache Rules |
| 接口名称 | 全量更新AI缓存规则 |
| 方法 | PUT |
| 路径 | `/open-api/v1/ai-cache-rules` |
| 说明 | 单事务内整体替换规则集合（先删除全部旧规则，再按数组顺序写入）；任一校验失败或事务失败则整体回滚，集合保持原状 |

### 6.2 接口参数说明

#### 6.2.1 请求参数

##### Body 参数

| 参数名 | 类型 | 必填 | 说明 | 合法性条件 |
|--------|------|------|------|------------|
| rules | array | Y | 规则列表，数组顺序即优先级 | 必填；`null`/缺省按 `[]` 处理（清空） |
| rules[].name | string | Y | 规则名称 | 必填；1-128 字符；同一集合内不得重复 |
| rules[].cond | string | Y | BFE 条件表达式 | 必填；必须能通过 BFE `condition.Build` 编译；同一集合内**不得重复**（逐字节比较） |
| rules[].cache_key_strategy | string | N | 缓存键策略 | `lastQuestion`（默认）/ `allQuestions` / `disabled` |
| rules[].cache_ttl | int | N | 缓存 TTL（秒），0 表示不过期 | ≥ 0，默认 0 |
| rules[].max_body_bytes | int64 | N | 请求体大小上限（字节） | > 0，默认 1048576 |
| rules[].max_value_bytes | int64 | N | 缓存值大小上限（字节） | > 0，默认 1048576 |

#### 6.2.2 返回数据字段

| 参数名 | 类型 | 说明 |
|--------|------|------|
| rules | []Rule | 规则列表（含默认值回填） |
| rules[].name | string | 规则名称 |
| rules[].cond | string | BFE 条件表达式 |
| rules[].cache_key_strategy | string | 缓存键策略 |
| rules[].cache_ttl | int | 缓存 TTL |
| rules[].max_body_bytes | int64 | 请求体大小上限 |
| rules[].max_value_bytes | int64 | 缓存值大小上限 |

约束：响应**不得含内部 `id` 字段**；无 `enabled` 字段（提交的列表即生效集合）；**不含逐行 `created_at`/`updated_at`**（整组替换语义下无信息价值，issue #258；请求体若携带按未知字段忽略）。同一集合内 `cond` 不得重复（逐字节比较）：BFE 以 `cond` 作为 product 内的规则键，重复会使整份 `ai_cache.data` 拒载（issue #261）。

### 6.3 测试场景总览

| 编号 | 场景 | 测试类型 | 缺陷家族 | 简要说明 |
|------|------|---------|---------|---------|
| AC-1-001 | 最小参数 | 正常参数 | 家族1 | 仅 name+cond → 200 且默认值回填（strategy=lastQuestion/ttl=0/bytes=1048576×2），GET 回读一致 |
| AC-1-002 | 完整参数 3 条 | 正常参数 | - | 显式非默认值、各不相同 → 200，响应顺序=提交顺序 |
| AC-1-003 | 全量替换 | 正常参数 | 家族1/3 | 删1/改1/增1 → GET 逐字段精确等于新提交集合；被删规则消失 |
| AC-1-004 | 清空 | 边界值 | - | `{"rules":[]}` 与 `{}`（rules 缺省）→ 200 且 GET `rules==[]`（非 null） |
| AC-1-005 | 非法 cond | 异常参数 | 家族3 | 语法错、`req_path_in` 单参数 → 422 + 错误可归因 + GET 回读逐字段一致 |
| AC-1-006 | name 重名 | 异常参数 | 家族3 | 集合内重名 → 422 + 回读零变更 |
| AC-1-007 | 非法值矩阵 | 异常参数 | 家族3/4 | 表驱动 7 子用例：name 空/129、strategy 大小写、ttl=-1、bytes=0/-1、null 元素 → 全 422 + 回读零变更 |
| AC-1-008 | 边界正值 | 边界值 | 家族4 | name=128、cache_ttl=0、max_body_bytes=1 → 200（边界±1） |
| AC-1-009 | 合同锁定 | 合同一致性 | 家族11/12 | 200 响应规则元素不得出现 `id`/`enabled` 键 |
| AC-1-010 | 操作审计 | 审计 | 家族7（#201/#155） | PUT 成功记 update 审计（before/after 集合快照、diff_keys 精确匹配、快照无 `id`）；PUT 422 记失败审计且身份不依赖请求体 |
| AC-1-011 | 混合集合假事务 | 异常参数 | 家族3/4 | 前 2 条合法第 3 条非法 → 422 且三条都未生效（GET 回读 + Inner 导出双重断言） |
| AC-1-012 | 并发全量替换（MySQL） | 并发 | 家族10 | 20 goroutine 并发 PUT 互不相同小集合 → 无 5xx，最终集合与某次提交完全相等 |
| AC-1-013 | 语义缓存开关往返 | 正常参数 | 家族1 | 显式 `true`/`false` 原样落库回读、省略回填 `false`（PUT 响应 = GET 回读） |
| AC-1-014 | 回读原样 PUT | 正常参数 | 家族1/11 | GET 的 `rules` 原样作为 PUT body → 200 且逐字段一致（键集合 7 字段，无时间戳） |
| AC-1-015 | 旧客户端携带只读时间戳 | 正常参数 | 家族1/11（#258） | 带 `created_at`/`updated_at` 的旧规则在前/在后两种排列 → 均 200、两条都写入、顺序=提交顺序 |
| AC-1-016 | 只读时间戳被忽略 | 合同一致性 | 家族11（#258） | 提交伪造的 2000 年时间戳 → 200，PUT 响应与 GET 回读均无 `created_at`/`updated_at` 键 |
| AC-1-017 | 集合内 cond 重复 | 异常参数 | 家族3/7（#261） | 同一 PUT 内两条规则仅 name/ttl 不同、`cond` 逐字节相同 → 422（ErrMsg 含 duplicate 与 cond 原文）+ 回读零变更 + 失败审计 before 快照取自库中现状 |
| AC-1-018 | 唯一 cond 集合幂等编辑 | 正常参数 | 家族1/11（#261） | 2 条 `cond` 互不相同 → 200；GET 回读原样作为 PUT body 再提交 → 200 且逐字段一致、规则元素仍为合同 7 键 |
| AC-1-019 | 整体替换冲突返回 409（MySQL） | 并发 | 家族10（#295） | 外部会话占住命名锁 → PUT 返回 409 且 `ErrMsg` 不含驱动错误原文；释放后同一请求 200 |

### 6.4 测试场景详细设计

#### 6.4.1 AC-1-001：最小参数（正常参数，家族1）

##### 设计思路

验证仅提交 `name`+`cond` 时，可选字段由服务端按合同默认值回填： `cache_key_strategy=lastQuestion`、`cache_ttl=0`、`max_body_bytes=max_value_bytes=1048576`；响应规则元素为 7 个业务字段（逐行时间戳自 issue #258 起不返回）。随后 GET 回读与 PUT 响应逐字段一致。

##### 前提数据准备

无（集合内容不影响本用例，用例自身即全量替换）。

##### 执行步骤

1. PUT `{"rules":[{"name":"<runID>-ac1001","cond":"req_path_in(\"/v1/chat/completions\", false)"}]}`。
2. 断言 200；逐字段断言默认值回填，并断言键集合精确为 7 字段（无 `created_at`/`updated_at`/`id`）。
3. GET `/open-api/v1/ai-cache-rules`，回读与 PUT 响应逐字段一致。

##### 预期返回结果

**ErrNum**：200，**ErrMsg**：success

| 字段 | 预期值 | 校验方式 |
|------|--------|---------|
| rules | 长度 1 | Len=1 |
| rules[0].name | 提交值 | Equals |
| rules[0].cond | 提交值 | Equals |
| rules[0].cache_key_strategy | "lastQuestion" | Equals（默认值回填） |
| rules[0].cache_ttl | 0 | Equals（默认值回填） |
| rules[0].max_body_bytes | 1048576 | Equals（默认值回填） |
| rules[0].max_value_bytes | 1048576 | Equals（默认值回填） |
| rules[0].created_at / updated_at | 键不存在 | NotContains（#258） |

---

#### 6.4.2 AC-1-002：完整参数 3 条（正常参数）

##### 设计思路

验证 3 条规则全部显式设置非默认且互不相同的参数值时写入成功，响应顺序严格等于提交顺序（first-match-wins）。

##### 执行步骤

1. PUT 3 条规则：strategy 分别 `allQuestions`/`disabled`/`lastQuestion`，ttl 3600/86400/60，bytes 各不相同。
2. 断言 200；按数组下标逐字段断言与提交值一致（顺序即优先级）。

##### 预期返回结果

**ErrNum**：200；`rules[i]` 各字段 Equals 提交值；响应数组顺序 Equals 提交顺序。

---

#### 6.4.3 AC-1-003：全量替换（正常参数，家族1/3）

##### 设计思路

验证全量替换语义：不在提交列表中的规则即删除；修改的规则按新值生效；新增的规则出现；被删规则消失。GET 回读逐字段精确等于新提交集合（含默认值回填）。

##### 执行步骤

1. PUT 集合 A（3 条：a1/a2/a3）。
2. PUT 集合 B（2 条：改 a1 的 ttl、新增 b1；a2/a3 删除）。
3. GET 断言：长度 2；[0]=修改后的 a1 逐字段精确；[1]=b1 逐字段精确（含默认值回填）；响应中不存在 a2/a3 的 name。

##### 预期返回结果

**ErrNum**：200；GET `rules` 与集合 B 逐字段 DeepEquals（键集合 7 字段，无时间戳）。

---

#### 6.4.4 AC-1-004：清空（边界值）

##### 设计思路

验证 `{"rules":[]}` 与 `{}`（rules 缺省）两种形态均清空全部规则，且 GET 返回 `rules==[]`（空数组而非 null）。

##### 执行步骤

1. PUT 1 条规则后 PUT `{"rules":[]}` → 200；GET 断言顶层键含 `rules` 且为 `[]`（非 null）。
2. PUT 1 条规则后 PUT `{}` → 200；GET 断言同上。

##### 预期返回结果

**ErrNum**：200；GET `Data.rules` 为空数组（`[]`，非 null）。

---

#### 6.4.5 AC-1-005：非法 cond（异常参数，家族3）

##### 设计思路

验证 cond 必须通过 BFE `condition.Build` 编译：`req_path_in` 必须两参数（合同示例形态），语法错误同样拒绝。两种非法形态各自断言 422、错误消息可归因（ErrMsg 含 "cond"），且**GET 回读与操作前逐字段一致**（回读零变更）。

##### 请求参数（子用例）

```json
{"rules":[{"name":"<runID>-ac1005a","cond":"not_a_valid_expr("}]}
{"rules":[{"name":"<runID>-ac1005b","cond":"req_path_in(\"/v1/chat/completions\")"}]}
```

##### 预期返回结果

**ErrNum**：422；**ErrMsg**：可归因到 cond 字段；GET 回读 DeepEquals 操作前集合（含时间戳）。

---

#### 6.4.6 AC-1-006：name 重名（异常参数，家族3）

##### 设计思路

验证同一 `rules` 集合内 name 不得重复 → 422 + 回读零变更。

##### 预期返回结果

**ErrNum**：422；**ErrMsg**：含重名提示；GET 回读零变更。

---

#### 6.4.7 AC-1-007：非法值矩阵（异常参数，家族3/4）

##### 设计思路

表驱动 7 个子用例，覆盖 api-define 中每条校验规则的负向路径，每个负向后接"GET 回读零变更"断言：

| 子用例 | 请求要点 | 预期 |
|--------|---------|------|
| name 空串 | `name:""` | 422 |
| name 129 字符 | `name:"<129 chars>"` | 422 |
| strategy 大小写 | `cache_key_strategy:"LastQuestion"` | 422（枚举区分大小写） |
| cache_ttl=-1 | `cache_ttl:-1` | 422 |
| max_body_bytes=0 | `max_body_bytes:0` | 422 |
| max_value_bytes=-1 | `max_value_bytes:-1` | 422 |
| rules 含 null 元素 | `rules:[{...}, null]` | 422 |

正向锚点（家族4 字符集收紧对照）：合法 `disabled` 策略用例在 AC-1-002 已覆盖。

---

#### 6.4.8 AC-1-008：边界正值（边界值，家族4）

##### 设计思路

边界±1 正向验证：name=128 字符（上限）、cache_ttl=0（≥0 边界）、max_body_bytes=1（>0 下界）→ 200 且逐字段回读一致。

---

#### 6.4.9 AC-1-009：合同锁定（合同一致性，家族11/12）

##### 设计思路

锁定"响应不得含内部 id 字段、无 enabled 字段、无逐行时间戳"的合同条款：对 200 响应的每个规则元素，键集合用 `assert.ElementsMatch` **精确匹配** `{name, cond, cache_key_strategy, cache_ttl, max_body_bytes, max_value_bytes, enable_semantic_cache}` 7 键，禁止 `Contains`（#201 幻影键不敏感；时间戳移除见 #258）。

---

#### 6.4.10 AC-1-010：操作审计（审计，家族7 / #201 / #155）

##### 设计思路

- **成功路径**：PUT 成功后轮询 `GET /open-api/v1/operation-logs`（testutil.WaitForOperationLog），等到 `resource_type=ai_cache_rule, action=update, status=1` 记录；断言 `change_summary` 键集合精确为 `{before, after, diff_keys}`，快照键名为小写 API 词汇（`rules`/规则字段），`diff_keys` 用 ElementsMatch 精确匹配（集合内容变化时含 `rules`），且 change_summary 序列化后不含 `"id"`。
- **失败路径**：PUT 422 后同样等待 `resource_type=ai_cache_rule, action=update, status=2` 记录，断言其 resource_id/resource_name 取自服务端固定身份（`ai_cache_rules`），不依赖请求体内容（#155）。

##### 预期返回结果

成功/失败两条审计均在有限轮次内出现；身份与快照键名符合合同。

---

#### 6.4.11 AC-1-011：混合集合假事务（异常参数，家族3/4）

##### 设计思路

同一 PUT 中前 2 条合法、第 3 条非法（cond 编译失败）→ 422；断言**三条都未生效**：GET 回读与操作前逐字段一致（家族3 假事务），且 Inner 导出与操作前完全一致（家族4 防泄漏——被拒绝的规则不得出现在下发 BFE 的产物中）。Inner 侧断言方式：拒绝前导出取 `Version=V1`，拒绝后带 `?version=V1` 再拉必须返回 `Data=null`（版本未推进即内容未变）。

##### 预期返回结果

**ErrNum**：422；GET 回读零变更；带 V1 增量拉取返回 `Data=null`。

---

#### 6.4.11.1 AC-1-019：整体替换冲突返回 409（MySQL，家族10，issue #295）

外部会话（`database/sql` 直连测试库）先 `SELECT GET_LOCK('ai_gateway:replace:ai_cache_rules', 0)` 占住集合级命名锁，再 `PUT /open-api/v1/ai-cache-rules`：断言 `ErrNum=409`、`ErrMsg` 以 `Conflict` 开头且不含 `Database Exception`/`Error 1213`/`Deadlock`；释放锁后同一请求返回 200，集合可用。该用例同时钉死「命名锁在真实请求路径生效」，避免 helper 因驱动判定失败退化为 no-op 而无人发现。

#### 6.4.12 AC-1-012：并发全量替换（MySQL，家族10）

##### 设计思路

`//go:build mysql` 隔离（SQLite 单写者结构性无法测并发，家族10 要求用例必须落盘）。20 个 goroutine 并发 PUT 互不相同的小集合（各 1 条 name/cond 唯一的规则）→ 全部 2xx、无 5xx/死锁；结束后 GET 集合与**某一次提交完全相等**（无混合状态）。

##### 预期返回结果

20 请求全 2xx；最终 GET `rules` 长度=1 且其 name/cond 命中某一提交。

---

#### 6.4.13 AC-1-013：语义缓存开关往返（正常参数，家族1）

##### 设计思路

验证规则级二期字段 `enable_semantic_cache`：显式 `true`/`false` 原样落库并在 PUT 响应与 GET 回读中回显；省略时按合同默认值回填 `false`。

##### 执行步骤

1. PUT 3 条：第 1 条显式 `true`、第 2 条显式 `false`、第 3 条省略该字段。
2. 断言 200，按数组下标断言 `enable_semantic_cache` 分别为 `true`/`false`/`false`，其余字段按默认值回填。
3. GET 回读与 PUT 响应逐字段一致。

##### 预期返回结果

**ErrNum**：200；`rules[i].enable_semantic_cache` = `true`/`false`/`false`；GET 回读 = PUT 响应。

---

#### 6.4.14 AC-1-014：回读原样 PUT（正常参数，家族1/11）

##### 设计思路

覆盖"读取—修改—写回"的最自然编辑路径：把 `GET` 回读的 `rules` **原样**作为 `PUT` body 提交，服务端必须接受并按提交顺序重建集合（issue #258 的触发形态）。

##### 执行步骤

1. PUT 2 条规则（其中一条显式 `cache_ttl`）。
2. GET 取回 `Data.rules`，原样作为 PUT body 再次提交。
3. 断言 200；PUT 响应与原 `Data.rules` 逐字段一致；每个规则元素的键集合精确为 7 字段（无 `created_at`/`updated_at`）。

##### 预期返回结果

**ErrNum**：200；`rules` 长度 2、顺序不变、键集合 7 字段。

---

#### 6.4.15 AC-1-015：旧客户端携带只读时间戳（正常参数，家族1/11，#258）

##### 设计思路

复现 issue #258 的两种排列（同一集合、仅时间戳字段存在性不同）：

| 提交内容 | 修复前 | 修复后 |
| --- | --- | --- |
| 旧规则（带 `created_at`/`updated_at`）在前 + 新规则（不带）在后 | `500 insert data not match` | 200 |
| 新规则在前 + 旧规则在后 | 200（旧行 `updated_at` 被静默丢弃） | 200 |

时间戳自 #258 起不在契约内，按未知字段忽略，因此两种排列行为必须完全一致（各自按提交顺序返回，两条都写入）。

##### 执行步骤

1. 子用例 `legacy_rule_first` / `legacy_rule_last`，各先 PUT 一条规则（模拟"已有规则"）。
2. 构造旧规则（7 业务字段 + 两个 RFC3339 时间戳）与新规则（仅 name/cond/ttl），按子用例顺序组成 2 条数组后 PUT。
3. 断言 200、`rules` 长度 2、name 顺序 = 提交顺序、每个元素不含 `created_at`/`updated_at`。

##### 预期返回结果

**ErrNum**：200（修复前 `legacy_rule_first` 为 500）；两条规则均写入。

---

#### 6.4.16 AC-1-016：只读时间戳被忽略（合同一致性，家族11，#258）

##### 设计思路

验证只读时间戳既不回显也不落库：提交伪造的 `2000-01-01T00:00:00Z`，响应与 GET 回读均不得出现这两个键（区别于"值不等于伪值"——字段本身不在契约内）。

##### 执行步骤

1. PUT 1 条规则，body 显式携带 `created_at`/`updated_at` = `2000-01-01T00:00:00Z`。
2. 断言 200；PUT 响应 `rules[0]` 不含 `created_at`/`updated_at`，`name` 等于提交值。
3. GET 回读，断言同样不含这两个键。

##### 预期返回结果

**ErrNum**：200；响应与回读均无时间戳键。

---

#### 6.4.17 AC-1-017：集合内 cond 重复（异常参数，家族3/7，#261）

##### 设计思路

BFE `mod_ai_cache` 以 `cond` 字符串作为 product 内的规则键，集合内重复会使整份 `ai_cache.data` 被拒载，conf-agent 每 5s 重试同一文件、symlink 不推进（issue #261）。因此控制面必须在写入前拦截：同一 PUT 的 `rules` 数组内任意两条 `cond` 逐字节相同即 422，集合零变更，并记一条失败审计。

##### 前提数据准备

PUT 1 条 `cond=req_path_in("/v1/chat/completions", false)` 的合法规则，作为"库中现状"。

##### 执行步骤

1. 取 `before` 快照（GET 回读整个集合）。
2. PUT 2 条规则：仅 `name` 与 `cache_ttl`（60 / 600）不同，`cond` 完全相同（判重只看 cond）。
3. 断言 422；`ErrMsg` 含 `duplicate` 且含该 `cond` 原文。
4. GET 回读，断言与 `before` 逐字段相等（拒绝即零变更，家族3）。
5. 轮询操作日志：`resource_type=ai_cache_rule, action=update, status=2` 且 `error_msg` 含该 cond 的记录；断言 `resource_id=resource_name=ai_cache_rules`，且 `change_summary.before.rules` 长度 1、内容为库中现状规则（before 快照取自库中现状而非请求体，#155）。

##### 预期返回结果

**ErrNum**：422；**ErrMsg**：`duplicate ai cache rule cond: <cond>`；GET 回读零变更；失败审计 1 条。

---

#### 6.4.18 AC-1-018：唯一 cond 集合幂等编辑（正常参数，家族1/11，#261）

##### 设计思路

验证新增的集合内 cond 唯一性校验不干扰正常编辑路径：`cond` 互不相同的集合可正常写入，"GET 回读 → 原样 PUT"仍 200 且响应与回读逐字段一致，规则元素仍是合同 7 字段（无 `created_at`/`updated_at`/`id`）。

##### 执行步骤

1. PUT 2 条规则：`cond` 分别为 `req_path_in("/v1/chat/completions", false) && req_body_json_in("model", "<name1>", false)` 与同形态的 `<name2>` 变体（逐条唯一），其中一条显式 `cache_ttl=120`、另一条显式 `cache_key_strategy=allQuestions`。
2. 断言 200；按数组下标逐字段断言（含默认值回填与合同 7 键锁）。
3. GET 回读 `Data.rules`，断言与 PUT 响应逐字段一致。
4. 把回读的 `rules` 原样作为 PUT body 再次提交 → 200；响应与原回读逐字段一致，每个规则元素键集合精确为 7 字段。

##### 预期返回结果

**ErrNum**：200（两次）；`rules` 长度 2、顺序不变、字段逐一致。

---

## 7. 查询AI缓存规则（GET）

### 7.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | AI Cache Rules |
| 接口名称 | 查询AI缓存规则 |
| 方法 | GET |
| 路径 | `/open-api/v1/ai-cache-rules` |
| 说明 | 全量查询；空集合返回 `{"rules": []}`；按优先级升序返回（顺序同导出到 BFE 的顺序） |

### 7.2 测试场景总览

| 编号 | 场景 | 测试类型 | 缺陷家族 | 简要说明 |
|------|------|---------|---------|---------|
| AC-2-001 | 空集合形状 | 边界值 | 家族11 | 顶层键含 `rules` 且为 `[]`（非 null） |
| AC-2-002 | 顺序 | 返回数据 | - | PUT 3 条后 GET 顺序=提交顺序 |
| AC-2-003 | 幂等 | 返回数据 | - | 连续两次 GET 响应逐字段一致 |

### 7.3 测试场景详细设计

#### 7.3.1 AC-2-001：空集合形状（边界值，家族11）

##### 设计思路

按合同，空集合返回 `{"rules": []}` 而非 `Data=null`。断言顶层键集合含 `rules`，且其值为 `[]`（区分于 null/缺失，防止客户端按零值解码）。

##### 执行步骤

1. PUT `{"rules":[]}` 清空。
2. GET，断言 `Data.rules` 存在、类型为数组、长度为 0。

#### 7.3.2 AC-2-002：顺序（返回数据）

##### 设计思路

PUT 3 条 name 各不相同的规则，GET 返回的 `rules[*].name` 序列严格等于提交序列。

#### 7.3.3 AC-2-003：幂等（返回数据）

##### 设计思路

连续两次 GET，响应 Data 逐字节/逐字段一致（无随机字段、无隐式变更）。

---

## 8. 导出 AI 缓存规则配置（InnerAPI）

### 8.1 接口信息

| 项目 | 值 |
|------|-----|
| 模块 | InnerAPI |
| 接口名称 | 导出 AI 缓存规则配置 |
| 方法 | GET |
| 路径 | `/inner-api/v1/configs/ai-cache-rule` |
| 说明 | Data 形状 `{"Config": {"AI_product": [规则...]}, "Version": "..."}`；一期每条规则仅导出 `cond`/`cacheKeyStrategy`/`cacheTTL`/`maxBodyBytes`/`maxValueBytes` 5 个 tag（大写驼峰，与 Open API 词汇不同）；version 相同（增量）时 Data 为 null |

### 8.2 测试场景总览

| 编号 | 场景 | 测试类型 | 缺陷家族 | 简要说明 |
|------|------|---------|---------|---------|
| AICE-1-001 | 首拉 | 正常参数 | 家族8 | PUT 2 条 → Config.AI_product 长度 2、顺序=提交顺序、5 个导出 tag 值正确；对原始 body 文本断言 `"cacheKeyStrategy":"lastQuestion"` 等 |
| AICE-1-002 | 二期字段缺席 | 合同一致性 | 家族8 | 导出 body 不得含 cacheKeyFrom/cacheValueFrom/cacheStreamValueFrom/cacheToolCallsFrom/responseTemplate/streamResponseTemplate |
| AICE-1-003 | 增量 | 正常参数 | - | 带 `?version=<刚返回Version>` 再拉 → Data null；不传 version 再拉 → 有数据且 Version 相同 |
| AICE-1-004 | 版本单调 | 并发语义 | 家族8/#142 | 同一秒内连续两次不同 PUT → 轮询导出（500ms×10）直到版本严格大于旧版本且内容收敛为最新集合 |
| AICE-1-005 | 清空导出 | 边界值 | 家族8 | PUT `{"rules":[]}` → Config.AI_product 为 `[]`（非 null，product 键 present） |
| AICE-1-006 | 422 防泄漏 | 异常参数 | 家族4 | PUT 被拒 → 导出内容与拒绝前完全一致（Version 不变） |

### 8.3 测试场景详细设计

#### 8.3.1 AICE-1-001：首拉（正常参数，家族8）

##### 设计思路

导出字段的存在性、取值与**文本形态**三重断言（#102：仅反序列化值比较会静默放过序列化形态错误）。

##### 执行步骤

1. PUT 2 条规则（一条全显式参数、一条仅 name+cond 依赖默认值回填）。
2. GET `/inner-api/v1/configs/ai-cache-rule`。
3. 断言 `Config.AI_product` 长度 2、顺序=提交顺序；每规则恰含 5 键 `{cond, cacheKeyStrategy, cacheTTL, maxBodyBytes, maxValueBytes}` 且值正确。
4. 原始响应 body 文本断言含 `"cacheKeyStrategy":"lastQuestion"`、`"cacheTTL":0`、`"maxBodyBytes":1048576`、`"maxValueBytes":1048576`。

##### 预期返回结果

**ErrNum**：200；Version 非空；值与文本形态均符合合同。

---

#### 8.3.2 AICE-1-002：二期字段缺席（合同一致性，家族8）

##### 设计思路

一期不导出的 6 个二期字段（cacheKeyFrom/cacheValueFrom/cacheStreamValueFrom/cacheToolCallsFrom/responseTemplate/streamResponseTemplate）不得出现在导出 JSON 中（含嵌套任意位置），断言作用于原始 body 文本。

---

#### 8.3.3 AICE-1-003：增量（正常参数）

##### 执行步骤

1. 首拉导出，记录 `Version=V1`。
2. 带 `?version=V1` 再拉 → 断言 `Data=null`。
3. 不带 version 再拉 → 断言有数据且 `Version==V1`（内容未变则版本不变）。

---

#### 8.3.4 AICE-1-004：版本单调（并发语义，家族8/#142）

##### 设计思路

版本号为 14 位定宽时间串（`yyyyMMddHHmmss`），字符串比较与时间比较等价。同一秒内连续两次内容变更（PUT A → 导出 v1 → PUT B → 轮询导出）必须产生严格大于 v1 的版本号且内容收敛为 B；防止"同秒碰撞 + sign 短路"楔死版本通道（issue #142 根因模式）。

##### 执行步骤

1. PUT 集合 A → 导出取 `Version=v1`。
2. **同一秒内** PUT 集合 B。
3. 轮询导出（间隔 500ms，最多 10 次）直到 `Version>v1`；断言收敛导出 `Config.AI_product` 与集合 B 逐字段一致。

---

#### 8.3.5 AICE-1-005：清空导出（边界值，家族8）

##### 执行步骤

1. PUT 2 条规则后 PUT `{"rules":[]}`。
2. 导出断言：`Config` 键 `AI_product` **存在**且为 `[]`（空数组非 null）；原始 body 文本含 `"AI_product":[]`。

---

#### 8.3.6 AICE-1-006：422 防泄漏（异常参数，家族4）

##### 执行步骤

1. PUT 合法集合 → 导出取 `Version=V1`（内容快照 S1）。
2. PUT 非法集合（422）。
3. 带 `?version=V1` 再拉 → 断言 `Data=null`（Version 未推进）；不带 version 再拉 → `Version==V1` 且内容仍为 S1。

---

## 9. 依赖与数据准备

1. 本模块为全局单例集合，无跨资源引用（cond 仅做表达式编译校验，不引用 cluster/provider）。
2. 规则 name 使用 run-scoped 唯一命名（`<runID>-ac<用例号>` 前缀 + 随机后缀），避免并发/重跑污染。
3. Inner 导出 product 键名取自测试环境配置 `AIRouteInnerProductName="AI_product"`。
4. AC-1-012 / AC-1-019 需 `AIAPI_MYSQL_DSN`（`go test -tags mysql`）；未设置时自动 Skip。AC-1-019 会等待 10s 命名锁超时后返回 409。

## 10. 注意事项

1. PUT 为全量替换，任一用例不得依赖执行顺序：依赖前置状态的用例先自行 PUT 构造集合。
2. 每个负向用例（422）都必须带"GET 回读零变更"断言（家族3）；涉及数据面下发的另加 Inner 导出防泄漏断言（家族4）。
3. 集合级资源无 `/{id}` 端点，审计身份固定为 `resource_type=ai_cache_rule`、`resource_id=resource_name=ai_cache_rules`。
4. 测试结束后集合内容不再恢复：全量替换语义保证后续用例自包含。
