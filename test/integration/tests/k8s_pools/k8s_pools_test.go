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

package k8s_pools_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func uniquePoolName() string {
	return testutil.UniqueName("oapi-pool")
}

func putInstances(t *testing.T, poolName string, instances []interface{}) *testutil.APIResponse {
	t.Helper()
	resp, err := testutil.GetClient().Put("/inner-api/v1/k8s_pools/"+poolName+"/instances", instances)
	require.NoError(t, err)
	return resp
}

func instanceBody(addr string, port int, weight interface{}) map[string]interface{} {
	inst := map[string]interface{}{"addr": addr, "port": port}
	if weight != nil {
		inst["weight"] = weight
	}
	return inst
}

func decodeData(t *testing.T, resp *testutil.APIResponse) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	return data
}

func findPool(t *testing.T, list []interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		require.True(t, ok)
		if entry["name"] == name {
			return entry
		}
	}
	t.Fatalf("pool %s not found in list", name)
	return nil
}

// TestOpenAPI_K8sPools_ReadOnlyLifecycle verifies the two OpenAPI read
// endpoints against the InnerAPI write path: list -> detail -> empty -> 404/422.
func TestOpenAPI_K8sPools_ReadOnlyLifecycle(t *testing.T) {
	poolName := uniquePoolName()
	t.Cleanup(func() {
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	resp := putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
		instanceBody("10.0.0.2", 8000, nil),
	})
	testutil.AssertSuccess(t, resp)

	// List: the pool appears with the correct count and sync time.
	resp, err := testutil.GetClient().Get("/open-api/v1/k8s_pools")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	listData := decodeData(t, resp)
	list, ok := listData["list"].([]interface{})
	require.True(t, ok)
	entry := findPool(t, list, poolName)
	assert.Equal(t, float64(2), entry["instance_count"])
	assert.Greater(t, entry["last_sync_time"], float64(0))

	// Detail: instances and system fields are returned.
	resp, err = testutil.GetClient().Get("/open-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	detail := decodeData(t, resp)
	assert.Equal(t, poolName, detail["name"])
	assert.Equal(t, float64(2), detail["instance_count"])
	instances, ok := detail["instances"].([]interface{})
	require.True(t, ok)
	require.Len(t, instances, 2)
	first := instances[0].(map[string]interface{})
	assert.Equal(t, "10.0.0.1", first["addr"])
	assert.Equal(t, float64(8000), first["port"])
	assert.Equal(t, float64(100), first["weight"])
	assert.Equal(t, false, first["disable"])

	// Empty pool is a valid 200 state, not 404.
	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{}))
	resp, err = testutil.GetClient().Get("/open-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	detail = decodeData(t, resp)
	assert.Equal(t, float64(0), detail["instance_count"])
	assert.Empty(t, detail["instances"].([]interface{}))

	// Missing pool: 404.
	missing := uniquePoolName()
	resp, err = testutil.GetClient().Get("/open-api/v1/k8s_pools/" + missing)
	require.NoError(t, err)
	testutil.AssertErrCode(t, resp, 404)

	// Invalid name: 422.
	resp, err = testutil.GetClient().Get("/open-api/v1/k8s_pools/-bad-")
	require.NoError(t, err)
	testutil.AssertErrCode(t, resp, 422)
}

// TestOpenAPI_K8sPools_ProviderDetailFlow verifies the intended console flow:
// a k8s_pool provider carries k8s_pool_name, and the pool detail endpoint
// returns the instances the provider detail page should display.
func TestOpenAPI_K8sPools_ProviderDetailFlow(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	t.Cleanup(func() {
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.1.1", 9000, nil),
		instanceBody("10.0.1.2", 9000, nil),
	}))

	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)

	resp, err := testutil.GetClient().Get("/open-api/v1/providers/" + providerName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	provider := decodeData(t, resp)
	assert.Equal(t, "k8s_pool", provider["instance_source"])
	assert.Equal(t, poolName, provider["k8s_pool_name"])

	resp, err = testutil.GetClient().Get("/open-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	pool := decodeData(t, resp)
	assert.Equal(t, poolName, pool["name"])
	assert.Equal(t, float64(2), pool["instance_count"])
	require.Len(t, pool["instances"].([]interface{}), 2)
}
