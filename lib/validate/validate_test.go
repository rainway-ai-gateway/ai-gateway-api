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
	assert.Error(t, UserName("root"))
	assert.Error(t, UserName("system"))
	assert.Error(t, UserName("ADMIN"))
	assert.Error(t, UserName("-user"))
	assert.Error(t, UserName("user."))
	assert.Error(t, UserName("user name"))
	assert.Error(t, UserName(""))
}

func TestUserNameRef(t *testing.T) {
	// Reserved names are valid references: the built-in account "admin"
	// must stay operable through by-name update endpoints (issue #226).
	assert.NoError(t, UserNameRef("admin"))
	assert.NoError(t, UserNameRef("root"))
	assert.NoError(t, UserNameRef("system"))
	assert.NoError(t, UserNameRef("Admin"))
	assert.NoError(t, UserNameRef("user_1"))
	assert.Error(t, UserNameRef("-user"))
	assert.Error(t, UserNameRef("user."))
	assert.Error(t, UserNameRef("user name"))
	assert.Error(t, UserNameRef(""))
	assert.Error(t, UserNameRef(strings.Repeat("a", MaxUserNameLength+1)))
}

func TestPassword(t *testing.T) {
	assert.NoError(t, Password("password123", "user1"))
	assert.NoError(t, Password(strings.Repeat("a", 72), "user1"))
	assert.Error(t, Password(strings.Repeat("a", 73), "user1"))
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

func TestDescription(t *testing.T) {
	t.Run("entity/entity-type boundary is 255", func(t *testing.T) {
		assert.NoError(t, Description(strings.Repeat("a", 255), MaxEntityTypeDescriptionLength, "description"))
		assert.NoError(t, Description(strings.Repeat("a", 255), MaxEntityDescriptionLength, "description"))
		assert.Error(t, Description(strings.Repeat("a", 256), MaxEntityTypeDescriptionLength, "description"))
		assert.Error(t, Description(strings.Repeat("a", 256), MaxEntityDescriptionLength, "description"))
	})

	t.Run("cluster boundary is 256", func(t *testing.T) {
		assert.NoError(t, Description(strings.Repeat("a", 256), MaxClusterDescriptionLength, "description"))
		assert.Error(t, Description(strings.Repeat("a", 257), MaxClusterDescriptionLength, "description"))
	})

	t.Run("certificate boundary is 256", func(t *testing.T) {
		assert.NoError(t, Description(strings.Repeat("a", 256), MaxCertificateDescriptionLength, "description"))
		assert.Error(t, Description(strings.Repeat("a", 257), MaxCertificateDescriptionLength, "description"))
	})

	t.Run("empty string is allowed", func(t *testing.T) {
		assert.NoError(t, Description("", MaxEntityTypeDescriptionLength, "description"))
		assert.NoError(t, Description("", MaxEntityDescriptionLength, "description"))
		assert.NoError(t, Description("", MaxClusterDescriptionLength, "description"))
		assert.NoError(t, Description("", MaxCertificateDescriptionLength, "description"))
	})

	t.Run("control characters are rejected", func(t *testing.T) {
		for _, s := range []string{"abc\x01def", "abc\ndef", "abc\x7fdef", "\x00"} {
			assert.Error(t, Description(s, MaxEntityTypeDescriptionLength, "description"), "%q must be rejected", s)
			assert.Error(t, Description(s, MaxClusterDescriptionLength, "description"), "%q must be rejected", s)
			assert.Error(t, Description(s, MaxCertificateDescriptionLength, "description"), "%q must be rejected", s)
		}
	})

	t.Run("length is counted in UTF-8 bytes", func(t *testing.T) {
		// 85 个汉字 = 255 字节（entity-type 上限内），86 个 = 258 字节（超限）
		assert.NoError(t, Description(strings.Repeat("汉", 85), MaxEntityTypeDescriptionLength, "description"))
		assert.Error(t, Description(strings.Repeat("汉", 86), MaxEntityTypeDescriptionLength, "description"))
	})

	t.Run("limit constants are pinned to the contract values", func(t *testing.T) {
		assert.Equal(t, 255, MaxEntityTypeDescriptionLength, "entity-types.md §1/§2.1: 0-255")
		assert.Equal(t, 255, MaxEntityDescriptionLength, "entities.md §1: 0-255")
		assert.Equal(t, 256, MaxClusterDescriptionLength, "clusters.md §1/§2.1: 0-256")
		assert.Equal(t, 256, MaxCertificateDescriptionLength, "certificates.md §1/§2.1: 必填 2-256")
	})
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

func TestRateLimitPolicy_BatchLimits(t *testing.T) {
	enabled := true

	// batch_limits alone satisfies the "at least one rule" requirement.
	policy := &shared.RateLimitPolicyParam{
		Enabled: &enabled,
		Rules:   &shared.RateLimitRules{BatchLimits: &shared.BatchLimits{}},
	}
	assert.NoError(t, RateLimitPolicy(policy))

	policy.Rules.BatchLimits = &shared.BatchLimits{
		MaxCreateRPM:     10,
		MaxActiveBatches: 5,
		MaxFileBytes:     104857600,
		MaxFileLines:     50000,
	}
	assert.NoError(t, RateLimitPolicy(policy))

	// Negative dimensions are rejected.
	policy.Rules.BatchLimits = &shared.BatchLimits{MaxCreateRPM: -1}
	assert.Error(t, RateLimitPolicy(policy))
	policy.Rules.BatchLimits = &shared.BatchLimits{MaxActiveBatches: -1}
	assert.Error(t, RateLimitPolicy(policy))
	policy.Rules.BatchLimits = &shared.BatchLimits{MaxFileBytes: -1}
	assert.Error(t, RateLimitPolicy(policy))
	policy.Rules.BatchLimits = &shared.BatchLimits{MaxFileLines: -1}
	assert.Error(t, RateLimitPolicy(policy))

	// Zero values are valid (dimension not limited).
	policy.Rules.BatchLimits = &shared.BatchLimits{MaxCreateRPM: 0}
	assert.NoError(t, RateLimitPolicy(policy))
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
		{"weekday_in with zone", "time_weekday_in(\"0|6\", \"H\")", false},
		{"weekday_in default zone", "time_weekday_in(\"1\")", false},
		{"weekday_in combined", "time_weekday_in(\"0|6\", \"H\") && bfe_periodic_time_range(\"090000H\", \"180000H\", \"\")", false},
		{"missing quotes", "req_path_in(/v1, false)", true},
		{"unknown function", "unknown_func()", true},
		{"unmatched parenthesis", "default_t(", true},
		{"empty string", "", true},
		{"weekday_in invalid weekday", "time_weekday_in(\"7\")", true},
		{"weekday_in invalid zone", "time_weekday_in(\"0\", \"QQ\")", true},
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

	// valid normalize_upstream_error (full and partial)
	c14 := *c
	c14.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		Enabled:            lib.PBool(true),
		StreamEnabled:      lib.PBool(true),
		UnrecognizedAction: lib.PString("rewrite_generic"),
		MaxBodyBytes:       lib.PInt64(65536),
		RedactSecrets:      lib.PBool(false),
	}
	assert.NoError(t, LLMConfig(&c14))

	c15 := *c
	c15.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		Enabled: lib.PBool(true),
	}
	assert.NoError(t, LLMConfig(&c15))

	// valid price_miss_action (issue #1479): three values + empty
	for _, action := range []string{"fallback_chat", "bill_zero", "reject", ""} {
		cpm := *c
		cpm.PriceMissAction = lib.PString(action)
		assert.NoError(t, LLMConfig(&cpm), action)
	}

	// invalid price_miss_action
	cpmBad := *c
	cpmBad.PriceMissAction = lib.PString("reject_all")
	assert.Error(t, LLMConfig(&cpmBad))

	// invalid normalize_upstream_error.unrecognized_action
	c16 := *c
	c16.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		UnrecognizedAction: lib.PString("replace"),
	}
	assert.Error(t, LLMConfig(&c16))

	// invalid normalize_upstream_error.max_body_bytes (negative / oversized)
	c17 := *c
	c17.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(-1),
	}
	assert.Error(t, LLMConfig(&c17))

	c18 := *c
	c18.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(4*1024*1024 + 1),
	}
	assert.Error(t, LLMConfig(&c18))

	// boundary values are accepted: 0 (BFE default) and exactly 4MB
	c19 := *c
	c19.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(0),
	}
	assert.NoError(t, LLMConfig(&c19))

	c20 := *c
	c20.NormalizeUpstreamError = &icluster_conf.NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(4 * 1024 * 1024),
	}
	assert.NoError(t, LLMConfig(&c20))
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

func TestAICacheRules(t *testing.T) {
	validCond := "req_path_in(\"/v1/chat/completions\", false) && req_body_json_in(\"model\", \"deepseek-chat\", false)"

	validRule := func() *shared.AICacheRuleParam {
		return &shared.AICacheRuleParam{
			Name:             lib.PString("cache-deepseek-chat"),
			Cond:             &validCond,
			CacheKeyStrategy: lib.PString(AICacheKeyStrategyLastQuestion),
			CacheTTL:         lib.PInt(3600),
			MaxBodyBytes:     lib.PInt64(1048576),
			MaxValueBytes:    lib.PInt64(1048576),
		}
	}

	// nil param and nil rules are accepted (null rules means clear all).
	assert.NoError(t, AICacheRules(nil))
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{}))

	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{validRule()}}))

	// minimal rule: only required fields.
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{
		{Name: lib.PString("minimal"), Cond: lib.PString("default_t()")},
	}}))

	// null rule element is rejected.
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{nil}}))

	// name: required, length 1-128, unique within the collection.
	rule := validRule()
	rule.Name = nil
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	rule = validRule()
	rule.Name = lib.PString("")
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	rule = validRule()
	rule.Name = lib.PString(string(make([]byte, MaxAICacheRuleNameLength+1)))
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	rule = validRule()
	rule.Name = lib.PString(string(make([]byte, MaxAICacheRuleNameLength)))
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	dup := validRule()
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{validRule(), dup}}))

	// cond: required and must compile.
	rule = validRule()
	rule.Cond = nil
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("")
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("unknown_func()")
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	// cond: unique within the collection (byte-wise, matching BFE's
	// ProductRuleConfListFile.Check). A duplicate cond makes the whole exported
	// file unloadable, so it must be rejected before the write (issue #261).
	condA := `req_path_in("/v1/chat/completions", false) && req_body_json_in("model", "issue-261-a", false)`
	condB := `req_path_in("/v1/chat/completions", false) && req_body_json_in("model", "issue-261-b", false)`
	first := validRule()
	first.Cond = &condA
	second := validRule()
	second.Name = lib.PString("cache-deepseek-chat-2")
	second.Cond = &condA
	second.CacheTTL = lib.PInt(60)
	err := AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{first, second}})
	assert.Error(t, err, "duplicate cond with a different name must be rejected")
	assert.Contains(t, err.Error(), "duplicate ai cache rule cond")
	assert.Contains(t, err.Error(), condA)

	// Distinct conds pass (positive anchor); byte-wise comparison means a
	// one-byte difference is enough (no normalisation on either side).
	second.Cond = &condB
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{first, second}}))

	// Duplicate name is still reported before duplicate cond.
	first.Name = lib.PString("same-name")
	second.Name = lib.PString("same-name")
	second.Cond = &condA
	err = AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{first, second}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate ai cache rule name")

	// Two identical unparsable conds report the compile error, not the
	// duplicate error (the compile check runs first). Names are distinct so the
	// name check cannot mask the assertion.
	bad := "unknown_func()"
	first.Name = lib.PString("compile-order-1")
	second.Name = lib.PString("compile-order-2")
	first.Cond = &bad
	second.Cond = &bad
	err = AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{first, second}})
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "duplicate ai cache rule cond")

	// cache_key_strategy enum.
	for _, strategy := range []string{AICacheKeyStrategyLastQuestion, AICacheKeyStrategyAllQuestions, AICacheKeyStrategyDisabled} {
		rule = validRule()
		rule.CacheKeyStrategy = lib.PString(strategy)
		assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
	}
	rule = validRule()
	rule.CacheKeyStrategy = lib.PString("firstQuestion")
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	// cache_ttl >= 0 (nil allowed, 0 allowed).
	rule = validRule()
	rule.CacheTTL = lib.PInt(-1)
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
	rule = validRule()
	rule.CacheTTL = lib.PInt(0)
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	// byte limits > 0 (nil allowed).
	rule = validRule()
	rule.MaxBodyBytes = lib.PInt64(0)
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
	rule = validRule()
	rule.MaxValueBytes = lib.PInt64(-1)
	assert.Error(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
	rule = validRule()
	rule.MaxBodyBytes = nil
	rule.MaxValueBytes = nil
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))

	// enable_semantic_cache: nil/true/false are all accepted (no combination
	// validation; a cache_key_strategy=disabled rule ignores the flag).
	rule = validRule()
	rule.EnableSemanticCache = nil
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
	rule = validRule()
	rule.EnableSemanticCache = lib.PBool(true)
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
	rule = validRule()
	rule.EnableSemanticCache = lib.PBool(false)
	rule.CacheKeyStrategy = lib.PString(AICacheKeyStrategyDisabled)
	assert.NoError(t, AICacheRules(&shared.AICacheRulesParam{Rules: []*shared.AICacheRuleParam{rule}}))
}

func TestAICacheSemanticSettings(t *testing.T) {
	// nil param and an all-nil body are accepted (defaults apply).
	assert.NoError(t, AICacheSemanticSettings(nil))
	assert.NoError(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{}))

	// top_k: 1-10 (boundaries accepted).
	for _, topK := range []int{1, 5, 10} {
		assert.NoError(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{TopK: lib.PInt(topK)}))
	}
	for _, topK := range []int{0, 11, -1} {
		assert.Error(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{TopK: lib.PInt(topK)}))
	}

	// threshold: 0-2 (boundaries accepted).
	for _, threshold := range []float64{0, 0.15, 2} {
		assert.NoError(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{Threshold: lib.PFloat64(threshold)}))
	}
	for _, threshold := range []float64{-0.1, 2.1} {
		assert.Error(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{Threshold: lib.PFloat64(threshold)}))
	}

	// threshold_relation: four-value enum, case-sensitive.
	for _, relation := range []string{"lt", "lte", "gt", "gte"} {
		assert.NoError(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{ThresholdRelation: lib.PString(relation)}))
	}
	for _, relation := range []string{"LT", "Lt", "between", ""} {
		assert.Error(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{ThresholdRelation: lib.PString(relation)}))
	}

	// full body passes.
	assert.NoError(t, AICacheSemanticSettings(&shared.AICacheSemanticSettingsParam{
		TopK:              lib.PInt(3),
		Threshold:         lib.PFloat64(0.5),
		ThresholdRelation: lib.PString("gte"),
	}))
}

func TestAIContextRules(t *testing.T) {
	validRule := func() *shared.AIContextRuleParam {
		return &shared.AIContextRuleParam{
			Cond:             lib.PString("req_path_in(\"/v1/chat/completions\", false)"),
			Mode:             lib.PString(AIContextModeBalanced),
			MaxContextTokens: lib.PInt(64000),
			ReserveTokens:    lib.PInt(8192),
		}
	}

	// nil param and nil rules are accepted (null rules means clear all).
	assert.NoError(t, AIContextRules(nil))
	assert.NoError(t, AIContextRules(&shared.AIContextRulesParam{}))

	// full body and minimal rule (required fields only) pass.
	assert.NoError(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{validRule()}}))
	assert.NoError(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{
		{Cond: lib.PString("default_t()"), Mode: lib.PString(AIContextModeOff)},
	}}))

	// null rule element is rejected.
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{nil}}))

	// cond: required, non-empty, and must compile (aligned with AICacheRules:
	// catch invalid expressions at PUT time instead of a whole-file BFE
	// rejection at load time).
	rule := validRule()
	rule.Cond = nil
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("")
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("default_t(")
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("unknown_func()")
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	// duplicate cond within the collection is rejected.
	dup := validRule()
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{validRule(), dup}}))

	// mode: required, four-value enum.
	for _, mode := range []string{AIContextModeOff, AIContextModeConservative, AIContextModeBalanced, AIContextModeAggressive} {
		rule = validRule()
		rule.Mode = lib.PString(mode)
		assert.NoError(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))
	}

	rule = validRule()
	rule.Mode = nil
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	rule = validRule()
	rule.Mode = lib.PString("")
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	rule = validRule()
	rule.Mode = lib.PString("hyper")
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	// budget fields: >= 0 (nil allowed, 0 allowed).
	rule = validRule()
	rule.MaxContextTokens = lib.PInt(-1)
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))
	rule = validRule()
	rule.MaxContextTokens = lib.PInt(0)
	assert.NoError(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))

	rule = validRule()
	rule.ReserveTokens = lib.PInt(-1)
	assert.Error(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))
	rule = validRule()
	rule.ReserveTokens = nil
	assert.NoError(t, AIContextRules(&shared.AIContextRulesParam{Rules: []*shared.AIContextRuleParam{rule}}))
}

func TestAIContextSettings(t *testing.T) {
	// nil param and an all-nil body are accepted (defaults apply).
	assert.NoError(t, AIContextSettings(nil))
	assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{}))

	// trigger_ratio: (0, 1] (boundaries accepted).
	for _, ratio := range []float64{0.1, 0.7, 1} {
		assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{TriggerRatio: lib.PFloat64(ratio)}))
	}
	for _, ratio := range []float64{0, -0.1, 1.1} {
		assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{TriggerRatio: lib.PFloat64(ratio)}))
	}

	// keep_latest_images / tool_result_max_chars / image_token_estimate: >= 0.
	for _, field := range []int{0, 1, 2000} {
		assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{
			KeepLatestImages:   lib.PInt(field),
			ToolResultMaxChars: lib.PInt(field),
			ImageTokenEstimate: lib.PInt(field),
		}))
	}
	assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{KeepLatestImages: lib.PInt(-1)}))
	assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{ToolResultMaxChars: lib.PInt(-1)}))
	assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{ImageTokenEstimate: lib.PInt(-1)}))

	// thinking_policy: two-value enum, case-sensitive.
	for _, policy := range []string{AIContextThinkingPolicyTrimAllButLast, AIContextThinkingPolicyKeep} {
		assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{ThinkingPolicy: lib.PString(policy)}))
	}
	for _, policy := range []string{"trim", "KEEP", ""} {
		assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{ThinkingPolicy: lib.PString(policy)}))
	}

	// chars_per_token: >= 1.
	assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{CharsPerToken: lib.PInt(1)}))
	assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{CharsPerToken: lib.PInt(3)}))
	assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{CharsPerToken: lib.PInt(0)}))

	// rewrite: nil sub-object accepted; fields validated when present.
	assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{
		Rewrite: &shared.AIContextRewriteParam{},
	}))
	for _, strength := range []string{AIContextRewriteStrengthLite, AIContextRewriteStrengthFull} {
		assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{
			Rewrite: &shared.AIContextRewriteParam{Strength: lib.PString(strength)},
		}))
	}
	assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{
		Rewrite: &shared.AIContextRewriteParam{Strength: lib.PString("max")},
	}))

	// rewrite.protected_survival_rate: (0, 1].
	for _, rate := range []float64{0.5, 0.95, 1} {
		assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{
			Rewrite: &shared.AIContextRewriteParam{ProtectedSurvivalRate: lib.PFloat64(rate)},
		}))
	}
	for _, rate := range []float64{0, -0.1, 1.1} {
		assert.Error(t, AIContextSettings(&shared.AIContextSettingsParam{
			Rewrite: &shared.AIContextRewriteParam{ProtectedSurvivalRate: lib.PFloat64(rate)},
		}))
	}

	// full body passes.
	assert.NoError(t, AIContextSettings(&shared.AIContextSettingsParam{
		TriggerRatio:       lib.PFloat64(0.8),
		KeepLatestImages:   lib.PInt(2),
		ToolResultMaxChars: lib.PInt(2000),
		ThinkingPolicy:     lib.PString(AIContextThinkingPolicyTrimAllButLast),
		CharsPerToken:      lib.PInt(3),
		ImageTokenEstimate: lib.PInt(1200),
		Rewrite: &shared.AIContextRewriteParam{
			Strength:              lib.PString(AIContextRewriteStrengthFull),
			ProtectedSurvivalRate: lib.PFloat64(0.9),
		},
	}))
}

func TestTrafficMirrorRules(t *testing.T) {
	validCond := "req_path_in(\"/v1/chat/completions\", false) && req_body_json_in(\"model\", \"gpt-4o\", false)"

	validRule := func() *shared.TrafficMirrorRuleParam {
		return &shared.TrafficMirrorRuleParam{
			Name:          lib.PString("mirror-gpt4o-to-shadow"),
			Cond:          &validCond,
			MirrorCluster: lib.PString("cluster_shadow"),
			Percentage:    lib.PInt(10),
			RemoveHeaders: &[]string{"Authorization", "Cookie", "X-Api-Key"},
			SetHeaders:    map[string]string{"X-Env": "shadow"},
			BodyRewrites:  []*shared.TrafficMirrorBodyRewriteParam{{Path: lib.PString("model"), Value: lib.PString("deepseek-v3")}},
			PathRewrite:   lib.PString("/v1/mirror"),
		}
	}

	// nil param and nil rules are accepted (null rules means clear all).
	assert.NoError(t, TrafficMirrorRules(nil))
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{}))

	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{validRule()}}))

	// minimal rule: only required fields.
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{
		{Name: lib.PString("minimal"), Cond: lib.PString("default_t()"), MirrorCluster: lib.PString("cluster_shadow")},
	}}))

	// null rule element is rejected.
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{nil}}))

	// name: required, length 1-128, unique within the collection.
	rule := validRule()
	rule.Name = nil
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.Name = lib.PString("")
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.Name = lib.PString(string(make([]byte, MaxTrafficMirrorRuleNameLength+1)))
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.Name = lib.PString(string(make([]byte, MaxTrafficMirrorRuleNameLength)))
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	dup := validRule()
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{validRule(), dup}}))

	// cond: required, must compile, unique within the collection (two
	// identical expressions such as default_t() are rejected).
	rule = validRule()
	rule.Cond = nil
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("")
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.Cond = lib.PString("unknown_func()")
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	other := validRule()
	other.Name = lib.PString("other")
	other.Cond = lib.PString("default_t()")
	dupCond := validRule()
	dupCond.Name = lib.PString("dup-cond")
	dupCond.Cond = lib.PString("default_t()")
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{other, dupCond}}))

	// mirror_cluster: required, length 1-128 (existence is an endpoint concern).
	rule = validRule()
	rule.MirrorCluster = nil
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.MirrorCluster = lib.PString("")
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	rule = validRule()
	rule.MirrorCluster = lib.PString(string(make([]byte, MaxTrafficMirrorClusterNameLength+1)))
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	// percentage: 0-100 (nil allowed).
	rule = validRule()
	rule.Percentage = lib.PInt(-1)
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.Percentage = lib.PInt(101)
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.Percentage = lib.PInt(0)
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.Percentage = nil
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	// remove_headers: elements must be non-empty (nil and explicit empty allowed).
	rule = validRule()
	rule.RemoveHeaders = &[]string{"Authorization", ""}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.RemoveHeaders = &[]string{}
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	// set_headers: key/value must be non-empty.
	rule = validRule()
	rule.SetHeaders = map[string]string{"": "v"}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.SetHeaders = map[string]string{"X-Key": ""}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	// body_rewrites: path must be "model", value required and non-empty.
	rule = validRule()
	rule.BodyRewrites = []*shared.TrafficMirrorBodyRewriteParam{nil}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.BodyRewrites = []*shared.TrafficMirrorBodyRewriteParam{{Path: lib.PString("temperature"), Value: lib.PString("0")}}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.BodyRewrites = []*shared.TrafficMirrorBodyRewriteParam{{Path: nil, Value: lib.PString("0")}}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.BodyRewrites = []*shared.TrafficMirrorBodyRewriteParam{{Path: lib.PString("model"), Value: nil}}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.BodyRewrites = []*shared.TrafficMirrorBodyRewriteParam{{Path: lib.PString("model"), Value: lib.PString("")}}
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.BodyRewrites = nil
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))

	// path_rewrite: empty or must start with '/'.
	rule = validRule()
	rule.PathRewrite = lib.PString("v1/mirror")
	assert.Error(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.PathRewrite = lib.PString("")
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
	rule = validRule()
	rule.PathRewrite = nil
	assert.NoError(t, TrafficMirrorRules(&shared.TrafficMirrorRulesParam{Rules: []*shared.TrafficMirrorRuleParam{rule}}))
}
