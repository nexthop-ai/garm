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

	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/runner"
)

// CollectJobMetric collects the metrics for the jobs recorded by GARM
// aggregating by status and entity (org/repo/enterprise).
func CollectJobMetric(ctx context.Context, r *runner.Runner) error {
	// reset metrics
	metrics.JobStatus.Reset()
	metrics.JobCount.Reset()

	jobs, err := r.ListAllJobs(ctx)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		metrics.JobStatus.WithLabelValues(
			fmt.Sprintf("%d", job.ID),            // label: job_id
			fmt.Sprintf("%d", job.WorkflowJobID), // label: workflow_job_id
			job.ScaleSetJobID,                    // label: scaleset_job_id
			fmt.Sprintf("%d", job.RunID),         // label: workflow_run_id
			job.Name,                             // label: name
			job.Status,                           // label: status
			job.Conclusion,                       // label: conclusion
			job.RunnerName,                       // label: runner_name
			job.RepositoryOwner,                  // label: owner
			job.RepositoryName,                   // label: repository
			strings.Join(job.Labels, " "),        // label: requested_labels
		).Set(1)
	}

	// Aggregate counts by status and entity
	type countKey struct {
		Status     string
		EntityType string
		EntityName string
	}
	counts := make(map[countKey]int)

	for _, job := range jobs {
		// Determine entity type and name
		var entityType, entityName string
		switch {
		case job.OrgID != nil:
			entityType = "organization"
			entityName = job.RepositoryOwner // org name is in RepositoryOwner for org-level jobs
		case job.RepoID != nil:
			entityType = "repository"
			entityName = job.RepositoryOwner + "/" + job.RepositoryName
		case job.EnterpriseID != nil:
			entityType = "enterprise"
			entityName = job.RepositoryOwner
		default:
			entityType = "unknown"
			entityName = "unknown"
		}

		key := countKey{
			Status:     job.Status,
			EntityType: entityType,
			EntityName: entityName,
		}
		counts[key]++
	}

	// Emit aggregate counts
	for key, count := range counts {
		metrics.JobCount.WithLabelValues(
			key.Status,     // label: status
			key.EntityType, // label: entity_type
			key.EntityName, // label: entity_name
		).Set(float64(count))
	}

	return nil
}
