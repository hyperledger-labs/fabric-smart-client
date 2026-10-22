/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"database/sql"
	"net"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
)

// defaultPingInterval is how often monitorDB pings the database.
const defaultPingInterval = 10 * time.Second

// dataSourceLabel returns host:port/dbname for a URL or key/value DSN, or for the
// name RegisterTLSConnection returned for one. It never contains the user or the
// password, so it is safe as a metric label and in logs. It returns "unknown" if
// the DSN cannot be parsed.
func dataSourceLabel(dataSource string) string {
	if dsn, ok := registeredDataSource(dataSource); ok {
		dataSource = dsn
	}
	cfg, err := pgconn.ParseConfig(dataSource)
	if err != nil {
		return "unknown"
	}
	return net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))) + "/" + cfg.Database
}

// poolGauges exports sql.DBStats of one pool.
type poolGauges struct {
	openConnections, inUse, idle, waitCount, waitDuration metrics.Gauge
}

func newPoolGauges(p metrics.Provider, label string) *poolGauges {
	gauge := func(name, help string) metrics.Gauge {
		return p.NewGauge(metrics.GaugeOpts{
			Namespace:  "fsc",
			Subsystem:  "database",
			Name:       name,
			Help:       help,
			LabelNames: []string{"data_source"},
		}).With("data_source", label)
	}
	return &poolGauges{
		openConnections: gauge("open_connections", "Number of established connections, in use and idle."),
		inUse:           gauge("in_use", "Number of connections in use."),
		idle:            gauge("idle", "Number of idle connections."),
		waitCount:       gauge("wait_count", "Total number of times a caller waited for a connection."),
		waitDuration:    gauge("wait_duration_seconds", "Total time callers waited for a connection."),
	}
}

func (g *poolGauges) set(s sql.DBStats) {
	g.openConnections.Set(float64(s.OpenConnections))
	g.inUse.Set(float64(s.InUse))
	g.idle.Set(float64(s.Idle))
	g.waitCount.Set(float64(s.WaitCount))
	g.waitDuration.Set(s.WaitDuration.Seconds())
}

// monitorDB pings db every interval until ctx is done and reports the result to t:
// Connected after a successful ping that follows a failed one (or on the first ping),
// Reconnecting after each failed ping, and Disconnected when ctx is done. While all
// connections of a connected pool are in use, the ping is skipped and the state kept.
// Each tick also exports the pool statistics of db as fsc_database_* gauges labeled
// with label.
// monitorDB blocks; run it in its own goroutine.
func monitorDB(ctx context.Context, db *sql.DB, label string, interval time.Duration, t *connstate.Tracker, p metrics.Provider) {
	gauges := newPoolGauges(p, label)
	connected := false
	check := func() {
		s := db.Stats()
		gauges.set(s)
		if connected && s.MaxOpenConnections > 0 && s.InUse >= s.MaxOpenConnections {
			// Every connection is busy, so the database is reachable; a ping would only
			// queue behind the callers and could time out.
			return
		}
		pingCtx, cancel := context.WithTimeout(ctx, interval)
		err := db.PingContext(pingCtx)
		cancel()
		switch {
		case ctx.Err() != nil:
			// Shutting down: the ping failed because ctx was canceled.
		case err != nil:
			if connected {
				logger.Warnf("database [%s] unreachable: %v", label, err)
			}
			connected = false
			t.Set(connstate.Reconnecting)
		case !connected:
			connected = true
			t.Set(connstate.Connected)
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for check(); ; check() {
		select {
		case <-ctx.Done():
			t.Set(connstate.Disconnected)
			return
		case <-ticker.C:
		}
	}
}
