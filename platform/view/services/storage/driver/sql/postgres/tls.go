/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// TLSConfig defines the configuration parameters for securing database connections.
type TLSConfig struct {
	Enabled      bool   `json:"enabled"        mapstructure:"enabled"        yaml:"enabled"`
	ServerName   string `json:"server_name"    mapstructure:"server_name"    yaml:"server_name"`
	CertPath     string `json:"cert_path"      mapstructure:"cert_path"      yaml:"cert_path"`
	KeyPath      string `json:"key_path"       mapstructure:"key_path"       yaml:"key_path"`
	RootCertPath string `json:"root_cert_path" mapstructure:"root_cert_path" yaml:"root_cert_path"`
	SSLMode      string `json:"ssl_mode"       mapstructure:"ssl_mode"       yaml:"ssl_mode"`
}

// createTLSConnConfig parses the datasource string and maps the libpq sslmode
// semantics onto Go's crypto/tls behaviour. The supported modes mirror libpq
// (https://www.postgresql.org/docs/current/libpq-ssl.html), except that an
// empty ssl_mode defaults to the strictest mode, verify-full, rather than to
// libpq's default of prefer.
//
// Every host of a multi-host datasource is tried in order, as pgx does for a
// datasource sslmode: each host gets the attempts its mode requires (TLS then
// plaintext for prefer, plaintext then TLS for allow), and its TLS config
// verifies against that host's name unless ServerName is set. A unix-socket
// host gets a single plaintext attempt, as in pgx. The settings pgx parses
// besides hosts and TLS, such as target_session_attrs, are kept.
func createTLSConnConfig(dataSource string, tlsCfg TLSConfig) (*pgx.ConnConfig, error) {
	connConfig, err := pgx.ParseConfig(dataSource)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse database datasource")
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if tlsCfg.RootCertPath != "" {
		caCert, err := os.ReadFile(tlsCfg.RootCertPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read root certificate")
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to append root certificate from PEM")
		}
		tlsConfig.RootCAs = caCertPool
	}

	if tlsCfg.CertPath != "" && tlsCfg.KeyPath != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertPath, tlsCfg.KeyPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load client key pair")
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// order lists, per host, whether each attempt uses TLS.
	var order []bool
	switch tlsCfg.SSLMode {
	case "disable":
		// No encryption.
		order = []bool{false}
	case "allow":
		// Try a plaintext connection first; fall back to TLS without verifying
		// the server certificate or hostname.
		tlsConfig.InsecureSkipVerify = true
		order = []bool{false, true}
	case "prefer":
		// Try TLS without verification first; fall back to a plaintext connection.
		tlsConfig.InsecureSkipVerify = true
		order = []bool{true, false}
	case "require":
		// Encrypt, but do not verify the server certificate or hostname.
		tlsConfig.InsecureSkipVerify = true
		order = []bool{true}
	case "verify-ca":
		// Verify the certificate chain against the CA, but not the hostname. Go
		// performs the default hostname check unless InsecureSkipVerify is set,
		// so we skip it and delegate the chain check to VerifyConnection.
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = verifyChain(tlsConfig.RootCAs)
		order = []bool{true}
	case "verify-full", "":
		// Verify both the certificate chain and the hostname (against ServerName).
		order = []bool{true}
	default:
		return nil, errors.Errorf("unsupported ssl mode: %s", tlsCfg.SSLMode)
	}

	// pgx.ParseConfig has already expanded every host into attempts derived
	// from the datasource's own sslmode (defaulting to prefer). Rebuild them
	// from the hosts so the mode above is authoritative.
	var attempts []*pgconn.FallbackConfig
	for _, host := range hosts(connConfig) {
		hostTLS := tlsConfig.Clone()
		hostTLS.ServerName = host.Host
		if tlsCfg.ServerName != "" {
			hostTLS.ServerName = tlsCfg.ServerName
		}
		hostOrder := order
		if network, _ := pgconn.NetworkAddress(host.Host, host.Port); network == "unix" {
			// Postgres does not offer TLS on a unix socket; pgx and libpq
			// connect to it in plaintext whatever the sslmode.
			hostOrder = []bool{false}
		}
		for _, useTLS := range hostOrder {
			attempt := &pgconn.FallbackConfig{Host: host.Host, Port: host.Port}
			if useTLS {
				attempt.TLSConfig = hostTLS
			}
			attempts = append(attempts, attempt)
		}
	}

	connConfig.Host = attempts[0].Host
	connConfig.Port = attempts[0].Port
	connConfig.TLSConfig = attempts[0].TLSConfig
	connConfig.Fallbacks = attempts[1:]

	return connConfig, nil
}

// hosts returns the hosts of the datasource in order. pgx stores the first
// attempt in Host/Port and the rest in Fallbacks, with consecutive entries for
// the same host when its sslmode tries it twice; those collapse into one, as
// does a host the datasource lists twice in a row.
func hosts(connConfig *pgx.ConnConfig) []*pgconn.FallbackConfig {
	all := append([]*pgconn.FallbackConfig{{Host: connConfig.Host, Port: connConfig.Port}}, connConfig.Fallbacks...)
	return slices.CompactFunc(all, func(a, b *pgconn.FallbackConfig) bool {
		return a.Host == b.Host && a.Port == b.Port
	})
}

// verifyChain returns a tls.Config.VerifyConnection callback that validates the
// server certificate chain against the given roots without checking the
// hostname (libpq "verify-ca" semantics).
func verifyChain(roots *x509.CertPool) func(tls.ConnectionState) error {
	return func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("no peer certificates presented")
		}
		opts := x509.VerifyOptions{
			Roots:         roots,
			Intermediates: x509.NewCertPool(),
		}
		for _, cert := range cs.PeerCertificates[1:] {
			opts.Intermediates.AddCert(cert)
		}
		if _, err := cs.PeerCertificates[0].Verify(opts); err != nil {
			return errors.Wrap(err, "failed to verify server certificate chain")
		}

		return nil
	}
}

// tlsConnKey identifies a registered TLS connection by its inputs. TLSConfig
// contains only comparable (bool/string) fields, so the composite is usable as
// a map key.
type tlsConnKey struct {
	dataSource string
	tlsCfg     TLSConfig
}

var (
	tlsConnMu    sync.Mutex
	tlsConnNames = map[tlsConnKey]string{}
)

// RegisterTLSConnection maps the sslmode onto a pgx connection config and
// registers it with the stdlib driver, returning the datasource name to use
// with sql.Open. For sslmode=disable it returns the original datasource
// unchanged, avoiding both the registration and any certificate file access.
//
// Registration is memoized per (dataSource, tlsCfg): identical inputs return
// the same stdlib name on every call. This keeps the returned name stable so
// the DB provider's connection-pool cache (keyed on the datasource) still
// deduplicates stores that share a database, and bounds the number of
// registered configs to one per distinct configuration.
func RegisterTLSConnection(dataSource string, tlsCfg TLSConfig) (string, error) {
	if tlsCfg.SSLMode == "disable" {
		return dataSource, nil
	}

	key := tlsConnKey{dataSource: dataSource, tlsCfg: tlsCfg}

	tlsConnMu.Lock()
	defer tlsConnMu.Unlock()

	if name, ok := tlsConnNames[key]; ok {
		return name, nil
	}

	connConfig, err := createTLSConnConfig(dataSource, tlsCfg)
	if err != nil {
		return "", err
	}

	name := stdlib.RegisterConnConfig(connConfig)
	tlsConnNames[key] = name

	return name, nil
}

// registeredDataSource returns the datasource that RegisterTLSConnection registered
// under name.
func registeredDataSource(name string) (string, bool) {
	tlsConnMu.Lock()
	defer tlsConnMu.Unlock()
	for k, n := range tlsConnNames {
		if n == name {
			return k.dataSource, true
		}
	}
	return "", false
}
