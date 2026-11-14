-- SQLite DDL for AI Gateway

-- create bfe_clusters
DROP TABLE IF EXISTS bfe_clusters;
CREATE TABLE bfe_clusters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  pool_name TEXT NOT NULL DEFAULT '',
  capacity INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  gtc_enabled INTEGER NOT NULL DEFAULT 1,
  gtc_manual_enabled INTEGER NOT NULL DEFAULT 1,
  exempt_traffic_check INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE TRIGGER bfe_clusters_updated_at AFTER UPDATE ON bfe_clusters
  FOR EACH ROW BEGIN UPDATE bfe_clusters SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create products
DROP TABLE IF EXISTS products;
CREATE TABLE products (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  mail_list TEXT NOT NULL,
  contact_person TEXT NOT NULL,
  sms_list TEXT NOT NULL DEFAULT 'no sms',
  description TEXT NOT NULL DEFAULT 'no desc',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE TRIGGER products_updated_at AFTER UPDATE ON products
  FOR EACH ROW BEGIN UPDATE products SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create domains
DROP TABLE IF EXISTS domains;
CREATE TABLE domains (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  product_id INTEGER NOT NULL,
  type INTEGER NOT NULL,
  using_advanced_redirect INTEGER NOT NULL DEFAULT 0,
  using_advanced_hsts INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE INDEX domains_product_id ON domains (product_id);
CREATE INDEX domains_type ON domains (type);
CREATE TRIGGER domains_updated_at AFTER UPDATE ON domains
  FOR EACH ROW BEGIN UPDATE domains SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create clusters
DROP TABLE IF EXISTS clusters;
CREATE TABLE clusters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT 'no desc',
  product_id INTEGER NOT NULL,
  protocol TEXT NOT NULL DEFAULT 'http',
  max_idle_conn_per_host INTEGER NOT NULL DEFAULT 2,
  timeout_conn_serv INTEGER NOT NULL DEFAULT 50000,
  timeout_response_header INTEGER NOT NULL DEFAULT 50000,
  timeout_readbody_client INTEGER NOT NULL DEFAULT 30000,
  timeout_read_client_again INTEGER NOT NULL DEFAULT 30000,
  timeout_write_client INTEGER NOT NULL DEFAULT 60000,
  healthcheck_schem TEXT NOT NULL DEFAULT 'http',
  healthcheck_interval INTEGER NOT NULL DEFAULT 1000,
  healthcheck_failnum INTEGER NOT NULL DEFAULT 10,
  healthcheck_host TEXT NOT NULL,
  healthcheck_uri TEXT NOT NULL,
  healthcheck_statuscode INTEGER NOT NULL DEFAULT 200,
  clientip_carry INTEGER NOT NULL DEFAULT 0,
  port_carry INTEGER NOT NULL DEFAULT 0,
  max_retry_in_cluster INTEGER NOT NULL DEFAULT 3,
  max_retry_cross_cluster INTEGER NOT NULL DEFAULT 0,
  ready INTEGER NOT NULL DEFAULT 1,
  hash_strategy INTEGER NOT NULL DEFAULT 0,
  cookie_key TEXT NOT NULL DEFAULT 'BAIDUID',
  hash_header TEXT NOT NULL DEFAULT 'Cookie:BAIDUID',
  session_sticky INTEGER NOT NULL DEFAULT 0,
  req_write_buffer_size INTEGER NOT NULL DEFAULT 512,
  req_flush_interval INTEGER NOT NULL DEFAULT 0,
  res_flush_interval INTEGER NOT NULL DEFAULT 20,
  cancel_on_client_close INTEGER NOT NULL DEFAULT 0,
  failure_status INTEGER NOT NULL DEFAULT 0,
  max_conns_per_host INTEGER NOT NULL DEFAULT 0,
  llm_config TEXT,
  balance_mode TEXT NOT NULL DEFAULT 'WRR',
  epp_config TEXT,
  epp_runtime TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE TRIGGER clusters_updated_at AFTER UPDATE ON clusters
  FOR EACH ROW BEGIN UPDATE clusters SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create epp_instances
DROP TABLE IF EXISTS epp_instances;
CREATE TABLE epp_instances (
  id TEXT NOT NULL PRIMARY KEY,
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  group_name TEXT NOT NULL,
  create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (host, port)
);
CREATE TRIGGER epp_instances_update_time AFTER UPDATE ON epp_instances
  FOR EACH ROW BEGIN UPDATE epp_instances SET update_time = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create epp_assignments
DROP TABLE IF EXISTS epp_assignments;
CREATE TABLE epp_assignments (
  cluster TEXT NOT NULL PRIMARY KEY,
  group_name TEXT NOT NULL,
  create_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  update_time DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER epp_assignments_update_time AFTER UPDATE ON epp_assignments
  FOR EACH ROW BEGIN UPDATE epp_assignments SET update_time = CURRENT_TIMESTAMP WHERE cluster = OLD.cluster; END;

-- create lb_matrices
DROP TABLE IF EXISTS lb_matrices;
CREATE TABLE lb_matrices (
  cluster_id INTEGER PRIMARY KEY AUTOINCREMENT,
  lb_matrix TEXT NOT NULL,
  product_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER lb_matrices_updated_at AFTER UPDATE ON lb_matrices
  FOR EACH ROW BEGIN UPDATE lb_matrices SET updated_at = CURRENT_TIMESTAMP WHERE cluster_id = OLD.cluster_id; END;

-- create sub_clusters
DROP TABLE IF EXISTS sub_clusters;
CREATE TABLE sub_clusters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  product_id INTEGER NOT NULL,
  description TEXT NOT NULL DEFAULT 'no desc',
  bns_name_id INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  role TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name, product_id)
);
CREATE INDEX sub_clusters_cluster_id ON sub_clusters (cluster_id);
CREATE TRIGGER sub_clusters_updated_at AFTER UPDATE ON sub_clusters
  FOR EACH ROW BEGIN UPDATE sub_clusters SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create pools
DROP TABLE IF EXISTS pools;
CREATE TABLE pools (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  product_id INTEGER NOT NULL DEFAULT 0,
  ready INTEGER NOT NULL DEFAULT 1,
  instance_detail TEXT,
  type INTEGER NOT NULL DEFAULT 1,
  tag INTEGER NOT NULL DEFAULT 0,
  role TEXT NOT NULL DEFAULT 'COMMON',
  epp_server TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE TRIGGER pools_updated_at AFTER UPDATE ON pools
  FOR EACH ROW BEGIN UPDATE pools SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create route_basic_rules
DROP TABLE IF EXISTS route_basic_rules;
CREATE TABLE route_basic_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  description TEXT NOT NULL DEFAULT '',
  product_id INTEGER NOT NULL,
  host_names TEXT NOT NULL,
  paths TEXT NOT NULL,
  cluster_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX route_basic_rules_product_id ON route_basic_rules (product_id);
CREATE TRIGGER route_basic_rules_updated_at AFTER UPDATE ON route_basic_rules
  FOR EACH ROW BEGIN UPDATE route_basic_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create route_advance_rules
DROP TABLE IF EXISTS route_advance_rules;
CREATE TABLE route_advance_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  product_id INTEGER NOT NULL,
  expression BLOB NOT NULL,
  cluster_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX route_advance_rules_product_id ON route_advance_rules (product_id);
CREATE TRIGGER route_advance_rules_updated_at AFTER UPDATE ON route_advance_rules
  FOR EACH ROW BEGIN UPDATE route_advance_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create route_cases
DROP TABLE IF EXISTS route_cases;
CREATE TABLE route_cases (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  description TEXT NOT NULL DEFAULT '',
  product_id INTEGER NOT NULL,
  url TEXT NOT NULL,
  method TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT '',
  header TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  expect_cluster TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX route_cases_product_id ON route_cases (product_id);
CREATE TRIGGER route_cases_updated_at AFTER UPDATE ON route_cases
  FOR EACH ROW BEGIN UPDATE route_cases SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create certificates
DROP TABLE IF EXISTS certificates;
CREATE TABLE certificates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  cert_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT 'no desc',
  is_default INTEGER NOT NULL DEFAULT 0,
  expired_date TEXT NOT NULL,
  cert_file_path TEXT NOT NULL,
  key_file_path TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (cert_name)
);
CREATE TRIGGER certificates_updated_at AFTER UPDATE ON certificates
  FOR EACH ROW BEGIN UPDATE certificates SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create extra_files
DROP TABLE IF EXISTS extra_files;
CREATE TABLE extra_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  product_id INTEGER NOT NULL DEFAULT 0,
  description TEXT NOT NULL DEFAULT '',
  md5 TEXT NOT NULL,
  content TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name, product_id)
);
CREATE INDEX extra_files_product_id ON extra_files (product_id);
CREATE TRIGGER extra_files_updated_at AFTER UPDATE ON extra_files
  FOR EACH ROW BEGIN UPDATE extra_files SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create config_versions
DROP TABLE IF EXISTS config_versions;
CREATE TABLE config_versions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  data_sign TEXT NOT NULL,
  version TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name, version)
);
CREATE TRIGGER config_versions_updated_at AFTER UPDATE ON config_versions
  FOR EACH ROW BEGIN UPDATE config_versions SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create users
DROP TABLE IF EXISTS users;
CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  type INTEGER NOT NULL DEFAULT 0,
  password TEXT NOT NULL DEFAULT '',
  ticket TEXT NOT NULL DEFAULT '',
  ticket_created_at DATETIME NOT NULL DEFAULT '0000-01-01 00:00:00',
  scopes TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name, type)
);
CREATE TRIGGER users_updated_at AFTER UPDATE ON users
  FOR EACH ROW BEGIN UPDATE users SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create user_products
DROP TABLE IF EXISTS user_products;
CREATE TABLE user_products (
  user_id INTEGER NOT NULL,
  product_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id, product_id)
);
CREATE TRIGGER user_products_updated_at AFTER UPDATE ON user_products
  FOR EACH ROW BEGIN UPDATE user_products SET updated_at = CURRENT_TIMESTAMP WHERE user_id = OLD.user_id AND product_id = OLD.product_id; END;

-- create api_keys
DROP TABLE IF EXISTS api_keys;
CREATE TABLE api_keys (
  inner_id INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL DEFAULT '',
  enable INTEGER NOT NULL DEFAULT 0,
  api_key TEXT NOT NULL DEFAULT '',
  api_key_hash TEXT NOT NULL DEFAULT '',
  description TEXT DEFAULT '',
  unlimited_quota INTEGER DEFAULT 0,
  product_name TEXT NOT NULL DEFAULT '',
  expired_time INTEGER NOT NULL DEFAULT -1,
  allowed_models TEXT,
  subnet TEXT,
  entity_id TEXT DEFAULT NULL,
  quota_plan_id INTEGER DEFAULT NULL,
  rate_limit_policy_id INTEGER DEFAULT NULL,
  route_rules_id INTEGER DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT '0000-01-01 00:00:00',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (id),
  UNIQUE (api_key_hash)
);
CREATE INDEX api_keys_product_name ON api_keys (product_name);
CREATE INDEX api_keys_entity_id ON api_keys (entity_id);
CREATE INDEX api_keys_quota_plan_id ON api_keys (quota_plan_id);
CREATE INDEX api_keys_rate_limit_policy_id ON api_keys (rate_limit_policy_id);
CREATE INDEX api_keys_route_rules_id ON api_keys (route_rules_id);
CREATE TRIGGER api_keys_updated_at AFTER UPDATE ON api_keys
  FOR EACH ROW BEGIN UPDATE api_keys SET updated_at = CURRENT_TIMESTAMP WHERE inner_id = OLD.inner_id; END;

-- create api_key_id_seq
DROP TABLE IF EXISTS api_key_id_seq;
CREATE TABLE api_key_id_seq (
  product_name TEXT NOT NULL PRIMARY KEY,
  next_seq INTEGER NOT NULL DEFAULT 1
);

-- create api_key_tokens
DROP TABLE IF EXISTS api_key_tokens;
CREATE TABLE api_key_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  api_key TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT '0000-01-01 00:00:00',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (api_key)
);
CREATE TRIGGER api_key_tokens_updated_at AFTER UPDATE ON api_key_tokens
  FOR EACH ROW BEGIN UPDATE api_key_tokens SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create ai_route_rules
DROP TABLE IF EXISTS ai_route_rules;
CREATE TABLE ai_route_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL DEFAULT '',
  basic TEXT NOT NULL,
  product_name TEXT NOT NULL DEFAULT '',
  idx INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT '0000-01-01 00:00:00',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE INDEX ai_route_rules_idx ON ai_route_rules (idx);
CREATE TRIGGER ai_route_rules_updated_at AFTER UPDATE ON ai_route_rules
  FOR EACH ROW BEGIN UPDATE ai_route_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create route_default_rules
DROP TABLE IF EXISTS route_default_rules;
CREATE TABLE route_default_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  cmd TEXT NOT NULL,
  params TEXT,
  product_id INTEGER NOT NULL,
  route_action TEXT,
  description TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT '0000-01-01 00:00:00',
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (product_id)
);
CREATE TRIGGER route_default_rules_updated_at AFTER UPDATE ON route_default_rules
  FOR EACH ROW BEGIN UPDATE route_default_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create entity_types
DROP TABLE IF EXISTS entity_types;
CREATE TABLE entity_types (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type_name TEXT NOT NULL,
  description TEXT DEFAULT '',
  level INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (type_name)
);
CREATE INDEX entity_types_level ON entity_types (level);
CREATE TRIGGER entity_types_updated_at AFTER UPDATE ON entity_types
  FOR EACH ROW BEGIN UPDATE entity_types SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create entities
DROP TABLE IF EXISTS entities;
CREATE TABLE entities (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_id TEXT NOT NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL,
  parent_id TEXT DEFAULT NULL,
  allow_models TEXT,
  block_models TEXT,
  quota_plan_id INTEGER DEFAULT NULL,
  rate_limit_policy_id INTEGER DEFAULT NULL,
  route_rules_id INTEGER DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (entity_id),
  UNIQUE (name)
);
CREATE INDEX entities_parent_id ON entities (parent_id);
CREATE INDEX entities_type ON entities (type);
CREATE INDEX entities_quota_plan_id ON entities (quota_plan_id);
CREATE INDEX entities_rate_limit_policy_id ON entities (rate_limit_policy_id);
CREATE INDEX entities_route_rules_id ON entities (route_rules_id);
CREATE TRIGGER entities_updated_at AFTER UPDATE ON entities
  FOR EACH ROW BEGIN UPDATE entities SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create entity_id_seq
DROP TABLE IF EXISTS entity_id_seq;
CREATE TABLE entity_id_seq (
  name TEXT NOT NULL PRIMARY KEY,
  next_seq INTEGER NOT NULL DEFAULT 1
);

-- create quota_plans
DROP TABLE IF EXISTS quota_plans;
CREATE TABLE quota_plans (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  unlimited INTEGER DEFAULT 1,
  pass_when_no_enough_quota INTEGER DEFAULT 0,
  quota REAL DEFAULT 0,
  unit TEXT DEFAULT 'total_token',
  reset_period TEXT DEFAULT 'never',
  last_reset_at DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX quota_plans_unlimited ON quota_plans (unlimited);
CREATE TRIGGER quota_plans_updated_at AFTER UPDATE ON quota_plans
  FOR EACH ROW BEGIN UPDATE quota_plans SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create model_prices
DROP TABLE IF EXISTS model_prices;
CREATE TABLE model_prices (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  base_model TEXT NOT NULL,
  mode TEXT NOT NULL,
  capabilities TEXT,
  supported_parameters TEXT,
  limits TEXT,
  prices TEXT NOT NULL,
  tier_prices TEXT,
  batch_discount REAL,
  price_currency TEXT DEFAULT 'RMB',
  metadata TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (provider, model, mode)
);
CREATE INDEX model_prices_provider ON model_prices (provider);
CREATE INDEX model_prices_mode ON model_prices (mode);
CREATE TRIGGER model_prices_updated_at AFTER UPDATE ON model_prices
  FOR EACH ROW BEGIN UPDATE model_prices SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create providers
DROP TABLE IF EXISTS providers;
CREATE TABLE providers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  model_endpoint TEXT,
  models TEXT,
  -- 列加密整列信封（enc$v1$ 前缀）：信封不是 JSON，MySQL 侧必须 MEDIUMTEXT（见 db_ddl.sql）；
  -- SQLite 不校验 JSON，TEXT 即可
  api_keys TEXT,
  instance_pool TEXT NOT NULL,
  instance_source TEXT NOT NULL DEFAULT 'instance_pool',
  k8s_pool_name TEXT,
  model_protocols TEXT NOT NULL,
  protocol_paths TEXT,
  time_zone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  tiers TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE INDEX providers_name ON providers (name);
CREATE TRIGGER providers_updated_at AFTER UPDATE ON providers
  FOR EACH ROW BEGIN UPDATE providers SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create k8s_pools
DROP TABLE IF EXISTS k8s_pools;
CREATE TABLE k8s_pools (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  instances TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE INDEX k8s_pools_name ON k8s_pools (name);
CREATE TRIGGER k8s_pools_updated_at AFTER UPDATE ON k8s_pools
  FOR EACH ROW BEGIN UPDATE k8s_pools SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create rate_limit_policies
DROP TABLE IF EXISTS rate_limit_policies;
CREATE TABLE rate_limit_policies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  enabled INTEGER DEFAULT 0,
  max_concurrency INTEGER DEFAULT -1,
  tpm_configs TEXT,
  rpm_configs TEXT,
  batch_limits TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX rate_limit_policies_enabled ON rate_limit_policies (enabled);
CREATE TRIGGER rate_limit_policies_updated_at AFTER UPDATE ON rate_limit_policies
  FOR EACH ROW BEGIN UPDATE rate_limit_policies SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create ai_cache_rules (AI缓存规则表)
DROP TABLE IF EXISTS ai_cache_rules;
CREATE TABLE ai_cache_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  cond TEXT NOT NULL,
  cache_key_strategy TEXT NOT NULL DEFAULT 'lastQuestion',
  cache_ttl INTEGER NOT NULL DEFAULT 0,
  max_body_bytes INTEGER NOT NULL DEFAULT 1048576,
  max_value_bytes INTEGER NOT NULL DEFAULT 1048576,
  enable_semantic_cache INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE TRIGGER ai_cache_rules_updated_at AFTER UPDATE ON ai_cache_rules
  FOR EACH ROW BEGIN UPDATE ai_cache_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create ai_cache_semantic_settings (AI缓存语义全局设置表，单行)
DROP TABLE IF EXISTS ai_cache_semantic_settings;
CREATE TABLE ai_cache_semantic_settings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  top_k INTEGER NOT NULL DEFAULT 1,
  threshold REAL NOT NULL DEFAULT 0.15,
  threshold_relation TEXT NOT NULL DEFAULT 'lt',
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER ai_cache_semantic_settings_updated_at AFTER UPDATE ON ai_cache_semantic_settings
  FOR EACH ROW BEGIN UPDATE ai_cache_semantic_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create ai_context_rules (AI上下文压缩规则表)
DROP TABLE IF EXISTS ai_context_rules;
CREATE TABLE ai_context_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  cond TEXT NOT NULL,
  mode TEXT NOT NULL,
  max_context_tokens INTEGER NOT NULL DEFAULT 0,
  reserve_tokens INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER ai_context_rules_updated_at AFTER UPDATE ON ai_context_rules
  FOR EACH ROW BEGIN UPDATE ai_context_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create ai_context_settings (AI上下文压缩全局设置表，单行)
DROP TABLE IF EXISTS ai_context_settings;
CREATE TABLE ai_context_settings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trigger_ratio REAL NOT NULL DEFAULT 0.7,
  keep_latest_images INTEGER NOT NULL DEFAULT 2,
  tool_result_max_chars INTEGER NOT NULL DEFAULT 2000,
  thinking_policy TEXT NOT NULL DEFAULT 'trim-all-but-last',
  chars_per_token INTEGER NOT NULL DEFAULT 4,
  image_token_estimate INTEGER NOT NULL DEFAULT 1200,
  rewrite_strength TEXT NOT NULL DEFAULT 'lite',
  rewrite_protected_survival_rate REAL NOT NULL DEFAULT 0.95,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER ai_context_settings_updated_at AFTER UPDATE ON ai_context_settings
  FOR EACH ROW BEGIN UPDATE ai_context_settings SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create traffic_mirror_rules (流量镜像规则表)
DROP TABLE IF EXISTS traffic_mirror_rules;
CREATE TABLE traffic_mirror_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  cond TEXT,
  mirror_cluster TEXT NOT NULL,
  percentage INTEGER NOT NULL DEFAULT 100,
  remove_headers TEXT,
  set_headers TEXT,
  body_rewrites TEXT,
  path_rewrite TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);
CREATE TRIGGER traffic_mirror_rules_updated_at AFTER UPDATE ON traffic_mirror_rules
  FOR EACH ROW BEGIN UPDATE traffic_mirror_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create intent_config (AI意图配置表)
DROP TABLE IF EXISTS intent_config;
CREATE TABLE intent_config (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  version TEXT NOT NULL,
  min_confidence REAL NOT NULL DEFAULT 0.6,
  questions TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TRIGGER intent_config_updated_at AFTER UPDATE ON intent_config
  FOR EACH ROW BEGIN UPDATE intent_config SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create route_rules (路由规则表)
DROP TABLE IF EXISTS route_rules;
CREATE TABLE route_rules (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL DEFAULT 'api_key',
  owner TEXT NOT NULL,
  enabled INTEGER DEFAULT 0,
  rules TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (type, owner)
);
CREATE INDEX route_rules_enabled ON route_rules (enabled);
CREATE TRIGGER route_rules_updated_at AFTER UPDATE ON route_rules
  FOR EACH ROW BEGIN UPDATE route_rules SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create operation_logs (操作日志表)
DROP TABLE IF EXISTS operation_logs;
CREATE TABLE operation_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  log_id TEXT NOT NULL,
  operator_type INTEGER NOT NULL DEFAULT 0,
  operator_id INTEGER NOT NULL DEFAULT 0,
  operator_name TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT NOT NULL DEFAULT '',
  resource_name TEXT NOT NULL DEFAULT '',
  resource_parent_id TEXT NOT NULL DEFAULT '',
  status INTEGER NOT NULL DEFAULT 1,
  error_msg TEXT NOT NULL DEFAULT '',
  change_summary TEXT,
  request_path TEXT NOT NULL DEFAULT '',
  request_method TEXT NOT NULL DEFAULT '',
  client_ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL
);
CREATE INDEX operation_logs_operator ON operation_logs (operator_type, operator_id);
CREATE INDEX operation_logs_resource ON operation_logs (resource_type, resource_id);
CREATE INDEX operation_logs_action ON operation_logs (action);
CREATE INDEX operation_logs_created_at ON operation_logs (created_at);
CREATE INDEX operation_logs_log_id ON operation_logs (log_id);
CREATE INDEX operation_logs_resource_parent ON operation_logs (resource_parent_id);

-- insert default user
INSERT INTO users (id, name, password, scopes, created_at) VALUES (1, 'admin', '$2a$10$w2oNyh4MO7SB.NHLPSq6kOj1GMiX1fApPYcWJmL8toZXGCQs3AJ0K', 'System', CURRENT_TIMESTAMP);

INSERT INTO products (id, name, description, mail_list, contact_person, created_at) VALUES
  (1, 'BFE', 'Build-in Product, User by System Manager', 'bfe@cncf.com', 'bfe', CURRENT_TIMESTAMP);

INSERT INTO products (name, description, mail_list, contact_person, created_at) VALUES ('AI_product', 'ai 产品线', '', '', CURRENT_TIMESTAMP);

-- pools 表初始化数据
INSERT INTO pools (id, name, product_id, ready, instance_detail, type, tag, role, created_at, updated_at) VALUES (
    1,
    'BFE.aipool',
    1,
    1,
    '[{"Name":"127.0.0.1","Addr":"127.0.0.1","Port":8080,"Ports":{"Default":8080},"tags":{"key":"value"},"Weight":1,"Disable":false}]',
    1,
    1,
    'COMMON',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

-- bfe_clusters 表初始化数据
INSERT INTO bfe_clusters (id, name, pool_name, capacity, enabled, gtc_enabled, gtc_manual_enabled, exempt_traffic_check, created_at, updated_at) VALUES (
    1,
    'BFE-AI_product.szyf',
    'BFE.aipool',
    0,
    1,
    1,
    1,
    0,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
);

-- 初始化默认 global 路由表
INSERT OR IGNORE INTO route_rules (type, owner, enabled, rules) VALUES ('global', 'global', 0, '[]');

-- key rotation sweep tasks (see design-docs/modifications/2026-10-05-db-encryption-at-rest)
CREATE TABLE IF NOT EXISTS keyrotate_sweep_tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL,
  mode TEXT NOT NULL,
  dry_run INTEGER NOT NULL DEFAULT 0,
  scope TEXT NOT NULL DEFAULT 'all',
  active_key_id INTEGER NOT NULL DEFAULT 0,
  scanned INTEGER NOT NULL DEFAULT 0,
  rewritten INTEGER NOT NULL DEFAULT 0,
  summary TEXT,
  error TEXT NOT NULL DEFAULT '',
  heartbeat_at DATETIME NOT NULL,
  started_at DATETIME NOT NULL,
  finished_at DATETIME,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS keyrotate_sweep_lock (
  id INTEGER PRIMARY KEY,
  holder_task_id TEXT NOT NULL DEFAULT ''
);
INSERT OR IGNORE INTO keyrotate_sweep_lock (id) VALUES (1);

-- create batch_files (批量文件元数据表，2026-10-07新增，批量与异步任务支持一期)
-- 同步源为 Redis BATCH_FILE:<provider>:<file_id>（TTL 48h），由对账 job 幂等 upsert 落库；
-- 表内不存文件内容，内容只在数据面转发链路流式通过。
DROP TABLE IF EXISTS batch_files;
CREATE TABLE batch_files (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id TEXT NOT NULL,
  provider TEXT NOT NULL,
  key_name TEXT,
  api_key_id TEXT NOT NULL,
  product_name TEXT NOT NULL,
  direction TEXT NOT NULL,
  purpose TEXT,
  filename TEXT,
  line_count INTEGER,
  bytes INTEGER,
  first_seen_at DATETIME,
  last_seen_at DATETIME,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (file_id, provider)
);
CREATE INDEX batch_files_apikey ON batch_files (api_key_id);
CREATE TRIGGER batch_files_updated_at AFTER UPDATE ON batch_files
  FOR EACH ROW BEGIN UPDATE batch_files SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;

-- create batch_tasks (批量任务表，2026-10-07新增，批量与异步任务支持一期)
-- 承载 provider 原生 Batch API 任务（OpenAI /v1/batches）在控制面的权威实体：
-- 状态机、用量、预留/结算/释放三态对账主字段。同步源为 Redis BATCH_TASK:<batch_id>（TTL 48h）。
DROP TABLE IF EXISTS batch_tasks;
CREATE TABLE batch_tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  batch_id TEXT NOT NULL,
  api_key_id TEXT NOT NULL,
  product_name TEXT NOT NULL,
  entity_id TEXT,
  provider TEXT NOT NULL,
  key_name TEXT,
  endpoint TEXT,
  input_file_id TEXT,
  output_file_id TEXT,
  status TEXT NOT NULL,
  request_counts TEXT,
  est_lines INTEGER,
  usage_input_tokens INTEGER,
  usage_output_tokens INTEGER,
  usage_source TEXT,
  reserve_units INTEGER,
  settle_units INTEGER,
  settle_status TEXT NOT NULL,
  over_reserved INTEGER DEFAULT 0,
  sync_source TEXT DEFAULT 'redis',
  idem_key TEXT NOT NULL UNIQUE,
  created_at DATETIME,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  terminal_at DATETIME
);
CREATE INDEX batch_tasks_status ON batch_tasks (status);
CREATE INDEX batch_tasks_apikey ON batch_tasks (api_key_id);
CREATE INDEX batch_tasks_terminal ON batch_tasks (terminal_at);
CREATE TRIGGER batch_tasks_updated_at AFTER UPDATE ON batch_tasks
  FOR EACH ROW BEGIN UPDATE batch_tasks SET updated_at = CURRENT_TIMESTAMP WHERE id = OLD.id; END;
