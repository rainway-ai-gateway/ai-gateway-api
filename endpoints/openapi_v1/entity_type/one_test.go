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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	modelentity "github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
)

func newEntityTypeOneRequest(typeName string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/entity-types/"+typeName, nil)
	return mux.SetURLVars(req, map[string]string{"type_name": typeName})
}

func TestEntityTypeOneAction_InvalidName(t *testing.T) {
	typ := &fakeDeleteEntityTypeStorager{
		entityType: &modelentity.EntityTypeParam{
			TypeName: lib.PString("org"),
			Level:    lib.PInt(1),
		},
	}
	ent := &fakeDeleteRefEntityStorager{}
	setupDeleteTypeManagers(t, typ, ent)

	_, err := EntityTypeOneAction(newEntityTypeOneRequest("-a"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 422, rr.ErrNo)
	assert.Equal(t, 0, typ.fetchCalls, "invalid name must be rejected at binding layer, before any storage access")
	assert.Equal(t, 0, ent.fetchListCalls)
}

func TestEntityTypeDeleteAction_InvalidName(t *testing.T) {
	typ := &fakeDeleteEntityTypeStorager{
		entityType: &modelentity.EntityTypeParam{
			TypeName: lib.PString("org"),
			Level:    lib.PInt(1),
		},
	}
	ent := &fakeDeleteRefEntityStorager{}
	setupDeleteTypeManagers(t, typ, ent)

	_, err := EntityTypeDeleteAction(newEntityTypeDeleteRequest("-a"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 422, rr.ErrNo)
	assert.Equal(t, 0, typ.fetchCalls, "invalid name must be rejected at binding layer, before any storage access")
	assert.Equal(t, 0, ent.fetchListCalls)
}
