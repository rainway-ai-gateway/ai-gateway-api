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

package entity

import (
	"context"
	"errors"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEntityTypeManager_CreateEntityType(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		store := &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				return nil, nil
			},
			createFn: func(ctx context.Context, param *EntityTypeParam) (int64, error) {
				return 42, nil
			},
		}
		m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

		id, err := m.CreateEntityType(ctx, &EntityTypeParam{
			TypeName: lib.PString("tenant"),
			Level:    lib.PInt(1),
		})

		require.NoError(t, err)
		assert.Equal(t, int64(42), id)
		assert.Len(t, store.fetched, 1)
		assert.Equal(t, lib.PString("tenant"), store.fetched[0].TypeName)
		assert.Len(t, store.created, 1)
	})

	t.Run("missing type_name", func(t *testing.T) {
		m := NewEntityTypeManager(&fakeTxn{}, &fakeEntityTypeStorager{}, &fakeEntityStorager{})
		_, err := m.CreateEntityType(ctx, &EntityTypeParam{Level: lib.PInt(1)})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "type_name is required")
	})

	t.Run("already existed", func(t *testing.T) {
		store := &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				return &EntityTypeParam{TypeName: lib.PString("tenant")}, nil
			},
		}
		m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

		_, err := m.CreateEntityType(ctx, &EntityTypeParam{TypeName: lib.PString("tenant")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Entity-Type Record Existed")
	})

	t.Run("fetch error", func(t *testing.T) {
		store := &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				return nil, errors.New("db down")
			},
		}
		m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

		_, err := m.CreateEntityType(ctx, &EntityTypeParam{TypeName: lib.PString("tenant")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})
}

func TestEntityTypeManager_FetchEntityType(t *testing.T) {
	ctx := context.Background()
	store := &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			if filter.TypeName != nil && *filter.TypeName == "tenant" {
				return &EntityTypeParam{TypeName: lib.PString("tenant"), Level: lib.PInt(1)}, nil
			}
			return nil, nil
		},
	}
	m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

	one, err := m.FetchEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")})
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, "tenant", *one.TypeName)
	assert.Equal(t, 1, *one.Level)
}

func TestEntityTypeManager_FetchEntityTypeList(t *testing.T) {
	ctx := context.Background()
	store := &fakeEntityTypeStorager{
		listFn: func(ctx context.Context, filter *EntityTypeFilter) ([]*EntityTypeParam, error) {
			return []*EntityTypeParam{
				{TypeName: lib.PString("tenant"), Level: lib.PInt(1)},
			}, nil
		},
	}
	m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

	list, err := m.FetchEntityTypeList(ctx, &EntityTypeFilter{})
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestEntityTypeManager_UpdateEntityType(t *testing.T) {
	ctx := context.Background()
	store := &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return &EntityTypeParam{TypeName: lib.PString("tenant"), Level: lib.PInt(1)}, nil
		},
		updateFn: func(ctx context.Context, filter *EntityTypeFilter, param *EntityTypeParam) (int64, error) {
			return 1, nil
		},
	}
	m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

	affected, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{Level: lib.PInt(2)})
	require.NoError(t, err)
	assert.Equal(t, int64(1), affected)
	assert.Len(t, store.updated, 1)
}

func TestEntityTypeManager_UpdateEntityType_NotFound(t *testing.T) {
	ctx := context.Background()
	store := &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return nil, nil
		},
	}
	m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

	_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{Level: lib.PInt(2)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Entity-Type Record Not Exist")
}

func TestEntityTypeManager_DeleteEntityType(t *testing.T) {
	ctx := context.Background()
	store := &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return &EntityTypeParam{TypeName: lib.PString("tenant"), Level: lib.PInt(1)}, nil
		},
		deleteFn: func(ctx context.Context, filter *EntityTypeFilter) error {
			return nil
		},
	}
	m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

	require.NoError(t, m.DeleteEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}))
	assert.Len(t, store.deleted, 1)
}

func TestEntityTypeManager_DeleteEntityType_NotFound(t *testing.T) {
	ctx := context.Background()
	store := &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return nil, nil
		},
	}
	m := NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{})

	err := m.DeleteEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Entity-Type Record Not Exist")
}

func TestEntityTypeManager_UpdateEntityType_LevelRange(t *testing.T) {
	ctx := context.Background()

	newUpdateManager := func(level int) (*EntityTypeManager, *fakeEntityTypeStorager) {
		store := &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				return &EntityTypeParam{TypeName: lib.PString("tenant"), Level: lib.PInt(3)}, nil
			},
			updateFn: func(ctx context.Context, filter *EntityTypeFilter, param *EntityTypeParam) (int64, error) {
				return 1, nil
			},
		}
		return NewEntityTypeManager(&fakeTxn{}, store, &fakeEntityStorager{}), store
	}

	// issue #313: the update path must enforce the same 1-5 range as create.
	for _, level := range []int{0, -1, 6, 9} {
		m, store := newUpdateManager(level)
		_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{
			Level: lib.PInt(level),
		})
		require.Error(t, err, "level %d must be rejected", level)
		assert.Contains(t, err.Error(), "between 1 and 5")
		assert.Empty(t, store.updated, "level %d: update must not reach the storager", level)
	}

	m, store := newUpdateManager(0)
	_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{
		Level: lib.PInt(2),
	})
	require.NoError(t, err)
	assert.Len(t, store.updated, 1)
}

func TestEntityTypeManager_UpdateEntityType_LevelConsistency(t *testing.T) {
	ctx := context.Background()

	// Hierarchy fixture: type "tenant"(L2) has entity e1; type "project"(L3)
	// has child entity e2 whose parent is e1.
	newManager := func(childTypeLevel int) (*EntityTypeManager, *fakeEntityTypeStorager) {
		typeStore := &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				switch *filter.TypeName {
				case "tenant":
					return &EntityTypeParam{TypeName: lib.PString("tenant"), Level: lib.PInt(2)}, nil
				case "project":
					return &EntityTypeParam{TypeName: lib.PString("project"), Level: lib.PInt(childTypeLevel)}, nil
				}
				return nil, nil
			},
			updateFn: func(ctx context.Context, filter *EntityTypeFilter, param *EntityTypeParam) (int64, error) {
				return 1, nil
			},
		}
		entityStore := &fakeEntityStorager{
			listFn: func(ctx context.Context, filter *EntityFilter) ([]*EntityParam, error) {
				switch {
				case filter.Type != nil && *filter.Type == "tenant":
					return []*EntityParam{{EntityID: lib.PString("e1"), Type: lib.PString("tenant")}}, nil
				case filter.Type != nil && *filter.Type == "project":
					return []*EntityParam{{EntityID: lib.PString("e2"), Type: lib.PString("project"), ParentID: lib.PString("e1")}}, nil
				case filter.ParentID != nil && *filter.ParentID == "e1":
					return []*EntityParam{{EntityID: lib.PString("e2"), Type: lib.PString("project"), ParentID: lib.PString("e1")}}, nil
				}
				return nil, nil
			},
			fetchFn: func(ctx context.Context, filter *EntityFilter) (*EntityParam, error) {
				return &EntityParam{EntityID: lib.PString("e1"), Type: lib.PString("tenant")}, nil
			},
		}
		return NewEntityTypeManager(&fakeTxn{}, typeStore, entityStore), typeStore
	}

	t.Run("raising parent type level above child type level is rejected with 409", func(t *testing.T) {
		// tenant 2 -> 3 equals the child's type level (3): violates 父 < 子.
		// e1 is also a parent with child e2 (project L3): newLevel 3 >= child
		// type level 3 -> violation from the parent direction as well.
		m, typeStore := newManager(3)
		_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{
			Level: lib.PInt(3),
		})
		require.Error(t, err)
		assert.Equal(t, 409, xerror.Resolve(err).ErrNo)
		assert.Empty(t, typeStore.updated, "conflicting update must not reach the storager")
	})

	t.Run("raising child type level above parent type level passes", func(t *testing.T) {
		// project 3 -> 4: as a child of e1 (tenant L2) 2 < 4 holds; e2 has no
		// children. A consistent in-range change must succeed.
		m, typeStore := newManager(3)
		_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("project")}, &EntityTypeParam{
			Level: lib.PInt(4),
		})
		require.NoError(t, err)
		assert.Len(t, typeStore.updated, 1)
	})

	t.Run("lowering child type level to parent's level is rejected with 409", func(t *testing.T) {
		// project 3 -> 2 equals the parent's type level (2): violates 父 < 子.
		m, typeStore := newManager(3)
		_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("project")}, &EntityTypeParam{
			Level: lib.PInt(2),
		})
		// e2's parent is e1 (tenant L2): newLevel 2 >= parent level 2 -> violation.
		require.Error(t, err)
		assert.Equal(t, 409, xerror.Resolve(err).ErrNo)
		assert.Empty(t, typeStore.updated)
	})

	t.Run("unchanged level skips the consistency check", func(t *testing.T) {
		// tenant stays at 2, which would conflict under the strict reading of
		// the fixture; an idempotent PATCH must not be blocked.
		m, typeStore := newManager(2)
		_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{
			Level: lib.PInt(2),
		})
		require.NoError(t, err)
		assert.Len(t, typeStore.updated, 1)
	})

	t.Run("description-only update does not trigger the consistency check", func(t *testing.T) {
		m, typeStore := newManager(3)
		desc := "new desc"
		_, err := m.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: lib.PString("tenant")}, &EntityTypeParam{
			Description: &desc,
		})
		require.NoError(t, err)
		assert.Len(t, typeStore.updated, 1)
	})
}
