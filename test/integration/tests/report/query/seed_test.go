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

// 种子数据设计（数值均可手算）：
//
// 聚合表 bfe_ai_metrics_1m，查询窗口 [2026-09-15 09:59, 10:05)（≤6h → 60s 桶）：
//
//	行  桶     模型     key   provider 协议     状态 流  err   req  err_cnt in   out  tot  all_time ttft_us tpot_us 币种  cost  rl  rej  cache_r cache_w cache_status mirror intent_answer
//	A  10:00 gpt-4o  key-1 openai   openai  200  1  ''    100  0       1000 200  1200 10000    50000    5000  USD  100   0   0    1000     200    hit          1      coding
//	B  10:00 gpt-4o  key-1 openai   openai  500  0  E500  200  200     2000 400  2400 20000        0       0  USD  200   2   0       0     400    miss         0      unknown
//	C  10:00 gpt-4   key-2 azure    openai  200  1  ''    300  0       3000 600  3600 30000   150000   15000  RMB  300   0   0    3000       0    hit          0      writing
//	D  10:01 gpt-4o  key-1 openai   openai  200  1  ''     50  0        500 100   600  5000    25000    2500  USD   50   0   0     500     100    hit          1      coding
//	E  10:01 claude   key-3 ''(空)  ''(空)  200  0  ''     10  0        100  20   120  1000        0       0  ''     0   0   1       0       0    ''           0      ''
//	F  10:01 llama-3 key-9 anthropic anthropic 200 0 ''    20  0          0   0     0  2000    40000    4000  ''     0   0   0       0       0    ''           0      ''
//
// 窗口合计：req=680 err=200 in=6600 out=1320 tot=7920 all_time=68000
//           stream 请求=450 ttft_us=225000 tpot_us=22500 rl=2 rej=1
//           行 F 是 issue #315 的污染复现行：非流式但携带 BFE 实测首包延迟
//           （ttft_us_sum=40000>0），流式口径合计（stream/ttft/tpot）不含 F；
//           全量口径（req/all_time 等）含 F。修复前 overview ttft_avg 会把 F
//           计入分子（(225000+40000)/450/1000=0.589ms），修复后必须为 0.5ms。
//           cost: USD=350 RMB=300（定点值；接口出口 ÷1e8 后为 3.5e-6 美元 / 3e-6 元）
//           cache: read=4500 write=700；ai_cache_status 分布 hit=450/miss=200/''=30（E+F）
//           mirror_hit: 1→150（A+D）、0→530（B+C+E+F）
//           ai_intent_answer: coding=150（A+D）、unknown=200（B）、writing=300（C）、''=30（E+F）
//
// 明细表 bfe_ai_request_log 6 行（log_time 倒序后 logid 为 1006..1001），
// 覆盖：err_code 非空、JSON 列非空、ai_apikey_id NULL（未认证行）、provider 空串，
// 以及缓存/镜像/意图新 10 列的多态取值（见下行注释的 ai_cache_status/mirror_hit/intent 列）。
//
//	行  cache_status mirror_hit intent_question intent_answer conf   source            latency_us cache_hit version
//	1001 hit          1          task_type       coding        0.95 classifier        1200       0         v3
//	1002 miss         0          task_type       unknown       0.30 classifier        2500       0         v3
//	1003 hit          0          task_type       writing       0.90 explicit_header   NULL       NULL      v2
//	1004 ''           0          ''              ''            NULL ''                NULL       NULL      ''
//	1005 skip         1          task_type       coding        0.88 cache             NULL       1         v3
//	1006 miss         0          ''              ''            NULL ''                NULL       NULL      ''
//
// 明细窗口 count（overview 缓存/镜像/意图组 + logs 过滤断言的手算基础）：
// cache hit=2(1001,1003) miss=2(1002,1006) skip=1(1005)；mirror_hit=2(1001,1005)；
// intent classified=3（coding×2 + writing）unknown=1(1002)。

const aggregateSeedSQL = `INSERT INTO bfe_ai_metrics_1m
(ts_min, ai_apikey_id, ai_requested_model, ai_target_model, ai_stream,
 res_status_code, err_code, ai_provider, ai_protocol, ai_cost_currency,
 request_count, error_count, input_tokens, output_tokens, total_tokens,
 ttft_us_sum, tpot_us_sum, all_time_sum, rate_limit_hits, auth_reject_count,
 ai_cost_value_sum, cache_read_tokens, cache_write_tokens,
 ai_cache_status, mirror_hit, ai_intent_answer)
VALUES
('2026-09-15 10:00:00', 'key-1', 'gpt-4', 'gpt-4o', 1, 200, '', 'openai', 'openai', 'USD', 100, 0, 1000, 200, 1200, 50000, 5000, 10000, 0, 0, 100, 1000, 200, 'hit', 1, 'coding'),
('2026-09-15 10:00:00', 'key-1', 'gpt-4', 'gpt-4o', 0, 500, 'E500', 'openai', 'openai', 'USD', 200, 200, 2000, 400, 2400, 0, 0, 20000, 2, 0, 200, 0, 400, 'miss', 0, 'unknown'),
('2026-09-15 10:00:00', 'key-2', 'gpt-4', 'gpt-4', 1, 200, '', 'azure', 'openai', 'RMB', 300, 0, 3000, 600, 3600, 150000, 15000, 30000, 0, 0, 300, 3000, 0, 'hit', 0, 'writing'),
('2026-09-15 10:01:00', 'key-1', 'gpt-4', 'gpt-4o', 1, 200, '', 'openai', 'openai', 'USD', 50, 0, 500, 100, 600, 25000, 2500, 5000, 0, 0, 50, 500, 100, 'hit', 1, 'coding'),
('2026-09-15 10:01:00', 'key-3', 'claude-3', 'claude', 0, 200, '', '', '', '', 10, 0, 100, 20, 120, 0, 0, 1000, 0, 1, 0, 0, 0, '', 0, ''),
('2026-09-15 10:01:00', 'key-9', 'llama-3', 'llama-3', 0, 200, '', 'anthropic', 'anthropic', '', 20, 0, 0, 0, 0, 40000, 4000, 2000, 0, 0, 0, 0, 0, '', 0, '')`

const detailSeedSQL = `INSERT INTO bfe_ai_request_log
(logid, log_time, hostid, product, ai_apikey_id, ai_requested_model,
 ai_target_model, ai_provider, ai_protocol, ai_mode, ai_stream,
 res_status_code, err_code, err_msg,
 ai_input_tokens, ai_output_tokens, ai_total_tokens, all_time,
 ai_ttft_us, ai_tpot_us, ai_cost_value, ai_cost_currency,
 ai_rate_limit_hits, ai_auth_reject_quota_plans,
 level1Name, level1, client_ip, header_host, origin_uri, req_headers, res_headers,
 ai_cache_status, mirror_hit, mirror_cluster,
 ai_intent_question, ai_intent_answer, ai_intent_confidence, ai_intent_source,
 ai_intent_latency_us, ai_intent_cache_hit, ai_intent_questions_version)
VALUES
(1001, '2026-09-15 10:00:10', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 1, 200, NULL, NULL, 100, 20, 120, 100, 50000, 5000, 100, 'USD', NULL, NULL, 'dep', 'ops', '10.0.0.1', 'api.example.org', '/v1/chat', NULL, NULL, 'hit', 1, 'mirror-bj', 'task_type', 'coding', 0.95, 'classifier', 1200, 0, 'v3'),
(1002, '2026-09-15 10:00:20', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 500, 'E500', 'backend timeout', 200, 40, 240, 200, NULL, NULL, 200, 'USD', '[{"rate_limit_policy_id":"p1","rate_limit_type":"rpm","rule_names":["r1"]}]', NULL, NULL, NULL, '10.0.0.2', 'api.example.org', '/v1/chat', '[{"X-Test":["1"]}]', NULL, 'miss', 0, '', 'task_type', 'unknown', 0.30, 'classifier', 2500, 0, 'v3'),
(1003, '2026-09-15 10:00:30', 'gw-02', 'BFE', 'key-2', 'gpt-4', 'gpt-4', 'azure', 'openai', 'chat', 1, 200, NULL, NULL, 300, 60, 360, 300, 150000, 15000, 300, 'RMB', NULL, NULL, NULL, NULL, '10.0.0.3', 'api.example.org', '/v1/chat', NULL, NULL, 'hit', 0, '', 'task_type', 'writing', 0.90, 'explicit_header', NULL, NULL, 'v2'),
(1004, '2026-09-15 10:00:40', 'gw-01', 'BFE', NULL, 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 401, 'E401', 'invalid api key', 50, 10, 60, 50, NULL, NULL, 0, NULL, NULL, '["plan-a"]', NULL, NULL, '10.0.0.4', 'api.example.org', '/v1/chat', NULL, NULL, '', 0, '', '', '', NULL, '', NULL, NULL, ''),
(1005, '2026-09-15 10:01:10', 'gw-01', 'BFE', 'key-3', 'claude-3', 'claude', '', 'openai', 'chat', 0, 200, NULL, NULL, 10, 2, 12, 10, NULL, NULL, 0, '', NULL, NULL, NULL, NULL, '10.0.0.5', 'api.example.org', '/v1/chat', NULL, NULL, 'skip', 1, 'mirror-bj', 'task_type', 'coding', 0.88, 'cache', NULL, 1, 'v3'),
(1006, '2026-09-15 10:02:00', 'gw-01', 'BFE', 'key-1', 'gpt-4', 'gpt-4o', 'openai', 'openai', 'chat', 0, 200, NULL, NULL, 60, 12, 72, 60, NULL, NULL, 60, 'USD', NULL, NULL, NULL, NULL, '10.0.0.6', 'api.example.org', '/v1/chat', NULL, NULL, 'miss', 0, '', '', '', NULL, '', NULL, NULL, '')`
