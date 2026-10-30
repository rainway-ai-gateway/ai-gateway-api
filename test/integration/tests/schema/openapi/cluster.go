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

package openapi

import "github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"

// InstanceSchema 集群实例 schema
var InstanceSchema = &testutil.ObjectSchema{
	Required: []string{"name", "addr", "weight", "port"},
	Fields: map[string]testutil.FieldSpec{
		"name":   {Type: testutil.TypeString},
		"addr":   {Type: testutil.TypeString},
		"weight": {Type: testutil.TypeInt},
		"port":   {Type: testutil.TypeInt},
	},
}

// BasicConnectionSchema basic.connection schema
var BasicConnectionSchema = &testutil.ObjectSchema{
	Required: []string{"max_idle_conn_per_rs", "cancel_on_client_close"},
	Fields: map[string]testutil.FieldSpec{
		"max_idle_conn_per_rs":   {Type: testutil.TypeInt},
		"cancel_on_client_close": {Type: testutil.TypeBool},
	},
}

// BasicRetriesSchema basic.retries schema
var BasicRetriesSchema = &testutil.ObjectSchema{
	Required: []string{"max_retry_in_cluster"},
	Fields: map[string]testutil.FieldSpec{
		"max_retry_in_cluster": {Type: testutil.TypeInt},
	},
}

// BasicBuffersSchema basic.buffers schema
var BasicBuffersSchema = &testutil.ObjectSchema{
	Required: []string{"req_write_buffer_size"},
	Fields: map[string]testutil.FieldSpec{
		"req_write_buffer_size": {Type: testutil.TypeInt},
	},
}

// BasicTimeoutsSchema basic.timeouts schema
var BasicTimeoutsSchema = &testutil.ObjectSchema{
	Required: []string{
		"timeout_conn_serv", "timeout_response_header",
		"timeout_readbody_client", "timeout_read_client_again", "timeout_write_client",
	},
	Fields: map[string]testutil.FieldSpec{
		"timeout_conn_serv":       {Type: testutil.TypeInt},
		"timeout_response_header": {Type: testutil.TypeInt},
		"timeout_readbody_client": {Type: testutil.TypeInt},
		"timeout_read_client_again": {Type: testutil.TypeInt},
		"timeout_write_client":    {Type: testutil.TypeInt},
	},
}

// BasicSchema 集群 basic 配置 schema
var BasicSchema = &testutil.ObjectSchema{
	Required: []string{"protocol", "connection", "retries", "buffers", "timeouts"},
	Fields: map[string]testutil.FieldSpec{
		"protocol":   {Type: testutil.TypeString},
		"connection": {Type: testutil.TypeObject, Nested: BasicConnectionSchema},
		"retries":    {Type: testutil.TypeObject, Nested: BasicRetriesSchema},
		"buffers":    {Type: testutil.TypeObject, Nested: BasicBuffersSchema},
		"timeouts":   {Type: testutil.TypeObject, Nested: BasicTimeoutsSchema},
	},
}

// StickySessionsSchema 会话保持 schema
var StickySessionsSchema = &testutil.ObjectSchema{
	Required: []string{"enabled", "hash_strategy", "hash_header"},
	Fields: map[string]testutil.FieldSpec{
		"enabled":       {Type: testutil.TypeBool},
		"hash_strategy": {Type: testutil.TypeString},
		"hash_header":   {Type: testutil.TypeString},
	},
}

// PassiveHealthCheckSchema 被动健康检查 schema
var PassiveHealthCheckSchema = &testutil.ObjectSchema{
	Required: []string{"interval", "failnum", "host", "uri", "statuscode"},
	Fields: map[string]testutil.FieldSpec{
		"interval":   {Type: testutil.TypeInt},
		"failnum":    {Type: testutil.TypeInt},
		"host":       {Type: testutil.TypeString},
		"uri":        {Type: testutil.TypeString},
		"statuscode": {Type: testutil.TypeInt},
	},
}

// ModelMappingSchema 模型映射 schema
var ModelMappingSchema = &testutil.ObjectSchema{
	Required: []string{"source_model", "target_model"},
	Fields: map[string]testutil.FieldSpec{
		"source_model": {Type: testutil.TypeString},
		"target_model": {Type: testutil.TypeString},
	},
}

// ClusterKeySchema llm_config.keys 元素 schema
var ClusterKeySchema = &testutil.ObjectSchema{
	Required: []string{"name", "weight"},
	Fields: map[string]testutil.FieldSpec{
		"name":   {Type: testutil.TypeString},
		"weight": {Type: testutil.TypeInt},
	},
}

// KeyPolicySchema llm_config.key_policy schema
var KeyPolicySchema = &testutil.ObjectSchema{
	Required: []string{"strategy", "max_retries", "retry_backoff_initial", "retry_backoff_max"},
	Fields: map[string]testutil.FieldSpec{
		"strategy":              {Type: testutil.TypeString},
		"max_retries":           {Type: testutil.TypeInt},
		"retry_backoff_initial": {Type: testutil.TypeInt},
		"retry_backoff_max":     {Type: testutil.TypeInt},
	},
}

// KeyAffinitySchema llm_config.key_affinity schema
var KeyAffinitySchema = &testutil.ObjectSchema{
	Required: []string{"enabled", "ttl", "redis_prefix", "penalty_enable"},
	Fields: map[string]testutil.FieldSpec{
		"enabled":        {Type: testutil.TypeBool},
		"ttl":            {Type: testutil.TypeInt},
		"redis_prefix":   {Type: testutil.TypeString},
		"penalty_enable": {Type: testutil.TypeBool},
	},
}

// NormalizeUpstreamErrorSchema llm_config.normalize_upstream_error schema
var NormalizeUpstreamErrorSchema = &testutil.ObjectSchema{
	Optional: []string{"enabled", "stream_enabled", "unrecognized_action", "max_body_bytes", "redact_secrets"},
	Fields: map[string]testutil.FieldSpec{
		"enabled":              {Type: testutil.TypeBool},
		"stream_enabled":       {Type: testutil.TypeBool},
		"unrecognized_action":  {Type: testutil.TypeString},
		"max_body_bytes":       {Type: testutil.TypeInt},
		"redact_secrets":       {Type: testutil.TypeBool},
	},
}

// LLMConfigSchema LLM 配置 schema
// model_endpoint、model_mappings、keys、key_policy、key_affinity、normalize_upstream_error、match_prefix、strip_prefix
// 在未配置时为 null，因此设为可选。
var LLMConfigSchema = &testutil.ObjectSchema{
	Required: []string{"models", "provider"},
	Optional: []string{
		"model_mappings", "keys",
		"key_policy", "key_affinity", "normalize_upstream_error", "price_miss_action", "match_prefix", "strip_prefix",
	},
	Fields: map[string]testutil.FieldSpec{
		"models":         {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"model_mappings": {Type: testutil.TypeArray, Elem: ModelMappingSchema},
		"keys":           {Type: testutil.TypeArray, Elem: ClusterKeySchema},
		"key_policy":     {Type: testutil.TypeObject, Nested: KeyPolicySchema},
		"key_affinity":   {Type: testutil.TypeObject, Nested: KeyAffinitySchema},
		"normalize_upstream_error": {Type: testutil.TypeObject, Nested: NormalizeUpstreamErrorSchema},
		"price_miss_action":        {Type: testutil.TypeString},
		"provider":       {Type: testutil.TypeString},
		"match_prefix":   {Type: testutil.TypeString},
		"strip_prefix":   {Type: testutil.TypeBool},
	},
}

// EppConfigFlowControlSchema epp_config.flow_control schema
// （clusters.md 表：epp_config.flow_control）
var EppConfigFlowControlSchema = &testutil.ObjectSchema{
	Optional: []string{"max_requests", "queue_ttl", "no_endpoint_queue_ttl", "enable_eviction"},
	Fields: map[string]testutil.FieldSpec{
		"max_requests":          {Type: testutil.TypeInt},
		"queue_ttl":             {Type: testutil.TypeInt},
		"no_endpoint_queue_ttl": {Type: testutil.TypeInt},
		"enable_eviction":       {Type: testutil.TypeBool},
	},
}

// EppConfigSchema 集群 epp_config（简化用户形态）schema。
// 存储保留用户原始 JSON——未显式携带的字段不落盘，因此全部可选；
// balance_mode=WRR 时 epp_config 可为 null（休眠保留），故 ClusterSchema 中设为 Optional。
var EppConfigSchema = &testutil.ObjectSchema{
	Optional: []string{
		"load_profile", "affinity",
		"prefix_cache_affinity", "session_affinity_enabled", "session_affinity_header",
		"kv_cache_utilization_max", "waiting_queue_max", "running_requests_max",
		"fallback_on_empty", "metrics_staleness_threshold_ms", "flow_control",
	},
	Fields: map[string]testutil.FieldSpec{
		"load_profile":                   {Type: testutil.TypeString, Enum: []interface{}{"queue-first", "balanced", "kv-first"}},
		"affinity":                       {Type: testutil.TypeString, Enum: []interface{}{"off", "low", "medium", "high"}},
		"prefix_cache_affinity":          {Type: testutil.TypeBool},
		"session_affinity_enabled":       {Type: testutil.TypeBool},
		"session_affinity_header":        {Type: testutil.TypeString},
		"kv_cache_utilization_max":       {Type: testutil.TypeNumber},
		"waiting_queue_max":              {Type: testutil.TypeNumber},
		"running_requests_max":           {Type: testutil.TypeNumber},
		"fallback_on_empty":              {Type: testutil.TypeBool},
		"metrics_staleness_threshold_ms": {Type: testutil.TypeNumber},
		"flow_control":                   {Type: testutil.TypeObject, Nested: EppConfigFlowControlSchema},
	},
}

// EppRuntimeCheckSchema epp_runtime.check schema（clusters.md 表：EPP运行时配置）
var EppRuntimeCheckSchema = &testutil.ObjectSchema{
	Optional: []string{"disabled", "check_interval", "fail_threshold", "cooldown", "success_threshold"},
	Fields: map[string]testutil.FieldSpec{
		"disabled":          {Type: testutil.TypeBool},
		"check_interval":    {Type: testutil.TypeString},
		"fail_threshold":    {Type: testutil.TypeInt},
		"cooldown":          {Type: testutil.TypeString},
		"success_threshold": {Type: testutil.TypeInt},
	},
}

// EppRuntimeTimeoutSchema epp_runtime.timeout schema（clusters.md 表：EPP运行时配置）
var EppRuntimeTimeoutSchema = &testutil.ObjectSchema{
	Optional: []string{"connect", "call"},
	Fields: map[string]testutil.FieldSpec{
		"connect": {Type: testutil.TypeString},
		"call":    {Type: testutil.TypeString},
	},
}

// EppRuntimeSchema 集群 epp_runtime（BFE 侧 EPP 运行时参数）schema。
// 存储保留用户原始 JSON（未显式携带的字段不落盘），且 balance_mode=WRR 时可为
// null（休眠保留），故全部可选。
var EppRuntimeSchema = &testutil.ObjectSchema{
	Optional: []string{"check", "timeout"},
	Fields: map[string]testutil.FieldSpec{
		"check":   {Type: testutil.TypeObject, Nested: EppRuntimeCheckSchema},
		"timeout": {Type: testutil.TypeObject, Nested: EppRuntimeTimeoutSchema},
	},
}

// ClusterSchema Cluster 数据模型 schema
// 通过 /clusters 接口创建的集群 llm_config 必填。
// balance_mode 必返回（缺省 WRR）；epp_config 未配置时为 null（Optional 允许 null）。
var ClusterSchema = &testutil.ObjectSchema{
	Required: []string{
		"name", "description", "llm_config",
		"basic", "sticky_sessions", "passive_health_check",
		"balance_mode",
	},
	Optional: []string{"epp_config", "epp_runtime"},
	Fields: map[string]testutil.FieldSpec{
		"name":                 {Type: testutil.TypeString},
		"description":          {Type: testutil.TypeString},
		"basic":                {Type: testutil.TypeObject, Nested: BasicSchema},
		"sticky_sessions":      {Type: testutil.TypeObject, Nested: StickySessionsSchema},
		"passive_health_check": {Type: testutil.TypeObject, Nested: PassiveHealthCheckSchema},
		"llm_config":           {Type: testutil.TypeObject, Nested: LLMConfigSchema},
		"balance_mode":         {Type: testutil.TypeString, Enum: []interface{}{"WRR", "EPP"}},
		"epp_config":           {Type: testutil.TypeObject, Nested: EppConfigSchema},
		"epp_runtime":          {Type: testutil.TypeObject, Nested: EppRuntimeSchema},
	},
}
