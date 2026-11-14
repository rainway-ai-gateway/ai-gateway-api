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

package icluster_conf

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
)
func TestValidateValidateLLMConfig(t *testing.T) {
	c := &LLMConfig{
		Provider: lib.PString("openai"),
		Models:   []string{"m1"},
		ModelMappings: []*Mapping{
			{SourceModel: lib.PString("old"), TargetModel: lib.PString("new")},
		},
		Keys: []ClusterKeyRef{
			{Name: lib.PString("key-primary"), Weight: lib.PInt(70)},
			{Name: lib.PString("key-secondary"), Weight: lib.PInt(30)},
		},
		KeyPolicy: &KeyPolicy{
			Strategy:            lib.PString("weighted_random"),
			MaxRetries:          lib.PInt(3),
			RetryBackoffInitial: lib.PInt(500),
			RetryBackoffMax:     lib.PInt(5000),
		},
	}
	assert.NoError(t, ValidateLLMConfig(c))

	// duplicate model
	c2 := *c
	c2.Models = []string{"m1", "m1"}
	assert.Error(t, ValidateLLMConfig(&c2))

	// total weight not 100
	c3 := *c
	c3.Keys = []ClusterKeyRef{
		{Name: lib.PString("k1"), Weight: lib.PInt(50)},
		{Name: lib.PString("k2"), Weight: lib.PInt(30)},
	}
	assert.Error(t, ValidateLLMConfig(&c3))

	// duplicate key name
	c4 := *c
	c4.Keys = []ClusterKeyRef{
		{Name: lib.PString("k1"), Weight: lib.PInt(50)},
		{Name: lib.PString("k1"), Weight: lib.PInt(50)},
	}
	assert.Error(t, ValidateLLMConfig(&c4))

	// missing provider
	c5 := &LLMConfig{
		Models: []string{"m1"},
	}
	assert.Error(t, ValidateLLMConfig(c5))

	// invalid key_policy retry_backoff_max < retry_backoff_initial
	c6 := *c
	c6.KeyPolicy = &KeyPolicy{
		Strategy:            lib.PString("weighted_random"),
		MaxRetries:          lib.PInt(3),
		RetryBackoffInitial: lib.PInt(500),
		RetryBackoffMax:     lib.PInt(100),
	}
	assert.Error(t, ValidateLLMConfig(&c6))

	// invalid key_policy strategy
	c7 := *c
	c7.KeyPolicy = &KeyPolicy{
		Strategy: lib.PString("invalid"),
	}
	assert.Error(t, ValidateLLMConfig(&c7))

	// strip_prefix=true without match_prefix
	c8 := *c
	c8.StripPrefix = lib.PBool(true)
	assert.Error(t, ValidateLLMConfig(&c8))

	// match_prefix not ending with '/'
	c9 := *c
	c9.MatchPrefix = lib.PString("openrouter")
	assert.Error(t, ValidateLLMConfig(&c9))

	// valid prefix configuration
	c10 := *c
	c10.MatchPrefix = lib.PString("openrouter/")
	c10.StripPrefix = lib.PBool(true)
	assert.NoError(t, ValidateLLMConfig(&c10))

	// valid key_affinity
	c11 := *c
	c11.KeyAffinity = &KeyAffinity{
		Enabled:        lib.PBool(true),
		TTL:            lib.PInt(600),
		RedisPrefix:    lib.PString("bfe:ai:key_affinity"),
		PenaltyEnable:  lib.PBool(true),
		PenaltyTTL429:  lib.PInt(60),
		PenaltyTTLAuth: lib.PInt(300),
	}
	assert.NoError(t, ValidateLLMConfig(&c11))

	// issue #1448: penalty TTL boundaries (429: [10,600], auth: [60,1800]).
	// nil means "unset" and passes; 0/negative/out-of-range are rejected.
	assert.NoError(t, ValidateLLMConfig(c))
	for _, ttl := range []int{10, 600} {
		cc := *c
		cc.KeyAffinity = &KeyAffinity{PenaltyTTL429: lib.PInt(ttl)}
		assert.NoError(t, ValidateLLMConfig(&cc), "penalty_ttl_429=%d", ttl)
	}
	for _, ttl := range []int{-1, 0, 9, 601} {
		cc := *c
		cc.KeyAffinity = &KeyAffinity{PenaltyTTL429: lib.PInt(ttl)}
		assert.Error(t, ValidateLLMConfig(&cc), "penalty_ttl_429=%d", ttl)
	}
	for _, ttl := range []int{60, 1800} {
		cc := *c
		cc.KeyAffinity = &KeyAffinity{PenaltyTTLAuth: lib.PInt(ttl)}
		assert.NoError(t, ValidateLLMConfig(&cc), "penalty_ttl_auth=%d", ttl)
	}
	for _, ttl := range []int{-1, 0, 59, 1801} {
		cc := *c
		cc.KeyAffinity = &KeyAffinity{PenaltyTTLAuth: lib.PInt(ttl)}
		assert.Error(t, ValidateLLMConfig(&cc), "penalty_ttl_auth=%d", ttl)
	}

	// invalid key_affinity.ttl
	c12 := *c
	c12.KeyAffinity = &KeyAffinity{
		TTL: lib.PInt(0),
	}
	assert.Error(t, ValidateLLMConfig(&c12))

	// invalid key_affinity.redis_prefix
	c13 := *c
	c13.KeyAffinity = &KeyAffinity{
		RedisPrefix: lib.PString(""),
	}
	assert.Error(t, ValidateLLMConfig(&c13))

	// valid normalize_upstream_error (full and partial)
	c14 := *c
	c14.NormalizeUpstreamError = &NormalizeUpstreamError{
		Enabled:            lib.PBool(true),
		StreamEnabled:      lib.PBool(true),
		UnrecognizedAction: lib.PString("rewrite_generic"),
		MaxBodyBytes:       lib.PInt64(65536),
		RedactSecrets:      lib.PBool(false),
	}
	assert.NoError(t, ValidateLLMConfig(&c14))

	c15 := *c
	c15.NormalizeUpstreamError = &NormalizeUpstreamError{
		Enabled: lib.PBool(true),
	}
	assert.NoError(t, ValidateLLMConfig(&c15))

	// valid price_miss_action (issue #1479): three values + empty
	for _, action := range []string{"fallback_chat", "bill_zero", "reject", ""} {
		cpm := *c
		cpm.PriceMissAction = lib.PString(action)
		assert.NoError(t, ValidateLLMConfig(&cpm), action)
	}

	// invalid price_miss_action
	cpmBad := *c
	cpmBad.PriceMissAction = lib.PString("reject_all")
	assert.Error(t, ValidateLLMConfig(&cpmBad))

	// invalid normalize_upstream_error.unrecognized_action
	c16 := *c
	c16.NormalizeUpstreamError = &NormalizeUpstreamError{
		UnrecognizedAction: lib.PString("replace"),
	}
	assert.Error(t, ValidateLLMConfig(&c16))

	// invalid normalize_upstream_error.max_body_bytes (negative / oversized)
	c17 := *c
	c17.NormalizeUpstreamError = &NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(-1),
	}
	assert.Error(t, ValidateLLMConfig(&c17))

	c18 := *c
	c18.NormalizeUpstreamError = &NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(4*1024*1024 + 1),
	}
	assert.Error(t, ValidateLLMConfig(&c18))

	// boundary values are accepted: 0 (BFE default) and exactly 4MB
	c19 := *c
	c19.NormalizeUpstreamError = &NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(0),
	}
	assert.NoError(t, ValidateLLMConfig(&c19))

	c20 := *c
	c20.NormalizeUpstreamError = &NormalizeUpstreamError{
		MaxBodyBytes: lib.PInt64(4 * 1024 * 1024),
	}
	assert.NoError(t, ValidateLLMConfig(&c20))
}

func TestValidateLLMConfigEnsureReasoningContent(t *testing.T) {
	base := func(protocols []string) *LLMConfig {
		return &LLMConfig{
			Provider:               lib.PString("deepseek"),
			Models:                 []string{"deepseek-chat"},
			EnsureReasoningContent: protocols,
		}
	}

	assert.NoError(t, ValidateLLMConfig(base(nil)))
	assert.NoError(t, ValidateLLMConfig(base([]string{})))
	assert.NoError(t, ValidateLLMConfig(base([]string{"openai"})))
	assert.NoError(t, ValidateLLMConfig(base([]string{"openai", "anthropic"})))

	// gemini has no reasoning_content injection point: rejected instead of
	// silently doing nothing
	assert.Error(t, ValidateLLMConfig(base([]string{"gemini"})))
	assert.Error(t, ValidateLLMConfig(base([]string{"foo"})))
	// empty and duplicate entries
	assert.Error(t, ValidateLLMConfig(base([]string{""})))
	assert.Error(t, ValidateLLMConfig(base([]string{"openai", "openai"})))
}
