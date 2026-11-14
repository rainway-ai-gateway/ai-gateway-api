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

package ireport

import (
	"context"
	"time"

	"github.com/go-playground/validator/v10"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// Parameter calibration constants shared by the report endpoints.
const (
	// MaxWindow is the upper bound of a report query window (7 days,
	// aligned with the detail retention period).
	MaxWindow = 7 * 24 * time.Hour

	// DefaultRankLimit / MaxRankLimit bound the rankings size.
	DefaultRankLimit = 10
	MaxRankLimit     = 50

	// DefaultPage / DefaultPageSize / MaxPageSize bound the log paging.
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100

	// MaxKeywordLength bounds the err_msg fuzzy keyword.
	MaxKeywordLength = 128
)

// BaseQuery carries the parameters shared by all report queries.
type BaseQuery struct {
	Start       time.Time `validate:"required"`
	End         time.Time `validate:"required"`
	Models      []string
	ApikeyIDs   []string
	Providers   []string
	Hosts       []string
	Stream      *bool
	StatusCodes []int
}

func (q *BaseQuery) filter() *Filter {
	return &Filter{
		Start:       q.Start,
		End:         q.End,
		Models:      q.Models,
		ApikeyIDs:   q.ApikeyIDs,
		Providers:   q.Providers,
		Hosts:       q.Hosts,
		Stream:      q.Stream,
		StatusCodes: q.StatusCodes,
	}
}

// OverviewQuery is the parameter of ReportManager.Overview.
type OverviewQuery struct {
	BaseQuery
}

// TimeSeriesQuery is the parameter of ReportManager.TimeSeries. Dimension is
// optional: empty keeps the plain per-time series; otherwise the series is
// split per dimension value (only the TimeSeriesDimensions set is accepted).
type TimeSeriesQuery struct {
	BaseQuery
	Metric    string `validate:"required"`
	Dimension string
}

// RankingsQuery is the parameter of ReportManager.Rankings.
type RankingsQuery struct {
	BaseQuery
	Dimension string `validate:"required"`
	Limit     int
}

// DistributionQuery is the parameter of ReportManager.Distribution.
type DistributionQuery struct {
	BaseQuery
	Dimension string `validate:"required"`
}

// LogsQuery is the parameter of ReportManager.Logs.
type LogsQuery struct {
	BaseQuery
	RequestedModels []string
	ErrOnly         bool
	Keyword         string
	CacheStatus     *string
	MirrorHit       *bool
	IntentQuestion  *string
	IntentAnswer    *string
	IntentSource    *string
	PriceWarn       *string // exact match on ai_price_warn (missing-price shape, issue #1479)
	Page            int
	PageSize        int
}

// ReportManagerInterface is the full interface exposed to the container.
type ReportManagerInterface interface {
	Overview(ctx context.Context, query *OverviewQuery) (*OverviewResult, error)
	TimeSeries(ctx context.Context, query *TimeSeriesQuery) ([]*MetricPoint, error)
	Rankings(ctx context.Context, query *RankingsQuery) ([]*RankingItem, error)
	Distribution(ctx context.Context, query *DistributionQuery) ([]*DistItem, error)
	Logs(ctx context.Context, query *LogsQuery) (*LogQueryResult, error)
}

// ReportManager validates report query parameters, calibrates buckets and
// delegates to the ReportStorager. It contains no SQL.
type ReportManager struct {
	storager ReportStorager
	validate *validator.Validate
}

// NewReportManager creates a new ReportManager.
func NewReportManager(storager ReportStorager) *ReportManager {
	return &ReportManager{
		storager: storager,
		validate: validator.New(),
	}
}

var _ ReportManagerInterface = (*ReportManager)(nil)

// Overview implements ReportManagerInterface.
func (m *ReportManager) Overview(ctx context.Context, query *OverviewQuery) (*OverviewResult, error) {
	if query == nil {
		return nil, xerror.WrapParamErrorWithMsg("%s", "query is nil")
	}
	if err := m.checkBase(&query.BaseQuery); err != nil {
		return nil, err
	}
	return m.storager.Overview(ctx, query.filter())
}

// TimeSeries implements ReportManagerInterface. The bucket width is
// computed from the window (<=6h -> 60s, <=3d -> 300s, <=7d -> 1800s).
func (m *ReportManager) TimeSeries(ctx context.Context, query *TimeSeriesQuery) ([]*MetricPoint, error) {
	if query == nil {
		return nil, xerror.WrapParamErrorWithMsg("%s", "query is nil")
	}
	if err := m.checkBase(&query.BaseQuery); err != nil {
		return nil, err
	}
	if !TimeSeriesMetrics[query.Metric] {
		return nil, xerror.WrapParamErrorWithMsg("invalid metric: %s", query.Metric)
	}
	if query.Dimension != "" {
		if !TimeSeriesDimensions[query.Dimension] {
			return nil, xerror.WrapParamErrorWithMsg("invalid dimension: %s", query.Dimension)
		}
		if err := m.checkDimensionSupported(query.Dimension); err != nil {
			return nil, err
		}
	}
	bucket := BucketSeconds(query.End.Sub(query.Start))
	return m.storager.TimeSeries(ctx, query.Metric, query.Dimension, query.filter(), bucket)
}

// Rankings implements ReportManagerInterface.
func (m *ReportManager) Rankings(ctx context.Context, query *RankingsQuery) ([]*RankingItem, error) {
	if query == nil {
		return nil, xerror.WrapParamErrorWithMsg("%s", "query is nil")
	}
	if err := m.checkBase(&query.BaseQuery); err != nil {
		return nil, err
	}
	if !RankingDimensions[query.Dimension] {
		return nil, xerror.WrapParamErrorWithMsg("invalid dimension: %s", query.Dimension)
	}
	if err := m.checkDimensionSupported(query.Dimension); err != nil {
		return nil, err
	}
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultRankLimit
	}
	if limit > MaxRankLimit {
		limit = MaxRankLimit
	}
	return m.storager.Rankings(ctx, query.Dimension, query.filter(), limit)
}

// Distribution implements ReportManagerInterface.
func (m *ReportManager) Distribution(ctx context.Context, query *DistributionQuery) ([]*DistItem, error) {
	if query == nil {
		return nil, xerror.WrapParamErrorWithMsg("%s", "query is nil")
	}
	if err := m.checkBase(&query.BaseQuery); err != nil {
		return nil, err
	}
	if !DistributionDimensions[query.Dimension] {
		return nil, xerror.WrapParamErrorWithMsg("invalid dimension: %s", query.Dimension)
	}
	if err := m.checkDimensionSupported(query.Dimension); err != nil {
		return nil, err
	}
	return m.storager.Distribution(ctx, query.Dimension, query.filter())
}

// Logs implements ReportManagerInterface.
func (m *ReportManager) Logs(ctx context.Context, query *LogsQuery) (*LogQueryResult, error) {
	if query == nil {
		return nil, xerror.WrapParamErrorWithMsg("%s", "query is nil")
	}
	if err := m.checkBase(&query.BaseQuery); err != nil {
		return nil, err
	}
	if len(query.Keyword) > MaxKeywordLength {
		return nil, xerror.WrapParamErrorWithMsg("keyword length exceeds %d", MaxKeywordLength)
	}

	page := query.Page
	if page <= 0 {
		page = DefaultPage
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	return m.storager.Logs(ctx, &LogFilter{
		Filter:          *query.filter(),
		RequestedModels: query.RequestedModels,
		ErrOnly:         query.ErrOnly,
		Keyword:         query.Keyword,
		CacheStatus:     query.CacheStatus,
		MirrorHit:       query.MirrorHit,
		IntentQuestion:  query.IntentQuestion,
		IntentAnswer:    query.IntentAnswer,
		IntentSource:    query.IntentSource,
		Page:            page,
		PageSize:        pageSize,
	})
}

// checkBase validates the shared window and filter parameters.
func (m *ReportManager) checkBase(query *BaseQuery) error {
	if err := m.validate.Struct(query); err != nil {
		return xerror.WrapParamError(err)
	}
	if !query.End.After(query.Start) {
		return xerror.WrapParamErrorWithMsg("%s", "end must be greater than start")
	}
	if query.End.Sub(query.Start) > MaxWindow {
		return xerror.WrapParamErrorWithMsg("query window exceeds %v", MaxWindow)
	}
	return nil
}

// checkDimensionSupported appends the backend capability check to the
// dimension whitelist validation: a dimension the configured backend does
// not support is a parameter error (422), never a silently empty report.
func (m *ReportManager) checkDimensionSupported(dimension string) error {
	caps := m.storager.Capabilities()
	if caps == nil {
		return nil
	}
	for _, one := range caps.SupportedDimensions {
		if one == dimension {
			return nil
		}
	}
	backend := caps.Backend
	if backend == "" {
		backend = "current"
	}
	return xerror.WrapParamErrorWithMsg(
		"dimension %s not supported by %s backend",
		dimension, backend)
}
