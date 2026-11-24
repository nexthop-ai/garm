// Copyright 2026 Cloudbase Solutions SRL
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
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner"
)

// The two kinds of job share the JobCount gauge, told apart by job_type.
// Each collector replaces only the series of its own kind.
const (
	jobTypeWebhook  = "webhook"
	jobTypeScaleSet = "scaleset"
)

type jobCountKey struct {
	Status     string
	EntityType string
	EntityName string
}

func setJobCounts(jobType string, counts map[jobCountKey]int) {
	metrics.JobCount.DeletePartialMatch(prometheus.Labels{"job_type": jobType})
	for key, count := range counts {
		metrics.JobCount.WithLabelValues(
			jobType,        // label: job_type
			key.Status,     // label: status
			key.EntityType, // label: entity_type
			key.EntityName, // label: entity_name
		).Set(float64(count))
	}
}

// scaleSetOwner returns the type and name of the entity a scale set belongs to.
func scaleSetOwner(scaleSet params.ScaleSet) (entityType, entityName string) {
	switch {
	case scaleSet.OrgName != "":
		return string(params.ForgeEntityTypeOrganization), scaleSet.OrgName
	case scaleSet.EnterpriseName != "":
		return string(params.ForgeEntityTypeEnterprise), scaleSet.EnterpriseName
	case scaleSet.RepoName != "":
		return string(params.ForgeEntityTypeRepository), scaleSet.RepoName
	}
	return "unknown", "unknown"
}

// CollectJobMetric collects the metrics for the jobs recorded by GARM
func CollectJobMetric(ctx context.Context, r *runner.Runner) error {
	// reset metrics
	metrics.JobStatus.Reset()

	jobs, err := r.ListAllJobs(ctx)
	if err != nil {
		return err
	}

	counts := make(map[jobCountKey]int)
	for _, job := range jobs {
		key := jobCountKey{Status: job.Status, EntityType: "unknown", EntityName: "unknown"}
		switch {
		case job.OrgID != nil:
			key.EntityType = string(params.ForgeEntityTypeOrganization)
			key.EntityName = job.RepositoryOwner
		case job.RepoID != nil:
			key.EntityType = string(params.ForgeEntityTypeRepository)
			key.EntityName = job.RepositoryOwner + "/" + job.RepositoryName
		case job.EnterpriseID != nil:
			key.EntityType = string(params.ForgeEntityTypeEnterprise)
			key.EntityName = job.RepositoryOwner
		}
		counts[key]++

		poolID := ""
		if job.PoolID != nil {
			poolID = job.PoolID.String()
		}
		metrics.JobStatus.WithLabelValues(
			fmt.Sprintf("%d", job.ID),            // label: job_id
			fmt.Sprintf("%d", job.WorkflowJobID), // label: workflow_job_id
			fmt.Sprintf("%d", job.RunID),         // label: workflow_run_id
			job.Name,                             // label: name
			job.Status,                           // label: status
			job.Conclusion,                       // label: conclusion
			job.RunnerName,                       // label: runner_name
			job.RepositoryOwner,                  // label: owner
			job.RepositoryName,                   // label: repository
			strings.Join(job.Labels, " "),        // label: requested_labels
			poolID,                               // label: pool_id
		).Set(1)
	}
	setJobCounts(jobTypeWebhook, counts)
	return nil
}

// CollectScaleSetJobMetric collects the metrics for jobs GitHub routed to
// scale sets.
func CollectScaleSetJobMetric(ctx context.Context, r *runner.Runner) error {
	metrics.ScaleSetJobStatus.Reset()
	metrics.ScaleSetJobCount.Reset()

	jobs, err := r.ListAllScaleSetJobs(ctx)
	if err != nil {
		return err
	}

	scaleSets, err := r.ListAllScaleSets(ctx)
	if err != nil {
		return err
	}
	scaleSetsByID := make(map[uint]params.ScaleSet, len(scaleSets))
	for _, scaleSet := range scaleSets {
		scaleSetsByID[scaleSet.ID] = scaleSet
	}

	type scaleSetJobCountKey struct {
		ScaleSetID uint
		Status     string
	}
	counts := make(map[jobCountKey]int)
	scaleSetCounts := make(map[scaleSetJobCountKey]int)
	for _, job := range jobs {
		key := jobCountKey{Status: job.Status, EntityType: "unknown", EntityName: "unknown"}
		if scaleSet, ok := scaleSetsByID[job.ScaleSetID]; ok {
			key.EntityType, key.EntityName = scaleSetOwner(scaleSet)
			scaleSetCounts[scaleSetJobCountKey{ScaleSetID: job.ScaleSetID, Status: job.Status}]++
		}
		counts[key]++
	}
	setJobCounts(jobTypeScaleSet, counts)
	for key, count := range scaleSetCounts {
		metrics.ScaleSetJobCount.WithLabelValues(
			fmt.Sprintf("%d", key.ScaleSetID),  // label: id
			scaleSetsByID[key.ScaleSetID].Name, // label: scaleset_name
			key.Status,                         // label: status
		).Set(float64(count))
	}

	for _, job := range jobs {
		scaleSetID := ""
		if job.ScaleSetID != 0 {
			scaleSetID = fmt.Sprintf("%d", job.ScaleSetID)
		}
		metrics.ScaleSetJobStatus.WithLabelValues(
			job.ScaleSetJobID,                    // label: scaleset_job_id
			scaleSetID,                           // label: scaleset_id
			fmt.Sprintf("%d", job.WorkflowRunID), // label: workflow_run_id
			job.Name,                             // label: name
			job.Status,                           // label: status
			job.Result,                           // label: result
			job.RunnerName,                       // label: runner_name
			job.RunnerGroupName,                  // label: runner_group
			job.RepositoryOwner,                  // label: owner
			job.RepositoryName,                   // label: repository
			strings.Join(job.RequestLabels, " "), // label: requested_labels
		).Set(1)
	}
	return nil
}
