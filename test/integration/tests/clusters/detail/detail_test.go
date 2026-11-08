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

package clusters_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"
	"github.com/stretchr/testify/assert"
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

func TestClusters_Detail(t *testing.T) {
	clusterName := testutil.UniqueClusterName()
	if _, err := testutil.CreateCluster(clusterName); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "name", clusterName)

	var data map[string]interface{}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("unmarshal data failed: %v", err)
	}
	assert.NotContains(t, data, "ready")
	assert.NotContains(t, data, "sub_clusters")
	assert.NotContains(t, data, "scheduler")
	assert.NotContains(t, data, "instance_pool")
	assert.Contains(t, data, "llm_config")

	sticky, ok := data["sticky_sessions"].(map[string]interface{})
	if assert.True(t, ok, "sticky_sessions should be an object") {
		assert.Equal(t, false, sticky["enabled"])
		assert.Equal(t, "CLIENT_IP_ONLY", sticky["hash_strategy"])
		assert.Equal(t, "", sticky["hash_header"])
	}

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
	})
}

// TestClusters_Detail_SingleCharName 验证单字符名称的集群可以正常查询。
// 回归测试：单查 URI 参数曾要求名称至少 2 个字符，导致创建成功的单字符集群无法被查询。
func TestClusters_Detail_SingleCharName(t *testing.T) {
	const clusterName = "c"
	if _, err := testutil.CreateCluster(clusterName); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + clusterName)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	testutil.AssertSuccess(t, resp)
	testutil.AssertDataFieldEquals(t, resp, "name", clusterName)

	t.Cleanup(func() {
		testutil.DeleteCluster(clusterName)
	})
}

// TestClusters_Detail_InvalidName 验证非法 cluster_name 返回 422 而非 404
//（issue #311：OneParam 此前仅 required,min=1，非法名落到记录查询返回 404，
// 与 PATCH/POST 的 ClusterName 422 语义不一致）。
func TestClusters_Detail_InvalidName(t *testing.T) {
	t.Run("257 字符名称必须 422", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + strings.Repeat("a", 257))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("65 字符超出 1-64 上限必须 422", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/" + strings.Repeat("a", 65))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("字符集外字符必须 422", func(t *testing.T) {
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/cluster+1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 422)
	})

	t.Run("合法名称但不存在的集群仍返回 404", func(t *testing.T) {
		// 对照臂：422 与 404 的区分是本次修复的核心语义。
		resp, err := testutil.GetClient().Get("/open-api/v1/clusters/no-such-cluster-xyz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		testutil.AssertErrCode(t, resp, 404)
	})
}
