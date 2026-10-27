/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/tlsgen"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

type mockConfig struct {
	values map[string]any
}

func (m *mockConfig) IsSet(key string) bool {
	_, ok := m.values[key]

	return ok
}

func (m *mockConfig) UnmarshalKey(key string, rawVal any) error {
	val, ok := m.values[key]
	if !ok {
		return nil
	}

	return decodeYAML(val, rawVal)
}

func (m *mockConfig) UnmarshalDriverOpts(name driver2.PersistenceName, v any) error {
	if opts, ok := m.values[string(name)+"Opts"]; ok {
		return decodeYAML(opts, v)
	}
	return nil
}

// decodeYAML mirrors the production config decoder
// (config.EnhancedExactUnmarshal), which decodes using the "yaml" struct tag.
// Using it here ensures the test exercises the same tag resolution as the real
// config path, so a missing/incorrect `yaml` tag (e.g. on Config.TLSConfig) is
// caught rather than masked by mapstructure's default field-name matching.
func decodeYAML(input, output any) error {
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "yaml",
		WeaklyTypedInput: true,
		Result:           output,
	})
	if err != nil {
		return err
	}

	return dec.Decode(input)
}

func generateSelfSignedCert(t *testing.T, tempDir string) (certPath, keyPath string) {
	t.Helper()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)

	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)

	certPath = filepath.Join(tempDir, "cert.pem")
	err = os.WriteFile(certPath, serverKeyPair.Cert, 0o644)
	require.NoError(t, err)

	keyPath = filepath.Join(tempDir, "key.pem")
	err = os.WriteFile(keyPath, serverKeyPair.Key, 0o600)
	require.NoError(t, err)

	return certPath, keyPath
}

func TestCreateTLSConnConfig(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	certPath, keyPath := generateSelfSignedCert(t, tempDir)

	tests := []struct {
		name       string
		dataSource string
		tlsCfg     TLSConfig
		verify     func(t *testing.T, connConfig *pgx.ConnConfig, err error)
	}{
		{
			// An empty ssl_mode intentionally defaults to the strictest mode
			// (verify-full): encrypted with full certificate and hostname checks.
			name:       "empty TLSConfig",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg:     TLSConfig{},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, connConfig.TLSConfig)
				assert.False(t, connConfig.TLSConfig.InsecureSkipVerify)
				assert.Nil(t, connConfig.TLSConfig.VerifyConnection)
				assert.Empty(t, connConfig.Fallbacks)
			},
		},
		{
			name:       "SSLMode disable",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode: "disable",
			},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Nil(t, connConfig.TLSConfig)
			},
		},
		{
			// libpq "require": encrypt, but do not verify the certificate or hostname.
			name:       "SSLMode require",
			dataSource: "postgres://postgres:password@localhost:5432/test",
			tlsCfg: TLSConfig{
				SSLMode: "require",
			},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, connConfig.TLSConfig)
				assert.True(t, connConfig.TLSConfig.InsecureSkipVerify)
				assert.Nil(t, connConfig.TLSConfig.VerifyConnection)
				assert.Empty(t, connConfig.Fallbacks)
				assert.Equal(t, "localhost", connConfig.TLSConfig.ServerName)
			},
		},
		{
			// libpq "allow": try plaintext first, fall back to TLS (no verification).
			name:       "SSLMode allow",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode: "allow",
			},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				assert.Nil(t, connConfig.TLSConfig)
				require.Len(t, connConfig.Fallbacks, 1)
				require.NotNil(t, connConfig.Fallbacks[0].TLSConfig)
				assert.True(t, connConfig.Fallbacks[0].TLSConfig.InsecureSkipVerify)
			},
		},
		{
			// libpq "prefer": try TLS (no verification) first, fall back to plaintext.
			name:       "SSLMode prefer",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode: "prefer",
			},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, connConfig.TLSConfig)
				assert.True(t, connConfig.TLSConfig.InsecureSkipVerify)
				require.Len(t, connConfig.Fallbacks, 1)
				assert.Nil(t, connConfig.Fallbacks[0].TLSConfig)
			},
		},
		{
			name:       "SSLMode verify-full with server name override",
			dataSource: "host=127.0.0.1 port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode:    "verify-full",
				ServerName: "custom.domain",
			},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, connConfig.TLSConfig)
				assert.False(t, connConfig.TLSConfig.InsecureSkipVerify)
				assert.Equal(t, "custom.domain", connConfig.TLSConfig.ServerName)
			},
		},
		{
			// libpq "verify-ca": verify the certificate chain against the CA, but
			// not the hostname. Go performs the default hostname check unless
			// InsecureSkipVerify is set, so verify-ca must set it and delegate the
			// chain check to a custom VerifyConnection callback.
			name:       "SSLMode verify-ca with Root CA and Client Certs",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode:      "verify-ca",
				RootCertPath: certPath,
				CertPath:     certPath,
				KeyPath:      keyPath,
			},
			verify: func(t *testing.T, connConfig *pgx.ConnConfig, err error) {
				t.Helper()
				require.NoError(t, err)
				require.NotNil(t, connConfig.TLSConfig)
				assert.True(t, connConfig.TLSConfig.InsecureSkipVerify)
				assert.NotNil(t, connConfig.TLSConfig.RootCAs)
				assert.Len(t, connConfig.TLSConfig.Certificates, 1)
				assert.NotNil(t, connConfig.TLSConfig.VerifyConnection)
				assert.Empty(t, connConfig.Fallbacks)

				// Test VerifyConnection callback
				cs := tls.ConnectionState{
					PeerCertificates: []*x509.Certificate{},
				}
				err = connConfig.TLSConfig.VerifyConnection(cs)
				assert.ErrorContains(t, err, "no peer certificates presented")
			},
		},
		{
			name:       "Invalid ssl mode",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode: "invalid-mode",
			},
			verify: func(t *testing.T, _ *pgx.ConnConfig, err error) {
				t.Helper()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported ssl mode")
			},
		},
		{
			name:       "Invalid certificate path",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode:      "verify-full",
				RootCertPath: filepath.Join(tempDir, "nonexistent.pem"),
			},
			verify: func(t *testing.T, _ *pgx.ConnConfig, err error) {
				t.Helper()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to read root certificate")
			},
		},
		{
			name:       "Invalid client key path",
			dataSource: "host=localhost port=5432 user=postgres dbname=test",
			tlsCfg: TLSConfig{
				SSLMode:  "verify-full",
				CertPath: certPath,
				KeyPath:  filepath.Join(tempDir, "nonexistent.pem"),
			},
			verify: func(t *testing.T, _ *pgx.ConnConfig, err error) {
				t.Helper()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to load client key pair")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			connConfig, err := createTLSConnConfig(tt.dataSource, tt.tlsCfg)
			tt.verify(t, connConfig, err)
		})
	}
}

// TestCreateTLSConnConfigMultiHost asserts that every host of a multi-host
// dataSource is tried, in order, with a TLS config whose ServerName is that
// host, and that prefer and allow give each host its TLS and plaintext attempt
// in the order pgx uses for a dataSource sslmode.
func TestCreateTLSConnConfigMultiHost(t *testing.T) {
	t.Parallel()

	type attempt struct {
		host       string
		port       uint16
		tls        bool
		serverName string
	}
	tlsTo := func(host string, port uint16) attempt { return attempt{host, port, true, host} }
	plain := func(host string, port uint16) attempt { return attempt{host: host, port: port} }

	const dataSource = "postgres://u:p@pg-a:5432,pg-b:5433/db?target_session_attrs=read-write"
	tests := []struct {
		sslMode string
		want    []attempt
	}{
		{"", []attempt{tlsTo("pg-a", 5432), tlsTo("pg-b", 5433)}},
		{"require", []attempt{tlsTo("pg-a", 5432), tlsTo("pg-b", 5433)}},
		{"verify-ca", []attempt{tlsTo("pg-a", 5432), tlsTo("pg-b", 5433)}},
		{"verify-full", []attempt{tlsTo("pg-a", 5432), tlsTo("pg-b", 5433)}},
		{"prefer", []attempt{tlsTo("pg-a", 5432), plain("pg-a", 5432), tlsTo("pg-b", 5433), plain("pg-b", 5433)}},
		{"allow", []attempt{plain("pg-a", 5432), tlsTo("pg-a", 5432), plain("pg-b", 5433), tlsTo("pg-b", 5433)}},
	}
	for _, tt := range tests {
		t.Run(tt.sslMode, func(t *testing.T) {
			t.Parallel()
			connConfig, err := createTLSConnConfig(dataSource, TLSConfig{SSLMode: tt.sslMode})
			require.NoError(t, err)

			got := []attempt{{connConfig.Host, connConfig.Port, connConfig.TLSConfig != nil, serverName(connConfig.TLSConfig)}}
			for _, f := range connConfig.Fallbacks {
				got = append(got, attempt{f.Host, f.Port, f.TLSConfig != nil, serverName(f.TLSConfig)})
			}
			assert.Equal(t, tt.want, got)
			assert.NotNil(t, connConfig.ValidateConnect, "target_session_attrs must be kept")
		})
	}

	t.Run("unix socket host stays plaintext", func(t *testing.T) {
		t.Parallel()
		connConfig, err := createTLSConnConfig("host=/var/run/postgresql,pg-b user=u dbname=db", TLSConfig{SSLMode: "prefer"})
		require.NoError(t, err)
		assert.Equal(t, "/var/run/postgresql", connConfig.Host)
		assert.Nil(t, connConfig.TLSConfig)
		require.Len(t, connConfig.Fallbacks, 2)
		assert.Equal(t, "pg-b", connConfig.Fallbacks[0].Host)
		require.NotNil(t, connConfig.Fallbacks[0].TLSConfig)
		assert.Nil(t, connConfig.Fallbacks[1].TLSConfig)
	})

	t.Run("server_name overrides every host", func(t *testing.T) {
		t.Parallel()
		connConfig, err := createTLSConnConfig(dataSource, TLSConfig{SSLMode: "verify-full", ServerName: "db.example.com"})
		require.NoError(t, err)
		require.NotNil(t, connConfig.TLSConfig)
		assert.Equal(t, "db.example.com", connConfig.TLSConfig.ServerName)
		require.Len(t, connConfig.Fallbacks, 1)
		assert.Equal(t, "pg-b", connConfig.Fallbacks[0].Host)
		assert.Equal(t, "db.example.com", connConfig.Fallbacks[0].TLSConfig.ServerName)
	})
}

// TestTLSConnConfigConnectsToSecondHost asserts that a connection built from a
// multi-host dataSource reaches the second host when the first one refuses
// the connection.
func TestTLSConnConfigConnectsToSecondHost(t *testing.T) {
	t.Parallel()

	// A port with nothing listening: the first host refuses the connection.
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	refusedPort := closed.Addr().(*net.TCPAddr).Port
	require.NoError(t, closed.Close())

	second, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = second.Close() }()
	reached := make(chan struct{})
	go func() {
		conn, err := second.Accept()
		if err != nil {
			return
		}
		close(reached)
		_ = conn.Close()
	}()

	dataSource := fmt.Sprintf("postgres://u:p@127.0.0.1:%d,127.0.0.1:%d/db?connect_timeout=5",
		refusedPort, second.Addr().(*net.TCPAddr).Port)
	connConfig, err := createTLSConnConfig(dataSource, TLSConfig{SSLMode: "require"})
	require.NoError(t, err)

	// The second host is not a Postgres server, so the connect itself fails.
	_, _ = pgconn.ConnectConfig(t.Context(), &connConfig.Config)
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the second host of the dataSource was never dialed")
	}
}

func serverName(c *tls.Config) string {
	if c == nil {
		return ""
	}
	return c.ServerName
}

// TestVerifyCAIgnoresHostname asserts that verify-ca validates the server
// certificate chain against the configured CA while ignoring a hostname
// mismatch (libpq "verify-ca" semantics), and still rejects a certificate that
// is not signed by that CA.
func TestVerifyCAIgnoresHostname(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	caPath := filepath.Join(tempDir, "ca.pem")
	require.NoError(t, os.WriteFile(caPath, ca.CertBytes(), 0o644))

	// Server certificate whose SAN ("db.internal") deliberately differs from the
	// connection host, so a hostname check would fail.
	serverKeyPair, err := ca.NewServerCertKeyPair("db.internal")
	require.NoError(t, err)

	connConfig, err := createTLSConnConfig(
		"host=127.0.0.1 port=5432 user=postgres dbname=test",
		TLSConfig{SSLMode: "verify-ca", RootCertPath: caPath},
	)
	require.NoError(t, err)
	require.NotNil(t, connConfig.TLSConfig.VerifyConnection)

	// Chain is valid; hostname differs from ServerName -> must still pass.
	err = connConfig.TLSConfig.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{serverKeyPair.TLSCert},
	})
	require.NoError(t, err)

	// A certificate signed by a different CA must be rejected.
	otherCA, err := tlsgen.NewCA()
	require.NoError(t, err)
	untrusted, err := otherCA.NewServerCertKeyPair("db.internal")
	require.NoError(t, err)
	err = connConfig.TLSConfig.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{untrusted.TLSCert},
	})
	require.Error(t, err)
}

// TestRegisterTLSConnectionMemoized asserts that registering the same
// (dataSource, TLSConfig) twice returns the same stdlib connection name. This
// keeps the DB provider's pool cache (keyed on the datasource) deduplicating
// stores that share a database, and prevents registrations from accumulating.
// Distinct inputs must yield distinct names.
func TestRegisterTLSConnectionMemoized(t *testing.T) { //nolint:paralleltest
	tempDir := t.TempDir()
	certPath, _ := generateSelfSignedCert(t, tempDir)

	const dataSource = "host=localhost port=5432 dbname=test"
	cfg := TLSConfig{Enabled: true, SSLMode: "verify-ca", RootCertPath: certPath}

	name1, err := RegisterTLSConnection(dataSource, cfg)
	require.NoError(t, err)
	assert.Contains(t, name1, "registeredConnConfig")
	t.Cleanup(func() { stdlib.UnregisterConnConfig(name1) })

	// Same inputs -> same registered name, no additional registration.
	name2, err := RegisterTLSConnection(dataSource, cfg)
	require.NoError(t, err)
	assert.Equal(t, name1, name2)

	// A different sslmode is a different configuration -> different name.
	otherMode := cfg
	otherMode.SSLMode = "require"
	name3, err := RegisterTLSConnection(dataSource, otherMode)
	require.NoError(t, err)
	t.Cleanup(func() { stdlib.UnregisterConnConfig(name3) })
	assert.NotEqual(t, name1, name3)

	// A different datasource is also a different configuration -> different name.
	name4, err := RegisterTLSConnection("host=otherhost port=5432 dbname=test", cfg)
	require.NoError(t, err)
	t.Cleanup(func() { stdlib.UnregisterConnConfig(name4) })
	assert.NotEqual(t, name1, name4)

	// sslmode=disable is never registered; the datasource is returned unchanged.
	name5, err := RegisterTLSConnection(dataSource, TLSConfig{Enabled: true, SSLMode: "disable"})
	require.NoError(t, err)
	assert.Equal(t, dataSource, name5)
}

func TestTLSConfigProvider(t *testing.T) { //nolint:paralleltest
	tempDir := t.TempDir()
	certPath, _ := generateSelfSignedCert(t, tempDir)

	// Keys use the same YAML names a user would write under
	// fsc.persistences.<name>.opts, so the test decodes through the real yaml
	// tags (in particular the `tls` block -> Config.TLSConfig).
	mockCfg := &mockConfig{
		values: map[string]any{
			"dbOpts": map[string]any{
				"dataSource": "host=localhost port=5432 dbname=test",
				"tls": map[string]any{
					"enabled":        true,
					"ssl_mode":       "require",
					"root_cert_path": certPath,
				},
			},
			"otherOpts": map[string]any{
				"dataSource": "host=localhost port=5432 dbname=test",
				"tls": map[string]any{
					"enabled":  true,
					"ssl_mode": "verify-full",
				},
			},
		},
	}

	t.Run("Persistence specific TLS config", func(t *testing.T) { //nolint:paralleltest
		provider := NewConfigProvider(mockCfg)

		opts, err := provider.GetOpts("db")
		require.NoError(t, err)
		assert.Contains(t, opts.DataSource, "registeredConnConfig")

		stdlib.UnregisterConnConfig(opts.DataSource)
	})

	t.Run("Other TLS config", func(t *testing.T) { //nolint:paralleltest
		provider := NewConfigProvider(mockCfg)

		opts, err := provider.GetOpts("other")
		require.NoError(t, err)
		assert.Contains(t, opts.DataSource, "registeredConnConfig")

		stdlib.UnregisterConnConfig(opts.DataSource)
	})
}
