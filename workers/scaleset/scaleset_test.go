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
package scaleset

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	runnerErrors "github.com/cloudbase/garm-provider-common/errors"
	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/database/common/mocks"
	"github.com/cloudbase/garm/params"
)

func newTestWorker(store *mocks.Store) *Worker {
	return &Worker{
		ctx:     context.Background(),
		store:   store,
		runners: make(map[string]params.Instance),
	}
}

func TestMarkRunnerPendingDeleteUpdatesLocalState(t *testing.T) {
	store := mocks.NewStore(t)
	w := newTestWorker(store)

	updated := params.Instance{
		ID:     "some-id",
		Name:   "runner-1",
		Status: commonParams.InstancePendingDelete,
	}
	store.On("UpdateInstance", mock.Anything, "runner-1", params.UpdateInstanceParams{
		Status: commonParams.InstancePendingDelete,
	}).Return(updated, nil)

	require.NoError(t, w.markRunnerPendingDelete("runner-1"))
	require.Equal(t, updated, w.runners["some-id"])
}

// TestMarkRunnerPendingDeleteEvictsWhenDBRecordIsGone is a regression test:
// when the DB record was already deleted, the previous code swallowed
// ErrNotFound and stored the zero-value instance returned alongside it into
// w.runners. Entries with an empty status match no cleanup path, so each one
// permanently occupied a max_runners slot, starving scale-up (observed in
// prod as multi-minute waits for a runner on an otherwise idle system).
func TestMarkRunnerPendingDeleteEvictsWhenDBRecordIsGone(t *testing.T) {
	store := mocks.NewStore(t)
	w := newTestWorker(store)

	stale := params.Instance{
		ID:     "stale-id",
		Name:   "runner-1",
		Status: commonParams.InstanceRunning,
	}
	w.runners[stale.ID] = stale

	store.On("UpdateInstance", mock.Anything, "runner-1", mock.Anything).
		Return(params.Instance{}, fmt.Errorf("fetching instance: %w", runnerErrors.ErrNotFound))

	require.NoError(t, w.markRunnerPendingDelete("runner-1"))

	// The stale entry must be evicted and no zero-value entry stored.
	require.Empty(t, w.runners)
}

func TestMarkRunnerPendingDeletePropagatesOtherErrors(t *testing.T) {
	store := mocks.NewStore(t)
	w := newTestWorker(store)

	store.On("UpdateInstance", mock.Anything, "runner-1", mock.Anything).
		Return(params.Instance{}, fmt.Errorf("db is on fire"))

	err := w.markRunnerPendingDelete("runner-1")
	require.Error(t, err)
	require.Empty(t, w.runners)
}

func TestDeleteRunnerEntryByName(t *testing.T) {
	w := newTestWorker(nil)
	w.runners["id-1"] = params.Instance{ID: "id-1", Name: "runner-1"}
	w.runners["id-2"] = params.Instance{ID: "id-2", Name: "runner-2"}
	// A zero-value entry left over by the old bug: keyed by ID, empty name.
	w.runners["id-3"] = params.Instance{}

	w.deleteRunnerEntryByName("runner-1")
	require.Len(t, w.runners, 2)
	require.Contains(t, w.runners, "id-2")
	require.Contains(t, w.runners, "id-3")
}

// TestSyncRunnersFromDB asserts that consolidation resyncs the worker-local
// runner map from the database, dropping stale entries (leaked by missed or
// reordered watcher events) that would otherwise inflate runnerCount() and
// starve scale-up, and adopting DB records the worker never saw.
func TestSyncRunnersFromDB(t *testing.T) {
	store := mocks.NewStore(t)
	w := newTestWorker(store)
	w.offlineSince = map[string]time.Time{
		"runner-real":  time.Now(),
		"runner-stale": time.Now(),
	}

	stale := params.Instance{ID: "id-stale", Name: "runner-stale", Status: commonParams.InstanceRunning}
	zombie := params.Instance{} // zero-value entry left over by the old ErrNotFound bug
	real := params.Instance{ID: "id-real", Name: "runner-real", Status: commonParams.InstanceRunning}
	missed := params.Instance{ID: "id-missed", Name: "runner-missed", Status: commonParams.InstanceRunning}

	w.runners[stale.ID] = stale
	w.runners["id-zombie"] = zombie
	w.runners[real.ID] = real

	store.On("ListScaleSetInstances", mock.Anything, w.scaleSet.ID, false).
		Return([]params.Instance{real, missed}, nil)

	require.NoError(t, w.syncRunnersFromDB())

	require.Equal(t, map[string]params.Instance{
		real.ID:   real,
		missed.ID: missed,
	}, w.runners)
	require.Contains(t, w.offlineSince, "runner-real")
	require.NotContains(t, w.offlineSince, "runner-stale")
}
