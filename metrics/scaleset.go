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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudbase/garm/params"
)

// scaleSetStatsLabels are the labels used by the scale set statistics gauges.
// They mirror the low cardinality naming used by the other scaleset metrics.
var scaleSetStatsLabels = []string{"scaleset_name", "provider"}

var (
	// ScaleSetStatus reports the status of each scaleset.
	// The value is 1 if the scaleset is enabled, 0 if disabled.
	ScaleSetStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "status",
		Help:      "Status of each scaleset (1=enabled, 0=disabled)",
	}, []string{"name", "state", "entity_type", "entity_name", "provider"})

	// ScaleSetRunnerCount counts runner instances per scaleset, broken down by
	// instance status and runner status. Use this to track per-scaleset capacity
	// and utilization.
	ScaleSetRunnerCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "runner_count",
		Help:      "Count of runner instances per scaleset by status",
	}, []string{"scaleset_name", "status", "runner_status", "provider"})

	// ScaleSetJobCount counts jobs per scaleset, broken down by job status.
	// Use this to monitor job queue depth per scaleset and decide when to
	// increase max_runners.
	ScaleSetJobCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "job_count",
		Help:      "Count of jobs per scaleset by status",
	}, []string{"scaleset_name", "status"})

	// The gauges below mirror the statistics GitHub attaches to every message
	// queue response. Unlike ScaleSetJobCount, which only knows about jobs GARM
	// has been told about, these are GitHub's own view of the scale set.
	ScaleSetAvailableJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "available_jobs",
		Help:      "Jobs queued on the forge and available to the scale set, as reported by GitHub",
	}, scaleSetStatsLabels)

	ScaleSetAcquiredJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "acquired_jobs",
		Help:      "Jobs acquired from the scale set queue, as reported by GitHub",
	}, scaleSetStatsLabels)

	ScaleSetAssignedJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "assigned_jobs",
		Help:      "Jobs assigned to the scale set, as reported by GitHub",
	}, scaleSetStatsLabels)

	ScaleSetRunningJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "running_jobs",
		Help:      "Jobs currently running on the scale set, as reported by GitHub",
	}, scaleSetStatsLabels)

	ScaleSetRegisteredRunners = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "registered_runners",
		Help:      "Runners registered with the scale set, as reported by GitHub",
	}, scaleSetStatsLabels)

	ScaleSetBusyRunners = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "busy_runners",
		Help:      "Registered runners currently executing a job, as reported by GitHub",
	}, scaleSetStatsLabels)

	ScaleSetIdleRunners = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "idle_runners",
		Help:      "Registered runners currently idle, as reported by GitHub",
	}, scaleSetStatsLabels)
)

// RecordScaleSetStatistics publishes the statistics GitHub reports for a scale
// set. TotalAvailableJobs is the real forge side queue depth: it is not gated
// by the capacity GARM advertises when longpolling for messages, which is what
// makes garm_job_count{status="queued"} and garm_scaleset_job_count underreport
// the backlog during saturation.
func RecordScaleSetStatistics(scaleSetName, provider string, stats *params.RunnerScaleSetStatistic) {
	if stats == nil {
		return
	}
	for gauge, value := range map[*prometheus.GaugeVec]int{
		ScaleSetAvailableJobs:     stats.TotalAvailableJobs,
		ScaleSetAcquiredJobs:      stats.TotalAcquiredJobs,
		ScaleSetAssignedJobs:      stats.TotalAssignedJobs,
		ScaleSetRunningJobs:       stats.TotalRunningJobs,
		ScaleSetRegisteredRunners: stats.TotalRegisteredRunners,
		ScaleSetBusyRunners:       stats.TotalBusyRunners,
		ScaleSetIdleRunners:       stats.TotalIdleRunners,
	} {
		gauge.WithLabelValues(scaleSetName, provider).Set(float64(value))
	}
}
