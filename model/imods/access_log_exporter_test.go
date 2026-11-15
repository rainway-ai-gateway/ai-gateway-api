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

package imods

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/access_log"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
)

// fakeAccessLogStorager implements access_log.AccessLogConfStorager.
type fakeAccessLogStorager struct {
	fetchFn func(ctx context.Context) (*access_log.AccessLogConf, error)
}

func (s *fakeAccessLogStorager) Fetch(ctx context.Context) (*access_log.AccessLogConf, error) {
	if s.fetchFn != nil {
		return s.fetchFn(ctx)
	}
	return nil, nil
}

func (s *fakeAccessLogStorager) Upsert(ctx context.Context, config *access_log.AccessLogConf) error {
	return nil
}

var _ access_log.AccessLogConfStorager = (*fakeAccessLogStorager)(nil)

func newTestAccessLogExporter(upsertFn func(ctx context.Context, css *iversion_control.ExportData) (string, error),
	storager access_log.AccessLogConfStorager) *AccessLogExporter {
	return NewAccessLogExporter(
		storager,
		iversion_control.NewVersionControllerManager(&fakeTxn{}, &fakeVersionControlStorager{upsertFn: upsertFn}))
}

func TestAccessLogExporterUnconfiguredExportsEmptyWhitelists(t *testing.T) {
	exporter := newTestAccessLogExporter(nil, &fakeAccessLogStorager{})

	conf, err := exporter.ConfigExport(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, conf)
	assert.Equal(t, []string{}, conf.AccessHeader.ReqHeaders)
	assert.Equal(t, []string{}, conf.AccessHeader.ResHeaders)
	assert.NotEmpty(t, conf.Version)
}

func TestAccessLogExporterConfigured(t *testing.T) {
	storager := &fakeAccessLogStorager{
		fetchFn: func(ctx context.Context) (*access_log.AccessLogConf, error) {
			return &access_log.AccessLogConf{
				ReqHeaders: `["Business_type","X-Request-Id"]`,
				ResHeaders: `["X-Trace-Id"]`,
			}, nil
		},
	}
	exporter := newTestAccessLogExporter(nil, storager)

	conf, err := exporter.ConfigExport(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, []string{"Business_type", "X-Request-Id"}, conf.AccessHeader.ReqHeaders)
	assert.Equal(t, []string{"X-Trace-Id"}, conf.AccessHeader.ResHeaders)
}

func TestAccessLogExporterVersionUnchangedReturnsNil(t *testing.T) {
	var signed []byte
	exporter := newTestAccessLogExporter(
		func(ctx context.Context, css *iversion_control.ExportData) (string, error) {
			signed, _ = json.Marshal(css.DataWithoutVersion)
			return "v-fixed", nil
		},
		&fakeAccessLogStorager{})

	conf, err := exporter.ConfigExport(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, conf)
	assert.Equal(t, "v-fixed", conf.Version)
	assert.NotEmpty(t, signed)

	// same version asked again -> nil (conf-agent skips)
	conf, err = exporter.ConfigExport(context.Background(), "v-fixed")
	require.NoError(t, err)
	assert.Nil(t, conf)
}

func TestAccessLogExporterJSONContractExact(t *testing.T) {
	storager := &fakeAccessLogStorager{
		fetchFn: func(ctx context.Context) (*access_log.AccessLogConf, error) {
			return &access_log.AccessLogConf{
				ReqHeaders: `["Business_type"]`,
				ResHeaders: `[]`,
			}, nil
		},
	}
	exporter := newTestAccessLogExporter(nil, storager)

	conf, err := exporter.ConfigExport(context.Background(), "")
	require.NoError(t, err)
	conf.Version = "20261115101530"

	raw, err := json.Marshal(conf)
	require.NoError(t, err)
	// the top-level keys must match the BFE mod_access_pb.data loader
	// contract byte for byte
	assert.JSONEq(t, `{
		"Version": "20261115101530",
		"access_header": {
			"req_headers": ["Business_type"],
			"res_headers": []
		}
	}`, string(raw))
}

func TestAccessLogExporterFetchError(t *testing.T) {
	storager := &fakeAccessLogStorager{
		fetchFn: func(ctx context.Context) (*access_log.AccessLogConf, error) {
			return nil, errors.New("db down")
		},
	}
	exporter := newTestAccessLogExporter(nil, storager)

	_, err := exporter.ConfigExport(context.Background(), "")
	require.Error(t, err)
}
