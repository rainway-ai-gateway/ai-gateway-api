// Copyright(c) 2026 The Rainway AI Gateway (壬远AI网关) Authors.
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

package icluster_conf

import (
	"context"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/api_key"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ibasic"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iprovider"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/shared"
)

type fakeTxn struct{}

func (f *fakeTxn) AtomExecute(ctx context.Context, do func(context.Context) error) error {
	return do(ctx)
}

type fakeProductStorager struct {
	fetchProductsFn func(ctx context.Context, param *ibasic.ProductFilter) ([]*ibasic.Product, error)
	deleteProductFn func(ctx context.Context, p *ibasic.Product) error
	createProductFn func(ctx context.Context, p *ibasic.ProductParam) error
	updateProductFn func(ctx context.Context, p *ibasic.Product, newVal *ibasic.ProductParam) error
}

func (f *fakeProductStorager) FetchProducts(ctx context.Context, param *ibasic.ProductFilter) ([]*ibasic.Product, error) {
	if f.fetchProductsFn != nil {
		return f.fetchProductsFn(ctx, param)
	}
	return nil, nil
}

func (f *fakeProductStorager) DeleteProduct(ctx context.Context, p *ibasic.Product) error {
	if f.deleteProductFn != nil {
		return f.deleteProductFn(ctx, p)
	}
	return nil
}

func (f *fakeProductStorager) CreateProduct(ctx context.Context, p *ibasic.ProductParam) error {
	if f.createProductFn != nil {
		return f.createProductFn(ctx, p)
	}
	return nil
}

func (f *fakeProductStorager) UpdateProduct(ctx context.Context, p *ibasic.Product, newVal *ibasic.ProductParam) error {
	if f.updateProductFn != nil {
		return f.updateProductFn(ctx, p, newVal)
	}
	return nil
}

type fakeBFEClusterStorager struct {
	fetchBFEClustersFn func(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error)
	createBFEClusterFn func(ctx context.Context, param *ibasic.BFEClusterParam) error
	deleteBFEClusterFn func(ctx context.Context, cluster *ibasic.BFECluster) error
}

func (f *fakeBFEClusterStorager) FetchBFEClusters(ctx context.Context, param *ibasic.BFEClusterFilter) ([]*ibasic.BFECluster, error) {
	if f.fetchBFEClustersFn != nil {
		return f.fetchBFEClustersFn(ctx, param)
	}
	return nil, nil
}

func (f *fakeBFEClusterStorager) CreateBFECluster(ctx context.Context, param *ibasic.BFEClusterParam) error {
	if f.createBFEClusterFn != nil {
		return f.createBFEClusterFn(ctx, param)
	}
	return nil
}

func (f *fakeBFEClusterStorager) DeleteBFECluster(ctx context.Context, cluster *ibasic.BFECluster) error {
	if f.deleteBFEClusterFn != nil {
		return f.deleteBFEClusterFn(ctx, cluster)
	}
	return nil
}

type fakeClusterStorager struct {
	fetchClusterFn      func(ctx context.Context, param *ClusterFilter) (*Cluster, error)
	fetchClusterListFn  func(ctx context.Context, param *ClusterFilter) ([]*Cluster, error)
	clusterUpdateFn     func(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error
	clusterCreateFn     func(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error)
	clusterDeleteFn     func(ctx context.Context, product *ibasic.Product, cluster *Cluster) error
	bindSubClusterFn    func(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error
	fetchLBMatrixListFn func(ctx context.Context) (map[int64]map[string]map[string]int, error)
}

func (f *fakeClusterStorager) FetchCluster(ctx context.Context, param *ClusterFilter) (*Cluster, error) {
	if f.fetchClusterFn != nil {
		return f.fetchClusterFn(ctx, param)
	}
	return nil, nil
}

func (f *fakeClusterStorager) FetchClusterList(ctx context.Context, param *ClusterFilter) ([]*Cluster, error) {
	if f.fetchClusterListFn != nil {
		return f.fetchClusterListFn(ctx, param)
	}
	return nil, nil
}

func (f *fakeClusterStorager) ClusterUpdate(ctx context.Context, product *ibasic.Product, old *Cluster, param *ClusterParam) error {
	if f.clusterUpdateFn != nil {
		return f.clusterUpdateFn(ctx, product, old, param)
	}
	return nil
}

func (f *fakeClusterStorager) ClusterCreate(ctx context.Context, product *ibasic.Product, param *ClusterParam, subClusters []*SubCluster) (int64, error) {
	if f.clusterCreateFn != nil {
		return f.clusterCreateFn(ctx, product, param, subClusters)
	}
	return 0, nil
}

func (f *fakeClusterStorager) ClusterDelete(ctx context.Context, product *ibasic.Product, cluster *Cluster) error {
	if f.clusterDeleteFn != nil {
		return f.clusterDeleteFn(ctx, product, cluster)
	}
	return nil
}

func (f *fakeClusterStorager) BindSubCluster(ctx context.Context, cluster *Cluster, appendSubClusters, unbindSubClusters []*SubCluster) error {
	if f.bindSubClusterFn != nil {
		return f.bindSubClusterFn(ctx, cluster, appendSubClusters, unbindSubClusters)
	}
	return nil
}

func (f *fakeClusterStorager) FetchLBMatrixList(ctx context.Context) (map[int64]map[string]map[string]int, error) {
	if f.fetchLBMatrixListFn != nil {
		return f.fetchLBMatrixListFn(ctx)
	}
	return nil, nil
}

type fakePoolStorager struct {
	fetchPoolFn  func(ctx context.Context, name string) (*Pool, error)
	fetchPoolsFn func(ctx context.Context, param *PoolFilter) ([]*Pool, error)
	createPoolFn func(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error)
	updatePoolFn func(ctx context.Context, oldData *Pool, diff *PoolParam) error
	deletePoolFn func(ctx context.Context, pool *Pool) error
}

func (f *fakePoolStorager) FetchPool(ctx context.Context, name string) (*Pool, error) {
	if f.fetchPoolFn != nil {
		return f.fetchPoolFn(ctx, name)
	}
	return nil, nil
}

func (f *fakePoolStorager) FetchPools(ctx context.Context, param *PoolFilter) ([]*Pool, error) {
	if f.fetchPoolsFn != nil {
		return f.fetchPoolsFn(ctx, param)
	}
	return nil, nil
}

func (f *fakePoolStorager) CreatePool(ctx context.Context, product *ibasic.Product, data *PoolParam) (*Pool, error) {
	if f.createPoolFn != nil {
		return f.createPoolFn(ctx, product, data)
	}
	return &Pool{}, nil
}

func (f *fakePoolStorager) UpdatePool(ctx context.Context, oldData *Pool, diff *PoolParam) error {
	if f.updatePoolFn != nil {
		return f.updatePoolFn(ctx, oldData, diff)
	}
	return nil
}

func (f *fakePoolStorager) DeletePool(ctx context.Context, pool *Pool) error {
	if f.deletePoolFn != nil {
		return f.deletePoolFn(ctx, pool)
	}
	return nil
}

type fakeProviderStorager struct {
	fetchFn     func(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error)
	fetchListFn func(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error)
}

func (f *fakeProviderStorager) CreateProvider(ctx context.Context, param *iprovider.ProviderParam) (int64, error) {
	return 1, nil
}

func (f *fakeProviderStorager) UpdateProvider(ctx context.Context, name string, param *iprovider.ProviderParam) error {
	return nil
}

func (f *fakeProviderStorager) DeleteProvider(ctx context.Context, name string) error {
	return nil
}

func (f *fakeProviderStorager) FetchProvider(ctx context.Context, filter *iprovider.ProviderFilter) (*iprovider.Provider, error) {
	if f.fetchFn != nil {
		return f.fetchFn(ctx, filter)
	}
	if filter != nil && filter.Name != nil {
		return &iprovider.Provider{
			Name:           *filter.Name,
			Models:         []string{"m1", "m2"},
			Keys:           []iprovider.ProviderKey{{Name: "k1", Key: "sk-xxx"}},
			InstancePool:   []iprovider.ProviderInstance{{Addr: "127.0.0.1", Port: 80, Weight: 10}},
			ModelProtocols: []string{"openai"},
		}, nil
	}
	return &iprovider.Provider{Name: "openai"}, nil
}

func (f *fakeProviderStorager) FetchProviderList(ctx context.Context, filter *iprovider.ProviderFilter) ([]*iprovider.Provider, int64, error) {
	if f.fetchListFn != nil {
		return f.fetchListFn(ctx, filter)
	}
	return nil, 0, nil
}

func (f *fakeProviderStorager) FetchProviderNames(ctx context.Context) ([]string, error) {
	return nil, nil
}

type fakeSubClusterStorager struct {
	fetchSubClusterListFn func(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error)
	createSubClusterFn    func(ctx context.Context, param *SubClusterParam) error
	deleteSubClusterFn    func(ctx context.Context, param *SubCluster) error
	updateSubClusterFn    func(ctx context.Context, one *SubCluster, param *SubClusterParam) error
}

func (f *fakeSubClusterStorager) FetchSubClusterList(ctx context.Context, param *SubClusterFilter) ([]*SubCluster, error) {
	if f.fetchSubClusterListFn != nil {
		return f.fetchSubClusterListFn(ctx, param)
	}
	return nil, nil
}

func (f *fakeSubClusterStorager) CreateSubCluster(ctx context.Context, param *SubClusterParam) error {
	if f.createSubClusterFn != nil {
		return f.createSubClusterFn(ctx, param)
	}
	return nil
}

func (f *fakeSubClusterStorager) DeleteSubCluster(ctx context.Context, param *SubCluster) error {
	if f.deleteSubClusterFn != nil {
		return f.deleteSubClusterFn(ctx, param)
	}
	return nil
}

func (f *fakeSubClusterStorager) UpdateSubCluster(ctx context.Context, one *SubCluster, param *SubClusterParam) error {
	if f.updateSubClusterFn != nil {
		return f.updateSubClusterFn(ctx, one, param)
	}
	return nil
}

type fakeAPIKeyStorager struct {
	fetchAPIKeyListFn      func(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error)
	createAPIKeyFn         func(ctx context.Context, param *api_key.APIKeyParam) (int64, error)
	updateAPIKeyFn         func(ctx context.Context, filter *api_key.APIKeyFilter, param *api_key.APIKeyParam) (int64, error)
	deleteAPIKeyFn         func(ctx context.Context, filter *api_key.APIKeyFilter) error
	createAPIKeyTokenFn    func(ctx context.Context, param *api_key.APIKeyTokenParam) (int64, error)
	updateAPIKeyTokenFn    func(ctx context.Context, filter *api_key.APIKeyTokenFilter, param *api_key.APIKeyTokenParam) error
	fetchAPIKeyTokenListFn func(ctx context.Context, filter *api_key.APIKeyTokenFilter) ([]*api_key.APIKeyTokenParam, error)
}

func (f *fakeAPIKeyStorager) FetchAPIKeyList(ctx context.Context, filter *api_key.APIKeyFilter) ([]*api_key.APIKeyParam, error) {
	if f.fetchAPIKeyListFn != nil {
		return f.fetchAPIKeyListFn(ctx, filter)
	}
	return nil, nil
}

func (f *fakeAPIKeyStorager) CreateAPIKey(ctx context.Context, param *api_key.APIKeyParam) (int64, error) {
	if f.createAPIKeyFn != nil {
		return f.createAPIKeyFn(ctx, param)
	}
	return 0, nil
}

func (f *fakeAPIKeyStorager) UpdateAPIKey(ctx context.Context, filter *api_key.APIKeyFilter, param *api_key.APIKeyParam) (int64, error) {
	if f.updateAPIKeyFn != nil {
		return f.updateAPIKeyFn(ctx, filter, param)
	}
	return 0, nil
}

func (f *fakeAPIKeyStorager) DeleteAPIKey(ctx context.Context, filter *api_key.APIKeyFilter) error {
	if f.deleteAPIKeyFn != nil {
		return f.deleteAPIKeyFn(ctx, filter)
	}
	return nil
}

func (f *fakeAPIKeyStorager) CreateAPIKeyToken(ctx context.Context, param *api_key.APIKeyTokenParam) (int64, error) {
	if f.createAPIKeyTokenFn != nil {
		return f.createAPIKeyTokenFn(ctx, param)
	}
	return 0, nil
}

func (f *fakeAPIKeyStorager) UpdateAPIKeyToken(ctx context.Context, filter *api_key.APIKeyTokenFilter, param *api_key.APIKeyTokenParam) error {
	if f.updateAPIKeyTokenFn != nil {
		return f.updateAPIKeyTokenFn(ctx, filter, param)
	}
	return nil
}

func (f *fakeAPIKeyStorager) FetchAPIKeyTokenList(ctx context.Context, filter *api_key.APIKeyTokenFilter) ([]*api_key.APIKeyTokenParam, error) {
	if f.fetchAPIKeyTokenListFn != nil {
		return f.fetchAPIKeyTokenListFn(ctx, filter)
	}
	return nil, nil
}

type fakeVersionControlStorager struct {
	upsertFn func(ctx context.Context, css *iversion_control.ExportData) (string, error)
}

func (f *fakeVersionControlStorager) UpsertConfigLastExportedVersion(ctx context.Context, css *iversion_control.ExportData) (string, error) {
	if f.upsertFn != nil {
		return f.upsertFn(ctx, css)
	}
	return "", nil
}

type fakeQuotaPlanStorager struct {
	createQuotaPlanFn func(ctx context.Context, param *shared.QuotaPlanParam) (int64, error)
	updateQuotaPlanFn func(ctx context.Context, id int64, param *shared.QuotaPlanParam) (int64, error)
	deleteQuotaPlanFn func(ctx context.Context, id int64) error
	fetchQuotaPlanFn  func(ctx context.Context, id int64) (*shared.QuotaPlanParam, error)
}

func (f *fakeQuotaPlanStorager) CreateQuotaPlan(ctx context.Context, param *shared.QuotaPlanParam) (int64, error) {
	if f.createQuotaPlanFn != nil {
		return f.createQuotaPlanFn(ctx, param)
	}
	return 0, nil
}

func (f *fakeQuotaPlanStorager) UpdateQuotaPlan(ctx context.Context, id int64, param *shared.QuotaPlanParam) (int64, error) {
	if f.updateQuotaPlanFn != nil {
		return f.updateQuotaPlanFn(ctx, id, param)
	}
	return 0, nil
}

func (f *fakeQuotaPlanStorager) DeleteQuotaPlan(ctx context.Context, id int64) error {
	if f.deleteQuotaPlanFn != nil {
		return f.deleteQuotaPlanFn(ctx, id)
	}
	return nil
}

func (f *fakeQuotaPlanStorager) FetchQuotaPlan(ctx context.Context, id int64) (*shared.QuotaPlanParam, error) {
	if f.fetchQuotaPlanFn != nil {
		return f.fetchQuotaPlanFn(ctx, id)
	}
	return nil, nil
}

type fakeRateLimitPolicyStorager struct {
	createRateLimitPolicyFn func(ctx context.Context, param *shared.RateLimitPolicyParam) (int64, error)
	updateRateLimitPolicyFn func(ctx context.Context, id int64, param *shared.RateLimitPolicyParam) (int64, error)
	deleteRateLimitPolicyFn func(ctx context.Context, id int64) error
	fetchRateLimitPolicyFn  func(ctx context.Context, id int64) (*shared.RateLimitPolicyParam, error)
}

func (f *fakeRateLimitPolicyStorager) CreateRateLimitPolicy(ctx context.Context, param *shared.RateLimitPolicyParam) (int64, error) {
	if f.createRateLimitPolicyFn != nil {
		return f.createRateLimitPolicyFn(ctx, param)
	}
	return 0, nil
}

func (f *fakeRateLimitPolicyStorager) UpdateRateLimitPolicy(ctx context.Context, id int64, param *shared.RateLimitPolicyParam) (int64, error) {
	if f.updateRateLimitPolicyFn != nil {
		return f.updateRateLimitPolicyFn(ctx, id, param)
	}
	return 0, nil
}

func (f *fakeRateLimitPolicyStorager) DeleteRateLimitPolicy(ctx context.Context, id int64) error {
	if f.deleteRateLimitPolicyFn != nil {
		return f.deleteRateLimitPolicyFn(ctx, id)
	}
	return nil
}

func (f *fakeRateLimitPolicyStorager) FetchRateLimitPolicy(ctx context.Context, id int64) (*shared.RateLimitPolicyParam, error) {
	if f.fetchRateLimitPolicyFn != nil {
		return f.fetchRateLimitPolicyFn(ctx, id)
	}
	return nil, nil
}

type fakeRouteRulesStorager struct {
	createRouteRulesFn    func(ctx context.Context, ruleType string, owner *string, param *shared.RouteRulesParam) (int64, error)
	fetchRouteRulesFn     func(ctx context.Context, ruleType string, owner *string) (*shared.RouteRulesParam, error)
	fetchRouteRulesListFn func(ctx context.Context, filter *shared.RouteRulesFilter) ([]*shared.RouteTableParam, int64, error)
	updateRouteRulesFn    func(ctx context.Context, id int64, param *shared.RouteRulesParam) (int64, error)
	deleteRouteRulesFn    func(ctx context.Context, id int64) error
	fetchRouteRulesByIDFn func(ctx context.Context, id int64) (*shared.RouteRulesParam, error)
}

func (f *fakeRouteRulesStorager) CreateRouteRules(ctx context.Context, ruleType string, owner *string, param *shared.RouteRulesParam) (int64, error) {
	if f.createRouteRulesFn != nil {
		return f.createRouteRulesFn(ctx, ruleType, owner, param)
	}
	return 0, nil
}

func (f *fakeRouteRulesStorager) FetchRouteRules(ctx context.Context, ruleType string, owner *string) (*shared.RouteRulesParam, error) {
	if f.fetchRouteRulesFn != nil {
		return f.fetchRouteRulesFn(ctx, ruleType, owner)
	}
	return nil, nil
}

func (f *fakeRouteRulesStorager) FetchRouteRulesList(ctx context.Context, filter *shared.RouteRulesFilter) ([]*shared.RouteTableParam, int64, error) {
	if f.fetchRouteRulesListFn != nil {
		return f.fetchRouteRulesListFn(ctx, filter)
	}
	return nil, 0, nil
}

func (f *fakeRouteRulesStorager) UpdateRouteRules(ctx context.Context, id int64, param *shared.RouteRulesParam) (int64, error) {
	if f.updateRouteRulesFn != nil {
		return f.updateRouteRulesFn(ctx, id, param)
	}
	return 0, nil
}

func (f *fakeRouteRulesStorager) DeleteRouteRules(ctx context.Context, id int64) error {
	if f.deleteRouteRulesFn != nil {
		return f.deleteRouteRulesFn(ctx, id)
	}
	return nil
}

func (f *fakeRouteRulesStorager) FetchRouteRulesByID(ctx context.Context, id int64) (*shared.RouteRulesParam, error) {
	if f.fetchRouteRulesByIDFn != nil {
		return f.fetchRouteRulesByIDFn(ctx, id)
	}
	return nil, nil
}

func (f *fakeRouteRulesStorager) FetchAllRouteRules(ctx context.Context) ([]*shared.RouteRulesParam, error) {
	return nil, nil
}

type fakeEntityStorager struct {
	fetchEntityFn func(ctx context.Context, filter *shared.EntityFilter) (*shared.EntitySummary, error)
}

func (f *fakeEntityStorager) FetchEntity(ctx context.Context, filter *shared.EntityFilter) (*shared.EntitySummary, error) {
	if f.fetchEntityFn != nil {
		return f.fetchEntityFn(ctx, filter)
	}
	return nil, nil
}

type fakeQuotaBalanceStorager struct {
	fetchQuotaBalanceFn  func(ctx context.Context, quotaPlanID int64) (*shared.BalanceSummary, error)
	createQuotaBalanceFn func(ctx context.Context, quotaPlanID int64, remaining *float64) error
	deleteQuotaBalanceFn func(ctx context.Context, quotaPlanID int64) error
}

func (f *fakeQuotaBalanceStorager) FetchQuotaBalance(ctx context.Context, quotaPlanID int64) (*shared.BalanceSummary, error) {
	if f.fetchQuotaBalanceFn != nil {
		return f.fetchQuotaBalanceFn(ctx, quotaPlanID)
	}
	return nil, nil
}

func (f *fakeQuotaBalanceStorager) CreateQuotaBalance(ctx context.Context, quotaPlanID int64, remaining *float64) error {
	if f.createQuotaBalanceFn != nil {
		return f.createQuotaBalanceFn(ctx, quotaPlanID, remaining)
	}
	return nil
}

func (f *fakeQuotaBalanceStorager) DeleteQuotaBalance(ctx context.Context, quotaPlanID int64) error {
	if f.deleteQuotaBalanceFn != nil {
		return f.deleteQuotaBalanceFn(ctx, quotaPlanID)
	}
	return nil
}

var fixedTestTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// fakeK8sPoolInstanceReader implements iprovider.K8sPoolInstanceReader for
// the realtime-resolution export path.
type fakeK8sPoolInstanceReader struct {
	instances []iprovider.ProviderInstance
	err       error
	calls     int
}

func (f *fakeK8sPoolInstanceReader) FetchK8sPoolInstances(ctx context.Context, poolName string) ([]iprovider.ProviderInstance, error) {
	f.calls++
	return f.instances, f.err
}

type fakeEPPAssignmentResolver struct {
	endpointsFn func(ctx context.Context, clusterName string) ([]string, bool, error)
}

func (f *fakeEPPAssignmentResolver) GetAssignmentEndpoints(ctx context.Context, clusterName string) ([]string, bool, error) {
	if f.endpointsFn != nil {
		return f.endpointsFn(ctx, clusterName)
	}
	return nil, false, nil
}
