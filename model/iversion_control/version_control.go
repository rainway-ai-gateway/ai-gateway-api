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

package iversion_control

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
)

func Version(t time.Time) string {
	return t.Format("20060102150405")
}

var ZeroVersion = Version(time.Time{})

func Sign(data interface{}) (string, error) {
	bs, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(bs)), nil
}

type ExportData struct {
	Topic              string
	DataWithoutVersion VersionValuable

	version                string
	DataSignWithoutVersion string
}

type VersionValuable interface {
	UpdateVersion(version string) error
}

func (cs *ExportData) Version() string {
	return cs.version
}

func (cs *ExportData) CalculateVersion() (string, error) {
	cs.version = Version(time.Now())
	return cs.version, nil
}

type VersionControlStorager interface {

	// UpsertConfigLastExportedVersion will got last export data
	// if config changed, create new version and return
	// if not, return last version
	UpsertConfigLastExportedVersion(ctx context.Context, css *ExportData) (string, error)
}

type VersionControlManager struct {
	storager VersionControlStorager
	txn      itxn.TxnStorager
}

func NewVersionControllerManager(txn itxn.TxnStorager, storager VersionControlStorager) *VersionControlManager {
	return &VersionControlManager{
		storager: storager,
		txn:      txn,
	}
}

type ConfigGenerator func(ctx context.Context) (*ExportData, error)

func (vcm *VersionControlManager) ExportConfig(ctx context.Context, configTopic string,
	generaotr ConfigGenerator) (lrv *ExportData, err error) {

	err = itxn.ExecuteWithRetry(ctx, vcm.txn, func(ctx context.Context) error {
		lrv, err = generaotr(ctx)
		if err != nil {
			return err
		}

		if err = lrv.DataWithoutVersion.UpdateVersion(ZeroVersion); err != nil {
			return err
		}

		lrv.DataSignWithoutVersion, err = Sign(lrv.DataWithoutVersion)
		if err != nil {
			return err
		}

		lrv.version, err = vcm.storager.UpsertConfigLastExportedVersion(ctx, lrv)
		if err != nil {
			// Duplicate-key retries exhausted (theoretical under the
			// FOR UPDATE pin, see issue #318): surface a distinguishable
			// 409 Conflict instead of an indistinguishable 500. Any other
			// error keeps its original semantics.
			if lib.DuplicateEntryError(err) {
				return xerror.WrapConflictErrorWithMsg("export version conflict after retries: %v", err)
			}
			return err
		}

		return lrv.DataWithoutVersion.UpdateVersion(lrv.version)
	})

	return
}
