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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
)

func TestResolvePagination(t *testing.T) {
	cases := []struct {
		name       string
		page       *int
		pageSize   *int
		wantPage   int
		wantSize   int
	}{
		{"both missing falls back to defaults", nil, nil, DefaultPage, DefaultPageSize},
		{"page only", lib.PInt(2), nil, 2, DefaultPageSize},
		{"page_size only", nil, lib.PInt(5), DefaultPage, 5},
		{"explicit values kept", lib.PInt(3), lib.PInt(7), 3, 7},
		{"zero values fall back to defaults", lib.PInt(0), lib.PInt(0), DefaultPage, DefaultPageSize},
		{"negative page falls back", lib.PInt(-3), nil, DefaultPage, DefaultPageSize},
		{"page_size at cap", nil, lib.PInt(MaxPageSize), DefaultPage, MaxPageSize},
		{"page_size above cap is truncated", nil, lib.PInt(MaxPageSize+1), DefaultPage, MaxPageSize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, pageSize := ResolvePagination(tc.page, tc.pageSize)
			assert.Equal(t, tc.wantPage, page, "page")
			assert.Equal(t, tc.wantSize, pageSize, "page_size")
		})
	}
}

func TestValidatePagination(t *testing.T) {
	require.NoError(t, ValidatePagination(nil, nil))
	require.NoError(t, ValidatePagination(lib.PInt(1), lib.PInt(1)))
	require.NoError(t, ValidatePagination(lib.PInt(9), lib.PInt(MaxPageSize)))

	for _, tc := range []struct {
		name     string
		page     *int
		pageSize *int
		wantMsg  string
	}{
		{"page zero", lib.PInt(0), nil, "page must be > 0, got 0"},
		{"page negative", lib.PInt(-1), nil, "page must be > 0, got -1"},
		{"page_size zero", nil, lib.PInt(0), "page_size must be between 1 and 100, got 0"},
		{"page_size above cap", nil, lib.PInt(101), "page_size must be between 1 and 100, got 101"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePagination(tc.page, tc.pageSize)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}
