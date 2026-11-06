/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"cmp"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
)

// TestClientStreamBindsServerCertificateToRemotePeerID dials peer V at a server presenting
// peer A's certificate, which the client trusts and which is valid for the dialed host. No
// stream to V may open, neither on a fresh connection nor on a cached connection to A.
func TestClientStreamBindsServerCertificateToRemotePeerID(t *testing.T) {
	t.Parallel()
	testSetup(t)

	serverTLSConfig, clientTLSConfig, clientID := testMutualTLSConfigs(t, false)
	peerA := serverPeerID(t, serverTLSConfig)
	vKey, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)
	vPub, err := x509.MarshalPKIXPublicKey(&vKey.PublicKey)
	require.NoError(t, err)
	vHash := sha256.Sum256(vPub)
	peerV := base64.StdEncoding.EncodeToString(vHash[:])

	srv := startTestServer(t, NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), serverTLSConfig, func(s host.P2PStream) { _ = s.Close() })
	t.Cleanup(srv.Close)
	info := func(peer host.PeerID) host.StreamInfo {
		return host.StreamInfo{
			RemotePeerID:      peer,
			RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"),
			ContextID:         "ctx",
			SessionID:         "session",
		}
	}

	t.Run("multiplexed", func(t *testing.T) {
		t.Parallel()
		p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
		t.Cleanup(func() { _ = p.Close() })

		_, err := p.NewClientStream(info(peerV), t.Context(), clientID, clientTLSConfig)
		require.ErrorContains(t, err, "peer identity binding failed")
		p.mu.RLock()
		require.Empty(t, p.clients, "a connection that failed the binding must not be cached")
		p.mu.RUnlock()

		// A cached connection to A must not carry a stream meant for V.
		s, err := p.NewClientStream(info(peerA), t.Context(), clientID, clientTLSConfig)
		require.NoError(t, err)
		t.Cleanup(func() { _ = s.Close() })
		_, err = p.NewClientStream(info(peerV), t.Context(), clientID, clientTLSConfig)
		require.ErrorContains(t, err, "peer identity binding failed")
	})

	t.Run("simple", func(t *testing.T) {
		t.Parallel()
		_, err := NewSimpleProvider().NewClientStream(info(peerV), t.Context(), clientID, clientTLSConfig)
		require.ErrorContains(t, err, "peer identity binding failed")
	})
}

// TestClientVerifiesServerChainWithoutHostName dials, on every dial path, servers whose
// certificates differ in one property each. The client matches no host name, so a server whose
// certificate names no host is reached, while the chain, the validity period, the server-auth key
// usage and the dialed peer ID still fail the handshake.
func TestClientVerifiesServerChainWithoutHostName(t *testing.T) {
	t.Parallel()
	testSetup(t)

	trusted := newTestCA(t)
	clientTLSConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{trusted.leaf(t, x509.ExtKeyUsageClientAuth)},
		RootCAs:      trusted.pool(),
	}
	clientCert, err := x509.ParseCertificate(clientTLSConfig.Certificates[0].Certificate[0])
	require.NoError(t, err)
	clientID, err := peerIDFromCertificate(clientCert)
	require.NoError(t, err)

	intermediate := trusted.intermediate(t)
	invalid := func(reason x509.InvalidReason) func(error) bool {
		return func(err error) bool {
			var e x509.CertificateInvalidError
			return errors.As(err, &e) && e.Reason == reason
		}
	}
	bindingFailed := func(err error) bool {
		return strings.Contains(err.Error(), "peer identity binding failed")
	}
	now := time.Now()

	tests := []struct {
		name   string
		cert   tls.Certificate
		dialed host.PeerID      // empty: the server's own peer ID
		reject func(error) bool // nil: the dial succeeds
	}{
		{name: "no SANs", cert: trusted.leaf(t, x509.ExtKeyUsageServerAuth)},
		{name: "via intermediate", cert: intermediate.leaf(t, x509.ExtKeyUsageServerAuth)},
		{name: "untrusted CA", cert: newTestCA(t).leaf(t, x509.ExtKeyUsageServerAuth), reject: func(err error) bool {
			var e x509.UnknownAuthorityError
			return errors.As(err, &e)
		}},
		{name: "client usage only", cert: trusted.leaf(t, x509.ExtKeyUsageClientAuth), reject: invalid(x509.IncompatibleUsage)},
		{name: "expired", cert: trusted.leafValid(t, x509.ExtKeyUsageServerAuth, now.Add(-2*time.Hour), now.Add(-time.Hour)), reject: invalid(x509.Expired)},
		{name: "not yet valid", cert: trusted.leafValid(t, x509.ExtKeyUsageServerAuth, now.Add(time.Hour), now.Add(2*time.Hour)), reject: invalid(x509.Expired)},
		{name: "another peer", cert: trusted.leaf(t, x509.ExtKeyUsageServerAuth), dialed: clientID, reject: bindingFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			serverTLSConfig := &tls.Config{
				MinVersion:   tls.VersionTLS13,
				MaxVersion:   tls.VersionTLS13,
				Certificates: []tls.Certificate{tt.cert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    trusted.pool(),
			}
			srv := startTestServer(t, NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), serverTLSConfig, func(s host.P2PStream) { _ = s.Close() })
			t.Cleanup(srv.Close)
			address := strings.TrimPrefix(srv.URL, "https://")
			info := host.StreamInfo{
				RemotePeerID:      cmp.Or(tt.dialed, serverPeerID(t, serverTLSConfig)),
				RemotePeerAddress: address,
				ContextID:         "ctx",
				SessionID:         "session",
			}
			check := func(t *testing.T, err error) {
				t.Helper()
				if tt.reject == nil {
					require.NoError(t, err)
					return
				}
				require.Error(t, err)
				require.True(t, tt.reject(err), "unexpected error: %v", err)
			}

			t.Run("multiplexed", func(t *testing.T) {
				t.Parallel()
				p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
				t.Cleanup(func() { _ = p.Close() })
				s, err := p.NewClientStream(info, t.Context(), clientID, clientTLSConfig)
				if err == nil {
					_ = s.Close()
				}
				check(t, err)
			})
			t.Run("simple", func(t *testing.T) {
				t.Parallel()
				s, err := NewSimpleProvider().NewClientStream(info, t.Context(), clientID, clientTLSConfig)
				if err == nil {
					_ = s.Close()
				}
				check(t, err)
			})
			t.Run("preamble", func(t *testing.T) {
				t.Parallel()
				_, err := ReadPreamble(t.Context(), info.RemotePeerID, address, clientTLSConfig)
				check(t, err)
			})
		})
	}
}

// TestClientRefusesAnotherPeerBeforeTheUpgrade dials peer V at a server presenting a trusted
// certificate of another peer. The handshake must fail, so the server never sees the upgrade
// request, on every dial path.
func TestClientRefusesAnotherPeerBeforeTheUpgrade(t *testing.T) {
	t.Parallel()
	testSetup(t)

	ca := newTestCA(t)
	clientCert := ca.leaf(t, x509.ExtKeyUsageClientAuth)
	parsed, err := x509.ParseCertificate(clientCert.Certificate[0])
	require.NoError(t, err)
	clientID, err := peerIDFromCertificate(parsed)
	require.NoError(t, err)
	clientTLSConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      ca.pool(),
	}

	dials := map[string]func(info host.StreamInfo) error{
		"multiplexed": func(info host.StreamInfo) error {
			p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
			defer func() { _ = p.Close() }()
			_, err := p.NewClientStream(info, t.Context(), clientID, clientTLSConfig)
			return err
		},
		"simple": func(info host.StreamInfo) error {
			_, err := NewSimpleProvider().NewClientStream(info, t.Context(), clientID, clientTLSConfig)
			return err
		},
		"preamble": func(info host.StreamInfo) error {
			_, err := ReadPreamble(t.Context(), info.RemotePeerID, info.RemotePeerAddress, clientTLSConfig)
			return err
		},
	}
	for path, dial := range dials {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				requests.Add(1)
			}))
			srv.TLS = &tls.Config{
				MinVersion:   tls.VersionTLS13,
				MaxVersion:   tls.VersionTLS13,
				Certificates: []tls.Certificate{ca.leaf(t, x509.ExtKeyUsageServerAuth)},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    ca.pool(),
			}
			srv.StartTLS()
			t.Cleanup(srv.Close)

			err := dial(host.StreamInfo{
				RemotePeerID:      clientID, // a trusted peer, but not the server
				RemotePeerAddress: strings.TrimPrefix(srv.URL, "https://"),
				ContextID:         "ctx",
				SessionID:         "session",
			})
			require.ErrorContains(t, err, "peer identity binding failed")
			require.Zero(t, requests.Load(), "the client sent the upgrade request to the wrong peer")
		})
	}
}

// TestVerifyServerUsesConfigTime checks that verifyServer verifies the chain at the config's
// Time, as crypto/tls does, so a valid certificate is refused at a time past its expiry.
func TestVerifyServerUsesConfigTime(t *testing.T) {
	t.Parallel()
	ca := newTestCA(t)
	leaf := ca.leaf(t, x509.ExtKeyUsageServerAuth)
	cert, err := x509.ParseCertificate(leaf.Certificate[0])
	require.NoError(t, err)
	id, err := peerIDFromCertificate(cert)
	require.NoError(t, err)
	cs := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}

	require.NoError(t, verifyServer(&tls.Config{RootCAs: ca.pool()}, id).VerifyConnection(cs))

	late := &tls.Config{RootCAs: ca.pool(), Time: func() time.Time { return cert.NotAfter.Add(time.Minute) }}
	var e x509.CertificateInvalidError
	require.ErrorAs(t, verifyServer(late, id).VerifyConnection(cs), &e)
	require.Equal(t, x509.Expired, e.Reason)
}

// TestClientSendsServerNameAsSNI checks the SNI name a server receives on every dial path: the
// config's ServerName when it is set, and the host of the dialed address otherwise.
func TestClientSendsServerNameAsSNI(t *testing.T) {
	t.Parallel()
	testSetup(t)

	ca := newTestCA(t)
	clientCert := ca.leaf(t, x509.ExtKeyUsageClientAuth)
	parsed, err := x509.ParseCertificate(clientCert.Certificate[0])
	require.NoError(t, err)
	clientID, err := peerIDFromCertificate(parsed)
	require.NoError(t, err)

	dials := map[string]func(t *testing.T, info host.StreamInfo, config *tls.Config) error{
		"multiplexed": func(t *testing.T, info host.StreamInfo, config *tls.Config) error {
			t.Helper()
			p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
			t.Cleanup(func() { _ = p.Close() })
			s, err := p.NewClientStream(info, t.Context(), clientID, config)
			if err == nil {
				_ = s.Close()
			}
			return err
		},
		"simple": func(t *testing.T, info host.StreamInfo, config *tls.Config) error {
			t.Helper()
			s, err := NewSimpleProvider().NewClientStream(info, t.Context(), clientID, config)
			if err == nil {
				_ = s.Close()
			}
			return err
		},
		"preamble": func(t *testing.T, info host.StreamInfo, config *tls.Config) error {
			t.Helper()
			_, err := ReadPreamble(t.Context(), info.RemotePeerID, info.RemotePeerAddress, config)
			return err
		},
	}
	for _, serverName := range []string{"", "peer.example.com"} {
		for path, dial := range dials {
			t.Run(path+"/ServerName="+serverName, func(t *testing.T) {
				t.Parallel()
				sni := make(chan string, 1)
				serverTLSConfig := &tls.Config{
					MinVersion:   tls.VersionTLS13,
					MaxVersion:   tls.VersionTLS13,
					Certificates: []tls.Certificate{ca.leaf(t, x509.ExtKeyUsageServerAuth)},
					ClientAuth:   tls.RequireAndVerifyClientCert,
					ClientCAs:    ca.pool(),
					GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
						select {
						case sni <- hello.ServerName:
						default: // a second handshake must not block the server
						}
						return nil, nil
					},
				}
				srv := startTestServer(t, NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0), serverTLSConfig, func(s host.P2PStream) { _ = s.Close() })
				t.Cleanup(srv.Close)
				// Dial by name: crypto/tls sends no SNI for an IP address.
				_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
				require.NoError(t, err)
				address := net.JoinHostPort("localhost", port)
				clientTLSConfig := &tls.Config{
					MinVersion:   tls.VersionTLS13,
					MaxVersion:   tls.VersionTLS13,
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      ca.pool(),
					ServerName:   serverName,
				}
				info := host.StreamInfo{
					RemotePeerID:      serverPeerID(t, serverTLSConfig),
					RemotePeerAddress: address,
					ContextID:         "ctx",
					SessionID:         "session",
				}

				require.NoError(t, dial(t, info, clientTLSConfig))
				want := cmp.Or(serverName, "localhost")
				require.Equal(t, want, <-sni)
			})
		}
	}
}

func (ca *testCA) pool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(ca.cert)
	return p
}

// testCA issues leaf certificates that carry no SANs, as identity certificates often do. An
// intermediate CA appends its own certificate to the chain of every leaf it issues.
type testCA struct {
	cert  *x509.Certificate
	key   *ecdsa.PrivateKey
	chain [][]byte
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)
	tpl := caTemplate("test-ca")
	der, err := x509.CreateCertificate(crand.Reader, tpl, tpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &testCA{cert: cert, key: key}
}

// intermediate returns a CA that ca signs. Only ca belongs in a trust pool.
func (ca *testCA) intermediate(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificate(crand.Reader, caTemplate("test-intermediate"), ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &testCA{cert: cert, key: key, chain: append([][]byte{der}, ca.chain...)}
}

func caTemplate(cn string) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
}

func (ca *testCA) leaf(t *testing.T, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	return ca.leafValid(t, usage, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
}

func (ca *testCA) leafValid(t *testing.T, usage x509.ExtKeyUsage, notBefore, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "node"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(crand.Reader, tpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: append([][]byte{der}, ca.chain...), PrivateKey: key}
}
