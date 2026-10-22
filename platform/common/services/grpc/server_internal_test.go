/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/testpb"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
)

type blockingEmptyService struct {
	testpb.UnimplementedEmptyServiceServer
	entered, release chan struct{}
}

func (s blockingEmptyService) EmptyCall(context.Context, *testpb.Empty) (*testpb.Empty, error) {
	close(s.entered)
	<-s.release
	return &testpb.Empty{}, nil
}

func TestStopGivesUpOnHandlerIgnoringCancellation(t *testing.T) { //nolint:paralleltest // overrides stopTimeout
	oldTimeout := stopTimeout
	stopTimeout = 50 * time.Millisecond
	t.Cleanup(func() { stopTimeout = oldTimeout })

	s, err := NewGRPCServer("127.0.0.1:0", ServerConfig{})
	require.NoError(t, err)
	svc := blockingEmptyService{entered: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(svc.release) })
	testpb.RegisterEmptyServiceServer(s.Server(), svc)
	go func() { _ = s.Start() }()

	cc, err := grpc.NewClient(s.Address(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })
	go func() { _, _ = testpb.NewEmptyServiceClient(cc).EmptyCall(t.Context(), &testpb.Empty{}) }()
	<-svc.entered

	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not give up on a handler that ignores cancellation")
	}
}

// The gRPC listener interprets RequireClientCert and ClientRootCAs exactly as
// SecureOptions.TLSConfig does for the web listener, so one configuration means one thing
// on both.
func TestNewGRPCServerClientAuth(t *testing.T) {
	t.Parallel()
	cert, key, ca := testKeyPair(t)
	for _, tc := range []struct {
		name    string
		require bool
		roots   [][]byte
		want    tls.ClientAuthType
	}{
		{name: "required", require: true, roots: [][]byte{ca}, want: tls.RequireAndVerifyClientCert},
		{name: "root CAs alone verify if given", roots: [][]byte{ca}, want: tls.VerifyClientCertIfGiven},
		{name: "neither asks for none", want: tls.NoClientCert},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			so := SecureOptions{
				UseTLS: true, Certificate: cert, Key: key,
				RequireClientCert: tc.require, ClientRootCAs: tc.roots,
			}
			srv := newClientAuthServer(t, so)
			cfg := srv.tls.Config()
			require.Equal(t, tc.want, cfg.ClientAuth)
			require.Equal(t, len(tc.roots) > 0, cfg.ClientCAs != nil)

			webCfg, err := so.TLSConfig()
			require.NoError(t, err)
			require.Equal(t, webCfg.ClientAuth, cfg.ClientAuth, "gRPC and web listeners must agree")
		})
	}
}

// With ClientRootCAs but no RequireClientCert, an offered certificate is verified: one from
// a trusted CA is accepted, one from an untrusted CA fails the handshake.
func TestNewGRPCServerVerifiesOfferedCert(t *testing.T) {
	t.Parallel()
	cert, key, ca := testKeyPair(t)
	trusted, err := tlsgen.NewCA()
	require.NoError(t, err)
	srv := newClientAuthServer(t, SecureOptions{
		UseTLS: true, Certificate: cert, Key: key, ClientRootCAs: [][]byte{trusted.CertBytes()},
	})
	go func() { _ = srv.Start() }()
	t.Cleanup(srv.Stop)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(ca))
	dial := func(clientCA tlsgen.CA) error {
		kp, err := clientCA.NewClientCertKeyPair()
		require.NoError(t, err)
		clientCert, err := tls.X509KeyPair(kp.Cert, kp.Key)
		require.NoError(t, err)
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", srv.Address(), &tls.Config{
			RootCAs: roots, ServerName: "127.0.0.1", NextProtos: []string{"h2"},
			MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12,
			// Certificates would be filtered against the server's acceptable CAs and not
			// sent; GetClientCertificate offers the certificate regardless.
			GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &clientCert, nil },
		})
		if err != nil {
			return err
		}
		return conn.Close()
	}

	require.NoError(t, dial(trusted))
	untrusted, err := tlsgen.NewCA()
	require.NoError(t, err)
	require.ErrorContains(t, dial(untrusted), "unknown certificate authority")
}

func newClientAuthServer(t *testing.T, so SecureOptions) *GRPCServer {
	t.Helper()
	srv, err := NewGRPCServer("127.0.0.1:0", ServerConfig{SecOpts: so})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Listener().Close() })
	return srv
}

// Stop releases the listen address even when Start never ran, so a node that fails to
// start can be started again on the same address.
func TestStopReleasesUnservedListener(t *testing.T) {
	t.Parallel()
	srv, err := NewGRPCServer("127.0.0.1:0", ServerConfig{})
	require.NoError(t, err)
	addr := srv.Address()

	srv.Stop()

	l, err := net.Listen("tcp", addr)
	require.NoError(t, err, "the address must be free after Stop")
	require.NoError(t, l.Close())
}
