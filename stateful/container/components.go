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

package container

import (
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ai_cache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ai_context"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/epp_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iai_route"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iintent_config"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ik8s_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/imodel_price"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/imods"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprotocol"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ireport"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iroute_conf"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/keyrotate"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quota"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quotacache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/rate_limit_policy"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/route_rules"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibatch"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/traffic_mirror"
)

var (
	TxnStoragerSingleton            itxn.TxnStorager
	VersionControlStoragerSingleton iversion_control.VersionControlStorager
	RouteRuleStoragerSingleton      iroute_conf.RouteRuleStorager
	ProductStoragerSingleton        ibasic.ProductStorager
	BFEClusterStoragerSingleton     ibasic.BFEClusterStorager
	DomainStoragerSingleton         iroute_conf.DomainStorager
	ClusterStoragerSingleton        icluster_conf.ClusterStorager
	APIKeyStorager                  api_key.APIKeyStorager
	KeyRotateStorager               keyrotate.Storager
	APIKeyIDGenerator               api_key.APIKeyIDGenerator
	PoolStoragerSingleton           icluster_conf.PoolStorager
	SubClusterStoragerSingleton     icluster_conf.SubClusterStorager
	CertificateStoragerSingleton    iprotocol.CertificateStorager
	AuthenticateStoragerSingleton   iauth.AuthenticateStorager
	AuthorizeStoragerSingleton      iauth.AuthorizeStorager
	ExtraFileStoragerSingleton      ibasic.ExtraFileStorager
	AIRouteRuleStorager             iai_route.AIRouteRuleStorager
	ExtraFileManager                *ibasic.ExtraFileManager
	ProductManager                  *ibasic.ProductManager
	DomainManager                   *iroute_conf.DomainManager
	BFEClusterManager               *ibasic.BFEClusterManager
	VersionControlManager           *iversion_control.VersionControlManager
	RouteRuleManager                *iroute_conf.RouteRuleManager
	ClusterManager                  *icluster_conf.ClusterManager
	SubClusterManager               *icluster_conf.SubClusterManager
	CertificateManager              *iprotocol.CertificateManager
	AuthenticateManager             *iauth.AuthenticateManager
	AuthorizeManager                *iauth.AuthorizeManager
	PoolManager                     *icluster_conf.PoolManager
	APIKeyRuleManager               *imods.APIKeyRuleManager
	ModBodyProcessManager           *imods.ModBodyProcessManager
	APIKeyManager                   *api_key.APIKeyManager
	AIRouteRuleManager              *iai_route.AIRouteRuleManager

	// Quota management
	EntityTypeStorager              entity.EntityTypeStorager
	EntityStorager                  entity.EntityStorager
	EntityIDGenerator               entity.EntityIDGenerator
	QuotaPlanStorager               quota.QuotaPlanStorager
	RateLimitPolicyStorager         rate_limit_policy.RateLimitPolicyStorager
	AICacheStorager                 ai_cache.AICacheStorager
	AICacheSemanticSettingsStorager ai_cache.AICacheSemanticSettingsStorager
	AIContextStorager               ai_context.AIContextStorager
	AIContextSettingsStorager       ai_context.AIContextSettingsStorager
	TrafficMirrorStorager           traffic_mirror.TrafficMirrorStorager
	IntentConfigStorager            iintent_config.IntentConfigStorager
	RouteRulesStorager              shared.RouteRulesStorager
	QuotaCacheSingleton             quotacache.QuotaCache

	// Model pricing
	ModelPriceStorager imodel_price.ModelPriceStorager
	ModelPriceManager  *imodel_price.Manager

	// Providers
	ProviderStoragerSingleton iprovider.ProviderStorager
	ProviderManager           *iprovider.ProviderManager

	// K8s instance pools (k8s_pools is the single source of truth for
	// k8s-sourced instances; consumers resolve them at read time through
	// InstancePoolResolver, k8s-pools.md)
	K8sPoolStorager      ik8s_pool.K8sPoolStorager
	K8sPoolManager       *ik8s_pool.K8sPoolManager
	InstancePoolResolver *iprovider.InstancePoolResolver

	EntityTypeManager      *entity.EntityTypeManager
	EntityManager          *entity.EntityManager
	QuotaPlanManager       *quota.QuotaPlanManager
	RateLimitPolicyManager *rate_limit_policy.RateLimitPolicyManager
	AICacheManager         *ai_cache.AICacheManager
	AIContextManager       *ai_context.AIContextManager
	TrafficMirrorManager   *traffic_mirror.TrafficMirrorManager
	IntentConfigManager    *iintent_config.IntentConfigManager
	RouteRulesManager      *route_rules.RouteRulesManager
	AIRouteExporter        *imods.AIRouteExporter
	BalanceSyncManager     *quota.BalanceSyncManager
	QuotaResetScheduler    *quota.QuotaResetScheduler

	// Operation logs
	OperationLogStorager ioperlog.OperationLogStorager
	OperationLogManager  ioperlog.OperationLogManagerInterface
	KeyRotateManager     *keyrotate.Manager

	// Batch & async tasks (批量任务与对账.md：管控 API + 对账 job)。
	// BatchManager 为 endpoints 面向接口；BatchJob 仅 [BatchJob].Enable
	// 时装配，否则 nil。
	BatchManager ibatch.BatchManager
	BatchJob     *ibatch.Job

	// Report query module (see design-docs modifications/2026-09-15-report-query-api);
	// nil when [Report].Backend is not configured.
	ReportManager ireport.ReportManagerInterface

	// EPP scheduling integration (see model/epp_pool and design-docs
	// modifications/2026-09-08-epp-scheduling-integration).
	EppPoolManager *epp_pool.EppPoolManager
)
