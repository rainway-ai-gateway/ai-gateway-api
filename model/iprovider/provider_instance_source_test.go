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
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestK8sPoolName(t *testing.T) {
	valid := []string{"svc-a", "svc_a", "svc.a", "a", "A1-b2.c3_d4"}
	for _, s := range valid {
		require.NoError(t, K8sPoolName(s), "name %q should be valid", s)
	}

	invalid := []string{
		"",                     // empty
		"-abc", "abc-", "_abc", // leading/trailing '-', '_'
		".abc", "abc.", // leading/trailing '.'
		"ab cd", "ab\tcd", // whitespace
		"ab/cd", "ab:cd", // invalid characters
	}
	for _, s := range invalid {
		require.Error(t, K8sPoolName(s), "name %q should be invalid", s)
	}

	require.NoError(t, K8sPoolName(strings.Repeat("a", MaxK8sPoolNameLength)), "max length name should be valid")
	require.Error(t, K8sPoolName(strings.Repeat("a", MaxK8sPoolNameLength+1)), "over max length name should be invalid")
}

func TestValidateProviderParam_InstanceSource(t *testing.T) {
	base := func() *ProviderParam {
		return validProviderParam()
	}

	t.Run("default source keeps legacy instance_pool rules", func(t *testing.T) {
		require.NoError(t, ValidateProviderParam(base()))
	})

	t.Run("explicit instance_pool source", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceInstancePool)
		require.NoError(t, ValidateProviderParam(param))
	})

	t.Run("instance_pool mode rejects empty pool", func(t *testing.T) {
		param := base()
		param.InstancePool = nil
		require.Error(t, ValidateProviderParam(param))
	})

	t.Run("k8s_pool mode requires k8s_pool_name", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		err := ValidateProviderParam(param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "k8s_pool_name is required")
	})

	t.Run("k8s_pool mode accepts empty k8s_pool_name rejection", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("")
		require.Error(t, ValidateProviderParam(param))
	})

	t.Run("k8s_pool mode allows empty dormant instance_pool", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("svc-a")
		param.InstancePool = nil
		require.NoError(t, ValidateProviderParam(param))
	})

	t.Run("k8s_pool mode does not validate dormant instance_pool members", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("svc-a")
		param.InstancePool = []ProviderInstance{
			{Addr: "", Port: 0, Weight: 200},
			{Addr: "", Port: 0, Weight: 200},
		}
		require.NoError(t, ValidateProviderParam(param))
	})

	t.Run("invalid instance_source enum rejected", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString("unknown")
		err := ValidateProviderParam(param)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid instance_source")
	})

	t.Run("k8s_pool_name format enforced in k8s_pool mode", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("-bad-")
		require.Error(t, ValidateProviderParam(param))
	})

	t.Run("k8s_pool_name dormant but format-checked in instance_pool mode", func(t *testing.T) {
		param := base()
		param.InstanceSource = lib.PString(InstanceSourceInstancePool)
		param.K8sPoolName = lib.PString("svc-a")
		require.NoError(t, ValidateProviderParam(param))

		param.K8sPoolName = lib.PString("bad name")
		require.Error(t, ValidateProviderParam(param))
	})
}

func TestValidateProviderParam_InstancePoolHostname(t *testing.T) {
	base := func() *ProviderParam {
		p := validProviderParam()
		// two instances with weight 0/100, mirroring the E2E repro shape
		p.InstancePool = []ProviderInstance{
			{Addr: "api.deepseek.com", Port: 443, Weight: 0},
			{Addr: "api.anthropic.com", Port: 443, Weight: 100},
		}
		return p
	}

	t.Run("baseline legal hostnames accepted", func(t *testing.T) {
		require.NoError(t, ValidateProviderParam(base()))
	})

	invalidAddrs := []struct {
		name string
		addr string
	}{
		{"issue repro leading hyphen", "-bad"},
		{"trailing hyphen", "bad-"},
		{"empty label", "b..d"},
		{"label too long", strings.Repeat("a", 64)},
		{"total too long", strings.Repeat("a.", 128) + "ab"},
		{"single char", "x"},
		{"whitespace", "bad addr"},
		{"underscore not in charset", "bad_addr"},
	}
	for _, c := range invalidAddrs {
		t.Run("rejects "+c.name, func(t *testing.T) {
			param := base()
			param.InstancePool[0].Addr = c.addr
			err := ValidateProviderParam(param)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "instance addr")
		})
	}

	validAddrs := []struct {
		name string
		addr string
	}{
		{"hostname", "api.example.com"},
		{"shortest legal", "a1"},
		{"ipv4 literal", "1.2.3.4"},
		{"ipv6 literal", "2001:db8::1"},
	}
	for _, c := range validAddrs {
		t.Run("accepts "+c.name, func(t *testing.T) {
			param := base()
			param.InstancePool[0].Addr = c.addr
			require.NoError(t, ValidateProviderParam(param))
		})
	}
}

func TestFillDefaults_InstanceSource(t *testing.T) {
	param := &ProviderParam{}
	FillDefaults(param)
	require.NotNil(t, param.InstanceSource)
	assert.Equal(t, InstanceSourceInstancePool, *param.InstanceSource)

	existing := InstanceSourceK8sPool
	param = &ProviderParam{InstanceSource: &existing}
	FillDefaults(param)
	assert.Equal(t, InstanceSourceK8sPool, *param.InstanceSource)
}

func TestApplyProviderUpdate_InstanceSource(t *testing.T) {
	existing := &Provider{
		Name:           "deepseek",
		InstanceSource: InstanceSourceInstancePool,
		InstancePool:   []ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
	}

	t.Run("nil new fields keep existing values", func(t *testing.T) {
		updated := applyProviderUpdate(existing, &ProviderParam{})
		assert.Equal(t, InstanceSourceInstancePool, updated.InstanceSource)
		assert.Nil(t, updated.K8sPoolName)
	})

	t.Run("explicit fields applied", func(t *testing.T) {
		poolName := "svc-a"
		updated := applyProviderUpdate(existing, &ProviderParam{
			InstanceSource: lib.PString(InstanceSourceK8sPool),
			K8sPoolName:    &poolName,
		})
		assert.Equal(t, InstanceSourceK8sPool, updated.InstanceSource)
		require.NotNil(t, updated.K8sPoolName)
		assert.Equal(t, "svc-a", *updated.K8sPoolName)
	})
}

func TestProviderManager_UpdateProvider_InstanceSourceHook(t *testing.T) {
	ctx := context.Background()

	t.Run("mode switch to k8s_pool triggers hook", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					InstanceSource: InstanceSourceInstancePool,
					InstancePool:   []ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
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
		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("svc-a")
		param.InstancePool = nil
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		require.Equal(t, 1, hookCalled)
		require.NotNil(t, hookNew)
		assert.Equal(t, InstanceSourceK8sPool, hookNew.InstanceSource)
	})

	t.Run("mode switch back to instance_pool triggers hook", func(t *testing.T) {
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					InstanceSource: InstanceSourceK8sPool,
					K8sPoolName:    lib.PString("svc-a"),
					InstancePool:   []ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
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
		param.InstanceSource = lib.PString(InstanceSourceInstancePool)
		param.K8sPoolName = nil // name stays dormant on the stored record
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 1, hookCalled)
	})

	t.Run("dormant instance_pool change in k8s_pool mode does not trigger hook", func(t *testing.T) {
		param := validProviderParam()
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					Models:         param.Models,
					Keys:           param.Keys,
					InstanceSource: InstanceSourceK8sPool,
					K8sPoolName:    lib.PString("svc-a"),
					InstancePool:   []ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
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

		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("svc-a")
		param.InstancePool = []ProviderInstance{{Addr: "5.6.7.8", Port: 443, Weight: 100}}
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 0, hookCalled)
	})

	t.Run("k8s_pool_name A to B in k8s mode does not trigger hook", func(t *testing.T) {
		param := validProviderParam()
		store := &fakeProviderStorager{
			fetchFn: func(ctx context.Context, filter *ProviderFilter) (*Provider, error) {
				return &Provider{
					Name:           "deepseek",
					Models:         param.Models,
					Keys:           param.Keys,
					InstanceSource: InstanceSourceK8sPool,
					K8sPoolName:    lib.PString("pool-a"),
					InstancePool:   []ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}},
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

		param.InstanceSource = lib.PString(InstanceSourceK8sPool)
		param.K8sPoolName = lib.PString("pool-b")
		param.InstancePool = nil
		err := m.UpdateProvider(ctx, "deepseek", param, hook)
		require.NoError(t, err)
		assert.Equal(t, 0, hookCalled, "placeholder row is unchanged, the exporter reads the new pool at export time")
	})
}
