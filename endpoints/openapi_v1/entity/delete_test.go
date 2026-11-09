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

package entity

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
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	modelentity "github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

type fakeDeleteTxn struct{}

func (f *fakeDeleteTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

var _ itxn.TxnStorager = (*fakeDeleteTxn)(nil)

type fakeDeleteEntityStorager struct {
	entity      *modelentity.EntityParam
	children    []*modelentity.EntityParam
	deleteCalls int
}

func (f *fakeDeleteEntityStorager) CreateEntity(ctx context.Context, param *modelentity.EntityParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteEntityStorager) FetchEntity(ctx context.Context, filter *modelentity.EntityFilter) (*modelentity.EntityParam, error) {
	return f.entity, nil
}

func (f *fakeDeleteEntityStorager) FetchEntityList(ctx context.Context, filter *modelentity.EntityFilter) ([]*modelentity.EntityParam, error) {
	if filter.ParentID != nil {
		return f.children, nil
	}
	if f.entity == nil {
		return nil, nil
	}
	return []*modelentity.EntityParam{f.entity}, nil
}

func (f *fakeDeleteEntityStorager) UpdateEntity(ctx context.Context, filter *modelentity.EntityFilter, param *modelentity.EntityParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteEntityStorager) DeleteEntity(ctx context.Context, filter *modelentity.EntityFilter) error {
	f.deleteCalls++
	return nil
}

var _ modelentity.EntityStorager = (*fakeDeleteEntityStorager)(nil)

type fakeDeleteAPIKeyStorager struct {
	keys []*api_key.APIKeyParam
}

func (f *fakeDeleteAPIKeyStorager) FetchAPIKeyList(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
	return f.keys, nil
}

func (f *fakeDeleteAPIKeyStorager) CreateAPIKey(ctx context.Context, param *api_key.APIKeyParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteAPIKeyStorager) UpdateAPIKey(ctx context.Context, filter *api_key.APIKeyFilter, param *api_key.APIKeyParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteAPIKeyStorager) DeleteAPIKey(ctx context.Context, filter *api_key.APIKeyFilter) error {
	return nil
}

func (f *fakeDeleteAPIKeyStorager) CreateAPIKeyToken(ctx context.Context, param *api_key.APIKeyTokenParam) (int64, error) {
	return 0, nil
}

func (f *fakeDeleteAPIKeyStorager) UpdateAPIKeyToken(ctx context.Context, filter *api_key.APIKeyTokenFilter, param *api_key.APIKeyTokenParam) error {
	return nil
}

func (f *fakeDeleteAPIKeyStorager) FetchAPIKeyTokenList(ctx context.Context, filter *api_key.APIKeyTokenFilter) ([]*api_key.APIKeyTokenParam, error) {
	return nil, nil
}

var _ api_key.APIKeyStorager = (*fakeDeleteAPIKeyStorager)(nil)

func setupDeleteManagers(t *testing.T, ent *fakeDeleteEntityStorager, key *fakeDeleteAPIKeyStorager) {
	t.Helper()
	oldEnt := container.EntityManager
	oldKey := container.APIKeyManager
	container.EntityManager = modelentity.NewEntityManager(&fakeDeleteTxn{}, ent, nil, nil, nil, nil, nil)
	container.APIKeyManager = api_key.NewAPIKeyManager(&fakeDeleteTxn{}, key, nil, nil, nil, nil, nil)
	t.Cleanup(func() {
		container.EntityManager = oldEnt
		container.APIKeyManager = oldKey
	})
}

func newEntityDeleteRequest(id string) *http.Request {
	req := httptest.NewRequest(http.MethodDelete, "/open-api/v1/entities/"+id, nil)
	return mux.SetURLVars(req, map[string]string{"id": id})
}

func TestEntityDeleteAction_MountedByAPIKey(t *testing.T) {
	ent := &fakeDeleteEntityStorager{
		entity: &modelentity.EntityParam{
			EntityID: lib.PString("entity-1"),
			Name:     lib.PString("leaf"),
		},
	}
	key := &fakeDeleteAPIKeyStorager{
		keys: []*api_key.APIKeyParam{{ID: lib.PString("ak-1"), EntityID: lib.PString("entity-1")}},
	}
	setupDeleteManagers(t, ent, key)

	_, err := EntityDeleteAction(newEntityDeleteRequest("entity-1"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 409, rr.ErrNo)
	assert.Contains(t, rr.Msg, "cannot delete entity with associated api-keys")
	assert.Equal(t, 0, ent.deleteCalls, "conflict must not trigger partial deletion")
}

func TestEntityDeleteAction_HasChildren(t *testing.T) {
	ent := &fakeDeleteEntityStorager{
		entity: &modelentity.EntityParam{
			EntityID: lib.PString("entity-1"),
			Name:     lib.PString("middle"),
		},
		children: []*modelentity.EntityParam{
			{EntityID: lib.PString("entity-2"), ParentID: lib.PString("entity-1")},
		},
	}
	key := &fakeDeleteAPIKeyStorager{}
	setupDeleteManagers(t, ent, key)

	_, err := EntityDeleteAction(newEntityDeleteRequest("entity-1"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 409, rr.ErrNo)
	assert.Equal(t, 0, ent.deleteCalls, "conflict must not trigger partial deletion")
}

func TestEntityDeleteAction_NotFound(t *testing.T) {
	ent := &fakeDeleteEntityStorager{}
	key := &fakeDeleteAPIKeyStorager{}
	setupDeleteManagers(t, ent, key)

	_, err := EntityDeleteAction(newEntityDeleteRequest("entity-missing"))
	require.Error(t, err)
	rr := xerror.Resolve(err)
	require.NotNil(t, rr)
	assert.Equal(t, 404, rr.ErrNo)
	assert.Equal(t, 0, ent.deleteCalls)
}

func TestEntityDeleteAction_Success(t *testing.T) {
	ent := &fakeDeleteEntityStorager{
		entity: &modelentity.EntityParam{
			EntityID: lib.PString("entity-1"),
			Name:     lib.PString("leaf"),
		},
	}
	key := &fakeDeleteAPIKeyStorager{}
	setupDeleteManagers(t, ent, key)

	_, err := EntityDeleteAction(newEntityDeleteRequest("entity-1"))
	require.NoError(t, err)
	assert.Equal(t, 1, ent.deleteCalls)
}
