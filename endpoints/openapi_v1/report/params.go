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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xreq"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/ireport"
	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// commonQuery binds the filter parameters shared by all report endpoints
// (see design-docs/api-define/OpenAPI接口定义/report.md §1.1).
type commonQuery struct {
	Start       *int64 `form:"start" validate:"required"`
	End         *int64 `form:"end" validate:"required"`
	Models      string `form:"models"`
	ApikeyIDs   string `form:"apikey_ids"`
	Providers   string `form:"providers"`
	Hosts       string `form:"hosts"`
	Stream      *bool  `form:"stream"`
	StatusCodes string `form:"status_codes"`
}

func (q *commonQuery) toBaseQuery() (*ireport.BaseQuery, error) {
	base := &ireport.BaseQuery{
		Start:     time.Unix(*q.Start, 0),
		End:       time.Unix(*q.End, 0),
		Models:    splitList(q.Models),
		ApikeyIDs: splitList(q.ApikeyIDs),
		Providers: splitList(q.Providers),
		Hosts:     splitList(q.Hosts),
		Stream:    q.Stream,
	}
	if q.StatusCodes != "" {
		codes, err := parseStatusCodes(q.StatusCodes)
		if err != nil {
			return nil, err
		}
		base.StatusCodes = codes
	}
	return base, nil
}

func bindCommonQuery(req *http.Request) (*commonQuery, error) {
	query := &commonQuery{}
	if err := xreq.BindForm(req, query); err != nil {
		return nil, err
	}
	return query, nil
}

// splitList splits a comma-separated parameter into trimmed, non-empty
// items; nil when nothing remains.
func splitList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	items := make([]string, 0, len(parts))
	for _, one := range parts {
		one = strings.TrimSpace(one)
		if one != "" {
			items = append(items, one)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func parseStatusCodes(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	codes := make([]int, 0, len(parts))
	for _, one := range parts {
		one = strings.TrimSpace(one)
		if one == "" {
			continue
		}
		code, err := strconv.Atoi(one)
		if err != nil {
			return nil, xerror.WrapParamErrorWithMsg("invalid status_codes: %s", raw)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// intentSources is the validation set of the intent_source log filter
// (mirrors the ai_intent_source log field).
var intentSources = map[string]bool{
	"explicit_header": true,
	"classifier":      true,
	"cache":           true,
}

// priceWarnShapes is the validation set of the ai_price_warn log filter
// (mirrors the ai_price_warn log field, issue #1479).
var priceWarnShapes = map[string]bool{
	"fallback":        true,
	"no_entry":        true,
	"no_baseline":     true,
	"reject":          true,
	"subitem_missing": true,
	"no_model_table":  true,
}

// bindLogsFilterParams binds the logs-only exact-match filter parameters
// introduced with the cache/mirror/intent log fields: cache_status,
// mirror_hit, intent_question, intent_answer and intent_source (see
// design-docs modifications/2026-09-27-report-cache-mirror-intent-fields),
// plus price_warn for the missing-price shape (issue #1479).
func bindLogsFilterParams(req *http.Request, logsQuery *ireport.LogsQuery) error {
	if raw := req.Form.Get("cache_status"); raw != "" {
		logsQuery.CacheStatus = &raw
	}
	if raw := req.Form.Get("mirror_hit"); raw != "" {
		value, err := parseBoolParam(raw, "mirror_hit")
		if err != nil {
			return err
		}
		logsQuery.MirrorHit = value
	}
	if raw := req.Form.Get("intent_question"); raw != "" {
		logsQuery.IntentQuestion = &raw
	}
	if raw := req.Form.Get("intent_answer"); raw != "" {
		logsQuery.IntentAnswer = &raw
	}
	if raw := req.Form.Get("intent_source"); raw != "" {
		if !intentSources[raw] {
			return xerror.WrapParamErrorWithMsg("invalid intent_source: %s", raw)
		}
		logsQuery.IntentSource = &raw
	}
	if raw := req.Form.Get("price_warn"); raw != "" {
		if !priceWarnShapes[raw] {
			return xerror.WrapParamErrorWithMsg("invalid price_warn: %s", raw)
		}
		logsQuery.PriceWarn = &raw
	}
	return nil
}

// reportManager resolves the assembled report manager; when the module is
// not assembled ([Report] missing) the routes are not registered at all,
// this guard only covers direct invocations.
func reportManager(req *http.Request) (ireport.ReportManagerInterface, error) {
	manager := container.ReportManager
	if manager == nil {
		return nil, xerror.WrapRecordNotExist("report module")
	}
	return manager, nil
}
