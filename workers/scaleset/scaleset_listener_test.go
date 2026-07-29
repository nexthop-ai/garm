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
	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm/metrics"
	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/util/github/scalesets"
)

// stubScaleSetHelper implements just enough of scaleSetHelper to exercise
// statistics handling and the metrics recording helpers, and records what
// was persisted.
type stubScaleSetHelper struct {
	scaleSet params.ScaleSet

	statistics []params.RunnerScaleSetStatistic
	started    [][]params.ScaleSetJobMessage
}

func (s *stubScaleSetHelper) GetScaleSet() params.ScaleSet { return s.scaleSet }
func (s *stubScaleSetHelper) GetScaleSetClient() (*scalesets.ScaleSetClient, error) {
	return nil, nil
}
func (s *stubScaleSetHelper) SetLastMessageID(_ int64) error { return nil }
func (s *stubScaleSetHelper) SetRunnerStatistics(stats params.RunnerScaleSetStatistic) error {
	s.statistics = append(s.statistics, stats)
	return nil
}

func (s *stubScaleSetHelper) Owner() string                                           { return "test-owner" }
func (s *stubScaleSetHelper) HandleJobsCompleted(_ []params.ScaleSetJobMessage) error { return nil }
func (s *stubScaleSetHelper) HandleJobsStarted(jobs []params.ScaleSetJobMessage) error {
	s.started = append(s.started, jobs)
	return nil
}
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

func newTestListener() (*scaleSetListener, *stubScaleSetHelper) {
	helper := &stubScaleSetHelper{
		scaleSet: params.ScaleSet{
			ID:           7,
			ScaleSetID:   42,
			Name:         "test-scaleset",
			ProviderName: "test-provider",
		},
	}
	return newListener(context.Background(), helper), helper
}

// GitHub attaches its statistics to every message, whatever the type. They
// are the only ungated view of the scale set's queue (TotalAvailableJobs is
// not capped by the capacity we advertise), so they must be persisted even
// for messages the listener otherwise ignores. Otherwise the API serves a
// stale number for as long as the scale set sees nothing but such messages.
func TestHandleSessionMessagePersistsStatisticsForNonJobMessages(t *testing.T) {
	l, helper := newTestListener()

	l.handleSessionMessage(params.RunnerScaleSetMessage{
		MessageID:   1,
		MessageType: "SomeOtherMessageType",
		Statistics: &params.RunnerScaleSetStatistic{
			TotalAvailableJobs: 99,
			TotalAssignedJobs:  10,
			TotalIdleRunners:   3,
		},
	})

	require.Len(t, helper.statistics, 1)
	got := helper.statistics[0]
	require.Equal(t, 99, got.TotalAvailableJobs)
	require.Equal(t, 10, got.TotalAssignedJobs)
	require.Equal(t, 3, got.TotalIdleRunners)
	require.Empty(t, helper.started, "a non job message must not be handled as jobs")
}

func TestHandleSessionMessageNilStatisticsIsNoop(t *testing.T) {
	l, helper := newTestListener()

	l.handleSessionMessage(params.RunnerScaleSetMessage{
		MessageID:   1,
		MessageType: "SomeOtherMessageType",
	})

	require.Empty(t, helper.statistics)
}

func TestRecordStatistics(t *testing.T) {
	metrics.ScaleSetAvailableJobs.Reset()
	metrics.ScaleSetAssignedJobs.Reset()
	metrics.ScaleSetIdleRunners.Reset()

	l, _ := newTestListener()
	l.recordStatistics(&params.RunnerScaleSetStatistic{
		TotalAvailableJobs: 250,
		TotalAssignedJobs:  10,
		TotalIdleRunners:   3,
	})

	lbls := []string{"test-scaleset", "test-provider"}

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

	l, _ := newTestListener()
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

	l, _ := newTestListener()
	l.handleSessionMessage(params.RunnerScaleSetMessage{
		MessageID:   1,
		MessageType: "SomeOtherMessageType",
		Statistics: &params.RunnerScaleSetStatistic{
			TotalAvailableJobs: 99,
		},
	})

	if got := gaugeValue(t, metrics.ScaleSetAvailableJobs, "test-scaleset", "test-provider"); got != 99 {
		t.Errorf("available_jobs = %v, want 99", got)
	}
}
