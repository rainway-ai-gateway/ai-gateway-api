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
//limitations under the License. All rights reserved.

// Copyright (c) 2021 The BFE Authors.
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

package product_cluster

import (
	"encoding/json"
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/validate"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
)

// StickySessions Request Param
type StickySessions struct {
	Enabled      bool   `json:"enabled"`
	HashStrategy string `json:"hash_strategy"`
	HashHeader   string `json:"hash_header"`
}

// Basic Request Param
type Basic struct {
	Connection *Connection `json:"connection"`
	Retries    *Retries    `json:"retries"`
	Buffers    *Buffers    `json:"buffers"`
	Timeouts   *Timeouts   `json:"timeouts"`
	Protocol   *string     `json:"protocol"`
}

// Connection Request Param
type Connection struct {
	MaxIdleConnPerRs    int16 `json:"max_idle_conn_per_rs"`
	CancelOnClientClose bool  `json:"cancel_on_client_close"`
}

// Retries Request Param
type Retries struct {
	MaxRetryInCluster int8 `json:"max_retry_in_cluster"`
}

// Buffers Request Param
type Buffers struct {
	ReqWriteBufferSize int32 `json:"req_write_buffer_size"`
}

// Timeouts Request Param
type Timeouts struct {
	TimeoutConnServ        int32 `json:"timeout_conn_serv"`
	TimeoutResponseHeader  int32 `json:"timeout_response_header"`
	TimeoutReadbodyClient  int32 `json:"timeout_readbody_client"`
	TimeoutReadClientAgain int32 `json:"timeout_read_client_again"`
	TimeoutWriteClient     int32 `json:"timeout_write_client"`
}

// OneParam Request Param
type OneParam struct {
	Name *string `uri:"cluster_name" validate:"required,min=1"`
}

// Validate performs centralized business validation on the request parameters.
// The charset check keeps GET/DELETE/ready consistent with PATCH/POST (which
// validate via UpsertParam): an illegal cluster_name is a 422 Param Illegal,
// not a 404 Record Not Exist (issue #311).
func (p *OneParam) Validate() error {
	return validate.ClusterName(*p.Name)
}

type PassiveHealthCheck struct {
	Interval   int32  `json:"interval"`
	Failnum    int32  `json:"failnum"`
	Statuscode int32  `json:"statuscode"`
	Host       string `json:"host"`
	Uri        string `json:"uri"`
}

// ClusterData Request Param
type ClusterData struct {
	Name               string                   `json:"name"`
	Description        string                   `json:"description"`
	Basic              *Basic                   `json:"basic"`
	StickySessions     *StickySessions          `json:"sticky_sessions"`
	PassiveHealthCheck *PassiveHealthCheck      `json:"passive_health_check"`
	LLMConfig          *icluster_conf.LLMConfig `json:"llm_config"`
	// BalanceMode is the explicit cluster balance mode (WRR default, EPP
	// optional); it is the single source of truth for the export BalanceMode.
	BalanceMode string `json:"balance_mode"`
	// EppConfig is the raw stored JSON of the simplified per-cluster EPP
	// scheduling configuration, returned verbatim (null when never set;
	// retained verbatim while dormant under WRR).
	EppConfig json.RawMessage `json:"epp_config"`
	// EppRuntime is the raw stored JSON of the BFE-side EPP runtime parameters
	// (check/timeout), returned verbatim (null when never set; retained
	// verbatim while dormant under WRR).
	EppRuntime json.RawMessage `json:"epp_runtime"`
}

type AutoLbMatrix struct {
	MaxRegionLoad    float64          `json:"max_region_load"`
	MaxBlackholeLoad float64          `json:"max_blackhole_load"`
	BlackholeEnabled bool             `json:"blackhole_enabled"`
	Capacity         map[string]int64 `json:"capacity"`
}

func clusterModel2Control(cluster *icluster_conf.Cluster) *ClusterData {
	rsp := &ClusterData{
		Name:        cluster.Name,
		Description: cluster.Description,
		Basic: &Basic{
			Connection: &Connection{
				MaxIdleConnPerRs:    cluster.Basic.Connection.MaxIdleConnPerRs,
				CancelOnClientClose: cluster.Basic.Connection.CancelOnClientClose,
			},
			Retries: &Retries{
				MaxRetryInCluster: cluster.Basic.Retries.MaxRetryInSubcluster,
			},
			Buffers: &Buffers{
				ReqWriteBufferSize: cluster.Basic.Buffers.ReqWriteBufferSize,
			},
			Timeouts: &Timeouts{
				TimeoutConnServ:        cluster.Basic.Timeouts.TimeoutConnServ,
				TimeoutResponseHeader:  cluster.Basic.Timeouts.TimeoutResponseHeader,
				TimeoutReadbodyClient:  cluster.Basic.Timeouts.TimeoutReadbodyClient,
				TimeoutReadClientAgain: cluster.Basic.Timeouts.TimeoutReadClientAgain,
				TimeoutWriteClient:     cluster.Basic.Timeouts.TimeoutWriteClient,
			},
			Protocol: cluster.Basic.Protocol,
		},
		StickySessions: &StickySessions{
			Enabled: cluster.StickySessions.SessionSticky,
			HashStrategy: map[int32]string{
				icluster_conf.ClusterHashStrategyClientIDOnlyI:     clusterHashStrategyClientIDOnly,
				icluster_conf.ClusterHashStrategyClientIPOnlyI:     clusterHashStrategyClientIPOnly,
				icluster_conf.ClusterHashStrategyClientIDPreferedI: clusterHashStrategyClientIDPrefered,
			}[cluster.StickySessions.HashStrategy],
			HashHeader: cluster.StickySessions.HashHeader,
		},

		PassiveHealthCheck: PassiveHealthCheckM2C(cluster.PassiveHealthCheck),

		LLMConfig: cluster.LLMConfig,
	}

	// balance_mode defaults to WRR when never written; epp_config is echoed
	// verbatim as the stored raw JSON (null when never set).
	rsp.BalanceMode = cluster.BalanceMode
	if rsp.BalanceMode == "" {
		rsp.BalanceMode = icluster_conf.BalanceModeWRR
	}
	if cluster.EppConfig != "" {
		rsp.EppConfig = json.RawMessage(cluster.EppConfig)
	}
	if cluster.EppRuntime != "" {
		rsp.EppRuntime = json.RawMessage(cluster.EppRuntime)
	}

	return rsp
}

func PassiveHealthCheckM2C(phc *icluster_conf.ClusterPassiveHealthCheck) *PassiveHealthCheck {
	if phc == nil {
		return nil
	}

	return &PassiveHealthCheck{
		Interval:   phc.Interval,
		Failnum:    phc.Failnum,
		Statuscode: phc.Statuscode,
		Host:       phc.Host,
		Uri:        phc.Uri,
	}
}

// OneRoute route
var OneEndpoint = &xreq.Endpoint{
	Path:       "/clusters/{cluster_name}",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(OneAction),
	Authorizer: iauth.FA(iauth.FeatureProductCluster, iauth.ActionRead),
}

// AUTO GEN BY ctrl, MODIFY AS U NEED
func newOneParam4One(req *http.Request) (*OneParam, error) {
	param := &OneParam{}
	err := xreq.BindURI(req, param)
	return param, err
}

var _ xreq.Handler = OneAction

// OneAction action
func OneAction(req *http.Request) (interface{}, error) {
	param, err := newOneParam4One(req)
	if err != nil {
		return nil, err
	}

	return oneActionProcess(req, param)
}

func oneActionProcess(req *http.Request, param *OneParam) (*ClusterData, error) {
	product, err := getDefaultProduct(req.Context())
	if err != nil {
		return nil, err
	}

	one, err := container.ClusterManager.FetchCluster(req.Context(), &icluster_conf.ClusterFilter{
		Name:    param.Name,
		Product: product,
	})
	if err != nil {
		return nil, err
	}
	if one == nil {
		return nil, xerror.WrapRecordNotExist("Cluster")
	}

	return clusterModel2Control(one), nil
}
