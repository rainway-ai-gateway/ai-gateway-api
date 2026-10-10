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

// GET /open-api/v1/security/reencrypt-sweeps 列表端点集成测试
// （场景登记见 design.md，模块前缀 SAR-3-xxx，2026-10-10 新增：
// modifications/2026-10-10-reencrypt-sweeps-list-endpoint）。
// 与 SAR-2-xxx 共用同包 helper（startSARServer/triggerSweep/waitSweepSucceeded/doRaw）。
package secret_at_rest_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sweepTaskItem 为列表/详情返回的任务对象（比 sweepTaskData 多时间/created_by 字段）。
type sweepTaskItem struct {
	TaskID      string                 `json:"task_id"`
	Status      string                 `json:"status"`
	Mode        string                 `json:"mode"`
	DryRun      bool                   `json:"dry_run"`
	Scope       string                 `json:"scope"`
	ActiveKeyID int                    `json:"active_key_id"`
	CreatedBy   string                 `json:"created_by"`
	StartedAt   string                 `json:"started_at"`
	FinishedAt  string                 `json:"finished_at"`
	DurationMs  int64                  `json:"duration_ms"`
	Summary     map[string]*tableCount `json:"summary"`
	Error       string                 `json:"error"`
}

type sweepsListData struct {
	List []*sweepTaskItem `json:"list"`
	Pagination struct {
		Page     int   `json:"page"`
		PageSize int   `json:"page_size"`
		Total    int64 `json:"total"`
	} `json:"pagination"`
}

// listSweeps 请求列表端点并解析 Data（非 200 时 data 为零值，调用方断言状态）。
func listSweeps(t *testing.T, baseURL string, query url.Values) (rawResp, sweepsListData) {
	t.Helper()

	u := baseURL + sweepPath
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	r := doRaw(t, http.MethodGet, u, nil)
	var data sweepsListData
	if r.Status == http.StatusOK {
		require.NoError(t, json.Unmarshal(r.Raw, &struct {
			Data *sweepsListData `json:"Data"`
		}{Data: &data}))
	}
	return r, data
}

// triggerSucceededSweep 触发任务并等待终态 succeeded，返回 task_id。
func triggerSucceededSweep(t *testing.T, baseURL string, body map[string]interface{}) string {
	t.Helper()

	r, trig := triggerSweep(t, baseURL, body)
	require.Equal(t, http.StatusOK, r.Status, "raw=%s", r.Raw)
	require.Equal(t, 200, r.ErrNum, "raw=%s", r.Raw)
	require.NotEmpty(t, trig.TaskID)
	waitSweepSucceeded(t, baseURL, trig.TaskID, 30*time.Second)
	return trig.TaskID
}

// taskIDSet 提取列表元素 task_id 集合，便于与触发集合做无序比对
// （started_at 精度内同秒任务排序不稳定，顺序断言一律用弱断言）。
func taskIDSet(items []*sweepTaskItem) map[string]bool {
	out := map[string]bool{}
	for _, it := range items {
		out[it.TaskID] = true
	}
	return out
}

// SAR-3-001 列表过滤与字段：触发 dry-run + 正式两个任务（顺序完成），
// 无参列表断言 total/字段完整性/created_by 键存在/dry-run 任务 error="dry_run"，
// 再逐组断言 status/dry_run/mode 过滤与时间区间闭区间过滤。
func TestSAR3_001_ListFiltersAndFields(t *testing.T) {
	krPath := filepath.Join(t.TempDir(), "master.keys")
	writeKeyringFile(t, krPath, 1, map[int][]byte{1: newMasterKey(t)})
	sm := startSARServer(t, securitySectionTOML(krPath))

	dryTaskID := triggerSucceededSweep(t, sm.ServerURL, map[string]interface{}{"dry_run": true, "scope": "providers"})
	realTaskID := triggerSucceededSweep(t, sm.ServerURL, map[string]interface{}{"dry_run": false, "scope": "providers"})

	// 无参列表：total=2，两个任务均出现；元素字段完整
	r, data := listSweeps(t, sm.ServerURL, nil)
	require.Equal(t, http.StatusOK, r.Status, "raw=%s", r.Raw)
	require.Equal(t, 200, r.ErrNum, "raw=%s", r.Raw)
	require.Equal(t, int64(2), data.Pagination.Total)
	require.Equal(t, 1, data.Pagination.Page)
	require.Equal(t, 20, data.Pagination.PageSize)
	require.Len(t, data.List, 2)
	require.True(t, taskIDSet(data.List)[dryTaskID], "list must contain dry-run task %s", dryTaskID)
	require.True(t, taskIDSet(data.List)[realTaskID], "list must contain real task %s", realTaskID)
	// created_by 键必须存在（集成环境 visitor 为 SkipUser，值为空串即可）
	require.Contains(t, string(r.Raw), `"created_by"`)
	// 默认 started_at 倒序（弱断言：首元素不早于末元素；同秒 tie 不判序）
	require.GreaterOrEqual(t, data.List[0].StartedAt, data.List[len(data.List)-1].StartedAt)

	// 元素字段：以正式任务为准
	var real *sweepTaskItem
	for _, it := range data.List {
		if it.TaskID == realTaskID {
			real = it
		}
	}
	require.NotNil(t, real)
	assert.Equal(t, "succeeded", real.Status)
	assert.Equal(t, "reencrypt", real.Mode)
	assert.False(t, real.DryRun)
	assert.Equal(t, "providers", real.Scope)
	assert.Equal(t, 1, real.ActiveKeyID)
	assert.NotEmpty(t, real.StartedAt)
	assert.NotEmpty(t, real.FinishedAt)
	assert.GreaterOrEqual(t, real.DurationMs, int64(0))
	require.NotNil(t, real.Summary["providers"], "summary must contain providers group")

	// dry-run 任务：error 固定为 dry_run（契约 §1：非失败归因）
	var dry *sweepTaskItem
	for _, it := range data.List {
		if it.TaskID == dryTaskID {
			dry = it
		}
	}
	require.NotNil(t, dry)
	assert.True(t, dry.DryRun)
	assert.Equal(t, "dry_run", dry.Error)

	// status 过滤
	_, f := listSweeps(t, sm.ServerURL, url.Values{"status": {"succeeded"}})
	require.Equal(t, int64(2), f.Pagination.Total)
	_, f = listSweeps(t, sm.ServerURL, url.Values{"status": {"running"}})
	require.Equal(t, int64(0), f.Pagination.Total)
	require.Len(t, f.List, 0)

	// dry_run 过滤（仅命中 dry-run 任务）
	_, f = listSweeps(t, sm.ServerURL, url.Values{"dry_run": {"true"}})
	require.Equal(t, int64(1), f.Pagination.Total)
	require.Len(t, f.List, 1)
	require.Equal(t, dryTaskID, f.List[0].TaskID)

	// mode 过滤（两个任务均为 reencrypt）
	_, f = listSweeps(t, sm.ServerURL, url.Values{"mode": {"reencrypt"}})
	require.Equal(t, int64(2), f.Pagination.Total)
	_, f = listSweeps(t, sm.ServerURL, url.Values{"mode": {"decrypt"}})
	require.Equal(t, int64(0), f.Pagination.Total)

	// 时间区间过滤（闭区间：start/end 各取一侧）
	now := time.Now()
	_, f = listSweeps(t, sm.ServerURL, url.Values{
		"start_time": {now.Add(-time.Hour).Format(time.RFC3339)},
		"end_time":   {now.Add(time.Hour).Format(time.RFC3339)},
	})
	require.Equal(t, int64(2), f.Pagination.Total)
	_, f = listSweeps(t, sm.ServerURL, url.Values{
		"end_time": {now.Add(-time.Hour).Format(time.RFC3339)},
	})
	require.Equal(t, int64(0), f.Pagination.Total)
	_, f = listSweeps(t, sm.ServerURL, url.Values{
		"start_time": {now.Add(time.Hour).Format(time.RFC3339)},
	})
	require.Equal(t, int64(0), f.Pagination.Total)
}

// SAR-3-002 列表分页与排序：触发 3 个任务，断言分页翻页并集完整、
// page 超界返回空页、page_size 超 100 截断、task_id 升降序、
// 非法 sort_by 回落默认排序。
func TestSAR3_002_ListPaginationAndSort(t *testing.T) {
	krPath := filepath.Join(t.TempDir(), "master.keys")
	writeKeyringFile(t, krPath, 1, map[int][]byte{1: newMasterKey(t)})
	sm := startSARServer(t, securitySectionTOML(krPath))

	want := map[string]bool{}
	for i := 0; i < 3; i++ {
		want[triggerSucceededSweep(t, sm.ServerURL, map[string]interface{}{"dry_run": true, "scope": "providers"})] = true
	}

	// page_size=2 翻两页：并集 == 触发的 3 个任务，total 恒为 3
	p1, d1 := listSweeps(t, sm.ServerURL, url.Values{"page": {"1"}, "page_size": {"2"}})
	require.Equal(t, http.StatusOK, p1.Status, "raw=%s", p1.Raw)
	require.Equal(t, 2, d1.Pagination.PageSize)
	require.Len(t, d1.List, 2)
	p2, d2 := listSweeps(t, sm.ServerURL, url.Values{"page": {"2"}, "page_size": {"2"}})
	require.Equal(t, http.StatusOK, p2.Status, "raw=%s", p2.Raw)
	require.Equal(t, 2, d2.Pagination.Page)
	require.Equal(t, int64(3), d2.Pagination.Total)
	require.Len(t, d2.List, 1)
	union := taskIDSet(d1.List)
	for id := range taskIDSet(d2.List) {
		union[id] = true
	}
	require.Equal(t, want, union)

	// page 超界：200 + 空页 + total 不变
	p9, d9 := listSweeps(t, sm.ServerURL, url.Values{"page": {"99"}, "page_size": {"2"}})
	require.Equal(t, http.StatusOK, p9.Status, "raw=%s", p9.Raw)
	require.Len(t, d9.List, 0)
	require.Equal(t, int64(3), d9.Pagination.Total)

	// page_size 超 100 截断为 100（响应回显截断后的值）
	pc, dc := listSweeps(t, sm.ServerURL, url.Values{"page_size": {"500"}})
	require.Equal(t, http.StatusOK, pc.Status, "raw=%s", pc.Raw)
	require.Equal(t, 100, dc.Pagination.PageSize)
	require.Equal(t, int64(3), dc.Pagination.Total)

	// sort_by=task_id 升序：全列表逐对非降（task_id 全局唯一，实为严格升序）
	pa, da := listSweeps(t, sm.ServerURL, url.Values{"sort_by": {"task_id"}, "sort_order": {"asc"}})
	require.Equal(t, http.StatusOK, pa.Status, "raw=%s", pa.Raw)
	require.Len(t, da.List, 3)
	for i := 0; i+1 < len(da.List); i++ {
		require.Less(t, da.List[i].TaskID, da.List[i+1].TaskID)
	}

	// 非法 sort_by 回落默认（started_at desc）：与无参列表首元素一致
	pf, df := listSweeps(t, sm.ServerURL, url.Values{"sort_by": {"evil"}, "sort_order": {"sideways"}})
	require.Equal(t, http.StatusOK, pf.Status, "raw=%s", pf.Raw)
	_, d0 := listSweeps(t, sm.ServerURL, nil)
	require.Equal(t, d0.List[0].TaskID, df.List[0].TaskID)
}

// SAR-3-003 列表参数校验：非法过滤值/时间格式/end 早于 start 均 422（字段级归因）。
func TestSAR3_003_ListInvalidParams(t *testing.T) {
	krPath := filepath.Join(t.TempDir(), "master.keys")
	writeKeyringFile(t, krPath, 1, map[int][]byte{1: newMasterKey(t)})
	sm := startSARServer(t, securitySectionTOML(krPath))

	triggerSucceededSweep(t, sm.ServerURL, map[string]interface{}{"dry_run": true, "scope": "providers"})

	cases := []struct {
		name  string
		query url.Values
		want  string
	}{
		{"bad status", url.Values{"status": {"done"}}, "status"},
		{"bad mode", url.Values{"mode": {"rot13"}}, "mode"},
		{"bad scope", url.Values{"scope": {"everything"}}, "scope"},
		{"bad start_time", url.Values{"start_time": {"2026-10-10"}}, "start_time"},
		{"bad end_time", url.Values{"end_time": {"not-a-time"}}, "end_time"},
		{"end before start", url.Values{
			"start_time": {"2026-10-02T00:00:00Z"},
			"end_time":   {"2026-10-01T00:00:00Z"},
		}, "end_time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := listSweeps(t, sm.ServerURL, tc.query)
			require.Equal(t, http.StatusUnprocessableEntity, r.Status, "raw=%s", r.Raw)
			require.Equal(t, 422, r.ErrNum, "raw=%s", r.Raw)
			require.Contains(t, r.ErrMsg, tc.want)
		})
	}

	// 参数合法但无匹配数据：200 + 空列表（非 4xx）
	r, data := listSweeps(t, sm.ServerURL, url.Values{"status": {"failed"}})
	require.Equal(t, http.StatusOK, r.Status, "raw=%s", r.Raw)
	require.Equal(t, int64(0), data.Pagination.Total)
	require.Len(t, data.List, 0)
}
