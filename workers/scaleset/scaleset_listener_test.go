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

	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/util/github/scalesets"
)

// stubScaleSetHelper implements just enough of scaleSetHelper to exercise
// statistics handling, and records what was persisted.
type stubScaleSetHelper struct {
	scaleSet params.ScaleSet

	statistics []params.RunnerScaleSetStatistic
}

func (s *stubScaleSetHelper) GetScaleSet() params.ScaleSet { return s.scaleSet }
func (s *stubScaleSetHelper) GetScaleSetClient() (*scalesets.ScaleSetClient, error) {
	return nil, nil
}
func (s *stubScaleSetHelper) SetLastMessageID(_ int64) error    { return nil }
func (s *stubScaleSetHelper) SetDesiredRunnerCount(_ int) error { return nil }
func (s *stubScaleSetHelper) SetRunnerStatistics(stats params.RunnerScaleSetStatistic) error {
	s.statistics = append(s.statistics, stats)
	return nil
}
func (s *stubScaleSetHelper) Owner() string                                           { return "test-owner" }
func (s *stubScaleSetHelper) HandleJobsCompleted(_ []params.ScaleSetJobMessage) error { return nil }
func (s *stubScaleSetHelper) HandleJobsStarted(_ []params.ScaleSetJobMessage) error   { return nil }
func (s *stubScaleSetHelper) HandleJobsAvailable(_ []params.ScaleSetJobMessage) error { return nil }

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

// The garm_scaleset_gh_* gauges are collected from the statistics stored on the
// scale set, so statistics must be persisted even for messages we otherwise
// drop on the floor. Otherwise the gauges go stale for as long as the scale set
// sees nothing but messages of a type we do not act on.
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

	if len(helper.statistics) != 1 {
		t.Fatalf("expected statistics to be persisted once, got %d", len(helper.statistics))
	}
	got := helper.statistics[0]
	if got.TotalAvailableJobs != 99 {
		t.Errorf("TotalAvailableJobs = %d, want 99", got.TotalAvailableJobs)
	}
	if got.TotalAssignedJobs != 10 {
		t.Errorf("TotalAssignedJobs = %d, want 10", got.TotalAssignedJobs)
	}
	if got.TotalIdleRunners != 3 {
		t.Errorf("TotalIdleRunners = %d, want 3", got.TotalIdleRunners)
	}
}

func TestHandleSessionMessageNilStatisticsIsNoop(t *testing.T) {
	l, helper := newTestListener()

	l.handleSessionMessage(params.RunnerScaleSetMessage{
		MessageID:   1,
		MessageType: "SomeOtherMessageType",
	})

	if len(helper.statistics) != 0 {
		t.Errorf("expected no statistics to be persisted, got %v", helper.statistics)
	}
}
