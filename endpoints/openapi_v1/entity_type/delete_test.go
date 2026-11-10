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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	modelentity "github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

type fakeDeleteTypeTxn struct{}

func (f *fakeDeleteTypeTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

var _ itxn.TxnStorager = (*fakeDeleteTypeTxn)(nil)

type fakeDeleteEntityTypeStorager struct {
	entityType  *modelentity.EntityTypeParam
	fetchCalls  int
	deleteCalls int
}

func (f *fakeDeleteEntityTypeStorager) CreateEntityType(ctx context.Context, param *modelentity.EntityTypeParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteEntityTypeStorager) FetchEntityType(ctx context.Context, filter *modelentity.EntityTypeFilter) (*modelentity.EntityTypeParam, error) {
	f.fetchCalls++
	if f.entityType == nil || filter == nil || filter.TypeName == nil ||
		f.entityType.TypeName == nil || *filter.TypeName != *f.entityType.TypeName {
		return nil, nil
	}
	return f.entityType, nil
}

func (f *fakeDeleteEntityTypeStorager) FetchEntityTypeList(ctx context.Context, filter *modelentity.EntityTypeFilter) ([]*modelentity.EntityTypeParam, error) {
	return nil, nil
}

func (f *fakeDeleteEntityTypeStorager) UpdateEntityType(ctx context.Context, filter *modelentity.EntityTypeFilter, param *modelentity.EntityTypeParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteEntityTypeStorager) DeleteEntityType(ctx context.Context, filter *modelentity.EntityTypeFilter) error {
	f.deleteCalls++
	return nil
}

var _ modelentity.EntityTypeStorager = (*fakeDeleteEntityTypeStorager)(nil)

type fakeDeleteRefEntityStorager struct {
	entities      []*modelentity.EntityParam
	fetchListCalls int
}

func (f *fakeDeleteRefEntityStorager) CreateEntity(ctx context.Context, param *modelentity.EntityParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteRefEntityStorager) FetchEntity(ctx context.Context, filter *modelentity.EntityFilter) (*modelentity.EntityParam, error) {
	return nil, nil
}

func (f *fakeDeleteRefEntityStorager) FetchEntityList(ctx context.Context, filter *modelentity.EntityFilter) ([]*modelentity.EntityParam, error) {
	f.fetchListCalls++
	if filter.Type != nil {
		return f.entities, nil
	}
	return nil, nil
}

func (f *fakeDeleteRefEntityStorager) UpdateEntity(ctx context.Context, filter *modelentity.EntityFilter, param *modelentity.EntityParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteRefEntityStorager) DeleteEntity(ctx context.Context, filter *modelentity.EntityFilter) error {
	return nil
}

var _ modelentity.EntityStorager = (*fakeDeleteRefEntityStorager)(nil)

func setupDeleteTypeManagers(t *testing.T, typ *fakeDeleteEntityTypeStorager, ent *fakeDeleteRefEntityStorager) {
	t.Helper()
	oldType := container.EntityTypeManager
	oldEnt := container.EntityManager
	container.EntityTypeManager = modelentity.NewEntityTypeManager(&fakeDeleteTypeTxn{}, typ, ent)
	container.EntityManager = modelentity.NewEntityManager(&fakeDeleteTypeTxn{}, ent, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		container.EntityTypeManager = oldType
		container.EntityManager = oldEnt
	})
}

func newEntityTypeDeleteRequest(typeName string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/open-api/v1/entity-types/"+typeName, nil)
	return mux.SetURLVars(req, map[string]string{"type_name": typeName})
}

func TestEntityTypeDeleteAction_ReferencedByEntity(t *testing.T) {
	typ := &fakeDeleteEntityTypeStorager{
		entityType: &modelentity.EntityTypeParam{
			TypeName: lib.PString("org"),
			Level:    lib.PInt(1),
		},
	}
	ent := &fakeDeleteRefEntityStorager{
		entities: []*modelentity.EntityParam{
			{EntityID: lib.PString("entity-1"), Type: lib.PString("org")},
		},
	}
	setupDeleteTypeManagers(t, typ, ent)

	_, err := EntityTypeDeleteAction(newEntityTypeDeleteRequest("org"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 409, rr.ErrNo)
	assert.Contains(t, rr.Msg, "cannot delete entity type with associated entities")
	assert.Equal(t, 0, typ.deleteCalls, "conflict must not trigger partial deletion")
}

func TestEntityTypeDeleteAction_NotFound(t *testing.T) {
	typ := &fakeDeleteEntityTypeStorager{}
	ent := &fakeDeleteRefEntityStorager{}
	setupDeleteTypeManagers(t, typ, ent)

	_, err := EntityTypeDeleteAction(newEntityTypeDeleteRequest("missing"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 404, rr.ErrNo)
	assert.Equal(t, 0, typ.deleteCalls)
}

func TestEntityTypeDeleteAction_Success(t *testing.T) {
	typ := &fakeDeleteEntityTypeStorager{
		entityType: &modelentity.EntityTypeParam{
			TypeName: lib.PString("org"),
			Level:    lib.PInt(1),
		},
	}
	ent := &fakeDeleteRefEntityStorager{}
	setupDeleteTypeManagers(t, typ, ent)

	_, err := EntityTypeDeleteAction(newEntityTypeDeleteRequest("org"))
	require.NoError(t, err)
	assert.Equal(t, 1, typ.deleteCalls)
}
