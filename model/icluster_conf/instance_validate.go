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

package icluster_conf

import (
	"fmt"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/validate"
	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
)

const (
	MaxInstanceNameLength = 128
)

// ValidateInstance validates a single instance (used by cluster instance
// pools). Moved from lib/validate (issue #321) so that model/iprovider can
// reuse validate.Hostname without an import cycle.
func ValidateInstance(inst Instance) error {
	if inst.Name != "" {
		if len(inst.Name) < 1 || len(inst.Name) > MaxInstanceNameLength {
			return xerror.WrapParamErrorWithMsg("instance name length must be between 1 and %d", MaxInstanceNameLength)
		}
	}
	if err := validate.Hostname(inst.Addr); err != nil {
		return xerror.WrapParamErrorWithMsg("instance addr: %v", err)
	}
	if err := validate.Port(inst.Port); err != nil {
		return xerror.WrapParamErrorWithMsg("instance port: %v", err)
	}
	if inst.Weight < 0 || inst.Weight > 100 {
		return xerror.WrapParamErrorWithMsg("instance weight must be between 0 and 100")
	}
	return nil
}

// ValidateInstancePool validates a list of instances.
func ValidateInstancePool(instances []Instance) error {
	if len(instances) == 0 {
		return xerror.WrapParamErrorWithMsg("instance_pool is required")
	}
	nameSet := map[string]struct{}{}
	comboSet := map[string]struct{}{}
	hasPositiveWeight := false
	for i, inst := range instances {
		if err := ValidateInstance(inst); err != nil {
			return xerror.WrapParamErrorWithMsg("instance_pool[%d]: %v", i, err)
		}
		if inst.Weight > 0 {
			hasPositiveWeight = true
		}
		if inst.Name != "" {
			if _, ok := nameSet[inst.Name]; ok {
				return xerror.WrapParamErrorWithMsg("duplicate instance name: %s", inst.Name)
			}
			nameSet[inst.Name] = struct{}{}
		}
		key := fmt.Sprintf("%s|%s|%d", inst.Name, inst.Addr, inst.Port)
		if _, ok := comboSet[key]; ok {
			return xerror.WrapParamErrorWithMsg("duplicate instance (name=%s, addr=%s, port=%d)", inst.Name, inst.Addr, inst.Port)
		}
		comboSet[key] = struct{}{}
	}
	if !hasPositiveWeight {
		return xerror.WrapParamErrorWithMsg("instance_pool must have at least one instance with weight > 0")
	}
	return nil
}
