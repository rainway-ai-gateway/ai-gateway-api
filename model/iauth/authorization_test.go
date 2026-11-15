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

package iauth

import (
	"context"
	"errors"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeManager_Authorizate(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	t.Run("admin allowed all", func(t *testing.T) {
		m := NewAuthorizeManager(&fakeTxn{}, &fakeAuthorizeStorager{})
		v := &Visitor{User: &User{Name: "admin", Admin: true}}
		ctx := NewVisitorContext(ctx, v)

		auth := NewFeatureAuthorization(FeatureUser, ActionDelete)
		require.NoError(t, m.Authorizate(ctx, auth))
	})

	t.Run("product scope allowed read", func(t *testing.T) {
		m := NewAuthorizeManager(&fakeTxn{}, &fakeAuthorizeStorager{})
		v := &Visitor{Token: &Token{Name: "token", Scope: ScopeProduct}}
		ctx := NewVisitorContext(ctx, v)

		auth := NewFeatureAuthorization(FeatureUser, ActionReadAll)
		require.NoError(t, m.Authorizate(ctx, auth))
	})

	t.Run("product scope denied delete", func(t *testing.T) {
		m := NewAuthorizeManager(&fakeTxn{}, &fakeAuthorizeStorager{})
		v := &Visitor{Token: &Token{Name: "token", Scope: ScopeProduct}}
		ctx := NewVisitorContext(ctx, v)

		auth := NewFeatureAuthorization(FeatureUser, ActionDelete)
		err := m.Authorizate(ctx, auth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Feature Access Deny")
	})

	t.Run("unknown scope denied", func(t *testing.T) {
		m := NewAuthorizeManager(&fakeTxn{}, &fakeAuthorizeStorager{})
		v := &Visitor{Token: &Token{Name: "token", Scope: "UnknownScope"}}
		ctx := NewVisitorContext(ctx, v)

		auth := NewFeatureAuthorization(FeatureUser, ActionRead)
		err := m.Authorizate(ctx, auth)
		require.Error(t, err)
	})

	t.Run("product validation success", func(t *testing.T) {
		product := &ibasic.Product{Name: "product-1"}
		authStore := &fakeAuthorizeStorager{
			isTokenProductGrantedFn: func(ctx context.Context, token *Token, product *ibasic.Product) (bool, error) {
				return true, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)
		v := &Visitor{Token: &Token{Name: "token", Scope: ScopeProduct}}
		ctx := NewVisitorContext(ctx, v)
		ctx = ibasic.NewProductContext(ctx, product)

		auth := NewFeatureAuthorizerWithFactoryWithProduct(FeatureAPIKey, ActionRead)
		require.NoError(t, m.Authorizate(ctx, auth))
	})

	t.Run("product validation fail", func(t *testing.T) {
		product := &ibasic.Product{Name: "product-1"}
		authStore := &fakeAuthorizeStorager{
			isTokenProductGrantedFn: func(ctx context.Context, token *Token, product *ibasic.Product) (bool, error) {
				return false, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)
		v := &Visitor{Token: &Token{Name: "token", Scope: ScopeProduct}}
		ctx := NewVisitorContext(ctx, v)
		ctx = ibasic.NewProductContext(ctx, product)

		auth := NewFeatureAuthorizerWithFactoryWithProduct(FeatureAPIKey, ActionRead)
		err := m.Authorizate(ctx, auth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Product Access Deny")
	})

	t.Run("visitor not login", func(t *testing.T) {
		stateful.DefaultConfig.RunTime.SkipTokenValidate = false
		m := NewAuthorizeManager(&fakeTxn{}, &fakeAuthorizeStorager{})

		auth := NewFeatureAuthorization(FeatureUser, ActionRead)
		err := m.Authorizate(ctx, auth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "User Not Login")
	})
}

func TestAuthorizeManager_IsVisitorProductGranted(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	t.Run("user granted", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			isUserProductGrantedFn: func(ctx context.Context, user *User, product *ibasic.Product) (bool, error) {
				return true, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)
		v := &Visitor{User: &User{Name: "alice"}}

		ok, err := m.IsVisitorProductGranted(ctx, v, &ibasic.Product{Name: "p1"})
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("token granted", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			isTokenProductGrantedFn: func(ctx context.Context, token *Token, product *ibasic.Product) (bool, error) {
				return true, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)
		v := &Visitor{Token: &Token{Name: "token"}}

		ok, err := m.IsVisitorProductGranted(ctx, v, &ibasic.Product{Name: "p1"})
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

func TestAuthorizeManager_FetchVisitorProductList(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	t.Run("user products", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			fetchUserProductsFn: func(ctx context.Context, user *User) ([]*ibasic.Product, error) {
				return []*ibasic.Product{{Name: "p1"}}, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)
		v := &Visitor{User: &User{Name: "alice"}}

		products, err := m.FetchVisitorProductList(ctx, v)
		require.NoError(t, err)
		assert.Len(t, products, 1)
	})

	t.Run("token product", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			fetchTokenProductFn: func(ctx context.Context, token *Token) (*ibasic.Product, error) {
				return &ibasic.Product{Name: "p1"}, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)
		v := &Visitor{Token: &Token{Name: "token"}}

		products, err := m.FetchVisitorProductList(ctx, v)
		require.NoError(t, err)
		assert.Len(t, products, 1)
	})
}

func TestAuthorizeManager_FetchProductUsers(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	authStore := &fakeAuthorizeStorager{
		fetchProductUsersFn: func(ctx context.Context, product *ibasic.Product) ([]*User, error) {
			return []*User{{Name: "alice"}}, nil
		},
	}
	m := NewAuthorizeManager(&fakeTxn{}, authStore)

	users, err := m.FetchProductUsers(ctx, &ibasic.Product{Name: "p1"})
	require.NoError(t, err)
	assert.Len(t, users, 1)
}

func TestAuthorizeManager_BindUserProduct(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	authStore := &fakeAuthorizeStorager{
		bindUserProductFn: func(ctx context.Context, user *User, product *ibasic.Product) error {
			return nil
		},
	}
	m := NewAuthorizeManager(&fakeTxn{}, authStore)

	require.NoError(t, m.BindUserProduct(ctx, &User{Name: "alice"}, &ibasic.Product{Name: "p1"}))
}

func TestAuthorizeManager_UnBindUserProduct(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	t.Run("success", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			isUserProductGrantedFn: func(ctx context.Context, user *User, product *ibasic.Product) (bool, error) {
				return true, nil
			},
			unbindUserProductFn: func(ctx context.Context, user *User, product *ibasic.Product) error {
				return nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		require.NoError(t, m.UnBindUserProduct(ctx, &User{Name: "alice"}, &ibasic.Product{Name: "p1"}))
	})

	t.Run("binding not exist", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			isUserProductGrantedFn: func(ctx context.Context, user *User, product *ibasic.Product) (bool, error) {
				return false, nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		err := m.UnBindUserProduct(ctx, &User{Name: "alice"}, &ibasic.Product{Name: "p1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "User Product Binding Record Not Exist")
	})
}

func TestAuthorizeManager_FetchProductTokens(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	authStore := &fakeAuthorizeStorager{
		fetchProductTokensFn: func(ctx context.Context, product *ibasic.Product) ([]*Token, error) {
			return []*Token{{Name: "token-1"}}, nil
		},
	}
	m := NewAuthorizeManager(&fakeTxn{}, authStore)

	tokens, err := m.FetchProductTokens(ctx, &ibasic.Product{Name: "p1"})
	require.NoError(t, err)
	assert.Len(t, tokens, 1)
}

func TestAuthorizeManager_UpdateUserRole(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	t.Run("set system admin", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			updateUserScopesFn: func(ctx context.Context, user *User, scopes []string) error {
				assert.Equal(t, []string{ScopeSystem}, scopes)
				return nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		require.NoError(t, m.UpdateUserRole(ctx, &User{Name: "alice"}, RoleSystemAdmin))
	})

	t.Run("set readonly admin", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			updateUserScopesFn: func(ctx context.Context, user *User, scopes []string) error {
				assert.Equal(t, []string{ScopeSystemReadonly}, scopes)
				return nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		require.NoError(t, m.UpdateUserRole(ctx, &User{Name: "alice"}, RoleSystemAdminReadonly))
	})

	t.Run("invalid role", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			updateUserScopesFn: func(ctx context.Context, user *User, scopes []string) error {
				t.Fatal("UpdateUserScopes must not be called for an invalid role")
				return nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		require.Error(t, m.UpdateUserRole(ctx, &User{Name: "alice"}, "other"))
	})

	t.Run("builtin admin downgrade rejected", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			updateUserScopesFn: func(ctx context.Context, user *User, scopes []string) error {
				t.Fatal("UpdateUserScopes must not be called when downgrading builtin admin")
				return nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		require.Error(t, m.UpdateUserRole(ctx, &User{Name: "admin"}, RoleSystemAdminReadonly))
		// Case-insensitive variant is rejected as well.
		require.Error(t, m.UpdateUserRole(ctx, &User{Name: "Admin"}, RoleSystemAdminReadonly))
	})

	t.Run("builtin admin pinned write", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			updateUserScopesFn: func(ctx context.Context, user *User, scopes []string) error {
				assert.Equal(t, []string{ScopeSystem}, scopes)
				return nil
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		// Pinning builtin admin to system_admin is an allowed idempotent write.
		require.NoError(t, m.UpdateUserRole(ctx, &User{Name: "admin"}, RoleSystemAdmin))
	})

	t.Run("storage error", func(t *testing.T) {
		authStore := &fakeAuthorizeStorager{
			updateUserScopesFn: func(ctx context.Context, user *User, scopes []string) error {
				return errors.New("db error")
			},
		}
		m := NewAuthorizeManager(&fakeTxn{}, authStore)

		err := m.UpdateUserRole(ctx, &User{Name: "alice"}, RoleSystemAdmin)
		require.Error(t, err)
	})
}

// allFeatures lists every Feature constant. Tests assert that
// scope2permission entries cover this list exactly, so a newly added Feature
// that nobody granted is caught here instead of silently defaulting to deny.
var allFeatures = []Feature{
	FeatureProxyPool, FeatureBFECluster, FeatureBFEPool, FeatureArea, FeatureDomain,
	FeatureProduct, FeatureExtraFile,
	FeatureProductPool, FeatureRoute, FeatureSubCluster, FeatureProductCluster,
	FeatureGeneral, FeatureTraffic, FeatureCert, FeatureActiveHealthCheck,
	FeatureProductUser, FeatureUser, FeatureToken,
	FeatureNLBPool, FeatureNLBCluster, FeatureAIRoute, FeatureAPIKey,
	FeatureEntityType, FeatureEntity, FeatureQuotaPlan, FeatureRateLimitPolicy,
	FeatureAICache, FeatureAIContext, FeatureTrafficMirror, FeatureAIIntent,
	FeatureAccessLog,
	FeatureModelPrice, FeatureProvider,
	FeatureK8sPool, FeatureOperationLog, FeatureReport, FeatureSecurity, FeatureBatch,
}

func TestScope2Permission_SystemReadonlyCoversAllFeatures(t *testing.T) {
	readonly, ok := scope2permission[ScopeSystemReadonly]
	require.True(t, ok, "SystemReadonly scope missing from scope2permission")

	writeActions := ActionUpdate | ActionCreate | ActionDelete | ActionExport | ActionCancel
	for _, f := range allFeatures {
		a, exists := readonly[f]
		require.True(t, exists, "SystemReadonly must explicitly list Feature %s", f)
		assert.True(t, a.IsAllowed(ActionRead), "%s must allow ActionRead", f)
		assert.False(t, a.IsAllowed(writeActions),
			"%s must not allow update/create/delete/export/cancel", f)
	}
	assert.Len(t, readonly, len(allFeatures), "no extra entries allowed (unlisted defaults to deny)")
}

func TestScope2Permission_SystemComplete(t *testing.T) {
	// The System entry must enumerate every Feature too. Today admins pass
	// through the IsAdmin() short-circuit so this is behaviour-preserving, but
	// it removes the hidden dependency should that short-circuit ever change.
	system, ok := scope2permission[ScopeSystem]
	require.True(t, ok)
	for _, f := range allFeatures {
		a, exists := system[f]
		require.True(t, exists, "System must explicitly list Feature %s", f)
		assert.Equal(t, actionAll, a, "%s must grant actionAll", f)
	}
	assert.Len(t, system, len(allFeatures))
}

func TestAuthorizeManager_Authorizate_SystemReadonly(t *testing.T) {
	ctx := context.Background()
	setupConfig(t)

	m := NewAuthorizeManager(&fakeTxn{}, &fakeAuthorizeStorager{})
	v := &Visitor{User: &User{Name: "viewer", Scopes: []string{ScopeSystemReadonly}}}
	ctx = NewVisitorContext(ctx, v)

	t.Run("read allowed", func(t *testing.T) {
		require.NoError(t, m.Authorizate(ctx, NewFeatureAuthorization(FeatureAPIKey, ActionRead)))
		require.NoError(t, m.Authorizate(ctx, NewFeatureAuthorization(FeatureAPIKey, ActionReadAll)))
		require.NoError(t, m.Authorizate(ctx, NewFeatureAuthorization(FeatureReport, ActionRead)))
		require.NoError(t, m.Authorizate(ctx, NewFeatureAuthorization(FeatureAIIntent, ActionRead)))
	})

	t.Run("write denied", func(t *testing.T) {
		for _, action := range []Action{ActionUpdate, ActionCreate, ActionDelete, ActionExport, ActionCancel} {
			err := m.Authorizate(ctx, NewFeatureAuthorization(FeatureAPIKey, action))
			require.Error(t, err, "action %d must be denied", action)
			assert.Contains(t, err.Error(), "Feature Access Deny")
		}
	})
}

func TestRoleScopesMapping(t *testing.T) {
	scopes, err := RoleToScopes(RoleSystemAdmin)
	require.NoError(t, err)
	assert.Equal(t, []string{ScopeSystem}, scopes)

	scopes, err = RoleToScopes(RoleSystemAdminReadonly)
	require.NoError(t, err)
	assert.Equal(t, []string{ScopeSystemReadonly}, scopes)

	_, err = RoleToScopes("other")
	require.Error(t, err)

	assert.Equal(t, RoleSystemAdmin, ScopesToRole([]string{ScopeSystem}, false))
	assert.Equal(t, RoleSystemAdminReadonly, ScopesToRole([]string{ScopeSystemReadonly}, false))
	// Fallback on the Admin flag for legacy users whose Scopes are empty.
	assert.Equal(t, RoleSystemAdmin, ScopesToRole(nil, true))
	assert.Equal(t, RoleSystemAdminReadonly, ScopesToRole(nil, false))
}

func TestUserGetScopesAndReadonly(t *testing.T) {
	// Populated scopes win.
	u := &User{Name: "viewer", Admin: false, Scopes: []string{ScopeSystemReadonly}}
	assert.Equal(t, []string{ScopeSystemReadonly}, u.GetScopes())
	assert.False(t, u.IsAdmin())

	v := &Visitor{User: u}
	assert.True(t, v.HasScope(ScopeSystemReadonly))
	assert.False(t, v.HasScope(ScopeSystem))
	assert.True(t, v.IsSystemReadonly())

	// Fallback derivation for legacy users without Scopes.
	legacy := &User{Name: "legacy", Admin: true}
	assert.Equal(t, []string{ScopeSystem}, legacy.GetScopes())
	assert.False(t, (&Visitor{User: legacy}).IsSystemReadonly())

	// A full admin holding extra scopes is never "read-only".
	admin := &User{Name: "root", Admin: true, Scopes: []string{ScopeSystem, ScopeSystemReadonly}}
	assert.False(t, (&Visitor{User: admin}).IsSystemReadonly())

	// Token-based visitors.
	tv := &Visitor{Token: &Token{Name: "t", Scope: ScopeSystemReadonly}}
	assert.True(t, tv.IsSystemReadonly())
	sv := &Visitor{Token: &Token{Name: "t", Scope: ScopeSupport}}
	assert.False(t, sv.IsSystemReadonly())
}
