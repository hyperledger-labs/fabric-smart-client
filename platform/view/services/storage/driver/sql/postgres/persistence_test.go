/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freePort returns a local TCP port that nothing listens on.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	require.NoError(t, l.Close())
	return port
}

func TestOpen_FailsAfterTimeout(t *testing.T) {
	t.Parallel()
	dsn := "postgres://u:secret@127.0.0.1:" + freePort(t) + "/db?sslmode=disable"

	start := time.Now()
	_, err := Open(Opts{DataSource: dsn, StartupConnectTimeout: time.Second})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 2*time.Second)
	assert.GreaterOrEqual(t, elapsed, time.Second, "Open must keep retrying until the timeout")
	assert.Contains(t, err.Error(), dataSourceLabel(dsn))
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), dsn)
}

func TestOpen_WaitsForDatabasePostgres(t *testing.T) {
	t.Parallel()
	cfg := ConfigFromEnv()
	cfg.Port = freePort(t)

	started := make(chan error, 1)
	go func() {
		time.Sleep(3 * time.Second)
		terminate, _, err := StartPostgres(t.Context(), cfg, nil)
		if err == nil {
			t.Cleanup(terminate)
		}
		started <- err
	}()

	// Generous timeout: the container may take a while to become healthy on a loaded runner.
	dbs, err := Open(Opts{DataSource: cfg.DataSource(), StartupConnectTimeout: 2 * time.Minute})
	require.NoError(t, <-started)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbs.WriteDB.Close() })
	require.NoError(t, dbs.WriteDB.PingContext(t.Context()))
}
