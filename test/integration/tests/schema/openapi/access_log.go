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

// AccessLogConfigSchema 访问日志配置单例 schema（GET/PUT 响应同构，
// api-define/access-log.md §1）。Required 恰为合同 2 字段；不得包含内部
// version/id（单行覆盖式存储，版本由下发链路数据签名承担，OpenAPI 不暴露）。
var AccessLogConfigSchema = &testutil.ObjectSchema{
	Required: []string{"req_headers", "res_headers"},
	Fields: map[string]testutil.FieldSpec{
		"req_headers": {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
		"res_headers": {Type: testutil.TypeArray, Item: &testutil.FieldSpec{Type: testutil.TypeString}},
	},
}
