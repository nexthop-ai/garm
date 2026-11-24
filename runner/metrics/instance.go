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
	"strconv"

	"github.com/cloudbase/garm/cache"
	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/runner"
)

type parentInfo struct {
	OwnerName    string
	Type         string
	ProviderName string
	// ScaleSetName is set for scale sets only.
	ScaleSetName string
}

// instanceCountKey and scaleSetRunnerCountKey are the label sets of the
// aggregate runner gauges.
type instanceCountKey struct {
	Status       string
	RunnerStatus string
	OwnerName    string
	Type         string
	ProviderName string
}

type scaleSetRunnerCountKey struct {
	ScaleSetID   string
	ScaleSetName string
	Status       string
	RunnerStatus string
	ProviderName string
}

// CollectInstanceMetric collects the metrics for the runner instances
// reflecting the statuses and the pool or scale set they belong to.
func CollectInstanceMetric(ctx context.Context, r *runner.Runner) error {
	// reset metrics
	metrics.InstanceStatus.Reset()
	metrics.InstanceCount.Reset()
	metrics.ScaleSetRunnerCount.Reset()

	instances, err := r.ListAllInstances(ctx)
	if err != nil {
		return err
	}

	// Sample the in-memory cache against the DB listing above. Sustained
	// divergence means the event-driven cache is leaking or losing entries.
	metrics.CacheDBInstancesCount.Set(float64(len(instances)))
	metrics.CacheInstancesCount.Set(float64(len(cache.GetAllInstancesCache())))

	pools, err := r.ListAllPools(ctx)
	if err != nil {
		return err
	}

	scaleSets, err := r.ListAllScaleSets(ctx)
	if err != nil {
		return err
	}

	poolParents := make(map[string]parentInfo, len(pools))
	for _, pool := range pools {
		info := parentInfo{
			Type:         string(pool.PoolType()),
			ProviderName: pool.ProviderName,
		}
		switch {
		case pool.OrgName != "":
			info.OwnerName = pool.OrgName
		case pool.EnterpriseName != "":
			info.OwnerName = pool.EnterpriseName
		default:
			info.OwnerName = pool.RepoName
		}
		poolParents[pool.ID] = info
	}

	scaleSetParents := make(map[uint]parentInfo, len(scaleSets))
	for _, scaleSet := range scaleSets {
		info := parentInfo{
			Type:         string(scaleSet.ScaleSetType()),
			ProviderName: scaleSet.ProviderName,
			ScaleSetName: scaleSet.Name,
		}
		switch {
		case scaleSet.OrgName != "":
			info.OwnerName = scaleSet.OrgName
		case scaleSet.EnterpriseName != "":
			info.OwnerName = scaleSet.EnterpriseName
		default:
			info.OwnerName = scaleSet.RepoName
		}
		scaleSetParents[scaleSet.ID] = info
	}

	counts := make(map[instanceCountKey]int)
	scaleSetCounts := make(map[scaleSetRunnerCountKey]int)
	for _, instance := range instances {
		var (
			parent     parentInfo
			poolID     string
			scaleSetID string
		)
		if instance.ScaleSetID != 0 {
			parent = scaleSetParents[instance.ScaleSetID]
			scaleSetID = strconv.FormatUint(uint64(instance.ScaleSetID), 10)
		} else {
			parent = poolParents[instance.PoolID]
			poolID = instance.PoolID
		}

		metrics.InstanceStatus.WithLabelValues(
			instance.Name,                 // label: name
			string(instance.Status),       // label: status
			string(instance.RunnerStatus), // label: runner_status
			parent.OwnerName,              // label: pool_owner
			parent.Type,                   // label: pool_type
			poolID,                        // label: pool_id
			scaleSetID,                    // label: scaleset_id
			parent.ProviderName,           // label: provider
		).Set(1)

		counts[instanceCountKey{
			Status:       string(instance.Status),
			RunnerStatus: string(instance.RunnerStatus),
			OwnerName:    parent.OwnerName,
			Type:         parent.Type,
			ProviderName: parent.ProviderName,
		}]++
		if instance.ScaleSetID != 0 {
			scaleSetCounts[scaleSetRunnerCountKey{
				ScaleSetID:   scaleSetID,
				ScaleSetName: parent.ScaleSetName,
				Status:       string(instance.Status),
				RunnerStatus: string(instance.RunnerStatus),
				ProviderName: parent.ProviderName,
			}]++
		}
	}

	for key, count := range counts {
		metrics.InstanceCount.WithLabelValues(
			key.Status,       // label: status
			key.RunnerStatus, // label: runner_status
			key.OwnerName,    // label: pool_owner
			key.Type,         // label: pool_type
			key.ProviderName, // label: provider
		).Set(float64(count))
	}
	for key, count := range scaleSetCounts {
		metrics.ScaleSetRunnerCount.WithLabelValues(
			key.ScaleSetID,   // label: id
			key.ScaleSetName, // label: scaleset_name
			key.Status,       // label: status
			key.RunnerStatus, // label: runner_status
			key.ProviderName, // label: provider
		).Set(float64(count))
	}
	return nil
}
