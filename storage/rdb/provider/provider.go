// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

// RDBProviderStorager implements iprovider.ProviderStorager using RDB.
type RDBProviderStorager struct {
	dbCtxFactory lib.DBContextFactory
}

var _ iprovider.ProviderStorager = &RDBProviderStorager{}

// NewRDBProviderStorager creates a new RDB-backed provider storager.
func NewRDBProviderStorager(dbCtxFactory lib.DBContextFactory) *RDBProviderStorager {
	return &RDBProviderStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

func (s *RDBProviderStorager) CreateProvider(ctx context.Context, param *iprovider.ProviderParam) (int64, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return 0, err
	}

	data, err := toDAOParam(param)
	if err != nil {
		return 0, err
	}

	return dao.TProviderCreate(dbCtx, data)
}

func (s *RDBProviderStorager) UpdateProvider(ctx context.Context, name string, param *iprovider.ProviderParam) error {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	data, err := toDAOParamForUpdate(param)
	if err != nil {
		return err
	}

	_, err = dao.TProviderUpdate(dbCtx, data, &dao.TProviderParam{Name: &name})
	return err
}

func (s *RDBProviderStorager) DeleteProvider(ctx context.Context, name string) error {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return err
	}

	_, err = dao.TProviderDelete(dbCtx, &dao.TProviderParam{Name: &name})
	return err
}

func (s *RDBProviderStorager) FetchProvider(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	where := filterToDAOParam(filter)
	one, err := dao.TProviderOne(dbCtx, where)
	if err != nil {
		return nil, err
	}
	if one == nil {
		return nil, nil
	}
	p, err := fromDAO(one)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *RDBProviderStorager) FetchProviderNames(ctx context.Context) ([]string, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, err
	}

	return dao.TProviderNames(dbCtx)
}

func (s *RDBProviderStorager) FetchProviderList(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
	dbCtx, err := s.dbCtxFactory(ctx)
	if err != nil {
		return nil, 0, err
	}

	where := filterToDAOParam(filter)

	// No pagination requested: return all matched records.
	if filter == nil || (filter.Page == nil && filter.PageSize == nil) {
		list, err := dao.TProviderListAll(dbCtx, where)
		if err != nil {
			return nil, 0, err
		}

		rst := make([]*iprovider.Provider, 0, len(list))
		for _, one := range list {
			p, err := fromDAO(one)
			if err != nil {
				return nil, 0, err
			}
			rst = append(rst, p)
		}
		return rst, int64(len(rst)), nil
	}

	page := 1
	pageSize := 50
	if filter.Page != nil && *filter.Page > 0 {
		page = *filter.Page
	}
	if filter.PageSize != nil && *filter.PageSize > 0 {
		pageSize = *filter.PageSize
		if pageSize > 1000 {
			pageSize = 1000
		}
	}

	list, err := dao.TProviderListWithPagination(dbCtx, where, page, pageSize)
	if err != nil {
		return nil, 0, err
	}

	total, err := dao.TProviderCount(dbCtx, where)
	if err != nil {
		return nil, 0, err
	}

	rst := make([]*iprovider.Provider, 0, len(list))
	for _, one := range list {
		p, err := fromDAO(one)
		if err != nil {
			return nil, 0, err
		}
		rst = append(rst, p)
	}
	return rst, total, nil
}

func toDAOParam(param *iprovider.ProviderParam) (*dao.TProviderParam, error) {
	if param == nil {
		return nil, nil
	}

	iprovider.FillDefaults(param)

	modelEndpoint, err := marshalJSON(param.ModelEndpoint)
	if err != nil {
		return nil, err
	}
	models, err := marshalJSON(param.Models)
	if err != nil {
		return nil, err
	}
	keys, err := marshalJSON(param.Keys)
	if err != nil {
		return nil, err
	}
	encKeys, err := stateful.EncryptIfEnabled(*keys)
	if err != nil {
		return nil, err
	}
	keys = lib.PString(encKeys)
	instancePool, err := marshalJSON(param.InstancePool)
	if err != nil {
		return nil, err
	}
	modelProtocols, err := marshalJSON(param.ModelProtocols)
	if err != nil {
		return nil, err
	}
	protocolPaths, err := marshalJSON(param.ProtocolPaths)
	if err != nil {
		return nil, err
	}
	tiers, err := marshalJSON(param.Tiers)
	if err != nil {
		return nil, err
	}

	return &dao.TProviderParam{
		Name:           param.Name,
		Description:    param.Description,
		ModelEndpoint:  modelEndpoint,
		Models:         models,
		Keys:           keys,
		InstancePool:   instancePool,
		InstanceSource: param.InstanceSource,
		K8sPoolName:    param.K8sPoolName,
		ModelProtocols: modelProtocols,
		ProtocolPaths:  protocolPaths,
		TimeZone:       param.TimeZone,
		Tiers:          tiers,
	}, nil
}

func filterToDAOParam(filter *iprovider.ProviderFilter) *dao.TProviderParam {
	if filter == nil {
		return nil
	}
	return &dao.TProviderParam{
		ID:             filter.ID,
		Name:           filter.Name,
		Names:          filter.Names,
		InstanceSource: filter.InstanceSource,
		K8sPoolName:    filter.K8sPoolName,
	}
}

func fromDAO(one *dao.TProvider) (*iprovider.Provider, error) {
	if one == nil {
		return nil, nil
	}

	keysJSON, err := stateful.DecryptIfEnabled(one.Keys)
	if err != nil {
		return nil, fmt.Errorf("provider %s: decrypt api_keys: %v", one.Name, err)
	}

	createTime := one.CreatedAt.Unix()
	updateTime := one.UpdatedAt.Unix()

	timeZone := one.TimeZone
	if timeZone == "" {
		timeZone = "Asia/Shanghai"
	}

	tiers := unmarshalTiers(one.Tiers)
	if tiers == nil {
		tiers = []iprovider.PricingTier{}
	}

	instanceSource := one.InstanceSource
	if instanceSource == "" {
		instanceSource = iprovider.InstanceSourceInstancePool
	}

	var k8sPoolName *string
	if one.K8sPoolName != "" {
		k8sPoolName = &one.K8sPoolName
	}

	keys, err := unmarshalKeys(keysJSON)
	if err != nil {
		// The api_keys column is a text column (it carries an enc$v1$
		// envelope at rest), so the DB no longer rejects malformed JSON for
		// us: surface the corruption instead of silently dropping the keys.
		return nil, fmt.Errorf("provider %s: parse api_keys: %v", one.Name, err)
	}

	return &iprovider.Provider{
		ID:             one.ID,
		Name:           one.Name,
		Description:    one.Description,
		ModelEndpoint:  unmarshalEndpoint(one.ModelEndpoint),
		Models:         unmarshalStringSlice(one.Models),
		Keys:           keys,
		InstancePool:   unmarshalInstancePool(one.InstancePool),
		InstanceSource: instanceSource,
		K8sPoolName:    k8sPoolName,
		ModelProtocols: unmarshalStringSlice(one.ModelProtocols),
		ProtocolPaths:  unmarshalStringMap(one.ProtocolPaths),
		TimeZone:       timeZone,
		Tiers:          tiers,
		CreateTime:     createTime,
		UpdateTime:     updateTime,
	}, nil
}

func toDAOParamForUpdate(param *iprovider.ProviderParam) (*dao.TProviderParam, error) {
	if param == nil {
		return nil, nil
	}

	modelEndpoint, err := marshalJSONPtr(param.ModelEndpoint)
	if err != nil {
		return nil, err
	}
	models, err := marshalJSONPtr(param.Models)
	if err != nil {
		return nil, err
	}
	keys, err := marshalJSONPtr(param.Keys)
	if err != nil {
		return nil, err
	}
	if keys != nil {
		encKeys, err := stateful.EncryptIfEnabled(*keys)
		if err != nil {
			return nil, err
		}
		keys = lib.PString(encKeys)
	}
	instancePool, err := marshalJSONPtr(param.InstancePool)
	if err != nil {
		return nil, err
	}
	modelProtocols, err := marshalJSONPtr(param.ModelProtocols)
	if err != nil {
		return nil, err
	}
	protocolPaths, err := marshalJSONPtr(param.ProtocolPaths)
	if err != nil {
		return nil, err
	}
	tiers, err := marshalJSONPtr(param.Tiers)
	if err != nil {
		return nil, err
	}

	return &dao.TProviderParam{
		Name:           param.Name,
		Description:    param.Description,
		ModelEndpoint:  modelEndpoint,
		Models:         models,
		Keys:           keys,
		InstancePool:   instancePool,
		InstanceSource: param.InstanceSource,
		K8sPoolName:    param.K8sPoolName,
		ModelProtocols: modelProtocols,
		ProtocolPaths:  protocolPaths,
		TimeZone:       param.TimeZone,
		Tiers:          tiers,
	}, nil
}

// marshalJSONPtr returns nil for a nil interface, nil pointer, or nil slice,
// so that the DAO layer's nil-skip keeps the existing column value on partial
// updates. Explicitly provided values (including empty slices) are marshaled
// normally.
func marshalJSONPtr(v interface{}) (*string, error) {
	if v == nil {
		return nil, nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Interface:
		if rv.IsNil() {
			return nil, nil
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return lib.PString(string(data)), nil
}

func marshalJSON(v interface{}) (*string, error) {
	if v == nil {
		return lib.PString("{}"), nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return lib.PString(string(data)), nil
}

func unmarshalEndpoint(s string) *iprovider.ProviderEndpoint {
	if s == "" || s == "null" {
		return iprovider.DefaultModelEndpoint()
	}
	var e iprovider.ProviderEndpoint
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		return iprovider.DefaultModelEndpoint()
	}
	if e.Schema == "" {
		e.Schema = "https"
	}
	if e.URI == "" {
		e.URI = "/v1/models"
	}
	return &e
}

func unmarshalStringSlice(s string) []string {
	if s == "" || s == "null" {
		return nil
	}
	var rst []string
	if err := json.Unmarshal([]byte(s), &rst); err != nil {
		return nil
	}
	return rst
}

func unmarshalKeys(s string) ([]iprovider.ProviderKey, error) {
	if s == "" || s == "null" {
		return nil, nil
	}
	var rst []iprovider.ProviderKey
	if err := json.Unmarshal([]byte(s), &rst); err != nil {
		// Deliberately do not echo the value: it may be plaintext key
		// material or an envelope.
		return nil, fmt.Errorf("invalid key list json")
	}
	return rst, nil
}

func unmarshalInstancePool(s string) []iprovider.ProviderInstance {
	if s == "" || s == "null" {
		return nil
	}
	var rst []iprovider.ProviderInstance
	if err := json.Unmarshal([]byte(s), &rst); err != nil {
		return nil
	}
	return rst
}

func unmarshalTiers(s string) []iprovider.PricingTier {
	if s == "" || s == "null" || s == "{}" {
		return nil
	}
	var rst []iprovider.PricingTier
	if err := json.Unmarshal([]byte(s), &rst); err != nil {
		return nil
	}
	return rst
}

func unmarshalStringMap(s string) map[string]string {
	if s == "" || s == "null" {
		return nil
	}
	var rst map[string]string
	if err := json.Unmarshal([]byte(s), &rst); err != nil {
		return nil
	}
	return rst
}
