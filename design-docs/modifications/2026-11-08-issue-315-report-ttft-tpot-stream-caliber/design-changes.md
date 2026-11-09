# Issue #315 设计变更说明：TTFT/TPOT 的流式口径与存量数据处置

> 配套文档：[change-summary.md](./change-summary.md)（背景、根因、方案取向、测试计划）。
> 本文只写「口径怎么落地」：查询表达式、聚合定义、等价性论证、部署/回滚与可选回填。

## 1. 两层口径定义

| 层 | 对象 | 定义（修复后） |
|----|------|---------------|
| 契约层 | `overview.ttft_avg_ms` / `overview.tpot_avg_ms` | `Σ(流式请求 ttft/tpot 微秒) / 流式请求数 / 1000`；流式请求数 = 0 时返回 `0` |
| 契约层 | `timeseries(metric=ttft\|tpot).value` | 同上，按 bucket 独立计算；桶内无流式请求时为 `0` |
| 数据层 | `bfe_ai_metrics_1m.ttft_us_sum` / `tpot_us_sum` | **仅** `ai_stream=1` 行的 `ai_ttft_us` / `ai_tpot_us` 微秒累计（聚合侧改造后成立） |
| 原始层 | `bfe_ai_request_log.ai_ttft_us` / `ai_tpot_us` | 保持不变：BFE 对所有经 body-process 的响应（file/batch 除外）如实记录首包/每 token 耗时，**非流式行可非 0** |

数据层口径是「列表意」，契约层口径是「查询结果语义」。两层解耦是本方案能分两步发布的前提：查询侧把契约层钉死，聚合侧再让数据层与之一致。

## 2. 查询侧改造：四后端表达式

### 2.1 Before / After

MySQL（`storage/mysqlreport/report.go`）、Doris（`storage/dorisreport/report.go`）、StarRocks（`storage/starrocksreport/report.go`）三处 `IFNULL` 方言相同：

```diff
 // overview（mysqlreport/report.go:240,242；dorisreport:234,236；starrocksreport:238,240）
-"IFNULL(SUM(ttft_us_sum),0) AS ttft_us_sum",
+"IFNULL(SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END),0) AS ttft_us_sum",
 "IFNULL(SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END),0) AS stream_requests",
-"IFNULL(SUM(tpot_us_sum),0) AS tpot_us_sum",
+"IFNULL(SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END),0) AS tpot_us_sum",

 // timeseries（mysqlreport/report.go:347-348；dorisreport:354-355；starrocksreport:358-359）
 case ireport.MetricTTFT, ireport.MetricTPOT:
     fields = append(fields,
-        "SUM(ttft_us_sum) AS ttft_us_sum",
-        "SUM(tpot_us_sum) AS tpot_us_sum",
+        "SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END) AS ttft_us_sum",
+        "SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END) AS tpot_us_sum",
         "SUM(CASE WHEN ai_stream=1 THEN request_count ELSE 0 END) AS stream_requests")
```

ClickHouse（`storage/clickhousereport/report.go:291,293` 与 `:417-418`）：别名必须留在 `_total` 命名空间，否则同一查询内 `ttft_us_sum` 的同名别名会遮蔽列名（该文件 `:278-283` 已记录该 CH 规则）：

```diff
-"ifNull(SUM(ttft_us_sum),0) AS ttft_us_sum_total",
+"ifNull(SUM(CASE WHEN ai_stream=1 THEN ttft_us_sum ELSE 0 END),0) AS ttft_us_sum_total",
-"ifNull(SUM(tpot_us_sum),0) AS tpot_us_sum_total",
+"ifNull(SUM(CASE WHEN ai_stream=1 THEN tpot_us_sum ELSE 0 END),0) AS tpot_us_sum_total",
```

不改的部分：分母字段 `stream_requests`、`rowToOverview` 的除式（四后端 overview 各一处，如 `mysqlreport/report.go:712-713`）、`avgLatencyMs`（如 `mysqlreport/report.go:874-878`）、返回结构体字段。

### 2.2 等价性论证（为什么查询侧过滤足够且安全）

1. **行内不混合**：`ai_stream` 是 `bfe_ai_metrics_1m` 的 GROUP BY 维度之一（MySQL `job.go:471-479`、Doris `bfe_ai_metrics_1m_job.sql:78`、StarRocks MV `bfe_ai_metrics_1m.sql:91`、ClickHouse MV `bfe_ai_metrics_1m_mv.sql:89`），且写入口全部归一为 `0/1`（`job.go:494` `IFNULL(ai_stream,0)`；三份 MV `COALESCE(ai_stream,0)`）。⇒ 一个聚合行要么是流式、要么是非流式，`CASE WHEN ai_stream=1` 精确切出流式行的贡献。
2. **与聚合侧口径恒等**：若列已是「流式累计」，`CASE` 对 `ai_stream=1` 行原样放行、其余行置 0，结果不变 ⇒ 两份改动可任意顺序发布，不会重复扣减。
3. **存量污染行被屏蔽**：非流式行即使 `ttft_us_sum > 0`，在 `CASE` 的 `ELSE 0` 分支归零 ⇒ **历史窗口自动正确**，无需等回填或等保留期滚出。
4. **NULL 安全**：明细 `ai_stream` 可空，但聚合侧写入口不会落下 NULL（MySQL 列 `NOT NULL DEFAULT 0`，Doris/StarRocks/ClickHouse 写入口以 `COALESCE(ai_stream,0)` 归一）；即便未来出现 NULL，`CASE` 的 `ELSE 0` 也按非流式处理，与既有 `COALESCE` 语义一致。
5. **分母不受影响**：`stream_requests` 原本就只统计 `ai_stream=1` 的 `request_count`，本次不动 ⇒ 修复前后分母恒等，仅分子单调不增，均值单调不增，不会出现负值/NaN/除零（`avgLatencyMs` 与 overview 的 `if streamRequests > 0` 短路保持）。

### 2.3 为何不用 WHERE 过滤 / 不新增列

- **WHERE 不可行**：overview 同一条 SQL 还要返回 `request_total`、`error_rate`、token、cost、`latency_max` 等全量口径，整体加 `ai_stream=1` 会把这些指标一起改错。条件聚合是唯一能在「同一扫描」里表达两种范围的方式。
- **新增 `ttft_us_stream_sum` 列（否决）**：需要四后端 DDL + 三仓 MV/JOB + 双写过渡 + 查询侧切换，成本远大于条件聚合，且不解决存量行；收益（保留「全行 ttft 累计」这一非契约指标）无消费方。

## 3. 聚合侧改造：四份资产与部署

### 3.1 表达式

| 资产 | 行 | 表达式 |
|------|----|--------|
| `ai-gateway-api/storage/mysqlreport/job.go` | 535-536 | `SUM(CASE WHEN ai_stream=1 THEN IFNULL(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum` / `…ai_tpot_us… AS tpot_us_sum` |
| `ai-gateway-observability/doris/sqls/bfe_ai_metrics_1m_job.sql` | 55-56 | `SUM(CASE WHEN COALESCE(ai_stream,0)=1 THEN COALESCE(ai_ttft_us,0) ELSE 0 END) AS ttft_us_sum`（`tpot` 同构） |
| `ai-gateway-observability/clickhouse/sqls/bfe_ai_metrics_1m_mv.sql` | 70-71 | `sumIf(ai_ttft_us, ai_stream=1) AS ttft_us_sum` / `sumIf(ai_tpot_us, ai_stream=1) AS tpot_us_sum` |
| `ai-gateway-observability/starrocks/sqls/bfe_ai_metrics_1m.sql` | 70-71 | 同 Doris JOB 写法（StarRocks 支持 `CASE`/`COALESCE`） |

ClickHouse 侧 `sumIf(...)` 与 `sum(CASE WHEN … THEN … ELSE 0 END)` 语义等价；若希望四份资产字面统一，可统一采用 `CASE` 写法——二选一，并在资产注释里钉死口径（当前 CH 文件其余指标均为 `sum(...)` 形式）。

### 3.2 部署与回滚

| 后端 | 生效方式 | 历史数据 | 回滚 |
|------|---------|---------|------|
| MySQL | 重启 API 进程即生效（JOB 在 API 进程内装配：`stateful/container/rdb/components.go:541`，`Job` 每分钟按 `DELETE + INSERT … SELECT` 幂等重算单分钟窗口，`job.go:448-556`） | 不变，仍为全行累计（查询侧已兜底） | 回退代码 |
| Doris | `DROP JOB WHERE JobName = 'bfe_ai_metrics_1m_job'` 后重新执行资产建 JOB（Doris 不支持就地改 JOB 的 DO 体；先例见 `doris/docs/user/HOWTO.md:275`） | 不变 | 重走同一流程建回旧 JOB |
| ClickHouse | `DROP MATERIALIZED VIEW bfe_ai_metrics_1m_mv` 后重新执行 `bfe_ai_metrics_1m_mv.sql`（MV 无 `POPULATE`，只对新建明细生效） | 不变（7 天 TTL 自然滚出，或按 §4.3 回填） | 同左 |
| StarRocks | `DROP MATERIALIZED VIEW` 后重新执行 `bfe_ai_metrics_1m.sql`；重建后刷新从基表重算 | **会自动修正**：异步 MV 刷新按基表重算，重建后 `partition_ttl=7 DAY` 内的历史分区回到流式口径——需在 QA 显式验证（见 §5） | 同左 |

发布顺序：查询侧（本仓）先发，可独立关闭 issue；聚合侧随后。二者无相互阻塞，且（§2.2 第 2 条）任意顺序都保持一致。

### 3.3 同步的文档

- `ai-gateway-observability` 三份 `docs/design/TABLE_DESIGN.md` 的指标口径段：注明 `ttft_us_sum` / `tpot_us_sum` 仅流式行贡献；
- 本仓 `db_ddl_report_mysql.sql:288-289` 列注释、`design-docs/sys-design/报表库设计文档.md:207`、`api-define/…/report.md`（见 change-summary §2.4）。

## 4. 可选存量回填（只为直连消费方，非关闭 issue 的必要条件）

前提：四张聚合表**都是按维度键做加和**的语义（MySQL 普通表靠 JOB 的 DELETE+INSERT 保幂等；Doris `AGGREGATE KEY`、ClickHouse `SummingMergeTree(… ttft_us_sum …)`、StarRocks MV 均按 key 合并），因此**任何回填都必须先删窗口再插**，否则会与原值叠加翻倍。窗口粒度用「天」即可：聚合行本身按分钟分桶，按天 `GROUP BY ts_min` 重算与 JOB 的分钟窗口结果一致。

### 4.1 MySQL

优先复用代码而不是手写 SQL：写一个包内一次性工具（`storage/mysqlreport` 内的 `_test.go` 或临时脚本），按天循环调用 `buildAggregateDeleteSQL("bfe_ai_metrics_1m")` 与 `buildAggregateInsertSQL("bfe_ai_request_log","bfe_ai_metrics_1m")`（`job.go:448,486`），把窗口参数换成 `[D, D+1)`。这样 `DATE_FORMAT` 分桶、JSON 打平、维度归一全部沿用 JOB 逻辑，避免手写 SQL 与代码漂移；单次回放按天提交，失败可重跑（DELETE 先执行，幂等）。

### 4.2 Doris

```sql
-- 逐天（低峰执行；DELETE 在 AGGREGATE KEY 表上可用）
DELETE FROM bfe_ai_metrics_1m WHERE ts_min >= '2026-11-08 00:00:00' AND ts_min < '2026-11-09 00:00:00';
INSERT INTO bfe_ai_metrics_1m SELECT <与 JOB 完全一致的 SELECT> FROM bfe_ai_request_log
WHERE log_time >= '2026-11-08 00:00:00' AND log_time < '2026-11-09 00:00:00' GROUP BY <40 维>;
```

### 4.3 ClickHouse

```sql
ALTER TABLE bfe_ai_metrics_1m DELETE WHERE ts_min >= '2026-11-08 00:00:00' AND ts_min < '2026-11-09 00:00:00';
INSERT INTO bfe_ai_metrics_1m SELECT <与 MV 完全一致的 SELECT> FROM bfe_ai_request_log
WHERE log_time >= '2026-11-08 00:00:00' AND log_time < '2026-11-09 00:00:00' GROUP BY <40 维>;
```
注意 ClickHouse mutation 为异步，需等 `system.mutations` 中该表无未完成 mutation 再 INSERT。

### 4.4 StarRocks

重建 MV（§3.2）即触发从基表重算，无需单独回填；验证方式是抽样比对某个历史分钟桶的 `ttft_us_sum` 与该桶流式明细行求和。

## 5. 测试矩阵与断言写法

| 层 | 位置 | 关注点 |
|----|------|--------|
| 单测（查询侧） | 四后端 `report_test.go` 的 overview / timeseries 期望串 | 期望串同步新表达式；补 `assert.NotContains(…, "SUM(ttft_us_sum)")` 钉死回归方向；CH 断言 `_total` 别名 |
| 单测（聚合侧） | `storage/mysqlreport/job_test.go` | 断言 `buildAggregateInsertSQL` 含条件聚合口径；既有「64 列 / 别名同序」正则断言（`job_test.go:38-67`）必须继续通过——条件聚合表达式内无 ` AS `，不会破坏别名解析 |
| 单测（纯函数） | 四后端 `rowToOverview` / `rowToMetricPoint` / `avgLatencyMs` | **不做改动**即通过，证明本次只改输入口径、未动除式 |
| 集成 | `test/integration/tests/report/query/` 四后端 | 新增「非流式聚合行 `ttft_us_sum>0`」种子（生产真实污染形态）；断言 overview 流式值与 timeseries 逐桶值；同时断言 `request_total` 等全量字段仍包含该行（防止改错范围） |
| 集成（MV 臂） | StarRocks / ClickHouse 种子走明细 | CH 组 2 直灌聚合种子（含行 F，同 MySQL 组口径）；SR 组 2 保持 6 行明细单源种子不动（追加会级联破坏 logs 断言，见 change-summary §7.2），SR 查询路径由组 1 覆盖；聚合侧改造完成后跑同一用例必须同结果（幂等性验证） |
| 手工验证（StarRocks 历史修正） | QA 环境 | 重建 MV 后抽样历史桶：`ttft_us_sum` 应等于该桶流式明细求和（验证 §3.2 的「自动修正」假设，若不成立则降级为 §4.2/§4.3 式回填） |

## 6. 未采用方案记录

| 方案 | 描述 | 否决理由 |
|------|------|---------|
| C. 数据面过滤 | BFE 对非流式不写 `ai_ttft_us` / `ai_tpot_us` | 丢失真实观测（非流式首包延迟）、需数据面全量发布、不修历史明细；口径归属应在聚合/查询层 |
| D. 新增流式专用列 | 聚合表加 `ttft_us_stream_sum` / `tpot_us_stream_sum` | 四后端 DDL + 三仓 MV/JOB + 双写过渡 + 查询切换，成本远超条件聚合，且无消费方需要「全行 ttft 累计」 |
| E. 只改分母 | 把分母改成全量请求数 | 违背 `report.md §2.2`「聚合于 stream 请求」，且会把非流式的首包语义混入每 token 延迟，指标含义被破坏 |
| F. 查询侧过滤 + 依赖明细表重算 | overview 直接从明细表 `SUM` 流式行 | MySQL/Doris/ClickHouse/StarRocks 均需扫明细（7 天窗口、99 列），违背「时序读聚合表」的既有设计（`report.md §2.1` 约束）；慢查询与降级风险高 |
