package validate

import (
	"strings"
	"testing"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
	"github.com/stretchr/testify/assert"
)

func TestHostname(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", "backend-1.example.com", false},
		{"valid ip", "192.0.2.1", false},
		{"valid ipv6", "2001:0db8::1", false},
		{"too short", "a", true},
		{"empty", "", true},
		{"too long", string(make([]byte, 256)), true},
		{"label starts with hyphen", "-host.example.com", true},
		{"label ends with hyphen", "host-.example.com", true},
		{"valid label with hyphen", "host-1.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Hostname(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIPAddress(t *testing.T) {
	assert.NoError(t, IPAddress("192.0.2.1"))
	assert.NoError(t, IPAddress("::1"))
	assert.Error(t, IPAddress("not-an-ip"))
}

func TestPort(t *testing.T) {
	assert.NoError(t, Port(1))
	assert.NoError(t, Port(65535))
	assert.Error(t, Port(0))
	assert.Error(t, Port(65536))
}

func TestCIDR(t *testing.T) {
	assert.NoError(t, CIDR("*"))
	assert.NoError(t, CIDR("192.0.2.0/24"))
	assert.NoError(t, CIDR("2001:0db8::/32"))
	assert.Error(t, CIDR("invalid"))
}

func TestUserName(t *testing.T) {
	assert.NoError(t, UserName("user_1"))
	assert.Error(t, UserName("admin"))
	assert.Error(t, UserName("-user"))
	assert.Error(t, UserName("user."))
	assert.Error(t, UserName("user name"))
	assert.Error(t, UserName(""))
}

func TestPassword(t *testing.T) {
	assert.NoError(t, Password("password123", "user1"))
	assert.Error(t, Password("short1", "user1"))
	assert.Error(t, Password("user1", "user1"))
	assert.Error(t, Password("1resu", "user1"))
	assert.Error(t, Password("pass word", "user1"))
}

func TestTokenName(t *testing.T) {
	assert.NoError(t, TokenName("token_1"))
	assert.Error(t, TokenName("default"))
	assert.Error(t, TokenName("-token"))
}

func TestClusterName(t *testing.T) {
	assert.NoError(t, ClusterName("cluster_1"))
	assert.Error(t, ClusterName("-cluster"))
	assert.Error(t, ClusterName("cluster."))
}

func TestCertName(t *testing.T) {
	assert.NoError(t, CertName("demo-cert"))
	assert.NoError(t, CertName("tc009.qa-20260904"))
	assert.NoError(t, CertName("my_cert_01"))
	assert.NoError(t, CertName("ab"))
	assert.NoError(t, CertName(strings.Repeat("a", 64)))

	assert.Error(t, CertName(""))
	assert.Error(t, CertName("a"))
	assert.Error(t, CertName(strings.Repeat("a", 65)))
	assert.Error(t, CertName("demo/child"))
	assert.Error(t, CertName("demo?x=1"))
	assert.Error(t, CertName("demo#1"))
	assert.Error(t, CertName("demo cert"))
	assert.Error(t, CertName("demo%2F"))
	assert.Error(t, CertName("-demo"))
	assert.Error(t, CertName("demo-"))
	assert.Error(t, CertName("_demo"))
	assert.Error(t, CertName("demo_"))
}

func TestEntityTypeName(t *testing.T) {
	assert.NoError(t, EntityTypeName("dep_1"))
	assert.Error(t, EntityTypeName("Dep"))
	assert.Error(t, EntityTypeName("-dep"))
}

func TestEntityName(t *testing.T) {
	assert.NoError(t, EntityName("dep"))
	assert.NoError(t, EntityName("dep_01"))
	assert.NoError(t, EntityName("ai-gateway"))
	assert.NoError(t, EntityName("dep@1"))
	assert.NoError(t, EntityName("zhanghuzhenyu@default"))
	assert.NoError(t, EntityName(strings.Repeat("a", 64)))

	assert.Error(t, EntityName(""))
	assert.Error(t, EntityName(strings.Repeat("a", 65)))
	assert.Error(t, EntityName("Dep"))
	assert.Error(t, EntityName("dep 1"))
	assert.Error(t, EntityName("部门"))
	assert.Error(t, EntityName("dep#1"))
	assert.Error(t, EntityName("@dep"))
	assert.Error(t, EntityName("dep@"))
	assert.Error(t, EntityName("-dep"))
	assert.Error(t, EntityName("_dep"))
	assert.Error(t, EntityName("dep-"))
	assert.Error(t, EntityName("dep_"))
}

func TestAPIKeyDescription(t *testing.T) {
	assert.NoError(t, APIKeyDescription("valid desc"))
	assert.Error(t, APIKeyDescription(""))
	long := make([]byte, 513)
	assert.Error(t, APIKeyDescription(string(long)))
}

func TestAPIKeyValue(t *testing.T) {
	assert.NoError(t, APIKeyValue("ak-123_test"))
	assert.Error(t, APIKeyValue(""))
	assert.Error(t, APIKeyValue("ak@123"))

	// 1-128 characters are allowed; 129 characters should be rejected to match
	// the api-keys.md definition and the MySQL DDL (varchar(128)).
	assert.NoError(t, APIKeyValue(strings.Repeat("a", 128)))
	assert.Error(t, APIKeyValue(strings.Repeat("a", 129)))
}

func TestQuotaPlan(t *testing.T) {
	assert.NoError(t, QuotaPlan(nil))
	q := float64(-1)
	assert.Error(t, QuotaPlan(&shared.QuotaPlanParam{Quota: &q}))
	q = 100
	unit := "invalid"
	assert.Error(t, QuotaPlan(&shared.QuotaPlanParam{Quota: &q, Unit: &unit}))
	unit = "total_token"
	assert.NoError(t, QuotaPlan(&shared.QuotaPlanParam{Quota: &q, Unit: &unit}))

	// RMB quota upper limit: 90,000,000.00 yuan
	unit = "RMB"
	q = 90000000.00
	assert.NoError(t, QuotaPlan(&shared.QuotaPlanParam{Quota: &q, Unit: &unit}))
	q = 90000000.00000001
	assert.Error(t, QuotaPlan(&shared.QuotaPlanParam{Quota: &q, Unit: &unit}))
	q = 90000001.00
	assert.Error(t, QuotaPlan(&shared.QuotaPlanParam{Quota: &q, Unit: &unit}))
}

func TestQuotaValue(t *testing.T) {
	q := float64(-1)
	assert.Error(t, QuotaValue(&q, "total_token"))

	q = 1.5
	assert.Error(t, QuotaValue(&q, "total_token"))

	q = 100
	assert.NoError(t, QuotaValue(&q, "total_token"))

	q = 100.123456789
	assert.Error(t, QuotaValue(&q, "RMB"))

	q = 100.12345678
	assert.NoError(t, QuotaValue(&q, "RMB"))

	q = 90000000.00
	assert.NoError(t, QuotaValue(&q, "RMB"))

	q = 90000000.00000001
	assert.Error(t, QuotaValue(&q, "RMB"))

	q = 90000001.00
	assert.Error(t, QuotaValue(&q, "RMB"))

	assert.NoError(t, QuotaValue(nil, "RMB"))

	// Unknown unit: only non-negative check applies
	q = 123.45
	assert.NoError(t, QuotaValue(&q, "unknown"))
}

func TestRateLimitPolicy(t *testing.T) {
	assert.NoError(t, RateLimitPolicy(nil))
	enabled := true
	policy := &shared.RateLimitPolicyParam{Enabled: &enabled}
	assert.Error(t, RateLimitPolicy(policy))

	policy.Rules = &shared.RateLimitRules{
		TpmConfigs: []shared.TPMConfig{
			{Name: "t1", Model: "*", WindowMinutes: 1, MaxTokens: 100, StepMinutes: 1},
		},
	}
	assert.NoError(t, RateLimitPolicy(policy))

	policy.Rules.TpmConfigs = append(policy.Rules.TpmConfigs, shared.TPMConfig{Name: "t1", Model: "*", WindowMinutes: 1, MaxTokens: 100, StepMinutes: 1})
	assert.Error(t, RateLimitPolicy(policy))
}

func TestRouteRules(t *testing.T) {
	name := "r1"
	cluster := "cluster_1"
	weight := 100

	validCond := "default_t()"
	rules := &shared.RouteRulesParam{
		Rules: []*shared.AiRouteRuleParam{
			{
				Name:    &name,
				Cond:    &validCond,
				Targets: []*shared.AiRouteTargetParam{{ClusterName: &cluster, Weight: &weight}},
			},
		},
	}
	assert.NoError(t, RouteRules(rules))

	weight = 50
	assert.Error(t, RouteRules(rules))
	weight = 100

	// valid cond with quoted path
	quotedPathCond := "req_path_in(\"/v1\", false)"
	rules.Rules[0].Cond = &quotedPathCond
	assert.NoError(t, RouteRules(rules))

	// invalid cond: missing quotes around path
	missingQuoteCond := "req_path_in(/v1, false)"
	rules.Rules[0].Cond = &missingQuoteCond
	assert.Error(t, RouteRules(rules))

	// invalid cond: unknown function
	unknownFuncCond := "unknown_func()"
	rules.Rules[0].Cond = &unknownFuncCond
	assert.Error(t, RouteRules(rules))

	// invalid cond: unmatched parenthesis
	unmatchedParenCond := "default_t("
	rules.Rules[0].Cond = &unmatchedParenCond
	assert.Error(t, RouteRules(rules))
}

func TestConditionExpression(t *testing.T) {
	cases := []struct {
		name    string
		cond    string
		wantErr bool
	}{
		{"default_t", "default_t()", false},
		{"req_path_in quoted", "req_path_in(\"/v1\", false)", false},
		{"combined expression", "req_method_in(\"POST\") && req_path_in(\"/v1\", false)", false},
		{"req_body_larger_than", "req_body_larger_than(8192)", false},
		{"req_body_less_than", "req_body_less_than(2048)", false},
		{"req_body_combined", "req_host_in(\"api.example.com\") && req_body_larger_than(8192)", false},
		{"missing quotes", "req_path_in(/v1, false)", true},
		{"unknown function", "unknown_func()", true},
		{"unmatched parenthesis", "default_t(", true},
		{"empty string", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ConditionExpression(tc.cond)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLLMConfig(t *testing.T) {
	c := &icluster_conf.LLMConfig{
		Provider: lib.PString("openai"),
		Models:   []string{"m1"},
		ModelMappings: []*icluster_conf.Mapping{
			{SourceModel: lib.PString("old"), TargetModel: lib.PString("new")},
		},
		Keys: []icluster_conf.ClusterKeyRef{
			{Name: lib.PString("key-primary"), Weight: lib.PInt(70)},
			{Name: lib.PString("key-secondary"), Weight: lib.PInt(30)},
		},
		KeyPolicy: &icluster_conf.KeyPolicy{
			Strategy:            lib.PString("weighted_random"),
			MaxRetries:          lib.PInt(3),
			RetryBackoffInitial: lib.PInt(500),
			RetryBackoffMax:     lib.PInt(5000),
		},
	}
	assert.NoError(t, LLMConfig(c))

	// duplicate model
	c2 := *c
	c2.Models = []string{"m1", "m1"}
	assert.Error(t, LLMConfig(&c2))

	// total weight not 100
	c3 := *c
	c3.Keys = []icluster_conf.ClusterKeyRef{
		{Name: lib.PString("k1"), Weight: lib.PInt(50)},
		{Name: lib.PString("k2"), Weight: lib.PInt(30)},
	}
	assert.Error(t, LLMConfig(&c3))

	// duplicate key name
	c4 := *c
	c4.Keys = []icluster_conf.ClusterKeyRef{
		{Name: lib.PString("k1"), Weight: lib.PInt(50)},
		{Name: lib.PString("k1"), Weight: lib.PInt(50)},
	}
	assert.Error(t, LLMConfig(&c4))

	// missing provider
	c5 := &icluster_conf.LLMConfig{
		Models: []string{"m1"},
	}
	assert.Error(t, LLMConfig(c5))

	// invalid key_policy retry_backoff_max < retry_backoff_initial
	c6 := *c
	c6.KeyPolicy = &icluster_conf.KeyPolicy{
		Strategy:            lib.PString("weighted_random"),
		MaxRetries:          lib.PInt(3),
		RetryBackoffInitial: lib.PInt(500),
		RetryBackoffMax:     lib.PInt(100),
	}
	assert.Error(t, LLMConfig(&c6))

	// invalid key_policy strategy
	c7 := *c
	c7.KeyPolicy = &icluster_conf.KeyPolicy{
		Strategy: lib.PString("invalid"),
	}
	assert.Error(t, LLMConfig(&c7))

	// strip_prefix=true without match_prefix
	c8 := *c
	c8.StripPrefix = lib.PBool(true)
	assert.Error(t, LLMConfig(&c8))

	// match_prefix not ending with '/'
	c9 := *c
	c9.MatchPrefix = lib.PString("openrouter")
	assert.Error(t, LLMConfig(&c9))

	// valid prefix configuration
	c10 := *c
	c10.MatchPrefix = lib.PString("openrouter/")
	c10.StripPrefix = lib.PBool(true)
	assert.NoError(t, LLMConfig(&c10))

	// valid key_affinity
	c11 := *c
	c11.KeyAffinity = &icluster_conf.KeyAffinity{
		Enabled:       lib.PBool(true),
		TTL:           lib.PInt(600),
		RedisPrefix:   lib.PString("bfe:ai:key_affinity"),
		PenaltyEnable: lib.PBool(true),
	}
	assert.NoError(t, LLMConfig(&c11))

	// invalid key_affinity.ttl
	c12 := *c
	c12.KeyAffinity = &icluster_conf.KeyAffinity{
		TTL: lib.PInt(0),
	}
	assert.Error(t, LLMConfig(&c12))

	// invalid key_affinity.redis_prefix
	c13 := *c
	c13.KeyAffinity = &icluster_conf.KeyAffinity{
		RedisPrefix: lib.PString(""),
	}
	assert.Error(t, LLMConfig(&c13))
}

func TestInstancePool(t *testing.T) {
	instances := []icluster_conf.Instance{
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 100},
	}
	assert.NoError(t, InstancePool(instances))

	instances[0].Weight = 0
	assert.Error(t, InstancePool(instances))

	// duplicate name
	instances[0].Weight = 50
	assert.Error(t, InstancePool([]icluster_conf.Instance{
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Name: "backend-1", Addr: "10.0.0.2", Port: 8080, Weight: 50},
	}))

	// duplicate (name, addr, port)
	assert.Error(t, InstancePool([]icluster_conf.Instance{
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 50},
	}))

	// same addr with empty name but different ports is allowed
	assert.NoError(t, InstancePool([]icluster_conf.Instance{
		{Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Addr: "10.0.0.1", Port: 8081, Weight: 50},
	}))

	// same addr and port with empty name is not allowed
	assert.Error(t, InstancePool([]icluster_conf.Instance{
		{Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Addr: "10.0.0.1", Port: 8080, Weight: 50},
	}))
}

func TestExpiredTime(t *testing.T) {
	minusOne := int64(-1)
	future := time.Now().Unix() + 1000
	past := time.Now().Unix() - 1000
	minusTwo := int64(-2)
	assert.NoError(t, ExpiredTime(&minusOne))
	assert.NoError(t, ExpiredTime(&future))
	assert.Error(t, ExpiredTime(&minusTwo))
	assert.Error(t, ExpiredTime(&past))
	assert.NoError(t, ExpiredTime(nil))
}
