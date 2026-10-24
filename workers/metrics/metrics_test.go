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
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	dbCommon "github.com/cloudbase/garm/database/common"
	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/params"
)

// queueDurationSamples returns the sample count of every
// garm_job_queue_duration_seconds series, keyed by "owner|requested_labels".
func queueDurationSamples(t *testing.T) map[string]uint64 {
	t.Helper()

	reg := prometheus.NewRegistry()
	if err := reg.Register(metrics.JobQueueDuration); err != nil {
		t.Fatalf("registering JobQueueDuration: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gathering metrics: %v", err)
	}

	out := map[string]uint64{}
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lbl := range m.GetLabel() {
				labels[lbl.GetName()] = lbl.GetValue()
			}
			out[labels["owner"]+"|"+labels["requested_labels"]] = m.GetHistogram().GetSampleCount()
		}
	}
	return out
}

func TestHandleJobEventLabelsQueueDurationByRunnerLabels(t *testing.T) {
	metrics.JobQueueDuration.Reset()
	w := NewWorker(context.Background())

	createdAt := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	job := params.Job{
		ID:              1,
		Status:          string(params.JobStatusInProgress),
		RepositoryOwner: "test-owner",
		Labels:          []string{"X64", "self-hosted", "linux"},
		CreatedAt:       createdAt,
		StartedAt:       createdAt.Add(7 * time.Minute),
	}
	w.handleJobEvent(dbCommon.UpdateOperation, job)

	other := job
	other.ID = 2
	other.Labels = []string{"self-hosted", "arm64"}
	w.handleJobEvent(dbCommon.UpdateOperation, other)

	got := queueDurationSamples(t)
	want := map[string]uint64{
		"test-owner|linux,self-hosted,x64": 1,
		"test-owner|arm64,self-hosted":     1,
	}
	if len(got) != len(want) {
		t.Fatalf("series = %v, want %v", got, want)
	}
	for key, count := range want {
		if got[key] != count {
			t.Errorf("series %q count = %d, want %d (all series: %v)", key, got[key], count, got)
		}
	}
}
