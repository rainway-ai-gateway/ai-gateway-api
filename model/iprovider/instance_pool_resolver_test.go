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
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeK8sPoolInstanceReader struct {
	instances []ProviderInstance
	err       error
	calls     int
}

func (f *fakeK8sPoolInstanceReader) FetchK8sPoolInstances(ctx context.Context, poolName string) ([]ProviderInstance, error) {
	f.calls++
	return f.instances, f.err
}

func TestInstancePoolResolver_Effective(t *testing.T) {
	ctx := context.Background()
	manual := []ProviderInstance{{Addr: "1.2.3.4", Port: 443, Weight: 100}}
	k8sInstances := []ProviderInstance{{Addr: "10.0.0.1", Port: 8000, Weight: 100}}

	t.Run("nil provider yields nil nil", func(t *testing.T) {
		r := NewInstancePoolResolver(&fakeK8sPoolInstanceReader{})
		inst, err := r.Effective(ctx, nil)
		require.NoError(t, err)
		assert.Nil(t, inst)
	})

	t.Run("manual source returns instance_pool without touching the reader", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{}
		r := NewInstancePoolResolver(reader)
		inst, err := r.Effective(ctx, &Provider{InstanceSource: InstanceSourceInstancePool, InstancePool: manual})
		require.NoError(t, err)
		assert.Equal(t, manual, inst)
		assert.Equal(t, 0, reader.calls)

		// Legacy rows without instance_source resolve to the manual pool too.
		inst, err = r.Effective(ctx, &Provider{InstancePool: manual})
		require.NoError(t, err)
		assert.Equal(t, manual, inst)
	})

	t.Run("k8s source resolves through the reader by pool name", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{instances: k8sInstances}
		r := NewInstancePoolResolver(reader)
		inst, err := r.Effective(ctx, &Provider{
			InstanceSource: InstanceSourceK8sPool,
			K8sPoolName:    lib.PString("svc-a"),
			InstancePool:   manual, // dormant: must not leak into the result
		})
		require.NoError(t, err)
		assert.Equal(t, k8sInstances, inst)
		assert.Equal(t, 1, reader.calls)
	})

	t.Run("k8s source with empty pool name yields a non-nil empty list", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{}
		r := NewInstancePoolResolver(reader)
		inst, err := r.Effective(ctx, &Provider{InstanceSource: InstanceSourceK8sPool})
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Len(t, inst, 0)
		assert.Equal(t, 0, reader.calls)

		inst, err = r.Effective(ctx, &Provider{InstanceSource: InstanceSourceK8sPool, K8sPoolName: lib.PString("")})
		require.NoError(t, err)
		require.NotNil(t, inst)
		assert.Len(t, inst, 0)
	})

	t.Run("reader error propagates", func(t *testing.T) {
		reader := &fakeK8sPoolInstanceReader{err: errors.New("db down")}
		r := NewInstancePoolResolver(reader)
		_, err := r.Effective(ctx, &Provider{InstanceSource: InstanceSourceK8sPool, K8sPoolName: lib.PString("svc-a")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "db down")
	})

	t.Run("unconfigured resolver is an explicit error, not an empty pool", func(t *testing.T) {
		var r *InstancePoolResolver
		_, err := r.Effective(ctx, &Provider{InstanceSource: InstanceSourceK8sPool, K8sPoolName: lib.PString("svc-a")})
		require.Error(t, err)

		r = NewInstancePoolResolver(nil)
		_, err = r.Effective(ctx, &Provider{InstanceSource: InstanceSourceK8sPool, K8sPoolName: lib.PString("svc-a")})
		require.Error(t, err)
	})

	t.Run("manual source works without any resolver", func(t *testing.T) {
		var r *InstancePoolResolver
		inst, err := r.Effective(ctx, &Provider{InstanceSource: InstanceSourceInstancePool, InstancePool: manual})
		require.NoError(t, err)
		assert.Equal(t, manual, inst)
	})
}
