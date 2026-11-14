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

package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ireport"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

type fakeReportManager struct {
	overviewFn     func(ctx context.Context, query *ireport.OverviewQuery) (*ireport.OverviewResult, error)
	timeSeriesFn   func(ctx context.Context, query *ireport.TimeSeriesQuery) ([]*ireport.MetricPoint, error)
	rankingsFn     func(ctx context.Context, query *ireport.RankingsQuery) ([]*ireport.RankingItem, error)
	distributionFn func(ctx context.Context, query *ireport.DistributionQuery) ([]*ireport.DistItem, error)
	logsFn         func(ctx context.Context, query *ireport.LogsQuery) (*ireport.LogQueryResult, error)
}

func (m *fakeReportManager) Overview(ctx context.Context, query *ireport.OverviewQuery) (*ireport.OverviewResult, error) {
	if m.overviewFn != nil {
		return m.overviewFn(ctx, query)
	}
	return &ireport.OverviewResult{}, nil
}

func (m *fakeReportManager) TimeSeries(ctx context.Context, query *ireport.TimeSeriesQuery) ([]*ireport.MetricPoint, error) {
	if m.timeSeriesFn != nil {
		return m.timeSeriesFn(ctx, query)
	}
	return nil, nil
}

func (m *fakeReportManager) Rankings(ctx context.Context, query *ireport.RankingsQuery) ([]*ireport.RankingItem, error) {
	if m.rankingsFn != nil {
		return m.rankingsFn(ctx, query)
	}
	return nil, nil
}

func (m *fakeReportManager) Distribution(ctx context.Context, query *ireport.DistributionQuery) ([]*ireport.DistItem, error) {
	if m.distributionFn != nil {
		return m.distributionFn(ctx, query)
	}
	return nil, nil
}

func (m *fakeReportManager) Logs(ctx context.Context, query *ireport.LogsQuery) (*ireport.LogQueryResult, error) {
	if m.logsFn != nil {
		return m.logsFn(ctx, query)
	}
	return &ireport.LogQueryResult{}, nil
}

func TestEndpoints(t *testing.T) {
	require.Len(t, Endpoints, 5)
	for _, ep := range Endpoints {
		require.NotNil(t, ep)
		assert.NotEmpty(t, ep.Path)
		assert.NotEmpty(t, ep.Method)
		require.NotNil(t, ep.Authorizer)
	}
}

const testQuery = "start=1782345600&end=1782349200"

func TestReportOverviewAction(t *testing.T) {
	var gotQuery *ireport.OverviewQuery
	manager := &fakeReportManager{
		overviewFn: func(ctx context.Context, query *ireport.OverviewQuery) (*ireport.OverviewResult, error) {
			gotQuery = query
			return &ireport.OverviewResult{RequestTotal: 42, LogsTotal: 42}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/overview?"+testQuery+"&models=gpt-4o,gpt-4&stream=true&status_codes=200,500", nil)
	resp, err := ReportOverviewAction(req)
	require.NoError(t, err)

	overview, ok := resp.(*ireport.OverviewResult)
	require.True(t, ok)
	assert.Equal(t, int64(42), overview.RequestTotal)

	require.NotNil(t, gotQuery)
	assert.Equal(t, time.Unix(1782345600, 0), gotQuery.Start)
	assert.Equal(t, time.Unix(1782349200, 0), gotQuery.End)
	assert.Equal(t, []string{"gpt-4o", "gpt-4"}, gotQuery.Models)
	require.NotNil(t, gotQuery.Stream)
	assert.True(t, *gotQuery.Stream)
	assert.Equal(t, []int{200, 500}, gotQuery.StatusCodes)
}

func TestReportTimeSeriesAction(t *testing.T) {
	var gotQuery *ireport.TimeSeriesQuery
	manager := &fakeReportManager{
		timeSeriesFn: func(ctx context.Context, query *ireport.TimeSeriesQuery) ([]*ireport.MetricPoint, error) {
			gotQuery = query
			value := 12.3
			return []*ireport.MetricPoint{{Time: 1782345600, Value: &value}}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/timeseries?"+testQuery+"&metric=qps", nil)
	resp, err := ReportTimeSeriesAction(req)
	require.NoError(t, err)

	series, ok := resp.(*timeSeriesResponse)
	require.True(t, ok)
	assert.Equal(t, 60, series.BucketSec)
	require.Len(t, series.Series, 1)
	require.NotNil(t, series.Series[0].Value)
	assert.InDelta(t, 12.3, *series.Series[0].Value, 1e-9)

	require.NotNil(t, gotQuery)
	assert.Equal(t, ireport.MetricQPS, gotQuery.Metric)
}

func TestReportTimeSeriesAction_MissingMetric(t *testing.T) {
	container.ReportManager = &fakeReportManager{}
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/timeseries?"+testQuery, nil)
	_, err := ReportTimeSeriesAction(req)
	require.Error(t, err)
}

func TestReportRankingsAction(t *testing.T) {
	var gotQuery *ireport.RankingsQuery
	manager := &fakeReportManager{
		rankingsFn: func(ctx context.Context, query *ireport.RankingsQuery) ([]*ireport.RankingItem, error) {
			gotQuery = query
			return []*ireport.RankingItem{{Name: "gpt-4o", RequestCount: 900}}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/rankings?"+testQuery+"&dimension=model&limit=25", nil)
	resp, err := ReportRankingsAction(req)
	require.NoError(t, err)

	rankings, ok := resp.(*rankingsResponse)
	require.True(t, ok)
	require.Len(t, rankings.Items, 1)
	assert.Equal(t, "gpt-4o", rankings.Items[0].Name)

	require.NotNil(t, gotQuery)
	assert.Equal(t, ireport.DimensionModel, gotQuery.Dimension)
	assert.Equal(t, 25, gotQuery.Limit)
}

func TestReportRankingsAction_DefaultLimit(t *testing.T) {
	var gotQuery *ireport.RankingsQuery
	manager := &fakeReportManager{
		rankingsFn: func(ctx context.Context, query *ireport.RankingsQuery) ([]*ireport.RankingItem, error) {
			gotQuery = query
			return nil, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/rankings?"+testQuery+"&dimension=provider", nil)
	_, err := ReportRankingsAction(req)
	require.NoError(t, err)
	require.NotNil(t, gotQuery)
	assert.Equal(t, 0, gotQuery.Limit) // manager applies the default
}

func TestReportRankingsAction_InvalidLimit(t *testing.T) {
	container.ReportManager = &fakeReportManager{}
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/rankings?"+testQuery+"&dimension=model&limit=abc", nil)
	_, err := ReportRankingsAction(req)
	require.Error(t, err)
}

func TestReportDistributionAction(t *testing.T) {
	var gotQuery *ireport.DistributionQuery
	manager := &fakeReportManager{
		distributionFn: func(ctx context.Context, query *ireport.DistributionQuery) ([]*ireport.DistItem, error) {
			gotQuery = query
			return []*ireport.DistItem{{Name: "200", RequestCount: 150, Ratio: 0.985}}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/distribution?"+testQuery+"&dimension=status", nil)
	resp, err := ReportDistributionAction(req)
	require.NoError(t, err)

	distribution, ok := resp.(*distributionResponse)
	require.True(t, ok)
	require.Len(t, distribution.Items, 1)
	assert.Equal(t, "200", distribution.Items[0].Name)

	require.NotNil(t, gotQuery)
	assert.Equal(t, ireport.DimensionStatus, gotQuery.Dimension)
}

func TestReportLogsAction(t *testing.T) {
	var gotQuery *ireport.LogsQuery
	manager := &fakeReportManager{
		logsFn: func(ctx context.Context, query *ireport.LogsQuery) (*ireport.LogQueryResult, error) {
			gotQuery = query
			return &ireport.LogQueryResult{
				Total:    1,
				Page:     2,
				PageSize: 50,
				Items:    []*ireport.LogRow{{LogTime: 1782345600}},
			}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	url := "/open-api/v1/report/logs?" + testQuery +
		"&models=gpt-4o&requested_models=gpt-4&err_only=true&keyword=timeout&page=2&page_size=50"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	resp, err := ReportLogsAction(req)
	require.NoError(t, err)

	logs, ok := resp.(*ireport.LogQueryResult)
	require.True(t, ok)
	assert.Equal(t, int64(1), logs.Total)
	require.Len(t, logs.Items, 1)
	assert.Equal(t, int64(1782345600), logs.Items[0].LogTime)

	require.NotNil(t, gotQuery)
	assert.Equal(t, []string{"gpt-4o"}, gotQuery.Models)
	assert.Equal(t, []string{"gpt-4"}, gotQuery.RequestedModels)
	assert.True(t, gotQuery.ErrOnly)
	assert.Equal(t, "timeout", gotQuery.Keyword)
	assert.Equal(t, 2, gotQuery.Page)
	assert.Equal(t, 50, gotQuery.PageSize)
}

func TestReportLogsAction_Defaults(t *testing.T) {
	var gotQuery *ireport.LogsQuery
	manager := &fakeReportManager{
		logsFn: func(ctx context.Context, query *ireport.LogsQuery) (*ireport.LogQueryResult, error) {
			gotQuery = query
			return &ireport.LogQueryResult{}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/logs?"+testQuery, nil)
	_, err := ReportLogsAction(req)
	require.NoError(t, err)
	require.NotNil(t, gotQuery)
	assert.False(t, gotQuery.ErrOnly)
	assert.Equal(t, "", gotQuery.Keyword)
	assert.Equal(t, 0, gotQuery.Page)
	assert.Equal(t, 0, gotQuery.PageSize)
	assert.Nil(t, gotQuery.CacheStatus)
	assert.Nil(t, gotQuery.MirrorHit)
	assert.Nil(t, gotQuery.IntentQuestion)
	assert.Nil(t, gotQuery.IntentAnswer)
	assert.Nil(t, gotQuery.IntentSource)
}

// TestReportLogsAction_CacheMirrorIntentFilters 验证五个新过滤参数绑定：
// cache_status / mirror_hit / intent_question / intent_answer / intent_source。
func TestReportLogsAction_CacheMirrorIntentFilters(t *testing.T) {
	var gotQuery *ireport.LogsQuery
	manager := &fakeReportManager{
		logsFn: func(ctx context.Context, query *ireport.LogsQuery) (*ireport.LogQueryResult, error) {
			gotQuery = query
			return &ireport.LogQueryResult{}, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	url := "/open-api/v1/report/logs?" + testQuery +
		"&cache_status=hit&mirror_hit=true&intent_question=task_type&intent_answer=unknown&intent_source=classifier" +
		"&price_warn=fallback"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	_, err := ReportLogsAction(req)
	require.NoError(t, err)

	require.NotNil(t, gotQuery)
	require.NotNil(t, gotQuery.CacheStatus)
	assert.Equal(t, "hit", *gotQuery.CacheStatus)
	require.NotNil(t, gotQuery.MirrorHit)
	assert.True(t, *gotQuery.MirrorHit)
	require.NotNil(t, gotQuery.IntentQuestion)
	assert.Equal(t, "task_type", *gotQuery.IntentQuestion)
	require.NotNil(t, gotQuery.IntentAnswer)
	assert.Equal(t, "unknown", *gotQuery.IntentAnswer)
	require.NotNil(t, gotQuery.IntentSource)
	assert.Equal(t, "classifier", *gotQuery.IntentSource)
	require.NotNil(t, gotQuery.PriceWarn)
	assert.Equal(t, "fallback", *gotQuery.PriceWarn)
}

// TestReportTimeSeriesAction_Dimension 验证 timeseries 可选 dimension 参数绑定。
func TestReportTimeSeriesAction_Dimension(t *testing.T) {
	var gotQuery *ireport.TimeSeriesQuery
	manager := &fakeReportManager{
		timeSeriesFn: func(ctx context.Context, query *ireport.TimeSeriesQuery) ([]*ireport.MetricPoint, error) {
			gotQuery = query
			return nil, nil
		},
	}
	container.ReportManager = manager
	defer func() { container.ReportManager = nil }()

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/timeseries?"+testQuery+"&metric=qps&dimension=ai_cache_status", nil)
	_, err := ReportTimeSeriesAction(req)
	require.NoError(t, err)
	require.NotNil(t, gotQuery)
	assert.Equal(t, ireport.DimensionCacheStatus, gotQuery.Dimension)
}

func TestReportLogsAction_InvalidParams(t *testing.T) {
	container.ReportManager = &fakeReportManager{}
	defer func() { container.ReportManager = nil }()

	cases := []string{
		"err_only=not-bool",
		"page=0",
		"page_size=-1",
		"status_codes=200,abc",
		"mirror_hit=not-bool",
		"intent_source=bogus",
		"price_warn=bogus",
	}
	for _, one := range cases {
		req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/logs?"+testQuery+"&"+one, nil)
		_, err := ReportLogsAction(req)
		require.Error(t, err, one)
	}
}

func TestReportActions_MissingWindow(t *testing.T) {
	container.ReportManager = &fakeReportManager{}
	defer func() { container.ReportManager = nil }()

	actions := []func(*http.Request) (interface{}, error){
		ReportOverviewAction,
		ReportTimeSeriesAction,
		ReportRankingsAction,
		ReportDistributionAction,
		ReportLogsAction,
	}
	for _, action := range actions {
		req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/overview?start=1782345600", nil)
		_, err := action(req)
		require.Error(t, err)
	}
}

func TestReportActions_ManagerNotAssembled(t *testing.T) {
	container.ReportManager = nil

	req := httptest.NewRequest(http.MethodGet, "/open-api/v1/report/overview?"+testQuery, nil)
	_, err := ReportOverviewAction(req)
	require.Error(t, err)
}
