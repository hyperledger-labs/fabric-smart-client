/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"cmp"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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

	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{})

	var wg sync.WaitGroup

	// server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
						if errors.Is(err, io.EOF) {
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

	srvEndpoint := strings.TrimPrefix(srv.URL, "http://")

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

		client, err := p.NewClientStream(infoWithInvalidHostAddress, context.Background(), "somePeerID", &tls.Config{})
		assert.Error(t, err)
		assert.Nil(t, client)

		// we should have no clients
		p.mu.RLock()
		assert.Equal(t, 0, len(p.clients))
		p.mu.RUnlock()
	})

	t.Run("many clients connect sequentially", func(t *testing.T) {
		for i := range numStreams {
			testClientRun(t, p, srvEndpoint, fmt.Sprintf("session-%d", i))
		}
	})

	t.Run("many clients connect concurrently", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := range numStreams {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				testClientRun(t, p, srvEndpoint, fmt.Sprintf("session-%d", i))
			}(i)
		}
		wg.Wait()
	})

	// we expect our client connection to still be open
	p.mu.RLock()
	assert.Equal(t, 1, len(p.clients))
	p.mu.RUnlock()

	wg.Wait()

	// close server
	srv.Close()

	err := p.KillAll()
	assert.NoError(t, err)

	p.mu.RLock()
	assert.Equal(t, 0, len(p.clients))
	p.mu.RUnlock()
}

func TestSendingOnClosedSubConnections(t *testing.T) {
	testSetup(t)

	// let check that at the end of this test all our go routines are stopped
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	p := NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{})
	var wg sync.WaitGroup

	wait := make(chan struct{})

	// server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	srvEndpoint := strings.TrimPrefix(srv.URL, "http://")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "someContextID",
		SessionID:         "someSessionID",
	}
	src := host.PeerID("somePeerID")
	config := &tls.Config{}

	client, err := p.NewClientStream(info, ctx, src, config)
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

	//time.Sleep(snoozeTime)
	// now we use our superpowers to let the server send another message
	wait <- struct{}{}

	wg.Wait()

	// close server
	srv.Close()

	err = p.KillAll()
	require.NoError(t, err)

	p.mu.RLock()
	require.Equal(t, 0, len(p.clients))
	p.mu.RUnlock()
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

func testClientRun(t *testing.T, p *MultiplexedProvider, srvEndpoint, sessionID string) {
	ctx, cancel := context.WithCancel(context.Background())
	info := host.StreamInfo{
		RemotePeerID:      "serverID",
		RemotePeerAddress: srvEndpoint,
		ContextID:         "someContextID",
		SessionID:         sessionID,
	}
	src := host.PeerID("somePeerID")
	config := &tls.Config{}

	client, err := p.NewClientStream(info, ctx, src, config)
	assert.NoError(t, err)

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

func sendMsg(stream host.P2PStream, msg []byte) error {
	// write length frame
	lenBuf := make([]byte, binary.MaxVarintLen64)
	_ = binary.PutUvarint(lenBuf, uint64(len(msg)))
	_, err := stream.Write(lenBuf)
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
	lenBuf := make([]byte, binary.MaxVarintLen64)
	_, err := stream.Read(lenBuf)
	if err != nil {
		return nil, err
	}

	// read message
	n, _ := binary.Uvarint(lenBuf)
	msgBuf := make([]byte, n)
	_, err = stream.Read(msgBuf)
	if err != nil {
		return nil, err
	}

	return msgBuf, nil
}
