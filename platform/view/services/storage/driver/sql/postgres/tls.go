/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// TLSConfig defines the configuration parameters for securing database connections.
type TLSConfig struct {
	Enabled      bool   `json:"enabled"        mapstructure:"enabled"        yaml:"enabled"`
	ServerName   string `json:"server_name"    mapstructure:"server_name"    yaml:"server_name"`
	CertPath     string `json:"cert_path"      mapstructure:"cert_path"      yaml:"cert_path"`
	KeyPath      string `json:"key_path"       mapstructure:"key_path"       yaml:"key_path"`
	ClientCACert string `json:"client_ca_cert" mapstructure:"client_ca_cert" yaml:"client_ca_cert"`
	RootCertPath string `json:"root_cert_path" mapstructure:"root_cert_path" yaml:"root_cert_path"`
	SSLMode      string `json:"ssl_mode"       mapstructure:"ssl_mode"       yaml:"ssl_mode"`
}


// createTLSConnConfig parses the datasource string and configures standard Go TLS.
func createTLSConnConfig(dataSource string, tlsCfg TLSConfig) (*pgx.ConnConfig, error) {
	connConfig, err := pgx.ParseConfig(dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database datasource: %w", err)
	}

	tlsConfig := &tls.Config{}

	if tlsCfg.ServerName != "" {
		tlsConfig.ServerName = tlsCfg.ServerName
	} else {
		tlsConfig.ServerName = connConfig.Host
	}

	if tlsCfg.RootCertPath != "" {
		caCert, err := os.ReadFile(tlsCfg.RootCertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read root certificate: %w", err)
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
			return nil, fmt.Errorf("failed to load client key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	sslMode := tlsCfg.SSLMode
	switch sslMode {
	case "disable":
		connConfig.TLSConfig = nil
	case "allow", "prefer":
		connConfig.TLSConfig = tlsConfig
	case "require":
		// 'require' mode enforces TLS handshake; verification follows default behavior
		connConfig.TLSConfig = tlsConfig
	case "verify-ca":
		// 'verify-ca' mode uses provided CA for verification

		tlsConfig.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no peer certificates presented")
			}
			opts := x509.VerifyOptions{
				DNSName: "",
				Roots:   tlsConfig.RootCAs,
			}
			if len(cs.PeerCertificates) > 1 {
				opts.Intermediates = x509.NewCertPool()
				for _, cert := range cs.PeerCertificates[1:] {
					opts.Intermediates.AddCert(cert)
				}
			}
			_, err := cs.PeerCertificates[0].Verify(opts)

			return err
		}
		connConfig.TLSConfig = tlsConfig
	case "verify-full", "":
		// tlsConfig.InsecureSkipVerify defaults to false; no need to set explicitly
		connConfig.TLSConfig = tlsConfig
	default:
		return nil, fmt.Errorf("unsupported ssl mode: %s", sslMode)
	}

	return connConfig, nil
}

// RegisterTLSConnection parses the datasource string, configures standard Go TLS,
// and registers the customized pgx connection with the stdlib driver.
func RegisterTLSConnection(dataSource string, tlsCfg TLSConfig) (string, error) {
	connConfig, err := createTLSConnConfig(dataSource, tlsCfg)
	if err != nil {
		return "", err
	}
	if tlsCfg.SSLMode == "disable" {
		return dataSource, nil
	}

	connStr := stdlib.RegisterConnConfig(connConfig)

	return connStr, nil
}
