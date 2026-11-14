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

package product_cluster

import (
	"fmt"
	"net"
	"regexp"

	"github.com/rainway-ai-gateway/ai-gateway-api/model/icluster_conf"
)

// ValidationError describes a single field validation failure.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func checkLLMConfig(llmConfig *icluster_conf.LLMConfig) error {
	return icluster_conf.ValidateLLMConfig(llmConfig)
}

// CheckEPPServer validates EPP server configuration.
func CheckEPPServer(server *icluster_conf.EPPServer) error {
	if server == nil {
		return nil
	}

	// Compatible with api-server behavior:
	// - domain + port mode
	// - endpoints mode
	if (server.Domain == nil || *server.Domain == "") && len(server.Endpoints) == 0 {
		return nil
	}

	if server.Domain != nil && *server.Domain != "" {
		if server.Port == nil {
			return &ValidationError{Field: "port", Message: "Must be set port"}
		}
		if !isValidDomain(*server.Domain) {
			return &ValidationError{Field: "domain", Message: "invalid domain"}
		}
		if *server.Port < 1 || *server.Port > 65535 {
			return &ValidationError{Field: "port", Message: "port must be in 1-65535"}
		}
		return nil
	}

	if len(server.Endpoints) == 0 {
		return &ValidationError{Field: "endpoints", Message: "Must set at least one endpoint"}
	}

	for i, endpoint := range server.Endpoints {
		if endpoint == nil {
			return &ValidationError{Field: fmt.Sprintf("endpoints[%d]", i), Message: "endpoints must not be empty"}
		}

		if endpoint.IP == nil {
			return &ValidationError{Field: fmt.Sprintf("endpoints[%d].ip", i), Message: "Must set ip"}
		}
		if *endpoint.IP == "" {
			return &ValidationError{Field: fmt.Sprintf("endpoints[%d].ip", i), Message: "Must set ip"}
		}

		if !isValidIP(*endpoint.IP) {
			return &ValidationError{Field: fmt.Sprintf("endpoints[%d].ip", i), Message: "invalid ip"}
		}

		if endpoint.Port == nil {
			return &ValidationError{Field: fmt.Sprintf("endpoints[%d].port", i), Message: "Must set port in endpoint"}
		}
		if *endpoint.Port < 1 || *endpoint.Port > 65535 {
			return &ValidationError{Field: fmt.Sprintf("endpoints[%d].port", i), Message: "port must be in 1-65535 in endpoint"}
		}
	}

	return nil
}

// isValidIP checks whether ip is a valid IPv4 or IPv6 address.
func isValidIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil
}

func isValidDomain(domain string) bool {
	domainRegex := regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)
	return domainRegex.MatchString(domain)
}
