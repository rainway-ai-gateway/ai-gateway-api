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

package version_control

import (
	"context"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
	"github.com/rainway-ai-gateway/ai-gateway-api/storage/rdb/internal/dao"
)

const (
	versionLayout       = "20060102150405"
	maxDuplicateRetries = 3

	// dupRetrySavepoint bounds the shared next-key lock lifetime of a
	// failed version INSERT (see the retry loop).
	dupRetrySavepoint = "vc_version_dup_retry"
)

var _ iversion_control.VersionControlStorager = &VersionControlStorager{}

type VersionControlStorager struct {
	dbCtxFactory lib.DBContextFactory
}

func NewVersionControllerStorage(dbCtxFactory lib.DBContextFactory) *VersionControlStorager {
	return &VersionControlStorager{
		dbCtxFactory: dbCtxFactory,
	}
}

func (vcs *VersionControlStorager) UpsertConfigLastExportedVersion(ctx context.Context, css *iversion_control.ExportData) (string, error) {
	dbCtx, err := vcs.dbCtxFactory(ctx)
	if err != nil {
		return "", err
	}

	latest, err := latestConfigVersion(dbCtx, css.Topic)
	if err != nil {
		return "", err
	}

	if latest != nil && latest.DataSign == css.DataSignWithoutVersion {
		return latest.Version, nil
	}

	version := iversion_control.Version(time.Now())
	if latest != nil {
		version = bumpVersion(latest.Version, version)
	}

	// Under MySQL's REPEATABLE READ, a failed INSERT's duplicate check
	// leaves a shared next-key lock held to transaction end, and the
	// exclusive pin below would deadlock concurrent losers against each
	// other; a savepoint bounds that lock's lifetime (issue #318). Outside
	// a transaction (autocommit) the lock dies with the statement anyway,
	// and non-MySQL drivers have different lock semantics, so the
	// savepoint dance is MySQL-in-transaction only.
	useSavepoint := dbCtx.InTx() && dao.IsMySQL(dbCtx)

	for i := 0; i < maxDuplicateRetries; i++ {
		if useSavepoint {
			if _, err = dbCtx.Execer().ExecContext(dbCtx,
				"SAVEPOINT "+dupRetrySavepoint); err != nil {
				return "", err
			}
		}

		_, err = dao.TConfigVersionCreate(dbCtx, &dao.TConfigVersionParam{
			Name:     &css.Topic,
			DataSign: &css.DataSignWithoutVersion,
			Version:  &version,
		})
		if err == nil {
			if useSavepoint {
				if _, err = dbCtx.Execer().ExecContext(dbCtx,
					"RELEASE SAVEPOINT "+dupRetrySavepoint); err != nil {
					return "", err
				}
			}
			return version, nil
		}
		if !lib.DuplicateEntryError(err) {
			return "", err
		}

		// Drop the failed insert and, on MySQL, its shared locks.
		if useSavepoint {
			if _, err = dbCtx.Execer().ExecContext(dbCtx,
				"ROLLBACK TO SAVEPOINT "+dupRetrySavepoint); err != nil {
				return "", err
			}
		}

		// A concurrent export inserted the same version first: pin the row
		// we collided with and either reuse its version (same content,
		// design line 421) or bump strictly past it, instead of failing
		// the whole export with a duplicate key error (issues #142, #318).
		//
		// The pin is a current read (SELECT ... FOR UPDATE on MySQL):
		// equality on the unique key takes a record lock only and sees the
		// winner's committed row instead of this transaction's stale
		// REPEATABLE READ snapshot — a plain re-read would make
		// bumpVersion return the already-attempted version and spin on the
		// same duplicate key.
		winner, err := dao.TConfigVersionOneForUpdate(dbCtx, css.Topic, version)
		if err != nil {
			return "", err
		}
		if winner == nil {
			return "", err
		}
		if winner.DataSign == css.DataSignWithoutVersion {
			return winner.Version, nil
		}
		version = bumpVersion(winner.Version, version)
	}

	return "", err
}

func latestConfigVersion(dbCtx lib.DBContexter, topic string) (*dao.TConfigVersion, error) {
	return dao.TConfigVersionOne(dbCtx, &dao.TConfigVersionParam{
		Name:    &topic,
		OrderBy: lib.PString("version DESC, id DESC"),
	})
}

// bumpVersion returns latest+1s when it is not before candidate, so the
// result is strictly greater than every existing version of the topic even
// when several config changes are exported within the same wall-clock second.
func bumpVersion(latest string, candidate string) string {
	lt, err := time.ParseInLocation(versionLayout, latest, time.Local)
	if err != nil {
		return candidate
	}
	next := iversion_control.Version(lt.Add(time.Second))
	if candidate < next {
		return next
	}
	return candidate
}
