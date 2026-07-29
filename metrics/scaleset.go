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
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudbase/garm/params"
)

// scaleSetStatsLabels are the labels used by the scale set statistics gauges.
// They are self sufficient on purpose, so that dashboards don't need to join
// against garm_scaleset_info to get a human readable scale set name.
var scaleSetStatsLabels = []string{"id", "scaleset_id", "name"}

var (
	ScaleSetInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "info",
		Help:      "Info of the scale set",
	}, []string{"id", "scaleset_id", "name", "image", "flavor", "prefix", "os_type", "os_arch", "tags", "provider", "runner_group", "scaleset_owner", "scaleset_type"})

	ScaleSetStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "status",
		Help:      "Status of the scale set",
	}, []string{"id", "enabled", "state"})

	ScaleSetMaxRunners = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "max_runners",
		Help:      "Maximum number of runners in the scale set",
	}, []string{"id"})

	ScaleSetMinIdleRunners = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "min_idle_runners",
		Help:      "Minimum number of idle runners in the scale set",
	}, []string{"id"})

	ScaleSetDesiredRunnerCount = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "desired_runner_count",
		Help:      "Desired runner count requested by GitHub for the scale set",
	}, []string{"id"})

	ScaleSetBootstrapTimeout = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsScaleSetSubsystem,
		Name:      "bootstrap_timeout",
		Help:      "Runner bootstrap timeout in the scale set",
	}, []string{"id"})

	// The gauges below mirror the statistics GitHub attaches to every message
	// queue response. Unlike garm_job_status, which only knows about jobs GARM
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
// makes garm_job_status underreport the backlog during saturation.
func RecordScaleSetStatistics(id, scaleSetID, name string, stats *params.RunnerScaleSetStatistic) {
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
		gauge.WithLabelValues(id, scaleSetID, name).Set(float64(value))
	}
}
