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
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudbase/garm/params"
)

func scaleSetStatsGauges() map[string]*prometheus.GaugeVec {
	return map[string]*prometheus.GaugeVec{
		"gh_available_jobs":     ScaleSetGHAvailableJobs,
		"gh_acquired_jobs":      ScaleSetGHAcquiredJobs,
		"gh_assigned_jobs":      ScaleSetGHAssignedJobs,
		"gh_running_jobs":       ScaleSetGHRunningJobs,
		"gh_registered_runners": ScaleSetGHRegisteredRunners,
		"gh_busy_runners":       ScaleSetGHBusyRunners,
		"gh_idle_runners":       ScaleSetGHIdleRunners,
	}
}

func resetScaleSetStats() {
	for _, gauge := range scaleSetStatsGauges() {
		gauge.Reset()
	}
}

func TestRecordScaleSetStatistics(t *testing.T) {
	resetScaleSetStats()

	RecordScaleSetStatistics("my-scaleset", "test-provider", &params.RunnerScaleSetStatistic{
		TotalAvailableJobs:     101,
		TotalAcquiredJobs:      7,
		TotalAssignedJobs:      5,
		TotalRunningJobs:       4,
		TotalRegisteredRunners: 10,
		TotalBusyRunners:       4,
		TotalIdleRunners:       6,
	})

	wantValues := map[string]float64{
		"gh_available_jobs":     101,
		"gh_acquired_jobs":      7,
		"gh_assigned_jobs":      5,
		"gh_running_jobs":       4,
		"gh_registered_runners": 10,
		"gh_busy_runners":       4,
		"gh_idle_runners":       6,
	}

	key := "provider=test-provider|scaleset_name=my-scaleset"
	for name, gauge := range scaleSetStatsGauges() {
		series := collect(t, gauge)
		pb, ok := series[key]
		if !ok {
			t.Fatalf("%s: no series for %q, got %v", name, key, series)
		}
		if got := pb.GetGauge().GetValue(); got != wantValues[name] {
			t.Errorf("%s = %v, want %v", name, got, wantValues[name])
		}
	}
}

func TestRecordScaleSetStatisticsNilIsNoop(t *testing.T) {
	resetScaleSetStats()

	RecordScaleSetStatistics("my-scaleset", "test-provider", nil)

	for name, gauge := range scaleSetStatsGauges() {
		if series := collect(t, gauge); len(series) != 0 {
			t.Errorf("%s: expected no series, got %v", name, series)
		}
	}
}
