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

package rdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bfenetworks/bfe/bfe_util/bns"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ai_cache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ai_context"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/epp_pool"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iai_route"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iauth"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibatch"
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
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quota"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/quotacache"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/rate_limit_policy"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/route_rules"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/clickhousereport"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/dorisreport"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/mysqlreport"
	aiCacheStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/ai_cache"
	aiContextStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/ai_context"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/ai_route"
	apiKeyStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/auth"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/basic"
	batchStoragePkg "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/batch"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/cluster_conf"
	entityStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/entity"
	eppPoolStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/epp_pool"
	intentConfigStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/iintent_config"
	operationLogStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/ioperlog"
	k8sPoolStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/k8s_pool"
	keyrotateStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/keyrotate"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/model_price"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/protocol"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/provider"
	quotaStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/quota"
	rateLimitPolicyStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/rate_limit_policy"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/route_conf"
	routeRulesStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/route_rules"
	trafficMirrorStorage "github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/traffic_mirror"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/txn"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/version_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/starrocksreport"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/entity"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/keyrotate"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/traffic_mirror"
)

func Init() error {
	container.TxnStoragerSingleton = txn.NewRDBTxnStorager(stateful.NewBFEDBContext)
	container.VersionControlStoragerSingleton = version_control.NewVersionControllerStorage(stateful.NewBFEDBContext)
	container.OperationLogStorager = operationLogStorage.NewOperationLogStorager(
		stateful.NewBFEDBContext, stateful.DefaultClientSet.RedisClient)
	container.KeyRotateStorager = keyrotateStorage.NewStorager(
		stateful.NewBFEDBContext, container.TxnStoragerSingleton, keyrotate.DefaultHeartbeatTTL)
	container.KeyRotateManager = keyrotate.NewManager(container.KeyRotateStorager, stateful.NewBFEDBContext)
	container.OperationLogManager = ioperlog.NewOperationLogManager(container.OperationLogStorager, 0)
	container.OperationLogManager.SetContextExtractor(operationLogContextExtractor)

	// Report query module (see design-docs
	// modifications/2026-09-15-report-query-api). Assembled only when
	// [Report].Backend is configured; otherwise ReportManager stays nil
	// and the /report/* routes are not registered.
	if err := initReport(); err != nil {
		return err
	}

	container.RouteRuleStoragerSingleton = route_conf.NewRouteRuleStorager(
		stateful.NewBFEDBContext,
		container.VersionControlStoragerSingleton)

	container.ProductStoragerSingleton = basic.NewProductManager(stateful.NewBFEDBContext)
	container.BFEClusterStoragerSingleton = basic.NewRDBBFEClusterStorager(stateful.NewBFEDBContext)
	container.PoolStoragerSingleton = cluster_conf.NewRDBPoolStorager(
		stateful.NewBFEDBContext,
		container.ProductStoragerSingleton)
	container.SubClusterStoragerSingleton = cluster_conf.NewRDBSubClusterStorager(
		stateful.NewBFEDBContext,
		container.PoolStoragerSingleton,
		container.ProductStoragerSingleton)
	container.ClusterStoragerSingleton = cluster_conf.NewRDBClusterStorager(
		stateful.NewBFEDBContext,
		container.SubClusterStoragerSingleton)

	container.ProviderStoragerSingleton = provider.NewRDBProviderStorager(stateful.NewBFEDBContext)
	container.ProviderManager = iprovider.NewProviderManager(
		container.TxnStoragerSingleton,
		container.ProviderStoragerSingleton)
	container.ProviderManager.SetOperationLogManager(container.OperationLogManager)

	container.APIKeyStorager = apiKeyStorage.NewAPIKeyStorager(
		stateful.NewBFEDBContext,
	)
	container.APIKeyIDGenerator = apiKeyStorage.NewRDBAPIKeyIDGenerator(
		stateful.NewBFEDBContext,
	)

	container.AIRouteRuleStorager = ai_route.NewRDBAIRouteRuleStorager(
		stateful.NewBFEDBContext,
	)
	container.CertificateStoragerSingleton = protocol.NewCertificateStorager(stateful.NewBFEDBContext)
	container.AuthenticateStoragerSingleton = auth.NewAuthenticateStorager(stateful.NewBFEDBContext)
	container.AuthorizeStoragerSingleton = auth.NewAuthorizeStorager(stateful.NewBFEDBContext,
		container.ProductStoragerSingleton,
		container.AuthenticateStoragerSingleton,
	)

	container.DomainStoragerSingleton = route_conf.NewDomainStorager(stateful.NewBFEDBContext)
	container.ExtraFileStoragerSingleton = basic.NewRDBExtraFileStorager(stateful.NewBFEDBContext)

	container.ExtraFileManager = ibasic.NewExtraFileManager(container.ExtraFileStoragerSingleton)
	container.VersionControlManager = iversion_control.NewVersionControllerManager(
		container.TxnStoragerSingleton,
		container.VersionControlStoragerSingleton)

	container.BFEClusterManager = ibasic.NewBFEClusterManager(
		container.TxnStoragerSingleton,
		container.BFEClusterStoragerSingleton)

	container.CertificateManager = iprotocol.NewCertificateManager(
		container.TxnStoragerSingleton,
		container.CertificateStoragerSingleton,
		container.VersionControlManager,
		container.ExtraFileStoragerSingleton)
	container.CertificateManager.SetOperationLogManager(container.OperationLogManager)

	container.ProductManager = ibasic.NewProductManager(
		container.TxnStoragerSingleton,
		container.ProductStoragerSingleton)

	container.AIRouteRuleManager = iai_route.NewAIRouteRuleManager(
		container.TxnStoragerSingleton,
		container.AIRouteRuleStorager,
		container.VersionControlManager,
		container.RouteRuleStoragerSingleton,
	)
	// Initialize model pricing before route rule manager so InnerAPI exports can
	// attach ModelTable to AIConf.
	container.ModelPriceStorager = model_price.NewRDBModelPriceStorager(stateful.NewBFEDBContext)
	container.ModelPriceManager = imodel_price.NewManager(
		container.TxnStoragerSingleton,
		container.ModelPriceStorager)
	container.ModelPriceManager.SetOperationLogManager(container.OperationLogManager)

	container.RouteRuleManager = iroute_conf.NewRouteRuleManager(
		container.TxnStoragerSingleton,
		container.RouteRuleStoragerSingleton,
		container.ClusterStoragerSingleton,
		container.ProductStoragerSingleton,
		container.VersionControlManager,
		container.DomainStoragerSingleton)
	container.RouteRuleManager.SetModelPriceStorager(container.ModelPriceStorager)
	container.RouteRuleManager.SetProviderStorager(container.ProviderStoragerSingleton)
	container.RouteRuleManager.SetOperationLogManager(container.OperationLogManager)

	// Initialize route rules components before cluster manager because the
	// cluster delete checker depends on RouteRulesManager.
	container.RouteRulesStorager = routeRulesStorage.NewRouteRulesStorager(stateful.NewBFEDBContext)
	container.RouteRulesManager = route_rules.NewRouteRulesManager(
		container.TxnStoragerSingleton,
		container.RouteRulesStorager)
	container.RouteRulesManager.SetOperationLogManager(container.OperationLogManager)

	container.ClusterManager = icluster_conf.NewClusterManager(
		container.TxnStoragerSingleton,
		container.ClusterStoragerSingleton,
		container.SubClusterStoragerSingleton,
		container.BFEClusterStoragerSingleton,
		container.PoolStoragerSingleton,
		container.ProviderStoragerSingleton,
		container.VersionControlManager,
		map[string]func(context.Context, *ibasic.Product, *icluster_conf.Cluster) error{
			"rules":       container.RouteRuleManager.ClusterDeleteChecker,
			"route_rules": container.RouteRulesManager.ClusterDeleteChecker,
		},
		map[string]func(context.Context, *ibasic.Product, *icluster_conf.Cluster, *icluster_conf.ClusterParam) error{
			"route_rules": container.RouteRulesManager.ClusterModelUpdateChecker,
		})
	container.ClusterManager.SetOperationLogManager(container.OperationLogManager)

	// K8s pool manager: maintains the k8s_pools table. k8s_pools is the
	// single source of truth for k8s-sourced instances; the storager also
	// backs the realtime resolution used by the cluster_table exporter and
	// the batch egress (k8s-pools.md).
	container.K8sPoolStorager = k8sPoolStorage.NewRDBK8sPoolStorager(stateful.NewBFEDBContext)
	container.InstancePoolResolver = iprovider.NewInstancePoolResolver(container.K8sPoolStorager)
	// The cluster manager resolves k8s_pool-mode provider instances at export
	// time; the provider manager needs no resolver (its sync hook only
	// compares instance_source / instance_pool).
	container.ClusterManager.SetInstancePoolResolver(container.InstancePoolResolver)
	container.K8sPoolManager = ik8s_pool.NewK8sPoolManager(
		container.TxnStoragerSingleton,
		container.K8sPoolStorager,
		container.ProviderStoragerSingleton)

	// EPP pool manager: the cluster manager provides the EPPClusterSource
	// (balance_mode=EPP clusters and their raw epp_config); ManagerOptions are
	// read from the RunTime config. The reconciler lifecycle follows the
	// process lifecycle like QuotaResetScheduler (design-changes.md §4.3).
	container.EppPoolManager = epp_pool.NewEppPoolManager(
		container.TxnStoragerSingleton,
		eppPoolStorage.NewEppPoolStorager(stateful.NewBFEDBContext),
		container.ClusterManager,
		container.VersionControlManager,
		&epp_pool.ManagerOptions{
			PoolName:          stateful.DefaultConfig.RunTime.DefaultEPPInstancePoolName,
			ReconcileInterval: time.Duration(stateful.DefaultConfig.RunTime.EPPReconcileIntervalSeconds) * time.Second,
		})
	container.ClusterManager.SetEppPoolManager(container.EppPoolManager)
	container.RouteRuleManager.SetEPPAssignmentResolver(container.EppPoolManager)
	container.EppPoolManager.StartReconciler()

	container.SubClusterManager = icluster_conf.NewSubClusterManager(
		container.TxnStoragerSingleton,
		container.SubClusterStoragerSingleton,
		container.ProductStoragerSingleton,
		container.PoolStoragerSingleton,
		container.ClusterStoragerSingleton)

	container.DomainManager = iroute_conf.NewDomainManager(
		container.TxnStoragerSingleton,
		container.DomainStoragerSingleton,
		container.RouteRuleManager)
	container.DomainManager.SetOperationLogManager(container.OperationLogManager)

	container.AuthenticateManager = iauth.NewAuthenticateManager(
		container.TxnStoragerSingleton,
		container.AuthenticateStoragerSingleton,
		container.AuthorizeStoragerSingleton,
	)
	container.AuthenticateManager.SetOperationLogManager(container.OperationLogManager)
	container.AuthorizeManager = iauth.NewAuthorizeManager(
		container.TxnStoragerSingleton,
		container.AuthorizeStoragerSingleton)
	container.AuthorizeManager.SetOperationLogManager(container.OperationLogManager)

	container.PoolManager = icluster_conf.NewPoolManager(
		container.TxnStoragerSingleton,
		container.PoolStoragerSingleton,
		container.BFEClusterStoragerSingleton,
		container.SubClusterStoragerSingleton)

	// Initialize quota management components
	container.EntityTypeStorager = entityStorage.NewEntityTypeStorager(stateful.NewBFEDBContext)
	container.EntityStorager = entityStorage.NewEntityStorager(stateful.NewBFEDBContext)
	container.EntityIDGenerator = entityStorage.NewRDBEntityIDGenerator(stateful.NewBFEDBContext)
	container.QuotaPlanStorager = quotaStorage.NewQuotaPlanStorager(stateful.NewBFEDBContext)
	container.RateLimitPolicyStorager = rateLimitPolicyStorage.NewRateLimitPolicyStorager(stateful.NewBFEDBContext)

	container.QuotaCacheSingleton = quotacache.NewRedisQuotaCache(
		stateful.DefaultClientSet.RedisClient,
	)

	container.EntityTypeManager = entity.NewEntityTypeManager(
		container.TxnStoragerSingleton,
		container.EntityTypeStorager,
		container.EntityStorager)
	container.EntityTypeManager.SetOperationLogManager(container.OperationLogManager)

	container.EntityManager = entity.NewEntityManager(
		container.TxnStoragerSingleton,
		container.EntityStorager,
		container.EntityTypeStorager,
		quota.NewQuotaPlanStoragerAdapter(container.QuotaPlanStorager),
		rate_limit_policy.NewRateLimitPolicyStoragerAdapter(container.RateLimitPolicyStorager),
		container.RouteRulesStorager,
		container.QuotaCacheSingleton)
	container.EntityManager.SetOperationLogManager(container.OperationLogManager)

	container.APIKeyRuleManager = imods.NewAPIKeyRuleManager(
		container.TxnStoragerSingleton,
		container.VersionControlManager,
		container.APIKeyStorager,
		container.AIRouteRuleStorager,
		container.QuotaPlanStorager,
		container.EntityStorager,
		container.EntityTypeStorager,
		container.QuotaCacheSingleton,
	)

	container.ModBodyProcessManager = imods.NewModBodyProcessManager(
		container.VersionControlManager,
	)

	container.QuotaPlanManager = quota.NewQuotaPlanManager(
		container.TxnStoragerSingleton,
		container.QuotaPlanStorager,
		container.APIKeyStorager,
		container.EntityStorager,
		container.QuotaCacheSingleton)
	container.QuotaPlanManager.SetOperationLogManager(container.OperationLogManager)

	container.RateLimitPolicyManager = rate_limit_policy.NewRateLimitPolicyManager(
		container.TxnStoragerSingleton,
		container.RateLimitPolicyStorager,
		container.APIKeyStorager,
		container.EntityStorager,
		container.VersionControlManager)
	container.RateLimitPolicyManager.SetOperationLogManager(container.OperationLogManager)

	container.AICacheStorager = aiCacheStorage.NewAICacheStorager(stateful.NewBFEDBContext)
	container.AICacheSemanticSettingsStorager = aiCacheStorage.NewAICacheSemanticSettingsStorager(stateful.NewBFEDBContext)
	container.AICacheManager = ai_cache.NewAICacheManager(
		container.TxnStoragerSingleton,
		container.AICacheStorager,
		container.AICacheSemanticSettingsStorager,
		container.VersionControlManager,
		stateful.DefaultConfig.RunTime.AIRouteInnerProductName)
	container.AICacheManager.SetOperationLogManager(container.OperationLogManager)

	container.AIContextStorager = aiContextStorage.NewAIContextStorager(stateful.NewBFEDBContext)
	container.AIContextSettingsStorager = aiContextStorage.NewAIContextSettingsStorager(stateful.NewBFEDBContext)
	container.AIContextManager = ai_context.NewAIContextManager(
		container.TxnStoragerSingleton,
		container.AIContextStorager,
		container.AIContextSettingsStorager,
		container.VersionControlManager,
		stateful.DefaultConfig.RunTime.AIRouteInnerProductName)
	container.AIContextManager.SetOperationLogManager(container.OperationLogManager)

	container.TrafficMirrorStorager = trafficMirrorStorage.NewTrafficMirrorStorager(stateful.NewBFEDBContext)
	container.TrafficMirrorManager = traffic_mirror.NewTrafficMirrorManager(
		container.TxnStoragerSingleton,
		container.TrafficMirrorStorager,
		container.VersionControlManager,
		stateful.DefaultConfig.RunTime.AIRouteInnerProductName)
	container.TrafficMirrorManager.SetOperationLogManager(container.OperationLogManager)

	container.IntentConfigStorager = intentConfigStorage.NewIntentConfigStorager(stateful.NewBFEDBContext)
	container.IntentConfigManager = iintent_config.NewIntentConfigManager(
		container.TxnStoragerSingleton,
		container.IntentConfigStorager,
		container.VersionControlManager)
	container.IntentConfigManager.SetOperationLogManager(container.OperationLogManager)

	// Wire nested-resource auditors so Entity/API Key nested quota-plan and
	// rate-limit-policy writes emit operation logs with resource_parent_id
	// filled (issue #161).
	container.EntityManager.SetQuotaPlanAuditor(container.QuotaPlanManager)
	container.EntityManager.SetRateLimitPolicyAuditor(container.RateLimitPolicyManager)

	container.AIRouteExporter = imods.NewAIRouteExporter(
		container.APIKeyStorager,
		container.EntityStorager,
		container.RouteRulesStorager,
		container.VersionControlManager)

	container.APIKeyManager = api_key.NewAPIKeyManager(
		container.TxnStoragerSingleton,
		container.APIKeyStorager,
		quota.NewQuotaPlanStoragerAdapter(container.QuotaPlanStorager),
		quota.NewRateLimitPolicyStoragerAdapter(container.RateLimitPolicyStorager),
		container.RouteRulesStorager,
		quota.NewEntityStoragerAdapter(container.EntityStorager),
		container.QuotaCacheSingleton,
	)
	container.APIKeyManager.SetOperationLogManager(container.OperationLogManager)
	container.APIKeyManager.SetQuotaPlanAuditor(container.QuotaPlanManager)
	container.APIKeyManager.SetRateLimitPolicyAuditor(container.RateLimitPolicyManager)

	// Batch & async tasks (批量任务与对账.md §4/§5): manager serves the
	// /batches control APIs; the reconcile job is assembled only when
	// [BatchJob].Enable is set (it needs the provider egress).
	if err := initBatch(); err != nil {
		return err
	}

	// Initialize quota reset scheduler
	container.BalanceSyncManager = quota.NewBalanceSyncManager(
		container.TxnStoragerSingleton,
		container.APIKeyStorager,
		container.QuotaPlanStorager,
		container.EntityStorager,
		container.QuotaCacheSingleton,
		quota.NewRealClock())

	container.QuotaResetScheduler = quota.NewQuotaResetScheduler(
		container.TxnStoragerSingleton,
		container.BalanceSyncManager,
		quotacache.NewRedisDistributedLock(stateful.DefaultClientSet.RedisClient))

	container.QuotaResetScheduler.Start()

	// Ensure the global route table exists on startup.
	if err := container.RouteRulesManager.EnsureGlobalRouteRules(context.Background()); err != nil {
		return err
	}

	// The batch reconcile job follows the process lifecycle like
	// QuotaResetScheduler (started in initBatch when enabled).
	return nil
}

// initBatch assembles the batch task manager (storage + RedisClient adapter +
// real querier dependencies) and, when [BatchJob].Enable is set, the
// reconcile job. Redis SCAN needs every backend instance address: the bfe
// redis client shards keys by hash and exposes no SCAN, so addresses are
// re-resolved from the same bns conf the client was built from.
func initBatch() error {
	batchStorage := batchStoragePkg.NewBatchStorager(stateful.NewBFEDBContext)
	redisAdapter := ibatch.NewRedisClientAdapter(stateful.DefaultClientSet.RedisClient, newBatchRedisScanner())

	manager := ibatch.NewManager(
		batchStorage,
		redisAdapter,
		container.APIKeyManager,
		container.ProviderManager,
		container.ModelPriceManager,
		nil, // httpClientFactory：http.DefaultClient + 请求级超时
		nil, // clock：系统时钟
	)
	manager.SetOperationLogManager(container.OperationLogManager)
	manager.SetInstancePoolResolver(container.InstancePoolResolver)
	container.BatchManager = manager

	if !stateful.DefaultConfig.BatchJob.Enable {
		return nil
	}
	db, err := stateful.BFEDB()
	if err != nil {
		return err
	}
	container.BatchJob = ibatch.NewJob(
		manager,
		db,
		time.Duration(stateful.DefaultConfig.BatchJob.IntervalSec)*time.Second,
	)
	container.BatchJob.Start()
	return nil
}

// newBatchRedisScanner 按 [RedisConf].Bns 解析全部后端实例地址，构建
// redigo SCAN 器。mock 模式 / 解析失败时返回 nil（同步职责降级为空，
// 其余职责不受影响）。多集群形态（"bns,weight|bns,weight"）逐一解析：
// bfe 客户端按 key 哈希把键分片到各集群，每个 bns 名背后的实例集合
// 持有该集群的全量键，SCAN 必须遍历全部集群。
func newBatchRedisScanner() ibatch.RedisScanner {
	cfg := stateful.DefaultConfig.RedisConf
	if cfg == nil || cfg.Bns == "" || cfg.Bns == "mock" {
		return nil
	}

	var addrs []string
	for _, part := range strings.Split(cfg.Bns, "|") {
		name := strings.TrimSpace(strings.SplitN(part, ",", 2)[0])
		if name == "" {
			continue
		}
		resolved, err := bns.NewClient().GetInstancesAddr(name)
		if err != nil {
			stateful.AccessLogger.Warn("batch: resolve redis bns %s for scanner failed: %v", name, err)
			continue
		}
		addrs = append(addrs, resolved...)
	}
	if len(addrs) == 0 {
		stateful.AccessLogger.Warn("batch: no redis instance resolved for scanner, sync duty degraded")
		return nil
	}

	timeout := time.Duration(cfg.ReadTimeout) * time.Millisecond
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return ibatch.NewRedigoScanner(addrs, cfg.Password, timeout)
}

func initReport() error {
	cfg := stateful.DefaultConfig.Report
	if cfg.Backend == "" {
		// Pure-incremental default: no report assembly, /report/* stay 404.
		return nil
	}

	db, err := stateful.DbGet(cfg.Datasource)
	if err != nil {
		return err
	}

	switch cfg.Backend {
	case "mysql":
		container.ReportManager = ireport.NewReportManager(mysqlreport.New(db, cfg.Database, "mysql"))
		if cfg.EnableAggregateJob || cfg.EnablePartitionMgmt {
			interval := time.Duration(cfg.AggregateIntervalSec) * time.Second
			if !cfg.EnableAggregateJob {
				interval = 0
			}
			retentionDays := cfg.RetentionDays
			if !cfg.EnablePartitionMgmt {
				retentionDays = 0
			}
			job := mysqlreport.NewJob(db, cfg.Database, interval, retentionDays)
			job.Start()
		}
	case "doris":
		// Doris aggregation is maintained by the existing Doris insert job;
		// the api only queries. The backend identifier feeds Capabilities(),
		// which gates the cache/mirror/intent dimensions (phase 1: mysql
		// only, see design-docs modifications/2026-09-27-report-cache-mirror-intent-fields).
		container.ReportManager = ireport.NewReportManager(dorisreport.New(db, cfg.Database, "doris"))
	case "clickhouse":
		// ClickHouse aggregation is maintained by the warehouse-side
		// materialized view and retention by the tables' TTL; the api only
		// queries, no local job (see design-docs
		// modifications/2026-10-02-report-clickhouse-backend).
		container.ReportManager = ireport.NewReportManager(clickhousereport.New(db, cfg.Database, "clickhouse"))
	case "starrocks":
		// StarRocks aggregation is maintained by the warehouse-side async
		// materialized view; retention by dynamic_partition + partition_ttl.
		// The api only queries, no local job (see design-docs
		// modifications/2026-10-02-report-starrocks-backend).
		container.ReportManager = ireport.NewReportManager(starrocksreport.New(db, cfg.Database, "starrocks"))
	default:
		container.ReportManager = nil
		return fmt.Errorf("unsupported [Report].Backend: %s", cfg.Backend)
	}
	return nil
}

func operationLogContextExtractor(ctx context.Context, entry *ioperlog.OperationLogEntry) {
	if visitor, err := iauth.MustGetVisitor(ctx); err == nil && visitor != nil {
		entry.OperatorName = visitor.GetName()
		if visitor.User != nil {
			entry.OperatorType = ioperlog.OperatorTypeUser
			entry.OperatorID = visitor.User.ID
		} else if visitor.Token != nil {
			entry.OperatorType = ioperlog.OperatorTypeToken
			entry.OperatorID = visitor.Token.ID
		}
	}

	if reqInfo := xreq.GetRequestInfo(ctx); reqInfo != nil {
		entry.LogID = reqInfo.LogID
		entry.RequestPath = reqInfo.URLPath
		entry.RequestMethod = reqInfo.Method
		entry.ClientIP = reqInfo.ClientIP
		entry.UserAgent = reqInfo.UserAgent
	}
}
