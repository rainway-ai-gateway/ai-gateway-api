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

package access_log

import (
	"net/textproto"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// canonKey centralizes the expected canonical form so the table states
// intent, not a hand-copied transformation.
func canonKey(s string) string {
	return textproto.CanonicalMIMEHeaderKey(s)
}

// TestValidateAccessLogConfig mirrors the BFE-side TestNormalizeHeaderNames
// (bfe bfe_modules/mod_access_pb3/mod_access_pb_data_test.go) so the two
// validation implementations cannot drift apart, plus the control-plane-only
// sensitive-header rejection.
func TestValidateAccessLogConfig(t *testing.T) {
	cases := []struct {
		name    string
		param   *AccessLogConfigParam
		wantReq []string
		wantRes []string
		wantErr bool
	}{
		{name: "nil document", param: nil, wantErr: true},
		{name: "nil lists become empty", param: &AccessLogConfigParam{}, wantReq: []string{}, wantRes: []string{}},
		{name: "canonicalize",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"business_type", "X-REQUEST-ID"}, ResHeaders: []string{"content-TYPE"}},
			wantReq: []string{"Business_type", "X-Request-Id"}, wantRes: []string{"Content-Type"}},
		{name: "trim spaces",
			param:   &AccessLogConfigParam{ReqHeaders: []string{" x-trace-id "}},
			wantReq: []string{"X-Trace-Id"}, wantRes: []string{}},
		{name: "all token chars accepted",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"x-token!#$%&'*+-.^_`|~"}},
			wantReq: []string{canonKey("x-token!#$%&'*+-.^_`|~")}, wantRes: []string{}},
		{name: "empty name rejected",
			param:   &AccessLogConfigParam{ReqHeaders: []string{""}},
			wantErr: true},
		{name: "blank name rejected",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"   "}},
			wantErr: true},
		{name: "too long rejected",
			param:   &AccessLogConfigParam{ReqHeaders: []string{strings.Repeat("a", maxHeaderNameLen+1)}},
			wantErr: true},
		{name: "boundary length accepted",
			param:   &AccessLogConfigParam{ReqHeaders: []string{strings.Repeat("a", maxHeaderNameLen)}},
			wantReq: []string{"A" + strings.Repeat("a", maxHeaderNameLen-1)}, wantRes: []string{}},
		{name: "space char rejected",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"bad name"}},
			wantErr: true},
		{name: "colon rejected",
			param:   &AccessLogConfigParam{ResHeaders: []string{"bad:name"}},
			wantErr: true},
		{name: "duplicate after canonicalization rejected",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"x-id", "X-ID", "X-Id"}},
			wantErr: true},
		// control-plane-only rules (BFE data plane does not restrict content)
		{name: "authorization rejected",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"Authorization"}},
			wantErr: true},
		{name: "proxy-authorization rejected case-insensitively",
			param:   &AccessLogConfigParam{ResHeaders: []string{"pRoXy-aUtHoRiZaTiOn"}},
			wantErr: true},
		{name: "cookie allowed",
			param:   &AccessLogConfigParam{ReqHeaders: []string{"Cookie"}, ResHeaders: []string{"Set-Cookie"}},
			wantReq: []string{"Cookie"}, wantRes: []string{"Set-Cookie"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, res, err := ValidateAccessLogConfig(c.param)
			if c.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, c.wantReq, req)
			assert.Equal(t, c.wantRes, res)
		})
	}
}

func TestContainsCookieHeader(t *testing.T) {
	assert.False(t, ContainsCookieHeader(nil, nil))
	assert.False(t, ContainsCookieHeader([]string{"Business_type"}, []string{"X-Trace-Id"}))
	assert.True(t, ContainsCookieHeader([]string{"Cookie"}, nil))
	assert.True(t, ContainsCookieHeader(nil, []string{"Set-Cookie"}))
}
