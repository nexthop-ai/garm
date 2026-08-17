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
)

var (
	// CacheInstancesCount tracks how many instances the in-memory cache
	// holds. Compare against garm_cache_db_instances_total: sustained
	// divergence means the event-driven cache is leaking or losing entries
	// (the UI dashboard and cache-backed views would report wrong counts).
	CacheInstancesCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsCacheSubsystem,
		Name:      "instances_total",
		Help:      "Number of instances currently held in the in-memory cache",
	})

	// CacheDBInstancesCount tracks how many instances exist in the database,
	// sampled at the same time as garm_cache_instances_total so the two can
	// be meaningfully compared.
	CacheDBInstancesCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsCacheSubsystem,
		Name:      "db_instances_total",
		Help:      "Number of instances in the database, sampled with the cache gauge",
	})

	// CacheInstancesPrunedCount counts cache entries evicted by the periodic
	// reconciliation because they no longer existed in the database. A
	// non-zero rate means instance delete events were missed.
	CacheInstancesPrunedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: metricsNamespace,
		Subsystem: metricsCacheSubsystem,
		Name:      "instances_pruned_total",
		Help:      "Total number of stale instances pruned from the cache by reconciliation",
	})
)
