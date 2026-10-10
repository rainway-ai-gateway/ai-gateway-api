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

package security

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/keyrotate"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

var TriggerRoute = &xreq.Endpoint{
	Path:       "/security/reencrypt-sweeps",
	Method:     http.MethodPost,
	Handler:    xreq.Convert(TriggerAction),
	Authorizer: iauth.FA(iauth.FeatureSecurity, iauth.ActionUpdate),
}

var GetRoute = &xreq.Endpoint{
	Path:       "/security/reencrypt-sweeps/{task_id}",
	Method:     http.MethodGet,
	Handler:    xreq.Convert(GetAction),
	Authorizer: iauth.FA(iauth.FeatureSecurity, iauth.ActionRead),
}

var Routes = []*xreq.Endpoint{TriggerRoute, ListRoute, GetRoute}

type triggerParam struct {
	Mode   *string `json:"mode"`
	DryRun *bool   `json:"dry_run"`
	Scope  *string `json:"scope"`
}

type triggerResponse struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	DryRun      bool   `json:"dry_run"`
	Scope       string `json:"scope"`
	ActiveKeyID int    `json:"active_key_id"`
}

type tableCount struct {
	Scanned   int64 `json:"scanned"`
	Rewritten int64 `json:"rewritten"`
	Skipped   int64 `json:"skipped"`
}

type taskResponse struct {
	TaskID      string                 `json:"task_id"`
	Status      string                 `json:"status"`
	Mode        string                 `json:"mode"`
	DryRun      bool                   `json:"dry_run"`
	Scope       string                 `json:"scope"`
	ActiveKeyID int                    `json:"active_key_id"`
	CreatedBy   string                 `json:"created_by"`
	StartedAt   string                 `json:"started_at"`
	FinishedAt  string                 `json:"finished_at,omitempty"`
	DurationMs  int64                  `json:"duration_ms"`
	Summary     map[string]*tableCount `json:"summary"`
	Error       string                 `json:"error"`
}

// taskToResponse maps a sweep task to the API task object (shared by the
// by-id and list endpoints).
func taskToResponse(task *keyrotate.SweepTask) *taskResponse {
	summary := map[string]*tableCount{}
	for name, c := range task.Summary {
		summary[name] = &tableCount{Scanned: c.Scanned, Rewritten: c.Rewritten, Skipped: c.Skipped}
	}
	resp := &taskResponse{
		TaskID:      task.TaskID,
		Status:      string(task.Status),
		Mode:        string(task.Mode),
		DryRun:      task.DryRun,
		Scope:       string(task.Scope),
		ActiveKeyID: task.ActiveKeyID,
		CreatedBy:   task.CreatedBy,
		StartedAt:   task.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
		DurationMs:  task.DurationMs,
		Summary:     summary,
		Error:       task.FailedCause,
	}
	if task.FinishedAt != nil {
		resp.FinishedAt = task.FinishedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return resp
}

func TriggerAction(req *http.Request) (interface{}, error) {
	param := &triggerParam{}
	if err := xreq.BindJSON(req, param); err != nil {
		return nil, err
	}

	var mode keyrotate.Mode
	if param.Mode != nil {
		switch keyrotate.Mode(*param.Mode) {
		case keyrotate.ModeReencrypt, keyrotate.ModeDecrypt:
			mode = keyrotate.Mode(*param.Mode)
		default:
			return nil, xerror.WrapParamErrorWithMsg("mode must be reencrypt or decrypt")
		}
	}
	var scope keyrotate.Scope
	if param.Scope != nil {
		switch keyrotate.Scope(*param.Scope) {
		case keyrotate.ScopeAll, keyrotate.ScopeProviders, keyrotate.ScopeAPIKeys:
			scope = keyrotate.Scope(*param.Scope)
		default:
			return nil, xerror.WrapParamErrorWithMsg("scope must be all, providers or api_keys")
		}
	}
	dryRun := param.DryRun != nil && *param.DryRun

	operator := ""
	if v, err := iauth.MustGetVisitor(req.Context()); err == nil && v != nil {
		operator = v.GetName()
	}

	task, err := container.KeyRotateManager.Trigger(req.Context(), mode, dryRun, scope, operator)
	if err != nil {
		var conflict *keyrotate.ErrConflict
		if errors.As(err, &conflict) {
			return nil, xerror.WrapConflictErrorWithMsg("another reencrypt sweep is running: %s", conflict.HolderTaskID)
		}
		return nil, err
	}
	return &triggerResponse{
		TaskID:      task.TaskID,
		Status:      string(task.Status),
		Mode:        string(task.Mode),
		DryRun:      task.DryRun,
		Scope:       string(task.Scope),
		ActiveKeyID: task.ActiveKeyID,
	}, nil
}

func GetAction(req *http.Request) (interface{}, error) {
	taskID := mux.Vars(req)["task_id"]
	if taskID == "" {
		return nil, xerror.WrapParamErrorWithMsg("task_id is required")
	}
	task, err := container.KeyRotateManager.GetTask(req.Context(), taskID)
	if err != nil {
		if xerror.Cause(err) == keyrotate.ErrTaskNotFound {
			return nil, xerror.WrapRecordNotExist("sweep task", taskID)
		}
		return nil, err
	}

	return taskToResponse(task), nil
}
