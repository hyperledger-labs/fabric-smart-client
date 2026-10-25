/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
)

func TestMetricsClient(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("# TYPE up gauge\nup 1\n"))
	}))
	t.Cleanup(ts.Close)

	c, err := NewMetricsClient(&grpc.ConnectionConfig{Address: strings.TrimPrefix(ts.URL, "http://")})
	require.NoError(t, err)
	m, err := c.Metrics()
	require.NoError(t, err)
	require.InDelta(t, 1.0, m["up"].GetMetric()[0].GetGauge().GetValue(), 0)
}

func TestMetricsClientNon200(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "client certificate required", http.StatusUnauthorized)
	}))
	t.Cleanup(ts.Close)

	c, err := NewMetricsClient(&grpc.ConnectionConfig{Address: strings.TrimPrefix(ts.URL, "http://")})
	require.NoError(t, err)
	_, err = c.Metrics()
	require.ErrorContains(t, err, "401")
	require.ErrorContains(t, err, "client certificate required")
}

// The metrics endpoint is found where the node serves it: on fsc.metrics.address when set,
// and on the web listener otherwise.
func TestNewMetricsClientConfigFromFSC(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{name: "web listener", yaml: "fsc:\n  web:\n    address: 127.0.0.1:9000\n", want: "127.0.0.1:9000"},
		{name: "own address", yaml: "fsc:\n  web:\n    address: 127.0.0.1:9000\n  metrics:\n    address: 127.0.0.1:9443\n", want: "127.0.0.1:9443"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(tc.yaml), 0o600))
			cc, err := NewMetricsClientConfigFromFSC(dir)
			require.NoError(t, err)
			require.Equal(t, tc.want, cc.Address)
			require.False(t, cc.TLS.UseTLS)
		})
	}
}

// The metrics endpoint's own TLS applies when it has its own address: with TLS on the web
// listener and off on the metrics listener, the client dials the metrics listener in
// plaintext, and the web listener with TLS otherwise.
func TestNewMetricsClientConfigFromFSCUsesTheServingListenersTLS(t *testing.T) {
	t.Parallel()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.crt"), kp.Cert, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tls.key"), kp.Key, 0o600))
	webTLS := "fsc:\n  tls:\n    enabled: true\n    cert:\n      file: tls.crt\n    key:\n      file: tls.key\n  web:\n    address: 127.0.0.1:9000\n"

	write := func(yaml string) string {
		d := t.TempDir()
		for _, f := range []string{"tls.crt", "tls.key"} {
			raw, err := os.ReadFile(filepath.Join(dir, f))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(d, f), raw, 0o600))
		}
		require.NoError(t, os.WriteFile(filepath.Join(d, "core.yaml"), []byte(yaml), 0o600))
		return d
	}

	onWeb, err := NewMetricsClientConfigFromFSC(write(webTLS))
	require.NoError(t, err)
	require.True(t, onWeb.TLS.UseTLS, "the web listener's TLS")
	require.Equal(t, [][]byte{kp.Cert}, onWeb.TLS.ServerRootCAs)

	own, err := NewMetricsClientConfigFromFSC(write(webTLS + "  metrics:\n    address: 127.0.0.1:9443\n    tls:\n      enabled: false\n"))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:9443", own.Address)
	require.False(t, own.TLS.UseTLS, "the metrics listener's own TLS, not the web listener's")
}

// A TLS metrics endpoint that demands a client certificate is read with the node's own
// certificate.
func TestMetricsClientMutualTLS(t *testing.T) {
	t.Parallel()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	kp, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	cert, err := tls.X509KeyPair(kp.Cert, kp.Key)
	require.NoError(t, err)
	// The listener verifies clients against the CA that issued the node's certificate, as a
	// node's clientRootCAs does.
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(ca.CertBytes()))

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# TYPE up gauge\nup 1\n"))
	}))
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: roots, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS12}
	ts.StartTLS()
	t.Cleanup(ts.Close)

	cc, err := selfConnection(strings.TrimPrefix(ts.URL, "https://"), grpc.SecureOptions{UseTLS: true, Certificate: kp.Cert, Key: kp.Key})
	require.NoError(t, err)
	c, err := NewMetricsClient(cc)
	require.NoError(t, err)
	m, err := c.Metrics()
	require.NoError(t, err)
	require.Contains(t, m, "up")
}
