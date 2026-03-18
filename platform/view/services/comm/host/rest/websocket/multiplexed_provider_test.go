/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"cmp"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
)

const (
	totalMsg   = 1000
	numStreams = 25
	snoozeTime = time.Second / 200
)

var (
	clientLogger = logging.MustGetLogger("client")
	serverLogger = logging.MustGetLogger("server")
)

func TestConnections(t *testing.T) {
	testSetup(t)

	// let check that at the end of this test all our go routines are stopped
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	for _, insecureSkipVerify := range []bool{true, false} {
		mode := fmt.Sprintf("InsecureSkipVerify=%v", insecureSkipVerify)
		t.Run(mode, func(t *testing.T) {
			p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
			serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, insecureSkipVerify)

			var wg sync.WaitGroup

			// server
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err := p.NewServerStream(w, r, func(s host.P2PStream) {
					wg.Add(1)
					go func(srv host.P2PStream) {
						defer wg.Done()
						serverLogger.Debugf("[server] new stream established with %v (ID=%v) sessionID=%v", srv.RemotePeerID(), srv.RemotePeerID(), srv.Hash())
						for {
							serverLogger.Debugf("[server] reading ...")
							answer, err := readMsg(srv)

							// deal with EOF
							if err != nil {
								if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
									return
								}
							}

							// ping
							assert.NoError(t, err)
							assert.EqualValues(t, []byte("ping"), answer)

							// pong
							serverLogger.Info("[server] sending pong ...")
							err = sendMsg(srv, []byte("pong"))
							assert.NoError(t, err)
						}
					}(s)
				})
				assert.NoError(t, err)
			}))
			srv.TLS = serverTLSConfig
			srv.StartTLS()
			defer srv.Close()

			srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

			time.Sleep(snoozeTime)

			t.Run("client cannot connect", func(t *testing.T) {
				// we should have no clients at this point
				p.mu.RLock()
				assert.Equal(t, 0, len(p.clients))
				p.mu.RUnlock()

				// creating this client should fail
				infoWithInvalidHostAddress := host.StreamInfo{
					RemotePeerID:      "serverID",
					RemotePeerAddress: "localhost:1234", // some wrong server address
					ContextID:         "someContextID",
					SessionID:         "testSessionID",
				}

				client, err := p.NewClientStream(infoWithInvalidHostAddress, context.Background(), srcID, clientTLSConfig)
				assert.Error(t, err)
				assert.Nil(t, client)

				// we should have no clients
				p.mu.RLock()
				assert.Equal(t, 0, len(p.clients))
				p.mu.RUnlock()
			})

			t.Run("many clients connect sequentially", func(t *testing.T) {
				for i := range numStreams {
					testClientRun(t, p, srvEndpoint, fmt.Sprintf("session-%d", i), srcID, clientTLSConfig)
				}
			})

			t.Run("many clients connect concurrently", func(t *testing.T) {
				var wg sync.WaitGroup
				for i := range numStreams {
					wg.Add(1)
					go func(i int) {
						defer wg.Done()
						testClientRun(t, p, srvEndpoint, fmt.Sprintf("session-%d", i), srcID, clientTLSConfig)
					}(i)
				}
				wg.Wait()
			})

			// we expect our client connection to still be open
			p.mu.RLock()
			assert.Equal(t, 1, len(p.clients))
			p.mu.RUnlock()

			wg.Wait()

			err := p.KillAll()
			assert.NoError(t, err)

			p.mu.RLock()
			assert.Equal(t, 0, len(p.clients))
			p.mu.RUnlock()
		})
	}
}

func TestSendingOnClosedSubConnections(t *testing.T) {
	testSetup(t)

	// let check that at the end of this test all our go routines are stopped
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	for _, insecureSkipVerify := range []bool{true, false} {
		mode := fmt.Sprintf("InsecureSkipVerify=%v", insecureSkipVerify)
		t.Run(mode, func(t *testing.T) {
			p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
			serverTLSConfig, clientTLSConfig, srcID := testMutualTLSConfigs(t, insecureSkipVerify)
			var wg sync.WaitGroup

			wait := make(chan struct{})

			// server
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err := p.NewServerStream(w, r, func(s host.P2PStream) {
					wg.Add(1)
					go func(srv host.P2PStream) {
						defer wg.Done()
						serverLogger.Debugf("[server] new stream established with %v (ID=%v) sessionID=%v", srv.RemotePeerID(), srv.RemotePeerID(), srv.Hash())
						serverLogger.Debugf("[server] reading ...")

						// server receives first message
						answer, err := readMsg(srv)
						require.NoError(t, err)
						require.EqualValues(t, []byte("ping"), answer)

						// sends back a message
						err = sendMsg(srv, []byte("pong"))
						require.NoError(t, err)

						// we wait for next orders
						<-wait

						// we send a few messages at a high rate; eventually we should receive a channel closed message
						require.EventuallyWithT(t, func(c *assert.CollectT) {
							err = sendMsg(srv, []byte("pong again"))
							require.ErrorIs(c, err, websocket.ErrCloseSent)
						}, 100*time.Millisecond, time.Nanosecond)
					}(s)
				})
				require.NoError(t, err)
			}))
			srv.TLS = serverTLSConfig
			srv.StartTLS()
			defer srv.Close()

			srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			info := host.StreamInfo{
				RemotePeerID:      "serverID",
				RemotePeerAddress: srvEndpoint,
				ContextID:         "someContextID",
				SessionID:         "someSessionID",
			}
			client, err := p.NewClientStream(info, ctx, srcID, clientTLSConfig)
			require.NoError(t, err)

			// send ping
			err = sendMsg(client, []byte("ping"))
			require.NoError(t, err)

			// expects pong
			answer, err := readMsg(client)
			require.NoError(t, err)
			require.EqualValues(t, []byte("pong"), answer)

			// now the client is actually done and closes the subconn
			err = client.Close()
			require.NoError(t, err)

			// now we use our superpowers to let the server send another message
			wait <- struct{}{}

			wg.Wait()

			err = p.KillAll()
			require.NoError(t, err)

			p.mu.RLock()
			require.Equal(t, 0, len(p.clients))
			p.mu.RUnlock()
		})
	}
}

func TestRejectsPeerIDMismatch(t *testing.T) {
	testSetup(t)
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	for _, insecureSkipVerify := range []bool{true, false} {
		mode := fmt.Sprintf("InsecureSkipVerify=%v", insecureSkipVerify)
		t.Run(mode, func(t *testing.T) {
			p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}, 0)
			serverTLSConfig, clientTLSConfig, _ := testMutualTLSConfigs(t, insecureSkipVerify)

			received := make(chan struct{}, 1)
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err := p.NewServerStream(w, r, func(_ host.P2PStream) {
					received <- struct{}{}
				})
				assert.NoError(t, err)
			}))
			srv.TLS = serverTLSConfig
			srv.StartTLS()
			defer srv.Close()

			srvEndpoint := strings.TrimPrefix(strings.TrimPrefix(srv.URL, "http://"), "https://")
			info := host.StreamInfo{
				RemotePeerID:      "serverID",
				RemotePeerAddress: srvEndpoint,
				ContextID:         "ctx",
				SessionID:         "sess",
			}

			client, err := p.NewClientStream(info, context.Background(), host.PeerID("invalid-peer-id"), clientTLSConfig)
			require.NoError(t, err)

			require.Eventually(t, func() bool {
				err := sendMsg(client, []byte("ping"))
				return err != nil
			}, 3*time.Second, 50*time.Millisecond)

			select {
			case <-received:
				t.Fatal("server accepted a stream with mismatched peer ID")
			default:
			}

			require.NoError(t, p.KillAll())
		})
	}
}

func testSetup(_ *testing.T) {
	logSpec := cmp.Or(
		os.Getenv("FABRIC_LOGGING_SPEC"),
		"error",
	)

	logging.Init(logging.Config{
		LogSpec: logSpec,
		Writer:  os.Stdout,
	})
}

func testClientRun(t *testing.T, p *MultiplexedProvider, srvEndpoint, sessionID string, src host.PeerID, config *tls.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "someContextID",
		SessionID:         sessionID,
	}
	client, err := p.NewClientStream(info, ctx, src, config)
	require.NoError(t, err)
	require.NotNil(t, client)

	// we should
	p.mu.RLock()
	assert.Equal(t, 1, len(p.clients))
	p.mu.RUnlock()

	for range totalMsg {
		// send ping
		clientLogger.Info("[client] sending ping ...")
		err = sendMsg(client, []byte("ping"))
		assert.NoError(t, err)

		// expect pong
		clientLogger.Info("[client] reading ...")
		answer, err := readMsg(client)
		assert.NoError(t, err)
		assert.EqualValues(t, []byte("pong"), answer)
	}

	// gracefully shutdown our client
	err = client.Close()
	assert.NoError(t, err)

	cancel()
}

func testMutualTLSConfigs(t *testing.T, insecureSkipVerify bool) (*tls.Config, *tls.Config, host.PeerID) {
	t.Helper()

	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	require.NoError(t, err)
	serial := big.NewInt(1)
	caTemplate := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(crand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	newLeaf := func(sn int64, cn string, dnsNames []string, ext []x509.ExtKeyUsage) tls.Certificate {
		priv, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
		require.NoError(t, err)
		leafTpl := &x509.Certificate{
			SerialNumber: big.NewInt(sn),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Minute),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  ext,
			DNSNames:     dnsNames,
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		leafDER, err := x509.CreateCertificate(crand.Reader, leafTpl, caCert, &priv.PublicKey, caPriv)
		require.NoError(t, err)
		return tls.Certificate{
			Certificate: [][]byte{leafDER, caDER},
			PrivateKey:  priv,
		}
	}

	serverCert := newLeaf(2, "server", []string{"localhost"}, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
	clientCert := newLeaf(3, "client", nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})

	clientX509Cert, err := x509.ParseCertificate(clientCert.Certificate[0])
	require.NoError(t, err)
	srcID, err := peerIDFromCertificate(clientX509Cert)
	require.NoError(t, err)

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert)

	serverTLSConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
	}
	clientTLSConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caCertPool,
	}
	if insecureSkipVerify {
		clientTLSConfig.InsecureSkipVerify = true
		clientTLSConfig.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			certs := make([]*x509.Certificate, len(rawCerts))
			for i, raw := range rawCerts {
				cert, err := x509.ParseCertificate(raw)
				if err != nil {
					return err
				}
				certs[i] = cert
			}
			intermediates := x509.NewCertPool()
			for _, cert := range certs[1:] {
				intermediates.AddCert(cert)
			}
			_, err := certs[0].Verify(x509.VerifyOptions{
				Roots:         caCertPool,
				Intermediates: intermediates,
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			})
			return err
		}
	} else {
		clientTLSConfig.ServerName = "localhost"
	}

	return serverTLSConfig, clientTLSConfig, srcID
}

func sendMsg(stream host.P2PStream, msg []byte) error {
	// write length frame
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, uint64(len(msg)))
	_, err := stream.Write(lenBuf[:n])
	if err != nil {
		return err
	}

	// write message
	_, err = stream.Write(msg)
	if err != nil {
		return err
	}

	return nil
}

func readMsg(stream host.P2PStream) ([]byte, error) {
	// read length frame
	lenBuf := make([]byte, 0, binary.MaxVarintLen64)
	for i := 0; i < binary.MaxVarintLen64; i++ {
		b := make([]byte, 1)
		if _, err := stream.Read(b); err != nil {
			return nil, err
		}
		lenBuf = append(lenBuf, b[0])
		if _, consumed := binary.Uvarint(lenBuf); consumed > 0 {
			break
		}
	}

	// read message
	n, _ := binary.Uvarint(lenBuf)
	msgBuf := make([]byte, n)
	_, err := stream.Read(msgBuf)
	if err != nil {
		return nil, err
	}

	return msgBuf, nil
}
