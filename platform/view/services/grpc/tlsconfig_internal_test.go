/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
)

func testKeyPair(t *testing.T) (certPEM, keyPEM, caPEM []byte) {
	t.Helper()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	return kp.Cert, kp.Key, ca.CertBytes()
}

func TestTLSConfig(t *testing.T) {
	t.Parallel()
	cert, key, ca := testKeyPair(t)
	_, foreignKey, _ := testKeyPair(t)
	enabled := func(extra func(*SecureOptions)) SecureOptions {
		so := SecureOptions{UseTLS: true, Certificate: cert, Key: key}
		if extra != nil {
			extra(&so)
		}
		return so
	}

	for _, tc := range []struct {
		name    string
		opts    SecureOptions
		wantErr string
		check   func(*testing.T, *tls.Config)
	}{{
		name: "disabled yields no config at all",
		opts: SecureOptions{UseTLS: false},
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.Nil(t, cfg)
		},
	}, {
		name: "RequireClientCert demands and verifies a client certificate",
		opts: enabled(func(so *SecureOptions) {
			so.RequireClientCert, so.ClientRootCAs = true, [][]byte{ca}
		}),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
			require.NotNil(t, cfg.ClientCAs)
			require.Len(t, cfg.Certificates, 1)
		},
	}, {
		// The web listener's third state: do not demand a client certificate, but verify
		// one if offered. Losing this silently stops it verifying optional client certs.
		name: "client root CAs alone verify a certificate only if offered",
		opts: enabled(func(so *SecureOptions) { so.ClientRootCAs = [][]byte{ca} }),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.Equal(t, tls.VerifyClientCertIfGiven, cfg.ClientAuth)
		},
	}, {
		name: "no client root CAs means no client auth",
		opts: enabled(nil),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.Equal(t, tls.NoClientCert, cfg.ClientAuth)
		},
	}, {
		name: "ServerNameOverride becomes the SNI name",
		opts: enabled(func(so *SecureOptions) {
			so.ServerRootCAs, so.ServerNameOverride = [][]byte{ca}, "orderer.example.com"
		}),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.Equal(t, "orderer.example.com", cfg.ServerName)
			require.NotNil(t, cfg.RootCAs)
		},
	}, {
		name: "TimeShift moves certificate validity into the past",
		opts: enabled(func(so *SecureOptions) { so.TimeShift = time.Hour }),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.NotNil(t, cfg.Time)
			require.WithinDuration(t, time.Now().Add(-time.Hour), cfg.Time(), time.Minute)
		},
	}, {
		// Static-RSA suites are dropped deliberately. Pin it so a future "restore the old
		// list" cannot happen quietly.
		name: "the default suites offer no static-RSA key exchange",
		opts: enabled(nil),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.NotEmpty(t, cfg.CipherSuites)
			require.NotContains(t, cfg.CipherSuites, uint16(tls.TLS_RSA_WITH_AES_128_GCM_SHA256))
			require.NotContains(t, cfg.CipherSuites, uint16(tls.TLS_RSA_WITH_AES_256_GCM_SHA384))
			require.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		},
	}, {
		name: "explicit cipher suites win over the defaults",
		opts: enabled(func(so *SecureOptions) {
			so.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
		}),
		check: func(t *testing.T, cfg *tls.Config) {
			t.Helper()
			require.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, cfg.CipherSuites)
		},
	}, {
		name:    "a root CA that is not PEM is rejected",
		opts:    enabled(func(so *SecureOptions) { so.ServerRootCAs = [][]byte{[]byte("not a pem block")} }),
		wantErr: "not a valid PEM block",
	}, {
		name:    "a mismatched keypair is rejected",
		opts:    SecureOptions{UseTLS: true, Certificate: cert, Key: foreignKey},
		wantErr: "failed loading TLS key pair",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := tc.opts.TLSConfig()
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			tc.check(t, cfg)
		})
	}
}
