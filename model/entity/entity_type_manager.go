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

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
)

// EntityTypeManager 定义 Entity 类型管理器
type EntityTypeManager struct {
	txn                 itxn.TxnStorager
	storager            EntityTypeStorager
	entityStorager      EntityStorager
	operationLogManager ioperlog.OperationLogRecorder
}

// NewEntityTypeManager 创建 Entity 类型管理器
func NewEntityTypeManager(txn itxn.TxnStorager, storager EntityTypeStorager, entityStorager EntityStorager) *EntityTypeManager {
	return &EntityTypeManager{
		txn:            txn,
		storager:       storager,
		entityStorager: entityStorager,
	}
}

// SetOperationLogManager injects the operation log recorder.
func (m *EntityTypeManager) SetOperationLogManager(manager ioperlog.OperationLogRecorder) {
	m.operationLogManager = manager
}

// CreateEntityType 创建 Entity 类型
func (m *EntityTypeManager) CreateEntityType(ctx context.Context, param *EntityTypeParam) (int64, error) {
	if param.TypeName == nil || *param.TypeName == "" {
		err := xerror.WrapParamErrorWithMsg("type_name is required")
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionCreate), param, nil, entityTypeParamToMap(param), err)
		return 0, err
	}

	existing, err := m.storager.FetchEntityType(ctx, &EntityTypeFilter{TypeName: param.TypeName})
	if err != nil {
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionCreate), param, nil, entityTypeParamToMap(param), err)
		return 0, err
	}
	if existing != nil {
		err := xerror.WrapRecordExisted("Entity-Type")
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionCreate), param, nil, entityTypeParamToMap(param), err)
		return 0, err
	}

	id, err := m.storager.CreateEntityType(ctx, param)
	if err != nil {
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionCreate), param, nil, entityTypeParamToMap(param), err)
		return 0, err
	}

	m.recordEntityTypeOperation(ctx, string(ioperlog.ActionCreate), param, nil, entityTypeParamToMap(param), nil)
	return id, nil
}

// FetchEntityType 查询单个 Entity 类型
func (m *EntityTypeManager) FetchEntityType(ctx context.Context, filter *EntityTypeFilter) (*EntityTypeParam, error) {
	return m.storager.FetchEntityType(ctx, filter)
}

// FetchEntityTypeList 查询 Entity 类型列表
func (m *EntityTypeManager) FetchEntityTypeList(ctx context.Context, filter *EntityTypeFilter) ([]*EntityTypeParam, error) {
	return m.storager.FetchEntityTypeList(ctx, filter)
}

// UpdateEntityType 更新 Entity 类型
func (m *EntityTypeManager) UpdateEntityType(ctx context.Context, filter *EntityTypeFilter, param *EntityTypeParam) (int64, error) {
	oldEntityType, err := m.storager.FetchEntityType(ctx, filter)
	if err != nil {
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), param, entityTypeParamToMap(param), nil, err)
		return 0, err
	}
	if oldEntityType == nil {
		err := xerror.WrapRecordNotExist("Entity-Type")
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), param, entityTypeParamToMap(param), nil, err)
		return 0, err
	}

	if param.Level != nil && (*param.Level < 1 || *param.Level > 5) {
		err := xerror.WrapParamErrorWithMsg("level must be between 1 and 5")
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), param, entityTypeParamToMap(param), nil, err)
		return 0, err
	}

	if param.Level != nil && (oldEntityType.Level == nil || *param.Level != *oldEntityType.Level) {
		if err := m.checkLevelChangeConsistency(ctx, *oldEntityType.TypeName, *param.Level); err != nil {
			m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), param, entityTypeParamToMap(oldEntityType), nil, err)
			return 0, err
		}
	}

	affected, err := m.storager.UpdateEntityType(ctx, filter, param)
	if err != nil {
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), oldEntityType, entityTypeParamToMap(oldEntityType), entityTypeParamToMap(param), err)
		return affected, err
	}

	m.recordEntityTypeOperation(ctx, string(ioperlog.ActionUpdate), oldEntityType, entityTypeParamToMap(oldEntityType), entityTypeParamToMap(param), nil)
	return affected, nil
}

// DeleteEntityType 删除 Entity 类型
func (m *EntityTypeManager) DeleteEntityType(ctx context.Context, filter *EntityTypeFilter) error {
	fallbackParam := &EntityTypeParam{}
	if filter != nil {
		fallbackParam.TypeName = filter.TypeName
	}

	oldEntityType, err := m.storager.FetchEntityType(ctx, filter)
	if err != nil {
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionDelete), fallbackParam, entityTypeParamToMap(fallbackParam), nil, err)
		return err
	}
	if oldEntityType == nil {
		err := xerror.WrapRecordNotExist("Entity-Type")
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionDelete), fallbackParam, entityTypeParamToMap(fallbackParam), nil, err)
		return err
	}

	if err := m.storager.DeleteEntityType(ctx, filter); err != nil {
		m.recordEntityTypeOperation(ctx, string(ioperlog.ActionDelete), oldEntityType, entityTypeParamToMap(oldEntityType), nil, err)
		return err
	}

	m.recordEntityTypeOperation(ctx, string(ioperlog.ActionDelete), oldEntityType, entityTypeParamToMap(oldEntityType), nil, nil)
	return nil
}

// checkLevelChangeConsistency rejects a level change that would leave existing
// entity parent/child relations violating the hierarchy rule: the parent
// entity-type level must be strictly less than the child entity-type level
// (entities.md §2.1/§2.5). It checks relations in both directions for every
// entity of the type being changed — as a child (its parent's type level must
// be < newLevel) and as a parent (each child's type level must be > newLevel).
// Any violation yields a 409 Conflict and the update makes no writes
// (issue #313). A nil entityStorager disables the check (tests).
func (m *EntityTypeManager) checkLevelChangeConsistency(ctx context.Context, typeName string, newLevel int) error {
	if m.entityStorager == nil {
		return nil
	}

	entities, err := m.entityStorager.FetchEntityList(ctx, &EntityFilter{Type: &typeName})
	if err != nil {
		return err
	}
	if len(entities) == 0 {
		return nil
	}

	violations := 0
	typeLevelCache := map[string]*int{}
	typeLevelOf := func(tn string) (*int, error) {
		if l, ok := typeLevelCache[tn]; ok {
			return l, nil
		}
		t, err := m.storager.FetchEntityType(ctx, &EntityTypeFilter{TypeName: &tn})
		if err != nil {
			return nil, err
		}
		var l *int
		if t != nil {
			l = t.Level
		}
		typeLevelCache[tn] = l
		return l, nil
	}

	// As a child: the parent's type level must be strictly less than newLevel.
	for _, e := range entities {
		if e == nil || e.ParentID == nil || *e.ParentID == "" {
			continue
		}
		parent, err := m.entityStorager.FetchEntity(ctx, &EntityFilter{EntityID: e.ParentID})
		if err != nil {
			return err
		}
		if parent == nil || parent.Type == nil || *parent.Type == "" {
			violations++
			continue
		}
		pl, err := typeLevelOf(*parent.Type)
		if err != nil {
			return err
		}
		if pl == nil || *pl >= newLevel {
			violations++
		}
	}

	// As a parent: each child's type level must be strictly greater than newLevel.
	for _, e := range entities {
		if e == nil || e.EntityID == nil {
			continue
		}
		children, err := m.entityStorager.FetchEntityList(ctx, &EntityFilter{ParentID: e.EntityID})
		if err != nil {
			return err
		}
		for _, c := range children {
			if c == nil || c.Type == nil || *c.Type == "" {
				violations++
				continue
			}
			cl, err := typeLevelOf(*c.Type)
			if err != nil {
				return err
			}
			if cl == nil || *cl <= newLevel {
				violations++
			}
		}
	}

	if violations > 0 {
		return xerror.WrapConflictErrorWithMsg("level change rejected: %d existing entity relation(s) would violate the parent/child level constraint", violations)
	}
	return nil
}
