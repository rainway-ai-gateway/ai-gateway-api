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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
)

func validModelPrice() *ModelPrice {
	return &ModelPrice{
		Provider:  "openai",
		Model:     "gpt-4",
		BaseModel: "gpt-4",
		Mode:      "chat",
		Prices: PriceMap{
			"input_cost_per_token":  0.001,
			"output_cost_per_token": 0.002,
		},
		PriceCurrency: "RMB",
	}
}

func TestValidateModelPrice(t *testing.T) {
	cases := []struct {
		name    string
		param   *ModelPrice
		wantErr bool
	}{
		{"valid", validModelPrice(), false},
		{"nil", nil, true},
		{"missing provider", func() *ModelPrice { p := validModelPrice(); p.Provider = ""; return p }(), true},
		{"missing model", func() *ModelPrice { p := validModelPrice(); p.Model = ""; return p }(), true},
		{"missing base_model", func() *ModelPrice { p := validModelPrice(); p.BaseModel = ""; return p }(), true},
		{"missing mode", func() *ModelPrice { p := validModelPrice(); p.Mode = ""; return p }(), true},
		{"invalid mode", func() *ModelPrice { p := validModelPrice(); p.Mode = "unknown"; return p }(), true},
		{"missing prices", func() *ModelPrice { p := validModelPrice(); p.Prices = nil; return p }(), true},
		{"empty prices", func() *ModelPrice { p := validModelPrice(); p.Prices = PriceMap{}; return p }(), true},
		{"invalid price key", func() *ModelPrice { p := validModelPrice(); p.Prices = PriceMap{"invalid_key": 1}; return p }(), true},
		{"negative price", func() *ModelPrice { p := validModelPrice(); p.Prices["input_cost_per_token"] = -1; return p }(), true},
		{"invalid currency", func() *ModelPrice { p := validModelPrice(); p.PriceCurrency = "USD"; return p }(), true},
		{"valid empty currency", func() *ModelPrice { p := validModelPrice(); p.PriceCurrency = ""; return p }(), false},
		{"invalid capability", func() *ModelPrice { p := validModelPrice(); p.Capabilities = []string{"unknown"}; return p }(), true},
		{"invalid supported parameter", func() *ModelPrice { p := validModelPrice(); p.SupportedParameters = []string{"unknown"}; return p }(), true},
		{"invalid limit key", func() *ModelPrice { p := validModelPrice(); p.Limits = map[string]interface{}{"unknown": 1}; return p }(), true},
		{"negative limit int", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": -1}
			return p
		}(), true},
		{"negative limit float64", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": float64(-1)}
			return p
		}(), true},
		{"non-integer limit", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": 1.5}
			return p
		}(), true},
		{"non-numeric limit", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": "abc"}
			return p
		}(), true},
		{"valid zero limit", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": 0}
			return p
		}(), false},
		{"valid positive limit", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": 128000}
			return p
		}(), false},
		{"valid positive limit float64", func() *ModelPrice {
			p := validModelPrice()
			p.Limits = map[string]interface{}{"context_window": float64(128000)}
			return p
		}(), false},
		{"invalid metadata key", func() *ModelPrice {
			p := validModelPrice()
			p.Metadata = map[string]interface{}{"unknown": 1}
			return p
		}(), true},
		{"valid tier prices", func() *ModelPrice {
			p := validModelPrice()
			p.TierPrices = TierPriceMap{
				"peak": {"input_cost_per_token": 0.002},
			}
			return p
		}(), false},
		{"invalid tier name", func() *ModelPrice {
			p := validModelPrice()
			p.TierPrices = TierPriceMap{
				"off_peak": {"input_cost_per_token": 0.002},
			}
			return p
		}(), true},
		{"invalid tier price key", func() *ModelPrice {
			p := validModelPrice()
			p.TierPrices = TierPriceMap{
				"peak": {"invalid_key": 0.002},
			}
			return p
		}(), true},
		{"negative tier price", func() *ModelPrice {
			p := validModelPrice()
			p.TierPrices = TierPriceMap{
				"peak": {"input_cost_per_token": -0.002},
			}
			return p
		}(), true},
		{"excessive price precision", func() *ModelPrice {
			p := validModelPrice()
			p.Prices["input_cost_per_token"] = (1 << 53) / 1e8
			return p
		}(), true},
		{"excessive tier price precision", func() *ModelPrice {
			p := validModelPrice()
			p.TierPrices = TierPriceMap{
				"peak": {"input_cost_per_token": (1 << 53) / 1e8},
			}
			return p
		}(), true},
		{"valid high precision price", func() *ModelPrice {
			p := validModelPrice()
			p.Prices["input_cost_per_token"] = 7.6234102728e-08
			return p
		}(), false},
		{"empty tier prices", func() *ModelPrice {
			p := validModelPrice()
			p.TierPrices = TierPriceMap{
				"peak": {},
			}
			return p
		}(), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateModelPrice(tc.param)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateModelPriceNewPriceKeys verifies the price keys newly added to
// ValidPriceKeys pass validation (including checkPricePrecision on
// normal-range values), while unknown length-tier keys are still rejected.
func TestValidateModelPriceNewPriceKeys(t *testing.T) {
	newKeys := []string{
		"input_cost_per_image_token",
		"input_cost_per_audio_token",
		"output_cost_per_audio_token",
		"cache_creation_input_token_cost_1h",
		"input_cost_per_token_above_256k_tokens",
		"output_cost_per_token_above_256k_tokens",
		"input_cost_per_token_above_272k_tokens",
		"output_cost_per_token_above_272k_tokens",
		"input_cost_per_token_above_512k_tokens",
		"output_cost_per_token_above_512k_tokens",
	}
	for _, key := range newKeys {
		t.Run("valid "+key, func(t *testing.T) {
			p := validModelPrice()
			p.Prices = PriceMap{key: 0.0001}
			assert.NoError(t, ValidateModelPrice(p))

			p.TierPrices = TierPriceMap{"peak": {key: 0.0001}}
			assert.NoError(t, ValidateModelPrice(p))
		})
	}

	t.Run("invalid unknown length tier key", func(t *testing.T) {
		p := validModelPrice()
		p.Prices = PriceMap{"input_cost_per_token_above_999k_tokens": 0.0001}
		assert.Error(t, ValidateModelPrice(p))
	})
}

func TestValidateImportFile(t *testing.T) {
	base := func() *ModelListFile {
		return &ModelListFile{
			Version:         "v1.0",
			DefaultCurrency: "RMB",
			Models:          []*ModelPrice{validModelPrice()},
		}
	}

	cases := []struct {
		name    string
		file    *ModelListFile
		wantErr bool
	}{
		{"valid", base(), false},
		{"nil", nil, true},
		{"empty version", func() *ModelListFile { f := base(); f.Version = ""; return f }(), true},
		{"unsupported version", func() *ModelListFile { f := base(); f.Version = "v2.0"; return f }(), true},
		{"empty default_currency", func() *ModelListFile { f := base(); f.DefaultCurrency = ""; return f }(), true},
		{"invalid default_currency", func() *ModelListFile { f := base(); f.DefaultCurrency = "USD"; return f }(), true},
		{"invalid model", func() *ModelListFile { f := base(); f.Models[0].Mode = "bad"; return f }(), true},
		{"duplicate model", func() *ModelListFile {
			f := base()
			f.Models = append(f.Models, validModelPrice())
			return f
		}(), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateImportFile(tc.file)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStringSliceContains(t *testing.T) {
	assert.True(t, StringSliceContains([]string{"a", "b"}, "b"))
	assert.False(t, StringSliceContains([]string{"a", "b"}, "c"))
}

func TestNormalizeCurrency(t *testing.T) {
	m := validModelPrice()
	m.PriceCurrency = ""
	NormalizeCurrency(m, "RMB")
	assert.Equal(t, "RMB", m.PriceCurrency)

	// Already set should not be overwritten.
	m.PriceCurrency = "RMB"
	NormalizeCurrency(m, "USD")
	assert.Equal(t, "RMB", m.PriceCurrency)

	// Nil should not panic.
	NormalizeCurrency(nil, "RMB")
}

func TestGroupByProvider(t *testing.T) {
	a := &ModelPrice{Provider: "p1", Model: "m1", BaseModel: "m1", Mode: "chat", Prices: PriceMap{"input_cost_per_token": 1}}
	b := &ModelPrice{Provider: "p2", Model: "m2", BaseModel: "m2", Mode: "chat", Prices: PriceMap{"input_cost_per_token": 1}}
	c := &ModelPrice{Provider: "p1", Model: "m3", BaseModel: "m3", Mode: "chat", Prices: PriceMap{"input_cost_per_token": 1}}

	groups := GroupByProvider([]*ModelPrice{a, b, c})
	assert.Len(t, groups["p1"], 2)
	assert.Len(t, groups["p2"], 1)

	// Nil entries are skipped.
	groups = GroupByProvider([]*ModelPrice{nil, a})
	assert.Len(t, groups["p1"], 1)
}

func TestErrorAtIndex(t *testing.T) {
	err := ErrorAtIndex(3, assert.AnError)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "models[3]:")
}

func TestValidateModelPrice_BatchDiscount(t *testing.T) {
	cases := []struct {
		name     string
		discount *float64
		wantErr  bool
	}{
		{"nil discount", nil, false},
		{"zero", lib.PFloat64(0), true},
		{"negative", lib.PFloat64(-0.5), true},
		{"greater than 1", lib.PFloat64(1.5), true},
		{"one", lib.PFloat64(1), false},
		{"half", lib.PFloat64(0.5), false},
		{"tiny", lib.PFloat64(0.0001), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validModelPrice()
			p.BatchDiscount = c.discount
			err := ValidateModelPrice(p)
			if c.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "batch_discount")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateModelPrice_BatchMode(t *testing.T) {
	p := validModelPrice()
	p.Mode = "batch"
	assert.NoError(t, ValidateModelPrice(p))

	// file mode stays out of the price system.
	p.Mode = "file"
	assert.Error(t, ValidateModelPrice(p))
}

func TestValidateModelPrice_FieldLength(t *testing.T) {
	longASCII := strings.Repeat("a", MaxModelPriceFieldLength+1)

	cases := []struct {
		name    string
		param   *ModelPrice
		wantErr bool
	}{
		{
			name: "all fields at 255 boundary",
			param: func() *ModelPrice {
				p := validModelPrice()
				p.Provider = strings.Repeat("a", MaxModelPriceFieldLength)
				p.Model = strings.Repeat("b", MaxModelPriceFieldLength)
				p.BaseModel = strings.Repeat("c", MaxModelPriceFieldLength)
				return p
			}(),
			wantErr: false,
		},
		{
			name: "provider 256",
			param: func() *ModelPrice {
				p := validModelPrice()
				p.Provider = longASCII
				return p
			}(),
			wantErr: true,
		},
		{
			name: "model 256",
			param: func() *ModelPrice {
				p := validModelPrice()
				p.Model = longASCII
				return p
			}(),
			wantErr: true,
		},
		{
			name: "base_model 256",
			param: func() *ModelPrice {
				p := validModelPrice()
				p.BaseModel = longASCII
				return p
			}(),
			wantErr: true,
		},
		{
			// Length is counted in characters (runes): 255 CJK characters
			// are within the limit even though they exceed 255 bytes
			// (issue #309, matching VARCHAR(255) character semantics).
			name: "255 CJK characters accepted",
			param: func() *ModelPrice {
				p := validModelPrice()
				p.Model = strings.Repeat("模", MaxModelPriceFieldLength)
				return p
			}(),
			wantErr: false,
		},
		{
			name: "whitespace-only provider still rejected by non-empty check",
			param: func() *ModelPrice {
				p := validModelPrice()
				p.Provider = " "
				return p
			}(),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateModelPrice(tc.param)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	// Error messages name the offending field.
	for field, mutate := range map[string]func(*ModelPrice){
		"provider":  func(p *ModelPrice) { p.Provider = longASCII },
		"model":     func(p *ModelPrice) { p.Model = longASCII },
		"base_model": func(p *ModelPrice) { p.BaseModel = longASCII },
	} {
		p := validModelPrice()
		mutate(p)
		err := ValidateModelPrice(p)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), field)
	}
}

func TestValidateImportFile_FieldLength(t *testing.T) {
	file := &ModelListFile{
		Version:         "v1.0",
		DefaultCurrency: "RMB",
		Models: []*ModelPrice{func() *ModelPrice {
			p := validModelPrice()
			p.Model = strings.Repeat("a", MaxModelPriceFieldLength+1)
			return p
		}()},
	}
	err := ValidateImportFile(file)
	assert.Error(t, err)
	// The import path wraps per-record errors with the record index.
	assert.Contains(t, err.Error(), "models[0]")
	assert.Contains(t, err.Error(), "model")
}
