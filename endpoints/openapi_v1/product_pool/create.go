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

package product_pool

import (
	"net/http"
	"strings"

	"github.com/rainway-ai-gateway/ai-gateway-api/endpoints/openapi_v1/product_cluster"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/validate"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

const (
	defaultWeight = 1
)

// UpsertParam Request Param
// AUTO GEN BY ctrl, MODIFY AS U NEED
type UpsertParam struct {
	Name      *string                  `json:"name" uri:"instance_pool_name"`
	Instances []*Instance              `json:"instances" uri:"instances" validate:"min=1,dive"`
	EPPServer *icluster_conf.EPPServer `json:"epp_server"`
	Role      *string                  `json:"role"`
}

// Validate performs centralized business validation on the request parameters.
func (p *UpsertParam) Validate() error {
	if len(p.Instances) == 0 {
		return nil
	}

	for i, inst := range p.Instances {
		if err := validate.Hostname(inst.Hostname); err != nil {
			return xerror.WrapParamErrorWithMsg("instances[%d].hostname: %v", i, err)
		}
		if err := validate.IPAddress(inst.IP); err != nil {
			return xerror.WrapParamErrorWithMsg("instances[%d].ip: %v", i, err)
		}
		if inst.Weight < 0 || inst.Weight > 100 {
			return xerror.WrapParamErrorWithMsg("instances[%d].weight must be between 0 and 100", i)
		}
		if len(inst.Ports) == 0 {
			return xerror.WrapParamErrorWithMsg("instances[%d].ports cannot be empty", i)
		}
		if _, ok := inst.Ports["Default"]; !ok {
			return xerror.WrapParamErrorWithMsg("instances[%d].ports must contain Default", i)
		}
		portValueSet := map[int]struct{}{}
		for name, port := range inst.Ports {
			if len(name) == 0 {
				return xerror.WrapParamErrorWithMsg("instances[%d].ports port name cannot be empty", i)
			}
			if err := validate.Port(port); err != nil {
				return xerror.WrapParamErrorWithMsg("instances[%d].ports.%s: %v", i, name, err)
			}
			if _, ok := portValueSet[port]; ok {
				return xerror.WrapParamErrorWithMsg("instances[%d].ports duplicate port value %d", i, port)
			}
			portValueSet[port] = struct{}{}
		}
	}

	return icluster_conf.ValidateInstancePool(Instancesc2i(p.Instances))
}

// CreateRoute route
// AUTO GEN BY ctrl, MODIFY AS U NEED
// deprecated, endpoint registration removed per optimization plan v1.2
// var CreateEndpoint = &xreq.Endpoint{
// 	Path:       "/instance-pools",
// 	Method:     http.MethodPost,
// 	Handler:    xreq.Convert(CreateAction),
// 	Authorizer: iauth.FAP(iauth.FeatureProductPool, iauth.ActionCreate),
// }

// AUTO GEN BY ctrl, MODIFY AS U NEED
func NewUpsertParam(req *http.Request) (*UpsertParam, error) {
	param := &UpsertParam{}
	err := xreq.Bind(req, param)
	if err != nil {
		return nil, err
	}

	if param.Role == nil || *param.Role == "" {
		param.Role = lib.PString(icluster_conf.ProductPoolRoleCommon)
	}

	switch *param.Role {
	case icluster_conf.ProductPoolRoleCommon:
	case icluster_conf.ProductPoolRoleEPP:
		if err := product_cluster.CheckEPPServer(param.EPPServer); err != nil {
			return nil, err
		}
	}

	return param, err
}

var _ xreq.Handler = CreateAction

// CreateAction action
// AUTO GEN BY ctrl, MODIFY AS U NEED
func CreateAction(req *http.Request) (interface{}, error) {
	param, err := NewUpsertParam(req)
	if err != nil {
		return nil, err
	}

	if param.Name == nil || *param.Name == "" {
		return nil, xerror.WrapParamErrorWithMsg("instance pool name is required")
	}

	product, err := getDefaultProduct(req.Context())
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(*param.Name, product.Name+".") {
		return nil, xerror.WrapParamErrorWithMsg("Want Prefix %s.", product.Name)
	}
	if len(*param.Name) == len(product.Name)+1 {
		return nil, xerror.WrapParamErrorWithMsg("Want Pool Name")
	}

	oneData, err := CreateProcess(req, product, param)
	if err != nil {
		return nil, err
	}

	return NewOneData(oneData), nil
}

func Instancesc2i(is []*Instance) []icluster_conf.Instance {
	rst := []icluster_conf.Instance{}
	for _, instance := range is {
		port := 0
		if instance.Ports != nil {
			port = instance.Ports["Default"]
		}

		weight := instance.Weight
		if weight == 0 {
			weight = defaultWeight
		}

		name := instance.Hostname
		if name == "" {
			name = instance.IP
		}

		rst = append(rst, icluster_conf.Instance{
			Name:   name,
			Addr:   instance.IP,
			Weight: weight,
			Port:   port,
		})
	}

	return rst
}

func CreateProcess(req *http.Request, product *ibasic.Product, param *UpsertParam) (*icluster_conf.Pool, error) {
	return container.PoolManager.CreateProductPool(req.Context(), product, &icluster_conf.PoolParam{
		Name:      param.Name,
		Instances: Instancesc2i(param.Instances),
		Role:      param.Role,
		EPPServer: param.EPPServer,
	})
}
