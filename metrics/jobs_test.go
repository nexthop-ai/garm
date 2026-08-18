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
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// collect gathers the current samples of a collector, keyed by the joined
// label values of each series.
func collect(t *testing.T, c prometheus.Collector) map[string]*dto.Metric {
	t.Helper()

	ch := make(chan prometheus.Metric, 64)
	c.Collect(ch)
	close(ch)

	out := map[string]*dto.Metric{}
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("writing metric: %v", err)
		}
		key := ""
		for _, lbl := range pb.GetLabel() {
			if key != "" {
				key += "|"
			}
			key += lbl.GetName() + "=" + lbl.GetValue()
		}
		out[key] = &pb
	}
	return out
}

func TestNormalizeRunnerLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected string
	}{
		{
			name:     "nil labels",
			labels:   nil,
			expected: "",
		},
		{
			name:     "empty labels",
			labels:   []string{},
			expected: "",
		},
		{
			name:     "labels are sorted",
			labels:   []string{"x64", "self-hosted", "linux"},
			expected: "linux,self-hosted,x64",
		},
		{
			name:     "permutations collapse to the same value",
			labels:   []string{"linux", "x64", "self-hosted"},
			expected: "linux,self-hosted,x64",
		},
		{
			name:     "blank and duplicate labels are dropped",
			labels:   []string{"linux", "  ", "linux", ""},
			expected: "linux",
		},
		{
			name:     "surrounding whitespace is trimmed",
			labels:   []string{" self-hosted ", "linux"},
			expected: "linux,self-hosted",
		},
		{
			// GitHub matches runner labels case-insensitively.
			name:     "labels are lowercased",
			labels:   []string{"Linux", "Self-Hosted", "X64"},
			expected: "linux,self-hosted,x64",
		},
		{
			name:     "case variants of one label collapse",
			labels:   []string{"linux", "Linux", "LINUX"},
			expected: "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeRunnerLabels(tt.labels); got != tt.expected {
				t.Errorf("NormalizeRunnerLabels() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestObserveJobQueueDuration(t *testing.T) {
	queued := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		queueTime   time.Time
		startTime   time.Time
		wantSamples uint64
		wantSum     float64
	}{
		{
			name:        "normal wait is observed",
			queueTime:   queued,
			startTime:   queued.Add(90 * time.Second),
			wantSamples: 1,
			wantSum:     90,
		},
		{
			name:        "zero queue time is dropped",
			queueTime:   time.Time{},
			startTime:   queued.Add(90 * time.Second),
			wantSamples: 0,
		},
		{
			name:        "zero start time is dropped",
			queueTime:   queued,
			startTime:   time.Time{},
			wantSamples: 0,
		},
		{
			name:      "negative duration is dropped",
			queueTime: queued,
			startTime: queued.Add(-time.Minute),
			// clock skew between GitHub components can produce this.
			wantSamples: 0,
		},
		{
			name:        "zero duration is observed",
			queueTime:   queued,
			startTime:   queued,
			wantSamples: 1,
			wantSum:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			JobQueueDuration.Reset()
			ObserveJobQueueDuration(
				JobQueueSourceScaleSet,
				[]string{"x64", "linux"},
				tt.queueTime, tt.startTime)

			series := collect(t, JobQueueDuration)
			if tt.wantSamples == 0 {
				if len(series) != 0 {
					t.Fatalf("expected no series, got %d", len(series))
				}
				return
			}

			key := "runner_labels=linux,x64|source=scaleset"
			pb, ok := series[key]
			if !ok {
				t.Fatalf("no series for %q, got %v", key, series)
			}
			if got := pb.GetHistogram().GetSampleCount(); got != tt.wantSamples {
				t.Errorf("sample count = %d, want %d", got, tt.wantSamples)
			}
			if got := pb.GetHistogram().GetSampleSum(); got != tt.wantSum {
				t.Errorf("sample sum = %v, want %v", got, tt.wantSum)
			}
		})
	}
}

func TestObserveJobQueueDurationSourcesAreSeparateSeries(t *testing.T) {
	JobQueueDuration.Reset()
	queued := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)

	ObserveJobQueueDuration(JobQueueSourceScaleSet, []string{"linux"}, queued, queued.Add(time.Minute))
	ObserveJobQueueDuration(JobQueueSourceWebhook, []string{"linux"}, queued, queued.Add(2*time.Minute))

	series := collect(t, JobQueueDuration)
	if len(series) != 2 {
		t.Fatalf("expected 2 series, got %d: %v", len(series), series)
	}
	for key, want := range map[string]float64{
		"runner_labels=linux|source=scaleset": 60,
		"runner_labels=linux|source=webhook":  120,
	} {
		pb, ok := series[key]
		if !ok {
			t.Fatalf("no series for %q", key)
		}
		if got := pb.GetHistogram().GetSampleSum(); got != want {
			t.Errorf("%s sample sum = %v, want %v", key, got, want)
		}
	}
}
