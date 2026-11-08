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

package auth_test

import (
	"os"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
)

var sm *testutil.ServerManager

func TestMain(m *testing.M) {
	var err error
	sm, err = testutil.StartServer()
	if err != nil {
		panic("failed to start server: " + err.Error())
	}
	code := m.Run()
	sm.Shutdown()
	os.Exit(code)
}

// TestAuth_InvalidNamePaths guards issue #308: by-name endpoints must reject
// out-of-charset names with 422 instead of letting the PAD SPACE storage
// comparison resolve them to a real user/token.
func TestAuth_InvalidNamePaths(t *testing.T) {
	if err := testutil.CreateUser("u", "password@123"); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	if _, err := testutil.CreateToken("t", "System"); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() {
		testutil.DeleteUser("u")
		testutil.DeleteToken("t")
	})

	t.Run("AUTH-INV-001 查询用户-尾随空格必须422", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/auth/users/u%20")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("AUTH-INV-002 查询用户-其他非法名必须422", func(t *testing.T) {
		for _, p := range []string{"%20u", "u%2B", "u.", "-u"} {
			resp, err := testutil.GetClient().Get("/open-api/v1/auth/users/" + p)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertErrCode(t, resp, 422)
		}
	})

	t.Run("AUTH-INV-003 删除用户-尾随空格必须422且不误删", func(t *testing.T) {
		resp, err := testutil.GetClient().Delete("/open-api/v1/auth/users/u%20")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		after, err := testutil.GetClient().Get("/open-api/v1/auth/users/u")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, after)
	})

	// 注：/auth/users/{user_name}/products/{product_name} 绑定/解绑端点
	// （ProductUserBindEndpoint / ProductUserUnbindEndpoint）仅定义、未注册进
	// auth.Endpoints（endpoints/openapi_v1/auth/endpoints.go），请求一律 404；
	// 它们的参数结构与 UserNameParam 相同，修复随 UserNameParam.Validate 一并
	// 覆盖。故本场景不对其发请求。

	t.Run("AUTH-INV-004 查询Token-非法名必须422", func(t *testing.T) {
		for _, p := range []string{"t%20", "%20t", "t%2B"} {
			resp, err := testutil.GetClient().Get("/open-api/v1/auth/tokens/" + p)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			testutil.AssertErrCode(t, resp, 422)
		}
	})

	t.Run("AUTH-INV-005 删除Token-尾随空格必须422且不误删", func(t *testing.T) {
		resp, err := testutil.GetClient().Delete("/open-api/v1/auth/tokens/t%20")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)

		after, err := testutil.GetClient().Get("/open-api/v1/auth/tokens/t")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, after)
	})

	t.Run("AUTH-INV-006 对照臂-合法名与保留名不受影响", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/auth/users/u")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "user_name", "u")

		resp, err = testutil.GetClient().Get("/open-api/v1/auth/tokens/t")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "name", "t")

		// 保留名引用：UserNameRef/TokenNameRef 刻意不含保留名检查，
		// 内置 admin 必须可按名查询。
		resp, err = testutil.GetClient().Get("/open-api/v1/auth/users/admin")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
	})

	t.Run("AUTH-INV-007 边界臂-64字符合法名全链路", func(t *testing.T) {
		name := strings.Repeat("a", 64)
		if err := testutil.CreateUser(name, "password@123"); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		t.Cleanup(func() {
			testutil.DeleteUser(name)
		})

		resp, err := testutil.GetClient().Get("/open-api/v1/auth/users/" + name)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "user_name", name)

		resp, err = testutil.GetClient().Delete("/open-api/v1/auth/users/" + name)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertSuccess(t, resp)
	})
}
