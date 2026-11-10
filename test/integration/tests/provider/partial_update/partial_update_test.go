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

package partial_update_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func fetchProviderData(t *testing.T, providerName string) map[string]interface{} {
	t.Helper()
	resp, err := testutil.GetClient().Get("/open-api/v1/providers/" + providerName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	return data
}

func TestProvider_PartialUpdate(t *testing.T) {
	providerName := testutil.UniqueProviderName()
	// Create with customized values for every field that PATCH must preserve
	// when omitted: time_zone / tiers / models / keys / model_endpoint.
	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"description": "partial-update baseline",
		"model_endpoint": map[string]interface{}{
			"schema": "http",
			"uri":    "/custom/models",
		},
		"models":   []string{"deepseek-chat", "deepseek-coder"},
		"time_zone": "UTC",
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	t.Cleanup(func() {
		testutil.DeleteProvider(providerName)
	})

	// Set peak pricing tiers via the dedicated endpoint (issue #147 repro path).
	resp, err := testutil.UpdatePricingTiers(providerName, map[string]interface{}{
		"time_zone": "UTC",
		"tiers": []interface{}{
			map[string]interface{}{
				"name": "peak",
				"time_ranges": []interface{}{
					map[string]interface{}{
						"weekdays": []int{1, 2, 3, 4, 5},
						"start":    "09:00",
						"end":      "18:00",
					},
				},
			},
		},
	})
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	t.Run("PV-4-007 省略字段保留原值", func(t *testing.T) {
		// issue #322: PATCH is a true partial update — the body carries only
		// the field under test; instance_pool / model_protocols must NOT be
		// required (providers.md §2.4).
		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"description": "只改 description",
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "description", "只改 description")

		// The fields omitted from the PATCH body must keep their values.
		data := fetchProviderData(t, providerName)
		assert.Equal(t, "UTC", data["time_zone"])
		tiers, _ := data["tiers"].([]interface{})
		require.Len(t, tiers, 1)
		tier, _ := tiers[0].(map[string]interface{})
		assert.Equal(t, "peak", tier["name"])
		models, _ := data["models"].([]interface{})
		assert.Equal(t, []interface{}{"deepseek-chat", "deepseek-coder"}, models)
		keys, _ := data["keys"].([]interface{})
		require.Len(t, keys, 2)
		endpoint, _ := data["model_endpoint"].(map[string]interface{})
		assert.Equal(t, "http", endpoint["schema"])
		assert.Equal(t, "/custom/models", endpoint["uri"])
		// Required-at-create fields omitted from the PATCH keep their values.
		protocols, _ := data["model_protocols"].([]interface{})
		assert.Equal(t, []interface{}{"openai"}, protocols)
		insts, _ := data["instance_pool"].([]interface{})
		require.Len(t, insts, 1)
		inst, _ := insts[0].(map[string]interface{})
		assert.Equal(t, "10.0.0.1", inst["addr"])
	})

	t.Run("PV-4-008 只更新 time_zone 其余字段保留", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"time_zone": "Europe/London",
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)
		testutil.AssertDataFieldEquals(t, resp, "time_zone", "Europe/London")

		data := fetchProviderData(t, providerName)
		assert.Equal(t, "只改 description", data["description"])
		tiers, _ := data["tiers"].([]interface{})
		require.Len(t, tiers, 1)
		keys, _ := data["keys"].([]interface{})
		assert.Len(t, keys, 2)
		endpoint, _ := data["model_endpoint"].(map[string]interface{})
		assert.Equal(t, "/custom/models", endpoint["uri"])
	})

	t.Run("PV-4-009 显式空数组清空 keys/tiers", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"keys":  []interface{}{},
			"tiers": []interface{}{},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)

		data := fetchProviderData(t, providerName)
		// models is required (issue #115): an explicit empty models array is
		// rejected (see PV-4-016), so the stored list is preserved here.
		models, _ := data["models"].([]interface{})
		assert.Equal(t, []interface{}{"deepseek-chat", "deepseek-coder"}, models)
		keys, _ := data["keys"].([]interface{})
		assert.Empty(t, keys)
		tiers, _ := data["tiers"].([]interface{})
		assert.Empty(t, tiers)
		// Fields omitted from this PATCH keep their previous values.
		assert.Equal(t, "Europe/London", data["time_zone"])
		assert.Equal(t, "只改 description", data["description"])
		endpoint, _ := data["model_endpoint"].(map[string]interface{})
		assert.Equal(t, "/custom/models", endpoint["uri"])
	})

	t.Run("PV-4-010 非法 time_zone", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"time_zone": "Not/AZone",
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)

		// The rejected update must not have changed the stored value.
		data := fetchProviderData(t, providerName)
		assert.Equal(t, "Europe/London", data["time_zone"])
	})

	t.Run("PV-4-016 显式空 models 数组拒绝", func(t *testing.T) {
		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"models": []string{},
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)

		// The rejected update must not have changed the stored value.
		data := fetchProviderData(t, providerName)
		models, _ := data["models"].([]interface{})
		assert.Equal(t, []interface{}{"deepseek-chat", "deepseek-coder"}, models)
	})

	t.Run("PV-4-017 K8s Provider 部分更新省略 model_protocols", func(t *testing.T) {
		// issue #322 K8s variant (SC1601-TC003 A-k8s-provider-crud): PATCH
		// carries only instance_source / k8s_pool_name plus the removed
		// k8s_instance_pool field (must be silently ignored); model_protocols
		// is omitted and must keep its stored value.
		poolName := "pool-" + testutil.RandomString(8)
		resp, err := testutil.GetClient().Put("/inner-api/v1/k8s_pools/"+poolName+"/instances", []interface{}{
			map[string]interface{}{"addr": "10.0.0.1", "port": 8080, "weight": 100},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)

		k8sName := testutil.UniqueProviderName()
		_, err = testutil.CreateProvider(k8sName, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
		})
		require.NoError(t, err, "setup failed")
		t.Cleanup(func() {
			testutil.DeleteProvider(k8sName)
			testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
		})

		resp, err = testutil.GetClient().Patch("/open-api/v1/providers/"+k8sName, map[string]interface{}{
			"instance_source":    "k8s_pool",
			"k8s_pool_name":      poolName,
			"k8s_instance_pool":  []interface{}{map[string]interface{}{"addr": "10.0.0.9", "port": 8080}},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)

		data := fetchProviderData(t, k8sName)
		protocols, _ := data["model_protocols"].([]interface{})
		assert.Equal(t, []interface{}{"openai"}, protocols, "omitted model_protocols must keep its stored value")
	})
}
