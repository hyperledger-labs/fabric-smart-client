/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"os"

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
func createTLSConnConfig(dataSource string, tlsCfg TLSConfig) (*pgx.ConnConfig, error) {
	connConfig, err := pgx.ParseConfig(dataSource)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse database datasource")
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if tlsCfg.ServerName != "" {
		tlsConfig.ServerName = tlsCfg.ServerName
	} else {
		tlsConfig.ServerName = connConfig.Host
	}

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

	// pgx.ParseConfig has already derived TLSConfig and Fallbacks from the
	// datasource's own sslmode (defaulting to prefer). Reset them so the mode
	// below is authoritative.
	connConfig.TLSConfig = nil
	connConfig.Fallbacks = nil

	switch tlsCfg.SSLMode {
	case "disable":
		// No encryption.
	case "allow":
		// Try a plaintext connection first; fall back to TLS without verifying
		// the server certificate or hostname.
		tlsConfig.InsecureSkipVerify = true
		connConfig.Fallbacks = tlsFallback(connConfig, tlsConfig)
	case "prefer":
		// Try TLS without verification first; fall back to a plaintext connection.
		tlsConfig.InsecureSkipVerify = true
		connConfig.TLSConfig = tlsConfig
		connConfig.Fallbacks = plaintextFallback(connConfig)
	case "require":
		// Encrypt, but do not verify the server certificate or hostname.
		tlsConfig.InsecureSkipVerify = true
		connConfig.TLSConfig = tlsConfig
	case "verify-ca":
		// Verify the certificate chain against the CA, but not the hostname. Go
		// performs the default hostname check unless InsecureSkipVerify is set,
		// so we skip it and delegate the chain check to VerifyConnection.
		tlsConfig.InsecureSkipVerify = true
		tlsConfig.VerifyConnection = verifyChain(tlsConfig.RootCAs)
		connConfig.TLSConfig = tlsConfig
	case "verify-full", "":
		// Verify both the certificate chain and the hostname (against ServerName).
		connConfig.TLSConfig = tlsConfig
	default:
		return nil, errors.Errorf("unsupported ssl mode: %s", tlsCfg.SSLMode)
	}

	return connConfig, nil
}

// tlsFallback returns a fallback to the same host that upgrades to TLS, used to
// implement the sslmode=allow behaviour (plaintext primary, TLS fallback).
func tlsFallback(connConfig *pgx.ConnConfig, tlsConfig *tls.Config) []*pgconn.FallbackConfig {
	return []*pgconn.FallbackConfig{{
		Host:      connConfig.Host,
		Port:      connConfig.Port,
		TLSConfig: tlsConfig,
	}}
}

// plaintextFallback returns a fallback to the same host without TLS, used to
// implement the sslmode=prefer behaviour (TLS primary, plaintext fallback).
func plaintextFallback(connConfig *pgx.ConnConfig) []*pgconn.FallbackConfig {
	return []*pgconn.FallbackConfig{{
		Host:      connConfig.Host,
		Port:      connConfig.Port,
		TLSConfig: nil,
	}}
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

// RegisterTLSConnection maps the sslmode onto a pgx connection config and
// registers it with the stdlib driver, returning the datasource name to use
// with sql.Open. For sslmode=disable it returns the original datasource
// unchanged, avoiding both the registration and any certificate file access.
func RegisterTLSConnection(dataSource string, tlsCfg TLSConfig) (string, error) {
	if tlsCfg.SSLMode == "disable" {
		return dataSource, nil
	}

	connConfig, err := createTLSConnConfig(dataSource, tlsCfg)
	if err != nil {
		return "", err
	}

	return stdlib.RegisterConnConfig(connConfig), nil
}
