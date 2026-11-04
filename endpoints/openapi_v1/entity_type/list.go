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

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

var EntityTypeListRoute = &xreq.Endpoint{
	Path:       "/entity-types",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(EntityTypeListAction),
	Authorizer: iauth.FA(iauth.FeatureEntityType, iauth.ActionReadAll),
}

type EntityTypeListResponse struct {
	List       []*entity.EntityTypeParam `json:"list"`
	Pagination struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
	} `json:"pagination"`
}

func EntityTypeListAction(req *http.Request) (interface{}, error) {
	filter := &entity.EntityTypeFilter{}
	if err := xreq.BindForm(req, filter); err != nil {
		return nil, err
	}

	// 未传 page/page_size 时表示「返回全部匹配记录」（entity-types.md §2.2，语义同 /providers）；
	// 空串参数（?page= / ?page_size=）按未传处理。
	page, pageSize, paginated := xreq.OptionalPagination(req.URL.Query(), filter.Page, filter.PageSize)
	if paginated {
		filter.Page = &page
		filter.PageSize = &pageSize
	}

	list, err := container.EntityTypeManager.FetchEntityTypeList(req.Context(), filter)
	if err != nil {
		return nil, err
	}

	total := len(list)
	if paginated {
		// total counts the whole filtered collection; reuse the same filters so it
		// never drifts from list (issue #303).
		countFilter := *filter
		countFilter.Page = nil
		countFilter.PageSize = nil
		allList, err := container.EntityTypeManager.FetchEntityTypeList(req.Context(), &countFilter)
		if err != nil {
			return nil, err
		}
		total = len(allList)
	} else if total > 0 {
		pageSize = total // page_size=total，与 /providers 一致；空集合保持 0
	}

	if list == nil {
		list = []*entity.EntityTypeParam{} // 空集合返回 []，不要返回 null
	}

	resp := &EntityTypeListResponse{
		List: list,
	}
	resp.Pagination.Page = page
	resp.Pagination.PageSize = pageSize
	resp.Pagination.Total = total

	return resp, nil
}
