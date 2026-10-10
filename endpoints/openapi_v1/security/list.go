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

package security

import (
	"net/http"
	"strings"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/keyrotate"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

var ListRoute = &xreq.Endpoint{
	Path:       "/security/reencrypt-sweeps",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(ListAction),
	Authorizer: iauth.FA(iauth.FeatureSecurity, iauth.ActionRead),
}

type listParam struct {
	Status    *string `form:"status"`
	Mode      *string `form:"mode"`
	Scope     *string `form:"scope"`
	DryRun    *bool   `form:"dry_run"`
	StartTime *string `form:"start_time"`
	EndTime   *string `form:"end_time"`
	Page      *int    `form:"page"`
	PageSize  *int    `form:"page_size"`
	SortBy    *string `form:"sort_by"`
	SortOrder *string `form:"sort_order"`
}

type listResponse struct {
	List       []*taskResponse `json:"list"`
	Pagination struct {
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Total    int64 `json:"total"`
	} `json:"pagination"`
}

func ListAction(req *http.Request) (interface{}, error) {
	param := &listParam{}
	if err := xreq.BindForm(req, param); err != nil {
		return nil, err
	}

	filter := &keyrotate.TaskFilter{}

	if param.Status != nil {
		s := keyrotate.Status(*param.Status)
		switch s {
		case keyrotate.StatusRunning, keyrotate.StatusSucceeded, keyrotate.StatusFailed:
			filter.Status = &s
		default:
			return nil, xerror.WrapParamErrorWithMsg("status must be running, succeeded or failed")
		}
	}
	if param.Mode != nil {
		m := keyrotate.Mode(*param.Mode)
		switch m {
		case keyrotate.ModeReencrypt, keyrotate.ModeDecrypt:
			filter.Mode = &m
		default:
			return nil, xerror.WrapParamErrorWithMsg("mode must be reencrypt or decrypt")
		}
	}
	if param.Scope != nil {
		sc := keyrotate.Scope(*param.Scope)
		switch sc {
		case keyrotate.ScopeAll, keyrotate.ScopeProviders, keyrotate.ScopeAPIKeys:
			filter.Scope = &sc
		default:
			return nil, xerror.WrapParamErrorWithMsg("scope must be all, providers or api_keys")
		}
	}
	if param.DryRun != nil {
		filter.DryRun = param.DryRun
	}

	startTime, err := parseRFC3339Query("start_time", param.StartTime)
	if err != nil {
		return nil, err
	}
	endTime, err := parseRFC3339Query("end_time", param.EndTime)
	if err != nil {
		return nil, err
	}
	if startTime != nil && endTime != nil && endTime.Before(*startTime) {
		return nil, xerror.WrapParamErrorWithMsg("end_time must not be earlier than start_time")
	}
	filter.StartTime = startTime
	filter.EndTime = endTime

	page, pageSize := 1, 20
	if param.Page != nil && *param.Page > 0 {
		page = *param.Page
	}
	if param.PageSize != nil && *param.PageSize > 0 {
		pageSize = *param.PageSize
	}
	if pageSize > 100 {
		pageSize = 100
	}

	sortBy := "started_at"
	if param.SortBy != nil && (*param.SortBy == "started_at" || *param.SortBy == "task_id") {
		sortBy = *param.SortBy
	}
	sortOrder := "desc"
	if param.SortOrder != nil && strings.EqualFold(*param.SortOrder, "asc") {
		sortOrder = "asc"
	}

	filter.Page = page
	filter.PageSize = pageSize
	filter.SortBy = sortBy
	filter.SortOrder = sortOrder

	tasks, total, err := container.KeyRotateManager.ListTasks(req.Context(), filter)
	if err != nil {
		return nil, err
	}

	resp := &listResponse{List: make([]*taskResponse, 0, len(tasks))}
	for _, task := range tasks {
		resp.List = append(resp.List, taskToResponse(task))
	}
	resp.Pagination.Page = page
	resp.Pagination.PageSize = pageSize
	resp.Pagination.Total = total

	return resp, nil
}

func parseRFC3339Query(name string, value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return nil, xerror.WrapParamErrorWithMsg("%s must be RFC3339", name)
	}
	return &t, nil
}
