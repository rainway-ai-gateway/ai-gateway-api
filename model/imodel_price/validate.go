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

package imodel_price

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/bfenetworks/go-lib/quota"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// Mode enums.
var ValidModes = map[string]bool{
	"chat":                true,
	"completion":          true,
	"responses":           true,
	"image_generation":    true,
	"image_edit":          true,
	"embedding":           true,
	"rerank":              true,
	"audio_speech":        true,
	"audio_transcription": true,
	"video_generation":    true,
	"ocr":                 true,
	"search":              true,
	"realtime":            true,
	"batch":               true,
}

// Capability enums.
var ValidCapabilities = map[string]bool{
	"chat":                true,
	"vision":              true,
	"audio_input":         true,
	"video_input":         true,
	"reasoning":           true,
	"tools":               true,
	"structured_outputs":  true,
	"function_calling":    true,
	"prompt_caching":      true,
	"computer_use":        true,
	"web_search":          true,
	"serverless":          true,
	"image_generation":    true,
	"embedding":           true,
	"rerank":              true,
	"audio_speech":        true,
	"audio_transcription": true,
	"video_generation":    true,
	"ocr":                 true,
	"search":              true,
	"realtime":            true,
}

// Supported parameter enums.
var ValidSupportedParameters = map[string]bool{
	"temperature":     true,
	"top_p":           true,
	"max_tokens":      true,
	"tools":           true,
	"tool_choice":     true,
	"response_format": true,
	"reasoning":       true,
	"image_input":     true,
	"video_input":     true,
	"audio_input":     true,
	"voice":           true,
	"speed":           true,
	"size":            true,
	"quality":         true,
	"style":           true,
}

// Limits key enums.
var ValidLimitKeys = map[string]bool{
	"context_window":    true,
	"max_input_tokens":  true,
	"max_output_tokens": true,
	"max_tokens":        true,
}

// Prices key enums.
var ValidPriceKeys = map[string]bool{
	"input_cost_per_token":                    true,
	"output_cost_per_token":                   true,
	"cache_read_input_token_cost":             true,
	"cache_creation_input_token_cost":         true,
	"input_cost_per_token_above_200k_tokens":  true,
	"output_cost_per_token_above_200k_tokens": true,
	"output_cost_per_image":                   true,
	"output_cost_per_pixel":                   true,
	"output_cost_per_image_low_quality":       true,
	"output_cost_per_image_high_quality":      true,
	"input_cost_per_audio_per_second":         true,
	"input_cost_per_video_per_second":         true,
	"output_cost_per_second":                  true,
	"input_cost_per_query":                    true,
	"search_context_cost_per_query":           true,
	"ocr_cost_per_page":                       true,
	"output_cost_per_character":               true,
	"output_cost_per_image_hd":                true,
	"output_cost_per_video":                   true,
	"output_cost_per_video_per_second":        true,
	// 对齐 BFE（BFE 已支持，api 缺失）
	"input_cost_per_image_token":  true,
	"input_cost_per_audio_token":  true,
	"output_cost_per_audio_token": true,
	// 新增：1h TTL 缓存写价
	"cache_creation_input_token_cost_1h": true,
	// 长度分档（硬编码；200k 档已有）
	"input_cost_per_token_above_256k_tokens":  true,
	"output_cost_per_token_above_256k_tokens": true,
	"input_cost_per_token_above_272k_tokens":  true,
	"output_cost_per_token_above_272k_tokens": true,
	"input_cost_per_token_above_512k_tokens":  true,
	"output_cost_per_token_above_512k_tokens": true,
}

// maxPriceFixedPoint is the largest price value (scaled by quota.RmbPrecision)
// that still keeps the downstream float64 cost computation
// (usage * (price * 1e8)) exactly representable: float64 integers stay exact
// up to 2^53.
const maxPriceFixedPoint = 1 << 53

// MaxModelPriceFieldLength bounds provider / model / base_model to the
// 1-255 character limit required by model-prices.md §1 and mirrored by the
// VARCHAR(255) columns of model_prices. The API layer must reject longer
// values with 422 instead of relying on the DB to error or truncate
// (issue #309). Length is counted in characters (runes), matching both the
// contract wording ("长度 1-255 字符") and MySQL VARCHAR(255) semantics.
const MaxModelPriceFieldLength = 255

// checkPricePrecision rejects prices whose scaled value reaches 2^53, where
// float64 can no longer represent the downstream cost computation exactly.
func checkPricePrecision(key string, v float64) error {
	if v*quota.RmbPrecision >= maxPriceFixedPoint {
		return xerror.WrapParamErrorWithMsg("price %s exceeds the maximum representable precision", key)
	}
	return nil
}

// Metadata key enums.
var ValidMetadataKeys = map[string]bool{
	"source": true,
	"notes":  true,
}

func validateLimitValue(key string, v interface{}) error {
	switch n := v.(type) {
	case int:
		if n < 0 {
			return xerror.WrapParamErrorWithMsg("limit %s must be >= 0", key)
		}
	case int8:
		if n < 0 {
			return xerror.WrapParamErrorWithMsg("limit %s must be >= 0", key)
		}
	case int16:
		if n < 0 {
			return xerror.WrapParamErrorWithMsg("limit %s must be >= 0", key)
		}
	case int32:
		if n < 0 {
			return xerror.WrapParamErrorWithMsg("limit %s must be >= 0", key)
		}
	case int64:
		if n < 0 {
			return xerror.WrapParamErrorWithMsg("limit %s must be >= 0", key)
		}
	case uint, uint8, uint16, uint32, uint64, uintptr:
		// non-negative integers by definition
	case float32:
		f := float64(n)
		if f < 0 || f != math.Trunc(f) {
			return xerror.WrapParamErrorWithMsg("limit %s must be a non-negative integer", key)
		}
	case float64:
		if n < 0 || n != math.Trunc(n) {
			return xerror.WrapParamErrorWithMsg("limit %s must be a non-negative integer", key)
		}
	default:
		return xerror.WrapParamErrorWithMsg("limit %s must be a non-negative integer", key)
	}
	return nil
}

// validatePriceField checks one of provider / model / base_model against the
// length limit. The non-empty check stays at the call site (TrimSpace-based,
// so a whitespace-only value is rejected before this runs).
func validatePriceField(name, s string) error {
	if utf8.RuneCountInString(s) > MaxModelPriceFieldLength {
		return xerror.WrapParamErrorWithMsg("%s length must be between 1 and %d characters", name, MaxModelPriceFieldLength)
	}
	return nil
}

// ValidateModelPrice validates a model price record.
func ValidateModelPrice(m *ModelPrice) error {
	if m == nil {
		return xerror.WrapParamErrorWithMsg("model price is required")
	}
	if strings.TrimSpace(m.Provider) == "" {
		return xerror.WrapParamErrorWithMsg("provider is required")
	}
	if err := validatePriceField("provider", m.Provider); err != nil {
		return err
	}
	if strings.TrimSpace(m.Model) == "" {
		return xerror.WrapParamErrorWithMsg("model is required")
	}
	if err := validatePriceField("model", m.Model); err != nil {
		return err
	}
	if strings.TrimSpace(m.BaseModel) == "" {
		return xerror.WrapParamErrorWithMsg("base_model is required")
	}
	if err := validatePriceField("base_model", m.BaseModel); err != nil {
		return err
	}
	if strings.TrimSpace(m.Mode) == "" {
		return xerror.WrapParamErrorWithMsg("mode is required")
	}
	if !ValidModes[m.Mode] {
		return xerror.WrapParamErrorWithMsg("invalid mode: %s", m.Mode)
	}

	if len(m.Prices) == 0 {
		return xerror.WrapParamErrorWithMsg("prices is required and must contain at least one price")
	}
	for k, v := range m.Prices {
		if !ValidPriceKeys[k] {
			return xerror.WrapParamErrorWithMsg("invalid price key: %s", k)
		}
		if v < 0 {
			return xerror.WrapParamErrorWithMsg("price %s must be >= 0", k)
		}
		if err := checkPricePrecision(k, v); err != nil {
			return err
		}
	}

	for tierName, tierPrices := range m.TierPrices {
		// 初期只支持 peak tier
		if tierName != "peak" {
			return xerror.WrapParamErrorWithMsg("invalid tier name: %s, only 'peak' is allowed", tierName)
		}
		if len(tierPrices) == 0 {
			return xerror.WrapParamErrorWithMsg("tier_prices.%s must contain at least one price", tierName)
		}
		for k, v := range tierPrices {
			if !ValidPriceKeys[k] {
				return xerror.WrapParamErrorWithMsg("invalid tier price key in tier %s: %s", tierName, k)
			}
			if v < 0 {
				return xerror.WrapParamErrorWithMsg("tier price %s in tier %s must be >= 0", k, tierName)
			}
			if err := checkPricePrecision(fmt.Sprintf("%s in tier %s", k, tierName), v); err != nil {
				return err
			}
		}
	}

	if m.BatchDiscount != nil {
		if *m.BatchDiscount <= 0 || *m.BatchDiscount > 1 {
			return xerror.WrapParamErrorWithMsg("batch_discount must be in (0, 1]")
		}
	}

	if m.PriceCurrency != "" && m.PriceCurrency != "RMB" {
		return xerror.WrapParamErrorWithMsg("price_currency must be RMB")
	}

	for _, c := range m.Capabilities {
		if !ValidCapabilities[c] {
			return xerror.WrapParamErrorWithMsg("invalid capability: %s", c)
		}
	}
	for _, p := range m.SupportedParameters {
		if !ValidSupportedParameters[p] {
			return xerror.WrapParamErrorWithMsg("invalid supported_parameter: %s", p)
		}
	}

	for k, v := range m.Limits {
		if !ValidLimitKeys[k] {
			return xerror.WrapParamErrorWithMsg("invalid limit key: %s", k)
		}
		if err := validateLimitValue(k, v); err != nil {
			return err
		}
	}

	for k := range m.Metadata {
		if !ValidMetadataKeys[k] {
			return xerror.WrapParamErrorWithMsg("invalid metadata key: %s", k)
		}
	}

	return nil
}

// ValidateImportFile validates the top-level fields of a model-list.yaml import.
func ValidateImportFile(file *ModelListFile) error {
	if file == nil {
		return xerror.WrapParamErrorWithMsg("import file is empty")
	}
	if strings.TrimSpace(file.Version) == "" {
		return xerror.WrapParamErrorWithMsg("version is required")
	}
	if file.Version != "v1.0" {
		return xerror.WrapParamErrorWithMsg("unsupported version: %s", file.Version)
	}
	if strings.TrimSpace(file.DefaultCurrency) == "" {
		return xerror.WrapParamErrorWithMsg("default_currency is required")
	}
	if file.DefaultCurrency != "RMB" {
		return xerror.WrapParamErrorWithMsg("default_currency must be RMB")
	}

	seen := map[string]bool{}
	for i, m := range file.Models {
		if err := ValidateModelPrice(m); err != nil {
			return xerror.WrapParamErrorWithMsg("models[%d]: %v", i, err)
		}
		if m.PriceCurrency == "" {
			m.PriceCurrency = file.DefaultCurrency
		}
		if seen[m.Key()] {
			return xerror.WrapParamErrorWithMsg("duplicate (provider, model, mode): %s", m.Key())
		}
		seen[m.Key()] = true
	}

	return nil
}

// StringSliceContains reports whether a string slice contains a value.
func StringSliceContains(slice []string, value string) bool {
	for _, s := range slice {
		if s == value {
			return true
		}
	}
	return false
}

// NormalizeCurrency fills empty PriceCurrency with the default currency.
func NormalizeCurrency(m *ModelPrice, defaultCurrency string) {
	if m == nil || strings.TrimSpace(m.PriceCurrency) != "" {
		return
	}
	m.PriceCurrency = defaultCurrency
}

// GroupByProvider groups model prices by provider.
func GroupByProvider(list []*ModelPrice) map[string][]*ModelPrice {
	rst := map[string][]*ModelPrice{}
	for _, m := range list {
		if m == nil {
			continue
		}
		rst[m.Provider] = append(rst[m.Provider], m)
	}
	return rst
}

// ErrorAtIndex wraps an error with a model index in an import file.
func ErrorAtIndex(i int, err error) error {
	return fmt.Errorf("models[%d]: %w", i, err)
}
