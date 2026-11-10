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

package entity_type

import (
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/validate"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

var EntityTypeOneRoute = &xreq.Endpoint{
	Path:       "/entity-types/{type_name}",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(EntityTypeOneAction),
	Authorizer: iauth.FA(iauth.FeatureEntityType, iauth.ActionRead),
}

type OneReq struct {
	TypeName *string `uri:"type_name" validate:"required"`
}

// Validate performs centralized business validation on the URI parameter.
func (p *OneReq) Validate() error {
	return validate.EntityTypeName(*p.TypeName)
}

func EntityTypeOneAction(req *http.Request) (interface{}, error) {
	oneReq := &OneReq{}
	if err := xreq.BindURI(req, oneReq); err != nil {
		return nil, err
	}

	one, err := container.EntityTypeManager.FetchEntityType(req.Context(), &entity.EntityTypeFilter{
		TypeName: oneReq.TypeName,
	})
	if err != nil {
		return nil, err
	}
	if one == nil {
		return nil, xerror.WrapRecordNotExist("Entity-Type")
	}

	return one, nil
}
