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

// poolCollector reads the pool at scrape time. Sampling on scrape rather than
// on a ticker means the numbers are the ones the pool held when Prometheus
// asked, and costs nothing while nobody is scraping.
type poolCollector struct {
	stats func() PoolStats

	maxConns          *prometheus.Desc
	totalConns        *prometheus.Desc
	acquiredConns     *prometheus.Desc
	idleConns         *prometheus.Desc
	constructingConns *prometheus.Desc

	acquires         *prometheus.Desc
	acquireSeconds   *prometheus.Desc
	emptyAcquires    *prometheus.Desc
	canceledAcquires *prometheus.Desc
	newConns         *prometheus.Desc
	lifetimeDestroys *prometheus.Desc
	idleDestroys     *prometheus.Desc
}

// NewPoolCollector builds a collector over a snapshot function. None of the
// metrics it publishes carries a label: there is one pool per process, and a
// connection is not a person.
func NewPoolCollector(stats func() PoolStats) prometheus.Collector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, "db_pool", name), help, nil, nil)
	}

	return &poolCollector{
		stats:             stats,
		maxConns:          desc("max_connections", "Maximum size of the pool."),
		totalConns:        desc("total_connections", "Connections currently in the pool, idle plus acquired plus being constructed."),
		acquiredConns:     desc("acquired_connections", "Connections currently checked out by a caller."),
		idleConns:         desc("idle_connections", "Connections currently idle in the pool."),
		constructingConns: desc("constructing_connections", "Connections currently being established."),

		acquires:         desc("acquires_total", "Cumulative number of successful connection acquisitions."),
		acquireSeconds:   desc("acquire_duration_seconds_total", "Cumulative time spent waiting for a connection, in seconds."),
		emptyAcquires:    desc("empty_acquires_total", "Cumulative number of acquisitions that had to wait for the pool to free a connection."),
		canceledAcquires: desc("canceled_acquires_total", "Cumulative number of acquisitions abandoned because their context ended."),
		newConns:         desc("new_connections_total", "Cumulative number of connections opened."),
		lifetimeDestroys: desc("max_lifetime_destroys_total", "Cumulative number of connections closed for reaching their maximum lifetime."),
		idleDestroys:     desc("max_idle_destroys_total", "Cumulative number of connections closed for being idle too long."),
	}
}

// RegisterPool adds a pool collector to the registry. It returns an error if a
// pool has already been registered, which the caller may treat as
// non-fatal: metrics are worth less than the process staying up.
func RegisterPool(stats func() PoolStats) error {
	return registry.Register(NewPoolCollector(stats))
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range c.descs() {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.stats == nil {
		return
	}
	s := c.stats()

	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}

	gauge(c.maxConns, float64(s.MaxConns))
	gauge(c.totalConns, float64(s.TotalConns))
	gauge(c.acquiredConns, float64(s.AcquiredConns))
	gauge(c.idleConns, float64(s.IdleConns))
	gauge(c.constructingConns, float64(s.ConstructingConns))

	counter(c.acquires, float64(s.AcquireCount))
	counter(c.acquireSeconds, s.AcquireDuration.Seconds())
	counter(c.emptyAcquires, float64(s.EmptyAcquireCount))
	counter(c.canceledAcquires, float64(s.CanceledAcquireCount))
	counter(c.newConns, float64(s.NewConnsCount))
	counter(c.lifetimeDestroys, float64(s.MaxLifetimeDestroyCount))
	counter(c.idleDestroys, float64(s.MaxIdleDestroyCount))
}

func (c *poolCollector) descs() []*prometheus.Desc {
	return []*prometheus.Desc{
		c.maxConns, c.totalConns, c.acquiredConns, c.idleConns, c.constructingConns,
		c.acquires, c.acquireSeconds, c.emptyAcquires, c.canceledAcquires,
		c.newConns, c.lifetimeDestroys, c.idleDestroys,
	}
}
