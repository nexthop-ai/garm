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

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	commonParams "github.com/cloudbase/garm-provider-common/params"
	"github.com/cloudbase/garm/cache"
	"github.com/cloudbase/garm/database/common/mocks"
	"github.com/cloudbase/garm/params"
)

// TestReconcileInstanceCache asserts that reconciliation makes the instance
// cache self-healing: entries missing from the DB (leaked by missed delete
// events) are pruned and DB records missing from the cache (missed create
// events) are added back.
func TestReconcileInstanceCache(t *testing.T) {
	store := mocks.NewStore(t)
	w := &Worker{ctx: context.Background(), store: store}

	stale := params.Instance{ID: "id-stale", Name: "reconcile-stale", Status: commonParams.InstanceDeleted}
	kept := params.Instance{ID: "id-kept", Name: "reconcile-kept", Status: commonParams.InstanceRunning}
	missed := params.Instance{ID: "id-missed", Name: "reconcile-missed", Status: commonParams.InstanceRunning}
	defer func() {
		for _, name := range []string{stale.Name, kept.Name, missed.Name} {
			cache.DeleteInstanceCache(name)
		}
	}()

	// Cache holds a leaked tombstone and a live instance; the DB holds the
	// live instance plus one the cache never saw.
	cache.SetInstanceCache(stale)
	cache.SetInstanceCache(kept)

	store.On("ListAllInstances", mock.Anything).Return([]params.Instance{kept, missed}, nil)

	require.NoError(t, w.reconcileInstanceCache())

	_, ok := cache.GetInstanceCache(stale.Name)
	require.False(t, ok, "instance missing from the DB must be pruned from the cache")

	cached, ok := cache.GetInstanceCache(kept.Name)
	require.True(t, ok)
	require.Equal(t, kept, cached)

	cached, ok = cache.GetInstanceCache(missed.Name)
	require.True(t, ok, "instance present in the DB must be added to the cache")
	require.Equal(t, missed, cached)
}
