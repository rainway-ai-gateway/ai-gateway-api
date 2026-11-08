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

package certificate

import (
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/validate"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprotocol"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// CreateRoute route
// AUTO GEN BY ctrl, MODIFY AS U NEED
var CreateEndpoint = &xreq.Endpoint{
	Path:       "/certificates",
	Method:     http.MethodPost,
	Handler:    xreq.Convert(CreateAction),
	Authorizer: iauth.FA(iauth.FeatureCert, iauth.ActionCreate),
}

// CreateParam Request Param
// AUTO GEN BY ctrl, MODIFY AS U NEED
type CreateParam struct {
	Name *string `json:"cert_name" validate:"required,min=2"`

	// The lower bound is checked in CreateAction via
	// validate.DescriptionRange (UTF-8 bytes), not via a `min` tag: the
	// validator counts `min` in runes while certificates.md §1/§2.1 defines
	// the bound in bytes (issue #310).
	Description *string `json:"description" validate:"required"`
	IsDefault   *bool   `json:"is_default" validate:"required"`

	CertFileContent *string `json:"cert_file_content" validate:"required,min=2"`
	KeyFileContent  *string `json:"key_file_content" validate:"required,min=2"`
}

// AUTO GEN BY ctrl, MODIFY AS U NEED
func newCreateParam4Create(req *http.Request) (*CreateParam, error) {
	param := &CreateParam{}
	err := xreq.BindJSON(req, param)
	return param, err
}

func createActionProcess(req *http.Request, param *CreateParam) (*OneData, error) {
	if err := container.CertificateManager.CreateCertificate(req.Context(), &iprotocol.CertificateParam{
		CertName:        param.Name,
		Description:     param.Description,
		IsDefault:       param.IsDefault,
		CertFileContent: param.CertFileContent,
		KeyFileContent:  param.KeyFileContent,
	}); err != nil {
		return nil, err
	}

	list, err := container.CertificateManager.FetchCertificates(req.Context(), &iprotocol.CertificateFilter{
		CertName: param.Name,
	})
	if err != nil {
		return nil, err
	}

	return newOneData(list[0]), nil
}

var _ xreq.Handler = CreateAction

// CreateAction action
// AUTO GEN BY ctrl, MODIFY AS U NEED
func CreateAction(req *http.Request) (interface{}, error) {
	param, err := newCreateParam4Create(req)
	if err != nil {
		return nil, err
	}

	if err := validate.CertName(*param.Name); err != nil {
		return nil, err
	}

	// certificates.md §1/§2.1：description 必填、长度 2-256 字节（按 UTF-8
	// 字节计）、不能包含控制字符。下限与上限统一在 DescriptionRange 按字节
	// 校验（issue #310）；校验位于 createActionProcess 之前，被拒请求不落库、
	// 不触发默认证书切换。
	if err := validate.DescriptionRange(*param.Description, 2, validate.MaxCertificateDescriptionLength, "description"); err != nil {
		return nil, err
	}

	return createActionProcess(req, param)
}
