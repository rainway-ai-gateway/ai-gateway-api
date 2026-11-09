# Issue #315 变更摘要：Report overview / timeseries 的 TTFT、TPOT 收敛为流式口径

> 对应上游 Issue：[rainway-ai-gateway/ai-gateway-api#315](https://github.com/rainway-ai-gateway/ai-gateway-api/issues/315)（SC2201-TC005：Report overview 的 TTFT/TPOT 均值把非流式行的首包延迟计入分子，混合流量下均值被放大）

## 1. 背景

### 1.1 问题现象

`GET /open-api/v1/report/overview` 的 `ttft_avg_ms` / `tpot_avg_ms` 在存在非流式请求的窗口里系统性偏高，偏差随非流式占比单调放大：

| run | overview 实际 | 按 `ai_stream=1` 明细行重算（Oracle） | 放大倍数 |
|-----|--------------|-----------------------------------|---------|
| `20261107-194156.768275483-sc2201tc005` | `7.082` / `0.875` | `2.609` / `0.342` | 2.71× / 2.56× |
| `20261108-182516-sc2201tc005` | `5.413` / `0.647` | `1.885` / `0.258` | 2.87× / 2.51× |

两次 run 在断言前均已轮询到 `request_total=logs_total=3`（聚合收敛），失败点固定为「overview 与明细流式口径对照」，非请求/鉴权/过滤/清理问题。`metric=ttft|tpot` 的时序点位同源同病。

约定违背：`report.md §2.2` 指标表将 `ttft` / `tpot` 定义为「首 Token / 每 Token 延迟（毫秒，聚合于 stream 请求）」，即**分子只应含流式请求的 `ai_ttft_us` / `ai_tpot_us`**，分母为流式请求数。当前实现分子取全行、分母只算流式行。

### 1.2 根因核实结论（源码直读，与 issue 定位一致并补齐跨仓链路）

| # | 位置 | 事实 | 核实 |
|---|------|------|------|
| 1 | `storage/mysqlreport/job.go:535-536` | 分钟聚合 JOB 以 `SUM(IFNULL(ai_ttft_us,0))` / `SUM(IFNULL(ai_tpot_us,0))` 对窗口内**全部**明细行求和，无 `ai_stream` 过滤 | ✅ 实锤（直读） |
| 2 | `storage/mysqlreport/report.go:240-242`、`:712-713` | overview 分子 `IFNULL(SUM(ttft_us_sum),0)` 取全行；分母 `SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END)` 只算流式行 → 分子分母口径错配 | ✅ 实锤（直读） |
| 3 | `storage/mysqlreport/report.go:345-349`、`:860`、`:874-878` | timeseries `ttft`/`tpot` 同一组字段；`avgLatencyMs(usSum, streamRequests)` 同一除式 | ✅ 实锤（直读） |
| 4 | `storage/dorisreport/report.go:234,354,776`；`storage/starrocksreport/report.go:238,358,778`；`storage/clickhousereport/report.go:291,417,827` | 另三后端字段清单与除式与 MySQL 完全同构 → **四后端同时命中**，与 issue「是否同样实现待确认」的悬念收敛为「已确认同构」 | ✅ 实锤（直读） |
| 5 | `ai-gateway-observability/doris/sqls/bfe_ai_metrics_1m_job.sql:55-56`、`clickhouse/sqls/bfe_ai_metrics_1m_mv.sql:70-71`、`starrocks/sqls/bfe_ai_metrics_1m.sql:70-71` | 三份聚合定义同样是「全行求和」口径 → `ttft_us_sum` 列在**所有**部署形态下都已污染 | ✅ 实锤（直读） |
| 6 | `bfe/bfe_modules/mod_body_process/mod_body_process.go:159-160,199-214` | `readResponseHandler` 对任意响应首包置 `TFirstToken`；`calcTokenTime` 无条件计算 `TTFT = TFirstToken - TReqEnd`（仅 file/batch 模式跳过）→ 非流式请求照样有 TTFT/TPOT | ✅ 实锤（直读） |
| 7 | `bfe/bfe_modules/mod_access_pb3/request_log.go:627-634` | `Ti.TTFT > 0` / `Ti.TPOT > 0` 即写 `AiTtftUs` / `AiTpotUs`，**不判 `ai_stream`** | ✅ 实锤（直读） |
| 8 | `report.md:90` | 「`ai_ttft_us` / `ai_tpot_us` … 未设置（非流式）为 **0**」——与 6、7 条事实相反，是该 bug 的「文档放大器」：它把「非流式行贡献为 0」写成契约事实，掩盖了聚合侧必须过滤 | ❌ 文档失真（本次一并修正） |
| 9 | `db_ddl_report_mysql.sql:244`、`doris/sqls/bfe_ai_metrics_1m.sql:9`（JOB 侧 `COALESCE(ai_stream,0)`）、`clickhouse/sqls/bfe_ai_metrics_1m.sql:24`、`starrocks/sqls/bfe_ai_metrics_1m.sql:27` | 聚合表 `ai_stream` 恒为 `0`/`1`（MySQL `NOT NULL DEFAULT 0`，其余由 MV/JOB `COALESCE` 归一），且 `ai_stream` 在 40 个 GROUP BY 维度之内 → **单个聚合行内不存在流/非流混合** | ✅ 实锤（直读） |

**等价性推论（后续方案的地基，来源为第 9 条）**：聚合表按 `(ts_min, …, ai_stream, …)` 分组，流式与非流式落在**不同行**。因此在查询侧对 `ttft_us_sum` / `tpot_us_sum` 加 `ai_stream=1` 过滤，与在聚合侧只累加流式行，二者结果恒等；且前者的正确性不依赖聚合列的既有内容（存量污染行会被过滤掉）。

**排除项**：
- 「窗口混入其他 run 的请求」——两次 run 均先轮询到 `request_total=logs_total=3` 才断言；
- 「分母多算」——分母口径与 `report.md §2.2` / `报表查询模块` 文档一致，本次不改分母；
- 「聚合未收敛」——见 `report.json` 的 `logs_total` 收敛前置断言。

### 1.3 契约依据与裁定需求

- `report.md §2.2`（当前 Release `0.5.0 / approved`，`normative`）：`ttft` / `tpot` = 「首 Token / 每 Token 延迟（毫秒，**聚合于 stream 请求**）」→ 分子范围已明确，**无需产品裁定**。
- `report.md §2.1` 只在返回示例中给出 `ttft_avg_ms` / `tpot_avg_ms`，约束段落未写口径 → 本次补齐（见 §2.4）。
- issue 原文「修复范围（聚合 JOB 侧过滤，还是查询侧过滤）由 owner 决定」：本文给出决策建议与理由（§2.1），需 owner 确认。**除此之外无待裁定事项**。

---

## 2. 修复方案

### 2.1 总体策略：查询侧收敛为主（本仓发布），聚合侧归位为辅（跨仓）

issue 推荐的顺序是「先改 `job.go` 聚合」。本方案调整为主次顺序，理由是发布可行性与「能否关闭 issue」：

| 方案 | 覆盖面 | 存量污染数据 | 跨仓依赖 | 结论 |
|------|--------|------------|---------|------|
| **A. 查询侧过滤**（四后端 overview + timeseries 加 `ai_stream=1`） | API 全部读取路径 | **天然正确**（污染行被过滤掉） | 无（全在本仓） | ✅ **release blocker，先落地** |
| **B. 聚合侧过滤**（`job.go` + observability 三份 SQL 资产） | 聚合列本身，惠及直连 BI / ad-hoc SQL | 不改历史：MySQL 需按窗口回放、Doris 需重跑 JOB、ClickHouse MV 无法回算历史 | 跨 2 仓 + 三套部署形态 | ✅ **辅助项，随后跟进**（§2.3） |
| C. 数据面过滤（BFE 对非流式不写 `ai_ttft_us` / `ai_tpot_us`） | 明细表 + 所有下游 | 不改历史明细 | 跨 `bfe` 仓 + 数据面全量发布 | ❌ 否决 |

**否决 C 的理由**：非流式的「首包耗时」是 BFE 真实测得的观测事实（`mod_body_process.go:159-160`），抹掉它会丢失有价值的数据（非流式首字节延迟分析），且这是一次数据面发布，收益（省掉一次 SQL 改动）远小于成本与风险；口径归属本就应落在聚合/查询层。

**只做 B 不做 A 不可行的理由**：存量聚合行已污染，B 上线当刻 overview 仍返回错误值，issue 无法关闭；`report.md` 的验收判据 1/2 会继续失败直到历史窗口滚出保留期。

**A 与 B 并存的安全性**：B 上线后 `ttft_us_sum` 列已是流式口径，A 的 `CASE WHEN ai_stream=1` 变成幂等过滤（流式行原样保留、非流式行为 0），不会重复扣减。两者可独立发布、互不阻塞。

### 2.2 改动 1：本仓查询侧口径收敛（四后端，8 处）

字段表达式改造（分子仅取流式行；分母、除式、`rowToOverview` / `rowToMetricPoint` 结构均不变）：

| 后端 | overview 字段 | timeseries 字段 | 新表达式 |
|------|--------------|----------------|---------|
| MySQL | `storage/mysqlreport/report.go:240,242` | `:347-348` | `IFNULL(SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END),0) AS ttft_us_sum`（`tpot_us_sum` 同构） |
| Doris | `storage/dorisreport/report.go:234,236` | `:354-355` | 同上（`IFNULL` 为 Doris 合法函数，与既有字段同一写法） |
| StarRocks | `storage/starrocksreport/report.go:238,240` | `:358-359` | 同上 |
| ClickHouse | `storage/clickhousereport/report.go:291,293` | `:417-418` | `ifNull(SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END),0) AS ttft_us_sum_total`（别名必须为 `_total` 后缀——CH 别名遮蔽同名列，见 `clickhousereport/report.go:278-283` 既有注释） |

保持不变的部分（避免回归）：
- 分母 `SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END) AS stream_requests`（四后端 `report.go` overview 与 timeseries 各一处）；
- 除式 `float64(ttftUsSum)/float64(streamRequests)/1000` 与 `avgLatencyMs`（`mysqlreport/report.go:712-713,874-878`，其余三后端对应行）；
- `ttft` 与 `tpot` 的 timeseries 共用同一组字段（`case ireport.MetricTTFT, ireport.MetricTPOT:`），一次改动同时覆盖两个 metric。

**为何加在字段而非 WHERE**：overview 同一条 SQL 还要返回 `request_total` / `error_rate` / token / cost 等全量口径，不能整体加 `ai_stream=1` 过滤；只有 TTFT/TPOT 两列需要流式范围，故用条件聚合表达。

### 2.3 改动 2：聚合口径归位（跨仓，`ai-gateway-observability` + 本仓 JOB）

目标：让 `ttft_us_sum` / `tpot_us_sum` 两列自解释为「**流式请求** ttft/tpot 微秒累计」，避免直连表 / BI / 未来聚合消费方再次踩坑。

| 资产 | 位置 | 新表达式 |
|------|------|---------|
| MySQL 聚合 JOB | `ai-gateway-api/storage/mysqlreport/job.go:535-536` | `SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum`（`tpot` 同构） |
| Doris 聚合 JOB | `ai-gateway-observability/doris/sqls/bfe_ai_metrics_1m_job.sql:55-56` | `SUM(CASE WHEN COALESCE(ai_stream,0)=1 THEN COALESCE(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum`（`tpot` 同构） |
| ClickHouse MV | `ai-gateway-observability/clickhouse/sqls/bfe_ai_metrics_1m_mv.sql:70-71` | `sumIf(ai_ttft_us, ai_stream=1) AS ttft_us_sum`（`tpot` 同构） |
| StarRocks MV | `ai-gateway-observability/starrocks/sqls/bfe_ai_metrics_1m.sql:70-71` | `SUM(CASE WHEN COALESCE(ai_stream,0)=1 THEN COALESCE(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum`（`tpot` 同构） |

注意点：
- 明细表 `ai_stream` 可空（`db_ddl_report_mysql.sql:163`、Doris/StarRocks 明细同为可空），`CASE` 的 `ELSE 0` 分支已把 NULL 归入非流式，无需额外 `IFNULL`；Doris/StarRocks 显式 `COALESCE` 是为与文件内既有归一风格一致。
- MySQL `job.go` 的 `TestBuildAggregateInsertSQL_Structure` 用正则解析 ` AS <alias>`，条件聚合表达式内无 ` AS `，不会破坏别名顺序断言（需补一条口径断言，见 §3.1）。
- 部署与回滚、可选存量回填：见 `design-changes.md` §3、§4。

### 2.4 改动 3：文档修订（与代码同卡落地）

| 文档 | 位置 | 修订 |
|------|------|------|
| `api-define/OpenAPI接口定义/report.md` | §1.2 明细行 `ai_ttft_us` / `ai_tpot_us` 行（第 90 行） | 删除「未设置（非流式）为 0」的错误断言，改为：**BFE 对所有经 body-process 的响应（含非流式，file/batch 除外）都会记录首包与每 token 耗时；按流式口径统计时必须叠加 `ai_stream=1`** |
| 同上 | §2.1 约束 | 新增口径行：`ttft_avg_ms = Σ(ai_stream=1 行的 ttft_us_sum) / 流式请求数 / 1000`，`tpot_avg_ms` 同构；分母为 0 时返回 `0` |
| 同上 | §2.2 指标表 `ttft` / `tpot` 行 | 明确到可断言粒度：**分子只取 `ai_stream=1` 的明细/聚合行，分母为流式请求数；仅含非流式行的 bucket 返回 `0`** |
| `sys-design/details/报表查询模块.md` | 第 106 行「指标口径常量」 | TTFT/TPOT 口径补「分子在 SQL 侧以 `CASE WHEN ai_stream=1` 收敛，与分母 `stream_requests` 同范围」 |
| `sys-design/报表库设计文档.md` | 第 207 行 24 个 SUM 指标列 | `TTFT/TPOT 累计` 注明「仅 `ai_stream=1` 行贡献（JOB/MV 侧条件聚合）」 |
| `db_ddl_report_mysql.sql` | 第 288-289 行 `ttft_us_sum` / `tpot_us_sum` 列 | 加列注释：`-- 流式请求 ttft/tpot 微秒累计（仅 ai_stream=1 行贡献）`（纯注释，无结构变更） |

### 2.5 修复后行为对照

| 场景（同窗口） | 修复前 | 修复后 |
|---------------|--------|--------|
| 2 条非流式（`ai_ttft_us`=1000/2000μs）+ 1 条流式（2609μs），流式 1 条 | `ttft_avg_ms=5.609`（(1000+2000+2609)/1/1000） | **`2.609`**（仅流式行 ÷ 1） |
| 纯流式窗口 | 正确 | 不变（回归守护点） |
| 纯非流式窗口 | `0`（分母 0 走 `avgLatencyMs` 短路） | `0`（分子被过滤 + 分母 0，双保险） |
| `metric=ttft&dimension=stream` | 非流式序列值 `0`（分母 0 短路） | `0`（分子也已为 0） |
| `request_total` / `error_rate` / token / cost / `latency_avg_ms` / `latency_max_ms` / 缓存·镜像·意图组 | — | **完全不变**（不涉及这两列） |

---

## 3. 测试计划

### 3.1 单元测试

| 包 | 用例 | 断言 |
|----|------|------|
| `storage/mysqlreport` | 既有 `overviewFields` 期望串常量（`report_test.go:74-88`，由 `TestBuildOverviewMetricsSQL_*` 使用） | 同步为新的条件聚合表达式；`stream_requests` 字段不变 |
| `storage/mysqlreport` | 既有 `TestBuildTimeSeriesSQL_AllMetrics` 的 TTFT 臂期望串（`report_test.go:171-176`） | 同上 |
| `storage/mysqlreport` | 新增 `TestOverviewTTFTStreamCaliber` | overview 字段串含 `SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END)` 且**不含**裸 `SUM(ttft_us_sum)`（用 `assert.NotContains` 钉死回归方向） |
| `storage/mysqlreport` | `job_test.go` 新增口径断言 | `buildAggregateInsertSQL(...)` 含 `SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum`，且既有 64 列/别名顺序断言仍过 |
| `storage/dorisreport` / `starrocksreport` / `clickhousereport` | 三个包各自的 overview / timeseries 期望串用例 | 同 MySQL 的两条断言（CH 断言 `AS ttft_us_sum_total` 别名形态） |
| `storage/*report` | 既有 `TestOverviewResultFromRow`（`mysqlreport/report_test.go:457-521`）/ `TestRowToMetricPoint`（`:539-575`） | 不改动即应通过——证明「只改输入口径，不改除式」 |

`make test-model` 不覆盖 storage；改后跑 `go test ./storage/...` 与全量 `make test`。

### 3.2 集成测试（`test/integration/tests/report/query/`）

核心种子设计：**新增一条「污染形态」非流式聚合行**——现有种子（`seed_test.go:19-81`，聚合行 B、E）里非流式行的 `ttft_us_sum`/`tpot_us_sum` 都是 0，因此当前用例在修复前后都会通过（无法守住本 bug）。需要补的是「非流式行 `ttft_us_sum > 0`」这一生产真实形态：

| 后端 | 灌数方式 | 新增种子 | 断言 |
|------|---------|---------|------|
| MySQL（`query_test.go`） | 直灌聚合表 | 桶 10:01 增加一条 `ai_stream=0`、`request_count=20`、`ttft_us_sum=40000`、`tpot_us_sum=4000` 的行；另建桶 10:03 只含非流式行且 `ttft_us_sum>0` | overview `ttft_avg_ms = 225000/450/1000`（**不受污染行影响**，手算注释同步更新）；`request_total` 等全量字段仍含该行；timeseries 10:03 桶 `value=0`（非空、非零） |
| Doris（`doris_dimensions_test.go` / 新增用例） | 直灌聚合表 | 同上 | 同上 |
| StarRocks（`starrocks_seed_test.go`） | 灌明细 + MV 回算 | 明细表加一条 `ai_stream=0`、`res_status_code=200`、`ai_ttft_us=40000`、`ai_tpot_us=4000` 的行 | 同上——该臂同时端到端复现了 MV 侧污染，是「验收判据 4（四后端一致）」的直接证据 |
| ClickHouse（`clickhouse_seed_test.go`） | 灌明细 + MV 回算 | 同上 | 同上 |

> StarRocks / ClickHouse 臂的期望值与「MV 是否已按流式口径改造」无关（查询侧过滤兜底）；聚合侧改造完成后再跑一遍仍应得到同一结果，用于验证 §2.3 的幂等性。

### 3.3 E2E / 验收判据

对照 issue 四条验收判据：

1. 「两条非流式 + 一条流式」后 overview 的 `ttft_avg_ms` / `tpot_avg_ms` 等于仅按 `ai_stream=1` 明细重算值（容差按 μs→ms 精度）；
2. `metric=ttft|tpot` 每 bucket 值 = 桶内流式行和 ÷ 流式请求数，仅含非流式行的桶为 `0`；
3. 纯流式窗口与修复前一致，`request_total` / `error_rate` / token / cost / `latency_avg_ms` / `latency_max_ms` 不回归；
4. MySQL / Doris / StarRocks / ClickHouse 四后端对同一 fixture 给出一致流式口径（四者均读 JSON 同构但列名方言不同的 `bfe_ai_metrics_1m`，本仓查询侧改动已保证一致）。

落地动作：requeue SC2201-TC005，`query-report-endpoints` 阶段 `A-report-query` 由 `FAILED_PRODUCT` 转 PASS；同场景复跑确认倍数从 2.7× 收敛到 1.0×。

---

## 4. 兼容性、迁移与风险

| 项 | 结论 |
|----|------|
| 响应结构 | 无变化；仅 `ttft_avg_ms` / `tpot_avg_ms` 与 `metric=ttft\|tpot` 的 `value` 数值下降（单调不增） |
| 是否可能变负 / NaN | 否：分子恒 ≥ 0，分母 ≤ 0 时走既有短路返回 `0` |
| 存量聚合数据 | **无 API 影响**（查询侧过滤已生效）；直连表 / BI 消费方在 §2.3 上线前仍会看到污染值，需在发布说明中提示。可选回填步骤见 `design-changes.md §4` |
| 性能 | 条件聚合与等值过滤等价，扫描行数、索引使用、返回行数均不变；无新增子查询/ JOIN |
| 数据面 | 零改动（BFE 继续如实记录 TTFT/TPOT） |
| 明细字段语义 | `ai_ttft_us` / `ai_tpot_us` 非流式行**保持非 0**（事实记录），文档按 §2.4 纠正，不再承诺「非流式为 0」 |
| Web 控制台 | 零改动（消费 `ttft_avg_ms` / `tpot_avg_ms`，无需适配） |
| 发布顺序 | A（本仓）先发即可关闭 issue；B（observability 仓三份 SQL + Doris JOB / CH·SR MV 重建）独立排期，回滚只需还原对象定义 |

---

## 5. 关联风险与后续事项

1. **跨仓跟进（P1）**：`ai-gateway-observability` 的 `doris/sqls/bfe_ai_metrics_1m_job.sql`、`clickhouse/sqls/bfe_ai_metrics_1m_mv.sql`、`starrocks/sqls/bfe_ai_metrics_1m.sql` 三份资产 + 各自 `docs/design/TABLE_DESIGN.md` 指标口径说明；Doris 需 `DROP JOB` + `CREATE JOB`（先例见 `doris/docs/user/HOWTO.md:275`），ClickHouse / StarRocks 需 `DROP MATERIALIZED VIEW` + 重建。
2. **可选存量回填**：不改历史也能让 API 正确，回填只服务直连消费方；按后端分档的成本与步骤见 `design-changes.md §4`。MySQL 可直接复用 JOB 的 `DELETE + INSERT … SELECT` 幂等窗口语义按分钟回放。
3. **直连消费方排查结论**：本仓与 `ai-gateway-observability` 的 Grafana 资产中**未发现**消费 `ttft_us_sum` / `tpot_us_sum` 的 panel（grep 全仓确认），故 §2.3 的收益主要是数据口径 hygiene 与未来消费方安全。
4. **`ai_stream` 语义依赖**：查询侧过滤依赖「聚合表 `ai_stream` ∈ {0,1}」这一事实（§1.2 第 9 条）。若未来某链路写入 NULL，`CASE … ELSE 0` 会把它当非流式处理——与 `COALESCE(ai_stream,0)` 的既有归一语义一致，不引入新歧义。
5. **`dimension=stream` + `metric=ttft`**：修复后非流式分组序列恒为 `0`（修复前是「分子非 0 但分母 0 短路」）。前端若曾把该序列展示为「非流式首包延迟」，需知会产品——按 `report.md §2.2` 该序列本就无定义。

## 6. 状态

- [x] 修复方案设计（本文档 + `design-changes.md`）
- [x] 根因源码直读核实（四后端查询侧 + 三仓聚合定义 + BFE 字段写入链路）
- [x] 代码实现（A）：四后端 overview / timeseries 共 8 处字段表达式
- [x] 代码实现（B，本仓）：`storage/mysqlreport/job.go` 聚合口径
- [x] 文档修订：`report.md` §1.2/§2.1/§2.2、`报表查询模块.md`、`报表库设计文档.md`、`db_ddl_report_mysql.sql` 列注释
- [x] 单元测试（四后端 `TestTTFTTPOTStreamCaliber` 口径钉死 + `job_test.go` 聚合口径断言 + 既有期望串同步；`go test ./...` 全量零失败）
- [x] 集成测试（MySQL/Doris/SR 组 1/CH 组 2 共用种子加行 F；新增 `TestTimeSeries_TTFTTPOT`；模块编译 `go vet` 通过——真实库执行待 QA/CI 环境）
- [ ] owner 确认方案取向（查询侧为主 A + 聚合侧归位 B，否决数据面改法 C）——实现已按此进行，确认后可合入
- [ ] 跨仓跟进（observability 仓三份 SQL 资产 + Doris JOB / CH·SR MV 重建 + TABLE_DESIGN 口径说明）
- [ ] QA requeue SC2201-TC005 转 PASS 并关闭 issue #315

## 7. 实施校准记录（实现过程中确认的事实与取舍）

1. **污染行的最终形态**：共享聚合种子（`seed_test.go` 行 F）放在 **10:01 与流式行 D 同桶**，维度签名全新（`key-9` / `llama-3` / `anthropic`），`request_count=20`、`ttft_us_sum=40000`、`all_time_sum=2000`（组均值 100ms，latency_avg/max 不变）。同桶设计使一条种子同时覆盖两条判别路径：overview 窗口级（修复前 (225000+40000)/450/1000=0.589ms → 修复后 0.5ms）与 timeseries 桶级（10:01 修复前 (25000+40000)/50/1000=1.3ms → 修复后 0.5ms）。级联手算面：request_total 660→680、status 200=480、mirror 0=530、cache/intent 空桶=30、protocol 新增 anthropic=20、行 E 空币种点合并（cost 时序点数不变）。
2. **StarRocks 组 2（真实 SR + 异步 MV）未追加污染明细行**：该臂是 6 行明细单源种子，logs 端点大量断言（总数 6、分页 2/2/2、过滤清单）与 MV 派生手算值绑定，追加第 7 行会级联破坏且收益有限——SR 查询侧代码路径已由组 1（同 SQL 方言、真实执行于 MySQL 实例）+ 四后端单测覆盖；MV 层聚合口径属 observability 仓改动，随跨仓任务验收（重建 MV 后抽样比对历史桶）。
3. **验收判据 2 的「仅含非流式行的 bucket」形态**：行 F 与流式行同桶后，种子不再覆盖该形态；该行为修复前后均为 `0`（`avgLatencyMs` 分母短路），本次改动不触碰该路径，由 SC2201-TC005 E2E 复跑兜底。
4. **单测断言口径**：overview 的 `stream_requests` 字段带 `IFNULL(...)` 包裹，`Contains` 断言必须写成 `...END),0) AS stream_requests`（初版漏掉 `,0)` 导致四后端同红，已修正）。
5. **CHANGELOG**：v0.0.11 之后无 Unreleased 段，惯例为发布时集中补录（参照 `aa3a504`），本次不单独添加。
