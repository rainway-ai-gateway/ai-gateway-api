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

package entity_type

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

func TestOneReqValidate(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		wantErr  bool
	}{
		{"single char", "a", false},
		{"normal", "org", false},
		{"mixed tokens", "a-b_c1", false},
		{"max length 32", strings.Repeat("a", 32), false},
		{"issue repro leading hyphen", "-a", true},
		{"trailing hyphen", "a-", true},
		{"leading underscore", "_a", true},
		{"trailing underscore", "a_", true},
		{"uppercase", "A", true},
		{"uppercase mixed", "Org", true},
		{"dot not allowed", "a.b", true},
		{"space not allowed", "a b", true},
		{"plus not allowed", "a+b", true},
		{"too long 33", strings.Repeat("a", 33), true},
		{"empty", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &OneReq{TypeName: lib.PString(c.typeName)}
			err := req.Validate()
			if c.wantErr {
				assert.Error(t, err)
				rr := xerror.Resolve(err)
				assert.Equal(t, 422, rr.ErrNo)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeleteReqValidate(t *testing.T) {
	cases := []struct {
		name     string
		typeName string
		wantErr  bool
	}{
		{"single char", "a", false},
		{"normal", "org", false},
		{"issue repro leading hyphen", "-a", true},
		{"trailing hyphen", "a-", true},
		{"uppercase", "A", true},
		{"too long 33", strings.Repeat("a", 33), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &DeleteReq{TypeName: lib.PString(c.typeName)}
			err := req.Validate()
			if c.wantErr {
				assert.Error(t, err)
				rr := xerror.Resolve(err)
				assert.Equal(t, 422, rr.ErrNo)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
