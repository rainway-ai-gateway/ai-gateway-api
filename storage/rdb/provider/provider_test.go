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
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStorager(t *testing.T) *RDBProviderStorager {
	s, _ := setupTestStoragerWithDB(t)
	return s
}

// setupTestStoragerWithDB also hands back the raw DB so tests can corrupt or
// inspect column values directly (e.g. the api_keys text column).
func setupTestStoragerWithDB(t *testing.T) (*RDBProviderStorager, *sql.DB) {
	// The DAO layer consults stateful.DefaultConfig when recording SQL.
	if stateful.DefaultConfig == nil {
		stateful.DefaultConfig = &stateful.Config{}
	}

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`
CREATE TABLE providers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  model_endpoint TEXT,
  models TEXT,
  api_keys TEXT,
  instance_pool TEXT NOT NULL,
  instance_source TEXT NOT NULL DEFAULT 'instance_pool',
  k8s_pool_name TEXT,
  model_protocols TEXT NOT NULL,
  protocol_paths TEXT,
  time_zone TEXT NOT NULL DEFAULT 'Asia/Shanghai',
  tiers TEXT,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (name)
);`)
	require.NoError(t, err)

	factory := lib.DBContextFactory(func(ctx context.Context, ops ...*lib.Op) (*lib.DBContext, error) {
		return lib.NewDBContext(ctx, db), nil
	})

	return NewRDBProviderStorager(factory), db
}

func TestToDAOParamForUpdate_OmittedFieldsStayNil(t *testing.T) {
	desc := "new description"
	param := &iprovider.ProviderParam{
		Name:        lib.PString("p1"),
		Description: &desc,
	}

	data, err := toDAOParamForUpdate(param)
	require.NoError(t, err)
	require.NotNil(t, data)

	assert.Equal(t, "p1", *data.Name)
	assert.Equal(t, desc, *data.Description)
	// Omitted fields must stay nil so the DAO layer skips them and the
	// existing column values are preserved (partial update semantics).
	assert.Nil(t, data.ModelEndpoint)
	assert.Nil(t, data.Models)
	assert.Nil(t, data.Keys)
	assert.Nil(t, data.InstancePool)
	assert.Nil(t, data.InstanceSource)
	assert.Nil(t, data.K8sPoolName)
	assert.Nil(t, data.ModelProtocols)
	assert.Nil(t, data.TimeZone)
	assert.Nil(t, data.Tiers)
}

func TestToDAOParamForUpdate_ExplicitEmptySlicesAreMarshaled(t *testing.T) {
	tz := "UTC"
	param := &iprovider.ProviderParam{
		Name:           lib.PString("p1"),
		Models:         []string{},
		Keys:           []iprovider.ProviderKey{},
		Tiers:          []iprovider.PricingTier{},
		InstancePool:   []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 9002, Weight: 100}},
		ModelProtocols: []string{"openai"},
		TimeZone:       &tz,
	}

	data, err := toDAOParamForUpdate(param)
	require.NoError(t, err)

	// Explicitly provided values (including empty slices) must be written,
	// so callers can still clear a field by passing an empty array.
	require.NotNil(t, data.Models)
	assert.Equal(t, "[]", *data.Models)
	require.NotNil(t, data.Keys)
	assert.Equal(t, "[]", *data.Keys)
	require.NotNil(t, data.Tiers)
	assert.Equal(t, "[]", *data.Tiers)
	require.NotNil(t, data.InstancePool)
	require.NotNil(t, data.ModelProtocols)
	require.NotNil(t, data.TimeZone)
	assert.Equal(t, tz, *data.TimeZone)
}

func TestToDAOParam_CreatePathDefaultsUnchanged(t *testing.T) {
	param := &iprovider.ProviderParam{
		Name:           lib.PString("p1"),
		InstancePool:   []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 9002, Weight: 100}},
		ModelProtocols: []string{"openai"},
	}

	data, err := toDAOParam(param)
	require.NoError(t, err)

	// Create path keeps filling defaults for omitted fields.
	require.NotNil(t, data.ModelEndpoint)
	assert.Contains(t, *data.ModelEndpoint, "/v1/models")
	require.NotNil(t, data.Models)
	assert.Equal(t, "[]", *data.Models)
	require.NotNil(t, data.Keys)
	assert.Equal(t, "[]", *data.Keys)
	require.NotNil(t, data.TimeZone)
	assert.Equal(t, "Asia/Shanghai", *data.TimeZone)
	require.NotNil(t, data.Tiers)
	require.NotNil(t, data.InstanceSource)
	assert.Equal(t, iprovider.InstanceSourceInstancePool, *data.InstanceSource)
	assert.Nil(t, data.K8sPoolName)
}

func TestMarshalJSONPtr(t *testing.T) {
	// nil interface
	p, err := marshalJSONPtr(nil)
	require.NoError(t, err)
	assert.Nil(t, p)

	// nil slice
	p, err = marshalJSONPtr([]string(nil))
	require.NoError(t, err)
	assert.Nil(t, p)

	// empty slice
	p, err = marshalJSONPtr([]string{})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, "[]", *p)

	// non-empty slice
	p, err = marshalJSONPtr([]string{"a"})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, `["a"]`, *p)

	// nil pointer
	p, err = marshalJSONPtr((*iprovider.ProviderEndpoint)(nil))
	require.NoError(t, err)
	assert.Nil(t, p)

	// non-nil pointer
	p, err = marshalJSONPtr(&iprovider.ProviderEndpoint{Schema: "https", URI: "/v1/models"})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Contains(t, *p, "/v1/models")
}

func TestRDBProviderStorager_UpdateProviderPartialUpdate(t *testing.T) {
	ctx := context.Background()
	s := setupTestStorager(t)

	customTZ := "UTC"
	_, err := s.CreateProvider(ctx, &iprovider.ProviderParam{
		Name:           lib.PString("p1"),
		Description:    lib.PString("original"),
		ModelEndpoint:  &iprovider.ProviderEndpoint{Schema: "http", URI: "/custom/models"},
		Models:         []string{"deepseek-chat"},
		Keys:           []iprovider.ProviderKey{{Name: "k1", Key: "sk-abc"}},
		InstancePool:   []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 9002, Weight: 100}},
		ModelProtocols: []string{"openai"},
		TimeZone:       &customTZ,
		Tiers: []iprovider.PricingTier{
			{
				Name:       "peak",
				TimeRanges: []iprovider.TimeRange{{Weekdays: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "18:00"}},
			},
		},
	})
	require.NoError(t, err)

	// PATCH with only description: every other field must keep its value.
	require.NoError(t, s.UpdateProvider(ctx, "p1", &iprovider.ProviderParam{
		Name:        lib.PString("p1"),
		Description: lib.PString("updated"),
	}))

	one, err := s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("p1")})
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, "updated", one.Description)
	assert.Equal(t, "UTC", one.TimeZone)
	assert.Equal(t, "http", one.ModelEndpoint.Schema)
	assert.Equal(t, "/custom/models", one.ModelEndpoint.URI)
	assert.Equal(t, []string{"deepseek-chat"}, one.Models)
	require.Len(t, one.Keys, 1)
	assert.Equal(t, "k1", one.Keys[0].Name)
	require.Len(t, one.Tiers, 1)
	assert.Equal(t, "peak", one.Tiers[0].Name)

	// PATCH changing only time_zone: tiers and models keep their values.
	london := "Europe/London"
	require.NoError(t, s.UpdateProvider(ctx, "p1", &iprovider.ProviderParam{
		Name:     lib.PString("p1"),
		TimeZone: &london,
	}))
	one, err = s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("p1")})
	require.NoError(t, err)
	assert.Equal(t, "Europe/London", one.TimeZone)
	require.Len(t, one.Tiers, 1)
	assert.Equal(t, []string{"deepseek-chat"}, one.Models)

	// PATCH explicitly passing empty slices clears those fields.
	require.NoError(t, s.UpdateProvider(ctx, "p1", &iprovider.ProviderParam{
		Name:           lib.PString("p1"),
		Models:         []string{},
		Keys:           []iprovider.ProviderKey{},
		Tiers:          []iprovider.PricingTier{},
		InstancePool:   []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 9002, Weight: 100}},
		ModelProtocols: []string{"openai"},
	}))
	one, err = s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("p1")})
	require.NoError(t, err)
	assert.Empty(t, one.Models)
	assert.Empty(t, one.Keys)
	assert.Empty(t, one.Tiers)
	// Omitted fields keep their previous values.
	assert.Equal(t, "Europe/London", one.TimeZone)
	assert.Equal(t, "http", one.ModelEndpoint.Schema)
}

func TestRDBProviderStorager_CreateProviderDefaults(t *testing.T) {
	ctx := context.Background()
	s := setupTestStorager(t)

	// Create without time_zone/models/keys: defaults must be applied.
	_, err := s.CreateProvider(ctx, &iprovider.ProviderParam{
		Name:           lib.PString("p1"),
		InstancePool:   []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 9002, Weight: 100}},
		ModelProtocols: []string{"openai"},
	})
	require.NoError(t, err)

	one, err := s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("p1")})
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, "Asia/Shanghai", one.TimeZone)
	assert.NotNil(t, one.ModelEndpoint)
	assert.Equal(t, "https", one.ModelEndpoint.Schema)
	assert.Empty(t, one.Models)
	assert.Empty(t, one.Keys)
	assert.Empty(t, one.Tiers)
	assert.Equal(t, iprovider.InstanceSourceInstancePool, one.InstanceSource)
	assert.Nil(t, one.K8sPoolName)
}

func TestRDBProviderStorager_InstanceSourceRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := setupTestStorager(t)

	// Create a k8s_pool-mode provider; no instance mirror is persisted, only
	// the pool name reference (k8s_pools is the single source of truth).
	_, err := s.CreateProvider(ctx, &iprovider.ProviderParam{
		Name:           lib.PString("k8s-provider"),
		Models:         []string{"deepseek-chat"},
		InstanceSource: lib.PString(iprovider.InstanceSourceK8sPool),
		K8sPoolName:    lib.PString("svc-a"),
		ModelProtocols: []string{"openai"},
	})
	require.NoError(t, err)

	one, err := s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("k8s-provider")})
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, iprovider.InstanceSourceK8sPool, one.InstanceSource)
	require.NotNil(t, one.K8sPoolName)
	assert.Equal(t, "svc-a", *one.K8sPoolName)
	assert.Empty(t, one.InstancePool)

	// An update that omits instance_pool keeps the stored value.
	require.NoError(t, s.UpdateProvider(ctx, "k8s-provider", &iprovider.ProviderParam{
		Name:        lib.PString("k8s-provider"),
		Description: lib.PString("desc"),
	}))
	one, err = s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("k8s-provider")})
	require.NoError(t, err)
	assert.Equal(t, "desc", one.Description)
}

// api_keys is a text column (it holds an enc$v1$ envelope at rest), so the DB
// no longer rejects malformed JSON for us: corruption must surface as an error
// instead of silently reading back as "no keys" (issue #264).
func TestRDBProviderStorager_FetchProviderRejectsCorruptKeys(t *testing.T) {
	ctx := context.Background()
	s, db := setupTestStoragerWithDB(t)

	_, err := s.CreateProvider(ctx, &iprovider.ProviderParam{
		Name:           lib.PString("p1"),
		Keys:           []iprovider.ProviderKey{{Name: "k1", Key: "sk-abc"}},
		InstancePool:   []iprovider.ProviderInstance{{Addr: "10.0.0.1", Port: 9002, Weight: 100}},
		ModelProtocols: []string{"openai"},
	})
	require.NoError(t, err)

	_, err = db.Exec(`UPDATE providers SET api_keys = 'not-json' WHERE name = 'p1'`)
	require.NoError(t, err)

	one, err := s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("p1")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse api_keys")
	assert.Nil(t, one)

	// Empty and null are legitimate "no keys" values and keep working.
	for _, raw := range []string{"", "null", "[]"} {
		_, err = db.Exec(`UPDATE providers SET api_keys = ? WHERE name = 'p1'`, raw)
		require.NoError(t, err)
		one, err = s.FetchProvider(ctx, &iprovider.ProviderFilter{Name: lib.PString("p1")})
		require.NoError(t, err)
		require.NotNil(t, one)
		assert.Empty(t, one.Keys)
	}
}
