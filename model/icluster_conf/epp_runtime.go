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
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// Default EPP runtime parameter values, aligned with BFE defaults
// (bfe/bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go). They are
// filled on write so GET round-trips the effective values; BFE applies the same
// defaults at load time for fields that are not exported.
const (
	DefaultEppCheckInterval    = "2s"
	DefaultEppFailThreshold    = 3
	DefaultEppCooldown         = "45s"
	DefaultEppSuccessThreshold = 2
	DefaultEppConnectTimeout   = "500ms"
	DefaultEppCallTimeout      = "11s"
)

// EppRuntimeParam is the control-plane shape of the BFE-side EPP runtime
// parameters, stored as raw JSON (mirroring epp_config). Only Check/Timeout are
// exposed; TLS (EPPTLS) and Breaker (EPPBreaker) are fixed on export
// (design-docs/modifications/2026-10-12-epp-runtime-params/design-changes.md §5).
type EppRuntimeParam struct {
	// TLS is not configurable in this phase: export fixes EPPTLS.Plaintext=true.
	//TLS *EppTLSParam `json:"tls,omitempty"`
	Check   *EppCheckParam   `json:"check,omitempty"`
	Timeout *EppTimeoutParam `json:"timeout,omitempty"`
	// Breaker is not exposed in this phase: export fixes EPPBreaker.Disabled=true.
	//Breaker *EppBreakerParam `json:"breaker,omitempty"`
}

// EppCheckParam mirrors BFE EPPCheckConf (health check and failover hysteresis).
type EppCheckParam struct {
	Disabled         *bool   `json:"disabled,omitempty"`
	CheckInterval    *string `json:"check_interval,omitempty"`
	FailThreshold    *int    `json:"fail_threshold,omitempty"`
	Cooldown         *string `json:"cooldown,omitempty"`
	SuccessThreshold *int    `json:"success_threshold,omitempty"`
}

// EppTimeoutParam mirrors BFE EPPTimeoutConf (EPP call timeouts).
type EppTimeoutParam struct {
	Connect *string `json:"connect,omitempty"`
	Call    *string `json:"call,omitempty"`
}

// ParseEppRuntime decodes the raw stored JSON of epp_runtime with strict field
// checking. Empty input returns nil (no config). Unknown fields (e.g. tls and
// breaker, which are not exposed) are rejected.
func ParseEppRuntime(raw string) (*EppRuntimeParam, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()

	conf := &EppRuntimeParam{}
	if err := decoder.Decode(conf); err != nil {
		return nil, xerror.WrapParamErrorWithMsg("epp_runtime is invalid: %s", err.Error())
	}

	return conf, nil
}

// ApplyDefaults fills the unset sub-fields of check/timeout with the BFE-aligned
// defaults. Nil check/timeout objects are left nil (not synthesized): an unset
// object is not exported and BFE applies the same defaults at load time.
func (p *EppRuntimeParam) ApplyDefaults() {
	if p == nil {
		return
	}

	if c := p.Check; c != nil {
		if c.Disabled == nil {
			c.Disabled = lib.PBool(false)
		}
		if c.CheckInterval == nil {
			c.CheckInterval = lib.PString(DefaultEppCheckInterval)
		}
		if c.FailThreshold == nil {
			c.FailThreshold = lib.PInt(DefaultEppFailThreshold)
		}
		if c.Cooldown == nil {
			c.Cooldown = lib.PString(DefaultEppCooldown)
		}
		if c.SuccessThreshold == nil {
			c.SuccessThreshold = lib.PInt(DefaultEppSuccessThreshold)
		}
	}

	if t := p.Timeout; t != nil {
		if t.Connect == nil {
			t.Connect = lib.PString(DefaultEppConnectTimeout)
		}
		if t.Call == nil {
			t.Call = lib.PString(DefaultEppCallTimeout)
		}
	}
}

// Validate checks the field-level rules of epp_runtime, mirroring BFE
// EPPCheckConfCheck / EPPTimeoutConfCheck. It must be enforced whenever
// epp_runtime is non-empty, regardless of balance_mode.
func (p *EppRuntimeParam) Validate() error {
	if p == nil {
		return nil
	}

	if c := p.Check; c != nil {
		if c.CheckInterval != nil && !isPositiveDuration(*c.CheckInterval) {
			return xerror.WrapParamErrorWithMsg("epp_runtime.check.check_interval %q is not a valid positive duration", *c.CheckInterval)
		}
		if c.Cooldown != nil && !isPositiveDuration(*c.Cooldown) {
			return xerror.WrapParamErrorWithMsg("epp_runtime.check.cooldown %q is not a valid positive duration", *c.Cooldown)
		}
		if c.FailThreshold != nil && *c.FailThreshold < 1 {
			return xerror.WrapParamErrorWithMsg("epp_runtime.check.fail_threshold must be >= 1")
		}
		if c.SuccessThreshold != nil && *c.SuccessThreshold < 1 {
			return xerror.WrapParamErrorWithMsg("epp_runtime.check.success_threshold must be >= 1")
		}
	}

	if t := p.Timeout; t != nil {
		if t.Connect != nil && !isPositiveDuration(*t.Connect) {
			return xerror.WrapParamErrorWithMsg("epp_runtime.timeout.connect %q is not a valid positive duration", *t.Connect)
		}
		if t.Call != nil && !isPositiveDuration(*t.Call) {
			return xerror.WrapParamErrorWithMsg("epp_runtime.timeout.call %q is not a valid positive duration", *t.Call)
		}
	}

	return nil
}

// isPositiveDuration reports whether s is a valid Go duration > 0.
func isPositiveDuration(s string) bool {
	d, err := time.ParseDuration(s)
	return err == nil && d > 0
}

// boolPtrValue dereferences p, defaulting to false when nil. BFE EPPCheckConf
// carries Disabled as a plain bool, so the exported value must be resolved.
func boolPtrValue(p *bool) bool {
	if p == nil {
		return false
	}
	return *p
}

// validateClusterEppRuntime parses, validates, applies defaults and re-marshals
// the raw epp_runtime JSON carried by a cluster write. nil input returns nil
// (field not carried, stored value retained on update); an explicit empty/null
// carried value is treated the same as not carried.
func validateClusterEppRuntime(runtime *string) (*string, error) {
	if runtime == nil {
		return nil, nil
	}

	conf, err := ParseEppRuntime(*runtime)
	if err != nil {
		return nil, err
	}
	if conf == nil {
		return nil, nil
	}

	if err := conf.Validate(); err != nil {
		return nil, err
	}
	conf.ApplyDefaults()

	normalized, err := json.Marshal(conf)
	if err != nil {
		return nil, xerror.WrapParamErrorWithMsg("epp_runtime is invalid: %s", err.Error())
	}
	s := string(normalized)

	return &s, nil
}
