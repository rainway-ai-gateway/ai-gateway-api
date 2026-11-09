// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//http://www.apache.org/licenses/LICENSE-2.0
//
//Unless required by applicable law or agreed to in writing, software
//distributed under the License is distributed on an "AS IS" BASIS,
//WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//See the License for the specific language governing permissions and
//limitations under the License.

package query_test

// StarRocks 后端种子的 fixture（真实 SR 端到端组专用，Backend=starrocks +
// REPORT_STARROCKS_DSN/REPORT_STARROCKS_DDL_DIR 门控，见 starrocks_test.go）。
//
// 与 seed_test.go 的 detailSeedSQL 同源（6 行明细，维度/指标/NULL 语义逐项
// 一致），仅按 StarRocks 表契约与物化视图机制做以下偏差（断言侧已同步调整）：
//
//  1. 时间整体平移到 2030-09-15：聚合表 bfe_ai_metrics_1m 是异步物化视图，
//     PARTITION BY ts_day + partition_ttl "7 DAY"（实机 SHOW CREATE TABLE
//     确认），固定 2026 的种子在"当前"运行会落入 TTL 过期窗口；平移到远未来
//     后不再触发 TTL（与 clickhouse_seed_test.go 偏差 1 同理）。
//
//  2. 聚合种子路径（实机确认过程与最终选择）：bfe_ai_metrics_1m 在
//     information_schema.tables 中 TABLE_TYPE=VIEW（CREATE MATERIALIZED
//     VIEW ... REFRESH ASYNC EVERY (INTERVAL 1 MINUTE)），直插被 FE 拒绝
//     （实机报 "The data of 'bfe_ai_metrics_1m' cannot be inserted because
//     'bfe_ai_metrics_1m' is a materialized view, and the data of materialized
//     view must be consistent with the base table."）。因此本 fixture 只灌
//     基表 bfe_ai_request_log（6 行明细），聚合数据由异步 MV 从基表按
//     40 维分钟桶聚合（端到端 1~2 分钟，本机 3.5.21 实测约 34s；
//     testutil.StartStarRocksReportServer 轮询 MV 行数达标后才启动 api）。
//     聚合端点断言值 = 下表"MV 派生手算值"，不再与 MySQL/doris 组的聚合表
//     手算值（680 请求口径）相同——单源种子下 6 行明细只能聚出 6 个请求。
//
//  3. 复杂列按 SR DDL 类型落地（实机 DESCRIBE 确认，见
//     ai-gateway-observability/starrocks/sqls/bfe_ai_request_log.sql）：
//     req_headers / res_headers / ai_rate_limit_hits 为 VARCHAR(JSON 文本)
//     直插字面量即可，SELECT 原样回显 JSON 文本；ai_auth_reject_quota_plans
//     为原生 ARRAY<VARCHAR>：
//       - 裸 CAST(varchar AS ARRAY<...>) 被 SR 拒绝（实机报 "Not support
//         cast"），必须 CAST(parse_json('...') AS ARRAY<VARCHAR(128)>)；
//       - ARRAY<STRUCT<...>> 的字段名（key/value 为保留字）须反引号
//         （STRUCT<`key` VARCHAR(128), ...>；双引号写法实机报语法错误），
//         本 fixture 的配额计划列为 ARRAY<VARCHAR>，不涉及 STRUCT；
//       - ARRAY 列经 MySQL 协议 SELECT 返回 JSON 文本（实机：
//         ["plan-a"]；CAST(parse_json('[]') AS ARRAY<VARCHAR(128)>) 返回
//         []），与 Doris JSON 列线格式一致，扫描层 NullString 直收。
//     req_headers 的 1002 行采用归一形态 [{"key":"X-Test","value":"1"}]
//     （即 clickhouse 组重组后的语义形态；SR 无 flatten_nested，VARCHAR
//     原样回显，插入什么文本就断言什么文本）。
//
//  4. MV 单源种子的补列（MySQL 组由聚合表种子承担的口径，改由明细行承担，
//     均为 MV 的 SUM/CASE 指标列或明细列，不参与 40 维分组键，也不出现在
//     logs 端点 47 列投影中，不影响任何明细断言）：
//       - ai_cache_read_tokens / ai_cache_write_tokens：1001 行 1000/200、
//         1002 行 0/400、1003 行 3000/0（对齐 MySQL 聚合行 A/B/C 的缓存
//         token 口径）→ 10:00 桶 read=4000、write=600；
//       - 1004 行 ai_auth_reject_reason='quota_exceeded'：MV 的
//         auth_reject_count 统计该列非空的行 → AuthRejects=1（对齐
//         MySQL 组；MySQL 明细种子该列为 NULL，因 MySQL 组由聚合行 E 承担）；
//       - rate_limit_hits：MV 按"ai_rate_limit_hits JSON 数组非空的行数"
//         计数（array_length(CAST(parse_json(...) AS ARRAY<STRUCT<...>))>0），
//         一行 JSON 无论几个元素只计 1 → RateLimitHits=1（MySQL 聚合行 B
//         直接给 2，此口径差异固定为 SR 组手算值）。
//
//  5. NULL 语义与 MySQL 组一致（区别于 CH 组偏差 5）：明细表可空列原样写
//     NULL——1004 的 ai_apikey_id / ai_cost_currency 为 NULL（响应 null，
//     非 CH 的 ""）；1001/1003 等行的 req_headers/res_headers 为 NULL
//     （响应 null，非 CH 数组列的 "[]"）。
//
//  6. FE 二进制协议陷阱（实机定位，2026-10-02）：SR FE（MySQL 协议重实现）
//     的 COM_STMT 二进制行包存在编码缺陷——JSON 列与 NULL 列相邻的行触发
//     解析错位（驱动 v1.6.0：行内 ''/NULL 静默串位；升级 v1.9.3 后同查询
//     改为 packets.go readRow panic，二者均为 FE 行包畸形、与驱动版本无
//     关；行 1004 稳定复现，列子集二分：投影列 24-25（JSON + NULL）即触发，
//     全 49 列在行中部错位）。规避：测试装配的 [Databases.report_db] 显式
//     InterpolateParams = true（参数客户端插值回归文本协议；SQL 语义不变，
//     mysql CLI/无参查询同为文本协议故正确）。该缺陷对 api 生产部署同样
//     成立（logs 47+ 列宽投影），SR 生产配置必须开启 InterpolateParams
//     （已写入 conf starrocks 段样例）；根治需 SR FE 修复，建议向 SR 提
//     issue（复现要点见 design-docs/modifications/
//     2026-10-02-report-mysql-driver-upgrade/change-summary.md）。
//
// MV 派生手算值（6 行明细经异步 MV，查询窗口 [2030-09-15 09:59, 10:05)，
// 60s 桶；本机 3.5.21 实机核对，见 starrocks_test.go 断言）：
//
//	overview:    req=6 err=2 rate=1/3 in=720 out=144 tot=864
//	             latency_avg=120ms(720/6) latency_max=300ms(1003 行组)
//	             ttft=100ms(200000us/2 流式) tpot=10ms(20000us/2)
//	             cost: USD=360(3.6e-6) RMB=300(3e-6) rl=1 rej=1 logs=6
//	             cache hit=2 miss=2 skip=1 rate=0.5 read=4000 write=600
//	             mirror=2 intent classified=3 unknown=1 rate=0.25
//	分位数(percentile_approx, 实机值): 窗口 6 值[100,200,300,50,10,60]
//	             p50=80 p99=300 p90≈289.99994(素描近似，只断存在性)
//	             10:00 桶 4 值 p50=150 p90=300 p99=300；空窗口返回 NULL(字段省略)
//	时序桶:      10:00 req=4 in=650 out=130 tot=780 all_time=650 max=300
//	             10:01 req=1 in=10 out=2 tot=12 all_time=10 max=10
//	             10:02 req=1 in=60 out=12 tot=72 all_time=60 max=60
//	成本时序:    (10:00,'')=0（1004 NULL 币种）(10:00,USD)=300 (10:00,RMB)=300
//	             (10:01,'')=0（1005）(10:02,USD)=60
//	cache_tokens: 10:00 read=4000 write=600；10:01/10:02 两桶均 0/0（SUM 零点仍在）
//	维度拆分:    qps_by_cache_status 五点——(10:00,hit)=2/60、(10:00,miss)=1/60、
//	             (10:00,'')=1/60（1004 空值桶）、(10:01,skip)=1/60、(10:02,miss)=1/60
//	排行:        model gpt-4o=4(err 2,in 410,out 82)/gpt-4=1(300,60)/claude=1(10,2)
//	             status 200=4/500=1/401=1；cache_status hit=2/miss=2/skip=1（''不进）
//	             mirror_hit 0=4/1=2；intent_answer coding=2/unknown=1/writing=1
//	分布:        status 200=4(4/6)/401=1/500=1；protocol openai=6（明细种子无空
//	             协议行，unknown 归一桶由 cache_status 覆盖：hit=2/miss=2/skip=1/
//	             unknown=1）；mirror_hit 0=4/1=2
//	             （另实机核对：若按 intent_answer 分布，1002 的真 unknown 与
//	             1004/1006 的空值在 CASE 归一下合并为 unknown=3 单桶）
//	过滤:        models=gpt-4o + status_codes=200 → req=2 err=0 in=160 logs=2
//	logs 明细:   与 MySQL 组同口径（同一组 6 行手算值，见 seed_test.go 头表）

// srDetailSeedSQL 即 detailSeedSQL 的 StarRocks 形态：时间平移到 2030-09-15，
// 复杂列按文件头偏差 3 的语法灌入，补列按偏差 4；窗口内 6 行的维度/指标取值
// 与 MySQL 种子一致（补列除外），明细过滤与 overview 缓存/镜像/意图 count 的
// 手算口径不变。只灌基表——bfe_ai_metrics_1m 是异步物化视图，不可直插（见
// 文件头偏差 2），聚合数据由 MV 刷新（testutil 轮询等待）提供。
const srDetailSeedSQL = `INSERT INTO bfe_ai_request_log
(logid, log_time, hostid, product, ai_apikey_id, ai_requested_model,
 ai_target_model, ai_provider, ai_protocol, ai_mode, ai_stream,
 res_status_code, err_code, err_msg,
 ai_input_tokens, ai_output_tokens, ai_total_tokens, all_time,
 ai_ttft_us, ai_tpot_us, ai_cost_value, ai_cost_currency,
 ai_rate_limit_hits, ai_auth_reject_quota_plans,
 ai_cache_read_tokens, ai_cache_write_tokens, ai_auth_reject_reason,
 level1Name, level1, client_ip, header_host, origin_uri,
 req_headers, res_headers,
 ai_cache_status, mirror_hit, mirror_cluster,
 ai_intent_question, ai_intent_answer, ai_intent_confidence, ai_intent_source,
 ai_intent_latency_us, ai_intent_cache_hit, ai_intent_questions_version)
VALUES
(1001, '2030-09-15 10:00:10', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 1, 200, NULL, NULL, 100, 20, 120, 100, 50000, 5000, 100, 'USD', NULL, NULL, 1000, 200, NULL, 'dep', 'ops', '10.0.0.1', 'api.example.org', '/v1/chat', NULL, NULL, 'hit', 1, 'mirror-bj', 'task_type', 'coding', 0.95, 'classifier', 1200, 0, 'v3'),
(1002, '2030-09-15 10:00:20', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 500, 'E500', 'backend timeout', 200, 40, 240, 200, NULL, NULL, 200, 'USD', '[{"rate_limit_policy_id":"p1","rate_limit_type":"rpm","rule_names":["r1"]}]', NULL, 0, 400, NULL, NULL, NULL, '10.0.0.2', 'api.example.org', '/v1/chat', '[{"key":"X-Test","value":"1"}]', NULL, 'miss', 0, '', 'task_type', 'unknown', 0.30, 'classifier', 2500, 0, 'v3'),
(1003, '2030-09-15 10:00:30', 'gw-02', 'BFE', 'key-2', 'gpt-4', 'gpt-4', 'azure', 'openai', 'chat', 1, 200, NULL, NULL, 300, 60, 360, 300, 150000, 15000, 300, 'RMB', NULL, NULL, 3000, 0, NULL, NULL, NULL, '10.0.0.3', 'api.example.org', '/v1/chat', NULL, NULL, 'hit', 0, '', 'task_type', 'writing', 0.90, 'explicit_header', NULL, NULL, 'v2'),
(1004, '2030-09-15 10:00:40', 'gw-01', 'BFE', NULL, 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 401, 'E401', 'invalid api key', 50, 10, 60, 50, NULL, NULL, 0, NULL, NULL, CAST(parse_json('["plan-a"]') AS ARRAY<VARCHAR(128)>), 0, 0, 'quota_exceeded', NULL, NULL, '10.0.0.4', 'api.example.org', '/v1/chat', NULL, NULL, '', 0, '', '', '', NULL, '', NULL, NULL, ''),
(1005, '2030-09-15 10:01:10', 'gw-01', 'BFE', 'key-3', 'claude-3', 'claude', '', 'openai', 'chat', 0, 200, NULL, NULL, 10, 2, 12, 10, NULL, NULL, 0, '', NULL, NULL, 0, 0, NULL, NULL, NULL, '10.0.0.5', 'api.example.org', '/v1/chat', NULL, NULL, 'skip', 1, 'mirror-bj', 'task_type', 'coding', 0.88, 'cache', NULL, 1, 'v3'),
(1006, '2030-09-15 10:02:00', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 200, NULL, NULL, 60, 12, 72, 60, NULL, NULL, 60, 'USD', NULL, NULL, 0, 0, NULL, NULL, NULL, '10.0.0.6', 'api.example.org', '/v1/chat', NULL, NULL, 'miss', 0, '', '', '', NULL, '', NULL, NULL, '')`
