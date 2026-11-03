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

package api_key

import (
	"context"
	"fmt"
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xmask"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
)

var ListRoute = &xreq.Endpoint{
	Path:       "/api-keys",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(ListAction),
	Authorizer: iauth.FA(iauth.FeatureAPIKey, iauth.ActionReadAll),
}

var _ xreq.Handler = OneAction

type APIKeyListReq struct {
	Page           *int    `form:"page"`
	PageSize       *int    `form:"page_size"`
	Enabled        *bool   `form:"enabled"`
	EntityID       *string `form:"entity_id"`
	UnlimitedQuota *bool   `form:"unlimited_quota"`
}

type APIKeyListResponse struct {
	List       []*api_key.APIKeyParam `json:"list"`
	Pagination struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
	} `json:"pagination"`
}

func ListAction(req *http.Request) (interface{}, error) {
	listReq := &APIKeyListReq{}
	if err := xreq.BindForm(req, listReq); err != nil {
		return nil, err
	}

	if err := xreq.ValidatePagination(listReq.Page, listReq.PageSize); err != nil {
		return nil, err
	}

	if listReq.EntityID != nil && len(*listReq.EntityID) > 64 {
		return nil, xerror.WrapParamErrorWithMsg("%s", fmt.Sprintf("entity_id must be <= 64 characters, got %d", len(*listReq.EntityID)))
	}

	page, pageSize := xreq.ResolvePagination(listReq.Page, listReq.PageSize)

	productName := defaultProductName

	filter := &api_key.APIKeyFilter{
		ProductName:    &productName,
		Enabled:        listReq.Enabled,
		EntityID:       listReq.EntityID,
		UnlimitedQuota: listReq.UnlimitedQuota,
		Page:           &page,
		PageSize:       &pageSize,
	}

	list, err := container.APIKeyManager.FetchAPIKeyList(req.Context(), filter)
	if err != nil {
		return nil, err
	}

	list, err = newResponse(req.Context(), list)
	if err != nil {
		return nil, err
	}

	// total counts the whole filtered collection; reuse the same filters so it
	// never drifts from list (issue #303).
	countFilter := *filter
	countFilter.Page = nil
	countFilter.PageSize = nil
	allList, err := container.APIKeyManager.FetchAPIKeyList(req.Context(), &countFilter)
	if err != nil {
		return nil, err
	}
	total := len(allList)

	if list == nil {
		list = []*api_key.APIKeyParam{}
	}

	resp := &APIKeyListResponse{
		List: list,
	}
	resp.Pagination.Page = page
	resp.Pagination.PageSize = pageSize
	resp.Pagination.Total = total

	return resp, nil
}

func newResponse(ctx context.Context, list []*api_key.APIKeyParam) ([]*api_key.APIKeyParam, error) {
	v, err := iauth.MustGetVisitor(ctx)
	if err != nil {
		return nil, err
	}
	readonly := v.IsSystemReadonly()

	for i, one := range list {
		if one.Key != nil {
			remainingQuota, err := api_key.GetRemainingQuota(ctx, container.QuotaCacheSingleton, one)
			if err != nil {
				return nil, err
			}
			list[i].RemainingQuota = remainingQuota
			if readonly {
				*list[i].Key = xmask.Secret(*one.Key)
			}
		}
	}

	return list, nil
}
