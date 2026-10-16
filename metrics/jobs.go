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
	"slices"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

var JobStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsJobsSubsystem,
	Name:      "status",
	Help:      "List of workflow jobs and their status",
}, []string{
	"job_id",
	"workflow_job_id",
	"scaleset_job_id",
	"workflow_run_id",
	"name",
	"status",
	"conclusion",
	"runner_name",
	"owner",
	"repository",
	"requested_labels",
})

// JobCount is a low-cardinality aggregate metric that counts jobs
// by status, entity_type (repository, organization, enterprise),
// and entity_name.
var JobCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsJobsSubsystem,
	Name:      "count",
	Help:      "Count of jobs by status and entity",
}, []string{"status", "entity_type", "entity_name"})

// NormalizeRunnerLabels renders a job's requested labels as a single, stable
// label value. The labels are lowercased, since GitHub matches runner labels
// case-insensitively and "Linux" must not split a series from "linux", and
// sorted so that permutations of the same label set collapse into one series.
//
// Cardinality: the labels come from the workflow's runs-on, which workflow
// authors control, so every novel label set mints a new series. Keep runs-on
// values to a bounded, agreed set; do not put per-job or per-run values there.
func NormalizeRunnerLabels(runnerLabels []string) string {
	if len(runnerLabels) == 0 {
		return ""
	}
	sorted := make([]string, 0, len(runnerLabels))
	for _, lbl := range runnerLabels {
		if lbl = strings.ToLower(strings.TrimSpace(lbl)); lbl != "" {
			sorted = append(sorted, lbl)
		}
	}
	slices.Sort(sorted)
	return strings.Join(slices.Compact(sorted), ",")
}
