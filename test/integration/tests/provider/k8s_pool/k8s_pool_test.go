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

package k8s_pool_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

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

func uniquePoolName() string {
	return "pool-" + testutil.RandomString(8)
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

func fetchProviderData(t *testing.T, providerName string) map[string]interface{} {
	t.Helper()
	resp, err := testutil.GetClient().Get("/open-api/v1/providers/" + providerName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	return data
}

// assertProviderHasNoMirror locks the contract: the k8s_instance_pool mirror
// field was removed from the provider response.
func assertProviderHasNoMirror(t *testing.T, providerName string) map[string]interface{} {
	t.Helper()
	data := fetchProviderData(t, providerName)
	_, ok := data["k8s_instance_pool"]
	assert.False(t, ok, "provider response must not contain the removed k8s_instance_pool field")
	return data
}

func clusterTableConfig(t *testing.T, resp *testutil.APIResponse) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	config, ok := data["Config"].(map[string]interface{})
	require.True(t, ok, "cluster_table Config should be an object")
	return config
}

func subClusterBackends(t *testing.T, config map[string]interface{}, clusterName string) []map[string]interface{} {
	t.Helper()
	cluster, ok := config[clusterName].(map[string]interface{})
	require.True(t, ok, "cluster %s should exist in cluster_table", clusterName)

	var backends []map[string]interface{}
	for _, v := range cluster {
		subClusterBackends, ok := v.([]interface{})
		if !ok {
			continue
		}
		for _, b := range subClusterBackends {
			backend, ok := b.(map[string]interface{})
			require.True(t, ok, "backend should be an object")
			backends = append(backends, backend)
		}
	}
	return backends
}

func assertBackendExists(t *testing.T, resp *testutil.APIResponse, clusterName, name string) {
	t.Helper()
	backends := subClusterBackends(t, clusterTableConfig(t, resp), clusterName)
	for _, b := range backends {
		if b["Name"] == name {
			return
		}
	}
	t.Fatalf("backend %s not found in cluster %s", name, clusterName)
}

func assertBackendNotExists(t *testing.T, resp *testutil.APIResponse, clusterName, name string) {
	t.Helper()
	backends := subClusterBackends(t, clusterTableConfig(t, resp), clusterName)
	for _, b := range backends {
		if b["Name"] == name {
			t.Fatalf("unexpected backend %s found in cluster %s", name, clusterName)
		}
	}
}

func exportClusterTable(t *testing.T) *testutil.APIResponse {
	t.Helper()
	resp, err := testutil.GetClient().Get("/inner-api/v1/configs/gslb_data/cluster_table")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	return resp
}

func createK8sCluster(t *testing.T, clusterName, providerName string) {
	t.Helper()
	_, err := testutil.GetClient().Post("/open-api/v1/clusters", map[string]interface{}{
		"name": clusterName,
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": providerName,
		},
	})
	require.NoError(t, err)
}

// TestK8sPool_EndToEnd exercises the full contract (k8s-pools.md) under
// realtime resolution: PUT pool -> create k8s_pool-mode provider -> create
// cluster WITHOUT any second pool PUT -> the export already carries the
// instances. This is the regression for the "mirror hole" incident: the
// provider created after the pool PUT used to see an empty mirror forever.
func TestK8sPool_EndToEnd(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	// 1. PUT pool with two instances (no provider references it yet).
	resp := putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
		instanceBody("10.0.0.2", 8000, nil), // weight omitted -> default 100
	})
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "name", poolName)
	testutil.AssertDataFieldEquals(t, resp, "instance_count", float64(2))
	testutil.AssertDataFieldNotEmpty(t, resp, "last_sync_time")

	// 2. GET the pool entry.
	resp, err := testutil.GetClient().Get("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "instance_count", float64(2))
	testutil.AssertListFieldLen(t, resp, "instances", 2)

	// 3. Create a k8s_pool-mode provider referencing the pool. No mirror
	// exists anymore; the response carries only the reference.
	_, err = testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)
	data := assertProviderHasNoMirror(t, providerName)
	assert.Equal(t, "k8s_pool", data["instance_source"])
	assert.Equal(t, poolName, data["k8s_pool_name"])

	// 4. Create a cluster referencing the provider. WITHOUT any second pool
	// PUT, the export must already carry the pool instances (the incident
	// regression: previously this required a re-PUT to fill the mirror).
	createK8sCluster(t, clusterName, providerName)
	resp = exportClusterTable(t)
	assertBackendExists(t, resp, clusterName, "10.0.0.1_8000")
	assertBackendExists(t, resp, clusterName, "10.0.0.2_8000")

	// 5. PUT again with one instance removed and one added: the export must
	// follow at the next read.
	resp = putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.2", 8000, 100),
		instanceBody("10.0.0.3", 9000, 100),
	})
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "instance_count", float64(2))

	resp = exportClusterTable(t)
	assertBackendNotExists(t, resp, clusterName, "10.0.0.1_8000")
	assertBackendExists(t, resp, clusterName, "10.0.0.2_8000")
	assertBackendExists(t, resp, clusterName, "10.0.0.3_9000")

	// 6. DELETE the referenced pool: the row is kept with zero instances, so
	// GET returns 200 + instance_count=0 and the export becomes an empty
	// entry (cluster key present, no backends).
	resp, err = testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataNull(t, resp)

	resp, err = testutil.GetClient().Get("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "instance_count", float64(0))

	resp = exportClusterTable(t)
	config := clusterTableConfig(t, resp)
	_, ok := config[clusterName].(map[string]interface{})
	require.True(t, ok, "cluster %s should still be exported as an empty entry", clusterName)
	assert.Empty(t, subClusterBackends(t, config, clusterName))
}

// TestK8sPool_ReverseOrder covers the opposite lifecycle: provider and
// cluster exist before any pool content; the first pool PUT makes the export
// light up without any fan-out.
func TestK8sPool_ReverseOrder(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)

	// Cluster on a not-yet-existing pool: created successfully, exported as
	// an empty entry.
	createK8sCluster(t, clusterName, providerName)
	resp := exportClusterTable(t)
	config := clusterTableConfig(t, resp)
	_, ok := config[clusterName].(map[string]interface{})
	require.True(t, ok, "cluster must be exported as an empty entry before the pool exists")
	assert.Empty(t, subClusterBackends(t, config, clusterName))

	// First pool PUT: the next export carries the instances (no mirror
	// backfill round-trip required).
	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
	}))
	resp = exportClusterTable(t)
	assertBackendExists(t, resp, clusterName, "10.0.0.1_8000")
}

// TestK8sPool_ClusterPatchKeepsExport pins the update path: patching a
// cluster that references a k8s provider must not clear the exported k8s
// instances (the derived pool is a placeholder and is never rewritten with
// instance data).
func TestK8sPool_ClusterPatchKeepsExport(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
	}))
	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)
	createK8sCluster(t, clusterName, providerName)

	resp := exportClusterTable(t)
	assertBackendExists(t, resp, clusterName, "10.0.0.1_8000")

	// PATCH the cluster's llm_config with the same provider: a no-op update
	// that used to rewrite the derived pool from the dormant instance_pool.
	resp, err = testutil.GetClient().Patch("/open-api/v1/clusters/"+clusterName, map[string]interface{}{
		"llm_config": map[string]interface{}{
			"models":   []string{"deepseek-chat"},
			"provider": providerName,
		},
	})
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	resp = exportClusterTable(t)
	backends := subClusterBackends(t, clusterTableConfig(t, resp), clusterName)
	require.NotEmpty(t, backends, "cluster PATCH must not clear the k8s export")
	assert.Equal(t, "10.0.0.1_8000", backends[0]["Name"])
}

// TestK8sPool_ProviderModeSwitch covers instance_pool <-> k8s_pool switches:
// the export must follow the effective source in both directions.
func TestK8sPool_ProviderModeSwitch(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	// Start as a manual provider with a manual instance.
	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_pool": []interface{}{instanceBody("1.2.3.4", 443, 100)},
	})
	require.NoError(t, err)
	createK8sCluster(t, clusterName, providerName)

	resp := exportClusterTable(t)
	assertBackendExists(t, resp, clusterName, "1.2.3.4_443")

	// Switch to k8s_pool mode: the manual backend disappears (derived pool
	// cleared to the empty placeholder) and the export follows the pool.
	resp, err = testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"model_protocols": []string{"openai"},
	})
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	resp = exportClusterTable(t)
	config := clusterTableConfig(t, resp)
	assert.Empty(t, subClusterBackends(t, config, clusterName), "manual backend must be cleared on manual -> k8s switch")

	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
	}))
	resp = exportClusterTable(t)
	assertBackendExists(t, resp, clusterName, "10.0.0.1_8000")

	// Switch back to instance_pool mode: the dormant manual pool takes over.
	resp, err = testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
		"instance_source": "instance_pool",
		"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
		"model_protocols": []string{"openai"},
	})
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	resp = exportClusterTable(t)
	assertBackendExists(t, resp, clusterName, "1.2.3.4_443")
	assertBackendNotExists(t, resp, clusterName, "10.0.0.1_8000")
}

func TestK8sPool_List(t *testing.T) {
	poolName := uniquePoolName()
	t.Cleanup(func() {
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	resp := putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
		instanceBody("10.0.0.2", 8000, 100),
		instanceBody("10.0.0.3", 8000, 100),
	})
	testutil.AssertSuccess(t, resp)

	resp, err := testutil.GetClient().Get("/inner-api/v1/k8s_pools")
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	list, err := testutil.GetDataListField(resp, "list")
	require.NoError(t, err)
	found := false
	for _, item := range list {
		entry, ok := item.(map[string]interface{})
		require.True(t, ok)
		if entry["name"] != poolName {
			continue
		}
		found = true
		assert.Equal(t, float64(3), entry["instance_count"])
		assert.NotZero(t, entry["last_sync_time"])
	}
	require.True(t, found, "pool %s should appear in the list", poolName)
}

func TestK8sPool_ValidationErrors(t *testing.T) {
	t.Run("PUT with invalid pool name", func(t *testing.T) {
		resp := putInstances(t, "-bad-", []interface{}{
			instanceBody("10.0.0.1", 8000, 100),
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("PUT with duplicate addr/port", func(t *testing.T) {
		resp := putInstances(t, uniquePoolName(), []interface{}{
			instanceBody("10.0.0.1", 8000, 100),
			instanceBody("10.0.0.1", 8000, 100),
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("PUT with out-of-range weight", func(t *testing.T) {
		resp := putInstances(t, uniquePoolName(), []interface{}{
			instanceBody("10.0.0.1", 8000, 101),
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("PUT with out-of-range port", func(t *testing.T) {
		resp := putInstances(t, uniquePoolName(), []interface{}{
			instanceBody("10.0.0.1", 0, 100),
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("PUT with missing addr", func(t *testing.T) {
		resp := putInstances(t, uniquePoolName(), []interface{}{
			map[string]interface{}{"port": 8000, "weight": 100},
		})
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("GET missing pool returns 404", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/inner-api/v1/k8s_pools/" + uniquePoolName())
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 404)
	})

	t.Run("DELETE missing pool returns 404", func(t *testing.T) {
		resp, err := testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + uniquePoolName())
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 404)
	})
}

// TestProvider_K8sPoolModeUpdate covers the OpenAPI side of the contract:
// creating and PATCHing providers in k8s_pool mode, the removed
// k8s_instance_pool field silent-ignore, and the legacy default-mode behavior.
func TestProvider_K8sPoolModeUpdate(t *testing.T) {
	t.Run("create silently ignores the removed k8s_instance_pool field", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", map[string]interface{}{
			"name":              providerName,
			"models":            []string{"deepseek-chat"},
			"model_protocols":   []string{"openai"},
			"instance_source":   "k8s_pool",
			"k8s_pool_name":     uniquePoolName(),
			"k8s_instance_pool": []interface{}{instanceBody("10.0.0.1", 8000, 100)},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)
		assertProviderHasNoMirror(t, providerName)
	})

	t.Run("create rejects invalid instance_source", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", map[string]interface{}{
			"name":            providerName,
			"models":          []string{"deepseek-chat"},
			"model_protocols": []string{"openai"},
			"instance_source": "unknown",
			"instance_pool":   []interface{}{instanceBody("10.0.0.1", 8000, 100)},
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("create k8s_pool mode without k8s_pool_name rejected", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", map[string]interface{}{
			"name":            providerName,
			"models":          []string{"deepseek-chat"},
			"model_protocols": []string{"openai"},
			"instance_source": "k8s_pool",
			"instance_pool":   []interface{}{instanceBody("10.0.0.1", 8000, 100)},
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("mode switch back to instance_pool recovers the dormant pool", func(t *testing.T) {
		poolName := uniquePoolName()
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })

		_, err := testutil.CreateProvider(providerName, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
		})
		require.NoError(t, err)

		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"instance_source": "instance_pool",
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
			"model_protocols": []string{"openai"},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)
		data := assertProviderHasNoMirror(t, providerName)
		assert.Equal(t, "instance_pool", data["instance_source"])
		// The k8s pool name is kept dormant for a later switch back.
		assert.Equal(t, poolName, data["k8s_pool_name"])
	})

	t.Run("k8s_pool_name in default mode is dormant but format-checked", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })

		_, err := testutil.CreateProvider(providerName, map[string]interface{}{
			"k8s_pool_name": "svc-dormant",
		})
		require.NoError(t, err)
		data := fetchProviderData(t, providerName)
		assert.Equal(t, "instance_pool", data["instance_source"])
		assert.Equal(t, "svc-dormant", data["k8s_pool_name"])

		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
			"model_protocols": []string{"openai"},
			"k8s_pool_name":   "bad name",
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)
	})
}

// TestK8sPool_DeleteUnreferenced ensures an unreferenced pool is fully
// removed (404 afterwards).
func TestK8sPool_DeleteUnreferenced(t *testing.T) {
	poolName := uniquePoolName()
	t.Cleanup(func() {
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
	}))

	resp, err := testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	resp, err = testutil.GetClient().Get("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertErrCode(t, resp, 404)
}

// TestK8sPool_DeleteReferenced ensures the reference-aware contract: a
// referenced pool is kept with zero instances (GET 200 + instance_count=0)
// and the export drains to an empty entry.
func TestK8sPool_DeleteReferenced(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
	}))
	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)
	createK8sCluster(t, clusterName, providerName)

	resp, err := testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	// The row is kept: 200 + zero instances, last_sync_time preserved.
	resp, err = testutil.GetClient().Get("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "instance_count", float64(0))
	testutil.AssertDataFieldNotEmpty(t, resp, "last_sync_time")

	resp = exportClusterTable(t)
	config := clusterTableConfig(t, resp)
	_, ok := config[clusterName].(map[string]interface{})
	require.True(t, ok, "cluster must be exported as an empty entry after referenced pool delete")
	assert.Empty(t, subClusterBackends(t, config, clusterName))
}

// TestK8sPool_ProviderReferencesMissingPool pins the zero-instance contract:
// a provider referencing a pool that does not exist exports an empty entry
// without any error.
func TestK8sPool_ProviderReferencesMissingPool(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
	})

	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)
	createK8sCluster(t, clusterName, providerName)

	resp := exportClusterTable(t)
	config := clusterTableConfig(t, resp)
	_, ok := config[clusterName].(map[string]interface{})
	require.True(t, ok, "cluster must be exported as an empty entry when the pool is missing")
	assert.Empty(t, subClusterBackends(t, config, clusterName))
}

// TestK8sPool_VersionBump pins the export freshness contract: changing the
// pool content must bump the exported cluster_table version.
func TestK8sPool_VersionBump(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	clusterName := testutil.UniqueClusterName()
	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	_, err := testutil.CreateProvider(providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{},
	})
	require.NoError(t, err)
	createK8sCluster(t, clusterName, providerName)

	exportVersion := func(t *testing.T) string {
		t.Helper()
		resp, err := testutil.GetClient().Get("/inner-api/v1/configs/gslb_data/cluster_table?version=")
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.Data, &data))
		version, ok := data["Version"].(string)
		require.True(t, ok, "cluster_table response should carry a version")
		return version
	}

	before := exportVersion(t)
	testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
		instanceBody("10.0.0.1", 8000, 100),
	}))
	after := exportVersion(t)
	assert.NotEqual(t, before, after, "pool PUT must bump the cluster_table version via the export signature")
}

// TestK8sPool_RejectedWritesLeaveStateUnchanged pins the read-back-zero-change
// contract: every rejected write here must leave the provider, the pool, and
// the exported cluster_table bit-identical.
func TestK8sPool_RejectedWritesLeaveStateUnchanged(t *testing.T) {
	t.Run("PATCH invalid k8s_pool_name → 422 → provider unchanged", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })

		_, err := testutil.CreateProvider(providerName, map[string]interface{}{
			"k8s_pool_name": "svc-dormant-x",
		})
		require.NoError(t, err)

		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"instance_pool":   []interface{}{instanceBody("10.0.0.2", 8081, 100)},
			"model_protocols": []string{"openai"},
			"k8s_pool_name":   "bad name",
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)

		data := fetchProviderData(t, providerName)
		assert.Equal(t, "svc-dormant-x", data["k8s_pool_name"], "rejected PATCH must not change stored k8s_pool_name")
		pool, _ := data["instance_pool"].([]interface{})
		require.Len(t, pool, 1)
		assert.Equal(t, "10.0.0.1", pool[0].(map[string]interface{})["addr"],
			"rejected PATCH must not persist the submitted instance_pool")
	})

	t.Run("create with removed k8s_instance_pool → silently ignored → provider created without mirror", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })
		resp, err := testutil.GetClient().Post("/open-api/v1/providers", map[string]interface{}{
			"name":              providerName,
			"models":            []string{"deepseek-chat"},
			"model_protocols":   []string{"openai"},
			"instance_source":   "k8s_pool",
			"k8s_pool_name":     uniquePoolName(),
			"instance_pool":     []interface{}{},
			"k8s_instance_pool": []interface{}{instanceBody("10.0.0.1", 8000, 100)},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)
		assertProviderHasNoMirror(t, providerName)
	})

	t.Run("PATCH switch to k8s_pool without name → 422 → provider unchanged", func(t *testing.T) {
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(providerName) })

		_, err := testutil.CreateProvider(providerName, nil)
		require.NoError(t, err)

		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"instance_source": "k8s_pool",
			"instance_pool":   []interface{}{instanceBody("10.0.0.1", 8080, 100)},
			"model_protocols": []string{"openai"},
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)

		data := assertProviderHasNoMirror(t, providerName)
		assert.Equal(t, "instance_pool", data["instance_source"], "rejected mode switch must not change stored source")
	})

	t.Run("failed pool PUT leaves pool and export unchanged", func(t *testing.T) {
		poolName := uniquePoolName()
		providerName := testutil.UniqueProviderName()
		clusterName := testutil.UniqueClusterName()
		t.Cleanup(func() {
			testutil.DeleteCluster(clusterName)
			testutil.DeleteProvider(providerName)
			testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
		})

		testutil.AssertSuccess(t, putInstances(t, poolName, []interface{}{
			instanceBody("10.0.0.1", 8000, 100),
		}))
		_, err := testutil.CreateProvider(providerName, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
			"instance_pool":   []interface{}{},
		})
		require.NoError(t, err)
		createK8sCluster(t, clusterName, providerName)

		// Rejected PUT: duplicate addr/port inside the new list.
		resp := putInstances(t, poolName, []interface{}{
			instanceBody("10.9.9.9", 9000, 100),
			instanceBody("10.9.9.9", 9000, 100),
		})
		testutil.AssertErrCode(t, resp, 422)

		entry := decodePoolEntry(t, fetchPoolEntry(t, poolName))
		assert.Equal(t, float64(1), entry["instance_count"], "rejected PUT must not touch the pool")
		assert.Equal(t, "10.0.0.1", entry["instances"].([]interface{})[0].(map[string]interface{})["addr"])

		// 防泄漏: the rejected instances must not reach the exported config.
		resp = exportClusterTable(t)
		assertBackendExists(t, resp, clusterName, "10.0.0.1_8000")
		assertBackendNotExists(t, resp, clusterName, "10.9.9.9_9000")
	})
}

// TestProvider_K8sPoolOmissionMatrix pins the omission semantics (PATCH 省略
// 字段保留原值矩阵) for the instance-source fields.
func TestProvider_K8sPoolOmissionMatrix(t *testing.T) {
	t.Run("k8s mode PATCH keeps every unsubmitted field", func(t *testing.T) {
		poolName := uniquePoolName()
		providerName := testutil.UniqueProviderName()
		t.Cleanup(func() {
			testutil.DeleteProvider(providerName)
			testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
		})

		_, err := testutil.CreateProvider(providerName, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
			"description":     "orig-desc",
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
		})
		require.NoError(t, err)

		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
			"model_protocols": []string{"openai"},
			"description":     "new-desc",
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)

		data := assertProviderHasNoMirror(t, providerName)
		assert.Equal(t, "new-desc", data["description"])
		assert.Equal(t, []interface{}{"deepseek-chat"}, data["models"])
		assert.Len(t, data["keys"], 2)
		pool, _ := data["instance_pool"].([]interface{})
		require.Len(t, pool, 1)
		assert.Equal(t, "1.2.3.4", pool[0].(map[string]interface{})["addr"],
			"dormant instance_pool must survive a PATCH that omits it")
		assert.Equal(t, poolName, data["k8s_pool_name"])
	})

	t.Run("explicit empty pool rejected at switch, dormant pool recovers when resubmitted", func(t *testing.T) {
		poolName := uniquePoolName()
		t.Cleanup(func() {
			testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
		})

		// Switching to instance_pool with an explicit empty pool is rejected
		// at the switch point and the dormant pool stays intact.
		p1 := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(p1) })
		_, err := testutil.CreateProvider(p1, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
		})
		require.NoError(t, err)

		resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+p1, map[string]interface{}{
			"instance_source": "instance_pool",
			"instance_pool":   []interface{}{},
			"model_protocols": []string{"openai"},
		})
		require.NoError(t, err)
		testutil.AssertErrCode(t, resp, 422)

		data := assertProviderHasNoMirror(t, p1)
		assert.Equal(t, "k8s_pool", data["instance_source"], "rejected switch must keep the stored mode")
		pool, _ := data["instance_pool"].([]interface{})
		require.Len(t, pool, 1, "dormant pool must stay intact after a rejected switch")
		assert.Equal(t, "1.2.3.4", pool[0].(map[string]interface{})["addr"])

		// Resubmitting the dormant pool value at switch time makes it
		// effective again, while k8s_pool_name stays dormant.
		p2 := testutil.UniqueProviderName()
		t.Cleanup(func() { testutil.DeleteProvider(p2) })
		_, err = testutil.CreateProvider(p2, map[string]interface{}{
			"instance_source": "k8s_pool",
			"k8s_pool_name":   poolName,
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
		})
		require.NoError(t, err)

		resp, err = testutil.GetClient().Patch("/open-api/v1/providers/"+p2, map[string]interface{}{
			"instance_source": "instance_pool",
			"instance_pool":   []interface{}{instanceBody("1.2.3.4", 443, 100)},
			"model_protocols": []string{"openai"},
		})
		require.NoError(t, err)
		testutil.AssertSuccess(t, resp)

		data = assertProviderHasNoMirror(t, p2)
		assert.Equal(t, "instance_pool", data["instance_source"])
		assert.Equal(t, poolName, data["k8s_pool_name"], "pool name stays dormant for a later switch back")
		pool, _ = data["instance_pool"].([]interface{})
		require.Len(t, pool, 1)
		assert.Equal(t, "1.2.3.4", pool[0].(map[string]interface{})["addr"])
	})
}

// TestProvider_K8sPoolModeSwitchAuditLog pins the audit contract: a
// mode-switch PATCH must produce a provider update log whose diff_keys
// exactly matches the fields that actually changed.
func TestProvider_K8sPoolModeSwitchAuditLog(t *testing.T) {
	poolName := uniquePoolName()
	providerName := testutil.UniqueProviderName()
	t.Cleanup(func() {
		testutil.DeleteProvider(providerName)
		testutil.GetClient().Delete("/inner-api/v1/k8s_pools/" + poolName)
	})

	// CreateProvider seeds instance_pool [10.0.0.1:8080] and
	// model_protocols [openai]; resubmitting identical values below keeps
	// them out of the expected diff.
	_, err := testutil.CreateProvider(providerName, nil)
	require.NoError(t, err)

	resp, err := testutil.GetClient().Patch("/open-api/v1/providers/"+providerName, map[string]interface{}{
		"instance_source": "k8s_pool",
		"k8s_pool_name":   poolName,
		"instance_pool":   []interface{}{instanceBody("10.0.0.1", 8080, 100)},
		"model_protocols": []string{"openai"},
	})
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)

	// The operation-log recorder flushes asynchronously; under a full-suite
	// run the backlog stretches the visible latency well past the 10s default,
	// so allow a generous window.
	entry, err := testutil.WaitForOperationLog(map[string]string{
		"resource_type": "provider",
		"action":        "update",
		"resource_name": providerName,
		"status":        "1",
	}, 30*time.Second)
	require.NoError(t, err, "mode-switch PATCH must produce a provider update audit log")

	require.NotNil(t, entry.ChangeSummary)
	diffKeys, ok := entry.ChangeSummary["diff_keys"].([]interface{})
	require.True(t, ok, "diff_keys should be an array")
	assert.ElementsMatch(t, []interface{}{"instance_source", "k8s_pool_name"}, diffKeys)

	after, ok := entry.ChangeSummary["after"].(map[string]interface{})
	require.True(t, ok, "after should be an object")
	assert.Equal(t, "k8s_pool", after["instance_source"])
	assert.Equal(t, poolName, after["k8s_pool_name"])
}

// fetchPoolEntry GETs one pool entry for read-back assertions.
func fetchPoolEntry(t *testing.T, poolName string) *testutil.APIResponse {
	t.Helper()
	resp, err := testutil.GetClient().Get("/inner-api/v1/k8s_pools/" + poolName)
	require.NoError(t, err)
	testutil.AssertSuccess(t, resp)
	return resp
}

// decodePoolEntry unmarshals a pool entry response into a field map.
func decodePoolEntry(t *testing.T, resp *testutil.APIResponse) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Data, &data))
	return data
}
