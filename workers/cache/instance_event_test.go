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
package cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/cache"
	"github.com/cloudbase/garm/database/common"
	"github.com/cloudbase/garm/params"
)

func instanceEvent(op common.OperationType, instance params.Instance) common.ChangePayload {
	return common.ChangePayload{
		EntityType: common.InstanceEntityType,
		Operation:  op,
		Payload:    instance,
	}
}

func TestInstanceUpdateEventUpsertsCache(t *testing.T) {
	w := &Worker{ctx: context.Background()}
	instance := params.Instance{
		ID:     "id-upsert",
		Name:   "cache-test-upsert",
		Status: commonParams.InstanceRunning,
	}
	defer cache.DeleteInstanceCache(instance.Name)

	w.handleInstanceEvent(instanceEvent(common.UpdateOperation, instance))

	cached, ok := cache.GetInstanceCache(instance.Name)
	require.True(t, ok)
	require.Equal(t, instance, cached)
}

// TestInstanceDeletedStatusEvictsCache is a regression test for a cache
// leak: deletion emits update(status=deleted) followed by a delete event. If
// the delete event is lost, or observed before the update, the upsert
// re-inserted a tombstone with the terminal "deleted" status that nothing
// ever evicted (observed in prod: 819 phantom instances after 9 days).
// Updates carrying the terminal status must evict instead of upsert.
func TestInstanceDeletedStatusEvictsCache(t *testing.T) {
	w := &Worker{ctx: context.Background()}
	instance := params.Instance{
		ID:     "id-deleted",
		Name:   "cache-test-deleted",
		Status: commonParams.InstanceRunning,
	}
	defer cache.DeleteInstanceCache(instance.Name)

	w.handleInstanceEvent(instanceEvent(common.CreateOperation, instance))
	_, ok := cache.GetInstanceCache(instance.Name)
	require.True(t, ok)

	instance.Status = commonParams.InstanceDeleted
	w.handleInstanceEvent(instanceEvent(common.UpdateOperation, instance))

	_, ok = cache.GetInstanceCache(instance.Name)
	require.False(t, ok, "update with status=deleted must evict the cache entry")
}

// TestInstanceDeleteAfterDeletedUpdateStaysEvicted simulates the reordered
// sequence that leaked entries: delete observed first, then the late update
// with status=deleted. The entry must not survive.
func TestInstanceDeleteAfterDeletedUpdateStaysEvicted(t *testing.T) {
	w := &Worker{ctx: context.Background()}
	instance := params.Instance{
		ID:     "id-reorder",
		Name:   "cache-test-reorder",
		Status: commonParams.InstanceRunning,
	}
	defer cache.DeleteInstanceCache(instance.Name)

	w.handleInstanceEvent(instanceEvent(common.CreateOperation, instance))

	// Delete event first (reordered)...
	w.handleInstanceEvent(instanceEvent(common.DeleteOperation, instance))
	// ...then the late update carrying the terminal status.
	instance.Status = commonParams.InstanceDeleted
	w.handleInstanceEvent(instanceEvent(common.UpdateOperation, instance))

	_, ok := cache.GetInstanceCache(instance.Name)
	require.False(t, ok, "reordered delete/update must not leak a cache entry")
}
