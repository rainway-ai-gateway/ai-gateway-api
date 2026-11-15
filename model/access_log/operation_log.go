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
)

// RecordPutAccessLogConfFailure records a failed update audit for a PUT
// rejected by validation (4xx). It is called from the endpoint layer, which
// validation never passes through Put. Identity is the fixed singleton id
// and the before snapshot is read from storage, so the audit never depends
// on the request body (issue #155 discipline).
func (m *AccessLogManager) RecordPutAccessLogConfFailure(ctx context.Context, param *AccessLogConfigParam, validateErr error) {
	before, _ := m.storager.Fetch(ctx)

	m.recordPutOperation(ctx, accessLogConfToMap(before), accessLogParamToMap(param), validateErr)
}
