// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openapi

import "github.com/rainway-ai-gateway/ai-gateway-api/integration/testutil"

// K8sPoolInstanceSchema is one instance element of a K8s pool detail.
var K8sPoolInstanceSchema = &testutil.ObjectSchema{
	Required: []string{"addr", "port", "weight", "disable"},
	Fields: map[string]testutil.FieldSpec{
		"addr":    {Type: testutil.TypeString},
		"port":    {Type: testutil.TypeInt},
		"weight":  {Type: testutil.TypeInt},
		"disable": {Type: testutil.TypeBool},
	},
}

// K8sPoolListEntrySchema is one summary element of GET /k8s_pools.
var K8sPoolListEntrySchema = &testutil.ObjectSchema{
	Required: []string{"name", "instance_count", "last_sync_time"},
	Fields: map[string]testutil.FieldSpec{
		"name":           {Type: testutil.TypeString},
		"instance_count": {Type: testutil.TypeInt},
		"last_sync_time": {Type: testutil.TypeInt},
	},
}

// K8sPoolListSchema is the GET /k8s_pools Data shape.
var K8sPoolListSchema = &testutil.ObjectSchema{
	Required: []string{"list"},
	Fields: map[string]testutil.FieldSpec{
		"list": {Type: testutil.TypeArray, Elem: K8sPoolListEntrySchema},
	},
}

// K8sPoolEntrySchema is the GET /k8s_pools/{name} Data shape.
var K8sPoolEntrySchema = &testutil.ObjectSchema{
	Required: []string{"name", "instances", "instance_count", "last_sync_time"},
	Fields: map[string]testutil.FieldSpec{
		"name":           {Type: testutil.TypeString},
		"instances":      {Type: testutil.TypeArray, Elem: K8sPoolInstanceSchema},
		"instance_count": {Type: testutil.TypeInt},
		"last_sync_time": {Type: testutil.TypeInt},
	},
}
