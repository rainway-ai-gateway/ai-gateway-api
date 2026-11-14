-- ===========================================================================
-- ai-gateway-api 数据库升级脚本：v0.0.10 -> v0.0.11（MySQL 8+）
--
-- 适用：由 v0.0.10 / v0.0.10.1 升级到 v0.0.11 的存量库。
-- 新装库请直接执行 db_ddl.sql（含全部表与种子数据）。
--
-- 使用说明：
--   1. 升级前务必备份数据库。
--   2. 本脚本为增量升级：新增列用 ALTER TABLE ADD COLUMN，新增表用
--      CREATE TABLE IF NOT EXISTS；不包含任何 DROP，避免误删数据。
--   3. MySQL 不支持 ADD COLUMN IF NOT EXISTS，脚本非幂等：已执行过的语句
--      重复执行会报 "Duplicate column/key"，跳过即可。
--   4. api_key_hash 的回填与唯一键切换见文件末尾「手动步骤」章节（SQL 无法
--      计算 HMAC-SHA256，需应用侧/外部脚本配合）。
--   5. SQLite 部署请参考 docs/zh_cn/upgrade.md 的 SQLite 对应语句。
-- ===========================================================================

-- ---------------------------------------------------------------------------
-- 1. 新增列
-- ---------------------------------------------------------------------------

-- 1.1 clusters：EPP 运行时参数
ALTER TABLE `clusters`
  ADD COLUMN `epp_runtime` text COMMENT 'EPP运行时配置(JSON: check/timeout)';

-- 1.2 api_keys：api_key 列扩容（加密态存储）+ api_key_hash 查询哈希
ALTER TABLE `api_keys`
  MODIFY COLUMN `api_key` varchar(255) NOT NULL DEFAULT '' COMMENT '具体的key（加密态存储，enc$v1$ 前缀；明文兼容）';
ALTER TABLE `api_keys`
  ADD COLUMN `api_key_hash` char(64) NOT NULL DEFAULT '' COMMENT 'api_key 的 HMAC-SHA256 查询哈希（稳定，不随加密轮换变化）';

-- 1.3 model_prices：批量价折扣系数（2026-10-07 批量任务支持）
ALTER TABLE `model_prices`
  ADD COLUMN `batch_discount` DOUBLE COMMENT '批量价折扣系数（0~1）：导出时把本行各价格键×系数展开生成 mode=batch 价格行；NULL=不展开';

-- 1.4 providers：实例供给方式（2026-09-24 k8s 实例池）
ALTER TABLE `providers`
  ADD COLUMN `instance_source` VARCHAR(32) NOT NULL DEFAULT 'instance_pool' COMMENT '实例供给方式：instance_pool（人维护）/ k8s_pool（发现组件维护）';
ALTER TABLE `providers`
  ADD COLUMN `k8s_pool_name` VARCHAR(255) NULL COMMENT '引用的 K8s 实例池名称（instance_source=k8s_pool 时有效）';
-- 注：k8s_instance_pool 镜像列随实时解析改造（2026-10-27，见
-- design-docs/modifications/2026-10-27-k8s-pool-realtime-resolution/）彻底删除，
-- 不再执行 ADD COLUMN；若预览版库已建该列，请手动 DROP：
-- ALTER TABLE `providers` DROP COLUMN `k8s_instance_pool`;

-- 1.5 rate_limit_policies：批量限流配置（2026-10-07 批量任务支持）
ALTER TABLE `rate_limit_policies`
  ADD COLUMN `batch_limits` TEXT COMMENT '批量限流配置（JSON对象）：max_create_rpm/max_active_batches/max_file_bytes/max_file_lines；省略/NULL=该策略不参与批量限流';

-- 1.6 providers：api_keys 列扩容（加密态存储，issue #264）
-- 该列入库的是「整列一条 enc$v1$ 信封」，而原声明 JSON：MySQL 严格模式对 JSON 列做语法校验，
-- 信封不是合法 JSON → Error 3140 → Provider 创建/修改整条失败。JSON<->文本只能 COPY 算法，
-- COPY 必须带锁（LOCK=SHARED：允许读、阻塞写）；providers 行数很小，锁窗口以毫秒计。
ALTER TABLE `providers`
  MODIFY COLUMN `api_keys` MEDIUMTEXT NULL COMMENT 'API key 列表（列加密整列信封，enc$v1$ 前缀；无前缀为明文 JSON 兼容）',
  ALGORITHM=COPY, LOCK=SHARED;

-- ---------------------------------------------------------------------------
-- 2. 新增表
-- ---------------------------------------------------------------------------

-- 2.1 k8s_pools（K8s 实例池表，2026-09-24）
CREATE TABLE IF NOT EXISTS `k8s_pools` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `name` VARCHAR(255) NOT NULL COMMENT 'K8s 实例池名称（控制面内普通标识符，与 K8s Service 名无耦合）',
  `instances` JSON NOT NULL COMMENT '实例列表（发现组件全量替换写入）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='K8s 实例池表';

-- 2.2 ai_cache_rules（AI 缓存规则表，2026-09-24 / 2026-09-30 含语义缓存列）
CREATE TABLE IF NOT EXISTS `ai_cache_rules` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（排序/优先级用，不暴露API）',
  `name` VARCHAR(128) NOT NULL COMMENT '规则名称（集合内唯一，可读性/审计用）',
  `cond` TEXT NOT NULL COMMENT 'BFE条件表达式，如 req_path_in(...) && req_body_json_in("model", ...)',
  `cache_key_strategy` VARCHAR(32) NOT NULL DEFAULT 'lastQuestion' COMMENT '缓存键策略：lastQuestion|allQuestions|disabled',
  `cache_ttl` INT NOT NULL DEFAULT 0 COMMENT '缓存TTL（秒），0表示不过期',
  `max_body_bytes` BIGINT NOT NULL DEFAULT 1048576 COMMENT '请求体大小上限（字节），超限不缓存',
  `max_value_bytes` BIGINT NOT NULL DEFAULT 1048576 COMMENT '缓存值大小上限（字节），超限不缓存',
  `enable_semantic_cache` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否启用语义缓存: 0-否, 1-是（二期）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI缓存规则表';

-- 2.3 ai_cache_semantic_settings（AI 缓存语义全局设置表，单行，2026-09-30）
CREATE TABLE IF NOT EXISTS `ai_cache_semantic_settings` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（恒为1语义，物理单行）',
  `top_k` INT NOT NULL DEFAULT 1 COMMENT '语义检索TopK（1-10）',
  `threshold` DOUBLE NOT NULL DEFAULT 0.15 COMMENT '相似度阈值（量纲与 threshold_relation 一致，0-2）',
  `threshold_relation` VARCHAR(8) NOT NULL DEFAULT 'lt' COMMENT '阈值比较: gt/gte/lt/lte',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI缓存语义全局设置表（单行）';

-- 2.4 ai_context_rules（AI 上下文压缩规则表，2026-10-01）
CREATE TABLE IF NOT EXISTS `ai_context_rules` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（数组顺序=id 升序=优先级）',
  `cond` VARCHAR(1024) NOT NULL COMMENT 'bfe 条件表达式',
  `mode` VARCHAR(16) NOT NULL COMMENT '压缩档位: off/conservative/balanced/aggressive',
  `max_context_tokens` INT NOT NULL DEFAULT 0 COMMENT '预算上限覆盖(token), 0=用模型表窗口',
  `reserve_tokens` INT NOT NULL DEFAULT 0 COMMENT '预留输出token, 0=自动clamp(窗口x15%,256,16000)',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI上下文压缩规则表';

-- 2.5 ai_context_settings（AI 上下文压缩全局设置表，单行，2026-10-01）
CREATE TABLE IF NOT EXISTS `ai_context_settings` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（恒为1语义，物理单行）',
  `trigger_ratio` DOUBLE NOT NULL DEFAULT 0.7 COMMENT 'proactive触发阈值(占预算比例, 0-1]',
  `keep_latest_images` INT NOT NULL DEFAULT 2 COMMENT '保留最近N张内联图片, 0=不裁图',
  `tool_result_max_chars` INT NOT NULL DEFAULT 2000 COMMENT '单条tool结果最大字符数, 0=不截断',
  `thinking_policy` VARCHAR(32) NOT NULL DEFAULT 'trim-all-but-last' COMMENT 'thinking块策略: trim-all-but-last/keep',
  `chars_per_token` INT NOT NULL DEFAULT 4 COMMENT '文本估算系数(字节/token), 中文密集可调3',
  `image_token_estimate` INT NOT NULL DEFAULT 1200 COMMENT '单张内联图片估值token',
  `rewrite_strength` VARCHAR(8) NOT NULL DEFAULT 'lite' COMMENT '改写强度: lite/full',
  `rewrite_protected_survival_rate` DOUBLE NOT NULL DEFAULT 0.95 COMMENT 'fidelity gate保护token存活率阈值(0-1]',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI上下文压缩全局设置表（单行）';

-- 2.6 traffic_mirror_rules（流量镜像规则表，2026-09-25）
CREATE TABLE IF NOT EXISTS `traffic_mirror_rules` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（排序/优先级用，不暴露API）',
  `name` VARCHAR(128) NOT NULL COMMENT '规则名称（集合内唯一，可读性/审计用）',
  `cond` TEXT COMMENT 'BFE条件表达式，如 req_path_prefix_in(...) && req_body_json_in("model", ...)；空串/NULL=全匹配',
  `mirror_cluster` VARCHAR(128) NOT NULL COMMENT '镜像目标cluster名（引用cluster表，PUT时校验存在性）',
  `percentage` INT NOT NULL DEFAULT 100 COMMENT '镜像采样百分比（0-100），0=命中但不采样',
  `remove_headers` TEXT COMMENT '镜像副本剔除Header黑名单（JSON数组）；NULL=控制面导出时填默认黑名单',
  `set_headers` TEXT COMMENT '镜像副本注入Header（JSON对象）；NULL=不注入',
  `body_rewrites` TEXT COMMENT 'body字段改写（JSON数组[{"path","value"}]）；一期path仅允许model；NULL=不改写',
  `path_rewrite` VARCHAR(1024) COMMENT '镜像路径整体替换（query保留）；NULL/空串=不改写',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='流量镜像规则表';

-- 2.7 intent_config（AI 意图配置表，单行，2026-09-26）
CREATE TABLE IF NOT EXISTS `intent_config` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID（固定 id=1，单行覆盖式存储）',
  `version` VARCHAR(32) NOT NULL COMMENT '当前配置版本，时间戳格式 yyyyMMddHHmmss，每次 PUT 更新',
  `min_confidence` DECIMAL(4,3) NOT NULL DEFAULT 0.600 COMMENT '全局置信度门控阈值 [0,1]',
  `questions` TEXT NOT NULL COMMENT '问题集 JSON（可为 []，表示停用意图分类），结构同 BFE intent_questions.data 的 Questions[]',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI意图配置表（mod_ai_intent intent_questions.data 数据源：问题集+门控阈值，单行覆盖式存储）';

-- 2.8 keyrotate_sweep_tasks / keyrotate_sweep_lock（密钥收敛任务表/单行锁，2026-10-05）
CREATE TABLE IF NOT EXISTS `keyrotate_sweep_tasks` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `task_id` varchar(64) NOT NULL comment "任务标识",
  `status` varchar(16) NOT NULL comment "running | succeeded | failed",
  `mode` varchar(16) NOT NULL comment "reencrypt | decrypt",
  `dry_run` tinyint(1) NOT NULL DEFAULT 0,
  `scope` varchar(16) NOT NULL DEFAULT 'all',
  `active_key_id` int NOT NULL DEFAULT 0,
  `scanned` bigint NOT NULL DEFAULT 0,
  `rewritten` bigint NOT NULL DEFAULT 0,
  `summary` text comment "按表分组计数 JSON",
  `error` varchar(1024) NOT NULL DEFAULT '',
  `heartbeat_at` datetime NOT NULL comment "执行心跳，失联接管判定依据",
  `started_at` datetime NOT NULL,
  `finished_at` datetime DEFAULT NULL,
  `duration_ms` bigint NOT NULL DEFAULT 0,
  `created_by` varchar(255) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_krst_task_id` (`task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 comment = "密钥收敛任务表";

CREATE TABLE IF NOT EXISTS `keyrotate_sweep_lock` (
  `id` bigint(20) NOT NULL,
  `holder_task_id` varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 comment = "密钥收敛任务单行锁（id 恒为 1）";
INSERT IGNORE INTO `keyrotate_sweep_lock` (`id`) VALUES (1);

-- 2.9 batch_files（批量文件元数据表，2026-10-07）
CREATE TABLE IF NOT EXISTS `batch_files` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `file_id` VARCHAR(128) NOT NULL COMMENT 'provider 侧文件 ID',
  `provider` VARCHAR(128) NOT NULL COMMENT 'provider 名',
  `key_name` VARCHAR(128) COMMENT '上传时使用的 provider key 名',
  `api_key_id` VARCHAR(128) NOT NULL COMMENT 'API-Key ID（逻辑引用 api_keys.id）',
  `product_name` VARCHAR(128) NOT NULL COMMENT '产品线名称（强制隔离维度）',
  `direction` VARCHAR(8) NOT NULL COMMENT '方向：input | output',
  `purpose` VARCHAR(32) COMMENT '文件用途（如 batch）',
  `filename` VARCHAR(256) COMMENT '文件名（provider File 对象给出，可空）',
  `line_count` INT COMMENT '文件行数（预留金额估算与对账用）',
  `bytes` BIGINT COMMENT '文件字节数',
  `first_seen_at` DATETIME COMMENT '首次见到时间',
  `last_seen_at` DATETIME COMMENT '最近见到时间（幂等 upsert 刷新）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_file_provider` (`file_id`, `provider`),
  KEY `idx_apikey` (`api_key_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='批量文件元数据表';

-- 2.10 batch_tasks（批量任务表，2026-10-07）
CREATE TABLE IF NOT EXISTS `batch_tasks` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `batch_id` VARCHAR(128) NOT NULL COMMENT 'provider 侧 batch ID（业务标识，详情/取消端点寻址键）',
  `api_key_id` VARCHAR(128) NOT NULL COMMENT 'API-Key ID（逻辑引用 api_keys.id）',
  `product_name` VARCHAR(128) NOT NULL COMMENT '产品线名称（管控 API 强制隔离维度）',
  `entity_id` VARCHAR(128) COMMENT 'Entity ID（可空）',
  `provider` VARCHAR(128) NOT NULL COMMENT 'provider 名',
  `key_name` VARCHAR(128) COMMENT '控制面直调 provider（cancel/兜底结算下载输出文件）用 key 名',
  `endpoint` VARCHAR(256) COMMENT '批量端点（如 /v1/chat/completions）',
  `input_file_id` VARCHAR(128) COMMENT '输入文件 ID',
  `output_file_id` VARCHAR(128) COMMENT '输出文件 ID（状态推进时发现即补登记输出 batch_files）',
  `status` VARCHAR(16) NOT NULL COMMENT '状态机，对齐 OpenAI：validating/queued/in_progress/finalizing/completed/expired/failed/cancelled/cancelling；二期方言（ended/deleted）为枚举增量；provider 返回状态原样存、终态不回退',
  `request_counts` JSON COMMENT '请求计数对象（provider 原样）',
  `est_lines` INT COMMENT '预估行数（创建时取自输入文件）',
  `usage_input_tokens` BIGINT COMMENT '结算 input tokens',
  `usage_output_tokens` BIGINT COMMENT '结算 output tokens',
  `usage_source` VARCHAR(16) COMMENT '用量来源：download（下载拦截解析）| reconcile（对账 job 兜底解析）',
  `reserve_units` BIGINT COMMENT '预留金额（1e-8 定点 RMB，与数据面同口径：行数×预估 token 上限×batch 价）',
  `settle_units` BIGINT COMMENT '实结金额（1e-8 定点 RMB；实结允许透支预留，over_reserved=1 标记）',
  `settle_status` VARCHAR(16) NOT NULL COMMENT '三态对账主字段：reserved | settled | released；兜底结算与释放均按 WHERE settle_status=reserved 条件更新推进，防双倍结算',
  `over_reserved` TINYINT DEFAULT 0 COMMENT '实际超出预留标记（允许透支，不静默截断）',
  `sync_source` VARCHAR(8) DEFAULT 'redis' COMMENT '同步来源：redis | log（日志补登标记，对账巡检告警）',
  `idem_key` VARCHAR(128) NOT NULL UNIQUE COMMENT '幂等键，恒等于 batch_id；Redis SCAN 同步 upsert 的冲突定位键',
  `created_at` DATETIME COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  `terminal_at` DATETIME COMMENT '进入终态时间',
  PRIMARY KEY (`id`),
  KEY `idx_status` (`status`),
  KEY `idx_apikey` (`api_key_id`),
  KEY `idx_terminal` (`terminal_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='批量任务表';

-- ---------------------------------------------------------------------------
-- 3. 手动步骤（SQL 无法自动完成，需人工/外部脚本执行一次）
-- ---------------------------------------------------------------------------

-- 3.1 api_key_hash 回填与唯一键切换
-- SQL 无法计算 HMAC-SHA256，需对存量行按下列规则回填：
--   api_key_hash = hex( HMAC-SHA256( key = sha256('rainway-ai-gateway:api-key-hash-pepper:v1'),
--                                    msg = api_key 明文 ) )
-- 该 pepper 为应用级常量（见 lib/xcrypto/keyring.go apiKeyHashPepper），恒定不随 keyring 轮换变化。
-- 回填完成后，再执行唯一键切换（回填前不要执行，否则多行 '' 会冲突）：

-- ALTER TABLE `api_keys` DROP INDEX `uk_api_key`;
-- ALTER TABLE `api_keys` ADD UNIQUE KEY `uk_api_key_hash` (`api_key_hash`);

-- 3.2 初始 admin 口令
-- 新装库（db_ddl.sql）已将 admin 初始口令写为 bcrypt 哈希。
-- 存量库若仍为明文 'admin'，无需在此处理：应用登录时按需惰性重哈希
-- （见 lib/xcrypto/password.go CheckPassword 的 needMigrate）。

-- 3.3 已在 2026-10-20 之前执行过本脚本的存量库（issue #264 补丁）
-- 只需补执行 §1.6 一条语句（脚本非幂等，不要整体重跑）：
-- ALTER TABLE `providers`
--   MODIFY COLUMN `api_keys` MEDIUMTEXT NULL COMMENT 'API key 列表（列加密整列信封，enc$v1$ 前缀；无前缀为明文 JSON 兼容）',
--   ALGORITHM=COPY, LOCK=SHARED;
-- 回滚（改回 JSON）前必须先执行 POST /open-api/v1/security/reencrypt-sweeps
-- {"mode":"decrypt","scope":"all"} 并复核无 enc$v1$ 残留，否则 ALTER 会因信封不是
-- 合法 JSON 而报 Error 3140；改回 JSON 后文本值需为合法 JSON 才能通过。SQLite 部署
-- 无需此步骤（TEXT 无需变更）。

-- ---------------------------------------------------------------------------
-- 4. 删除列（EPP 单实例化，见 design-docs/modifications/2026-10-14-epp-single-instance/）
-- ---------------------------------------------------------------------------

-- 4.1 epp_assignments：删除主实例 id 列（分配粒度收敛为实例组，实例由组派生）
-- 建议升级前先将 EPP 实例池收敛为单实例组（旧版本最后一次 PATCH /epp-pool
-- 写入单实例组）；未收敛时组内实例按 id 序派生，可能出现一次指向切换。
ALTER TABLE `epp_assignments` DROP COLUMN `primary_instance_id`;

-- 回滚（仅供应急回滚 v0.0.11 该变更；回滚后应用按旧契约运行，列值需先回填，
-- 否则 UpsertAssignment 写入会报错）：
-- ALTER TABLE `epp_assignments`
--   ADD COLUMN `primary_instance_id` varchar(128) NOT NULL DEFAULT '' COMMENT '主实例id';
-- 回填方式（按"分配的组内 id 序第一条实例"恢复语义）：
--   UPDATE `epp_assignments` AS a
--   JOIN (
--     SELECT group_name, MIN(id) AS first_id
--     FROM `epp_instances` GROUP BY group_name
--   ) AS g ON a.group_name = g.group_name
--   SET a.primary_instance_id = g.first_id;

-- ---------------------------------------------------------------------------
-- 5. 新增索引（operation-logs 过滤列组合索引，issue #280）
-- ---------------------------------------------------------------------------
-- 背景：issue #275（commit 75d0c51）已为 db_ddl.sql（全量建库）定义
-- idx_operator_name_created / idx_status_created / idx_resource_name_created
-- 三个过滤列组合索引，但本脚本此前未包含对应 ALTER，按本脚本升级的存量库
-- 过滤查询仍全表扫描（25 万行规模首页 3.6-20 s，issue #280）。
-- 本期再补 idx_resource_id_created：现有 idx_resource(resource_type, resource_id)
-- 前导列是 resource_type，resource_id 单独过滤用不上（实测 17.4 s vs 56 ms）。
-- 已在 2026-10-23 之前执行过本脚本的存量库：无需重跑全文，只需补执行本节
-- （脚本非幂等，重复执行会报 "Duplicate key"，跳过即可）。
ALTER TABLE `operation_logs`
  ADD INDEX `idx_operator_name_created` (`operator_name`, `created_at`),
  ADD INDEX `idx_status_created` (`status`, `created_at`),
  ADD INDEX `idx_resource_name_created` (`resource_name`, `created_at`),
  ADD INDEX `idx_resource_id_created` (`resource_id`, `created_at`),
  ALGORITHM=INPLACE, LOCK=NONE;

-- 回滚（仅供应急）：
-- ALTER TABLE `operation_logs`
--   DROP INDEX `idx_operator_name_created`,
--   DROP INDEX `idx_status_created`,
--   DROP INDEX `idx_resource_name_created`,
--   DROP INDEX `idx_resource_id_created`;

-- ===========================================================================
-- 升级完成。完整新装 schema 见 db_ddl.sql。
-- ===========================================================================