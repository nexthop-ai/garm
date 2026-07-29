// Copyright 2026 Cloudbase Solutions SRL
//
//	Licensed under the Apache License, Version 2.0 (the "License"); you may
//	not use this file except in compliance with the License. You may obtain
//	a copy of the License at
//
//	     http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//	WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//	License for the specific language governing permissions and limitations
//	under the License.

//go:build testing

package scaleset

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/util/github/scalesets"
)

// stubScaleSetHelper implements just enough of scaleSetHelper to exercise the
// metrics recording helpers.
type stubScaleSetHelper struct {
	scaleSet params.ScaleSet
}

func (s *stubScaleSetHelper) GetScaleSet() params.ScaleSet { return s.scaleSet }
func (s *stubScaleSetHelper) GetScaleSetClient() (*scalesets.ScaleSetClient, error) {
	return nil, nil
}
func (s *stubScaleSetHelper) SetLastMessageID(_ int64) error                          { return nil }
func (s *stubScaleSetHelper) SetDesiredRunnerCount(_ int) error                       { return nil }
func (s *stubScaleSetHelper) Owner() string                                           { return "test-owner" }
func (s *stubScaleSetHelper) HandleJobsCompleted(_ []params.ScaleSetJobMessage) error { return nil }
func (s *stubScaleSetHelper) HandleJobsStarted(_ []params.ScaleSetJobMessage) error   { return nil }
func (s *stubScaleSetHelper) HandleJobsAvailable(_ []params.ScaleSetJobMessage) error { return nil }

func gaugeValue(t *testing.T, gauge *prometheus.GaugeVec, labels ...string) float64 {
	t.Helper()

	m, err := gauge.GetMetricWithLabelValues(labels...)
	if err != nil {
		t.Fatalf("getting gauge: %v", err)
	}
	var pb dto.Metric
	if err := m.Write(&pb); err != nil {
		t.Fatalf("writing gauge: %v", err)
	}
	return pb.GetGauge().GetValue()
}

func newTestListener() *scaleSetListener {
	helper := &stubScaleSetHelper{
		scaleSet: params.ScaleSet{
			ID:         7,
			ScaleSetID: 42,
			Name:       "test-scaleset",
		},
	}
	l := newListener(context.Background(), helper)
	return l
}

func TestRecordStatistics(t *testing.T) {
	metrics.ScaleSetAvailableJobs.Reset()
	metrics.ScaleSetAssignedJobs.Reset()
	metrics.ScaleSetIdleRunners.Reset()

	l := newTestListener()
	l.recordStatistics(&params.RunnerScaleSetStatistic{
		TotalAvailableJobs: 250,
		TotalAssignedJobs:  10,
		TotalIdleRunners:   3,
	})

	// GARM's internal ID, GitHub's numeric ID and the scale set name.
	lbls := []string{"7", "42", "test-scaleset"}

	if got := gaugeValue(t, metrics.ScaleSetAvailableJobs, lbls...); got != 250 {
		t.Errorf("available_jobs = %v, want 250", got)
	}
	if got := gaugeValue(t, metrics.ScaleSetAssignedJobs, lbls...); got != 10 {
		t.Errorf("assigned_jobs = %v, want 10", got)
	}
	if got := gaugeValue(t, metrics.ScaleSetIdleRunners, lbls...); got != 3 {
		t.Errorf("idle_runners = %v, want 3", got)
	}
}

func TestRecordStatisticsNilIsNoop(t *testing.T) {
	metrics.ScaleSetAvailableJobs.Reset()

	l := newTestListener()
	l.recordStatistics(nil)

	ch := make(chan prometheus.Metric, 8)
	metrics.ScaleSetAvailableJobs.Collect(ch)
	close(ch)
	if len(ch) != 0 {
		t.Errorf("expected no series, got %d", len(ch))
	}
}

// TestHandleSessionMessageIgnoresNonJobMessagesButKeepsStatistics documents
// that statistics ride along on every message, even ones we otherwise drop.
func TestHandleSessionMessageIgnoresNonJobMessagesButKeepsStatistics(t *testing.T) {
	metrics.ScaleSetAvailableJobs.Reset()

	l := newTestListener()
	l.handleSessionMessage(params.RunnerScaleSetMessage{
		MessageID:   1,
		MessageType: "SomeOtherMessageType",
		Statistics: &params.RunnerScaleSetStatistic{
			TotalAvailableJobs: 99,
		},
	})

	if got := gaugeValue(t, metrics.ScaleSetAvailableJobs, "7", "42", "test-scaleset"); got != 99 {
		t.Errorf("available_jobs = %v, want 99", got)
	}
}
