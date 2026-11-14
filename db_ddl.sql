DROP DATABASE IF EXISTS `open_bfe`;
CREATE DATABASE open_bfe;

USE open_bfe;

-- create bfe_clusters
DROP TABLE IF EXISTS `bfe_clusters`;
CREATE TABLE `bfe_clusters` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `pool_name` varchar(255) NOT NULL DEFAULT '',
  `capacity` bigint(20) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `gtc_enabled` tinyint(1) NOT NULL DEFAULT '1',
  `gtc_manual_enabled` tinyint(1) NOT NULL DEFAULT '1',
  `exempt_traffic_check` tinyint(1) NOT NULL DEFAULT '0',
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_uni` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create products
DROP TABLE IF EXISTS `products`;
CREATE TABLE `products` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL ,
  `mail_list` varchar(4096) NOT NULL ,
  `contact_person` varchar(4096) NOT NULL ,
  `sms_list` varchar(4096) NOT NULL DEFAULT "no sms" ,
  `description` varchar(1024) NOT NULL DEFAULT "no desc" ,
  `created_at` datetime NOT NULL ,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP ,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_uni` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create domains
DROP TABLE IF EXISTS `domains`;
CREATE TABLE `domains` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) CHARACTER SET utf8 COLLATE utf8_unicode_ci NOT NULL,
  `product_id` bigint(20) NOT NULL,
  `type` int(11) NOT NULL,
  `using_advanced_redirect` tinyint(1) NOT NULL DEFAULT 0,
  `using_advanced_hsts` tinyint(1) NOT NULL DEFAULT 0,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_uni` (`name`),
  INDEX `product_id` (`product_id`),
  INDEX `type` (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create clusters
DROP TABLE IF EXISTS `clusters`;
CREATE TABLE `clusters` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) CHARACTER SET utf8 COLLATE utf8_unicode_ci NOT NULL,
  `description` varchar(1024) CHARACTER SET utf8 COLLATE utf8_unicode_ci NOT NULL DEFAULT "no desc",
  `product_id` bigint(20) NOT NULL,
  `protocol` varchar(255) NOT NULL DEFAULT 'http' COMMENT 'http or https',
  `max_idle_conn_per_host` smallint(6) NOT NULL DEFAULT '2',
  `timeout_conn_serv` int(11) NOT NULL DEFAULT '50000',
  `timeout_response_header` int(11) NOT NULL DEFAULT '50000',
  `timeout_readbody_client` int(11) NOT NULL DEFAULT '30000',
  `timeout_read_client_again` int(11) NOT NULL DEFAULT '30000',
  `timeout_write_client` int(11) NOT NULL DEFAULT '60000',
  `healthcheck_schem` varchar(16) NOT NULL DEFAULT 'http',
  `healthcheck_interval` int(11) NOT NULL DEFAULT '1000',
  `healthcheck_failnum` int(11) NOT NULL DEFAULT '10',
  `healthcheck_host` varchar(255) NOT NULL,
  `healthcheck_uri` varchar(255) NOT NULL,
  `healthcheck_statuscode` int(11) NOT NULL DEFAULT '200',
  `clientip_carry` tinyint(4) NOT NULL DEFAULT '0',
  `port_carry` tinyint(1) NOT NULL DEFAULT '0',
  `max_retry_in_cluster` tinyint(4) NOT NULL DEFAULT '3',
  `max_retry_cross_cluster` tinyint(4) NOT NULL DEFAULT '0',
  `ready` tinyint(1) NOT NULL DEFAULT '1',
  `hash_strategy` int NOT NULL DEFAULT '0',
  `cookie_key` varchar(255) NOT NULL DEFAULT 'BAIDUID',
  `hash_header` varchar(255) NOT NULL DEFAULT 'Cookie:BAIDUID',
  `session_sticky` tinyint(1) NOT NULL DEFAULT '0',
  `req_write_buffer_size` int(11) NOT NULL DEFAULT '512',
  `req_flush_interval` int(11) NOT NULL DEFAULT '0',
  `res_flush_interval` int(11) NOT NULL DEFAULT '20',
  `cancel_on_client_close` tinyint(1) NOT NULL DEFAULT '0',
  `failure_status` tinyint(1) NOT NULL DEFAULT '0',
  `max_conns_per_host` int(11) NOT NULL DEFAULT '0',
  `llm_config` text,
  `balance_mode` varchar(16) NOT NULL DEFAULT 'WRR' COMMENT '均衡模式：WRR/EPP',
  `epp_config` text COMMENT 'EPP调度配置（简化用户形态JSON）',
  `epp_runtime` text COMMENT 'EPP运行时配置(JSON: check/timeout)',
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_index` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;


-- create epp_instances (EPP实例池表)
DROP TABLE IF EXISTS `epp_instances`;
CREATE TABLE `epp_instances` (
  `id` varchar(128) NOT NULL COMMENT '实例id，池内全局唯一',
  `host` varchar(255) NOT NULL COMMENT '实例主机名或IP（IPv6字面量不带括号）',
  `port` int(11) NOT NULL COMMENT '实例端口',
  `group_name` varchar(128) NOT NULL COMMENT '实例组名',
  `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_host_port` (`host`, `port`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='EPP实例池表';

-- create epp_assignments (EPP分配表，分配粒度为实例组)
DROP TABLE IF EXISTS `epp_assignments`;
CREATE TABLE `epp_assignments` (
  `cluster` varchar(255) NOT NULL COMMENT 'cluster名',
  `group_name` varchar(128) NOT NULL COMMENT '实例组名',
  `create_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `update_time` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`cluster`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='EPP分配表';


-- create lb_matrices
DROP TABLE IF EXISTS `lb_matrices`;
CREATE TABLE `lb_matrices` (
  `cluster_id` bigint(20) NOT NULL AUTO_INCREMENT,
  `lb_matrix` varchar(8192) NOT NULL,
  `product_id` bigint(20) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`cluster_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create sub_clusters
DROP TABLE IF EXISTS `sub_clusters`;
CREATE TABLE `sub_clusters` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `cluster_id` bigint(20) NOT NULL,
  `product_id` bigint(20) NOT NULL,
  `description` varchar(1024) CHARACTER SET utf8 COLLATE utf8_unicode_ci NOT NULL DEFAULT "no desc",
  `bns_name_id` bigint(20) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT '1',
  `role` varchar(32) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_product_index` (`name`, `product_id`),
  INDEX `cluster_id` (`cluster_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;


-- create pools
DROP TABLE IF EXISTS `pools`;
CREATE TABLE `pools` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `product_id` bigint(20) NOT NULL DEFAULT 0,
  `ready`  boolean NOT NULL DEFAULT 1,
  `instance_detail` mediumtext,
  `type` tinyint(4) NOT NULL DEFAULT 1,
  `tag` tinyint(4) NOT NULL DEFAULT 0,
  `role` varchar(32) NOT NULL DEFAULT 'COMMON',
  `epp_server` mediumtext,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_uni` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;


-- create route_basic_rules
DROP TABLE IF EXISTS `route_basic_rules`;
CREATE TABLE `route_basic_rules` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `description` varchar(1024) NOT NULL DEFAULT '',
  `product_id` bigint(20) NOT NULL,
  `host_names` text NOT NULL,
  `paths` text NOT NULL,
  `cluster_id` bigint(20) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  INDEX `product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;


-- create route_advance_rules
DROP TABLE IF EXISTS `route_advance_rules`;
CREATE TABLE `route_advance_rules` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `description` varchar(1024) NOT NULL DEFAULT '',
  `product_id` bigint(20) NOT NULL,
  `expression` varchar(4096) binary NOT NULL,
  `cluster_id` bigint(20) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  INDEX `product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create forward_cases
DROP TABLE IF EXISTS `route_cases`;
CREATE TABLE `route_cases` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `description` varchar(1024) NOT NULL DEFAULT '',
  `product_id` bigint(20) NOT NULL,
  `url` varchar(4096) NOT NULL,
  `method` varchar(255) NOT NULL DEFAULT "",
  `protocol` varchar(255) NOT NULL DEFAULT "",
  `header` varchar(4096) NOT NULL,
  `body` varchar(4096) NOT NULL DEFAULT "",
  `expect_cluster` varchar(255) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  INDEX `product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create certificates
DROP TABLE IF EXISTS `certificates`;
CREATE TABLE `certificates` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `cert_name` varchar(255) NOT NULL,
  `description` varchar(1024) NOT NULL DEFAULT 'no desc',
  `is_default` tinyint(1) NOT NULL DEFAULT '0',
  `expired_date` varchar(255) NOT NULL,
  `cert_file_path` varchar(255) NOT NULL,
  `key_file_path` varchar(255) NOT NULL,

  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`cert_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create extra_files
DROP TABLE IF EXISTS `extra_files`;
CREATE TABLE `extra_files` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `product_id` bigint(20) NOT NULL DEFAULT 0,
  `description` varchar(1024) NOT NULL DEFAULT '',
  `md5` varchar(64) NOT NULL,
  `content` mediumtext,
  `created_at` datetime NOT NULL ,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP ,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_product` (`name`, `product_id`),
  INDEX `product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

DROP TABLE IF EXISTS `config_versions`;
CREATE TABLE `config_versions` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `data_sign` varchar(255) NOT NULL,
  `version` varchar(255) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name_version` (`name`, `version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;


-- create users
DROP TABLE IF EXISTS `users`;
CREATE TABLE `users` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `type` tinyint(1) NOT NULL DEFAULT '0',
  `password` varchar(255) NOT NULL DEFAULT '',
  `ticket` varchar(20) NOT NULL DEFAULT '',
  `ticket_created_at` datetime NOT NULL DEFAULT '0000-01-01 00:00:00',
  `scopes` varchar(2048) NOT NULL DEFAULT '',
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_uni` (`name`, `type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;

-- create user_products
DROP TABLE IF EXISTS `user_products`;
CREATE TABLE `user_products` (
  `user_id` bigint(20) NOT NULL,
  `product_id` bigint(20) NOT NULL,
  `created_at` datetime NOT NULL,
  `updated_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`user_id`, `product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;


-- create api_keys
DROP TABLE IF EXISTS `api_keys`;
CREATE TABLE api_keys (
  `inner_id` bigint(20) NOT NULL AUTO_INCREMENT comment "内部id",
  `id` varchar(255) NOT NULL DEFAULT '' comment "API-Key标识",
  `enable` boolean NOT NULL DEFAULT false comment "api keys开关",
  `api_key` varchar(255) NOT NULL default '' comment "具体的key（加密态存储，enc$v1$ 前缀；明文兼容）",
  `api_key_hash` char(64) NOT NULL DEFAULT '' comment "api_key 的 HMAC-SHA256 查询哈希（稳定，不随加密轮换变化）",
  `description` varchar(512) DEFAULT '' comment "API-Key描述",
  `unlimited_quota` tinyint(1) DEFAULT 0 comment "是否无限配额：0-有限，1-无限",
  `product_name` varchar(255) NOT NULL DEFAULT '' comment "产品线名称",
  `expired_time` bigint NOT NULL DEFAULT -1 comment "过期时间（Unix时间戳，-1表示永不过期）",
  `allowed_models` text comment "允许的模型",
  `subnet` text comment "允许的客户端子网（JSON数组）",
  `entity_id` varchar(64) DEFAULT NULL comment "挂载的Entity ID",
  `quota_plan_id` bigint DEFAULT NULL comment "配额计划ID",
  `rate_limit_policy_id` bigint DEFAULT NULL comment "限流策略ID",
  `route_rules_id` bigint DEFAULT NULL comment "路由规则ID",
  `created_at` datetime NOT NULL DEFAULT '0000-01-01 00:00:00' COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP  comment "更新时间",
  PRIMARY KEY (`inner_id`),
  UNIQUE KEY `uk_id` (`id`),
  UNIQUE KEY `uk_api_key_hash` (`api_key_hash`),
  INDEX idx_product_name (product_name),
  INDEX idx_entity_id (entity_id),
  INDEX idx_quota_plan_id (quota_plan_id),
  INDEX idx_rate_limit_policy_id (rate_limit_policy_id),
  INDEX idx_route_rules_id (route_rules_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 comment = "api keys";

-- create api_key_id_seq
DROP TABLE IF EXISTS `api_key_id_seq`;
CREATE TABLE `api_key_id_seq` (
  `product_name` varchar(255) NOT NULL COMMENT '产品线名称',
  `next_seq` bigint NOT NULL DEFAULT '1' COMMENT '下一个可用序号',
  PRIMARY KEY (`product_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='API-Key ID 序号分配表';

-- create api_key_tokens
DROP TABLE IF EXISTS `api_key_tokens`;
CREATE TABLE api_key_tokens (
  `id` bigint NOT NULL AUTO_INCREMENT comment "表ID",
  `api_key` varchar(128) NOT NULL DEFAULT '' comment "存储的api_key",
  `created_at` datetime NOT NULL DEFAULT '0000-01-01 00:00:00' COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP  comment "更新时间",
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_key` (`api_key`)
)ENGINE=InnoDB DEFAULT CHARSET=utf8 comment = "api-key存储表"; 

-- create ai_route_rules
DROP TABLE IF EXISTS `ai_route_rules`;
CREATE TABLE `ai_route_rules` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT COMMENT '唯一ID',
  `name` varchar(255) NOT NULL DEFAULT '' COMMENT '规则名称',
  `basic` text NOT NULL COMMENT '基础配置',
  `product_name` varchar(255) NOT NULL DEFAULT '' COMMENT '产品线名称',
  `idx` bigint(20) NOT NULL DEFAULT '0' COMMENT '排序索引',
  `created_at` datetime NOT NULL DEFAULT '0000-01-01 00:00:00' COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `name_idx` (`name`),
  INDEX `idx_index` (`idx`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT = 'AI路由规则表';

-- create route_default_rules
DROP TABLE IF EXISTS `route_default_rules`;
CREATE TABLE `route_default_rules` (
`id` bigint(20) NOT NULL AUTO_INCREMENT comment "唯一id",
`cmd` varchar(20) NOT NULL comment "命令",
`params` text COMMENT '命令参数',
`product_id` bigint(20) NOT NULL comment "产品线id",
`route_action` text COMMENT '转发动作',
`description` varchar(1024) NOT NULL DEFAULT '' comment "描述",
`created_at` datetime NOT NULL DEFAULT '0000-01-01 00:00:00' COMMENT '创建时间',
`updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP  comment "更新时间",
PRIMARY KEY (`id`),
UNIQUE INDEX `product_id` (`product_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8 comment = "默认转发规则信息";

-- create entity_types (Entity类型定义表)
DROP TABLE IF EXISTS `entity_types`;
CREATE TABLE `entity_types` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `type_name` VARCHAR(32) NOT NULL COMMENT '类型标识',
  `description` VARCHAR(256) DEFAULT '' COMMENT '类型描述',
  `level` INT NOT NULL DEFAULT 1 COMMENT '层级级别（1-5，数字越小级别越高）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_type_name` (`type_name`),
  INDEX `idx_level` (`level`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Entity类型定义表';

-- create entities (Entity实体表)
DROP TABLE IF EXISTS `entities`;
CREATE TABLE `entities` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `entity_id` VARCHAR(64) NOT NULL COMMENT 'Entity唯一标识（业务ID）',
  `name` VARCHAR(128) NOT NULL COMMENT 'Entity名称',
  `description` VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'Entity描述',
  `type` VARCHAR(32) NOT NULL COMMENT 'Entity类型（关联entity_types.type_name）',
  `parent_id` VARCHAR(64) DEFAULT NULL COMMENT '父Entity ID',
  `allow_models` TEXT COMMENT '允许访问的模型白名单（JSON数组）',
  `block_models` TEXT COMMENT '禁止访问的模型黑名单（JSON数组）',
  `quota_plan_id` BIGINT DEFAULT NULL COMMENT '配额计划ID',
  `rate_limit_policy_id` BIGINT DEFAULT NULL COMMENT '限流策略ID',
  `route_rules_id` BIGINT DEFAULT NULL COMMENT '路由规则ID',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_entity_id` (`entity_id`),
  UNIQUE KEY `uk_name` (`name`),
  INDEX `idx_parent_id` (`parent_id`),
  INDEX `idx_type` (`type`),
  INDEX `idx_quota_plan_id` (`quota_plan_id`),
  INDEX `idx_rate_limit_policy_id` (`rate_limit_policy_id`),
  INDEX `idx_route_rules_id` (`route_rules_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Entity实体表';

-- create entity_id_seq (Entity ID序号分配表)
DROP TABLE IF EXISTS `entity_id_seq`;
CREATE TABLE `entity_id_seq` (
  `name` varchar(32) NOT NULL COMMENT '序号键（固定为entity）',
  `next_seq` bigint NOT NULL DEFAULT '1' COMMENT '下一个可用序号',
  PRIMARY KEY (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Entity ID序号分配表';

-- create quota_plans (配额计划表)
DROP TABLE IF EXISTS `quota_plans`;
CREATE TABLE `quota_plans` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `unlimited` TINYINT(1) DEFAULT 1 COMMENT '是否无限配额：0-有限，1-无限',
  `pass_when_no_enough_quota` TINYINT(1) DEFAULT 0 COMMENT '配额不足时是否放行：0-拒绝，1-放行',
  `quota` DECIMAL(18,8) DEFAULT 0 COMMENT '配额总量',
  `unit` VARCHAR(32) DEFAULT 'total_token' COMMENT '配额单位：total_token/RMB',
  `reset_period` VARCHAR(16) DEFAULT 'never' COMMENT '配额重置周期：never/weekly/monthly，重置均基于日历周期（如自然周/自然月）',
  `last_reset_at` DATETIME DEFAULT NULL COMMENT '上次重置时间',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  INDEX `idx_unlimited` (`unlimited`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='配额计划表';

-- create model_prices (模型定价表)
DROP TABLE IF EXISTS `model_prices`;
CREATE TABLE `model_prices` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `provider` VARCHAR(255) NOT NULL COMMENT 'Provider / Cluster 标识',
  `model` VARCHAR(255) NOT NULL COMMENT '模型名',
  `base_model` VARCHAR(255) NOT NULL COMMENT '归一化基础模型名',
  `mode` VARCHAR(50) NOT NULL COMMENT '模型模式',
  `capabilities` JSON COMMENT '能力列表',
  `supported_parameters` JSON COMMENT '支持的请求参数列表',
  `limits` JSON COMMENT '限制对象',
  `prices` JSON NOT NULL COMMENT '价格对象',
  `tier_prices` JSON COMMENT '分时段价格对象',
  `batch_discount` DOUBLE COMMENT '批量价折扣系数（0~1，2026-10-07新增）：导出时把本行各价格键×系数展开生成 mode=batch 价格行；NULL=不展开',
  `price_currency` VARCHAR(10) DEFAULT 'RMB' COMMENT '币种',
  `metadata` JSON COMMENT '元数据',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_provider_model_mode` (`provider`, `model`, `mode`),
  INDEX `idx_provider` (`provider`),
  INDEX `idx_mode` (`mode`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='模型定价表';

-- create providers (模型提供商表)
DROP TABLE IF EXISTS `providers`;
CREATE TABLE `providers` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `name` VARCHAR(255) NOT NULL COMMENT 'Provider 标识',
  `description` VARCHAR(1024) NOT NULL DEFAULT '' COMMENT '描述',
  `model_endpoint` JSON COMMENT '模型发现端点配置',
  `models` JSON COMMENT '支持的模型列表',
  `api_keys` MEDIUMTEXT NULL COMMENT 'API key 列表（列加密整列信封，enc$v1$ 前缀；无前缀为明文 JSON 兼容）',
  `instance_pool` JSON NOT NULL COMMENT '实例池列表（instance_source=instance_pool 时有效）',
  `instance_source` VARCHAR(32) NOT NULL DEFAULT 'instance_pool' COMMENT '实例供给方式：instance_pool（人维护）/ k8s_pool（发现组件维护）',
  `k8s_pool_name` VARCHAR(255) NULL COMMENT '引用的 K8s 实例池名称（instance_source=k8s_pool 时有效）',
  `model_protocols` JSON NOT NULL COMMENT '支持的模型协议列表',
  `protocol_paths` JSON COMMENT '按协议的上游路径前缀（protocol -> base path）',
  `time_zone` VARCHAR(255) NOT NULL DEFAULT 'Asia/Shanghai' COMMENT '计算时段所使用的时区',
  `tiers` JSON COMMENT '时段 tier 定义列表',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='模型提供商表';

-- create k8s_pools (K8s 实例池表)
DROP TABLE IF EXISTS `k8s_pools`;
CREATE TABLE `k8s_pools` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `name` VARCHAR(255) NOT NULL COMMENT 'K8s 实例池名称（控制面内普通标识符，与 K8s Service 名无耦合）',
  `instances` JSON NOT NULL COMMENT '实例列表（发现组件全量替换写入）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='K8s 实例池表';

-- create rate_limit_policies (限流策略表)
DROP TABLE IF EXISTS `rate_limit_policies`;
CREATE TABLE `rate_limit_policies` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `enabled` TINYINT(1) DEFAULT 0 COMMENT '是否启用：0-禁用，1-启用',
  `max_concurrency` INT DEFAULT -1 COMMENT '最大并发数（-1表示不限制）',
  `tpm_configs` TEXT COMMENT 'TPM限流配置（JSON数组）',
  `rpm_configs` TEXT COMMENT 'RPM限流配置（JSON数组）',
  `batch_limits` TEXT COMMENT '批量限流配置（JSON对象，2026-10-07新增）：max_create_rpm/max_active_batches/max_file_bytes/max_file_lines；省略/NULL=该策略不参与批量限流',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  INDEX `idx_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='限流策略表';

-- create ai_cache_rules (AI缓存规则表)
DROP TABLE IF EXISTS `ai_cache_rules`;
CREATE TABLE `ai_cache_rules` (
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

-- create ai_cache_semantic_settings (AI缓存语义全局设置表，单行)
DROP TABLE IF EXISTS `ai_cache_semantic_settings`;
CREATE TABLE `ai_cache_semantic_settings` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（恒为1语义，物理单行）',
  `top_k` INT NOT NULL DEFAULT 1 COMMENT '语义检索TopK（1-10）',
  `threshold` DOUBLE NOT NULL DEFAULT 0.15 COMMENT '相似度阈值（量纲与 threshold_relation 一致，0-2）',
  `threshold_relation` VARCHAR(8) NOT NULL DEFAULT 'lt' COMMENT '阈值比较: gt/gte/lt/lte',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI缓存语义全局设置表（单行）';

-- create ai_context_rules (AI上下文压缩规则表)
DROP TABLE IF EXISTS `ai_context_rules`;
CREATE TABLE `ai_context_rules` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID（数组顺序=id 升序=优先级）',
  `cond` VARCHAR(1024) NOT NULL COMMENT 'bfe 条件表达式',
  `mode` VARCHAR(16) NOT NULL COMMENT '压缩档位: off/conservative/balanced/aggressive',
  `max_context_tokens` INT NOT NULL DEFAULT 0 COMMENT '预算上限覆盖(token), 0=用模型表窗口',
  `reserve_tokens` INT NOT NULL DEFAULT 0 COMMENT '预留输出token, 0=自动clamp(窗口x15%,256,16000)',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI上下文压缩规则表';

-- create ai_context_settings (AI上下文压缩全局设置表，单行)
DROP TABLE IF EXISTS `ai_context_settings`;
CREATE TABLE `ai_context_settings` (
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

-- create traffic_mirror_rules (流量镜像规则表)
DROP TABLE IF EXISTS `traffic_mirror_rules`;
CREATE TABLE `traffic_mirror_rules` (
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

-- create intent_config (AI意图配置表)
DROP TABLE IF EXISTS `intent_config`;
CREATE TABLE `intent_config` (
  `id` BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID（固定 id=1，单行覆盖式存储）',
  `version` VARCHAR(32) NOT NULL COMMENT '当前配置版本，时间戳格式 yyyyMMddHHmmss，每次 PUT 更新',
  `min_confidence` DECIMAL(4,3) NOT NULL DEFAULT 0.600 COMMENT '全局置信度门控阈值 [0,1]',
  `questions` TEXT NOT NULL COMMENT '问题集 JSON（可为 []，表示停用意图分类），结构同 BFE intent_questions.data 的 Questions[]',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI意图配置表（mod_ai_intent intent_questions.data 数据源：问题集+门控阈值，单行覆盖式存储）';

-- create route_rules (路由规则表)
DROP TABLE IF EXISTS `route_rules`;
CREATE TABLE `route_rules` (
  `id` BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '主键ID',
  `type` VARCHAR(32) NOT NULL DEFAULT 'api_key' COMMENT '规则类型：api_key/entity/global',
  `owner` VARCHAR(64) NOT NULL COMMENT '所有者标识：api_key_id/entity_id/global',
  `enabled` TINYINT(1) DEFAULT 0 COMMENT '是否启用：0-禁用，1-启用',
  `rules` TEXT COMMENT '路由规则列表（JSON数组）',
  `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  UNIQUE KEY `uk_type_owner` (`type`, `owner`),
  INDEX `idx_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='路由规则表';

-- create operation_logs
DROP TABLE IF EXISTS `operation_logs`;
CREATE TABLE `operation_logs` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `log_id` varchar(64) NOT NULL COMMENT '请求唯一标识，与 access log 中的 LogID 一致，用于关联与去重',
  `operator_type` tinyint(4) NOT NULL DEFAULT '0' COMMENT '操作者类型：0=user, 1=token',
  `operator_id` bigint(20) NOT NULL DEFAULT '0' COMMENT '操作者在对应表中的主键 ID',
  `operator_name` varchar(255) NOT NULL DEFAULT '' COMMENT '操作者名称（user_name 或 token_name）',
  `action` varchar(32) NOT NULL COMMENT '操作动作：create/update/delete/reset/...',
  `resource_type` varchar(64) NOT NULL COMMENT '资源类型：entity/api_key/provider/...',
  `resource_id` varchar(255) NOT NULL DEFAULT '' COMMENT '被操作资源业务 ID',
  `resource_name` varchar(512) NOT NULL DEFAULT '' COMMENT '被操作资源名称，便于展示',
  `resource_parent_id` varchar(255) NOT NULL DEFAULT '' COMMENT '资源父级业务 ID（如 entity 层级中的父节点）',
  `status` tinyint(4) NOT NULL DEFAULT '1' COMMENT '操作结果：1=success, 2=failed',
  `error_msg` varchar(1024) NOT NULL DEFAULT '' COMMENT '失败时的简要错误信息',
  `change_summary` mediumtext COMMENT '变更摘要 JSON，记录变更前后关键字段（脱敏后）',
  `request_path` varchar(512) NOT NULL DEFAULT '' COMMENT '请求路径',
  `request_method` varchar(16) NOT NULL DEFAULT '' COMMENT '请求方法',
  `client_ip` varchar(64) NOT NULL DEFAULT '' COMMENT '客户端 IP',
  `user_agent` varchar(512) NOT NULL DEFAULT '' COMMENT 'User-Agent',
  `created_at` datetime NOT NULL COMMENT '操作发生时间',
  PRIMARY KEY (`id`),
  KEY `idx_operator` (`operator_type`, `operator_id`),
  KEY `idx_resource` (`resource_type`, `resource_id`),
  KEY `idx_action` (`action`),
  KEY `idx_created_at` (`created_at`),
  KEY `idx_log_id` (`log_id`),
  KEY `idx_resource_parent` (`resource_parent_id`),
  KEY `idx_operator_name_created` (`operator_name`, `created_at`),
  KEY `idx_status_created` (`status`, `created_at`),
  KEY `idx_resource_name_created` (`resource_name`, `created_at`),
  KEY `idx_resource_id_created` (`resource_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI 网关配置操作日志表';

-- insert default user
insert into users (id, name, password, scopes, created_at) values(1, 'admin', '$2a$10$w2oNyh4MO7SB.NHLPSq6kOj1GMiX1fApPYcWJmL8toZXGCQs3AJ0K', 'System', now());

insert into products (id, name, `description`,                              mail_list,       contact_person, created_at) values
                     (1, 'BFE', 'Build-in Product, User by System Manager', 'bfe@cncf.com', 'bfe',          now());
                     
-- insert default AI product name
insert into products (name, description,mail_list,contact_person, created_at) values ('AI_product', 'ai 产品线', '', '',now());

-- pools 表初始化数据
INSERT INTO `pools` (
    `id`,
    `name`,
    `product_id`,
    `ready`,
    `instance_detail`,
    `type`,
    `tag`,
    `role`,
    `created_at`,
    `updated_at`
) VALUES (
    1,
    'BFE.aipool',
    1,
    1,
    '[{"Name":"127.0.0.1","Addr":"127.0.0.1","Port":8080,"Ports":{"Default":8080},"tags":{"key":"value"},"Weight":1,"Disable":false}]',
    1,
    1,
    'COMMON',
    NOW(),
    NOW()
);

-- bfe_clusters 表初始化数据
INSERT INTO `bfe_clusters` (
    `id`,
    `name`,
    `pool_name`,
    `capacity`,
    `enabled`,
    `gtc_enabled`,
    `gtc_manual_enabled`,
    `exempt_traffic_check`,
    `created_at`,
    `updated_at`
) VALUES (
    1,
    'BFE-AI_product.szyf',
    'BFE.aipool',
    0,
    1,
    1,
    1,
    0,
    NOW(),
    NOW()
);

-- 初始化默认 global 路由表
INSERT IGNORE INTO `route_rules` (`type`, `owner`, `enabled`, `rules`) VALUES ('global', 'global', 0, '[]');

-- key rotation sweep tasks (see design-docs/modifications/2026-10-05-db-encryption-at-rest)
DROP TABLE IF EXISTS `keyrotate_sweep_tasks`;
CREATE TABLE `keyrotate_sweep_tasks` (
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

DROP TABLE IF EXISTS `keyrotate_sweep_lock`;
CREATE TABLE `keyrotate_sweep_lock` (
  `id` bigint(20) NOT NULL,
  `holder_task_id` varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 comment = "密钥收敛任务单行锁（id 恒为 1）";
INSERT INTO `keyrotate_sweep_lock` (`id`) VALUES (1);

-- create batch_files (批量文件元数据表，2026-10-07新增，批量与异步任务支持一期)
-- 同步源为 Redis BATCH_FILE:<provider>:<file_id>（TTL 48h），由对账 job 幂等 upsert 落库；
-- 表内不存文件内容，内容只在数据面转发链路流式通过。
DROP TABLE IF EXISTS `batch_files`;
CREATE TABLE `batch_files` (
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

-- create batch_tasks (批量任务表，2026-10-07新增，批量与异步任务支持一期)
-- 承载 provider 原生 Batch API 任务（OpenAI /v1/batches）在控制面的权威实体：
-- 状态机、用量、预留/结算/释放三态对账主字段。同步源为 Redis BATCH_TASK:<batch_id>（TTL 48h）。
DROP TABLE IF EXISTS `batch_tasks`;
CREATE TABLE `batch_tasks` (
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
