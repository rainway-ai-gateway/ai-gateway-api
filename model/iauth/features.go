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

package iauth

type (
	Feature string
	Action  int64
)

type FeatureAuthorition struct {
	Feature Feature
	Action  Action
}

type Authorization struct {
	FeatureAuthorizer *FeatureAuthorition
	ValidateProduct   bool
}

func NewFeatureAuthorization(f Feature, a Action) *Authorization {
	return &Authorization{
		FeatureAuthorizer: &FeatureAuthorition{
			Feature: f,
			Action:  a,
		},
	}
}

func NewFeatureAuthorizerWithFactoryWithProduct(f Feature, a Action) *Authorization {
	tmp := NewFeatureAuthorization(f, a)
	tmp.ValidateProduct = true

	return tmp
}

var (
	FA  = NewFeatureAuthorization
	FAP = NewFeatureAuthorizerWithFactoryWithProduct
)

const (
	ActionDeny    Action = 1 << iota // 1 << 0 = 1
	ActionRead                       // 1 << 1 = 2
	ActionReadAll                    // 1 << 2 = 4 (NOT equal to ActionRead)
	ActionUpdate                     // 1 << 3 = 8
	ActionCreate                     // 1 << 4 = 16
	ActionDelete                     // 1 << 5 = 32
	ActionExport                     // 1 << 6 = 64
	ActionCancel                     // 1 << 7 = 128
)

func (a Action) Revoke(b Action) Action {
	return a & (^b)
}

func (a Action) Grant(b Action) Action {
	return a | b
}

func (a Action) IsAllowed(b Action) bool {
	return a&b != 0
}

const (
	// global resource
	FeatureProxyPool  Feature = "ProxyPool"
	FeatureBFECluster Feature = "BFECluster"
	FeatureBFEPool    Feature = "BFEPool"
	FeatureArea       Feature = "Area"
	FeatureDomain     Feature = "Domain"
	FeatureProduct    Feature = "Product"
	FeatureExtraFile  Feature = "ExtraFile"

	// product resource
	FeatureProductPool       Feature = "ProductPool"
	FeatureRoute             Feature = "Route"
	FeatureSubCluster        Feature = "SubCluster"
	FeatureProductCluster    Feature = "ProductCluster"
	FeatureGeneral           Feature = "General"
	FeatureTraffic           Feature = "Traffic"
	FeatureCert              Feature = "Cert"
	FeatureActiveHealthCheck Feature = "ActiveHealthCheck"

	// auth
	FeatureProductUser Feature = "AuthProductUser"
	FeatureUser        Feature = "User"
	FeatureToken       Feature = "Token"

	// nlb resource
	FeatureNLBPool    Feature = "NLBPool"
	FeatureNLBCluster Feature = "NLBCluster"
	FeatureAIRoute    Feature = "AIRoute"
	FeatureAPIKey     Feature = "APIKey"

	// quota management
	FeatureEntityType      Feature = "EntityType"
	FeatureEntity          Feature = "Entity"
	FeatureQuotaPlan       Feature = "QuotaPlan"
	FeatureRateLimitPolicy Feature = "RateLimitPolicy"

	// ai cache
	FeatureAICache Feature = "AICache"

	// ai context compress
	FeatureAIContext Feature = "AIContext"

	// traffic mirror
	FeatureTrafficMirror Feature = "TrafficMirror"

	// ai intent
	FeatureAIIntent Feature = "AIIntent"

	// access log (mod_access_pb.data 白名单管理)
	FeatureAccessLog Feature = "AccessLog"

	// model pricing
	FeatureModelPrice Feature = "ModelPrice"
	FeatureProvider   Feature = "Provider"

	// K8s instance pools (discovery-component write channel, k8s-pools.md)
	FeatureK8sPool Feature = "K8sPool"

	// operation logs
	FeatureOperationLog Feature = "OperationLog"
	FeatureSecurity     Feature = "Security"

	// report query
	FeatureReport Feature = "Report"

	// batch & async tasks (批量任务与对账.md：读 + 取消两类动作)
	FeatureBatch Feature = "Batch"
)

var (
	actionProductNormal = ActionDeny.
				Grant(ActionRead).
				Grant(ActionUpdate).
				Grant(ActionCreate).
				Grant(ActionReadAll).
				Grant(ActionDelete)

	actionAll = ActionDeny.
			Grant(ActionRead).
			Grant(ActionUpdate).
			Grant(ActionCreate).
			Grant(ActionDelete).
			Grant(ActionReadAll).
			Grant(ActionExport)

	// actionReadonly grants both read actions: ActionReadAll is a distinct
	// bit (4) from ActionRead (2), and read endpoints declare either of them.
	actionReadonly = ActionRead.Grant(ActionReadAll)
)

var scope2permission = map[string]map[Feature]Action{
	ScopeSystem: {
		FeatureProxyPool:  actionAll,
		FeatureBFECluster: actionAll,
		FeatureBFEPool:    actionAll,
		FeatureArea:       actionAll,
		FeatureDomain:     actionAll,
		FeatureProduct:    actionAll,
		FeatureExtraFile:  actionAll,

		FeatureProductPool:       actionAll,
		FeatureRoute:             actionAll,
		FeatureSubCluster:        actionAll,
		FeatureProductCluster:    actionAll,
		FeatureTraffic:           actionAll,
		FeatureGeneral:           actionAll,
		FeatureCert:              actionAll,
		FeatureActiveHealthCheck: actionAll,

		FeatureProductUser: actionAll,
		FeatureUser:        actionAll,
		FeatureToken:       actionAll,

		FeatureNLBPool:    actionAll,
		FeatureNLBCluster: actionAll,
		FeatureAIRoute:    actionAll,
		FeatureAPIKey:     actionAll,

		// Quota / AI cache / context / mirror / intent features. These were
		// missing from this map and only worked because admins short-circuit
		// IsAdmin() in Authorizate; listed explicitly so the map stays the
		// full inventory for non-admin scopes (e.g. ScopeSystemReadonly).
		FeatureEntityType:      actionAll,
		FeatureEntity:          actionAll,
		FeatureQuotaPlan:       actionAll,
		FeatureRateLimitPolicy: actionAll,
		FeatureAICache:         actionAll,
		FeatureAIContext:       actionAll,
		FeatureTrafficMirror:   actionAll,
		FeatureAIIntent:        actionAll,
		FeatureAccessLog:       actionAll,

		FeatureModelPrice:   actionAll,
		FeatureProvider:     actionAll,
		FeatureK8sPool:      actionAll,
		FeatureOperationLog: actionAll,

		FeatureReport: actionAll,

		FeatureSecurity: actionAll,

		FeatureBatch: actionAll,
	},
	// ScopeSystemReadonly: the "system admin (read-only)" role. Every Feature
	// must be listed explicitly (unlisted Features default to deny), each
	// granted ActionRead only — no Update/Create/Delete/Export/Cancel.
	ScopeSystemReadonly: {
		FeatureProxyPool:  actionReadonly,
		FeatureBFECluster: actionReadonly,
		FeatureBFEPool:    actionReadonly,
		FeatureArea:       actionReadonly,
		FeatureDomain:     actionReadonly,
		FeatureProduct:    actionReadonly,
		FeatureExtraFile:  actionReadonly,

		FeatureProductPool:       actionReadonly,
		FeatureRoute:             actionReadonly,
		FeatureSubCluster:        actionReadonly,
		FeatureProductCluster:    actionReadonly,
		FeatureTraffic:           actionReadonly,
		FeatureGeneral:           actionReadonly,
		FeatureCert:              actionReadonly,
		FeatureActiveHealthCheck: actionReadonly,

		FeatureProductUser: actionReadonly,
		FeatureUser:        actionReadonly,
		FeatureToken:       actionReadonly,

		FeatureNLBPool:    actionReadonly,
		FeatureNLBCluster: actionReadonly,
		FeatureAIRoute:    actionReadonly,
		FeatureAPIKey:     actionReadonly,

		FeatureEntityType:      actionReadonly,
		FeatureEntity:          actionReadonly,
		FeatureQuotaPlan:       actionReadonly,
		FeatureRateLimitPolicy: actionReadonly,
		FeatureAICache:         actionReadonly,
		FeatureAIContext:       actionReadonly,
		FeatureTrafficMirror:   actionReadonly,
		FeatureAIIntent:        actionReadonly,
		FeatureAccessLog:       actionReadonly,

		FeatureModelPrice: actionReadonly,
		FeatureProvider:   actionReadonly,

		FeatureK8sPool:      actionReadonly,
		FeatureOperationLog: actionReadonly,

		FeatureReport: actionReadonly,

		FeatureSecurity: actionReadonly,

		FeatureBatch: actionReadonly,
	},
	ScopeProduct: {
		FeatureUser:       ActionReadAll,
		FeatureToken:      ActionReadAll,
		FeatureProxyPool:  actionProductNormal.Grant(ActionReadAll),
		FeatureBFECluster: actionProductNormal,
		FeatureBFEPool:    actionProductNormal,
		FeatureArea:       actionProductNormal.Grant(ActionReadAll),
		FeatureDomain:     actionProductNormal.Grant(ActionReadAll),
		FeatureProduct:    actionProductNormal.Grant(ActionReadAll),

		FeatureProductPool:       actionProductNormal,
		FeatureRoute:             actionProductNormal,
		FeatureSubCluster:        actionProductNormal,
		FeatureProductCluster:    actionProductNormal,
		FeatureTraffic:           actionProductNormal,
		FeatureGeneral:           actionProductNormal,
		FeatureCert:              actionProductNormal,
		FeatureActiveHealthCheck: actionProductNormal,

		FeatureProductUser: actionProductNormal,

		FeatureNLBPool:    actionProductNormal,
		FeatureNLBCluster: actionProductNormal,
		FeatureAIRoute:    actionProductNormal,
		FeatureAPIKey:     actionProductNormal,

		// Report stays read-only for the product scope; it is reserved for
		// the tenant self-service usage report.
		FeatureReport: ActionRead,

		// Batch tasks of the product line: read + cancel (api-changes.md
		// §4.5：FeatureBatch actionRead/actionCancel 挂既有角色体系)。
		FeatureBatch: ActionRead.Grant(ActionCancel),
	},
	ScopeSupport: {
		FeatureProxyPool:         ActionExport,
		FeatureRoute:             ActionExport,
		FeatureCert:              ActionExport,
		FeatureActiveHealthCheck: ActionExport,
		FeatureExtraFile:         ActionExport,
		FeatureAPIKey:            ActionExport,
		FeatureRateLimitPolicy:   ActionExport,
		FeatureAICache:           ActionExport,
		FeatureAIContext:         ActionExport,
		FeatureTrafficMirror:     ActionExport,
		FeatureAIIntent:          ActionExport,
	},
}
