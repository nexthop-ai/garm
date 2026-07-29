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
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Job queue duration sources. The two code paths that can observe a
// queue-to-start latency are mutually exclusive for any given job.
const (
	// JobQueueSourceScaleSet is used for jobs reported by the scale set
	// listener via GitHub's Actions message queue.
	JobQueueSourceScaleSet = "scaleset"
	// JobQueueSourceWebhook is used for jobs reported by a workflow_job
	// webhook. Scale set jobs are excluded from this source.
	JobQueueSourceWebhook = "webhook"
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

// JobQueueDuration tracks how long jobs waited between being queued on the
// forge and being assigned to a runner. Durations are always computed from
// timestamps reported by the forge, never from GARM's local clock: scale set
// messages are delivered late by design and GitHub withholds job messages
// beyond the advertised runner capacity, so local receive time would badly
// understate the real wait.
var JobQueueDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: metricsNamespace,
	Subsystem: metricsJobsSubsystem,
	Name:      "queue_duration_seconds",
	Help:      "Time jobs spent queued on the forge before a runner started them",
	Buckets: []float64{
		15, 30, 60, 120, 300, 600, 1200, 2400, 3600, 7200, 14400,
	},
}, []string{"runner_labels", "source"})

// ObserveJobQueueDuration records a queue-to-start latency for a job.
//
// queueTime and startTime must both come from the forge. Zero timestamps and
// negative durations are dropped: the forge occasionally omits either
// timestamp, and clock skew between GitHub's own components can produce a
// start time that precedes the queue time.
func ObserveJobQueueDuration(source string, runnerLabels []string, queueTime, startTime time.Time) {
	if queueTime.IsZero() || startTime.IsZero() {
		return
	}
	duration := startTime.Sub(queueTime)
	if duration < 0 {
		return
	}
	JobQueueDuration.WithLabelValues(
		NormalizeRunnerLabels(runnerLabels), // label: runner_labels
		source,                              // label: source
	).Observe(duration.Seconds())
}

// NormalizeRunnerLabels renders a job's requested labels as a single, stable
// label value. The labels are sorted so that permutations of the same label
// set collapse into one time series, keeping cardinality bounded.
func NormalizeRunnerLabels(runnerLabels []string) string {
	if len(runnerLabels) == 0 {
		return ""
	}
	sorted := make([]string, 0, len(runnerLabels))
	for _, lbl := range runnerLabels {
		if lbl = strings.TrimSpace(lbl); lbl != "" {
			sorted = append(sorted, lbl)
		}
	}
	slices.Sort(sorted)
	return strings.Join(slices.Compact(sorted), ",")
}
