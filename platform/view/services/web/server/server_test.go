/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server_test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tedsuo/ifrit"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/tlsgen"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations/fakes"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

const someURL = "/some-URL"

// tlsFixture holds a CA, server options that require a client certificate
// issued by it, and a client certificate that satisfies them.
type tlsFixture struct {
	ca         tlsgen.CA
	options    server.Options
	clientCert tls.Certificate
}

func newTLSFixture(t *testing.T) *tlsFixture {
	t.Helper()
	ca, err := tlsgen.NewCA()
	require.NoError(t, err)
	serverKeyPair, err := ca.NewServerCertKeyPair("127.0.0.1")
	require.NoError(t, err)
	clientKeyPair, err := ca.NewClientCertKeyPair()
	require.NoError(t, err)
	clientCert, err := tls.X509KeyPair(clientKeyPair.Cert, clientKeyPair.Key)
	require.NoError(t, err)

	return &tlsFixture{
		ca: ca,
		options: server.Options{
			ListenAddress: "127.0.0.1:0",
			TLS: grpc.SecureOptions{
				UseTLS:            true,
				Certificate:       serverKeyPair.Cert,
				Key:               serverKeyPair.Key,
				RequireClientCert: true,
				ClientRootCAs:     [][]byte{ca.CertBytes()},
			},
		},
		clientCert: clientCert,
	}
}

// client returns an HTTP client that trusts the fixture's CA and, if
// withClientCert is set, presents the fixture's client certificate.
func (f *tlsFixture) client(withClientCert bool) *http.Client {
	rootCAs := x509.NewCertPool()
	rootCAs.AppendCertsFromPEM(f.ca.CertBytes())
	tlsConfig := &tls.Config{RootCAs: rootCAs}
	if withClientCert {
		tlsConfig.Certificates = []tls.Certificate{f.clientCert}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
}

// newServer returns a server for options that is stopped when t ends.
func newServer(t *testing.T, options server.Options) *server.Server {
	t.Helper()
	srv := server.NewServer(options)
	t.Cleanup(func() { _ = srv.Stop() })
	return srv
}

func TestServer(t *testing.T) {
	t.Parallel()

	t.Run("rejects a client connecting without mutual TLS", func(t *testing.T) {
		t.Parallel()
		f := newTLSFixture(t)
		srv := newServer(t, f.options)
		require.NoError(t, srv.Start())

		resp, err := f.client(false).Get(fmt.Sprintf("https://%s%s", srv.Addr(), someURL)) //nolint:bodyclose // the handshake fails, so there is no body
		require.ErrorContains(t, err, "remote error: tls: certificate required")
		require.Nil(t, resp)
	})

	t.Run("hosts a secure endpoint for additional APIs when added", func(t *testing.T) {
		t.Parallel()
		f := newTLSFixture(t)
		srv := newServer(t, f.options)
		srv.RegisterHandler(someURL, &fakes.Handler{Code: http.StatusOK, Text: "secure"}, true)
		require.NoError(t, srv.Start())

		resp, err := f.client(true).Get(fmt.Sprintf("https://%s%s", srv.Addr(), someURL))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "secure", string(body))
	})

	t.Run("accepts clients without certificates when TLS is enabled without client authentication", func(t *testing.T) {
		t.Parallel()
		f := newTLSFixture(t)
		f.options.TLS.RequireClientCert = false
		f.options.TLS.ClientRootCAs = nil // no client CAs at all: NoClientCert
		srv := newServer(t, f.options)
		srv.RegisterHandler(someURL, &fakes.Handler{Code: http.StatusOK, Text: "open"}, false)
		require.NoError(t, srv.Start())

		resp, err := f.client(false).Get(fmt.Sprintf("https://%s%s", srv.Addr(), someURL))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("does not host an insecure endpoint for additional APIs by default when TLS is disabled", func(t *testing.T) {
		t.Parallel()
		f := newTLSFixture(t)
		f.options.TLS.UseTLS = false
		srv := newServer(t, f.options)
		require.NoError(t, srv.Start())

		resp, err := f.client(true).Get(fmt.Sprintf("http://%s%s", srv.Addr(), someURL))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("returns an error when listen fails", func(t *testing.T) {
		t.Parallel()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = listener.Close() })

		f := newTLSFixture(t)
		f.options.ListenAddress = listener.Addr().String()
		srv := newServer(t, f.options)
		require.ErrorContains(t, srv.Start(), "bind: address already in use")
	})

	// Certificate files are read and validated before the options reach this
	// package, so what can still fail here is material that does not parse.
	t.Run("returns an error when a bad TLS configuration is provided", func(t *testing.T) {
		t.Parallel()
		f := newTLSFixture(t)
		f.options.TLS.Certificate = []byte("not a certificate")
		srv := newServer(t, f.options)
		require.ErrorContains(t, srv.Start(), "failed loading TLS key pair")
	})

	t.Run("supports ifrit", func(t *testing.T) {
		t.Parallel()
		srv := newServer(t, newTLSFixture(t).options)
		process := ifrit.Invoke(srv)
		requireReceive(t, process.Ready())

		process.Signal(syscall.SIGTERM)
		require.NoError(t, requireReceive(t, process.Wait()))
	})

	t.Run("does not close the ready channel when start fails and ifrit is used", func(t *testing.T) {
		t.Parallel()
		f := newTLSFixture(t)
		f.options.TLS.Certificate = []byte("not a certificate")
		srv := newServer(t, f.options)
		process := ifrit.Invoke(srv)

		require.ErrorContains(t, requireReceive(t, process.Wait()), "failed loading TLS key pair")
		select {
		case <-process.Ready():
			t.Fatal("ready channel closed although start failed")
		default:
		}
	})
}

// requireReceive returns the next value from ch, or the zero value once ch is
// closed. It fails t if neither happens within five seconds.
func requireReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting on channel")
		var zero T
		return zero
	}
}
