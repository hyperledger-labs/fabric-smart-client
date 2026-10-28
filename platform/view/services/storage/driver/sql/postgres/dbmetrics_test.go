/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	prom "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate/connstatetest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/prometheus"
)

func TestDataSourceLabel_StripsCredentials(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{
		"postgres://u:secret@h:5432/db",
		"host=h port=5432 user=u password=secret dbname=db",
	} {
		assert.Equal(t, "h:5432/db", dataSourceLabel(dsn), dsn)
	}
	assert.Equal(t, "unknown", dataSourceLabel("postgres://u:secret@h:notaport/db"))
}

func TestDataSourceLabel_ResolvesTLSRegisteredName(t *testing.T) {
	t.Parallel()

	name, err := RegisterTLSConnection("postgres://u:secret@h:5432/db", TLSConfig{Enabled: true, SSLMode: "require"})
	require.NoError(t, err)
	t.Cleanup(func() { stdlib.UnregisterConnConfig(name) })
	assert.Equal(t, "h:5432/db", dataSourceLabel(name))
}

const testLabel = "h:5432/db"

// newTestMetrics returns a database tracker recorded by rec, and a provider whose
// fsc_database_* gauges record into pool, keyed by metric name.
func newTestMetrics() (*connstate.Tracker, *connstatetest.Recorder, map[string]*mock.Gauge, metrics.Provider) {
	m, rec := connstatetest.NewMetrics()
	pool := map[string]*mock.Gauge{}
	p := &mock.Provider{}
	p.NewGaugeCalls(func(o metrics.GaugeOpts) metrics.Gauge {
		g := &mock.Gauge{}
		g.WithReturns(g)
		pool[o.Name] = g
		return g
	})
	return m.Tracker("", "", connstate.ComponentDatabase, testLabel), rec, pool, p
}

// runMonitor runs monitorDB until all expected pings happened and at least minStates
// states were recorded, then cancels it and waits for it to return.
func runMonitor(t *testing.T, interval time.Duration, minStates int, setup func(sqlmock.Sqlmock)) (*connstatetest.Recorder, map[string]*mock.Gauge) {
	t.Helper()
	db, m, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	setup(m)

	tr, rec, pool, p := newTestMetrics()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { monitorDB(ctx, db, testLabel, interval, tr, p); close(done) }()

	require.Eventually(t, func() bool {
		return m.ExpectationsWereMet() == nil && len(rec.States(testLabel)) >= minStates
	}, 5*time.Second, 5*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("monitorDB did not return after cancel")
	}
	return rec, pool
}

func TestDBMonitor_PingFailureReportsReconnecting(t *testing.T) {
	t.Parallel()
	rec, pool := runMonitor(t, 10*time.Millisecond, 3, func(m sqlmock.Sqlmock) {
		m.ExpectPing()
		m.ExpectPing().WillReturnError(errors.New("connection refused"))
		m.ExpectPing()
	})

	// Further pings after the expectations are met fail too (sqlmock has none left),
	// so only the prefix is deterministic.
	got := rec.States(testLabel)
	require.GreaterOrEqual(t, len(got), 4)
	assert.Equal(t, []connstate.State{connstate.Connected, connstate.Reconnecting, connstate.Connected}, got[:3])
	assert.Equal(t, connstate.Disconnected, got[len(got)-1])
	for _, name := range []string{"open_connections", "in_use", "idle", "wait_count", "wait_duration_seconds"} {
		require.Contains(t, pool, name)
		assert.Equal(t, []string{"data_source", testLabel}, pool[name].WithArgsForCall(0), name)
		assert.Positive(t, pool[name].SetCallCount(), name)
	}
}

func TestDBMonitor_StopsOnContextCancel(t *testing.T) {
	t.Parallel()
	rec, _ := runMonitor(t, time.Hour, 1, func(m sqlmock.Sqlmock) { m.ExpectPing() })
	assert.Equal(t, []connstate.State{connstate.Connected, connstate.Disconnected}, rec.States(testLabel))
}

// testDBMonitorExportsMetrics checks against the live database at dsn that a provider
// built by NewDbProviderWithMetrics exports the database connection state and pool
// gauges to Prometheus, labeled without credentials.
func testDBMonitorExportsMetrics(t *testing.T, dsn string) {
	t.Helper()
	cfg, err := pgconn.ParseConfig(dsn)
	require.NoError(t, err)
	label := dataSourceLabel(dsn)
	assert.NotContains(t, label, cfg.User)
	assert.NotContains(t, label, cfg.Password)

	pp := &prometheus.Provider{}
	_, err = NewDbProviderWithMetrics(connstate.NewMetrics(pp), pp).Get(Opts{DataSource: dsn, MaxOpenConns: 2, MaxIdleConns: 2})
	require.NoError(t, err)

	families := map[string]*dto.MetricFamily{}
	require.Eventually(t, func() bool {
		mfs, err := prom.DefaultGatherer.Gather()
		require.NoError(t, err)
		for _, mf := range mfs {
			families[mf.GetName()] = mf
		}
		return gaugeValue(families["fsc_connection_state"], "endpoint", label) == float64(connstate.Connected) &&
			gaugeValue(families["fsc_database_open_connections"], "data_source", label) >= 1
	}, 5*time.Second, 50*time.Millisecond)

	for _, name := range []string{"fsc_database_in_use", "fsc_database_idle", "fsc_database_wait_count", "fsc_database_wait_duration_seconds"} {
		assert.GreaterOrEqual(t, gaugeValue(families[name], "data_source", label), 0.0, name)
	}
}

// gaugeValue returns the value of the series of mf whose label name equals value, or -1.
func gaugeValue(mf *dto.MetricFamily, name, value string) float64 {
	for _, m := range mf.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == name && l.GetValue() == value {
				return m.GetGauge().GetValue()
			}
		}
	}
	return -1
}

func TestDBMonitor_BusyPoolStaysConnected(t *testing.T) {
	t.Parallel()
	db, m, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	m.ExpectPing()

	tr, rec, pool, p := newTestMetrics()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { monitorDB(ctx, db, testLabel, 10*time.Millisecond, tr, p); close(done) }()
	require.Eventually(t, func() bool { return len(rec.States(testLabel)) == 1 }, 5*time.Second, time.Millisecond)

	// Hold the only connection; without the skip, the next ping would time out.
	conn, err := db.Conn(t.Context())
	require.NoError(t, err)
	n := pool["in_use"].SetCallCount()
	require.Eventually(t, func() bool { return pool["in_use"].SetCallCount() > n+3 }, 5*time.Second, time.Millisecond)
	require.NoError(t, conn.Close())
	cancel()
	<-done
	assert.Equal(t, []connstate.State{connstate.Connected, connstate.Disconnected}, rec.States(testLabel))
}
