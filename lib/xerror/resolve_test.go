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

package xerror

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDaoErrorHidesDriverDetails(t *testing.T) {
	err := WrapDaoError(errors.New("Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction"))
	rr := Resolve(err)
	require.NotNil(t, rr)

	assert.Equal(t, 500, rr.ErrNo)
	assert.Equal(t, "Database Exception", rr.Type)
	assert.Equal(t, "database operation failed", rr.Msg)
	assert.NotContains(t, rr.Msg, "1213")

	// Details stay available for logs.
	assert.Contains(t, rr.FullMsg(), "1213")
}

func TestResolveConflictError(t *testing.T) {
	rr := Resolve(WrapConflictErrorWithMsg("collection is being replaced concurrently, retry later"))
	require.NotNil(t, rr)

	assert.Equal(t, 409, rr.ErrNo)
	assert.Equal(t, "Conflict", rr.Type)
	assert.Equal(t, "collection is being replaced concurrently, retry later", rr.Msg)
}

func TestResolveParamErrorKeepsMessage(t *testing.T) {
	rr := Resolve(WrapParamErrorWithMsg("quota must be >= 0"))
	require.NotNil(t, rr)

	assert.Equal(t, 422, rr.ErrNo)
	assert.Equal(t, "Param Illegal", rr.Type)
	assert.Equal(t, "quota must be >= 0", rr.Msg)
}
