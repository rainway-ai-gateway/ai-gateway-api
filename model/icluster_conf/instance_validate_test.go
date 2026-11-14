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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInstancePool(t *testing.T) {
	instances := []Instance{
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 100},
	}
	assert.NoError(t, ValidateInstancePool(instances))

	instances[0].Weight = 0
	assert.Error(t, ValidateInstancePool(instances))

	// duplicate name
	instances[0].Weight = 50
	assert.Error(t, ValidateInstancePool([]Instance{
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Name: "backend-1", Addr: "10.0.0.2", Port: 8080, Weight: 50},
	}))

	// duplicate (name, addr, port)
	assert.Error(t, ValidateInstancePool([]Instance{
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Name: "backend-1", Addr: "10.0.0.1", Port: 8080, Weight: 50},
	}))

	// same addr with empty name but different ports is allowed
	assert.NoError(t, ValidateInstancePool([]Instance{
		{Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Addr: "10.0.0.1", Port: 8081, Weight: 50},
	}))

	// same addr and port with empty name is not allowed
	assert.Error(t, ValidateInstancePool([]Instance{
		{Addr: "10.0.0.1", Port: 8080, Weight: 50},
		{Addr: "10.0.0.1", Port: 8080, Weight: 50},
	}))
}

func TestValidateInstanceHostname(t *testing.T) {
	// invalid addr must be rejected (behavior preserved after the move from
	// lib/validate, issue #321)
	assert.Error(t, ValidateInstance(Instance{Addr: "-bad", Port: 8080, Weight: 100}))
	assert.Error(t, ValidateInstance(Instance{Addr: "bad-", Port: 8080, Weight: 100}))
	assert.NoError(t, ValidateInstance(Instance{Addr: "api.example.com", Port: 8080, Weight: 100}))
	assert.NoError(t, ValidateInstance(Instance{Addr: "1.2.3.4", Port: 8080, Weight: 100}))
}
