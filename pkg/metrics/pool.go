// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// PoolStats is a snapshot of a database connection pool, in the shape
// pgxpool.Stat exposes. It is a plain struct rather than the pgx type so that
// this package neither depends on pgx nor needs a live database to be tested;
// pkg/database owns the two-line adaptation.
type PoolStats struct {
	MaxConns          int32
	TotalConns        int32
	AcquiredConns     int32
	IdleConns         int32
	ConstructingConns int32

	AcquireCount            int64
	AcquireDuration         time.Duration
	EmptyAcquireCount       int64
	CanceledAcquireCount    int64
	NewConnsCount           int64
	MaxLifetimeDestroyCount int64
	MaxIdleDestroyCount     int64
}

// poolMetric is one published pool figure: its descriptor, whether it is a
// gauge or a counter, and how to read it from a snapshot.
type poolMetric struct {
	desc  *prometheus.Desc
	kind  prometheus.ValueType
	value func(PoolStats) float64
}

// poolCollector reads the pool at scrape time. Sampling on scrape rather than
// on a ticker means the numbers are the ones the pool held when Prometheus
// asked, and costs nothing while nobody is scraping.
type poolCollector struct {
	stats   func() PoolStats
	metrics []poolMetric
}

// NewPoolCollector builds a collector over a snapshot function. None of the
// metrics it publishes carries a label: there is one pool per process, and a
// connection is not a person.
func NewPoolCollector(stats func() PoolStats) prometheus.Collector {
	m := func(name, help string, kind prometheus.ValueType, value func(PoolStats) float64) poolMetric {
		desc := prometheus.NewDesc(prometheus.BuildFQName(namespace, "db_pool", name), help, nil, nil)
		return poolMetric{desc: desc, kind: kind, value: value}
	}
	gauge, counter := prometheus.GaugeValue, prometheus.CounterValue

	return &poolCollector{stats: stats, metrics: []poolMetric{
		m("max_connections", "Maximum size of the pool.", gauge,
			func(s PoolStats) float64 { return float64(s.MaxConns) }),
		m("total_connections", "Connections currently in the pool, idle plus acquired plus being constructed.", gauge,
			func(s PoolStats) float64 { return float64(s.TotalConns) }),
		m("acquired_connections", "Connections currently checked out by a caller.", gauge,
			func(s PoolStats) float64 { return float64(s.AcquiredConns) }),
		m("idle_connections", "Connections currently idle in the pool.", gauge,
			func(s PoolStats) float64 { return float64(s.IdleConns) }),
		m("constructing_connections", "Connections currently being established.", gauge,
			func(s PoolStats) float64 { return float64(s.ConstructingConns) }),

		m("acquires_total", "Cumulative number of successful connection acquisitions.", counter,
			func(s PoolStats) float64 { return float64(s.AcquireCount) }),
		m("acquire_duration_seconds_total", "Cumulative time spent waiting for a connection, in seconds.", counter,
			func(s PoolStats) float64 { return s.AcquireDuration.Seconds() }),
		m("empty_acquires_total", "Cumulative number of acquisitions that had to wait for the pool to free a connection.", counter,
			func(s PoolStats) float64 { return float64(s.EmptyAcquireCount) }),
		m("canceled_acquires_total", "Cumulative number of acquisitions abandoned because their context ended.", counter,
			func(s PoolStats) float64 { return float64(s.CanceledAcquireCount) }),
		m("new_connections_total", "Cumulative number of connections opened.", counter,
			func(s PoolStats) float64 { return float64(s.NewConnsCount) }),
		m("max_lifetime_destroys_total", "Cumulative number of connections closed for reaching their maximum lifetime.", counter,
			func(s PoolStats) float64 { return float64(s.MaxLifetimeDestroyCount) }),
		m("max_idle_destroys_total", "Cumulative number of connections closed for being idle too long.", counter,
			func(s PoolStats) float64 { return float64(s.MaxIdleDestroyCount) }),
	}}
}

// RegisterPool adds a pool collector to the registry. It returns an error if a
// pool has already been registered, which the caller may treat as
// non-fatal: metrics are worth less than the process staying up.
func RegisterPool(stats func() PoolStats) error {
	return registry.Register(NewPoolCollector(stats))
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m.desc
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.stats == nil {
		return
	}
	s := c.stats()
	for _, m := range c.metrics {
		ch <- prometheus.MustNewConstMetric(m.desc, m.kind, m.value(s))
	}
}
