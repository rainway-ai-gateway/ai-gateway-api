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

package iprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validProviderParam() *ProviderParam {
	return &ProviderParam{
		Name:        lib.PString("deepseek"),
		Description: lib.PString("DeepSeek API"),
		ModelEndpoint: &ProviderEndpoint{
			Schema: "https",
			URI:    "/v1/models",
		},
		Models: []string{"deepseek-chat", "deepseek-coder"},
		Keys: []ProviderKey{
			{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"},
		},
		InstancePool: []ProviderInstance{
			{Addr: "api.deepseek.com", Port: 443, Weight: 100},
		},
		ModelProtocols: []string{"openai"},
	}
}

func TestNewProviderManager(t *testing.T) {
	m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
	require.NotNil(t, m)
}

func TestProviderManager_CreateProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		store := &fakeProviderStorager{}
		m := NewProviderManager(&fakeTxn{}, store)
		id, err := m.CreateProvider(ctx, validProviderParam())
		require.NoError(t, err)
		assert.Equal(t, int64(1), id)
	})

	t.Run("duplicate", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{Name: "deepseek"}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		_, err := m.CreateProvider(ctx, validProviderParam())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider Record Existed")
	})

	t.Run("invalid name", func(t *testing.T) {
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		param := validProviderParam()
		param.Name = lib.PString("")
		_, err := m.CreateProvider(ctx, param)
		require.Error(t, err)
	})

	t.Run("missing models rejected", func(t *testing.T) {
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		param := validProviderParam()
		param.Models = nil
		_, err := m.CreateProvider(ctx, param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "models is required")
	})

	t.Run("empty models rejected", func(t *testing.T) {
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		param := validProviderParam()
		param.Models = []string{}
		_, err := m.CreateProvider(ctx, param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one element")
	})
}

func TestProviderManager_UpdateProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		updated := false
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{Name: "deepseek"}, nil
			},
			updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
				updated = true
				return nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		err := m.UpdateProvider(ctx, "deepseek", validProviderParam())
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("not found", func(t *testing.T) {
		store := &fakeProviderStorager{}
		m := NewProviderManager(&fakeTxn{}, store)
		err := m.UpdateProvider(ctx, "deepseek", validProviderParam())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "provider Record Not Exist")
	})

	t.Run("empty models rejected", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{Name: "deepseek"}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		param := validProviderParam()
		param.Models = []string{}
		err := m.UpdateProvider(ctx, "deepseek", param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one element")
	})

	t.Run("nil models omitted keeps existing", func(t *testing.T) {
		updated := false
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{Name: "deepseek"}, nil
			},
			updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
				updated = true
				assert.Nil(t, param.Models)
				return nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		param := validProviderParam()
		param.Models = nil
		err := m.UpdateProvider(ctx, "deepseek", param)
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("sync hook invoked when instance_pool changes", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name: "deepseek",
					InstancePool: []ProviderInstance{
						{Addr: "1.2.3.4", Port: 443, Weight: 100},
					},
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hookCalled := 0
		var hookOld, hookNew *Provider
		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			hookCalled++
			hookOld = oldProvider
			hookNew = newProvider
			return nil
		}

		param := validProviderParam()
		param.InstancePool = []ProviderInstance{
			{Addr: "5.6.7.8", Port: 443, Weight: 100},
		}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 1, hookCalled)
		require.NotNil(t, hookOld)
		require.NotNil(t, hookNew)
		assert.Equal(t, "1.2.3.4", hookOld.InstancePool[0].Addr)
		assert.Equal(t, "5.6.7.8", hookNew.InstancePool[0].Addr)
	})

	t.Run("sync hook invoked when model_protocols changes", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					ModelProtocols: []string{"openai", "anthropic"},
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hookCalled := 0
		var hookNew *Provider
		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			hookCalled++
			hookNew = newProvider
			return nil
		}

		param := validProviderParam()
		param.ModelProtocols = []string{"openai"}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 1, hookCalled, "a protocol narrowing must reach the reference checkers")
		require.NotNil(t, hookNew)
		assert.Equal(t, []string{"openai"}, hookNew.ModelProtocols)
	})

	t.Run("sync hook not invoked when model_protocols unchanged", func(t *testing.T) {
		// The stored provider must match the param on every other
		// cluster-relevant field, otherwise those changes would trigger the
		// hooks and the assertion would not isolate model_protocols.
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					ModelProtocols: []string{"openai"},
					Models:         []string{"deepseek-chat", "deepseek-coder"},
					Keys:           []ProviderKey{{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"}},
					InstancePool:   []ProviderInstance{{Addr: "api.deepseek.com", Port: 443, Weight: 100}},
					InstanceSource: InstanceSourceInstancePool,
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hookCalled := 0
		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			hookCalled++
			return nil
		}

		param := validProviderParam()
		param.ModelProtocols = []string{"openai"}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 0, hookCalled)
	})

	t.Run("sync hook not invoked when instance_pool unchanged", func(t *testing.T) {
		param := validProviderParam()
		param.InstancePool = []ProviderInstance{
			{Addr: "1.2.3.4", Port: 443, Weight: 100},
		}

		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					Models:         param.Models,
					Keys:           param.Keys,
					InstancePool:   param.InstancePool,
					ModelProtocols: param.ModelProtocols,
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hookCalled := 0
		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			hookCalled++
			return nil
		}

		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 0, hookCalled)
	})

	t.Run("sync hook error rolls back", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name: "deepseek",
					InstancePool: []ProviderInstance{
						{Addr: "1.2.3.4", Port: 443, Weight: 100},
					},
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			return errors.New("sync failed")
		}

		param := validProviderParam()
		param.InstancePool = []ProviderInstance{
			{Addr: "5.6.7.8", Port: 443, Weight: 100},
		}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sync failed")
	})

	t.Run("key ref checker error rolls back", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name: "deepseek",
					Keys: []ProviderKey{{Name: "k1", Key: "sk-old"}},
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			return errors.New("key referenced")
		}

		param := validProviderParam()
		param.Keys = []ProviderKey{{Name: "k2", Key: "sk-new"}}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key referenced")
	})

	t.Run("model ref checker error rolls back", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:   "deepseek",
					Models: []string{"m1"},
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			return errors.New("model referenced")
		}

		param := validProviderParam()
		param.Models = []string{"m2"}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model referenced")
	})

	t.Run("hooks not invoked when keys and models unchanged", func(t *testing.T) {
		param := validProviderParam()
		param.InstancePool = []ProviderInstance{
			{Addr: "1.2.3.4", Port: 443, Weight: 100},
		}

		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					Models:         param.Models,
					Keys:           param.Keys,
					InstancePool:   param.InstancePool,
					ModelProtocols: param.ModelProtocols,
				}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)

		hookCalled := 0
		hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
			hookCalled++
			return errors.New("should not be called")
		}

		// Only change the description; none of instance_pool/keys/models changed.
		param.Description = lib.PString("new description")
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 0, hookCalled)
	})
}

func TestProviderManager_UpdateProvider_OmittedFieldsReachStoragerAsNil(t *testing.T) {
	ctx := context.Background()

	var captured *ProviderParam
	store := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
			return &Provider{
				Name:         "deepseek",
				Models:       []string{"deepseek-chat"},
				Keys:         []ProviderKey{{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"}},
				TimeZone:     "UTC",
				InstancePool: []ProviderInstance{{Addr: "api.deepseek.com", Port: 443, Weight: 100}},
			}, nil
		},
		updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
			captured = param
			return nil
		},
	}
	m := NewProviderManager(&fakeTxn{}, store)

	// PATCH body only carries instance_pool/model_protocols (required by
	// validation) plus a new description; all other fields are omitted.
	param := &ProviderParam{
		Name:           lib.PString("deepseek"),
		Description:    lib.PString("new description"),
		InstancePool:   []ProviderInstance{{Addr: "api.deepseek.com", Port: 443, Weight: 100}},
		ModelProtocols: []string{"openai"},
	}
	err := m.UpdateProvider(ctx, "deepseek", param)
	require.NoError(t, err)
	require.NotNil(t, captured)

	// Omitted fields must reach the storager as nil so the DAO layer keeps
	// the existing column values (partial update semantics).
	assert.Nil(t, captured.ModelEndpoint)
	assert.Nil(t, captured.Models)
	assert.Nil(t, captured.Keys)
	assert.Nil(t, captured.TimeZone)
	assert.Nil(t, captured.Tiers)
	// Provided fields are passed through.
	require.NotNil(t, captured.Description)
	assert.Equal(t, "new description", *captured.Description)
	assert.NotEmpty(t, captured.InstancePool)
	assert.NotEmpty(t, captured.ModelProtocols)
}

func TestProviderManager_UpdateProvider_ExplicitEmptyKeysTriggerHook(t *testing.T) {
	ctx := context.Background()

	var captured *ProviderParam
	store := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
			return &Provider{
				Name:           "deepseek",
				Models:         []string{"deepseek-chat"},
				Keys:           []ProviderKey{{Name: "key-primary", Key: "sk-aaaaaaaaaaaa"}},
				InstancePool:   []ProviderInstance{{Addr: "api.deepseek.com", Port: 443, Weight: 100}},
				ModelProtocols: []string{"openai"},
			}, nil
		},
		updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
			captured = param
			return nil
		},
	}
	m := NewProviderManager(&fakeTxn{}, store)

	hookCalled := 0
	hook := func(ctx context.Context, oldProvider, newProvider *Provider) error {
		hookCalled++
		return nil
	}

	// Explicitly passing an empty key list is an intentional clear; the key
	// ref checker hook must run, and the storager must receive a non-nil
	// empty slice (not nil, which would mean "keep existing").
	param := &ProviderParam{
		Name:           lib.PString("deepseek"),
		Keys:           []ProviderKey{},
		InstancePool:   []ProviderInstance{{Addr: "api.deepseek.com", Port: 443, Weight: 100}},
		ModelProtocols: []string{"openai"},
	}
	err := m.UpdateProvider(ctx, "deepseek", param, hook)
	require.NoError(t, err)
	assert.Equal(t, 1, hookCalled)
	require.NotNil(t, captured)
	require.NotNil(t, captured.Keys)
	assert.Empty(t, captured.Keys)
}

func TestProviderManager_DeleteProvider(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		deleted := false
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{Name: "deepseek"}, nil
			},
			deleteFn: func(ctx context.Context, name string) error {
				deleted = true
				return nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		err := m.DeleteProvider(ctx, "deepseek")
		require.NoError(t, err)
		assert.True(t, deleted)
	})

	t.Run("referenced", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{Name: "deepseek"}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		err := m.DeleteProvider(ctx, "deepseek", func(ctx context.Context, name string) error {
			return errors.New("referenced by cluster")
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "referenced by cluster")
	})
}

func TestProviderManager_FetchProvider(t *testing.T) {
	ctx := context.Background()
	expected := &Provider{Name: "deepseek"}
	store := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
			return expected, nil
		},
	}
	m := NewProviderManager(&fakeTxn{}, store)
	got, err := m.FetchProvider(ctx, &ProviderFilter{Name: lib.PString("deepseek")})
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestProviderManager_ListProviderNames(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchNamesFn: func(ctx context.Context) ([]string, error) {
				return []string{"deepseek", "openai", "anthropic"}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		names, err := m.ListProviderNames(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"deepseek", "openai", "anthropic"}, names)
	})

	t.Run("empty", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchNamesFn: func(ctx context.Context) ([]string, error) {
				return []string{}, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		names, err := m.ListProviderNames(ctx)
		require.NoError(t, err)
		assert.Empty(t, names)
	})
}

func TestProviderManager_FetchProviderList(t *testing.T) {
	ctx := context.Background()

	t.Run("no filter", func(t *testing.T) {
		expected := []*Provider{{Name: "deepseek"}}
		store := &fakeProviderStorager{
			fetchListFn: func(ctx context.Context, filter *ProviderFilter) ([]*Provider, int64, error) {
				return expected, 1, nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		list, total, err := m.FetchProviderList(ctx, &ProviderFilter{})
		require.NoError(t, err)
		assert.Equal(t, expected, list)
		assert.Equal(t, int64(1), total)
	})

	t.Run("filter by model_protocol", func(t *testing.T) {
		all := []*Provider{
			{Name: "openai-provider", ModelProtocols: []string{"openai"}},
			{Name: "anthropic-provider", ModelProtocols: []string{"anthropic"}},
			{Name: "multi-provider", ModelProtocols: []string{"openai", "anthropic"}},
		}
		store := &fakeProviderStorager{
			fetchListFn: func(ctx context.Context, filter *ProviderFilter) ([]*Provider, int64, error) {
				return all, int64(len(all)), nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		proto := "openai"
		list, total, err := m.FetchProviderList(ctx, &ProviderFilter{ModelProtocol: &proto})
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		require.Len(t, list, 2)
		assert.Equal(t, "openai-provider", list[0].Name)
		assert.Equal(t, "multi-provider", list[1].Name)
	})

	t.Run("no pagination returns all", func(t *testing.T) {
		all := []*Provider{
			{Name: "deepseek"},
			{Name: "openai"},
			{Name: "anthropic"},
		}
		store := &fakeProviderStorager{
			fetchListFn: func(ctx context.Context, filter *ProviderFilter) ([]*Provider, int64, error) {
				return all, int64(len(all)), nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, store)
		list, total, err := m.FetchProviderList(ctx, &ProviderFilter{})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Equal(t, all, list)
	})

}

func TestProviderManager_DiscoverModels(t *testing.T) {
	ctx := context.Background()

	t.Run("success openai", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.deepseek.com",
			Port:          443,
			URI:           "/v1/models",
			APIKey:        "sk-xxx",
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		models, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, []string{"m1", "m2"}, models)
	})

	t.Run("success anthropic", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "anthropic",
			Schema:        "https",
			Addr:          "api.anthropic.com",
			Port:          443,
			URI:           "/v1/models",
			APIKey:        "sk-xxx",
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"models":[{"model_id":"claude-3-opus-20240229","display_name":"Claude 3 Opus"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		models, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, []string{"claude-3-opus-20240229"}, models)
	})

	t.Run("success gemini", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "gemini",
			Schema:        "https",
			Addr:          "generativelanguage.googleapis.com",
			Port:          443,
			APIKey:        "sk-xxx",
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-2.5-flash"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		models, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, []string{"gemini-2.5-pro", "gemini-2.5-flash"}, models)
		assert.Equal(t, "https://generativelanguage.googleapis.com:443/v1beta/models", caller.lastURL)
		assert.Equal(t, "sk-xxx", caller.lastHeaders["x-goog-api-key"])
	})

	t.Run("default uri", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
			APIKey:        "sk-xxx",
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"data":[{"id":"m1"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		models, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, []string{"m1"}, models)
	})

	t.Run("no apikey", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"data":[{"id":"m1"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		models, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, []string{"m1"}, models)
	})

	t.Run("invalid model_protocol", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "unknown",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
		}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		_, err := m.DiscoverModelsWithCaller(ctx, param, &fakeDiscoverCaller{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid model_protocol")
	})

	t.Run("missing addr", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Port:          443,
		}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		_, err := m.DiscoverModelsWithCaller(ctx, param, &fakeDiscoverCaller{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "addr is required")
	})

	t.Run("url and auth header", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
			URI:           "/v1/models",
			APIKey:        "sk-xxx",
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"data":[{"id":"m1"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		_, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, "https://api.example.com:443/v1/models", caller.lastURL)
		assert.Equal(t, "Bearer sk-xxx", caller.lastHeaders["Authorization"])
	})

	t.Run("no auth header when apikey empty", func(t *testing.T) {
		param := &DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "http",
			Addr:          "api.example.com",
			Port:          80,
		}
		caller := &fakeDiscoverCaller{body: []byte(`{"data":[{"id":"m1"}]}`)}
		m := NewProviderManager(&fakeTxn{}, &fakeProviderStorager{})
		_, err := m.DiscoverModelsWithCaller(ctx, param, caller)
		require.NoError(t, err)
		assert.Equal(t, "http://api.example.com:80/v1/models", caller.lastURL)
		assert.Empty(t, caller.lastHeaders["Authorization"])
	})
}

func TestValidateProviderParam(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, ValidateProviderParam(validProviderParam()))
	})

	t.Run("nil", func(t *testing.T) {
		require.Error(t, ValidateProviderParam(nil))
	})

	t.Run("missing name", func(t *testing.T) {
		p := validProviderParam()
		p.Name = nil
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("empty instance pool", func(t *testing.T) {
		p := validProviderParam()
		p.InstancePool = nil
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("empty model protocols", func(t *testing.T) {
		p := validProviderParam()
		p.ModelProtocols = nil
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("invalid protocol", func(t *testing.T) {
		p := validProviderParam()
		p.ModelProtocols = []string{"unknown"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("gemini protocol", func(t *testing.T) {
		p := validProviderParam()
		p.ModelProtocols = []string{"gemini"}
		require.NoError(t, ValidateProviderParam(p))
	})

	t.Run("duplicate protocol", func(t *testing.T) {
		p := validProviderParam()
		p.ModelProtocols = []string{"openai", "openai"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("invalid endpoint uri", func(t *testing.T) {
		p := validProviderParam()
		p.ModelEndpoint.URI = "models"
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("duplicate model", func(t *testing.T) {
		p := validProviderParam()
		p.Models = []string{"m", "m"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("nil models kept optional for partial update", func(t *testing.T) {
		p := validProviderParam()
		p.Models = nil
		require.NoError(t, ValidateProviderParam(p))
	})

	t.Run("empty models rejected", func(t *testing.T) {
		p := validProviderParam()
		p.Models = []string{}
		err := ValidateProviderParam(p)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one element")
	})

	t.Run("blank model element", func(t *testing.T) {
		p := validProviderParam()
		p.Models = []string{"m", " "}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("duplicate key name", func(t *testing.T) {
		p := validProviderParam()
		p.Keys = []ProviderKey{{Name: "k", Key: "a"}, {Name: "k", Key: "b"}}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("invalid time zone", func(t *testing.T) {
		p := validProviderParam()
		p.TimeZone = lib.PString("Not/AZone")
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("valid time zone", func(t *testing.T) {
		p := validProviderParam()
		p.TimeZone = lib.PString("UTC")
		require.NoError(t, ValidateProviderParam(p))
	})

	t.Run("nil time zone kept optional", func(t *testing.T) {
		p := validProviderParam()
		p.TimeZone = nil
		require.NoError(t, ValidateProviderParam(p))
	})
}

func TestProviderName(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, ProviderName("deepseek"))
	})

	t.Run("empty", func(t *testing.T) {
		require.Error(t, ProviderName(""))
	})

	t.Run("invalid chars", func(t *testing.T) {
		require.Error(t, ProviderName("deep seek"))
	})

	t.Run("edge chars", func(t *testing.T) {
		require.Error(t, ProviderName("-deepseek"))
	})
}

func TestFillDefaults(t *testing.T) {
	p := &ProviderParam{}
	FillDefaults(p)
	require.NotNil(t, p.ModelEndpoint)
	assert.Equal(t, "https", p.ModelEndpoint.Schema)
	assert.Equal(t, "/v1/models", p.ModelEndpoint.URI)
	assert.NotNil(t, p.Models)
	assert.NotNil(t, p.Keys)
}

func TestKeyMap(t *testing.T) {
	keys := []ProviderKey{{Name: "a", Key: "1"}, {Name: "b", Key: "2"}}
	m := KeyMap(keys)
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, m)
}

func TestHasModel(t *testing.T) {
	p := &Provider{Models: []string{"m1"}}
	assert.True(t, HasModel(p, "m1"))
	assert.False(t, HasModel(p, "m2"))
	assert.False(t, HasModel(nil, "m1"))
}

func TestValidateDiscoverModelsParam(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, ValidateDiscoverModelsParam(&DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
			URI:           "/v1/models",
			APIKey:        "sk-xxx",
		}))
	})

	t.Run("nil", func(t *testing.T) {
		require.Error(t, ValidateDiscoverModelsParam(nil))
	})

	t.Run("missing model_protocol", func(t *testing.T) {
		require.Error(t, ValidateDiscoverModelsParam(&DiscoverModelsParam{
			Schema: "https",
			Addr:   "api.example.com",
			Port:   443,
		}))
	})

	t.Run("invalid model_protocol", func(t *testing.T) {
		require.Error(t, ValidateDiscoverModelsParam(&DiscoverModelsParam{
			ModelProtocol: "unknown",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
		}))
	})

	t.Run("invalid schema", func(t *testing.T) {
		require.Error(t, ValidateDiscoverModelsParam(&DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "ftp",
			Addr:          "api.example.com",
			Port:          443,
		}))
	})

	t.Run("invalid port", func(t *testing.T) {
		require.Error(t, ValidateDiscoverModelsParam(&DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          0,
		}))
	})

	t.Run("uri must start with slash", func(t *testing.T) {
		require.Error(t, ValidateDiscoverModelsParam(&DiscoverModelsParam{
			ModelProtocol: "openai",
			Schema:        "https",
			Addr:          "api.example.com",
			Port:          443,
			URI:           "v1/models",
		}))
	})
}

func TestBuildAuthHeader(t *testing.T) {
	k, v := BuildAuthHeader("openai", "sk-xxx")
	assert.Equal(t, "Authorization", k)
	assert.Equal(t, "Bearer sk-xxx", v)

	k, v = BuildAuthHeader("anthropic", "sk-xxx")
	assert.Equal(t, "x-api-key", k)
	assert.Equal(t, "sk-xxx", v)

	k, v = BuildAuthHeader("gemini", "sk-xxx")
	assert.Equal(t, "x-goog-api-key", k)
	assert.Equal(t, "sk-xxx", v)
}

func TestParseModelDiscoveryResponse(t *testing.T) {
	t.Run("openai data", func(t *testing.T) {
		models, err := ParseModelDiscoveryResponse([]byte(`{"data":[{"id":"m1"},{"id":"m2"}]}`), "openai")
		require.NoError(t, err)
		assert.Equal(t, []string{"m1", "m2"}, models)
	})

	t.Run("anthropic models", func(t *testing.T) {
		body := []byte(`{"models":[{"model_id":"claude-3-opus-20240229","display_name":"Claude 3 Opus"},{"model_id":"claude-3-sonnet-20240229","display_name":"Claude 3 Sonnet"}]}`)
		models, err := ParseModelDiscoveryResponse(body, "anthropic")
		require.NoError(t, err)
		assert.Equal(t, []string{"claude-3-opus-20240229", "claude-3-sonnet-20240229"}, models)
	})

	t.Run("gemini models with prefix stripped", func(t *testing.T) {
		body := []byte(`{"models":[{"name":"models/gemini-2.5-pro"},{"name":"models/gemini-2.5-flash"}]}`)
		models, err := ParseModelDiscoveryResponse(body, "gemini")
		require.NoError(t, err)
		assert.Equal(t, []string{"gemini-2.5-pro", "gemini-2.5-flash"}, models)
	})

	t.Run("generic fallback models array with name", func(t *testing.T) {
		models, err := ParseModelDiscoveryResponse([]byte(`{"models":[{"name":"m1"},{"name":"m2"}]}`), "custom-protocol")
		require.NoError(t, err)
		assert.Equal(t, []string{"m1", "m2"}, models)
	})

	t.Run("generic fallback models array of strings", func(t *testing.T) {
		models, err := ParseModelDiscoveryResponse([]byte(`{"models":["m1","m2"]}`), "custom-protocol")
		require.NoError(t, err)
		assert.Equal(t, []string{"m1", "m2"}, models)
	})

	t.Run("generic fallback single model object", func(t *testing.T) {
		models, err := ParseModelDiscoveryResponse([]byte(`{"models":{"name":"m1"}}`), "custom-protocol")
		require.NoError(t, err)
		assert.Equal(t, []string{"m1"}, models)
	})

	t.Run("empty", func(t *testing.T) {
		_, err := ParseModelDiscoveryResponse([]byte(`{}`), "openai")
		require.Error(t, err)
	})
}

func TestValidatePricingTiersParam(t *testing.T) {
	t.Run("valid peak tier", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "Asia/Shanghai",
			Tiers: []PricingTier{
				{
					Name: "peak",
					TimeRanges: []TimeRange{
						{Weekdays: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "12:00"},
						{Weekdays: []int{1, 2, 3, 4, 5}, Start: "14:00", End: "18:00"},
					},
				},
			},
		}
		require.NoError(t, ValidatePricingTiersParam(param))
	})

	t.Run("empty tiers allowed", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "Asia/Shanghai",
			Tiers:    []PricingTier{},
		}
		require.NoError(t, ValidatePricingTiersParam(param))
	})

	t.Run("default time zone", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "",
			Tiers:    []PricingTier{},
		}
		require.NoError(t, ValidatePricingTiersParam(param))
	})

	t.Run("invalid time zone", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "Invalid/TimeZone",
			Tiers:    []PricingTier{},
		}
		require.Error(t, ValidatePricingTiersParam(param))
	})

	t.Run("unsupported tier name", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "Asia/Shanghai",
			Tiers: []PricingTier{
				{Name: "off_peak", TimeRanges: []TimeRange{{Start: "00:00", End: "08:00"}}},
			},
		}
		require.Error(t, ValidatePricingTiersParam(param))
	})

	t.Run("end must be greater than start", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "Asia/Shanghai",
			Tiers: []PricingTier{
				{Name: "peak", TimeRanges: []TimeRange{{Start: "12:00", End: "09:00"}}},
			},
		}
		require.Error(t, ValidatePricingTiersParam(param))
	})

	t.Run("invalid weekday", func(t *testing.T) {
		param := &PricingTiersParam{
			TimeZone: "Asia/Shanghai",
			Tiers: []PricingTier{
				{Name: "peak", TimeRanges: []TimeRange{{Weekdays: []int{7}, Start: "09:00", End: "12:00"}}},
			},
		}
		require.Error(t, ValidatePricingTiersParam(param))
	})
}

func TestProviderInstancePoolEqual(t *testing.T) {
	a := []ProviderInstance{
		{Addr: "1.2.3.4", Port: 443, Weight: 100, Disable: false},
	}
	b := []ProviderInstance{
		{Addr: "1.2.3.4", Port: 443, Weight: 100, Disable: false},
	}
	c := []ProviderInstance{
		{Addr: "1.2.3.4", Port: 8443, Weight: 100, Disable: false},
	}

	assert.True(t, providerInstancePoolEqual(a, b))
	assert.False(t, providerInstancePoolEqual(a, c))
	assert.False(t, providerInstancePoolEqual(a, nil))
	assert.True(t, providerInstancePoolEqual(nil, nil))
}

func TestApplyProviderUpdate(t *testing.T) {
	existing := &Provider{
		Name:        "deepseek",
		Description: "old",
		Models:      []string{"m1"},
		InstancePool: []ProviderInstance{
			{Addr: "1.2.3.4", Port: 443, Weight: 100},
		},
	}

	param := &ProviderParam{
		Description: lib.PString("new"),
		Models:      []string{"m1", "m2"},
		InstancePool: []ProviderInstance{
			{Addr: "5.6.7.8", Port: 443, Weight: 100},
		},
	}

	updated := applyProviderUpdate(existing, param)
	require.NotNil(t, updated)
	assert.Equal(t, "deepseek", updated.Name)
	assert.Equal(t, "new", updated.Description)
	assert.Equal(t, []string{"m1", "m2"}, updated.Models)
	// Unchanged fields are preserved.
	assert.Equal(t, existing.ModelProtocols, updated.ModelProtocols)
}

func TestValidateProviderParam_ProtocolPaths(t *testing.T) {
	base := func() *ProviderParam {
		p := validProviderParam()
		p.ModelProtocols = []string{"openai", "anthropic"}
		return p
	}

	t.Run("nil protocol paths is valid", func(t *testing.T) {
		require.NoError(t, ValidateProviderParam(base()))
	})

	t.Run("empty protocol paths is valid", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{}
		require.NoError(t, ValidateProviderParam(p))
	})

	t.Run("valid provider paths", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{
			"openai":    "/compatible-mode/v1",
			"anthropic": "/apps/anthropic",
		}
		require.NoError(t, ValidateProviderParam(p))
	})

	t.Run("unsupported protocol key", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"gemini": "/v1beta"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("key not declared in model_protocols", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"anthropic": "/apps/anthropic"}
		p.ModelProtocols = []string{"openai"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("empty base path", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"openai": ""}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("missing leading slash", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"openai": "compatible-mode/v1"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("trailing slash", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"openai": "/compatible-mode/"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("root slash", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"openai": "/"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("dot dot", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"openai": "/.. /x"}
		require.Error(t, ValidateProviderParam(p))
	})

	t.Run("query and fragment", func(t *testing.T) {
		for _, base0 := range []string{"/api?v=1", "/api#frag"} {
			p := base()
			p.ProtocolPaths = map[string]string{"openai": base0}
			require.Error(t, ValidateProviderParam(p))
		}
	})

	t.Run("too long", func(t *testing.T) {
		p := base()
		p.ProtocolPaths = map[string]string{"openai": "/" + strings.Repeat("a", 128)}
		require.Error(t, ValidateProviderParam(p))
	})
}

func TestApplyProviderUpdate_ProtocolPaths(t *testing.T) {
	existing := &Provider{
		Name:           "bailian",
		ModelProtocols: []string{"openai", "anthropic"},
		ProtocolPaths:  map[string]string{"openai": "/compatible-mode/v1", "anthropic": "/apps/anthropic"},
	}

	t.Run("nil param preserves existing", func(t *testing.T) {
		updated := applyProviderUpdate(existing, &ProviderParam{Name: lib.PString("bailian")})
		require.NotNil(t, updated)
		assert.Equal(t, existing.ProtocolPaths, updated.ProtocolPaths)
	})

	t.Run("explicit empty map clears", func(t *testing.T) {
		updated := applyProviderUpdate(existing, &ProviderParam{
			Name:          lib.PString("bailian"),
			ProtocolPaths: map[string]string{},
		})
		require.NotNil(t, updated)
		assert.Equal(t, map[string]string{}, updated.ProtocolPaths)
	})

	t.Run("new value replaces", func(t *testing.T) {
		updated := applyProviderUpdate(existing, &ProviderParam{
			Name:          lib.PString("bailian"),
			ProtocolPaths: map[string]string{"openai": "/v1"},
		})
		require.NotNil(t, updated)
		assert.Equal(t, map[string]string{"openai": "/v1"}, updated.ProtocolPaths)
	})
}

func TestProviderManager_UpdatePricingTiers_PreservesProtocolPaths(t *testing.T) {
	existing := &Provider{
		Name:           "bailian",
		Description:    "desc",
		ModelProtocols: []string{"openai", "anthropic"},
		ProtocolPaths:  map[string]string{"openai": "/compatible-mode/v1", "anthropic": "/apps/anthropic"},
	}
	var updated *ProviderParam
	storager := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
			return existing, nil
		},
		updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
			updated = param
			return nil
		},
	}
	m := NewProviderManager(&fakeTxn{}, storager)
	err := m.UpdatePricingTiers(context.Background(), "bailian", &PricingTiersParam{
		TimeZone: "Asia/Shanghai",
		Tiers: []PricingTier{
			{Name: "peak", TimeRanges: []TimeRange{{Start: "09:00", End: "12:00"}}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, existing.ProtocolPaths, updated.ProtocolPaths)
}

func TestProviderManager_UpdatePricingTiers_LegacyEmptyModels(t *testing.T) {
	// A legacy provider stored with an empty models list must remain
	// maintainable: models stays nil so the storager's nil-skip preserves
	// the stored value.
	existing := &Provider{
		Name:   "legacy",
		Models: []string{},
	}
	var updated *ProviderParam
	storager := &fakeProviderStorager{
		fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
			return existing, nil
		},
		updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
			updated = param
			return nil
		},
	}
	m := NewProviderManager(&fakeTxn{}, storager)
	err := m.UpdatePricingTiers(context.Background(), "legacy", &PricingTiersParam{
		TimeZone: "Asia/Shanghai",
		Tiers: []PricingTier{
			{Name: "peak", TimeRanges: []TimeRange{{Start: "09:00", End: "12:00"}}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Nil(t, updated.Models)
}

func TestProviderManager_UpdateProvider_EffectiveProtocolPaths(t *testing.T) {
	existing := &Provider{
		Name:           "bailian",
		Description:    "desc",
		ModelProtocols: []string{"openai", "anthropic"},
		ProtocolPaths:  map[string]string{"openai": "/v1", "anthropic": "/anthropic"},
	}
	baseParam := func() *ProviderParam {
		return &ProviderParam{
			Name:           lib.PString("bailian"),
			InstancePool:   []ProviderInstance{{Addr: "api.example.com", Port: 443, Weight: 100}},
			ModelProtocols: []string{"openai"},
		}
	}

	t.Run("omitted protocol_paths orphaned by shrunk protocols rejected", func(t *testing.T) {
		updateCalled := false
		storager := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return existing, nil
			},
			updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
				updateCalled = true
				return nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, storager)
		err := m.UpdateProvider(context.Background(), "bailian", baseParam())
		require.Error(t, err)
		assert.False(t, updateCalled, "storager must not be called when effective check fails")
	})

	t.Run("protocol_paths provided matching shrunk protocols passes", func(t *testing.T) {
		updateCalled := false
		storager := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return existing, nil
			},
			updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
				updateCalled = true
				return nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, storager)
		param := baseParam()
		param.ProtocolPaths = map[string]string{"openai": "/v1"}
		err := m.UpdateProvider(context.Background(), "bailian", param)
		require.NoError(t, err)
		assert.True(t, updateCalled)
	})

	t.Run("protocols unchanged passes with stored paths", func(t *testing.T) {
		updateCalled := false
		storager := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return existing, nil
			},
			updateFn: func(ctx context.Context, name string, param *ProviderParam) error {
				updateCalled = true
				return nil
			},
		}
		m := NewProviderManager(&fakeTxn{}, storager)
		param := baseParam()
		param.ModelProtocols = []string{"openai", "anthropic"}
		err := m.UpdateProvider(context.Background(), "bailian", param)
		require.NoError(t, err)
		assert.True(t, updateCalled)
	})
}
