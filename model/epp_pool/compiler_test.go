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

package epp_pool

import (
	"encoding/json"
	"testing"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findProfilePlugin(t *testing.T, conf *EndpointPickerConfig, pluginRef string) *ProfilePluginConfig {
	t.Helper()
	require.Len(t, conf.SchedulingProfiles, 1)
	for _, p := range conf.SchedulingProfiles[0].Plugins {
		if p.PluginRef == pluginRef {
			return p
		}
	}
	return nil
}

func findPlugin(t *testing.T, conf *EndpointPickerConfig, name string) *PluginConfig {
	t.Helper()
	for _, p := range conf.Plugins {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func TestCompileEppConfig_LoadProfiles(t *testing.T) {
	cases := []struct {
		profile string
		kv      float64
		queue   float64
	}{
		{LoadProfileQueueFirst, 0.2, 1.0},
		{LoadProfileBalanced, 0.6, 0.6},
		{LoadProfileKVFirst, 1.0, 0.2},
	}

	for _, tc := range cases {
		conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
			LoadProfile: lib.PString(tc.profile),
		})

		kv := findProfilePlugin(t, conf, pluginNameKVScorer)
		require.NotNil(t, kv)
		require.NotNil(t, kv.Weight)
		assert.Equal(t, tc.kv, *kv.Weight)

		queue := findProfilePlugin(t, conf, pluginNameQueueScorer)
		require.NotNil(t, queue)
		require.NotNil(t, queue.Weight)
		assert.Equal(t, tc.queue, *queue.Weight)
	}

	// Default (unset) equals balanced.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	assert.Equal(t, 0.6, *findProfilePlugin(t, conf, pluginNameKVScorer).Weight)
	assert.Equal(t, 0.6, *findProfilePlugin(t, conf, pluginNameQueueScorer).Weight)
}

func TestCompileEppConfig_Affinity(t *testing.T) {
	cases := []struct {
		affinity string
		want     float64
	}{
		{AffinityLow, 0.3},
		{AffinityMedium, 0.6},
		{AffinityHigh, 1.0},
	}

	for _, tc := range cases {
		conf := CompileEppConfig("cluster-a", &EppConfigSimplified{Affinity: lib.PString(tc.affinity)})
		prefix := findProfilePlugin(t, conf, pluginNamePrefixScorer)
		require.NotNil(t, prefix)
		require.NotNil(t, prefix.Weight)
		assert.Equal(t, tc.want, *prefix.Weight)
	}

	// Default is medium.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	assert.Equal(t, 0.6, *findProfilePlugin(t, conf, pluginNamePrefixScorer).Weight)

	// off disables the affinity scorers entirely, even when feature switches on.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		Affinity:               lib.PString(AffinityOff),
		PrefixCacheAffinity:    lib.PBool(true),
		SessionAffinityEnabled: lib.PBool(true),
		SessionAffinityHeader:  lib.PString("x-session-id"),
	})
	assert.Nil(t, findPlugin(t, conf, pluginNamePrefixScorer))
	assert.Nil(t, findPlugin(t, conf, pluginNameSessionScorer))
	assert.Nil(t, findProfilePlugin(t, conf, pluginNamePrefixScorer))
	assert.Nil(t, findProfilePlugin(t, conf, pluginNameSessionScorer))
}

func TestCompileEppConfig_PrefixCacheAffinity(t *testing.T) {
	// Default: prefix scorer injected with the affinity default weight.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	require.NotNil(t, findPlugin(t, conf, pluginNamePrefixScorer))
	prefix := findProfilePlugin(t, conf, pluginNamePrefixScorer)
	require.NotNil(t, prefix)
	assert.Equal(t, 0.6, *prefix.Weight)

	// Explicit false: not injected.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		PrefixCacheAffinity: lib.PBool(false),
	})
	assert.Nil(t, findPlugin(t, conf, pluginNamePrefixScorer))
	assert.Nil(t, findProfilePlugin(t, conf, pluginNamePrefixScorer))
}

func TestCompileEppConfig_SessionAffinity(t *testing.T) {
	// Disabled by default: no session scorer.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	assert.Nil(t, findPlugin(t, conf, pluginNameSessionScorer))
	assert.Nil(t, findProfilePlugin(t, conf, pluginNameSessionScorer))

	// Enabled: injected with header source and the affinity default weight.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		SessionAffinityEnabled: lib.PBool(true),
		SessionAffinityHeader:  lib.PString("x-session-id"),
	})
	plugin := findPlugin(t, conf, pluginNameSessionScorer)
	require.NotNil(t, plugin)
	assert.Equal(t, pluginTypeSessionScorer, plugin.Type)
	assert.Equal(t, "session_id", plugin.Parameters["strategy"])
	sessionIDConfig, ok := plugin.Parameters["sessionIdConfig"].(map[string]interface{})
	require.True(t, ok)
	sources, ok := sessionIDConfig["sources"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, sources, 1)
	assert.Equal(t, "x-session-id", sources[0]["header"])

	profilePlugin := findProfilePlugin(t, conf, pluginNameSessionScorer)
	require.NotNil(t, profilePlugin)
	assert.Equal(t, 0.6, *profilePlugin.Weight)
}

func TestCompileEppConfig_FixedPlugins(t *testing.T) {
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})

	discovery := findPlugin(t, conf, pluginNameDiscovery)
	require.NotNil(t, discovery)
	assert.Equal(t, pluginTypeDiscovery, discovery.Type)
	assert.Equal(t, "cluster-a", discovery.Parameters["clusterName"])

	utilFilter := findPlugin(t, conf, pluginNameUtilFilter)
	require.NotNil(t, utilFilter)
	assert.Equal(t, pluginTypeUtilFilter, utilFilter.Type)
	conditions, ok := utilFilter.Parameters["conditions"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, conditions, 1)
	assert.Equal(t, metricKVCacheUtilization, conditions[0]["metric"])
	assert.Equal(t, DefaultKVCacheUtilizationMax, conditions[0]["maxValue"])
	assert.Equal(t, DefaultFallbackOnEmpty, utilFilter.Parameters["fallbackOnEmpty"])

	// Saturation detector is always injected.
	detector := findPlugin(t, conf, pluginNameSaturationDetector)
	require.NotNil(t, detector)
	assert.Equal(t, pluginTypeSaturationDetector, detector.Type)

	require.NotNil(t, findPlugin(t, conf, pluginNameKVScorer))
	require.NotNil(t, findPlugin(t, conf, pluginNameQueueScorer))
	require.NotNil(t, findPlugin(t, conf, pluginNameMaxScorePicker))
	require.NotNil(t, findPlugin(t, conf, pluginNameOpenAIParser))

	require.Equal(t, pluginNameDiscovery, conf.DataLayer.Discovery.Endpoints.PluginRef)
	require.Len(t, conf.RequestHandler.Parsers, 1)
	assert.Equal(t, pluginNameOpenAIParser, conf.RequestHandler.Parsers[0].PluginRef)

	// No flow_control: the flowControl section is still emitted (detector
	// reference + band 0), but the feature gate stays off.
	require.NotNil(t, conf.FlowControl)
	require.NotNil(t, conf.FlowControl.SaturationDetector)
	assert.Equal(t, pluginNameSaturationDetector, conf.FlowControl.SaturationDetector.PluginRef)
	assert.Empty(t, conf.FeatureGates)

	// Filter precedes scorers, picker is last.
	plugins := conf.SchedulingProfiles[0].Plugins
	require.Len(t, plugins, 5)
	assert.Equal(t, pluginNameUtilFilter, plugins[0].PluginRef)
	assert.Equal(t, pluginNameMaxScorePicker, plugins[len(plugins)-1].PluginRef)
}

func TestCompileEppConfig_UtilFilter(t *testing.T) {
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		KVCacheUtilizationMax: float64Ptr(0.5),
		WaitingQueueMax:       float64Ptr(8),
		RunningRequestsMax:    float64Ptr(16),
		FallbackOnEmpty:       lib.PBool(true),
	})
	params := findPlugin(t, conf, pluginNameUtilFilter).Parameters
	assert.Equal(t, true, params["fallbackOnEmpty"])

	conditions, ok := params["conditions"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, conditions, 3)
	assert.Equal(t, metricKVCacheUtilization, conditions[0]["metric"])
	assert.Equal(t, 0.5, conditions[0]["maxValue"])
	assert.Equal(t, metricWaitingQueue, conditions[1]["metric"])
	assert.Equal(t, float64(8), conditions[1]["maxValue"])
	assert.Equal(t, metricRunningRequests, conditions[2]["metric"])
	assert.Equal(t, float64(16), conditions[2]["maxValue"])
}

func TestCompileEppConfig_SaturationDetector(t *testing.T) {
	// Defaults.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	params := findPlugin(t, conf, pluginNameSaturationDetector).Parameters
	assert.Equal(t, DefaultKVCacheUtilizationMax, params["kvCacheUtilThreshold"])
	assert.Equal(t, defaultSaturationQueueDepthThreshold, params["queueDepthThreshold"])
	assert.Equal(t, stalenessPolicyIgnore, params["stalenessPolicy"])
	assert.Equal(t, 0.0, params["headroom"])
	assert.Equal(t, "200ms", params["metricsStalenessThreshold"])

	// Explicit thresholds: kv shared with filter; waiting mirrors queue depth;
	// staleness serialized as milliseconds.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		KVCacheUtilizationMax:       float64Ptr(0.85),
		WaitingQueueMax:             float64Ptr(8),
		MetricsStalenessThresholdMs: lib.PInt(300),
	})
	params = findPlugin(t, conf, pluginNameSaturationDetector).Parameters
	assert.Equal(t, 0.85, params["kvCacheUtilThreshold"])
	assert.Equal(t, 8, params["queueDepthThreshold"])
	assert.Equal(t, "300ms", params["metricsStalenessThreshold"])

	// A fractional waiting_queue_max below 1 falls back to the default depth
	// (the detector requires queueDepthThreshold > 0).
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{WaitingQueueMax: float64Ptr(0.5)})
	assert.Equal(t, defaultSaturationQueueDepthThreshold,
		findPlugin(t, conf, pluginNameSaturationDetector).Parameters["queueDepthThreshold"])
}

func TestCompileEppConfig_FlowControl(t *testing.T) {
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{
			MaxRequests:        lib.PInt(1000),
			QueueTTL:           lib.PInt(30),
			NoEndpointQueueTTL: lib.PInt(600),
			EnableEviction:     lib.PBool(true), // not emitted (unwired in EPP)
		},
	})

	require.NotNil(t, conf.FlowControl)
	assert.Equal(t, "1000", conf.FlowControl.MaxRequests)
	assert.Equal(t, "30s", conf.FlowControl.DefaultRequestTTL)
	assert.Equal(t, "10m0s", conf.FlowControl.NoEndpointRequestTTL)
	assert.False(t, conf.FlowControl.EnableEviction)
	assert.Equal(t, []string{featureGateFlowControl}, conf.FeatureGates)
	require.NotNil(t, conf.FlowControl.SaturationDetector)
	assert.Equal(t, pluginNameSaturationDetector, conf.FlowControl.SaturationDetector.PluginRef)

	// band 0 mirrors the global limit (no silent truncation by the llm-d
	// hidden band default of 5000).
	require.Len(t, conf.FlowControl.PriorityBands, 1)
	band := conf.FlowControl.PriorityBands[0]
	assert.Equal(t, 0, band.Priority)
	assert.Equal(t, "1000", band.MaxRequests)
	assert.Equal(t, defaultPriorityBandMaxBytes, band.MaxBytes)
}

func TestCompileEppConfig_FlowControlAlwaysEmitted(t *testing.T) {
	// No flow_control: the section is still present (detector + band 0), but
	// the feature gate stays off (no queueing/backpressure).
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	require.NotNil(t, conf.FlowControl)
	assert.Empty(t, conf.FlowControl.MaxRequests)
	require.NotNil(t, conf.FlowControl.SaturationDetector)
	require.Len(t, conf.FlowControl.PriorityBands, 1)
	assert.Equal(t, defaultPriorityBandMaxRequests, conf.FlowControl.PriorityBands[0].MaxRequests)
	assert.Equal(t, 0, conf.FlowControl.PriorityBands[0].Priority)
	assert.Empty(t, conf.FeatureGates)
}

func TestCompileEppConfig_FlowControlMaxRequestsOmitted(t *testing.T) {
	// Unset: global omitted, band 0 carries the explicit default.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{QueueTTL: lib.PInt(30)},
	})
	assert.Empty(t, conf.FlowControl.MaxRequests)
	require.Len(t, conf.FlowControl.PriorityBands, 1)
	assert.Equal(t, defaultPriorityBandMaxRequests, conf.FlowControl.PriorityBands[0].MaxRequests)
	assert.Equal(t, 0, conf.FlowControl.PriorityBands[0].Priority)

	// Explicit -1 (unlimited): global omitted, band 0 still bounded.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{MaxRequests: lib.PInt(-1)},
	})
	assert.Empty(t, conf.FlowControl.MaxRequests)
	require.Len(t, conf.FlowControl.PriorityBands, 1)
	assert.Equal(t, defaultPriorityBandMaxRequests, conf.FlowControl.PriorityBands[0].MaxRequests)
}

func TestCompileEppConfig_FlowControlDefaultTTL(t *testing.T) {
	// flow_control 存在但未配 queue_ttl：显式补默认 9s（不再回落 llm-d 的 60s，
	// 否则 60s 会大于 epp_runtime 调用超时默认 11s，默认组合不满足 call > max(TTL)）
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{MaxRequests: lib.PInt(100)},
	})
	assert.Equal(t, "9s", conf.FlowControl.DefaultRequestTTL)
	assert.Equal(t, "9s", conf.FlowControl.NoEndpointRequestTTL)

	// flow_control 整体缺省：不补 TTL（feature gate 关闭，无排队/背压语义）
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{})
	assert.Empty(t, conf.FlowControl.DefaultRequestTTL)
	assert.Empty(t, conf.FlowControl.NoEndpointRequestTTL)
}

func TestCompileEppConfig_FlowControlZeroTTL(t *testing.T) {
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{QueueTTL: lib.PInt(0)},
	})
	assert.Equal(t, "0s", conf.FlowControl.DefaultRequestTTL)
}

func TestCompileEppConfig_NoEndpointTTLExpansion(t *testing.T) {
	// no_endpoint_queue_ttl unset: expanded explicitly to queue_ttl.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{QueueTTL: lib.PInt(45)},
	})
	assert.Equal(t, "45s", conf.FlowControl.DefaultRequestTTL)
	assert.Equal(t, "45s", conf.FlowControl.NoEndpointRequestTTL)

	// Explicit value wins.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{QueueTTL: lib.PInt(45), NoEndpointQueueTTL: lib.PInt(600)},
	})
	assert.Equal(t, "45s", conf.FlowControl.DefaultRequestTTL)
	assert.Equal(t, "10m0s", conf.FlowControl.NoEndpointRequestTTL)
}

func TestCompileEppConfig_FlowControlJSONShape(t *testing.T) {
	// The saturation detector reference is always emitted; enableEviction is
	// never emitted (apix omitempty parity); maxRequests stays the string form
	// of the apix resource.Quantity.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{
			MaxRequests: lib.PInt(1000),
			QueueTTL:    lib.PInt(30),
		},
	})
	bs, err := json.Marshal(conf.FlowControl)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"maxRequests": "1000",
		"defaultRequestTTL": "30s",
		"noEndpointRequestTTL": "30s",
		"saturationDetector": {"pluginRef": "saturation-detector"},
		"priorityBands": [{"priority": 0, "maxRequests": "1000", "maxBytes": "5Gi"}]
	}`, string(bs))
	assert.NotContains(t, string(bs), "enableEviction")

	// Even an explicit enable_eviction=true is not emitted.
	conf = CompileEppConfig("cluster-a", &EppConfigSimplified{
		FlowControl: &FlowControlSimplified{EnableEviction: lib.PBool(true)},
	})
	bs, err = json.Marshal(conf.FlowControl)
	require.NoError(t, err)
	assert.NotContains(t, string(bs), "enableEviction")
}

func TestCompileEppConfig_PriorityBand0(t *testing.T) {
	tests := []struct {
		name        string
		maxRequests *int
		wantGlobal  string // empty means the global limit is omitted
		wantBand    string
	}{
		{"全局设置：band0 与全局一致", lib.PInt(2000), "2000", "2000"},
		{"未设置：全局不限，band0 用显式默认", nil, "", defaultPriorityBandMaxRequests},
		{"显式 -1 不限：band0 仍受限", lib.PInt(FlowControlUnlimited), "", defaultPriorityBandMaxRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := CompileEppConfig("cluster-a", &EppConfigSimplified{
				FlowControl: &FlowControlSimplified{MaxRequests: tt.maxRequests},
			})
			assert.Equal(t, tt.wantGlobal, conf.FlowControl.MaxRequests)

			require.Len(t, conf.FlowControl.PriorityBands, 1)
			band := conf.FlowControl.PriorityBands[0]
			assert.Equal(t, 0, band.Priority)
			assert.Equal(t, tt.wantBand, band.MaxRequests)
			assert.Equal(t, defaultPriorityBandMaxBytes, band.MaxBytes)
		})
	}
}

func TestCompileEppConfig_WeightTagParity(t *testing.T) {
	// apix SchedulingPlugin.Weight has no omitempty: the unweighted picker
	// serializes as "weight": null, which the EPP strict decode accepts.
	conf := CompileEppConfig("cluster-a", &EppConfigSimplified{})
	bs, err := json.Marshal(conf.SchedulingProfiles[0])
	require.NoError(t, err)

	var profile struct {
		Plugins []struct {
			PluginRef string   `json:"pluginRef"`
			Weight    *float64 `json:"weight"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(bs, &profile))

	weighted := map[string]bool{}
	for _, p := range profile.Plugins {
		if p.Weight != nil {
			weighted[p.PluginRef] = true
		}
	}
	assert.True(t, weighted[pluginNameKVScorer])
	assert.True(t, weighted[pluginNameQueueScorer])
	assert.True(t, weighted[pluginNamePrefixScorer])
	assert.False(t, weighted[pluginNameUtilFilter])
	assert.False(t, weighted[pluginNameMaxScorePicker])
}

func TestCompileEppConfig_DeterministicJSON(t *testing.T) {
	build := func() *EndpointPickerConfig {
		return CompileEppConfig("cluster-a", &EppConfigSimplified{
			LoadProfile:                 lib.PString(LoadProfileBalanced),
			KVCacheUtilizationMax:       float64Ptr(0.9),
			WaitingQueueMax:             float64Ptr(8),
			MetricsStalenessThresholdMs: lib.PInt(200),
			SessionAffinityEnabled:      lib.PBool(true),
			SessionAffinityHeader:       lib.PString("x-session-id"),
			FlowControl: &FlowControlSimplified{
				MaxRequests:        lib.PInt(1000),
				QueueTTL:           lib.PInt(30),
				NoEndpointQueueTTL: lib.PInt(600),
			},
		})
	}

	bs1, err := json.Marshal(build())
	require.NoError(t, err)
	bs2, err := json.Marshal(build())
	require.NoError(t, err)
	assert.JSONEq(t, string(bs1), string(bs2))

	// The compiled JSON must decode back into the minimal struct set.
	var roundtrip EndpointPickerConfig
	require.NoError(t, json.Unmarshal(bs1, &roundtrip))
	require.Len(t, roundtrip.SchedulingProfiles, 1)
}

func TestSecondsToDuration(t *testing.T) {
	assert.Equal(t, "0s", secondsToDuration(0))
	assert.Equal(t, "30s", secondsToDuration(30))
	assert.Equal(t, "10m0s", secondsToDuration(600))
	assert.Equal(t, "1h0m0s", secondsToDuration(3600))
}
