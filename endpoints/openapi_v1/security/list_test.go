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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/keyrotate"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

type fakeSweepStorager struct {
	lastFilter *keyrotate.TaskFilter
	tasks      []*keyrotate.SweepTask
	total      int64
}

func (f *fakeSweepStorager) AcquireTask(ctx context.Context, task *keyrotate.SweepTask) error {
	return nil
}
func (f *fakeSweepStorager) GetTask(ctx context.Context, taskID string) (*keyrotate.SweepTask, error) {
	return nil, nil
}
func (f *fakeSweepStorager) ListTasks(ctx context.Context, filter *keyrotate.TaskFilter) ([]*keyrotate.SweepTask, int64, error) {
	f.lastFilter = filter
	return f.tasks, f.total, nil
}
func (f *fakeSweepStorager) AddProgress(ctx context.Context, dbCtx lib.DBContexter, taskID string, scanned, rewritten int64) error {
	return nil
}
func (f *fakeSweepStorager) FinishTask(ctx context.Context, taskID string, status keyrotate.Status, cause string,
	summary map[string]*keyrotate.TableCount, duration time.Duration) error {
	return nil
}
func (f *fakeSweepStorager) ListRows(ctx context.Context, t keyrotate.SweepTable, cursor interface{}, limit int) ([]keyrotate.RowRef, error) {
	return nil, nil
}
func (f *fakeSweepStorager) RewriteRow(ctx context.Context, dbCtx lib.DBContexter, t keyrotate.SweepTable, pk interface{}, value string) error {
	return nil
}

func TestListAction(t *testing.T) {
	started := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	st := &fakeSweepStorager{
		tasks: []*keyrotate.SweepTask{
			{
				TaskID:      "rsp-01J8XQ",
				Status:      keyrotate.StatusSucceeded,
				Mode:        keyrotate.ModeReencrypt,
				Scope:       keyrotate.ScopeAll,
				ActiveKeyID: 2,
				CreatedBy:   "admin",
				StartedAt:   started,
				FinishedAt:  &started,
				DurationMs:  8300,
				Summary: map[string]*keyrotate.TableCount{
					"providers": {Scanned: 120, Rewritten: 118, Skipped: 2},
				},
				FailedCause: "",
			},
		},
		total: 1,
	}
	container.KeyRotateManager = keyrotate.NewManager(st, nil)

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/security/reencrypt-sweeps?status=succeeded&mode=reencrypt&scope=all&dry_run=false&start_time=2026-10-01T00:00:00Z&end_time=2026-10-31T23:59:59Z&page=2&page_size=10&sort_by=task_id&sort_order=asc", nil)
	resp, err := ListAction(req)
	require.NoError(t, err)

	listResp, ok := resp.(*listResponse)
	require.True(t, ok)
	assert.Equal(t, 2, listResp.Pagination.Page)
	assert.Equal(t, 10, listResp.Pagination.PageSize)
	assert.Equal(t, int64(1), listResp.Pagination.Total)
	require.Len(t, listResp.List, 1)
	assert.Equal(t, "rsp-01J8XQ", listResp.List[0].TaskID)
	assert.Equal(t, "succeeded", listResp.List[0].Status)
	assert.Equal(t, "admin", listResp.List[0].CreatedBy)
	require.NotNil(t, listResp.List[0].Summary["providers"])
	assert.Equal(t, int64(120), listResp.List[0].Summary["providers"].Scanned)
	assert.Equal(t, int64(118), listResp.List[0].Summary["providers"].Rewritten)
	assert.Equal(t, int64(2), listResp.List[0].Summary["providers"].Skipped)

	require.NotNil(t, st.lastFilter)
	require.NotNil(t, st.lastFilter.Status)
	assert.Equal(t, keyrotate.StatusSucceeded, *st.lastFilter.Status)
	require.NotNil(t, st.lastFilter.Mode)
	assert.Equal(t, keyrotate.ModeReencrypt, *st.lastFilter.Mode)
	require.NotNil(t, st.lastFilter.Scope)
	assert.Equal(t, keyrotate.ScopeAll, *st.lastFilter.Scope)
	require.NotNil(t, st.lastFilter.DryRun)
	assert.False(t, *st.lastFilter.DryRun)
	require.NotNil(t, st.lastFilter.StartTime)
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), *st.lastFilter.StartTime)
	require.NotNil(t, st.lastFilter.EndTime)
	assert.Equal(t, time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC), *st.lastFilter.EndTime)
	assert.Equal(t, 2, st.lastFilter.Page)
	assert.Equal(t, 10, st.lastFilter.PageSize)
	assert.Equal(t, "task_id", st.lastFilter.SortBy)
	assert.Equal(t, "asc", st.lastFilter.SortOrder)
}

func TestListAction_Defaults(t *testing.T) {
	st := &fakeSweepStorager{}
	container.KeyRotateManager = keyrotate.NewManager(st, nil)

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/security/reencrypt-sweeps", nil)
	resp, err := ListAction(req)
	require.NoError(t, err)

	listResp, ok := resp.(*listResponse)
	require.True(t, ok)
	assert.Equal(t, 1, listResp.Pagination.Page)
	assert.Equal(t, 20, listResp.Pagination.PageSize)
	assert.Equal(t, int64(0), listResp.Pagination.Total)
	require.NotNil(t, st.lastFilter)
	assert.Nil(t, st.lastFilter.Status)
	assert.Nil(t, st.lastFilter.Mode)
	assert.Nil(t, st.lastFilter.Scope)
	assert.Nil(t, st.lastFilter.DryRun)
	assert.Nil(t, st.lastFilter.StartTime)
	assert.Nil(t, st.lastFilter.EndTime)
	assert.Equal(t, "started_at", st.lastFilter.SortBy)
	assert.Equal(t, "desc", st.lastFilter.SortOrder)
}

func TestListAction_NormalizeAndFallback(t *testing.T) {
	st := &fakeSweepStorager{}
	container.KeyRotateManager = keyrotate.NewManager(st, nil)

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/security/reencrypt-sweeps?page=0&page_size=500&sort_by=evil&sort_order=DESC", nil)
	_, err := ListAction(req)
	require.NoError(t, err)

	require.NotNil(t, st.lastFilter)
	assert.Equal(t, 1, st.lastFilter.Page)
	assert.Equal(t, 100, st.lastFilter.PageSize)
	assert.Equal(t, "started_at", st.lastFilter.SortBy)
	assert.Equal(t, "desc", st.lastFilter.SortOrder)
}

func TestListAction_InvalidParams(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		errMsg string
	}{
		{"bad status", "status=done", "status must be running, succeeded or failed"},
		{"bad mode", "mode=rotate", "mode must be reencrypt or decrypt"},
		{"bad scope", "scope=everything", "scope must be all, providers or api_keys"},
		{"bad start_time", "start_time=2026-10-01", "start_time must be RFC3339"},
		{"bad end_time", "end_time=not-a-time", "end_time must be RFC3339"},
		{"end before start", "start_time=2026-10-02T00:00:00Z&end_time=2026-10-01T00:00:00Z", "end_time must not be earlier than start_time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeSweepStorager{}
			container.KeyRotateManager = keyrotate.NewManager(st, nil)

			req := httptest.NewRequest(http.MethodGet, "/open-api/v1/security/reencrypt-sweeps?"+tc.query, nil)
			_, err := ListAction(req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
			assert.Nil(t, st.lastFilter)
		})
	}
}
