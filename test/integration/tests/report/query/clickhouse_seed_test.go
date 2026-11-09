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

// ClickHouse 后端种子的 fixture（真实 CH 端到端组专用，
// Backend=clickhouse + REPORT_CLICKHOUSE_DSN 门控，见 clickhouse_test.go）。
//
// 与 seed_test.go 的 aggregateSeedSQL / detailSeedSQL 同源（数值口径、行结构、
// 手算期望完全一致），仅按 ClickHouse 表契约做以下偏差（断言侧已同步调整）：
//
//  1. 时间整体平移到 2030-09-15：CH 两表 DDL 带 TTL 7 天（log_time/ts_min + 7 DAY），
//     固定 2026-09-15 的种子在"当前"运行会立即落入 TTL 过期窗口，且 TTL 删除由后台
//     merge 触发、时序不可控（查询可能时有时无）；平移到远未来后不再触发 TTL。
//     分钟桶对齐方式不变，桶内手算值与 MySQL/doris 组逐项一致。
//  2. req_headers / res_headers：CH 明细表 DDL 写作 Nested(key String, value String)，
//     但服务端 flatten_nested 默认 1，建表后物理列是展开的两个平行数组
//     `req_headers.key` / `req_headers.value`（`Nested` 整体名只是虚拟投影，
//     非物理列——ai-gateway-observability bfe_ai_log_load_mv.sql 头注释同述）。
//     种子按物理子列插入（1002 行写入一组真实头，其余行空数组）；
//     查询侧以 arrayMap 重组具名 Tuple 后 toJSONString，序列化形态
//     `[{"key":"X-Test","value":"1"}]` 与 MySQL JSON 文本一致，内容断言可用。
//  3. ai_rate_limit_hits：CH 为 Array(Tuple(rate_limit_policy_id String,
//     rate_limit_type String, rule_names Array(String))) → 1002 行改用 CH 数组
//     字面量 [('p1','rpm',['r1'])]（语义同 MySQL 种子 JSON）。
//  4. ai_auth_reject_quota_plans：CH 为 Array(String) → 1004 行改用 ['plan-a']。
//     上述两列的响应序列化形态未锁定（跨引擎文本表示差异），内容断言同样跳过。
//  5. MySQL 明细表允许 NULL 的 String 列（1004 的 ai_apikey_id / ai_cost_currency、
//     1002-1006 的 level1Name / level1 等），CH 明细表对应列为 String DEFAULT ''
//     （非 NULL 列）→ 原样写 NULL，依赖服务端默认落入 ''；对应响应字段为 ""
//     而非 null（clickhouse_test.go 的明细行形状断言按此调整）。

// chAggregateSeedSQL 即 aggregateSeedSQL 的 ClickHouse 形态：仅时间列由
// 2026-09-15 平移到 2030-09-15（见文件头偏差 1），列清单与取值逐项一致
// （CH 聚合表 40 维全 String/数值 NOT NULL 列，字面量兼容）。与 MySQL 组
// 一样含行 F（issue #315 污染复现行：非流式但 ttft_us_sum=40000>0）。
const chAggregateSeedSQL = `INSERT INTO bfe_ai_metrics_1m
(ts_min, ai_apikey_id, ai_requested_model, ai_target_model, ai_stream,
 res_status_code, err_code, ai_provider, ai_protocol, ai_cost_currency,
 request_count, error_count, input_tokens, output_tokens, total_tokens,
 ttft_us_sum, tpot_us_sum, all_time_sum, rate_limit_hits, auth_reject_count,
 ai_cost_value_sum, cache_read_tokens, cache_write_tokens,
 ai_cache_status, mirror_hit, ai_intent_answer)
VALUES
('2030-09-15 10:00:00', 'key-1', 'gpt-4', 'gpt-4o', 1, 200, '', 'openai', 'openai', 'USD', 100, 0, 1000, 200, 1200, 50000, 5000, 10000, 0, 0, 100, 1000, 200, 'hit', 1, 'coding'),
('2030-09-15 10:00:00', 'key-1', 'gpt-4', 'gpt-4o', 0, 500, 'E500', 'openai', 'openai', 'USD', 200, 200, 2000, 400, 2400, 0, 0, 20000, 2, 0, 200, 0, 400, 'miss', 0, 'unknown'),
('2030-09-15 10:00:00', 'key-2', 'gpt-4', 'gpt-4', 1, 200, '', 'azure', 'openai', 'RMB', 300, 0, 3000, 600, 3600, 150000, 15000, 30000, 0, 0, 300, 3000, 0, 'hit', 0, 'writing'),
('2030-09-15 10:01:00', 'key-1', 'gpt-4', 'gpt-4o', 1, 200, '', 'openai', 'openai', 'USD', 50, 0, 500, 100, 600, 25000, 2500, 5000, 0, 0, 50, 500, 100, 'hit', 1, 'coding'),
('2030-09-15 10:01:00', 'key-3', 'claude-3', 'claude', 0, 200, '', '', '', '', 10, 0, 100, 20, 120, 0, 0, 1000, 0, 1, 0, 0, 0, '', 0, ''),
('2030-09-15 10:01:00', 'key-9', 'llama-3', 'llama-3', 0, 200, '', 'anthropic', 'anthropic', '', 20, 0, 0, 0, 0, 40000, 4000, 2000, 0, 0, 0, 0, 0, '', 0, '')`

// chDetailSeedSQL 即 detailSeedSQL 的 ClickHouse 形态：时间平移到 2030-09-15，
// 复杂类型列按文件头偏差 2-5 调整；窗口内 6 行的维度/指标取值与 MySQL 种子一致，
// 明细过滤与 overview 缓存/镜像/意图 count 的手算口径不变。
const chDetailSeedSQL = `INSERT INTO bfe_ai_request_log
(logid, log_time, hostid, product, ai_apikey_id, ai_requested_model,
 ai_target_model, ai_provider, ai_protocol, ai_mode, ai_stream,
 res_status_code, err_code, err_msg,
 ai_input_tokens, ai_output_tokens, ai_total_tokens, all_time,
 ai_ttft_us, ai_tpot_us, ai_cost_value, ai_cost_currency,
 ai_rate_limit_hits, ai_auth_reject_quota_plans,
 level1Name, level1, client_ip, header_host, origin_uri,
 "req_headers.key", "req_headers.value", "res_headers.key", "res_headers.value",
 ai_cache_status, mirror_hit, mirror_cluster,
 ai_intent_question, ai_intent_answer, ai_intent_confidence, ai_intent_source,
 ai_intent_latency_us, ai_intent_cache_hit, ai_intent_questions_version)
VALUES
(1001, '2030-09-15 10:00:10', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 1, 200, NULL, NULL, 100, 20, 120, 100, 50000, 5000, 100, 'USD', NULL, NULL, 'dep', 'ops', '10.0.0.1', 'api.example.org', '/v1/chat', [], [], [], [], 'hit', 1, 'mirror-bj', 'task_type', 'coding', 0.95, 'classifier', 1200, 0, 'v3'),
(1002, '2030-09-15 10:00:20', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 500, 'E500', 'backend timeout', 200, 40, 240, 200, NULL, NULL, 200, 'USD', [('p1', 'rpm', ['r1'])], NULL, NULL, NULL, '10.0.0.2', 'api.example.org', '/v1/chat', ['X-Test'], ['1'], [], [], 'miss', 0, '', 'task_type', 'unknown', 0.30, 'classifier', 2500, 0, 'v3'),
(1003, '2030-09-15 10:00:30', 'gw-02', 'BFE', 'key-2', 'gpt-4', 'gpt-4', 'azure', 'openai', 'chat', 1, 200, NULL, NULL, 300, 60, 360, 300, 150000, 15000, 300, 'RMB', NULL, NULL, NULL, NULL, '10.0.0.3', 'api.example.org', '/v1/chat', [], [], [], [], 'hit', 0, '', 'task_type', 'writing', 0.90, 'explicit_header', NULL, NULL, 'v2'),
(1004, '2030-09-15 10:00:40', 'gw-01', 'BFE', NULL, 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 401, 'E401', 'invalid api key', 50, 10, 60, 50, NULL, NULL, 0, NULL, NULL, ['plan-a'], NULL, NULL, '10.0.0.4', 'api.example.org', '/v1/chat', [], [], [], [], '', 0, '', '', '', NULL, '', NULL, NULL, ''),
(1005, '2030-09-15 10:01:10', 'gw-01', 'BFE', 'key-3', 'claude-3', 'claude', '', 'openai', 'chat', 0, 200, NULL, NULL, 10, 2, 12, 10, NULL, NULL, 0, '', NULL, NULL, NULL, NULL, '10.0.0.5', 'api.example.org', '/v1/chat', [], [], [], [], 'skip', 1, 'mirror-bj', 'task_type', 'coding', 0.88, 'cache', NULL, 1, 'v3'),
(1006, '2030-09-15 10:02:00', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 200, NULL, NULL, 60, 12, 72, 60, NULL, NULL, 60, 'USD', NULL, NULL, NULL, NULL, '10.0.0.6', 'api.example.org', '/v1/chat', [], [], [], [], 'miss', 0, '', '', '', NULL, '', NULL, NULL, '')`
