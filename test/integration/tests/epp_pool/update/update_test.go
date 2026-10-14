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

package epp_pool_test

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

func patchEppPool(t *testing.T, body map[string]interface{}) *testutil.APIResponse {
	resp, err := testutil.GetClient().Patch("/open-api/v1/epp-pool", body)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func TestEppPool_Update(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]interface{}
		wantCode int
		check    func(t *testing.T, resp *testutil.APIResponse)
	}{
		{
			name: "EP-2-001 全量替换两组（单实例组）",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						},
					},
					map[string]interface{}{
						"name": "g2",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-c", "host": "10.0.0.3", "port": 9002},
						},
					},
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				groups := data["groups"].([]interface{})
				assert.Len(t, groups, 2)
			},
		},
		{
			name: "EP-2-002 全量替换语义（第二次 PATCH 覆盖第一次）",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g-only",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-x", "host": "10.0.1.1", "port": 9002},
						},
					},
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				getResp, err := testutil.GetClient().Get("/open-api/v1/epp-pool")
				if err != nil {
					t.Fatalf("get failed: %v", err)
				}
				testutil.AssertSuccess(t, getResp)
				assert.Equal(t, string(resp.Data), string(getResp.Data))
				var data map[string]interface{}
				json.Unmarshal(getResp.Data, &data)
				groups := data["groups"].([]interface{})
				require.Len(t, groups, 1)
				assert.Equal(t, "g-only", groups[0].(map[string]interface{})["name"])
			},
		},
		{
			name:     "EP-2-003 groups 为空数组",
			body:     map[string]interface{}{"groups": []interface{}{}},
			wantCode: 422,
		},
		{
			name:     "EP-2-004 缺少 groups",
			body:     map[string]interface{}{},
			wantCode: 422,
		},
		{
			name: "EP-2-005 组名重复",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						},
					},
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-b", "host": "10.0.0.2", "port": 9002},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-006 空组（instances 为空）",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name":      "g1",
						"instances": []interface{}{},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-007 实例 id 池内重复（跨组）",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						},
					},
					map[string]interface{}{
						"name": "g2",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.2", "port": 9002},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-008 (host, port) 池内重复",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						},
					},
					map[string]interface{}{
						"name": "g2",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-b", "host": "10.0.0.1", "port": 9002},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-009 非法 port（0）",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 0},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-010 非法 port（65536）",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 65536},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-011 非法 host",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "-bad-host", "port": 9002},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-012 实例 id 为空",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "", "host": "10.0.0.1", "port": 9002},
						},
					},
				},
			},
			wantCode: 422,
		},
		{
			name: "EP-2-013 单实例组校验通过",
			body: map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "g1",
						"instances": []interface{}{
							map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						},
					},
				},
			},
			wantCode: 200,
			check: func(t *testing.T, resp *testutil.APIResponse) {
				var data map[string]interface{}
				json.Unmarshal(resp.Data, &data)
				groups := data["groups"].([]interface{})
				require.Len(t, groups, 1)
				insts := groups[0].(map[string]interface{})["instances"].([]interface{})
				assert.Len(t, insts, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := patchEppPool(t, tt.body)
			if resp.ErrNum != tt.wantCode {
				t.Errorf("expected ErrNum=%d, got ErrNum=%d, ErrMsg=%s", tt.wantCode, resp.ErrNum, resp.ErrMsg)
			}
			if tt.check != nil && resp.ErrNum == 200 {
				tt.check(t, resp)
			}
		})
	}
}

// TestEppPool_Update_GroupSize 验证单实例组规模规则（每组恰 1 实例）的边界行为：
// 2 个及以上实例的组被拒绝且全量替换不生效。
func TestEppPool_Update_GroupSize(t *testing.T) {
	t.Run("EP-2-014 三实例组拒绝且池内容不变", func(t *testing.T) {
		base := map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": "g1",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
					},
				},
			},
		}
		testutil.AssertSuccess(t, patchEppPool(t, base))

		resp := patchEppPool(t, map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": "g1",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						map[string]interface{}{"id": "epp-b", "host": "10.0.0.2", "port": 9002},
						map[string]interface{}{"id": "epp-c", "host": "10.0.0.3", "port": 9002},
					},
				},
			},
		})
		testutil.AssertErrCode(t, resp, 422)

		// 拒绝后池内容保持基线（全量替换未生效）。
		getResp, err := testutil.GetClient().Get("/open-api/v1/epp-pool")
		require.NoError(t, err)
		testutil.AssertSuccess(t, getResp)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(getResp.Data, &data))
		groups := data["groups"].([]interface{})
		require.Len(t, groups, 1)
		insts := groups[0].(map[string]interface{})["instances"].([]interface{})
		require.Len(t, insts, 1)
		assert.Equal(t, "epp-a", insts[0].(map[string]interface{})["id"])
	})

	t.Run("EP-2-015 双实例组（主+备）拒绝且池内容不变", func(t *testing.T) {
		base := map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": "g1",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
					},
				},
			},
		}
		testutil.AssertSuccess(t, patchEppPool(t, base))

		resp := patchEppPool(t, map[string]interface{}{
			"groups": []interface{}{
				map[string]interface{}{
					"name": "g1",
					"instances": []interface{}{
						map[string]interface{}{"id": "epp-a", "host": "10.0.0.1", "port": 9002},
						map[string]interface{}{"id": "epp-b", "host": "10.0.0.2", "port": 9002},
					},
				},
			},
		})
		testutil.AssertErrCode(t, resp, 422)

		// 拒绝后池内容保持基线（全量替换未生效）。
		getResp, err := testutil.GetClient().Get("/open-api/v1/epp-pool")
		require.NoError(t, err)
		testutil.AssertSuccess(t, getResp)
		var data map[string]interface{}
		require.NoError(t, json.Unmarshal(getResp.Data, &data))
		groups := data["groups"].([]interface{})
		require.Len(t, groups, 1)
		insts := groups[0].(map[string]interface{})["instances"].([]interface{})
		require.Len(t, insts, 1)
		assert.Equal(t, "epp-a", insts[0].(map[string]interface{})["id"])
	})
}
