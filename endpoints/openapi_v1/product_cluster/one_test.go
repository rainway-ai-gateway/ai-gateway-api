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

package product_cluster

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterModel2Control(t *testing.T) {
	cluster := &icluster_conf.Cluster{
		Name:        "my-cluster",
		Description: "test cluster",
		Basic: &icluster_conf.ClusterBasic{
			Protocol: lib.PString("https"),
			Connection: &icluster_conf.ClusterBasicConnection{
				MaxIdleConnPerRs:    0,
				CancelOnClientClose: false,
			},
			Retries: &icluster_conf.ClusterBasicRetries{
				MaxRetryInSubcluster: 2,
			},
			Buffers: &icluster_conf.ClusterBasicBuffers{
				ReqWriteBufferSize: 512,
				ReqFlushInterval:   icluster_conf.ClusterDefaultReqFlushInterval,
				ResFlushInterval:   icluster_conf.ClusterDefaultResFlushInterval,
			},
			Timeouts: &icluster_conf.ClusterBasicTimeouts{
				TimeoutConnServ:        50000,
				TimeoutResponseHeader:  50000,
				TimeoutReadbodyClient:  30000,
				TimeoutReadClientAgain: 30000,
				TimeoutWriteClient:     60000,
			},
		},
		StickySessions: &icluster_conf.ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  icluster_conf.ClusterHashStrategyClientIPOnlyI,
			HashHeader:    "",
		},
		PassiveHealthCheck: &icluster_conf.ClusterPassiveHealthCheck{
			Interval:   1000,
			Failnum:    3,
			Statuscode: 0,
			Host:       "",
			Uri:        "/",
		},
		LLMConfig: &icluster_conf.LLMConfig{
			Provider: lib.PString("openai"),
			Models:   []string{"gpt-4"},
		},
	}

	rsp := clusterModel2Control(cluster)
	assert.Equal(t, "my-cluster", rsp.Name)
	assert.Equal(t, "openai", *rsp.LLMConfig.Provider)

	// balance_mode defaults to WRR and epp_config is null when never set.
	assert.Equal(t, icluster_conf.BalanceModeWRR, rsp.BalanceMode)
	assert.Nil(t, rsp.EppConfig)

	data, err := json.Marshal(rsp)
	assert.NoError(t, err)

	body := string(data)
	assert.False(t, strings.Contains(body, `"instance_pool"`))
	assert.False(t, strings.Contains(body, `"hostname"`))
	assert.False(t, strings.Contains(body, `"ip"`))
	assert.False(t, strings.Contains(body, `"ports"`))
	assert.Contains(t, body, `"balance_mode":"WRR"`)
	assert.Contains(t, body, `"epp_config":null`)
}

func TestClusterModel2Control_EPPFields(t *testing.T) {
	// balance_mode=EPP with a stored raw epp_config: both are echoed
	// verbatim (the dormant WRR retention keeps the stored JSON as-is).
	cluster := &icluster_conf.Cluster{
		Name: "epp-cluster",
		Basic: &icluster_conf.ClusterBasic{
			Protocol: lib.PString("https"),
			Connection: &icluster_conf.ClusterBasicConnection{
				MaxIdleConnPerRs:    0,
				CancelOnClientClose: false,
			},
			Retries: &icluster_conf.ClusterBasicRetries{
				MaxRetryInSubcluster: 2,
			},
			Buffers: &icluster_conf.ClusterBasicBuffers{
				ReqWriteBufferSize: 512,
			},
			Timeouts: &icluster_conf.ClusterBasicTimeouts{
				TimeoutConnServ:        50000,
				TimeoutResponseHeader:  50000,
				TimeoutReadbodyClient:  30000,
				TimeoutReadClientAgain: 30000,
				TimeoutWriteClient:     60000,
			},
		},
		StickySessions: &icluster_conf.ClusterStickySessions{
			SessionSticky: false,
			HashStrategy:  icluster_conf.ClusterHashStrategyClientIPOnlyI,
		},
		BalanceMode: icluster_conf.BalanceModeEPP,
		EppConfig:   `{"load_profile":"balanced","kv_cache_utilization_max":0.9}`,
	}

	rsp := clusterModel2Control(cluster)
	assert.Equal(t, icluster_conf.BalanceModeEPP, rsp.BalanceMode)
	assert.JSONEq(t,
		`{"load_profile":"balanced","kv_cache_utilization_max":0.9}`,
		string(rsp.EppConfig))

	data, err := json.Marshal(rsp)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"balance_mode":"EPP"`)
	assert.Contains(t, string(data), `"epp_config":{"load_profile":"balanced","kv_cache_utilization_max":0.9}`)

	// WRR dormancy: a stored epp_config is still returned verbatim.
	cluster.BalanceMode = icluster_conf.BalanceModeWRR
	rsp = clusterModel2Control(cluster)
	assert.Equal(t, icluster_conf.BalanceModeWRR, rsp.BalanceMode)
	assert.JSONEq(t,
		`{"load_profile":"balanced","kv_cache_utilization_max":0.9}`,
		string(rsp.EppConfig))
}

func TestClusterModel2Control_EppRuntimeFields(t *testing.T) {
	baseCluster := func() *icluster_conf.Cluster {
		return &icluster_conf.Cluster{
			Name: "epp-cluster",
			Basic: &icluster_conf.ClusterBasic{
				Protocol: lib.PString("https"),
				Connection: &icluster_conf.ClusterBasicConnection{
					MaxIdleConnPerRs:    0,
					CancelOnClientClose: false,
				},
				Retries: &icluster_conf.ClusterBasicRetries{
					MaxRetryInSubcluster: 2,
				},
				Buffers: &icluster_conf.ClusterBasicBuffers{
					ReqWriteBufferSize: 512,
				},
				Timeouts: &icluster_conf.ClusterBasicTimeouts{
					TimeoutConnServ:        50000,
					TimeoutResponseHeader:  50000,
					TimeoutReadbodyClient:  30000,
					TimeoutReadClientAgain: 30000,
					TimeoutWriteClient:     60000,
				},
			},
			StickySessions: &icluster_conf.ClusterStickySessions{
				SessionSticky: false,
				HashStrategy:  icluster_conf.ClusterHashStrategyClientIPOnlyI,
			},
			BalanceMode: icluster_conf.BalanceModeEPP,
			EppConfig:   `{"load_profile":"balanced"}`,
		}
	}

	t.Run("stored epp_runtime echoed verbatim", func(t *testing.T) {
		cluster := baseCluster()
		cluster.EppRuntime = `{"check":{"check_interval":"5s"},"timeout":{"call":"4s"}}`
		rsp := clusterModel2Control(cluster)
		assert.JSONEq(t,
			`{"check":{"check_interval":"5s"},"timeout":{"call":"4s"}}`,
			string(rsp.EppRuntime))

		data, err := json.Marshal(rsp)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"epp_runtime":{"check":{"check_interval":"5s"},"timeout":{"call":"4s"}}`)
	})

	t.Run("unset epp_runtime marshals as null", func(t *testing.T) {
		cluster := baseCluster()
		rsp := clusterModel2Control(cluster)
		assert.Nil(t, rsp.EppRuntime)

		data, err := json.Marshal(rsp)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"epp_runtime":null`)
	})
}

func TestOneParamValidate(t *testing.T) {
	cases := []struct {
		name    string
		param   *OneParam
		wantErr bool
	}{
		{
			name:    "valid",
			param:   &OneParam{Name: lib.PString("cluster-1")},
			wantErr: false,
		},
		{
			// issue #311 repro: GET/DELETE used to fall through to a 404.
			name:    "257 chars",
			param:   &OneParam{Name: lib.PString(strings.Repeat("a", 257))},
			wantErr: true,
		},
		{
			name:    "65 chars exceeds 1-64",
			param:   &OneParam{Name: lib.PString(strings.Repeat("a", 65))},
			wantErr: true,
		},
		{
			name:    "out-of-charset character",
			param:   &OneParam{Name: lib.PString("cluster+1")},
			wantErr: true,
		},
		{
			name:    "trailing space",
			param:   &OneParam{Name: lib.PString("cluster ")},
			wantErr: true,
		},
		{
			name:    "leading dash",
			param:   &OneParam{Name: lib.PString("-cluster")},
			wantErr: true,
		},
		{
			name:    "trailing dot",
			param:   &OneParam{Name: lib.PString("cluster.")},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.param.Validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
