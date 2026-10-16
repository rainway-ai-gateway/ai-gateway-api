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

// Package epp_pool implements the EPP instance pool, the cluster to
// instance-group assignment model and the epp_data export generator.
// See design-docs/modifications/2026-09-08-epp-scheduling-integration/.
package epp_pool

import (
	"context"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/rainway-ai-gateway/ai-gateway-api/lib/xerror"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/itxn"
	"github.com/rainway-ai-gateway/ai-gateway-api/model/iversion_control"
)

const (
	// DefaultEPPInstancePoolName is the default singleton pool name used when
	// the deployment does not configure one (see RunTime.DefaultEPPInstancePoolName).
	DefaultEPPInstancePoolName = "EPP.pool"

	// DefaultReconcileInterval is the default period of the assignment reconciler.
	DefaultReconcileInterval = 30 * time.Second
)

// InstanceParam describes one EPP instance of the pool.
type InstanceParam struct {
	ID   string `json:"id"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// GroupName is an internal storage detail; it never appears in the
	// serialized API views of /epp-pool and /epp-assignments.
	GroupName string `json:"-"`
}

// InstanceFilter filters instance queries.
type InstanceFilter struct {
	ID        *string
	GroupName *string
}

// InstanceGroup is a named group of EPP instances (primary/standby unit).
type InstanceGroup struct {
	Name      string           `json:"name"`
	Instances []*InstanceParam `json:"instances"`
}

// EppPool is the singleton EPP instance pool.
type EppPool struct {
	Name   string           `json:"name"`
	Groups []*InstanceGroup `json:"groups"`
}

// AssignmentParam is one cluster to instance-group assignment (primary only;
// standby is expanded at read time from the instances of the same group).
type AssignmentParam struct {
	Cluster           string `json:"cluster"`
	GroupName         string `json:"group_name"`
	PrimaryInstanceID string `json:"primary_instance_id"`
}

// EPPClusterInfo describes one cluster whose balance_mode is EPP, as seen by
// the epp_pool domain. EppConfigJSON is the raw simplified epp_config stored
// in clusters.epp_config (empty string if not set).
type EPPClusterInfo struct {
	Name          string
	EppConfigJSON string
}

// EPPClusterSource provides the EPP-mode clusters. It is implemented by the
// cluster model (icluster_conf) and injected at wiring time.
type EPPClusterSource interface {
	// FetchEPPClusters returns all clusters with balance_mode=EPP.
	FetchEPPClusters(ctx context.Context) ([]*EPPClusterInfo, error)
}

// EppPoolStorager defines the persistence operations of the EPP instance pool
// and the cluster assignments.
type EppPoolStorager interface {
	// FetchInstanceList returns all instances matching the filter.
	FetchInstanceList(ctx context.Context, filter *InstanceFilter) ([]*InstanceParam, error)
	// CreateInstances batch creates instances.
	CreateInstances(ctx context.Context, params []*InstanceParam) (int64, error)
	// DeleteInstances deletes instances matching the filter.
	DeleteInstances(ctx context.Context, filter *InstanceFilter) (int64, error)

	// FetchAssignment returns the assignment of one cluster, nil if absent.
	FetchAssignment(ctx context.Context, cluster string) (*AssignmentParam, error)
	// FetchAssignmentList returns all assignments.
	FetchAssignmentList(ctx context.Context) ([]*AssignmentParam, error)
	// UpsertAssignment inserts or replaces the assignment of one cluster.
	UpsertAssignment(ctx context.Context, param *AssignmentParam) error
	// DeleteAssignment deletes the assignment of one cluster.
	DeleteAssignment(ctx context.Context, cluster string) error
}

// ManagerOptions carries deployment-shape tunables of EppPoolManager.
type ManagerOptions struct {
	// PoolName is the singleton pool name, default DefaultEPPInstancePoolName.
	PoolName string
	// ReconcileInterval is the period of the assignment reconciler,
	// default DefaultReconcileInterval.
	ReconcileInterval time.Duration
}

func (o *ManagerOptions) withDefaults() {
	if o.PoolName == "" {
		o.PoolName = DefaultEPPInstancePoolName
	}
	if o.ReconcileInterval <= 0 {
		o.ReconcileInterval = DefaultReconcileInterval
	}
}

// EppPoolManager manages the EPP instance pool, the cluster assignments and
// the epp_data export.
type EppPoolManager struct {
	txn                   itxn.TxnStorager
	storager              EppPoolStorager
	clusterSource         EPPClusterSource
	versionControlManager *iversion_control.VersionControlManager

	poolName          string
	reconcileInterval time.Duration

	reconciler *EppReconciler
}

// NewEppPoolManager creates the manager. clusterSource may be nil: assignment
// listing/allocation that needs the EPP cluster list is skipped in that case.
// versionControlManager may be nil: epp_data export triggering is skipped then.
func NewEppPoolManager(
	txn itxn.TxnStorager,
	storager EppPoolStorager,
	clusterSource EPPClusterSource,
	versionControlManager *iversion_control.VersionControlManager,
	options *ManagerOptions,
) *EppPoolManager {
	opts := ManagerOptions{}
	if options != nil {
		opts = *options
	}
	opts.withDefaults()

	m := &EppPoolManager{
		txn:                   txn,
		storager:              storager,
		clusterSource:         clusterSource,
		versionControlManager: versionControlManager,
		poolName:              opts.PoolName,
		reconcileInterval:     opts.ReconcileInterval,
	}
	m.reconciler = NewEppReconciler(m.reconcileInterval, m.Reconcile)

	return m
}

// PoolName returns the configured singleton pool name.
func (m *EppPoolManager) PoolName() string {
	return m.poolName
}

// StartReconciler starts the periodic assignment reconciler.
func (m *EppPoolManager) StartReconciler() {
	m.reconciler.Start()
}

// StopReconciler stops the periodic assignment reconciler.
func (m *EppPoolManager) StopReconciler() {
	m.reconciler.Stop()
}

// ReconcileInterval returns the configured reconciler period.
func (m *EppPoolManager) ReconcileInterval() time.Duration {
	return m.reconcileInterval
}

// GetPool returns the full instance pool (all groups with their instances).
// Per epp-pool.md §2.1 the pool is a singleton created by the first PATCH:
// when no instance has ever been registered the pool does not exist and a
// record-not-exist error is returned (404 semantics at the API layer).
func (m *EppPoolManager) GetPool(ctx context.Context) (*EppPool, error) {
	instances, err := m.storager.FetchInstanceList(ctx, &InstanceFilter{})
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, xerror.WrapRecordNotExist("epp pool")
	}

	return buildPool(m.poolName, instances), nil
}

// PatchPool validates and fully replaces the instance pool inside a single
// transaction, then repairs dangling assignments (see RepairDangling).
func (m *EppPoolManager) PatchPool(ctx context.Context, groups []*InstanceGroup) (*EppPool, error) {
	flat, err := m.validateGroups(groups)
	if err != nil {
		return nil, err
	}

	err = m.txn.AtomExecute(ctx, func(ctx context.Context) error {
		if _, err := m.storager.DeleteInstances(ctx, &InstanceFilter{}); err != nil {
			return err
		}
		if len(flat) > 0 {
			if _, err := m.storager.CreateInstances(ctx, flat); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := m.RepairDangling(ctx); err != nil {
		return nil, err
	}

	return m.GetPool(ctx)
}

// validateGroups checks the pool shape and returns the flattened instance list.
func (m *EppPoolManager) validateGroups(groups []*InstanceGroup) ([]*InstanceParam, error) {
	if len(groups) == 0 {
		return nil, xerror.WrapParamErrorWithMsg("epp pool requires at least one group")
	}

	groupNames := map[string]bool{}
	instanceIDs := map[string]bool{}
	addresses := map[string]bool{}

	var flat []*InstanceParam
	for _, group := range groups {
		if group == nil {
			return nil, xerror.WrapParamErrorWithMsg("epp pool group is nil")
		}
		if group.Name == "" {
			return nil, xerror.WrapParamErrorWithMsg("epp pool group name is empty")
		}
		if groupNames[group.Name] {
			return nil, xerror.WrapParamErrorWithMsg("epp pool group name %q is duplicated", group.Name)
		}
		groupNames[group.Name] = true

		if err := m.validateGroupSize(group); err != nil {
			return nil, err
		}

		for _, inst := range group.Instances {
			if inst == nil {
				return nil, xerror.WrapParamErrorWithMsg("epp pool instance of group %q is nil", group.Name)
			}
			if inst.ID == "" {
				return nil, xerror.WrapParamErrorWithMsg("epp pool instance id is empty (group %q)", group.Name)
			}
			if instanceIDs[inst.ID] {
				return nil, xerror.WrapParamErrorWithMsg("epp pool instance id %q is duplicated", inst.ID)
			}
			instanceIDs[inst.ID] = true

			if err := validateHost(inst.Host); err != nil {
				return nil, xerror.WrapParamErrorWithMsg("epp pool instance %q host: %s", inst.ID, err.Error())
			}
			if err := validatePort(inst.Port); err != nil {
				return nil, xerror.WrapParamErrorWithMsg("epp pool instance %q port: %s", inst.ID, err.Error())
			}

			addr := instanceAddress(inst.Host, inst.Port)
			if addresses[addr] {
				return nil, xerror.WrapParamErrorWithMsg("epp pool instance address %s is duplicated", addr)
			}
			addresses[addr] = true

			flat = append(flat, &InstanceParam{
				ID:        inst.ID,
				Host:      inst.Host,
				Port:      inst.Port,
				GroupName: group.Name,
			})
		}
	}

	return flat, nil
}

// validateGroupSize enforces the per-group instance count: exactly 1
// (a group configures a single EPP instance; backup/standby pairs are
// no longer accepted at the API write path).
func (m *EppPoolManager) validateGroupSize(group *InstanceGroup) error {
	if n := len(group.Instances); n != 1 {
		return xerror.WrapParamErrorWithMsg("epp pool group %q requires exactly 1 instance, got %d", group.Name, n)
	}
	return nil
}

// buildPool groups flat instances by group name with deterministic order.
func buildPool(name string, instances []*InstanceParam) *EppPool {
	groupMap := map[string][]*InstanceParam{}
	for _, inst := range instances {
		groupMap[inst.GroupName] = append(groupMap[inst.GroupName], inst)
	}

	groupNames := make([]string, 0, len(groupMap))
	for groupName := range groupMap {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)

	pool := &EppPool{Name: name, Groups: []*InstanceGroup{}}
	for _, groupName := range groupNames {
		insts := groupMap[groupName]
		sort.Slice(insts, func(i, j int) bool { return insts[i].ID < insts[j].ID })
		pool.Groups = append(pool.Groups, &InstanceGroup{
			Name:      groupName,
			Instances: insts,
		})
	}

	return pool
}

func instanceAddress(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func wrapModelError(err error) error {
	if err == nil {
		return nil
	}
	return xerror.WrapModelErrorWithMsg("epp pool: %s", err.Error())
}
