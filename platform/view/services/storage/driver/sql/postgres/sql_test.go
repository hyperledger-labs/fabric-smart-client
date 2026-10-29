/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
)

// setupDB starts a postgres container in a go test environment.
// It registers the termination function of the container with testing.Cleanup.
func setupDB(tb testing.TB) string {
	tb.Helper()

	terminate, pgConnStr, err := StartPostgres(tb.Context(), ConfigFromEnv(), nil)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(terminate)

	return pgConnStr
}

func TestPostgres(t *testing.T) {
	t.Parallel()
	pgConnStr := setupDB(t)
	t.Log("postgres ready")

	cp := NewConfigProvider(drivertesting.MockConfig(Config{
		DataSource: pgConnStr,
	}))
	sqlcommon.TestCases(t, func(string) (driver.KeyValueStore, error) {
		return NewPersistenceWithOpts(cp, NewDbProvider(), "", NewKeyValueStore)
	}, nil, func(p driver.KeyValueStore) *sqlcommon.KeyValueStore {
		return p.(*KeyValueStore).KeyValueStore
	})
	t.Run("DBMonitor", func(t *testing.T) {
		t.Parallel()
		testDBMonitorExportsMetrics(t, pgConnStr)
	})
}

func setupDBWithTLS(tb testing.TB) (pgConnStr, caPath string) {
	tb.Helper()

	cfg := ConfigFromEnv()
	caPath, err := EnableTLS(cfg, tb.TempDir())
	require.NoError(tb, err)

	terminate, pgConnStr, err := StartPostgres(tb.Context(), cfg, &testLogger{tb})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(terminate)

	return pgConnStr, caPath
}

func TestPostgresWithTLS(t *testing.T) {
	t.Parallel()
	pgConnStr, caPath := setupDBWithTLS(t)
	t.Log("postgres with TLS ready")

	cp := NewConfigProvider(drivertesting.MockConfig(Config{
		DataSource: pgConnStr,
		TLSConfig: &TLSConfig{
			Enabled:      true,
			SSLMode:      "verify-ca",
			RootCertPath: caPath,
		},
	}))
	sqlcommon.TestCases(t, func(string) (driver.KeyValueStore, error) {
		return NewPersistenceWithOpts(cp, NewDbProvider(), "", NewKeyValueStore)
	}, nil, func(p driver.KeyValueStore) *sqlcommon.KeyValueStore {
		return p.(*KeyValueStore).KeyValueStore
	})
}

type testLogger struct {
	testing.TB
}

func (l *testLogger) Debugf(format string, args ...any) {
	l.Logf("[DEBUG] "+format, args...)
}

func (l *testLogger) Errorf(format string, args ...any) {
	l.Logf("[ERROR] "+format, args...)
}
