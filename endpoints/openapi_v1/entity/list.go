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

package entity

import (
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

var EntityListRoute = &xreq.Endpoint{
	Path:       "/entities",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(EntityListAction),
	Authorizer: iauth.FA(iauth.FeatureEntity, iauth.ActionReadAll),
}

type EntityListResponse struct {
	List       []*entity.EntityParam `json:"list"`
	Pagination struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
	} `json:"pagination"`
}

func EntityListAction(req *http.Request) (interface{}, error) {
	filter := &entity.EntityFilter{}
	if err := xreq.BindForm(req, filter); err != nil {
		return nil, err
	}

	page, pageSize := xreq.ResolvePagination(filter.Page, filter.PageSize)
	filter.Page = &page
	filter.PageSize = &pageSize

	if filter.Type != nil && *filter.Type == "" {
		filter.Type = nil
	}

	if filter.ParentID != nil && *filter.ParentID == "" {
		filter.ParentID = nil
	}

	list, err := container.EntityManager.FetchEntityList(req.Context(), filter)
	if err != nil {
		return nil, err
	}

	// total counts the whole filtered collection; reuse the same filters so it
	// never drifts from list (issue #303).
	countFilter := *filter
	countFilter.Page = nil
	countFilter.PageSize = nil
	allList, err := container.EntityManager.FetchEntityList(req.Context(), &countFilter)
	if err != nil {
		return nil, err
	}
	total := len(allList)

	resp := &EntityListResponse{
		List: list,
	}
	resp.Pagination.Page = page
	resp.Pagination.PageSize = pageSize
	resp.Pagination.Total = total

	return resp, nil
}
