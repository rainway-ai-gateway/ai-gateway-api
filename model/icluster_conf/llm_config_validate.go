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
	"strings"

	"github.com/bfenetworks/bfe/bfe_basic"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// ValidateLLMConfig validates the LLM configuration block used by clusters.
// Moved from lib/validate (issue #321) so that lib/validate no longer depends
// on model/icluster_conf, unblocking model/iprovider from reusing
// validate.Hostname.
func ValidateLLMConfig(c *LLMConfig) error {
	if c == nil {
		return xerror.WrapParamErrorWithMsg("llm_config is required")
	}
	if c.Provider == nil || *c.Provider == "" {
		return xerror.WrapParamErrorWithMsg("llm_config.provider is required")
	}
	if len(c.Models) == 0 {
		return xerror.WrapParamErrorWithMsg("llm_config.models is required")
	}
	modelSet := map[string]struct{}{}
	for _, model := range c.Models {
		if len(model) == 0 {
			return xerror.WrapParamErrorWithMsg("llm_config.models cannot contain empty model name")
		}
		if _, ok := modelSet[model]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate model in llm_config.models: %s", model)
		}
		modelSet[model] = struct{}{}
	}

	sourceSet := map[string]struct{}{}
	for _, mapping := range c.ModelMappings {
		if mapping.SourceModel == nil || *mapping.SourceModel == "" {
			return xerror.WrapParamErrorWithMsg("llm_config.model_mappings.source_model is required")
		}
		if mapping.TargetModel == nil || *mapping.TargetModel == "" {
			return xerror.WrapParamErrorWithMsg("llm_config.model_mappings.target_model is required")
		}
		if _, ok := sourceSet[*mapping.SourceModel]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate source_model in llm_config.model_mappings: %s", *mapping.SourceModel)
		}
		sourceSet[*mapping.SourceModel] = struct{}{}
	}

	// Validate key references
	if len(c.Keys) > 0 {
		nameSet := map[string]struct{}{}
		totalWeight := 0
		for i, key := range c.Keys {
			if key.Name == nil || *key.Name == "" {
				return xerror.WrapParamErrorWithMsg("llm_config.keys[%d].name is required", i)
			}
			if len(*key.Name) < 1 || len(*key.Name) > 128 {
				return xerror.WrapParamErrorWithMsg("llm_config.keys[%d].name length must be between 1 and 128", i)
			}
			if _, ok := nameSet[*key.Name]; ok {
				return xerror.WrapParamErrorWithMsg("duplicate name in llm_config.keys: %s", *key.Name)
			}
			nameSet[*key.Name] = struct{}{}

			if key.Weight == nil {
				return xerror.WrapParamErrorWithMsg("llm_config.keys[%d].weight is required", i)
			}
			if *key.Weight < 0 || *key.Weight > 100 {
				return xerror.WrapParamErrorWithMsg("llm_config.keys[%d].weight must be between 0 and 100", i)
			}
			totalWeight += *key.Weight
		}
		if totalWeight != 100 {
			return xerror.WrapParamErrorWithMsg("llm_config.keys total weight must be 100, got %d", totalWeight)
		}
	}

	// Validate key_policy
	if c.KeyPolicy != nil {
		if c.KeyPolicy.Strategy != nil && *c.KeyPolicy.Strategy != "" && *c.KeyPolicy.Strategy != "weighted_random" {
			return xerror.WrapParamErrorWithMsg("llm_config.key_policy.strategy must be weighted_random")
		}
		if c.KeyPolicy.MaxRetries != nil && *c.KeyPolicy.MaxRetries < 0 {
			return xerror.WrapParamErrorWithMsg("llm_config.key_policy.max_retries must be >= 0")
		}
		if c.KeyPolicy.RetryBackoffInitial != nil && *c.KeyPolicy.RetryBackoffInitial < 0 {
			return xerror.WrapParamErrorWithMsg("llm_config.key_policy.retry_backoff_initial must be >= 0")
		}
		if c.KeyPolicy.RetryBackoffMax != nil && *c.KeyPolicy.RetryBackoffMax < 0 {
			return xerror.WrapParamErrorWithMsg("llm_config.key_policy.retry_backoff_max must be >= 0")
		}
		if c.KeyPolicy.RetryBackoffInitial != nil && c.KeyPolicy.RetryBackoffMax != nil &&
			*c.KeyPolicy.RetryBackoffMax < *c.KeyPolicy.RetryBackoffInitial {
			return xerror.WrapParamErrorWithMsg("llm_config.key_policy.retry_backoff_max must be >= retry_backoff_initial")
		}
	}

	// Validate key_affinity
	if c.KeyAffinity != nil {
		if c.KeyAffinity.TTL != nil && *c.KeyAffinity.TTL <= 0 {
			return xerror.WrapParamErrorWithMsg("llm_config.key_affinity.ttl must be > 0")
		}
		if c.KeyAffinity.RedisPrefix != nil && *c.KeyAffinity.RedisPrefix == "" {
			return xerror.WrapParamErrorWithMsg("llm_config.key_affinity.redis_prefix must not be empty")
		}
		// issue #1448: both ranges exclude 0 on purpose. An unset field is nil
		// and filled with the default; 0 is not a way to disable the penalty,
		// because Redis treats expire=0 as "never expire".
		if c.KeyAffinity.PenaltyTTL429 != nil &&
			(*c.KeyAffinity.PenaltyTTL429 < 10 || *c.KeyAffinity.PenaltyTTL429 > 600) {
			return xerror.WrapParamErrorWithMsg("llm_config.key_affinity.penalty_ttl_429 must be between 10 and 600")
		}
		if c.KeyAffinity.PenaltyTTLAuth != nil &&
			(*c.KeyAffinity.PenaltyTTLAuth < 60 || *c.KeyAffinity.PenaltyTTLAuth > 1800) {
			return xerror.WrapParamErrorWithMsg("llm_config.key_affinity.penalty_ttl_auth must be between 60 and 1800")
		}
	}

	// Validate price_miss_action (issue #1479); the rules mirror the BFE
	// AIConfCheck exactly so an accepted config never fails BFE loading.
	if c.PriceMissAction != nil {
		switch *c.PriceMissAction {
		case "", "fallback_chat", "bill_zero", "reject":
		default:
			return xerror.WrapParamErrorWithMsg(
				"llm_config.price_miss_action must be fallback_chat, bill_zero or reject")
		}
	}

	// Validate normalize_upstream_error; the rules mirror the BFE
	// AIConfCheck exactly so an accepted config never fails BFE loading.
	if c.NormalizeUpstreamError != nil {
		nue := c.NormalizeUpstreamError
		if nue.UnrecognizedAction != nil && *nue.UnrecognizedAction != "" &&
			*nue.UnrecognizedAction != "passthrough" && *nue.UnrecognizedAction != "rewrite_generic" {
			return xerror.WrapParamErrorWithMsg(
				"llm_config.normalize_upstream_error.unrecognized_action must be passthrough or rewrite_generic")
		}
		if nue.MaxBodyBytes != nil && (*nue.MaxBodyBytes < 0 || *nue.MaxBodyBytes > 4*1024*1024) {
			return xerror.WrapParamErrorWithMsg(
				"llm_config.normalize_upstream_error.max_body_bytes must be between 0 and 4194304")
		}
	}

	// Validate ensure_reasoning_content; the accepted set is deliberately
	// narrower than provider model_protocols (no gemini: its contents/parts
	// body has no reasoning_content field), and it mirrors the bfe
	// AIConfCheck rules so an accepted config never fails the bfe load.
	if len(c.EnsureReasoningContent) > 0 {
		seen := make(map[string]bool, len(c.EnsureReasoningContent))
		for _, p := range c.EnsureReasoningContent {
			switch p {
			case bfe_basic.AuthStyleOpenAI, bfe_basic.AuthStyleAnthropic:
			default:
				return xerror.WrapParamErrorWithMsg(
					"llm_config.ensure_reasoning_content contains unsupported protocol %q (only openai, anthropic)", p)
			}
			if seen[p] {
				return xerror.WrapParamErrorWithMsg(
					"llm_config.ensure_reasoning_content must not contain empty or duplicate entries")
			}
			seen[p] = true
		}
	}

	// Validate prefix stripping configuration
	if c.StripPrefix != nil && *c.StripPrefix {
		if c.MatchPrefix == nil || *c.MatchPrefix == "" {
			return xerror.WrapParamErrorWithMsg("llm_config.match_prefix is required when strip_prefix is true")
		}
	}
	if c.MatchPrefix != nil && *c.MatchPrefix != "" && !strings.HasSuffix(*c.MatchPrefix, "/") {
		return xerror.WrapParamErrorWithMsg("llm_config.match_prefix must end with '/'")
	}

	return nil
}
