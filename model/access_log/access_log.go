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

// Package access_log manages the access log dynamic configuration singleton
// (request/response header whitelists), the data source of the BFE
// mod_access_pb3 data file mod_access_pb.data (bfenetworks/bfe#1506).
package access_log

import (
	"context"
	"net/textproto"
	"strings"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// Bounds mirrored from the BFE data plane (bfe_modules/mod_access_pb3/
// mod_access_pb_data.go). The control plane validates with exactly the same
// rules so an accepted config never fails the BFE-side load; relaxing a bound
// only needs a control-plane constant change, never a BFE change.
const (
	// maxHeaderNameLen is the control-plane cap of a whitelisted header
	// name, aligned with the downstream Doris key column width.
	maxHeaderNameLen = 128

	// AccessLogConfFixedID is the fixed primary key of the singleton row
	// (single-row overwrite storage).
	AccessLogConfFixedID = int64(1)
)

// Sensitive header names rejected by the control plane (case-insensitive).
// The BFE data plane deliberately does NOT restrict whitelist content; the
// restriction lives only here. Raw API Keys are still masked at runtime by
// the BFE maskSensitiveCredentials gate (bfenetworks/bfe#1357).
var sensitiveHeaderNames = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
}

// Cookie-like headers allowed but worth an audit warning (session credentials
// may leak into access logs).
var cookieHeaderNames = map[string]bool{
	"Cookie":     true,
	"Set-Cookie": true,
}

// AccessLogConf is the storage-level access log configuration: a global
// singleton persisted as a single overwritten row (fixed id, no history
// versions). Header lists are stored as JSON text in canonical form.
type AccessLogConf struct {
	Id         int64
	ReqHeaders string
	ResHeaders string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// AccessLogConfigParam is the Open API representation of the singleton.
// A nil or empty list means "collect nothing" (the BFE default).
type AccessLogConfigParam struct {
	ReqHeaders []string `json:"req_headers"`
	ResHeaders []string `json:"res_headers"`
}

// AccessLogConfStorager defines storage operations for the access log config
// singleton. Upsert is executed inside a transaction provided by
// itxn.TxnStorager.
type AccessLogConfStorager interface {
	// Upsert overwrites the singleton row (insert when absent, overwrite
	// when present; fixed id=1).
	Upsert(ctx context.Context, config *AccessLogConf) error
	// Fetch reads the singleton row; returns nil, nil when never configured
	// (the unconfigured state is not an error and exports as empty whitelists).
	Fetch(ctx context.Context) (*AccessLogConf, error)
}

// ValidateAccessLogConfig validates the whole document: every header name
// must be a valid RFC 7230 token name within the byte cap, and is
// canonicalized (http.CanonicalHeaderKey) so downstream matching is
// case-insensitive by construction and the logged proto key is stable for
// Doris aggregation. Sensitive headers (Authorization/Proxy-Authorization)
// are rejected here on purpose: whitelist content is NOT restricted on the
// BFE data plane, so the control plane is the single enforcement point.
// Duplicate names after canonicalization are rejected (fail-fast).
func ValidateAccessLogConfig(param *AccessLogConfigParam) (reqCanonical, resCanonical []string, err error) {
	if param == nil {
		return nil, nil, xerror.WrapParamErrorWithMsg("access log config is required")
	}

	reqCanonical, err = normalizeHeaderNames(param.ReqHeaders, "req_headers")
	if err != nil {
		return nil, nil, err
	}
	resCanonical, err = normalizeHeaderNames(param.ResHeaders, "res_headers")
	if err != nil {
		return nil, nil, err
	}

	return reqCanonical, resCanonical, nil
}

// ContainsCookieHeader reports whether the canonical whitelists include a
// cookie-like header; the manager logs an audit warning in that case.
func ContainsCookieHeader(reqCanonical, resCanonical []string) bool {
	for _, name := range reqCanonical {
		if cookieHeaderNames[name] {
			return true
		}
	}
	for _, name := range resCanonical {
		if cookieHeaderNames[name] {
			return true
		}
	}
	return false
}

// normalizeHeaderNames validates and canonicalizes one whitelist. It mirrors
// the BFE normalizeHeaderNames (mod_access_pb_data.go) rule by rule; the
// sensitive-header rejection is control-plane-only.
func normalizeHeaderNames(names []string, field string) ([]string, error) {
	if names == nil {
		return []string{}, nil
	}

	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return nil, xerror.WrapParamErrorWithMsg("%s contains an empty header name", field)
		}
		if len(trimmed) > maxHeaderNameLen {
			return nil, xerror.WrapParamErrorWithMsg("%s header name %q exceeds %d bytes",
				field, trimmed, maxHeaderNameLen)
		}
		if !isValidHeaderName(trimmed) {
			return nil, xerror.WrapParamErrorWithMsg(
				"%s header name %q is invalid (allowed: RFC 7230 token chars)", field, trimmed)
		}

		canonical := textproto.CanonicalMIMEHeaderKey(trimmed)
		if sensitiveHeaderNames[canonical] {
			return nil, xerror.WrapParamErrorWithMsg(
				"%s must not contain sensitive header %q (credential collection is rejected)",
				field, canonical)
		}
		if seen[canonical] {
			return nil, xerror.WrapParamErrorWithMsg(
				"%s contains duplicate header name %q after case-insensitive normalization",
				field, canonical)
		}
		seen[canonical] = true
		out = append(out, canonical)
	}
	return out, nil
}

// isValidHeaderName reports whether s consists of RFC 7230 token characters
// (the legal set for HTTP header field names). Mirrors the BFE isTokenChar.
func isValidHeaderName(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return false
		}
	}
	return true
}

func isTokenChar(c byte) bool {
	if c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}
