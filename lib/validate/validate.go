// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validate

import (
	"fmt"
	"math"
	"net"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
	golibquota "github.com/bfenetworks/go-lib/quota"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
)

var (
	// hostnameLabel matches a single DNS label (RFC 1123) with length 1-63.
	hostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	// nameToken matches the character set used by UserName/TokenName/ClusterName.
	nameToken = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	// entityNameToken matches the character set used by EntityName.
	// '@' is allowed to support names in the form "user@project".
	entityNameToken = regexp.MustCompile(`^[a-z0-9_@_-]+$`)
	// entityTypeToken matches the character set used by EntityTypeName.
	entityTypeToken = regexp.MustCompile(`^[a-z0-9_-]+$`)
	// rateLimitNameToken matches the character set used by rate-limit rule names.
	rateLimitNameToken = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

const (
	MaxUserNameLength       = 64
	MaxTokenNameLength      = 64
	MaxClusterNameLength    = 64
	MaxCertNameLength       = 64
	MaxEntityTypeNameLength = 32
	MaxEntityNameLength     = 64
	MaxDescriptionLength    = 256
	MaxAPIDescriptionLength = 512
	MaxLLMKeyLength         = 512
	MaxRateLimitNameLength  = 128
)

var reservedUserNames = map[string]bool{
	"admin":  true,
	"root":   true,
	"system": true,
}

var reservedTokenNames = map[string]bool{
	"admin":   true,
	"system":  true,
	"default": true,
}

// Hostname validates a hostname according to RFC 1123, with the additional
// system requirement that the total length must be at least 2 characters.
// Valid IPv4/IPv6 addresses are also accepted.
func Hostname(s string) error {
	if len(s) == 0 {
		return xerror.WrapParamErrorWithMsg("hostname is required")
	}
	if len(s) > 255 {
		return xerror.WrapParamErrorWithMsg("hostname length must be <= 255")
	}
	if len(s) < 2 {
		return xerror.WrapParamErrorWithMsg("hostname length must be >= 2")
	}
	if net.ParseIP(s) != nil {
		return nil
	}
	labels := strings.Split(s, ".")
	for _, label := range labels {
		if !hostnameLabel.MatchString(label) {
			return xerror.WrapParamErrorWithMsg("hostname %q contains invalid label %q", s, label)
		}
	}
	return nil
}

// IPAddress validates a string as an IPv4 or IPv6 address.
func IPAddress(s string) error {
	if net.ParseIP(s) == nil {
		return xerror.WrapParamErrorWithMsg("invalid IP address: %s", s)
	}
	return nil
}

// Port validates a port number.
func Port(i int) error {
	if i < 1 || i > 65535 {
		return xerror.WrapParamErrorWithMsg("port must be between 1 and 65535, got %d", i)
	}
	return nil
}

// CIDR validates a CIDR string. The special value "*" is allowed.
func CIDR(s string) error {
	if s == "*" {
		return nil
	}
	_, _, err := net.ParseCIDR(s)
	if err != nil {
		return xerror.WrapParamErrorWithMsg("invalid CIDR: %s", s)
	}
	return nil
}

// AIModelFormat validates a model name format. The special value "*" is allowed.
// Existence in cluster configuration is checked separately by the endpoint.
func AIModelFormat(s string) error {
	if s == "*" {
		return nil
	}
	if len(s) == 0 {
		return xerror.WrapParamErrorWithMsg("model name cannot be empty")
	}
	return nil
}

// NoControlChars returns an error if the string contains Unicode control characters.
func NoControlChars(s string) error {
	for _, r := range s {
		if r < 32 || r == 127 {
			return xerror.WrapParamErrorWithMsg("string contains control characters")
		}
	}
	return nil
}

// NoLeadingTrailingSpace returns an error if the string has leading or trailing whitespace.
func NoLeadingTrailingSpace(s string) error {
	if len(s) == 0 {
		return nil
	}
	if unicode.IsSpace(rune(s[0])) || unicode.IsSpace(rune(s[len(s)-1])) {
		return xerror.WrapParamErrorWithMsg("string cannot have leading or trailing whitespace")
	}
	return nil
}

// NoWhitespace returns an error if the string contains any whitespace character.
func NoWhitespace(s string) error {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return xerror.WrapParamErrorWithMsg("string cannot contain whitespace")
		}
	}
	return nil
}

func validateName(s string, minLen, maxLen int, name string) error {
	if len(s) < minLen || len(s) > maxLen {
		return xerror.WrapParamErrorWithMsg("%s length must be between %d and %d", name, minLen, maxLen)
	}
	return nil
}

func validateNamePattern(s string, re *regexp.Regexp, name string) error {
	if !re.MatchString(s) {
		return xerror.WrapParamErrorWithMsg("%s contains invalid characters", name)
	}
	return nil
}

func validateNameEdges(s string, name string) error {
	first := s[0]
	last := s[len(s)-1]
	if first == '.' || first == '-' || first == '_' || last == '.' || last == '-' || last == '_' {
		return xerror.WrapParamErrorWithMsg("%s cannot start or end with '.', '-', or '_'", name)
	}
	return nil
}

// UserName validates a user name.
func UserName(s string) error {
	if err := validateName(s, 1, MaxUserNameLength, "user_name"); err != nil {
		return err
	}
	if err := validateNamePattern(s, nameToken, "user_name"); err != nil {
		return err
	}
	if err := validateNameEdges(s, "user_name"); err != nil {
		return err
	}
	if reservedUserNames[strings.ToLower(s)] {
		return xerror.WrapParamErrorWithMsg("user_name %q is reserved", s)
	}
	return nil
}

// Password validates a password. It must not equal the user name or its reverse.
func Password(password, userName string) error {
	if len(password) < 8 || len(password) > 128 {
		return xerror.WrapParamErrorWithMsg("password length must be between 8 and 128")
	}
	for _, r := range password {
		if unicode.IsSpace(r) {
			return xerror.WrapParamErrorWithMsg("password cannot contain whitespace")
		}
	}
	if password == userName || password == reverseString(userName) {
		return xerror.WrapParamErrorWithMsg("password cannot be the same as or the reverse of user_name")
	}
	return nil
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// TokenName validates a token name.
func TokenName(s string) error {
	if err := validateName(s, 1, MaxTokenNameLength, "token_name"); err != nil {
		return err
	}
	if err := validateNamePattern(s, nameToken, "token_name"); err != nil {
		return err
	}
	if err := validateNameEdges(s, "token_name"); err != nil {
		return err
	}
	if reservedTokenNames[strings.ToLower(s)] {
		return xerror.WrapParamErrorWithMsg("token_name %q is reserved", s)
	}
	return nil
}

// ClusterName validates a cluster name.
func ClusterName(s string) error {
	if err := validateName(s, 1, MaxClusterNameLength, "cluster_name"); err != nil {
		return err
	}
	if err := validateNamePattern(s, nameToken, "cluster_name"); err != nil {
		return err
	}
	if err := validateNameEdges(s, "cluster_name"); err != nil {
		return err
	}
	return nil
}

// CertName validates a certificate name.
// Rules are aligned with ClusterName, except the length limit starts at 2
// to match the existing "required,min=2" constraint on cert_name.
func CertName(s string) error {
	if err := validateName(s, 2, MaxCertNameLength, "cert_name"); err != nil {
		return err
	}
	if err := validateNamePattern(s, nameToken, "cert_name"); err != nil {
		return err
	}
	if err := validateNameEdges(s, "cert_name"); err != nil {
		return err
	}
	return nil
}

// EntityName validates an entity name.
// Rules are aligned with EntityTypeName, except the length limit (64 vs 32)
// and the additional '@' character (for names in the form "user@project").
func EntityName(s string) error {
	if err := validateName(s, 1, MaxEntityNameLength, "name"); err != nil {
		return err
	}
	if err := validateNamePattern(s, entityNameToken, "name"); err != nil {
		return err
	}
	first := s[0]
	last := s[len(s)-1]
	if first == '-' || first == '_' || first == '@' || last == '-' || last == '_' || last == '@' {
		return xerror.WrapParamErrorWithMsg("name cannot start or end with '-', '_', or '@'")
	}
	return nil
}

// EntityTypeName validates an entity type name.
func EntityTypeName(s string) error {
	if err := validateName(s, 1, MaxEntityTypeNameLength, "type_name"); err != nil {
		return err
	}
	if err := validateNamePattern(s, entityTypeToken, "type_name"); err != nil {
		return err
	}
	first := s[0]
	last := s[len(s)-1]
	if first == '-' || first == '_' || last == '-' || last == '_' {
		return xerror.WrapParamErrorWithMsg("type_name cannot start or end with '-' or '_'")
	}
	return nil
}

// Description validates a description string: no control chars, length <= max.
func Description(s string, maxLen int, name string) error {
	if len(s) > maxLen {
		return xerror.WrapParamErrorWithMsg("%s length must be <= %d", name, maxLen)
	}
	return NoControlChars(s)
}

// APIKeyDescription validates API-Key description.
func APIKeyDescription(s string) error {
	if len(s) == 0 {
		return xerror.WrapParamErrorWithMsg("description is required")
	}
	return Description(s, MaxAPIDescriptionLength, "description")
}

// APIKeyValue validates API-Key value format.
func APIKeyValue(s string) error {
	if len(s) == 0 || len(s) > 128 {
		return xerror.WrapParamErrorWithMsg("key length must be between 1 and 128")
	}
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return xerror.WrapParamErrorWithMsg("key contains invalid characters")
		}
	}
	return nil
}

// ExpiredTime validates an expiration Unix timestamp. -1 means never expire.
// A nil pointer means the field was not provided.
func ExpiredTime(t *int64) error {
	if t == nil {
		return nil
	}
	if *t < -1 {
		return xerror.WrapParamErrorWithMsg("expired_time must be -1 or a valid Unix timestamp")
	}
	if *t != -1 && *t < time.Now().Unix() {
		return xerror.WrapParamErrorWithMsg("expired_time must be >= current time")
	}
	return nil
}

// Scope validates an auth token scope.
func Scope(s string) error {
	if s != "System" && s != "Support" {
		return xerror.WrapParamErrorWithMsg("scope must be System or Support")
	}
	return nil
}

// IsAdmin validates that is_admin is true (the system only supports true).
func IsAdmin(b bool) error {
	if !b {
		return xerror.WrapParamErrorWithMsg("is_admin must be true")
	}
	return nil
}

// QuotaPlan validates a quota plan configuration.
func QuotaPlan(p *shared.QuotaPlanParam) error {
	if p == nil {
		return nil
	}

	unit := "total_token"
	if p.Unit != nil && *p.Unit != "" {
		unit = *p.Unit
	}
	if unit != "total_token" && unit != "RMB" {
		return xerror.WrapParamErrorWithMsg("unit must be total_token or RMB")
	}

	if err := QuotaValue(p.Quota, unit); err != nil {
		return err
	}

	if p.ResetPeriod != nil && *p.ResetPeriod != "" {
		switch *p.ResetPeriod {
		case "never", "weekly", "monthly":
		default:
			return xerror.WrapParamErrorWithMsg("reset_period must be never, weekly or monthly")
		}
	}
	return nil
}

// QuotaValue validates a single quota value for the given unit.
// A nil quota is treated as valid (not provided).
func QuotaValue(quota *float64, unit string) error {
	if quota == nil {
		return nil
	}
	if *quota < 0 {
		return xerror.WrapParamErrorWithMsg("quota must be >= 0")
	}
	if unit == "total_token" {
		if math.Mod(*quota, 1) != 0 {
			return xerror.WrapParamErrorWithMsg("quota must be an integer when unit is total_token")
		}
	} else if unit == "RMB" {
		if !validDecimalPlaces(*quota, 8) {
			return xerror.WrapParamErrorWithMsg("quota can have at most 8 decimal places when unit is RMB")
		}
		if *quota > golibquota.MaxRMBQuota {
			return xerror.WrapParamErrorWithMsg("quota must be <= %v when unit is RMB", golibquota.MaxRMBQuota)
		}
	}
	return nil
}

func validDecimalPlaces(v float64, maxPlaces int) bool {
	mult := math.Pow(10, float64(maxPlaces))
	scaled := v * mult
	return math.Abs(scaled-math.Round(scaled)) < 1e-9
}

// RateLimitPolicy validates a rate limit policy configuration.
func RateLimitPolicy(p *shared.RateLimitPolicyParam) error {
	if p == nil {
		return nil
	}
	if p.Enabled != nil && *p.Enabled {
		if p.Rules == nil {
			return xerror.WrapParamErrorWithMsg("when rate_limit_policy.enabled is true, rules must be set")
		}
		hasTpm := len(p.Rules.TpmConfigs) > 0
		hasRpm := len(p.Rules.RpmConfigs) > 0
		hasConcurrency := p.Rules.MaxConcurrency != nil && *p.Rules.MaxConcurrency >= 0
		if !hasTpm && !hasRpm && !hasConcurrency {
			return xerror.WrapParamErrorWithMsg("when rate_limit_policy.enabled is true, at least one of rules.tpm, rules.rpm, or rules.max_concurrency(>=0) must be set")
		}
	}
	if p.Rules == nil {
		return nil
	}
	if len(p.Rules.TpmConfigs) > 3 {
		return xerror.WrapParamErrorWithMsg("tpm configs must be <= 3")
	}
	if len(p.Rules.RpmConfigs) > 3 {
		return xerror.WrapParamErrorWithMsg("rpm configs must be <= 3")
	}
	if p.Rules.MaxConcurrency != nil && *p.Rules.MaxConcurrency < -1 {
		return xerror.WrapParamErrorWithMsg("max_concurrency must be -1 or >= 0")
	}

	nameSet := map[string]struct{}{}
	tpmKeySet := map[string]struct{}{}
	for _, tpm := range p.Rules.TpmConfigs {
		if len(tpm.Name) == 0 {
			return xerror.WrapParamErrorWithMsg("tpm name is required")
		}
		if len(tpm.Name) > MaxRateLimitNameLength {
			return xerror.WrapParamErrorWithMsg("tpm name length must be <= %d", MaxRateLimitNameLength)
		}
		if !rateLimitNameToken.MatchString(tpm.Name) {
			return xerror.WrapParamErrorWithMsg("tpm name can only contain letters, digits, '_' and '-'")
		}
		if _, ok := nameSet[tpm.Name]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate rate limit name: %s", tpm.Name)
		}
		nameSet[tpm.Name] = struct{}{}
		if err := AIModelFormat(tpm.Model); err != nil {
			return xerror.WrapParamErrorWithMsg("tpm model: %v", err)
		}
		if tpm.WindowMinutes < 1 || tpm.WindowMinutes > 360 {
			return xerror.WrapParamErrorWithMsg("tpm window_minutes must be between 1 and 360")
		}
		if tpm.MaxTokens < 0 {
			return xerror.WrapParamErrorWithMsg("tpm max_tokens must be >= 0")
		}
		if tpm.StepMinutes < 1 || tpm.StepMinutes > 360 {
			return xerror.WrapParamErrorWithMsg("tpm step_minutes must be between 1 and 360")
		}
		if tpm.StepMinutes > tpm.WindowMinutes {
			return xerror.WrapParamErrorWithMsg("tpm step_minutes must be <= window_minutes")
		}
		key := fmt.Sprintf("%s|%d|%d|%d", tpm.Model, tpm.WindowMinutes, tpm.MaxTokens, tpm.StepMinutes)
		if _, ok := tpmKeySet[key]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate tpm config: model=%s window_minutes=%d max_tokens=%d step_minutes=%d", tpm.Model, tpm.WindowMinutes, tpm.MaxTokens, tpm.StepMinutes)
		}
		tpmKeySet[key] = struct{}{}
	}

	rpmKeySet := map[string]struct{}{}
	for _, rpm := range p.Rules.RpmConfigs {
		if len(rpm.Name) == 0 {
			return xerror.WrapParamErrorWithMsg("rpm name is required")
		}
		if len(rpm.Name) > MaxRateLimitNameLength {
			return xerror.WrapParamErrorWithMsg("rpm name length must be <= %d", MaxRateLimitNameLength)
		}
		if !rateLimitNameToken.MatchString(rpm.Name) {
			return xerror.WrapParamErrorWithMsg("rpm name can only contain letters, digits, '_' and '-'")
		}
		if _, ok := nameSet[rpm.Name]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate rate limit name: %s", rpm.Name)
		}
		nameSet[rpm.Name] = struct{}{}
		if err := AIModelFormat(rpm.Model); err != nil {
			return xerror.WrapParamErrorWithMsg("rpm model: %v", err)
		}
		if rpm.WindowMinutes < 1 || rpm.WindowMinutes > 360 {
			return xerror.WrapParamErrorWithMsg("rpm window_minutes must be between 1 and 360")
		}
		if rpm.MaxRequests < 0 {
			return xerror.WrapParamErrorWithMsg("rpm max_requests must be >= 0")
		}
		key := fmt.Sprintf("%s|%d|%d", rpm.Model, rpm.WindowMinutes, rpm.MaxRequests)
		if _, ok := rpmKeySet[key]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate rpm config: model=%s window_minutes=%d max_requests=%d", rpm.Model, rpm.WindowMinutes, rpm.MaxRequests)
		}
		rpmKeySet[key] = struct{}{}
	}

	return nil
}

// ConditionExpression validates a BFE condition expression string.
func ConditionExpression(cond string) error {
	if _, err := condition.Build(cond); err != nil {
		return xerror.WrapParamErrorWithMsg("invalid route rule Cond: %v", err)
	}
	return nil
}

// RouteRules validates a route rule set.
func RouteRules(p *shared.RouteRulesParam) error {
	if p == nil {
		return nil
	}
	if p.Rules == nil {
		return nil
	}

	nameSet := map[string]struct{}{}
	for _, rule := range p.Rules {
		if rule.Name == nil || *rule.Name == "" {
			return xerror.WrapParamErrorWithMsg("route rule name is required")
		}
		if _, ok := nameSet[*rule.Name]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate route rule name: %s", *rule.Name)
		}
		nameSet[*rule.Name] = struct{}{}

		if rule.Cond == nil || *rule.Cond == "" {
			return xerror.WrapParamErrorWithMsg("route rule Cond is required")
		}
		if err := ConditionExpression(*rule.Cond); err != nil {
			return err
		}

		if len(rule.Targets) == 0 {
			return xerror.WrapParamErrorWithMsg("route rule targets cannot be empty")
		}

		totalWeight := 0
		targetKeySet := map[string]struct{}{}
		for _, target := range rule.Targets {
			if target.Weight == nil {
				return xerror.WrapParamErrorWithMsg("target weight is required")
			}
			if *target.Weight < 0 || *target.Weight > 100 {
				return xerror.WrapParamErrorWithMsg("target weight must be between 0 and 100")
			}
			totalWeight += *target.Weight

			if target.ClusterName == nil || *target.ClusterName == "" {
				return xerror.WrapParamErrorWithMsg("target ClusterName is required")
			}
			if err := ClusterName(*target.ClusterName); err != nil {
				return xerror.WrapParamErrorWithMsg("target ClusterName: %v", err)
			}
			model := ""
			if target.Model != nil {
				model = *target.Model
			}
			if model != "" {
				if err := AIModelFormat(model); err != nil {
					return xerror.WrapParamErrorWithMsg("target Model: %v", err)
				}
			}
			key := fmt.Sprintf("%s|%s", *target.ClusterName, model)
			if _, ok := targetKeySet[key]; ok {
				return xerror.WrapParamErrorWithMsg("duplicate target (ClusterName=%s, Model=%s)", *target.ClusterName, model)
			}
			targetKeySet[key] = struct{}{}
		}
		if totalWeight != 100 {
			return xerror.WrapParamErrorWithMsg("rule %s targets total weight must be 100, got %d", *rule.Name, totalWeight)
		}

		fallbackKeySet := map[string]struct{}{}
		for _, fallback := range rule.Fallbacks {
			if fallback.ClusterName == nil || *fallback.ClusterName == "" {
				return xerror.WrapParamErrorWithMsg("fallback ClusterName is required")
			}
			if err := ClusterName(*fallback.ClusterName); err != nil {
				return xerror.WrapParamErrorWithMsg("fallback ClusterName: %v", err)
			}
			model := ""
			if fallback.Model != nil {
				model = *fallback.Model
			}
			if model != "" {
				if err := AIModelFormat(model); err != nil {
					return xerror.WrapParamErrorWithMsg("fallback Model: %v", err)
				}
			}
			key := fmt.Sprintf("%s|%s", *fallback.ClusterName, model)
			if _, ok := fallbackKeySet[key]; ok {
				return xerror.WrapParamErrorWithMsg("duplicate fallback (ClusterName=%s, Model=%s)", *fallback.ClusterName, model)
			}
			fallbackKeySet[key] = struct{}{}
		}
	}

	return nil
}

// LLMConfig validates the LLM configuration block used by clusters.
func LLMConfig(c *icluster_conf.LLMConfig) error {
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

const (
	MaxInstanceNameLength = 128
)

// Instance validates a single instance (used by cluster/alb-pool).
func Instance(inst icluster_conf.Instance) error {
	if inst.Name != "" {
		if len(inst.Name) < 1 || len(inst.Name) > MaxInstanceNameLength {
			return xerror.WrapParamErrorWithMsg("instance name length must be between 1 and %d", MaxInstanceNameLength)
		}
	}
	if err := Hostname(inst.Addr); err != nil {
		return xerror.WrapParamErrorWithMsg("instance addr: %v", err)
	}
	if err := Port(inst.Port); err != nil {
		return xerror.WrapParamErrorWithMsg("instance port: %v", err)
	}
	if inst.Weight < 0 || inst.Weight > 100 {
		return xerror.WrapParamErrorWithMsg("instance weight must be between 0 and 100")
	}
	return nil
}

// InstancePool validates a list of instances.
func InstancePool(instances []icluster_conf.Instance) error {
	if len(instances) == 0 {
		return xerror.WrapParamErrorWithMsg("instance_pool is required")
	}
	nameSet := map[string]struct{}{}
	comboSet := map[string]struct{}{}
	hasPositiveWeight := false
	for i, inst := range instances {
		if err := Instance(inst); err != nil {
			return xerror.WrapParamErrorWithMsg("instance_pool[%d]: %v", i, err)
		}
		if inst.Weight > 0 {
			hasPositiveWeight = true
		}
		if inst.Name != "" {
			if _, ok := nameSet[inst.Name]; ok {
				return xerror.WrapParamErrorWithMsg("duplicate instance name: %s", inst.Name)
			}
			nameSet[inst.Name] = struct{}{}
		}
		key := fmt.Sprintf("%s|%s|%d", inst.Name, inst.Addr, inst.Port)
		if _, ok := comboSet[key]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate instance (name=%s, addr=%s, port=%d)", inst.Name, inst.Addr, inst.Port)
		}
		comboSet[key] = struct{}{}
	}
	if !hasPositiveWeight {
		return xerror.WrapParamErrorWithMsg("instance_pool must have at least one instance with weight > 0")
	}
	return nil
}
