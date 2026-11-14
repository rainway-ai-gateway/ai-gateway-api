# 报表查询（Report）集成测试设计文档

## 1. 模块概述

报表查询模块为 `ai-gateway-api` 提供统一的报表查询 API（`/open-api/v1/report/*`），查询后端通过 `[Report].Backend` 配置切换 MySQL（轻量形态，log-reader `mod_log_mysql` 插件落库）或 Doris（标准形态）。时序/排行/分布读分钟聚合表 `bfe_ai_metrics_1m`，明细查询读明细表 `bfe_ai_request_log`；MySQL 形态下聚合表由 api 进程内的分钟聚合 JOB 生成（本测试关闭 JOB，直接向两张表灌确定性种子数据验证查询端点）。

集成测试分三个用例组：

- **组 A（not_assembled，离线必跑）**：`[Report]` 缺省时模块不装配，五个端点整体不注册，请求返回 404。
- **组 B（query，环境变量门控）**：真实 MySQL 8.x 上建专用库、套用项目 DDL、灌确定性种子，验证五端点的口径、分页、过滤与参数校验。
- **组 C（partition，环境变量门控）**：真实 MySQL 8.x 上验证分区维护 JOB 的自动建分区行为（issue #191 回归：配置 `[Report].Database` 后分区查询不匹配 information_schema、JOB 静默降级 DELETE 兜底、log-reader 写入 Error 1526）。
- **组 D（schema，见 `tests/schema/report/`）**：/report/* 的 api-define 契约形状守卫，REPORT_MYSQL_DSN 门控，与组 B 共用 `testutil.StartReportServer` 装配；种子定点成本取非整数金额（RMB 66900 → 0.000669），形状断言（value 为 number）叠加 0<value<1 语义断言锁死成本金额口径（issue #207）。
- **组 E（dedup_key，环境变量门控）**：明细表幂等键的**请求级判别能力**（bfenetworks/bfe#1417）——同秒 + 同 `hostid`/`ai_apikey_id`/`ai_requested_model` 的 3 条并发请求必须各自成行（修复前按 `ON DUPLICATE KEY UPDATE` 全列覆盖只留 1 行、丢率 (n−1)/n）；含"旧四列键对照臂"（同批数据写入旧键镜像表 → 只留 1 行，证明唯一差别就是 `logid`）与"重放仍幂等臂"。见 §9。

## 2. 接口列表

| 编号 | 接口名称 | 方法 | 路径 | 说明 |
|------|----------|------|------|------|
| RP-1 | 总览指标 | GET | `/open-api/v1/report/overview` | 请求总量/错误率/Token/延迟/TTFT/TPOT/成本/限流/认证拒绝/日志总量；缓存（次数/命中率/cache token）/镜像（命中数）/意图（分类数/unknown 率）三组指标 |
| RP-2 | 时序数据 | GET | `/open-api/v1/report/timeseries` | `metric=qps\|tokens\|latency\|ttft\|tpot\|cost\|cache_tokens`（cache_tokens 按 kind 分序列）；可选 `dimension=ai_intent_answer\|ai_cache_status\|mirror_hit`（仅 mysql 后端，doris 422） |
| RP-3 | 维度排行 | GET | `/open-api/v1/report/rankings` | `dimension=model\|requested_model\|provider\|apikey\|host\|status\|protocol\|mode`（+ `ai_intent_answer\|ai_cache_status\|mirror_hit`，仅 mysql），按 request_count 降序 |
| RP-4 | 占比分布 | GET | `/open-api/v1/report/distribution` | `dimension=status\|protocol\|mode\|stream`（+ 上述三个新维度，仅 mysql），空维度归一为 `unknown` |
| RP-5 | 日志明细 | GET | `/open-api/v1/report/logs` | 按 log_time 倒序分页，支持 err_only/keyword/requested_models 及 `cache_status`/`mirror_hit`/`intent_question`/`intent_answer`/`intent_source`/`price_warn` 精确过滤（两后端均可用）；明细行新增 `ai_price_miss`/`ai_price_warn` 两列（issue #1479，列见 `db_ddl_report_mysql.sql`） |

## 3. 测试用例统计

| 场景 | 用例组 | 测试用例数 |
|------|--------|-----------|
| 未装配 404（含未知子路径） | A | 2 |
| overview 口径（含缓存/镜像/意图组与过滤后口径） | B | 3 |
| timeseries（qps/tokens/cost/cache_tokens/维度拆分） | B | 5 |
| rankings（model/status/limit/三个新维度） | B | 6 |
| distribution（status/protocol+unknown/三个新维度） | B | 5 |
| logs（分页/过滤/行形状/requested_models/page_size 封顶/新五参数过滤/新列行形状） | B | 7 |
| 参数校验（422，含 mirror_hit/intent_source/price_warn/timeseries 维度白名单） | B | 9（子用例+4） |
| doris 后端新维度门控 422 + 明细能力放行 | B | 2 |
| 启动即建分区（两表同验） | C | 1 |
| 覆盖期内写入成功 | C | 1 |
| 边界外写入拒绝（Error 1526） | C | 1 |
| 幂等键请求级判别（明细保留 / 旧四列键对照 / 重放幂等） | E | 3 |
| **合计** | | **35+** |

## 4. 认证方式

测试环境配置 `SkipTokenValidate=true`，所有请求无需携带认证头。

## 5. 目录结构

```
report/
├── design.md
├── not_assembled/
│   └── not_assembled_test.go   # 组 A：[Report] 缺省 → 五端点 404
├── query/
│   ├── query_test.go           # TestMain：MySQL 环境准备、DDL、服务装配
│   ├── seed_test.go            # 组 B 种子数据（聚合表 5 行 + 明细表 6 行）
│   └── cases_test.go           # 组 B 用例断言
├── partition/
│   ├── partition_test.go       # TestMain：MySQL 环境准备、DDL（INIT_DATE=昨天）、服务装配
│   └── cases_test.go           # 组 C 用例断言（C-1 建分区 / C-2 覆盖期写入 / C-3 边界外拒绝）
└── dedup_key/
    └── dedup_key_test.go       # 组 E：幂等键请求级判别（明细保留 / 旧四列键对照 / 重放幂等，issue #1417）
```

## 6. 组 A：未装配 404（not_assembled）

### 6.1 设计思路

默认测试 conf（`test/integration/conf/ai_gateway_api.toml`）未配置 `[Report]`，`stateful/container/rdb.initReport()` 直接返回，`container.ReportManager` 为 nil，`endpoints/openapi_v1` 不注册 `/report/*`。请求落到 mux 的 NotFoundHandler，返回 JSON 404 信封（`{"ErrNum":404,"ErrMsg":"Not Found"}`）。

### 6.2 校验点

- `/open-api/v1/report/overview|timeseries|rankings|distribution|logs` 五个端点的 `ErrNum` 均为 404（不依赖请求参数是否合法，路由未注册）。
- `/open-api/v1/report/nonexistent` 未知子路径同样 404。

### 6.3 运行方式

无外部依赖，常态回归：

```bash
go test -v -count=1 ./tests/report/not_assembled/
```

## 7. 组 B：五端点真实数据（query）

### 7.1 环境门控与数据源

> issue #1479：明细表新增 `ai_price_miss`/`ai_price_warn` 两列，`/logs` 增加 `price_warn` 精确过滤（取值白名单 fallback/no_entry/no_baseline/reject/subitem_missing/no_model_table）。本批**只新增非法值 422 用例**（`invalid_price_warn`），正向过滤口径由单元测试 `TestLogWhere_PriceWarn` 锁定、端到端正向用例待各后端种子统一补列后追加（与缓存/镜像/意图字段当时的分批策略一致）。

- 环境变量 `REPORT_MYSQL_DSN`：`user:pass@tcp(host:port)/` 格式（参照 log-reader LR03 的 `LR_MYSQL_DSN` 约定，库名段可省略，不使用其中自带的库名）。
- **未设置时**：`TestMain` 打印设置说明后以 0 退出（等价全组 Skip）；单条用例不执行。
- 账号需具备 `CREATE/DROP DATABASE` 权限。

```bash
REPORT_MYSQL_DSN="root:****@tcp(127.0.0.1:3306)/" go test -v -count=1 ./tests/report/query/
```

### 7.2 数据构造

1. 创建专用随机名数据库 `report_it_<ns>`（先 `DROP DATABASE IF EXISTS` 防御残留）。
2. 套用项目根 `db_ddl_report_mysql.sql`（`--` 注释行剔除、按分号切分逐条执行），`${INIT_DATE}` 替换为**今天+3 天**，使历史时间戳（2026-09-15）落入 `p_init` 分区。
3. 灌种子数据（数值均可手算）：

**聚合表 `bfe_ai_metrics_1m`**（查询窗口 `[2026-09-15 09:59, 10:05)`，≤6h → 60s 桶）：

| 行 | 桶 | 模型 | key | provider | 协议 | 状态 | 流 | err | req | err_cnt | in | out | tot | all_time | ttft_us | tpot_us | 币种 | cost | rl | rej |
|----|----|------|-----|----------|------|------|----|-----|-----|---------|----|----|----|----------|---------|---------|------|------|----|-----|
| A | 10:00 | gpt-4o | key-1 | openai | openai | 200 | 1 | '' | 100 | 0 | 1000 | 200 | 1200 | 10000 | 50000 | 5000 | USD | 100 | 0 | 0 |
| B | 10:00 | gpt-4o | key-1 | openai | openai | 500 | 0 | E500 | 200 | 200 | 2000 | 400 | 2400 | 20000 | 0 | 0 | USD | 200 | 2 | 0 |
| C | 10:00 | gpt-4 | key-2 | azure | openai | 200 | 1 | '' | 300 | 0 | 3000 | 600 | 3600 | 30000 | 150000 | 15000 | RMB | 300 | 0 | 0 |
| D | 10:01 | gpt-4o | key-1 | openai | openai | 200 | 1 | '' | 50 | 0 | 500 | 100 | 600 | 5000 | 25000 | 2500 | USD | 50 | 0 | 0 |
| E | 10:01 | claude | key-3 | ''(空) | ''(空) | 200 | 0 | '' | 10 | 0 | 100 | 20 | 120 | 1000 | 0 | 0 | '' | 0 | 0 | 1 |

**明细表 `bfe_ai_request_log`**（6 行，log_time 倒序 logid 为 1006..1001）：

| logid | log_time | key | 目标模型 | provider | 状态 | err | 说明 |
|-------|----------|-----|----------|----------|------|-----|------|
| 1001 | 10:00:10 | key-1 | gpt-4o | openai | 200 | - | level1Name/level1 标签打平 |
| 1002 | 10:00:20 | key-1 | gpt-4o | openai | 500 | backend timeout | ai_rate_limit_hits/req_headers JSON 非空 |
| 1003 | 10:00:30 | key-2 | gpt-4 | azure | 200 | - | |
| 1004 | 10:00:40 | NULL(未认证) | gpt-4o | openai | 401 | invalid api key | ai_apikey_id NULL、ai_auth_reject_quota_plans JSON |
| 1005 | 10:01:10 | key-3 | claude | ''(空) | 200 | - | provider 空串 |
| 1006 | 10:02:00 | key-1 | gpt-4o | openai | 200 | - | 最新一行 |

### 7.3 服务装配

通过 `testutil.StartServerWithExtraConfig` 把以下配置**追加**到临时 `ai_gateway_api.toml` 末尾（端口/DB/Redis 补丁之后）：

```toml
[Databases.report_db]
Driver = "mysql"
DBName = "report_it_<ns>"
Addr = "<host:port>"
User = "<user>"
Passwd = "<pass>"
MaxOpenConns = 10
MaxIdleConns = 5

[Report]
Backend = "mysql"
Datasource = "report_db"
EnableAggregateJob = false     # 集成层只验证查询端点，JOB 行为由单测覆盖
EnablePartitionMgmt = false
```

### 7.4 期望值（手算）

- **overview**：request_total=660，error_total=200，error_rate=200/660，input/output/total_tokens=6600/1320/7920；latency_avg=66000/660=100ms，latency_max=100ms（各组均值的 MAX）；ttft_avg=225000/450/1000=0.5ms、tpot_avg=22500/450/1000=0.05ms（stream 请求 450）；cost：USD=350、RMB=300（定点种子；接口出口 ÷1e8 后 USD=3.5e-6、RMB=3e-6）；rate_limit_hits=2，auth_rejects=1，logs_total=6；MySQL 后端不返回 latency_p50/p90/p99（字段不存在）。
- **timeseries(metric=qps)**：bucket_sec=60（≤6h 窗口）；两个桶：10:00→600/60=10，10:01→60/60=1。
- **timeseries(metric=tokens)**：10:00 → input=6000/60=100、output=20、total=120；10:01 → 10/2/12。
- **rankings(model)**：gpt-4o(350, err 200, in 3500, out 700) > gpt-4(300) > claude(10)。
- **rankings(status)**："200"(460) > "500"(200)。
- **distribution(status)**：ratio 200≈0.69697、500≈0.30303，合计 1。
- **distribution(protocol)**：openai=650，unknown=10（行 E 空协议归一）。
- **logs**：total=6；page=1&page_size=2 → [1006,1005]，page=2 → [1004,1003]；err_only=true → [1004,1002]；keyword=timeout → [1002]；requested_models=claude-3 → [1005]；行形状：log_time 为 Unix 秒、1004.ai_apikey_id 为 null、1002 的 JSON 列为原样字符串、1001 标签打平字段。

### 7.5 参数校验（以 api 实际返回为准）

| 场景 | 入参 | 预期 |
|------|------|------|
| start=end | start=end=10:05:00 | ErrNum=422（Param Illegal） |
| 窗口超 7 天 | end=start+7d+1s | 422 |
| 缺 end | 仅 start | 422 |
| metric 非法 | metric=bogus | 422 |
| 缺 metric | - | 422 |
| rankings dimension 非法 | dimension=stream | 422 |
| distribution dimension 非法 | dimension=model | 422 |
| keyword 超长 | 129 字符 | 422 |
| page_size 超上限 | page_size=101 | **封顶为 100 并成功**（对齐 operation-logs 分页惯例：超限截断而非报错） |

### 7.6 时区口径

查询侧所有「DATETIME → Unix 秒」的渲染使用 `TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', <col>)`（纯日历算术），不依赖 MySQL 会话时区——`UNIX_TIMESTAMP` 会按会话时区解读墙钟，而 log-reader 以 UTC 墙钟写入，二者混用会产生 8 小时偏移（SC31 实测）。因此断言直接对比 UTC epoch 常量（`epoch("2026-09-15 10:00:00")` 等），无需时区解耦绕法。

### 7.7 清理方式

- `TestMain` 结束（`m.Run()` 之后）依次：`sm.Shutdown()`（停 api 进程、删临时 conf、删 SQLite、关 miniredis）→ 关种子连接 → `DROP DATABASE IF EXISTS report_it_<ns>`。
- 启动失败路径同样执行 DROP，防御半初始化残留。
- 数据库名含纳秒时间戳，多进程并发互不干扰。

## 8. 组 C：分区自动维护（partition）

### 8.1 设计思路

MySQL 形态下 `bfe_ai_request_log` / `bfe_ai_metrics_1m` 按天 RANGE 分区（`TO_DAYS(log_time)` / `TO_DAYS(ts_min)`），分区由 api 进程内分区管理 JOB 滚动维护（每 6h 巡检、向前建 3 天、DROP 超保留期分区）。issue #191：配置 `[Report].Database` 后表名带 schema 限定，而 `information_schema.PARTITIONS` 的 `TABLE_NAME` 只存裸表名，分区查询恒返回 0 行，JOB 误判表未分区、静默降级为 DELETE 兜底，从不 ADD PARTITION；日志时间越过 `p_init` 边界后 log-reader 写入 Error 1526 丢行。

组 C 按 issue 生产同型装配（`Database` 非空 + `EnablePartitionMgmt=true`），断言 JOB 启动后即建好前瞻窗口分区，并验证"覆盖期内可写入、边界外拒绝"的分区契约。修复前 C-1 恒超时失败，构成 red→green 回归证据。

### 8.2 环境门控与数据源

- 与组 B 共用 `REPORT_MYSQL_DSN`（`user:pass@tcp(host:port)/`）；未设置时 TestMain 打印说明后以 0 退出，离线 CI 默认 Skip。
- 账号需具备 `CREATE/DROP DATABASE`（建清理专用库）与 `ALTER`（JOB 建/删分区）权限。
- api 子进程运行项目根 `ai-gateway-api.exe`：运行前须先 `make build`，确保二进制包含待验证代码。

```bash
REPORT_MYSQL_DSN="root:****@tcp(127.0.0.1:3306)/" go test -v -count=1 ./tests/report/partition/
```

### 8.3 数据构造

1. 创建专用随机名数据库 `report_it_<ns>`（先 `DROP DATABASE IF EXISTS` 防御残留）。
2. 套用项目根 `db_ddl_report_mysql.sql`，**`${INIT_DATE}` 替换为昨天**——与组 B 的"今天+3"相反：p_init 边界 < 今天，待建窗口 [今天, 今天+3) 非空，JOB 必须有 ADD 动作；若按组 B 方式建表，p_init 已覆盖前瞻窗口，修复前后用例都通过，失去回归意义。
3. 不灌种子数据；用例自行向两张表插行验证写入路径。

### 8.4 服务装配

```toml
[Report]
Backend = "mysql"
Datasource = "report_db"
Database = "report_it_<ns>"     # 关键：非空，issue #191 的触发条件（schema 限定名路径）
EnableAggregateJob = false      # 聚焦分区行为；聚合 JOB 由 storage/mysqlreport 单测覆盖
EnablePartitionMgmt = true
RetentionDays = 7
```

### 8.5 用例与校验点

| 用例 | 断言 |
|------|------|
| C-1 启动即建分区 | 轮询 information_schema（30s 超时/200ms 步长）：两表均出现 `p<今天>`、`p<今天+1>`、`p<今天+2>`，边界分别为 TO_DAYS(今天+1/+2/+3)（8.x 回显求值整数形态；文本形态取内嵌日期再求 TO_DAYS）；`p_init`（边界 TO_DAYS(昨天)，即建表 INIT_DATE）保留 |
| C-2 覆盖期内写入成功 | 两表插入 `log_time`/`ts_min = 今天`、`今天+2 23:59:59` 成功（模拟 log-reader 写入路径，锁定 Error 1526 消除） |
| C-3 边界外写入拒绝 | 两表插入 `今天+3` 失败且为 Error 1526（`*mysql.MySQLError.Number=1526`），固定"只保有 3 天前瞻"的契约 |

### 8.6 时序与时区口径

- JOB 在 `Start()` 时立即跑一轮周期，但服务器就绪（TCP 可拨号）与周期完成之间无时序保证，故用例轮询 information_schema 而非定长 sleep；已就绪即返回。
- "今天"取测试进程本地民用日（`time.Now().Date()`），JOB 在子进程内使用同一继承时区；断言秒级完成，跨自然日漂移风险可忽略。
- DATETIME 一律以 `YYYY-MM-DD HH:MM:SS` 字符串插入，不依赖驱动/会话时区。

## 9. 组 E：幂等键请求级判别（dedup_key）

### 9.1 设计思路

明细表 `bfe_ai_request_log` 的幂等键 `uk_dedup` 决定"同秒同 Key 同模型的并发请求"能否各自成行：键里没有请求级判别列时，写入侧的 `ON DUPLICATE KEY UPDATE`（全列覆盖）会把 n 个并发行压成 1 行（bfenetworks/bfe#1417，生产实测丢率恰为 (n−1)/n、窗口 13.5%）。本组把该行为钉在**报表出口**上（bfe 侧只断言 pb 日志、log-reader LR03 只断言"写入 MySQL"，报表 API 出口此前没有载体）：

1. **明细侧保留**：3 条"同秒 + 同 Key + 同模型、仅 `logid` 不同"的行必须全部可查；
2. **对照臂**：同批数据写入"旧四列键"镜像表 → 只留 1 行，证明这些行的唯一差别就是 `logid`；
3. **重放幂等**：同 `logid` 再写一次（值变更）→ 仍 3 行且值被覆盖，确认覆盖语义只作用于同一请求的重放，未把"丢行"换成"重复行"。

### 9.2 环境门控与数据源

与组 B/组 D 同：`REPORT_MYSQL_DSN`（`user:pass@tcp(host:port)/`）；未设置时 TestMain 打印说明后以 0 退出，离线 CI 默认 Skip。装配用 `testutil.StartReportServer`（聚合/分区 JOB 关闭）。

### 9.3 数据构造

- **明细 3 行**（走生产同款多值 `INSERT ... ON DUPLICATE KEY UPDATE`）：共享 `(hostid='gw-01', log_time='2026-09-15 10:00:00', ai_apikey_id='key-1', ai_requested_model='gpt-4')`——即旧四列键的完整键值；`logid` 3001/3002/3003，`ai_total_tokens` 15/25/35，`ai_cost_value` 1500/2500/3500（定点，÷1e8 后为 0.000015/0.000025/0.000035）。
- **聚合 1 行**（与明细一致）：`request_count=3`、`input/output/total_tokens=60/15/75`、`ai_cost_value_sum=7500`、`ai_cost_currency='RMB'`。
- **对照臂镜像表** `bfe_ai_request_log_legacy_uk`：同库在建，复刻修复前的四列唯一键。

### 9.4 用例与校验点

| 子用例 | 校验点 |
|--------|--------|
| `detail_key_keeps_concurrent_rows` | `/report/overview.logs_total=3`；`/report/logs.total=3` 且返回 3 个不同 `logid`；3 行的 `log_time` 同秒且 `hostid`/`ai_apikey_id`/`ai_requested_model` 全同（键前提）；聚合口径 `request_total=3`、`total_tokens=75`、cost=`0.000075` |
| `legacy_four_column_key_collapses` | 镜像表 `COUNT(*)=1`、幸存 `logid=3003`（最后写入行）——即 issue 现场"后写行整行存活、先写行消失" |
| `replay_still_idempotent` | 重放后 `/report/logs.total=3`（不新增行）、token 求和被覆盖为 600（100+200+300） |

> 聚合表在本组由测试预置（JOB 关闭），故 `request_total/total_tokens/cost` 仅用于口径一致性；**缺陷判据是明细侧行数**——聚合"不低估"以明细"不丢行"为前提。

### 9.5 运行方式与红/绿

```bash
REPORT_MYSQL_DSN="root:****@tcp(127.0.0.1:3306)/" go test -v -count=1 ./tests/report/dedup_key/
```

把 `db_ddl_report_mysql.sql` 回退为修复前（旧四列键）→ 子用例 1/3 失败（并发行只剩 1 行）；恢复后全绿。因 seed 走生产同款 `ON DUPLICATE KEY UPDATE`，红色以**行数断言失败**呈现，而非 SQL 报重复键。

## 10. testutil 钩子

`testutil/server.go` 新增：

```go
func StartServerWithExtraConfig(extraTOML string) (*ServerManager, error)
```

- 复用 `StartServerWithSharedInfra` 全部逻辑（实现上抽公共 `startServer(sharedRedis, sharedDBPath, extraTOML)`）；
- `createTempConfig` 在端口/DB/Redis 补丁之后把 `extraTOML`（去空白后非空才追加）写到临时 `ai_gateway_api.toml` 末尾；
- `extraTOML == ""` 时与 `StartServer()` 行为完全一致（既有测试零影响）；
- 非共享模式下照常 `SetServerURL`，全局客户端语义不变。

## 11. 注意事项

1. 组 B/组 C 需要真实 MySQL 8.x（≥5.7.8，JSON 列），离线 CI 默认 Skip；组 A 离线必跑。
2. 聚合表种子只 INSERT 查询涉及的列，其余维度列走 `NOT NULL DEFAULT ''`、指标列走 `DEFAULT NULL`（SUM 口径不受影响）。
3. 组 B `${INIT_DATE}` 必须替换为今天+3 天，否则建表后历史数据写入会因无匹配分区失败（Error 1526）；组 C 相反，必须替换为昨天（见 §8.3）。
4. 组 B 不启动聚合/分区 JOB（`Enable* = false`）；组 C 启动分区 JOB、关闭聚合 JOB——分区行为在集成层验证（issue #191 后补），聚合行为仍由 `storage/mysqlreport` 单测覆盖。
