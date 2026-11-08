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

package certificate_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

func assertCertMeta(t *testing.T, resp *testutil.APIResponse, certName string) {
	testutil.AssertDataFieldEquals(t, resp, "cert_name", certName)
	exists, _ := testutil.FieldExists(resp, "cert_file_content")
	assert.False(t, exists, "should not return cert_file_content")
	exists, _ = testutil.FieldExists(resp, "key_file_content")
	assert.False(t, exists, "should not return key_file_content")
}

func TestCertificate_Create(t *testing.T) {
	defaultCert := testutil.UniqueCertName()
	if _, err := testutil.CreateCertificate(defaultCert, true); err != nil {
		t.Fatalf("setup default cert failed: %v", err)
	}

	cert1 := testutil.UniqueCertName()
	certDefault := testutil.UniqueCertName()
	certDot := testutil.UniqueCertName() + ".qa"
	certDescBoundary := testutil.UniqueCertName()

	t.Run("CERT-1-001 创建非默认证书", func(t *testing.T) {
		resp, err := testutil.CreateCertificate(cert1, false)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		_ = resp
		getResp, err := testutil.GetClient().Get("/open-api/v1/certificates/" + cert1)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertSuccess(t, getResp)
		assertCertMeta(t, getResp, cert1)
		testutil.AssertDataFieldEquals(t, getResp, "is_default", false)
	})

	t.Run("CERT-1-002 创建默认证书", func(t *testing.T) {
		resp, err := testutil.CreateCertificate(certDefault, true)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		_ = resp
		getResp, err := testutil.GetClient().Get("/open-api/v1/certificates/" + certDefault)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertSuccess(t, getResp)
		testutil.AssertDataFieldEquals(t, getResp, "is_default", true)
	})

	t.Run("CERT-1-003 证书与密钥不匹配", func(t *testing.T) {
		resp, err := testutil.GetClient().Post("/open-api/v1/certificates", map[string]interface{}{
			"cert_name":         testutil.UniqueCertName(),
			"description":       "不匹配证书",
			"is_default":        false,
			"cert_file_content": "-----BEGIN CERTIFICATE-----INVALID-----END CERTIFICATE-----",
			"key_file_content":  "-----BEGIN RSA PRIVATE KEY-----INVALID-----END RSA PRIVATE KEY-----",
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.ErrNum != 422 && resp.ErrNum != 500 {
			t.Errorf("expected ErrNum=422 or 500, got ErrNum=%d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
		}
	})

	// CERT-1-004 ~ CERT-1-012: cert_name 命名规范校验（Issue #133）
	createWithName := func(t *testing.T, certName string) *testutil.APIResponse {
		t.Helper()
		certPEM, keyPEM, err := testutil.GenerateTestCert(certName)
		if err != nil {
			t.Fatalf("generate test cert failed: %v", err)
		}
		resp, err := testutil.GetClient().Post("/open-api/v1/certificates", map[string]interface{}{
			"cert_name":         certName,
			"description":       "命名规范校验",
			"is_default":        false,
			"cert_file_content": certPEM,
			"key_file_content":  keyPEM,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp
	}

	invalidNameCases := []struct {
		name     string
		certName string
	}{
		{"CERT-1-004 cert_name 含 /", "demo/child"},
		{"CERT-1-005 cert_name 含 ?", "demo?x=1"},
		{"CERT-1-006 cert_name 含 #", "demo#1"},
		{"CERT-1-007 cert_name 含空格", "demo cert"},
		{"CERT-1-008 cert_name 含 %", "demo%2F"},
		{"CERT-1-009 cert_name 长度为 1", "a"},
		{"CERT-1-010 cert_name 长度为 65", strings.Repeat("a", 65)},
		{"CERT-1-011 cert_name 以 - 开头", "-demo"},
		{"CERT-1-012 cert_name 以 _ 结尾", "demo_"},
	}
	for _, tc := range invalidNameCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := createWithName(t, tc.certName)
			if resp.ErrNum != 422 {
				t.Errorf("expected ErrNum=422, got ErrNum=%d, ErrMsg=%s", resp.ErrNum, resp.ErrMsg)
			}
		})
	}

	t.Run("CERT-1-013 cert_name 含 .（合法）", func(t *testing.T) {
		resp := createWithName(t, certDot)
		testutil.AssertSuccess(t, resp)
		assertCertMeta(t, resp, certDot)
	})

	// CERT-1-014 ~ CERT-1-017: description 合法性条件
	// （certificates.md §1/§2.1：必填；长度 2-256 字节；不能包含控制字符）
	createWithDescription := func(t *testing.T, certName, description string) *testutil.APIResponse {
		t.Helper()
		certPEM, keyPEM, err := testutil.GenerateTestCert(certName)
		if err != nil {
			t.Fatalf("generate test cert failed: %v", err)
		}
		resp, err := testutil.GetClient().Post("/open-api/v1/certificates", map[string]interface{}{
			"cert_name":         certName,
			"description":       description,
			"is_default":        false,
			"cert_file_content": certPEM,
			"key_file_content":  keyPEM,
		})
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		return resp
	}

	// assertCertNotCreated 断言被拒请求没有产生任何副作用（证书未落库）。
	assertCertNotCreated := func(t *testing.T, certName string) {
		t.Helper()
		getResp, err := testutil.GetClient().Get("/open-api/v1/certificates/" + certName)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertErrCode(t, getResp, 404)
	}

	t.Run("CERT-1-014 description 恰好 256 字节（合法边界）", func(t *testing.T) {
		desc := strings.Repeat("b", 256)
		resp := createWithDescription(t, certDescBoundary, desc)
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", desc)

		getResp, err := testutil.GetClient().Get("/open-api/v1/certificates/" + certDescBoundary)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertSuccess(t, getResp)
		testutil.AssertDataFieldEquals(t, getResp, "description", desc)
	})

	t.Run("CERT-1-015 description 257 字节被拒绝且未创建", func(t *testing.T) {
		certName := testutil.UniqueCertName()
		resp := createWithDescription(t, certName, strings.Repeat("c", 257))
		testutil.AssertErrCode(t, resp, 422)
		assertCertNotCreated(t, certName)
	})

	t.Run("CERT-1-016 description 含控制字符被拒绝且未创建", func(t *testing.T) {
		certName := testutil.UniqueCertName()
		resp := createWithDescription(t, certName, "abc\x01def")
		testutil.AssertErrCode(t, resp, 422)
		assertCertNotCreated(t, certName)
	})

	t.Run("CERT-1-017 description 长度为 1 字节被拒绝", func(t *testing.T) {
		certName := testutil.UniqueCertName()
		resp := createWithDescription(t, certName, "x")
		testutil.AssertErrCode(t, resp, 422)
		assertCertNotCreated(t, certName)
	})

	t.Run("CERT-1-018 description 单个多字节字符合法（3 字节 ≥ 2 字节下限）", func(t *testing.T) {
		// issue #310：certificates.md 的下限按 UTF-8 字节计，单个汉字
		// （1 字符 / 3 字节）合法；修复前被 `min=2` tag 按字符误拒。
		certName := testutil.UniqueCertName()
		defer testutil.DeleteCertificate(certName)
		resp := createWithDescription(t, certName, "汉")
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", "汉")

		getResp, err := testutil.GetClient().Get("/open-api/v1/certificates/" + certName)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}
		testutil.AssertSuccess(t, getResp)
		testutil.AssertDataFieldEquals(t, getResp, "description", "汉")
	})

	t.Cleanup(func() {
		testutil.DeleteCertificate(cert1)
		testutil.DeleteCertificate(certDefault)
		testutil.DeleteCertificate(defaultCert)
		testutil.DeleteCertificate(certDot)
		testutil.DeleteCertificate(certDescBoundary)
	})
}
