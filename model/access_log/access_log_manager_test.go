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

package access_log

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/ioperlog"
)

func newTestManager(storager *fakeAccessLogConfStorager, recorder *fakeOperationLogRecorder) *AccessLogManager {
	m := NewAccessLogManager(&fakeTxn{}, storager)
	if recorder != nil {
		m.SetOperationLogManager(recorder)
	}
	return m
}

func TestManagerGetUnconfiguredRendersEmpty(t *testing.T) {
	m := newTestManager(&fakeAccessLogConfStorager{}, nil)

	param, err := m.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, param)
	assert.Equal(t, []string{}, param.ReqHeaders)
	assert.Equal(t, []string{}, param.ResHeaders)
}

func TestManagerPutCanonicalizesAndPersists(t *testing.T) {
	storager := &fakeAccessLogConfStorager{}
	recorder := &fakeOperationLogRecorder{}
	m := newTestManager(storager, recorder)

	param, err := m.Put(context.Background(), &AccessLogConfigParam{
		ReqHeaders: []string{"business_type", "X-REQUEST-ID"},
		ResHeaders: []string{"x-trace-id"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"Business_type", "X-Request-Id"}, param.ReqHeaders)
	assert.Equal(t, []string{"X-Trace-Id"}, param.ResHeaders)

	// stored as canonical JSON text
	stored, err := storager.Fetch(context.Background())
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, `["Business_type","X-Request-Id"]`, stored.ReqHeaders)
	assert.Equal(t, `["X-Trace-Id"]`, stored.ResHeaders)

	// success audit recorded with the singleton identity
	entry := recorder.last()
	require.NotNil(t, entry)
	assert.Equal(t, string(ioperlog.ActionUpdate), entry.Action)
	assert.Equal(t, string(ioperlog.ResourceTypeAccessLogConf), entry.ResourceType)
	assert.Equal(t, ioperlog.StatusSuccess, entry.Status)
}

func TestManagerPutRejectsInvalidWithoutWrite(t *testing.T) {
	storager := &fakeAccessLogConfStorager{}
	recorder := &fakeOperationLogRecorder{}
	m := newTestManager(storager, recorder)

	_, err := m.Put(context.Background(), &AccessLogConfigParam{
		ReqHeaders: []string{"Authorization"},
	})
	require.Error(t, err)

	stored, fetchErr := storager.Fetch(context.Background())
	require.NoError(t, fetchErr)
	assert.Nil(t, stored, "rejected PUT must not write")

	// the manager itself records no audit for a validation-rejected PUT;
	// the endpoint records it via RecordPutAccessLogConfFailure (the
	// request body must not be trusted for identity, issue #155)
	assert.Nil(t, recorder.last())

	m.RecordPutAccessLogConfFailure(context.Background(),
		&AccessLogConfigParam{ReqHeaders: []string{"Authorization"}}, err)
	entry := recorder.last()
	require.NotNil(t, entry)
	assert.Equal(t, ioperlog.StatusFailed, entry.Status)
	assert.NotEmpty(t, entry.ErrorMsg)
}

func TestManagerPutUpsertFailureRecordsAudit(t *testing.T) {
	storager := &fakeAccessLogConfStorager{
		upsertFn: func(ctx context.Context, config *AccessLogConf) error {
			return errors.New("db down")
		},
	}
	recorder := &fakeOperationLogRecorder{}
	m := newTestManager(storager, recorder)

	_, err := m.Put(context.Background(), &AccessLogConfigParam{
		ReqHeaders: []string{"business_type"},
	})
	require.Error(t, err)

	entry := recorder.last()
	require.NotNil(t, entry)
	assert.Equal(t, ioperlog.StatusFailed, entry.Status)
}

func TestRecordPutAccessLogConfFailure(t *testing.T) {
	storager := &fakeAccessLogConfStorager{}
	recorder := &fakeOperationLogRecorder{}
	m := newTestManager(storager, recorder)

	validateErr := errors.New("invalid")
	m.RecordPutAccessLogConfFailure(context.Background(),
		&AccessLogConfigParam{ReqHeaders: []string{"bad name"}}, validateErr)

	entry := recorder.last()
	require.NotNil(t, entry)
	assert.Equal(t, ioperlog.StatusFailed, entry.Status)
	assert.Equal(t, validateErr.Error(), entry.ErrorMsg)
}

func TestManagerGetFetchErrorPropagates(t *testing.T) {
	storager := &fakeAccessLogConfStorager{
		fetchFn: func(ctx context.Context) (*AccessLogConf, error) {
			return nil, errors.New("db down")
		},
	}
	m := newTestManager(storager, nil)

	_, err := m.Get(context.Background())
	require.Error(t, err)
}

func TestAccessLogConfToParamGuardsBadJSON(t *testing.T) {
	// corrupted storage content renders as empty lists rather than failing:
	// the PUT-time validation is the single gate, storage corruption degrades
	// to the safe default (collect nothing).
	param := accessLogConfToParam(&AccessLogConf{
		ReqHeaders: `{corrupted`,
		ResHeaders: `["X-Trace-Id"]`,
	})
	assert.Equal(t, []string{}, param.ReqHeaders)
	assert.Equal(t, []string{"X-Trace-Id"}, param.ResHeaders)
}
