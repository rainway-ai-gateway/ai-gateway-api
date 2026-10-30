-- ============================================================================
-- Rainway AI Gateway (壬远AI网关) - 报表库 DDL（MySQL 形态）
--
-- 本文件定义报表库（独立于控制面库，见 design-docs/sys-design/报表库设计文档.md）
-- 的两张表：
--   1. bfe_ai_request_log  明细表（101 列 = 99 列 + issue #1479 缺价信号 2 列，与 Doris 明细表同名同列）：由 log-reader
--      mod_log_mysql 插件批量幂等写入（INSERT ... ON DUPLICATE KEY UPDATE）；
--   2. bfe_ai_metrics_1m   分钟聚合表（40 维 + 24 指标，与 Doris 同名表对齐）：
--      由 ai-gateway-api 分钟聚合 JOB 生成，供 /open-api/v1/report/* 查询。
--
-- 执行方式（schema-first：先建表后启用 log-reader 插件，插件不自动建表）：
--   1. 将本文件中的 ${INIT_DATE} 替换为建表当日 +3 天的日期（YYYY-MM-DD），
--      使 p_init 分区覆盖建表后 3 天；后续分区由 ai-gateway-api 分区管理 JOB
--      自动滚动维护（每 6h 巡检，向前建 3 天、DROP 超 RetentionDays 的分区）；
--   2. 通过 mysql 客户端在目标实例执行，例如：
--      mysql -u<user> -p<password> < db_ddl_report_mysql.sql
--   3. 要求 MySQL >= 5.7.8（JSON 列），推荐 8.0；引擎 InnoDB，字符集 utf8mb4。
--
-- 存量表加列（2026-10-30 issue #1479 缺价信号字段，普通 ALTER 即可）：
--   ALTER TABLE bfe_ai_request_log
--     ADD COLUMN ai_price_miss TINYINT DEFAULT NULL AFTER ai_intent_questions_version,
--     ADD COLUMN ai_price_warn VARCHAR(32) DEFAULT NULL AFTER ai_price_miss;
--   说明：两列可空且无默认值——NULL = 该行未发生缺价（bfe 缺价时才写出）；
--   ai_price_miss 在非 2xx 的 reject 行也会出现（bfe 写出时机为请求级）。
--
-- 存量表加列（2026-09-27 缓存/镜像/意图字段，普通 ALTER 即可）：
--   ALTER TABLE bfe_ai_request_log
--     ADD COLUMN ai_cache_status VARCHAR(16) NOT NULL DEFAULT '' AFTER ai_auth_hit_quota_plans,
--     ADD COLUMN mirror_hit TINYINT NOT NULL DEFAULT 0 AFTER ai_cache_status,
--     ADD COLUMN mirror_cluster VARCHAR(128) NOT NULL DEFAULT '' AFTER mirror_hit,
--     ADD COLUMN ai_intent_question VARCHAR(64) NOT NULL DEFAULT '' AFTER mirror_cluster,
--     ADD COLUMN ai_intent_answer VARCHAR(64) NOT NULL DEFAULT '' AFTER ai_intent_question,
--     ADD COLUMN ai_intent_confidence DOUBLE DEFAULT NULL AFTER ai_intent_answer,
--     ADD COLUMN ai_intent_source VARCHAR(32) NOT NULL DEFAULT '' AFTER ai_intent_confidence,
--     ADD COLUMN ai_intent_latency_us BIGINT DEFAULT NULL AFTER ai_intent_source,
--     ADD COLUMN ai_intent_cache_hit TINYINT DEFAULT NULL AFTER ai_intent_latency_us,
--     ADD COLUMN ai_intent_questions_version VARCHAR(32) NOT NULL DEFAULT '' AFTER ai_intent_cache_hit;
--   ALTER TABLE bfe_ai_metrics_1m
--     ADD COLUMN ai_cache_status VARCHAR(16) NOT NULL DEFAULT '' AFTER ai_auth_reject_quota_plans_slot5,
--     ADD COLUMN mirror_hit TINYINT NOT NULL DEFAULT 0 AFTER ai_cache_status,
--     ADD COLUMN ai_intent_answer VARCHAR(64) NOT NULL DEFAULT '' AFTER mirror_hit;
--
-- 存量表迁移（2026-10-13，issue #1417：幂等键补 logid，需重建唯一键）：
--   ALTER TABLE bfe_ai_request_log
--     MODIFY COLUMN logid BIGINT NOT NULL DEFAULT 0,
--     DROP KEY uk_dedup,
--     ADD UNIQUE KEY uk_dedup (hostid, log_time, ai_apikey_id, ai_requested_model, logid);
--   说明：
--     1) logid 由可空收紧为 NOT NULL DEFAULT 0——唯一键视 NULL 互不相等，可空会让缺
--        id 的行逃出全部去重（同一行重放插入重复行），DEFAULT 0 与旧四元组覆盖行为等价；
--     2) 迁移前统计并【回填】NULL 行：
--        SELECT COUNT(*) FROM bfe_ai_request_log WHERE logid IS NULL;
--        若 >0，必须先 UPDATE bfe_ai_request_log SET logid = 0 WHERE logid IS NULL;
--        ——MySQL 默认严格模式下 MODIFY ... NOT NULL 遇到 NULL 行会直接报 Error 1138
--        （Invalid use of NULL value；实测于 MySQL 8.4.9），不会静默补 0；失败是原子的，
--        键与列属性保持原样（fail-fast，不留半成品）；
--     3) 本表按天 RANGE 分区，唯一键必须包含分区列 log_time（新键已包含）；
--     4) 建议低峰窗口执行：仓库内无在线 DDL 先例，先在实际版本的预发库量测耗时，
--        必要时用 pt-online-schema-change；回滚为 DROP/ADD 回四列 uk；
--     5) 生效前提：bfe 侧已修复 pb logid 恒 0（bfenetworks/bfe#1417），否则新行 logid 仍为 0；
--        迁移后校验见 design-docs/modifications/2026-10-13-issue-1417-report-detail-dedup-key-logid/design-changes.md §5.3。
--
-- 账号权限（最小权限原则，详见报表库设计文档 §5）：
--   - 写入账号（log-reader）：明细表 INSERT/UPDATE，无 DDL/DELETE；
--   - 查询/维护账号（ai-gateway-api）：明细表/聚合表 SELECT、聚合表 INSERT/DELETE、
--     明细表 DELETE（非分区降级清理）、ALTER TABLE（分区管理）。
-- ============================================================================

-- ---------------------------------
-- 明细表 bfe_ai_request_log（99 列）
-- 列清单/列序/类型与 Doris 明细表一致，差异仅在类型适配：
--   ARRAY<STRUCT<...>> 列用 JSON 存储（7 个：req_headers、res_headers、
--   ai_route_rule_hits、ai_cluster_key_names、ai_rate_limit_hits、
--   ai_auth_reject_quota_plans、ai_auth_hit_quota_plans），log_time 用 DATETIME。
-- 三处修正已经过 log-reader LR03 集成测试（真实 MySQL 8.4）验证：
--   1. 长文本列（origin_uri/final_uri/x_forward_for/authorization/referrer/
--      user_agent/cookie/res_location）使用 TEXT——utf8mb4 下 VARCHAR 长度总和
--      会超过 MySQL 65535 字节行上限（Error 1118）；
--   2. 幂等键中的字符串列 ai_apikey_id / ai_requested_model 允许 NULL——写入侧
--      零值规则把空字符串映射为 NULL（未认证请求即如此），NOT NULL 会使这类
--      写入整批失败（Error 1048）；hostid / log_time 由 log-reader 构造保证
--      非空，保持 NOT NULL；
--   3. 按天 RANGE 分区——MySQL 无动态分区，分区管理 JOB 必须先于数据到达建好
--      分区，否则对应日期的写入被直接拒绝（Error 1526）。
CREATE TABLE IF NOT EXISTS bfe_ai_request_log (
    hostid                  VARCHAR(256)  NOT NULL DEFAULT '',
    log_time                DATETIME      NOT NULL,
    ai_apikey_id            VARCHAR(256)  DEFAULT NULL,
    ai_requested_model      VARCHAR(128)  DEFAULT NULL,
    -- logid：BFE 逐请求唯一标识（bfe#1417 起为非 0 十进制值）。
    -- NOT NULL DEFAULT 0 而非可空：唯一键视 NULL 互不相等，可空会让缺 id 的行
    -- 逃出全部去重（同一行重放插入重复行）；DEFAULT 0 与旧四元组覆盖行为逐行等价。
    logid                   BIGINT        NOT NULL DEFAULT 0,
    product                 VARCHAR(64)   DEFAULT NULL,
    log_tag                 VARCHAR(64)   DEFAULT NULL,
    -- 客户端连接
    client_ip               VARCHAR(64)   DEFAULT NULL,
    client_network          VARCHAR(16)   DEFAULT NULL,
    is_trust_src_ip         TINYINT       DEFAULT NULL,
    req_num                 INT           DEFAULT NULL,
    session_id              BIGINT        DEFAULT NULL,
    bfe_ip                  VARCHAR(64)   DEFAULT NULL,
    sock_src_ip             VARCHAR(64)   DEFAULT NULL,
    vip                     VARCHAR(64)   DEFAULT NULL,
    vip6                    VARCHAR(128)  DEFAULT NULL,
    -- 错误
    err_code                VARCHAR(64)   DEFAULT NULL,
    err_msg                 VARCHAR(512)  DEFAULT NULL,
    -- 请求（长文本列用 TEXT，见修正 1）
    proto                   VARCHAR(16)   DEFAULT NULL,
    header_host             VARCHAR(256)  DEFAULT NULL,
    origin_uri              TEXT          DEFAULT NULL,
    final_uri               TEXT          DEFAULT NULL,
    method                  VARCHAR(16)   DEFAULT NULL,
    content_type            VARCHAR(128)  DEFAULT NULL,
    x_forward_for           TEXT          DEFAULT NULL,
    accept_language         VARCHAR(256)  DEFAULT NULL,
    authorization           TEXT          DEFAULT NULL,
    transfer_encoding       VARCHAR(64)   DEFAULT NULL,
    referrer                TEXT          DEFAULT NULL,
    user_agent              TEXT          DEFAULT NULL,
    delegation              VARCHAR(256)  DEFAULT NULL,
    uid                     VARCHAR(256)  DEFAULT NULL,
    cookie                  TEXT          DEFAULT NULL,
    req_headers             JSON          DEFAULT NULL,
    req_header_len          INT           DEFAULT NULL,
    req_body_len            INT           DEFAULT NULL,
    -- 路由
    cluster                 VARCHAR(256)  DEFAULT NULL,
    sub_cluster             VARCHAR(256)  DEFAULT NULL,
    backend_info            VARCHAR(256)  DEFAULT NULL,
    backend_retry           TINYINT       DEFAULT NULL,
    -- 响应
    res_status_code         SMALLINT      DEFAULT NULL,
    res_header_len          INT           DEFAULT NULL,
    res_body_len            INT           DEFAULT NULL,
    res_content_type        VARCHAR(128)  DEFAULT NULL,
    res_location            TEXT          DEFAULT NULL,
    res_transfer_encoding   VARCHAR(64)   DEFAULT NULL,
    res_headers             JSON          DEFAULT NULL,
    -- 耗时（毫秒）
    all_time                INT           DEFAULT NULL,
    read_client_time        INT           DEFAULT NULL,
    cluster_serve_time      INT           DEFAULT NULL,
    backend_serve_time      INT           DEFAULT NULL,
    write_client_time       INT           DEFAULT NULL,
    connect_backend_time    INT           DEFAULT NULL,
    proxy_delay_time        INT           DEFAULT NULL,
    session_offset_time     INT           DEFAULT NULL,
    -- API Key 标签（写入时由 log-reader 从 ai_apikeytags 打平）
    level1Name              VARCHAR(128)  DEFAULT NULL,
    level1                  VARCHAR(128)  DEFAULT NULL,
    level2Name              VARCHAR(128)  DEFAULT NULL,
    level2                  VARCHAR(128)  DEFAULT NULL,
    level3Name              VARCHAR(128)  DEFAULT NULL,
    level3                  VARCHAR(128)  DEFAULT NULL,
    level4Name              VARCHAR(128)  DEFAULT NULL,
    level4                  VARCHAR(128)  DEFAULT NULL,
    level5Name              VARCHAR(128)  DEFAULT NULL,
    level5                  VARCHAR(128)  DEFAULT NULL,
    -- AI 可观测
    ai_target_model         VARCHAR(128)  DEFAULT NULL,
    ai_stream               TINYINT       DEFAULT NULL,
    ai_input_tokens         BIGINT        DEFAULT NULL,
    ai_output_tokens        BIGINT        DEFAULT NULL,
    ai_total_tokens         BIGINT        DEFAULT NULL,
    ai_cache_read_tokens    BIGINT        DEFAULT NULL,
    ai_cache_write_tokens   BIGINT        DEFAULT NULL,
    ai_audio_input_tokens   BIGINT        DEFAULT NULL,
    ai_audio_output_tokens  BIGINT        DEFAULT NULL,
    ai_image_count          BIGINT        DEFAULT NULL,
    ai_ttft_us              BIGINT        DEFAULT NULL,
    ai_tpot_us              BIGINT        DEFAULT NULL,
    ai_provider             VARCHAR(64)   DEFAULT NULL,
    ai_protocol             VARCHAR(64)   DEFAULT NULL,
    ai_mode                 VARCHAR(64)   DEFAULT NULL,
    ai_retry_count          INT           DEFAULT NULL,
    ai_cost_value           BIGINT        DEFAULT NULL,
    ai_cost_currency        VARCHAR(16)   DEFAULT NULL,
    ai_route_rule_hits      JSON          DEFAULT NULL,
    ai_cluster_key_names    JSON          DEFAULT NULL,
    ai_rate_limit_hits      JSON          DEFAULT NULL,
    ai_auth_reject_reason   VARCHAR(256)  DEFAULT NULL,
    ai_auth_reject_quota_plans JSON       DEFAULT NULL,
    ai_auth_hit_quota_plans JSON          DEFAULT NULL,
    -- AI 缓存/镜像/意图（2026-09-27 加列，proto v0.3.7/3.8/3.9 字段；
    -- 列序与 log-reader mod_log_mysql/field_mapper.go 扩列后一致，三处同 PR 评审；
    -- ai_cache_key、mirror_status~mirror_error 不进报表：前者 debug 专用防膨胀，
    -- 后者 bfe 设计即走 Prometheus 不回写日志）
    ai_cache_status         VARCHAR(16)   NOT NULL DEFAULT '',
    mirror_hit              TINYINT       NOT NULL DEFAULT 0,
    mirror_cluster          VARCHAR(128)  NOT NULL DEFAULT '',
    ai_intent_question      VARCHAR(64)   NOT NULL DEFAULT '',
    ai_intent_answer        VARCHAR(64)   NOT NULL DEFAULT '',
    ai_intent_confidence    DOUBLE        DEFAULT NULL,
    ai_intent_source        VARCHAR(32)   NOT NULL DEFAULT '',
    ai_intent_latency_us    BIGINT        DEFAULT NULL,
    ai_intent_cache_hit     TINYINT       DEFAULT NULL,
    ai_intent_questions_version VARCHAR(32) NOT NULL DEFAULT '',
    -- 缺价信号（2026-10-30 加列，issue #1479，bfe-access-pb v0.3.17 字段 763/766；
    -- NULL = 未发生缺价。ai_price_warn 取值：fallback/no_entry/no_baseline/
    -- reject/subitem_missing/no_model_table；与 log-reader 扩列后列序一致）
    ai_price_miss           TINYINT       DEFAULT NULL,
    ai_price_warn           VARCHAR(32)   DEFAULT NULL,
    -- 幂等键：请求级判别列 logid 入键后，同秒同 Key 同模型的并发请求各自成行；
    -- 只有同一请求重放（同 logid）才命中覆盖，重发/-b 补读仍幂等（issue #1417）。
    -- 键长 (256+256+128)*4 + 长度字节 + 5(log_time) + 8(BIGINT) ≈ 2579 字节 < 3072
    -- InnoDB 上限（DYNAMIC 行格式），无需前缀索引；
    -- 唯一键包含分区列 log_time（MySQL 分区表约束）
    UNIQUE KEY uk_dedup (hostid, log_time, ai_apikey_id, ai_requested_model, logid),
    -- 索引服务于报表查询过滤维度：模型/API Key/提供商/Host/状态码 + 时间
    KEY idx_model_time (ai_target_model, log_time),
    KEY idx_apikey_time (ai_apikey_id, log_time),
    KEY idx_provider_time (ai_provider, log_time),
    KEY idx_host_time (hostid, log_time),
    KEY idx_status_time (res_status_code, log_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE (TO_DAYS(log_time)) (
    PARTITION p_init VALUES LESS THAN (TO_DAYS('${INIT_DATE}'))
);

-- ---------------------------------
-- 聚合表 bfe_ai_metrics_1m（40 维 + 24 指标）
-- 维度集合与指标列与 Doris bfe_ai_metrics_1m 对齐
--（见 ai-gateway-observability/doris/sqls/bfe_ai_metrics_1m.sql；Doris 侧
--  ai_cache_status/mirror_hit/ai_intent_answer 三维度二期 AGGREGATE KEY 重建
--  对齐，一期仅 MySQL 侧可用、查询层按 backend 能力门控），MySQL 形态为
-- 普通 InnoDB 表：
--   - 不设唯一键/主键：37 个维度列（含多个 VARCHAR(256)）无法构成 InnoDB 唯一键
--     （3072 字节上限），且幂等性由聚合 JOB 的「DELETE 窗口 + INSERT SELECT 事务」
--     保证；InnoDB 隐式 rowid 作为主键；
--   - 维度列全部 NOT NULL DEFAULT ''：聚合 JOB 写入时 IFNULL(col,'') 归一，保证
--     GROUP BY 与过滤谓词行为对齐 Doris；
--   - 维度列长度取 Doris 定义与明细表的交集（如 ai_apikey_id 聚合表 128 vs
--     明细表 256，截断口径由聚合 SQL 保证一致）；
--   - 索引围绕 ts_min 与排行热维度，与查询 WHERE ts_min BETWEEN ... GROUP BY
--     <维度> 的模式匹配。
CREATE TABLE IF NOT EXISTS bfe_ai_metrics_1m (
    ts_min             DATETIME      NOT NULL,
    hostid             VARCHAR(256)  NOT NULL DEFAULT '',
    ai_apikey_id       VARCHAR(128)  NOT NULL DEFAULT '',
    ai_requested_model VARCHAR(128)  NOT NULL DEFAULT '',
    ai_target_model    VARCHAR(128)  NOT NULL DEFAULT '',
    ai_stream          TINYINT       NOT NULL DEFAULT 0,
    product            VARCHAR(64)   NOT NULL DEFAULT '',
    cluster            VARCHAR(64)   NOT NULL DEFAULT '',
    sub_cluster        VARCHAR(64)   NOT NULL DEFAULT '',
    backend_info       VARCHAR(256)  NOT NULL DEFAULT '',
    method             VARCHAR(16)   NOT NULL DEFAULT '',
    res_status_code    SMALLINT      NOT NULL DEFAULT 0,
    err_code           VARCHAR(64)   NOT NULL DEFAULT '',
    header_host        VARCHAR(256)  NOT NULL DEFAULT '',
    ai_provider        VARCHAR(64)   NOT NULL DEFAULT '',
    ai_protocol        VARCHAR(64)   NOT NULL DEFAULT '',
    ai_mode            VARCHAR(64)   NOT NULL DEFAULT '',
    ai_cost_currency   VARCHAR(16)   NOT NULL DEFAULT '',
    level1Name         VARCHAR(128)  NOT NULL DEFAULT '',
    level1             VARCHAR(128)  NOT NULL DEFAULT '',
    level2Name         VARCHAR(128)  NOT NULL DEFAULT '',
    level2             VARCHAR(128)  NOT NULL DEFAULT '',
    level3Name         VARCHAR(128)  NOT NULL DEFAULT '',
    level3             VARCHAR(128)  NOT NULL DEFAULT '',
    level4Name         VARCHAR(128)  NOT NULL DEFAULT '',
    level4             VARCHAR(128)  NOT NULL DEFAULT '',
    level5Name         VARCHAR(128)  NOT NULL DEFAULT '',
    level5             VARCHAR(128)  NOT NULL DEFAULT '',
    rate_limit_policy_id VARCHAR(128) NOT NULL DEFAULT '',
    rate_limit_type    VARCHAR(32)   NOT NULL DEFAULT '',
    rate_limit_rule_name VARCHAR(128) NOT NULL DEFAULT '',
    ai_auth_reject_reason VARCHAR(256) NOT NULL DEFAULT '',
    ai_auth_reject_quota_plans_slot1 VARCHAR(128) NOT NULL DEFAULT '',
    ai_auth_reject_quota_plans_slot2 VARCHAR(128) NOT NULL DEFAULT '',
    ai_auth_reject_quota_plans_slot3 VARCHAR(128) NOT NULL DEFAULT '',
    ai_auth_reject_quota_plans_slot4 VARCHAR(128) NOT NULL DEFAULT '',
    ai_auth_reject_quota_plans_slot5 VARCHAR(128) NOT NULL DEFAULT '',
    -- 缓存/镜像/意图维度（2026-09-27 加列；低基数：status ≤4 值、mirror_hit 2 值、
    -- intent_answer ≤ 问题选项数；行数膨胀上线后监控，预案见报表库设计文档 §4）
    ai_cache_status    VARCHAR(16)   NOT NULL DEFAULT '',
    mirror_hit         TINYINT       NOT NULL DEFAULT 0,
    ai_intent_answer   VARCHAR(64)   NOT NULL DEFAULT '',
    -- 聚合指标（24 列，BIGINT，SUM 口径）
    request_count      BIGINT        DEFAULT NULL,
    error_count        BIGINT        DEFAULT NULL,
    auth_reject_count  BIGINT        DEFAULT NULL,
    input_tokens       BIGINT        DEFAULT NULL,
    output_tokens      BIGINT        DEFAULT NULL,
    total_tokens       BIGINT        DEFAULT NULL,
    ttft_us_sum        BIGINT        DEFAULT NULL,
    tpot_us_sum        BIGINT        DEFAULT NULL,
    req_header_bytes   BIGINT        DEFAULT NULL,
    req_body_bytes     BIGINT        DEFAULT NULL,
    res_header_bytes   BIGINT        DEFAULT NULL,
    res_body_bytes     BIGINT        DEFAULT NULL,
    rate_limit_hits    BIGINT        DEFAULT NULL,
    backend_retries    BIGINT        DEFAULT NULL,
    all_time_sum       BIGINT        DEFAULT NULL,
    cluster_serve_sum  BIGINT        DEFAULT NULL,
    backend_serve_sum  BIGINT        DEFAULT NULL,
    ai_retry_count_sum BIGINT        DEFAULT NULL,
    ai_cost_value_sum  BIGINT        DEFAULT NULL,
    cache_read_tokens  BIGINT        DEFAULT NULL,
    cache_write_tokens BIGINT        DEFAULT NULL,
    ai_audio_input_tokens  BIGINT    DEFAULT NULL,
    ai_audio_output_tokens BIGINT    DEFAULT NULL,
    ai_image_count     BIGINT        DEFAULT NULL,
    KEY idx_ts (ts_min),
    KEY idx_model_ts (ai_target_model, ts_min),
    KEY idx_apikey_ts (ai_apikey_id, ts_min),
    KEY idx_provider_ts (ai_provider, ts_min)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
PARTITION BY RANGE (TO_DAYS(ts_min)) (
    PARTITION p_init VALUES LESS THAN (TO_DAYS('${INIT_DATE}'))
);
