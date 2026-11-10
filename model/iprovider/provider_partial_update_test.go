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

package iprovider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
)

// staticExisting returns the stored shape of a static (instance_pool) provider
// as FetchProvider would return it.
func staticExisting(name string) *Provider {
	return &Provider{
		ID:             1,
		Name:           name,
		Description:    "baseline",
		InstanceSource: InstanceSourceInstancePool,
		InstancePool:   []ProviderInstance{{Addr: "10.0.0.1", Port: 8080, Weight: 100}},
		ModelProtocols: []string{"openai"},
	}
}

func newUpdateTestManager(existing *Provider) (*ProviderManager, *ProviderParam) {
	captured := &ProviderParam{}
	store := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
			return existing, nil
		},
		updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
			*captured = *param
			return nil
		},
	}
	return NewProviderManager(&fakeTxn{}, store), captured
}

func TestProviderManager_UpdateProvider_PartialUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("description only on static provider", func(t *testing.T) {
		name := "prov-static"
		m, captured := newUpdateTestManager(staticExisting(name))

		err := m.UpdateProvider(ctx, name, &ProviderParam{
			Name:        &name,
			Description: lib.PString("updated"),
		})
		require.NoError(t, err)
		assert.Nil(t, captured.Models, "nil-skip semantics must not be polluted by merged validation")
		assert.Nil(t, captured.InstancePool)
		assert.Nil(t, captured.ModelProtocols)
	})

	t.Run("description only on k8s_pool provider", func(t *testing.T) {
		name := "prov-k8s"
		existing := staticExisting(name)
		existing.InstanceSource = InstanceSourceK8sPool
		existing.K8sPoolName = lib.PString("svc-a")
		existing.InstancePool = nil
		m, _ := newUpdateTestManager(existing)

		err := m.UpdateProvider(ctx, name, &ProviderParam{
			Name:        &name,
			Description: lib.PString("updated"),
		})
		require.NoError(t, err)
	})

	t.Run("legacy provider with empty stored models stays maintainable", func(t *testing.T) {
		name := "prov-legacy"
		existing := staticExisting(name)
		existing.Models = []string{}
		m, _ := newUpdateTestManager(existing)

		err := m.UpdateProvider(ctx, name, &ProviderParam{
			Name:        &name,
			Description: lib.PString("updated"),
		})
		require.NoError(t, err, "models check must apply only to explicitly provided values")
	})

	t.Run("explicit empty instance_pool still rejected", func(t *testing.T) {
		name := "prov-static"
		m, captured := newUpdateTestManager(staticExisting(name))

		param := validProviderParam()
		param.Name = &name
		param.InstancePool = []ProviderInstance{}
		err := m.UpdateProvider(ctx, name, param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "instance_pool")
		assert.Nil(t, captured.Name, "rejected update must not reach the storager")
	})

	t.Run("explicit empty model_protocols still rejected", func(t *testing.T) {
		name := "prov-static"
		m, _ := newUpdateTestManager(staticExisting(name))

		param := validProviderParam()
		param.Name = &name
		param.ModelProtocols = []string{}
		err := m.UpdateProvider(ctx, name, param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model_protocols")
	})

	t.Run("mode switch to k8s_pool without name still rejected", func(t *testing.T) {
		name := "prov-static"
		m, _ := newUpdateTestManager(staticExisting(name))

		param := validProviderParam()
		param.Name = &name
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = nil
		err := m.UpdateProvider(ctx, name, param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "k8s_pool_name")
	})

	t.Run("model_endpoint default fill side effect preserved", func(t *testing.T) {
		name := "prov-static"
		m, captured := newUpdateTestManager(staticExisting(name))

		param := validProviderParam()
		param.Name = &name
		param.ModelEndpoint = &ProviderEndpoint{URI: "/v2/models"}
		err := m.UpdateProvider(ctx, name, param)
		require.NoError(t, err)
		require.NotNil(t, captured.ModelEndpoint)
		assert.Equal(t, "https", captured.ModelEndpoint.Schema)
		assert.Equal(t, "/v2/models", captured.ModelEndpoint.URI)
	})

	t.Run("omitted paths and protocols keep stored consistency", func(t *testing.T) {
		name := "prov-static"
		existing := staticExisting(name)
		existing.ProtocolPaths = map[string]string{"openai": "/v1"}
		m, _ := newUpdateTestManager(existing)

		err := m.UpdateProvider(ctx, name, &ProviderParam{
			Name:        &name,
			Description: lib.PString("updated"),
		})
		require.NoError(t, err, "omitted protocols must be fallen back for the paths consistency check")
	})

	t.Run("explicit path for undeclared protocol still rejected", func(t *testing.T) {
		name := "prov-static"
		m, _ := newUpdateTestManager(staticExisting(name))

		param := validProviderParam()
		param.Name = &name
		param.ProtocolPaths = map[string]string{"anthropic": "/v1"}
		err := m.UpdateProvider(ctx, name, param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not declared")
	})

	t.Run("provider not found returns record-not-exist", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return nil, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		name := "prov-missing"
		err := m.UpdateProvider(ctx, name, &ProviderParam{Name: &name})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Not Exist")
	})
}
