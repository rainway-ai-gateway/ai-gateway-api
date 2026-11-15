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
	"net/http"

	"github.com/rainway-ai-gateway/ai-gateway-api/stateful/container"
)

// AccessLogConfigGetAction returns the current access log config (header
// whitelists in canonical form). An unconfigured state returns empty arrays
// (the BFE default: collect nothing), not 404.
func AccessLogConfigGetAction(req *http.Request) (interface{}, error) {
	return container.AccessLogManager.Get(req.Context())
}
