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

package openapi

import "github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"

// EppClusterAssignmentSchema /epp-assignments §2.1 全量视图 clusters[] 元素 schema。
// group/primary 在未分配时为 null，故设为 Optional
// （testutil schema 校验中 Optional 字段为 null 时跳过类型校验）。
// standby 字段已从 OpenAPI 契约删除（2026-10-14-epp-openapi-single-instance）。
var EppClusterAssignmentSchema = &testutil.ObjectSchema{
	Required: []string{"cluster", "degraded"},
	Optional: []string{"group", "primary"},
	Fields: map[string]testutil.FieldSpec{
		"cluster":  {Type: testutil.TypeString},
		"group":    {Type: testutil.TypeString},
		"primary":  {Type: testutil.TypeObject, Nested: EppInstanceSchema},
		"degraded": {Type: testutil.TypeBool},
	},
}

// EppClusterAssignmentOverrideSchema /epp-assignments §2.2 PUT 覆写响应 schema。
// 实现（ClusterAssignmentOverrideData）按契约注释不含 degraded 标记，故此处不要求。
// standby 字段已从 OpenAPI 契约删除（2026-10-14-epp-openapi-single-instance）。
var EppClusterAssignmentOverrideSchema = &testutil.ObjectSchema{
	Required: []string{"cluster"},
	Optional: []string{"group", "primary"},
	Fields: map[string]testutil.FieldSpec{
		"cluster": {Type: testutil.TypeString},
		"group":   {Type: testutil.TypeString},
		"primary": {Type: testutil.TypeObject, Nested: EppInstanceSchema},
	},
}

// EppAssignmentsViewSchema /epp-assignments 全量视图 schema
// （GET 全量视图与 GET ?cluster= 过滤响应同结构）。
var EppAssignmentsViewSchema = &testutil.ObjectSchema{
	Required: []string{"clusters", "unassigned_clusters", "idle_groups"},
	Fields: map[string]testutil.FieldSpec{
		"clusters":            {Type: testutil.TypeArray, Elem: EppClusterAssignmentSchema},
		"unassigned_clusters": {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"idle_groups":         {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
	},
}
