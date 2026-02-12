// Copyright 2025 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package metrics

import (
	"context"
	"fmt"

	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/runner"
)

// CollectInstanceMetric collects the metrics for the runner instances
// reflecting the statuses and the pool/scaleset they belong to.
func CollectInstanceMetric(ctx context.Context, r *runner.Runner) error {
	// reset metrics
	metrics.InstanceStatus.Reset()
	metrics.InstanceCount.Reset()

	instances, err := r.ListAllInstances(ctx)
	if err != nil {
		return err
	}

	pools, err := r.ListAllPools(ctx)
	if err != nil {
		return err
	}

	scalesets, err := r.ListAllScaleSets(ctx)
	if err != nil {
		return err
	}

	type entityInfo struct {
		Name         string
		Type         string
		ProviderName string
	}

	// Build lookup maps for pools and scalesets
	poolInfo := make(map[string]entityInfo)
	for _, pool := range pools {
		switch {
		case pool.OrgName != "":
			poolInfo[pool.ID] = entityInfo{
				Name:         pool.OrgName,
				Type:         string(pool.PoolType()),
				ProviderName: pool.ProviderName,
			}
		case pool.EnterpriseName != "":
			poolInfo[pool.ID] = entityInfo{
				Name:         pool.EnterpriseName,
				Type:         string(pool.PoolType()),
				ProviderName: pool.ProviderName,
			}
		default:
			poolInfo[pool.ID] = entityInfo{
				Name:         pool.RepoName,
				Type:         string(pool.PoolType()),
				ProviderName: pool.ProviderName,
			}
		}
	}

	scalesetInfo := make(map[uint]entityInfo)
	for _, ss := range scalesets {
		switch {
		case ss.OrgName != "":
			scalesetInfo[ss.ID] = entityInfo{
				Name:         ss.OrgName,
				Type:         "organization",
				ProviderName: ss.ProviderName,
			}
		case ss.EnterpriseName != "":
			scalesetInfo[ss.ID] = entityInfo{
				Name:         ss.EnterpriseName,
				Type:         "enterprise",
				ProviderName: ss.ProviderName,
			}
		default:
			scalesetInfo[ss.ID] = entityInfo{
				Name:         ss.RepoName,
				Type:         "repository",
				ProviderName: ss.ProviderName,
			}
		}
	}

	// Aggregate counts by status/runner_status/pool_owner/pool_type/provider
	type countKey struct {
		Status       string
		RunnerStatus string
		PoolOwner    string
		PoolType     string
		Provider     string
	}
	counts := make(map[countKey]int)

	for _, instance := range instances {
		var info entityInfo
		var entityID string

		// Look up entity info from pool or scaleset
		if instance.PoolID != "" {
			info = poolInfo[instance.PoolID]
			entityID = instance.PoolID
		} else if instance.ScaleSetID > 0 {
			info = scalesetInfo[instance.ScaleSetID]
			entityID = fmt.Sprintf("scaleset-%d", instance.ScaleSetID)
		}

		// Per-instance metric (high cardinality)
		scalesetID := ""
		if instance.ScaleSetID > 0 {
			scalesetID = fmt.Sprintf("%d", instance.ScaleSetID)
		}
		metrics.InstanceStatus.WithLabelValues(
			instance.Name,                 // label: name
			string(instance.Status),       // label: status
			string(instance.RunnerStatus), // label: runner_status
			info.Name,                     // label: pool_owner
			info.Type,                     // label: pool_type
			entityID,                      // label: pool_id
			scalesetID,                    // label: scaleset_id
			info.ProviderName,             // label: provider
		).Set(1)

		// Aggregate count
		key := countKey{
			Status:       string(instance.Status),
			RunnerStatus: string(instance.RunnerStatus),
			PoolOwner:    info.Name,
			PoolType:     info.Type,
			Provider:     info.ProviderName,
		}
		counts[key]++
	}

	// Emit aggregate counts (low cardinality)
	for key, count := range counts {
		metrics.InstanceCount.WithLabelValues(
			key.Status,       // label: status
			key.RunnerStatus, // label: runner_status
			key.PoolOwner,    // label: pool_owner
			key.PoolType,     // label: pool_type
			key.Provider,     // label: provider
		).Set(float64(count))
	}

	return nil
}
