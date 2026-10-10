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

package openapi

import "github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"

// ---------- security/reencrypt-sweeps（密钥收敛任务，security.md） ----------

// SweepTriggerSchema 触发收敛任务（POST）响应 schema。
// 触发响应不回填 created_by/时间/summary（任务对象见 SweepTaskSchema）。
var SweepTriggerSchema = &testutil.ObjectSchema{
	Required: []string{"task_id", "status", "mode", "dry_run", "scope", "active_key_id"},
	Fields: map[string]testutil.FieldSpec{
		"task_id":       {Type: testutil.TypeString},
		"status":        {Type: testutil.TypeString, Enum: []interface{}{"running"}},
		"mode":          {Type: testutil.TypeString, Enum: []interface{}{"reencrypt", "decrypt"}},
		"dry_run":       {Type: testutil.TypeBool},
		"scope":         {Type: testutil.TypeString, Enum: []interface{}{"all", "providers", "api_keys"}},
		"active_key_id": {Type: testutil.TypeInt},
	},
}

// SweepTableCountSchema summary 中按表分组计数 schema。
var SweepTableCountSchema = &testutil.ObjectSchema{
	Required: []string{"scanned", "rewritten", "skipped"},
	Fields: map[string]testutil.FieldSpec{
		"scanned":   {Type: testutil.TypeInt},
		"rewritten": {Type: testutil.TypeInt},
		"skipped":   {Type: testutil.TypeInt},
	},
}

// SweepTaskSummarySchema 任务 summary schema（键为扫描表名；scope=providers
// 时恰含 providers 组，scope=all 时允许额外 api_keys 组）。
var SweepTaskSummarySchema = &testutil.ObjectSchema{
	Required: []string{"providers"},
	Fields: map[string]testutil.FieldSpec{
		"providers": {Type: testutil.TypeObject, Nested: SweepTableCountSchema},
		"api_keys":  {Type: testutil.TypeObject, Nested: SweepTableCountSchema},
	},
}

// SweepTaskSchema 任务对象 schema（GET by task_id 与 GET 列表元素共用）。
// finished_at 为 omitempty：running 时缺席、终态存在。dry-run 终态任务
// error 固定为 "dry_run"（定向断言见测试）。
var SweepTaskSchema = &testutil.ObjectSchema{
	Required: []string{"task_id", "status", "mode", "dry_run", "scope", "active_key_id", "created_by", "started_at", "duration_ms", "summary", "error"},
	Optional: []string{"finished_at"},
	Fields: map[string]testutil.FieldSpec{
		"task_id":       {Type: testutil.TypeString},
		"status":        {Type: testutil.TypeString, Enum: []interface{}{"running", "succeeded", "failed"}},
		"mode":          {Type: testutil.TypeString, Enum: []interface{}{"reencrypt", "decrypt"}},
		"dry_run":       {Type: testutil.TypeBool},
		"scope":         {Type: testutil.TypeString, Enum: []interface{}{"all", "providers", "api_keys"}},
		"active_key_id": {Type: testutil.TypeInt},
		"created_by":    {Type: testutil.TypeString},
		"started_at":    {Type: testutil.TypeString},
		"finished_at":   {Type: testutil.TypeString},
		"duration_ms":   {Type: testutil.TypeInt},
		"summary":       {Type: testutil.TypeObject, Nested: SweepTaskSummarySchema},
		"error":         {Type: testutil.TypeString},
	},
}
