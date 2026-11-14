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

// Package k8s_pools exposes the OpenAPI read-only view of K8s instance pools
// (design-docs/api-define/OpenAPI接口定义/k8s-pools.md). Pool membership is
// written exclusively by the K8s discovery component through the InnerAPI;
// this package only exposes list/detail reads for the console.
package k8s_pools

import "github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"

// Endpoints exposes the read-only K8s pool endpoints.
var Endpoints = []*xreq.Endpoint{
	ListEndpoint,
	GetEndpoint,
}
