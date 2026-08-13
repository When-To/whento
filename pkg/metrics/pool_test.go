// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPoolCollector(t *testing.T) {
	tests := []struct {
		name   string
		stats  PoolStats
		metric string
		want   string
	}{
		{
			name:   "pool size",
			stats:  PoolStats{MaxConns: 25},
			metric: "whento_db_pool_max_connections",
			want: `# HELP whento_db_pool_max_connections Maximum size of the pool.
# TYPE whento_db_pool_max_connections gauge
whento_db_pool_max_connections 25
`,
		},
		{
			name:   "connections in use",
			stats:  PoolStats{AcquiredConns: 7},
			metric: "whento_db_pool_acquired_connections",
			want: `# HELP whento_db_pool_acquired_connections Connections currently checked out by a caller.
# TYPE whento_db_pool_acquired_connections gauge
whento_db_pool_acquired_connections 7
`,
		},
		{
			name:   "idle connections",
			stats:  PoolStats{IdleConns: 3},
			metric: "whento_db_pool_idle_connections",
			want: `# HELP whento_db_pool_idle_connections Connections currently idle in the pool.
# TYPE whento_db_pool_idle_connections gauge
whento_db_pool_idle_connections 3
`,
		},
		{
			// The one that says the pool is the bottleneck: acquisitions that
			// had to wait because every connection was taken.
			name:   "starved acquisitions",
			stats:  PoolStats{EmptyAcquireCount: 12},
			metric: "whento_db_pool_empty_acquires_total",
			want: `# HELP whento_db_pool_empty_acquires_total Cumulative number of acquisitions that had to wait for the pool to free a connection.
# TYPE whento_db_pool_empty_acquires_total counter
whento_db_pool_empty_acquires_total 12
`,
		},
		{
			name:   "time spent waiting is exported in seconds",
			stats:  PoolStats{AcquireDuration: 1500 * time.Millisecond},
			metric: "whento_db_pool_acquire_duration_seconds_total",
			want: `# HELP whento_db_pool_acquire_duration_seconds_total Cumulative time spent waiting for a connection, in seconds.
# TYPE whento_db_pool_acquire_duration_seconds_total counter
whento_db_pool_acquire_duration_seconds_total 1.5
`,
		},
		{
			name:   "connections closed for old age",
			stats:  PoolStats{MaxLifetimeDestroyCount: 4},
			metric: "whento_db_pool_max_lifetime_destroys_total",
			want: `# HELP whento_db_pool_max_lifetime_destroys_total Cumulative number of connections closed for reaching their maximum lifetime.
# TYPE whento_db_pool_max_lifetime_destroys_total counter
whento_db_pool_max_lifetime_destroys_total 4
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewPoolCollector(func() PoolStats { return tt.stats })

			if err := testutil.CollectAndCompare(c, strings.NewReader(tt.want), tt.metric); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestPoolCollectorPublishesEveryStat guards against a field being added to
// PoolStats and never wired to a Desc.
func TestPoolCollectorPublishesEveryStat(t *testing.T) {
	c := NewPoolCollector(func() PoolStats { return PoolStats{} })

	const want = 12
	if got := testutil.CollectAndCount(c); got != want {
		t.Errorf("the collector publishes %d metrics, want %d", got, want)
	}
}

// TestPoolCollectorReadsAtScrapeTime is the reason the collector takes a
// function: the numbers must be the pool's at the moment Prometheus asked, not
// at the moment it was registered.
func TestPoolCollectorReadsAtScrapeTime(t *testing.T) {
	stats := PoolStats{TotalConns: 1}
	c := NewPoolCollector(func() PoolStats { return stats })

	if got := testutil.ToFloat64(collectOne(t, c, "whento_db_pool_total_connections")); got != 1 {
		t.Fatalf("total_connections = %v, want 1", got)
	}

	stats.TotalConns = 9
	if got := testutil.ToFloat64(collectOne(t, c, "whento_db_pool_total_connections")); got != 9 {
		t.Errorf("total_connections = %v, want 9 after the pool grew", got)
	}
}

// TestPoolCollectorWithoutASource covers the defensive path: a scrape must
// never be able to panic the process.
func TestPoolCollectorWithoutASource(t *testing.T) {
	if got := testutil.CollectAndCount(NewPoolCollector(nil)); got != 0 {
		t.Errorf("a collector with no source published %d metrics, want 0", got)
	}
}

func TestRegisterPoolRefusesADuplicate(t *testing.T) {
	stats := func() PoolStats { return PoolStats{} }

	reg := prometheus.NewRegistry()
	if err := reg.Register(NewPoolCollector(stats)); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := reg.Register(NewPoolCollector(stats)); err == nil {
		t.Error("registering a second pool collector succeeded, want a duplicate error")
	}
}

// TestRegisterPoolOnTheDefaultRegistry checks the exported path, which is what
// cmd/main.go calls.
func TestRegisterPoolOnTheDefaultRegistry(t *testing.T) {
	if err := RegisterPool(func() PoolStats { return PoolStats{MaxConns: 25} }); err != nil {
		t.Fatalf("RegisterPool: %v", err)
	}
	t.Cleanup(func() { registry.Unregister(NewPoolCollector(nil)) })

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var found bool
	for _, family := range families {
		if family.GetName() != "whento_db_pool_max_connections" {
			continue
		}
		found = true
		for _, metric := range family.GetMetric() {
			// A connection is not a person: these carry no labels at all.
			if len(metric.GetLabel()) != 0 {
				t.Errorf("pool metrics must carry no label, got %v", metric.GetLabel())
			}
			if metric.GetGauge().GetValue() != 25 {
				t.Errorf("max_connections = %v, want 25", metric.GetGauge().GetValue())
			}
		}
	}
	if !found {
		t.Error("the pool collector was registered but publishes nothing")
	}
}

// collectOne gathers a single named metric from a collector so it can be read
// with testutil.ToFloat64.
func collectOne(t *testing.T, c prometheus.Collector, name string) prometheus.Gauge {
	t.Helper()

	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	defer reg.Unregister(c)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe"})
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			g.Set(metric.GetGauge().GetValue())
			return g
		}
	}

	t.Fatalf("%s was not collected", name)
	return nil
}
