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
	"errors"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOperationLogRecorder struct {
	entries []*ioperlog.OperationLogEntry
}

func (r *fakeOperationLogRecorder) Record(ctx context.Context, entry *ioperlog.OperationLogEntry) {
	r.entries = append(r.entries, entry)
}

func TestEntityManager_CreateEntity_RecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	entityID := "ent-1"
	entityName := "entity-one"
	entityType := "tenant"

	manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
		fetchFn: func(ctx context.Context, filter *EntityFilter) (*EntityParam, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, param *EntityParam) (int64, error) {
			return 100, nil
		},
	}, &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return &EntityTypeParam{TypeName: lib.PString(entityType), Level: lib.PInt(1)}, nil
		},
	}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
	manager.SetOperationLogManager(recorder)

	_, err := manager.CreateEntity(ctx, &EntityParam{
		EntityID: &entityID,
		Name:     &entityName,
		Type:     &entityType,
	})
	require.NoError(t, err)

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionCreate), entry.Action)
	assert.Equal(t, string(ioperlog.ResourceTypeEntity), entry.ResourceType)
	assert.Equal(t, entityID, entry.ResourceID)
	assert.Equal(t, entityName, entry.ResourceName)
	assert.Equal(t, ioperlog.StatusSuccess, entry.Status)
}

func TestEntityManager_UpdateEntity_RecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	entityID := "ent-1"
	oldName := "old-name"
	newName := "new-name"

	manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
		listFn: func(ctx context.Context, filter *EntityFilter) ([]*EntityParam, error) {
			return []*EntityParam{{EntityID: &entityID, Name: &oldName}}, nil
		},
		updateFn: func(ctx context.Context, filter *EntityFilter, param *EntityParam) (int64, error) {
			return 1, nil
		},
	}, &fakeEntityTypeStorager{}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
	manager.SetOperationLogManager(recorder)

	_, err := manager.UpdateEntity(ctx, &EntityFilter{EntityID: &entityID}, &EntityParam{
		Name: &newName,
	})
	require.NoError(t, err)

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
	assert.Equal(t, entityID, entry.ResourceID)
	require.NotNil(t, entry.ChangeSummary)
	assert.Equal(t, []string{"name"}, entry.ChangeSummary["diff_keys"])
}

func TestEntityTypeManager_CreateEntityType_RecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	typeName := "tenant"
	description := "tenant type"

	manager := NewEntityTypeManager(&fakeTxn{}, &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, param *EntityTypeParam) (int64, error) {
			return 1, nil
		},
	}, &fakeEntityStorager{})
	manager.SetOperationLogManager(recorder)

	_, err := manager.CreateEntityType(ctx, &EntityTypeParam{
		TypeName:    &typeName,
		Description: &description,
	})
	require.NoError(t, err)

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionCreate), entry.Action)
	assert.Equal(t, string(ioperlog.ResourceTypeEntityType), entry.ResourceType)
	assert.Equal(t, typeName, entry.ResourceID)
	assert.Equal(t, description, entry.ResourceName)
	assert.Equal(t, ioperlog.StatusSuccess, entry.Status)
}

func TestEntityTypeManager_UpdateEntityType_RecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	typeName := "tenant"
	oldDescription := "old tenant"
	newDescription := "new tenant"

	manager := NewEntityTypeManager(&fakeTxn{}, &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return &EntityTypeParam{TypeName: &typeName, Description: &oldDescription}, nil
		},
		updateFn: func(ctx context.Context, filter *EntityTypeFilter, param *EntityTypeParam) (int64, error) {
			return 1, nil
		},
	}, &fakeEntityStorager{})
	manager.SetOperationLogManager(recorder)

	_, err := manager.UpdateEntityType(ctx, &EntityTypeFilter{TypeName: &typeName}, &EntityTypeParam{
		Description: &newDescription,
	})
	require.NoError(t, err)

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
	assert.Equal(t, typeName, entry.ResourceID)
	require.NotNil(t, entry.ChangeSummary)
	assert.Equal(t, []string{"description"}, entry.ChangeSummary["diff_keys"])
}

func TestEntityTypeManager_DeleteEntityType_RecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	typeName := "tenant"
	description := "tenant type"

	manager := NewEntityTypeManager(&fakeTxn{}, &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return &EntityTypeParam{TypeName: &typeName, Description: &description}, nil
		},
		deleteFn: func(ctx context.Context, filter *EntityTypeFilter) error {
			return nil
		},
	}, &fakeEntityStorager{})
	manager.SetOperationLogManager(recorder)

	err := manager.DeleteEntityType(ctx, &EntityTypeFilter{TypeName: &typeName})
	require.NoError(t, err)

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionDelete), entry.Action)
	assert.Equal(t, typeName, entry.ResourceID)
	assert.Equal(t, description, entry.ResourceName)
}

func TestEntityManager_DeleteEntity_RecordsOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	entityID := "ent-1"
	entityName := "entity-one"

	manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
		fetchFn: func(ctx context.Context, filter *EntityFilter) (*EntityParam, error) {
			return &EntityParam{EntityID: &entityID, Name: &entityName}, nil
		},
		listFn: func(ctx context.Context, filter *EntityFilter) ([]*EntityParam, error) {
			// First call returns the entity; second call (children check) returns empty.
			if filter.EntityID != nil && *filter.EntityID == entityID {
				return []*EntityParam{{EntityID: &entityID, Name: &entityName}}, nil
			}
			return nil, nil
		},
		deleteFn: func(ctx context.Context, filter *EntityFilter) error {
			return nil
		},
	}, &fakeEntityTypeStorager{}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
	manager.SetOperationLogManager(recorder)

	err := manager.DeleteEntity(ctx, &EntityFilter{EntityID: &entityID})
	require.NoError(t, err)

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionDelete), entry.Action)
	assert.Equal(t, entityID, entry.ResourceID)
	assert.Equal(t, entityName, entry.ResourceName)
}

func TestEntityManager_CreateEntity_RecordsFailedOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	entityID := "ent-1"
	entityName := "entity-one"
	entityType := "tenant"

	manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
		fetchFn: func(ctx context.Context, filter *EntityFilter) (*EntityParam, error) {
			return nil, nil
		},
		createFn: func(ctx context.Context, param *EntityParam) (int64, error) {
			return 0, errors.New("db connection failed")
		},
	}, &fakeEntityTypeStorager{
		fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
			return &EntityTypeParam{TypeName: lib.PString(entityType), Level: lib.PInt(1)}, nil
		},
	}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
	manager.SetOperationLogManager(recorder)

	_, err := manager.CreateEntity(ctx, &EntityParam{
		EntityID: &entityID,
		Name:     &entityName,
		Type:     &entityType,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db connection failed")

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionCreate), entry.Action)
	assert.Equal(t, string(ioperlog.ResourceTypeEntity), entry.ResourceType)
	assert.Equal(t, entityID, entry.ResourceID)
	assert.Equal(t, entityName, entry.ResourceName)
	assert.Equal(t, ioperlog.StatusFailed, entry.Status)
	assert.Contains(t, entry.ErrorMsg, "db connection failed")
}

func TestEntityManager_DeleteEntity_RecordsFailedOperationLog(t *testing.T) {
	ctx := context.Background()
	recorder := &fakeOperationLogRecorder{}

	entityID := "ent-1"
	entityName := "entity-one"

	manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
		fetchFn: func(ctx context.Context, filter *EntityFilter) (*EntityParam, error) {
			return &EntityParam{EntityID: &entityID, Name: &entityName}, nil
		},
		listFn: func(ctx context.Context, filter *EntityFilter) ([]*EntityParam, error) {
			if filter.EntityID != nil && *filter.EntityID == entityID {
				return []*EntityParam{{EntityID: &entityID, Name: &entityName}}, nil
			}
			return nil, nil
		},
		deleteFn: func(ctx context.Context, filter *EntityFilter) error {
			return errors.New("delete constraint violation")
		},
	}, &fakeEntityTypeStorager{}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
	manager.SetOperationLogManager(recorder)

	err := manager.DeleteEntity(ctx, &EntityFilter{EntityID: &entityID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete constraint violation")

	require.Len(t, recorder.entries, 1)
	entry := recorder.entries[0]
	assert.Equal(t, string(ioperlog.ActionDelete), entry.Action)
	assert.Equal(t, entityID, entry.ResourceID)
	assert.Equal(t, entityName, entry.ResourceName)
	assert.Equal(t, ioperlog.StatusFailed, entry.Status)
	assert.Contains(t, entry.ErrorMsg, "delete constraint violation")
}

// issue #155：前置校验失败（极简 body，无 id/name）时，失败审计日志
// 必须能从库中记录 / URI 寻址解析资源身份，不得为空串。
func TestEntityManager_UpdateEntity_PreCheckFailureLogKeepsResourceIdentity(t *testing.T) {
	ctx := context.Background()

	t.Run("record exists: identity from db snapshot", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}

		entityID := "entity-270"
		entityName := "entity-old-name"
		entityType := "tenant"

		manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
			listFn: func(ctx context.Context, filter *EntityFilter) ([]*EntityParam, error) {
				return []*EntityParam{{EntityID: &entityID, Name: &entityName, Type: &entityType}}, nil
			},
			fetchFn: func(ctx context.Context, filter *EntityFilter) (*EntityParam, error) {
				return nil, nil // parent entity not found
			},
		}, &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				return &EntityTypeParam{TypeName: &entityType, Level: lib.PInt(2)}, nil
			},
		}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
		manager.SetOperationLogManager(recorder)

		// 极简 body：仅 parent_id，无 id/name（SC2101-TC046 场景）
		_, err := manager.UpdateEntity(ctx, &EntityFilter{EntityID: &entityID}, &EntityParam{
			ParentID: lib.PString("entity-888"),
		})
		require.Error(t, err)

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
		assert.Equal(t, entityID, entry.ResourceID)
		assert.Equal(t, entityName, entry.ResourceName)
		assert.Equal(t, "entity-888", entry.ResourceParentID)
		assert.Equal(t, ioperlog.StatusFailed, entry.Status)
		assert.Contains(t, entry.ErrorMsg, "parent entity not found")
	})

	t.Run("record not exist: identity falls back to uri filter", func(t *testing.T) {
		recorder := &fakeOperationLogRecorder{}

		entityID := "entity-404"

		manager := NewEntityManager(&fakeTxn{}, &fakeEntityStorager{
			listFn: func(ctx context.Context, filter *EntityFilter) ([]*EntityParam, error) {
				return nil, nil
			},
		}, &fakeEntityTypeStorager{
			fetchFn: func(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
				return &EntityTypeParam{TypeName: lib.PString("tenant"), Level: lib.PInt(1)}, nil
			},
		}, &fakeSharedQuotaPlanStorager{}, &fakeSharedRateLimitPolicyStorager{}, &fakeRouteRulesStorager{}, nil)
		manager.SetOperationLogManager(recorder)

		_, err := manager.UpdateEntity(ctx, &EntityFilter{EntityID: &entityID}, &EntityParam{
			ParentID: lib.PString("entity-888"),
		})
		require.Error(t, err)

		require.Len(t, recorder.entries, 1)
		entry := recorder.entries[0]
		assert.Equal(t, entityID, entry.ResourceID)
		assert.Equal(t, ioperlog.StatusFailed, entry.Status)
	})
}
