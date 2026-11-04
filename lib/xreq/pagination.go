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

package xreq

import (
	"net/url"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

// Pagination defaults shared by list endpoints. See
// design-docs/api-define/OpenAPI接口定义/00-common.md#通用Query参数.
const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ResolvePagination returns the effective page and page_size of a list request.
//
// The defaults apply whenever the value is missing or non-positive, so callers
// must not gate this on "did the request carry pagination parameters" (that
// mistake made the no-parameter list response report page=0/page_size=0, issue
// #303). page_size is capped at MaxPageSize.
//
// The resolved values must also be pushed down to the storager filter: the DAO
// layer only emits LIMIT/OFFSET when both Page and PageSize are set.
func ResolvePagination(page, pageSize *int) (int, int) {
	effectivePage := DefaultPage
	if page != nil && *page > 0 {
		effectivePage = *page
	}

	effectivePageSize := DefaultPageSize
	if pageSize != nil && *pageSize > 0 {
		effectivePageSize = *pageSize
	}
	if effectivePageSize > MaxPageSize {
		effectivePageSize = MaxPageSize
	}

	return effectivePage, effectivePageSize
}

// ValidatePagination enforces the strict pagination contract used by endpoints
// whose API definition requires 422 on invalid values (e.g. /api-keys).
//
// Endpoints that clamp instead (e.g. /entities, /entity-types) must not call
// this: for them ResolvePagination alone defines the behaviour.
func ValidatePagination(page, pageSize *int) error {
	if page != nil && *page <= 0 {
		return xerror.WrapParamErrorWithMsg("page must be > 0, got %d", *page)
	}
	if pageSize != nil && (*pageSize <= 0 || *pageSize > MaxPageSize) {
		return xerror.WrapParamErrorWithMsg("page_size must be between 1 and %d, got %d", MaxPageSize, *pageSize)
	}
	return nil
}

// OptionalPagination resolves pagination for list endpoints whose contract is
// "no pagination parameters means return everything" (/providers, /entities,
// /entity-types).
//
// The decision is taken from the raw query values, not from the bound pointers:
// BindForm turns "?page=" into a non-nil pointer holding 0, which would
// otherwise look like an explicit (invalid) page request.
//
// paginate == false means the caller must not push paging down and should
// report page=DefaultPage and page_size=total after fetching.
func OptionalPagination(values url.Values, page, pageSize *int) (int, int, bool) {
	if values.Get("page") == "" && values.Get("page_size") == "" {
		return DefaultPage, 0, false
	}

	effectivePage, effectivePageSize := ResolvePagination(page, pageSize)
	return effectivePage, effectivePageSize, true
}
