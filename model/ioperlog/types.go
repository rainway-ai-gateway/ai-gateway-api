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

package ioperlog

import (
	"context"
	"time"
)

// OperatorType defines the type of operator who performed the operation.
type OperatorType int8

const (
	// OperatorTypeUser indicates a human user.
	OperatorTypeUser OperatorType = 0
	// OperatorTypeToken indicates an API token.
	OperatorTypeToken OperatorType = 1
)

// Operation status constants.
const (
	// StatusSuccess indicates the operation succeeded.
	StatusSuccess int8 = 1
	// StatusFailed indicates the operation failed.
	StatusFailed int8 = 2
)

// ActionType defines the operation action type.
type ActionType string

const (
	// ActionCreate creates a resource.
	ActionCreate ActionType = "create"
	// ActionUpdate updates a resource.
	ActionUpdate ActionType = "update"
	// ActionDelete deletes a resource.
	ActionDelete ActionType = "delete"
	// ActionReset resets a resource (e.g., quota balance).
	ActionReset ActionType = "reset"
	// ActionImport imports resources in batch.
	ActionImport ActionType = "import"
	// ActionBind binds a resource to another.
	ActionBind ActionType = "bind"
	// ActionUnbind unbinds a resource from another.
	ActionUnbind ActionType = "unbind"
)

// ResourceType defines the type of resource being operated on.
type ResourceType string

const (
	ResourceTypeEntity                  ResourceType = "entity"
	ResourceTypeEntityType              ResourceType = "entity_type"
	ResourceTypeAPIKey                  ResourceType = "api_key"
	ResourceTypeProvider                ResourceType = "provider"
	ResourceTypeCluster                 ResourceType = "cluster"
	ResourceTypeRoute                   ResourceType = "route"
	ResourceTypeDomain                  ResourceType = "domain"
	ResourceTypeCertificate             ResourceType = "certificate"
	ResourceTypeQuotaPlan               ResourceType = "quota_plan"
	ResourceTypeRateLimitPolicy         ResourceType = "rate_limit_policy"
	ResourceTypeAICacheRule             ResourceType = "ai_cache_rule"
	ResourceTypeAICacheSemanticSettings ResourceType = "ai_cache_semantic_settings"
	ResourceTypeAIContextRule           ResourceType = "ai_context_rule"
	ResourceTypeAIContextSettings       ResourceType = "ai_context_settings"
	ResourceTypeTrafficMirrorRule       ResourceType = "traffic_mirror_rule"
	ResourceTypeIntentConfig            ResourceType = "intent_config"
	ResourceTypeAccessLogConf           ResourceType = "access_log_conf"
	ResourceTypeModelPrice              ResourceType = "model_price"
	ResourceTypeBatchTask               ResourceType = "batch_task"
	ResourceTypeUser                    ResourceType = "user"
	ResourceTypeToken                   ResourceType = "token"
)

// OperationLogEntry represents a single operation log record.
type OperationLogEntry struct {
	ID               int64
	LogID            string
	OperatorType     OperatorType
	OperatorID       int64
	OperatorName     string
	Action           string
	ResourceType     string
	ResourceID       string
	ResourceName     string
	ResourceParentID string
	Status           int8
	ErrorMsg         string
	ChangeSummary    map[string]interface{}
	RequestPath      string
	RequestMethod    string
	ClientIP         string
	UserAgent        string
	CreatedAt        time.Time
}

// OperationLogFilter defines filter criteria for querying operation logs.
type OperationLogFilter struct {
	LogID            *string
	OperatorType     *OperatorType
	OperatorID       *int64
	OperatorName     *string
	Action           *string
	ResourceType     *string
	ResourceID       *string
	ResourceName     *string
	ResourceParentID *string
	Status           *int8
	StartTime        *time.Time
	EndTime          *time.Time
	Page             *int
	PageSize         *int
}

// OperationLogQueryResult holds the result of a query.
type OperationLogQueryResult struct {
	Total    int64
	Page     int
	PageSize int
	List     []*OperationLogEntry
}

// OperationLogStorager defines storage operations for operation logs.
type OperationLogStorager interface {
	BatchCreate(ctx context.Context, entries []*OperationLogEntry) error
	List(ctx context.Context, filter *OperationLogFilter) ([]*OperationLogEntry, int64, error)
}

// OperationLogRecorder defines the minimal surface used by managers to record logs.
type OperationLogRecorder interface {
	Record(ctx context.Context, entry *OperationLogEntry)
}

// OperationLogManagerInterface is the full interface exposed to the container.
type OperationLogManagerInterface interface {
	OperationLogRecorder
	SetContextExtractor(extractor ContextExtractor)
	QueryLogs(ctx context.Context, filter *OperationLogFilter) (*OperationLogQueryResult, error)
	Close() error
}

// Ensure OperationLogManager implements the interfaces.
var _ OperationLogRecorder = (*OperationLogManager)(nil)
var _ OperationLogManagerInterface = (*OperationLogManager)(nil)
