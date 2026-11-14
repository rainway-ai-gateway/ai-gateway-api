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

package icluster_conf

import (
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEppRuntime(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		for _, raw := range []string{"", "   ", "\t\n"} {
			conf, err := ParseEppRuntime(raw)
			require.NoError(t, err, "raw=%q", raw)
			assert.Nil(t, conf, "raw=%q", raw)
		}
	})

	t.Run("valid config parses", func(t *testing.T) {
		conf, err := ParseEppRuntime(`{"check":{"check_interval":"2s","fail_threshold":3},"timeout":{"call":"3s"}}`)
		require.NoError(t, err)
		require.NotNil(t, conf)
		require.NotNil(t, conf.Check)
		require.NotNil(t, conf.Check.CheckInterval)
		assert.Equal(t, "2s", *conf.Check.CheckInterval)
		require.NotNil(t, conf.Check.FailThreshold)
		assert.Equal(t, 3, *conf.Check.FailThreshold)
		require.NotNil(t, conf.Timeout)
		require.NotNil(t, conf.Timeout.Call)
		assert.Equal(t, "3s", *conf.Timeout.Call)
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		for _, raw := range []string{
			`{"tls":{"plaintext":true}}`,
			`{"breaker":{"disabled":true}}`,
			`{"check":{"bogus":1}}`,
			`{"unknown_top":true}`,
		} {
			conf, err := ParseEppRuntime(raw)
			require.Error(t, err, "raw=%s", raw)
			assert.Nil(t, conf, "raw=%s", raw)
			assert.Contains(t, err.Error(), "epp_runtime is invalid", "raw=%s", raw)
		}
	})

	t.Run("malformed JSON is rejected", func(t *testing.T) {
		conf, err := ParseEppRuntime("{")
		require.Error(t, err)
		assert.Nil(t, conf)
	})
}

func TestEppRuntimeParam_Validate(t *testing.T) {
	t.Run("nil is valid", func(t *testing.T) {
		var p *EppRuntimeParam
		require.NoError(t, p.Validate())
	})

	t.Run("empty struct is valid", func(t *testing.T) {
		require.NoError(t, (&EppRuntimeParam{}).Validate())
	})

	t.Run("invalid check durations", func(t *testing.T) {
		for _, v := range []string{"abc", "0s", "-1s", ""} {
			err := (&EppRuntimeParam{Check: &EppCheckParam{CheckInterval: lib.PString(v)}}).Validate()
			require.Error(t, err, "check_interval=%q", v)
			assert.Contains(t, err.Error(), "epp_runtime.check.check_interval")
		}
		for _, v := range []string{"abc", "0s", "-1s"} {
			err := (&EppRuntimeParam{Check: &EppCheckParam{Cooldown: lib.PString(v)}}).Validate()
			require.Error(t, err, "cooldown=%q", v)
			assert.Contains(t, err.Error(), "epp_runtime.check.cooldown")
		}
	})

	t.Run("invalid timeouts", func(t *testing.T) {
		for _, v := range []string{"abc", "0s", "-1s"} {
			err := (&EppRuntimeParam{Timeout: &EppTimeoutParam{Connect: lib.PString(v)}}).Validate()
			require.Error(t, err, "connect=%q", v)
			assert.Contains(t, err.Error(), "epp_runtime.timeout.connect")

			err = (&EppRuntimeParam{Timeout: &EppTimeoutParam{Call: lib.PString(v)}}).Validate()
			require.Error(t, err, "call=%q", v)
			assert.Contains(t, err.Error(), "epp_runtime.timeout.call")
		}
	})

	t.Run("thresholds", func(t *testing.T) {
		for _, v := range []int{0, -1} {
			err := (&EppRuntimeParam{Check: &EppCheckParam{FailThreshold: lib.PInt(v)}}).Validate()
			require.Error(t, err, "fail_threshold=%d", v)
			assert.Contains(t, err.Error(), "epp_runtime.check.fail_threshold")

			err = (&EppRuntimeParam{Check: &EppCheckParam{SuccessThreshold: lib.PInt(v)}}).Validate()
			require.Error(t, err, "success_threshold=%d", v)
			assert.Contains(t, err.Error(), "epp_runtime.check.success_threshold")
		}

		// Boundary: 1 is legal.
		require.NoError(t, (&EppRuntimeParam{Check: &EppCheckParam{
			FailThreshold:    lib.PInt(1),
			SuccessThreshold: lib.PInt(1),
		}}).Validate())
	})

	t.Run("fully valid config", func(t *testing.T) {
		p := &EppRuntimeParam{
			Check: &EppCheckParam{
				Disabled:         lib.PBool(false),
				CheckInterval:    lib.PString("2s"),
				FailThreshold:    lib.PInt(3),
				Cooldown:         lib.PString("45s"),
				SuccessThreshold: lib.PInt(2),
			},
			Timeout: &EppTimeoutParam{Connect: lib.PString("500ms"), Call: lib.PString("3s")},
		}
		require.NoError(t, p.Validate())
	})
}

func TestEppRuntimeParam_ApplyDefaults(t *testing.T) {
	t.Run("nil receiver is a no-op", func(t *testing.T) {
		var p *EppRuntimeParam
		p.ApplyDefaults() // must not panic
	})

	t.Run("empty check object fills all defaults", func(t *testing.T) {
		p := &EppRuntimeParam{Check: &EppCheckParam{}}
		p.ApplyDefaults()
		require.NotNil(t, p.Check)
		assert.Equal(t, false, *p.Check.Disabled)
		assert.Equal(t, DefaultEppCheckInterval, *p.Check.CheckInterval)
		assert.Equal(t, DefaultEppFailThreshold, *p.Check.FailThreshold)
		assert.Equal(t, DefaultEppCooldown, *p.Check.Cooldown)
		assert.Equal(t, DefaultEppSuccessThreshold, *p.Check.SuccessThreshold)
		// timeout object was nil: not synthesized.
		assert.Nil(t, p.Timeout)
	})

	t.Run("empty timeout object fills all defaults", func(t *testing.T) {
		p := &EppRuntimeParam{Timeout: &EppTimeoutParam{}}
		p.ApplyDefaults()
		require.NotNil(t, p.Timeout)
		assert.Equal(t, DefaultEppConnectTimeout, *p.Timeout.Connect)
		assert.Equal(t, DefaultEppCallTimeout, *p.Timeout.Call)
		assert.Nil(t, p.Check)
	})

	t.Run("provided values are kept", func(t *testing.T) {
		p := &EppRuntimeParam{Check: &EppCheckParam{CheckInterval: lib.PString("5s")}}
		p.ApplyDefaults()
		assert.Equal(t, "5s", *p.Check.CheckInterval)
		assert.Equal(t, DefaultEppFailThreshold, *p.Check.FailThreshold)
		assert.Equal(t, DefaultEppCooldown, *p.Check.Cooldown)
		assert.Equal(t, DefaultEppSuccessThreshold, *p.Check.SuccessThreshold)
	})

	t.Run("both objects nil stay nil", func(t *testing.T) {
		p := &EppRuntimeParam{}
		p.ApplyDefaults()
		assert.Nil(t, p.Check)
		assert.Nil(t, p.Timeout)
	})
}

func TestValidateClusterEppRuntime(t *testing.T) {
	t.Run("nil is not carried", func(t *testing.T) {
		got, err := validateClusterEppRuntime(nil)
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("empty/null carried is treated as not carried", func(t *testing.T) {
		for _, raw := range []string{"", "  ", "null"} {
			got, err := validateClusterEppRuntime(lib.PString(raw))
			require.NoError(t, err, "raw=%q", raw)
			assert.Nil(t, got, "raw=%q", raw)
		}
	})

	t.Run("defaults are applied and normalized", func(t *testing.T) {
		got, err := validateClusterEppRuntime(lib.PString(`{"timeout":{"connect":"1s"}}`))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.JSONEq(t, `{"timeout":{"connect":"1s","call":"11s"}}`, *got)
	})

	t.Run("full config normalized", func(t *testing.T) {
		got, err := validateClusterEppRuntime(lib.PString(`{"check":{"fail_threshold":5},"timeout":{}}`))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.JSONEq(t, `{
			"check":{"disabled":false,"check_interval":"2s","fail_threshold":5,"cooldown":"45s","success_threshold":2},
			"timeout":{"connect":"500ms","call":"11s"}
		}`, *got)
	})

	t.Run("invalid config rejected", func(t *testing.T) {
		_, err := validateClusterEppRuntime(lib.PString(`{"check":{"fail_threshold":0}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fail_threshold")

		_, err = validateClusterEppRuntime(lib.PString(`{"tls":{"plaintext":true}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "epp_runtime is invalid")
	})
}
