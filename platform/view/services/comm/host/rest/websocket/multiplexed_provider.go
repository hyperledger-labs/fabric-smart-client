/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/client/web"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

type SubConnId = string

func NewSubConnId() SubConnId {
	return strconv.FormatInt(rand.Int63(), 10) // TODO: AF
}

type MultiplexedMessage struct {
	ID  SubConnId `json:"id"`
	Msg []byte    `json:"msg"`
}

type MultiplexedProvider struct {
	clients map[string]*multiplexedClientConn
	mu      sync.RWMutex
}

func NewMultiplexedProvider() *MultiplexedProvider {
	return &MultiplexedProvider{clients: make(map[string]*multiplexedClientConn)}
}

func (c *MultiplexedProvider) NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error) {
	logger.Debugf("Creating new stream from [%s] to [%s@%s]...", src, info.RemotePeerID, info.RemotePeerAddress)
	tlsEnabled := config.InsecureSkipVerify || config.RootCAs != nil
	url := url.URL{Scheme: schemes[tlsEnabled], Host: info.RemotePeerAddress, Path: "/p2p"}
	// We use the background context instead of passing the existing context,
	// because the net/http server doesn't monitor connections upgraded to WebSocket.
	// Hence, when the connection is lost, the context will not be cancelled.
	c.mu.RLock()
	conn, ok := c.clients[url.String()]
	c.mu.RUnlock()
	if ok {
		return conn.newClientSubConn(ctx, src, info)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if conn, ok = c.clients[url.String()]; ok {
		return conn.newClientSubConn(ctx, src, info)
	}

	wsConn, err := web2.OpenWSClientConn(url.String(), config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open websocket")
	}
	conn = newClientConn(wsConn, func() {
		logger.Infof("Closing websocket client for [%s@%s]...", src, info.RemotePeerAddress)
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.clients, url.String())
	})
	c.clients[url.String()] = conn

	return conn.newClientSubConn(ctx, src, info)
}

func (c *MultiplexedProvider) NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error {
	conn, err := web.OpenWSServerConn(writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to open websocket")
	}
	newServerConn(conn, newStreamCallback)
	return nil
}

// Client

type multiplexedClientConn struct {
	*multiplexedBaseConn
	mu sync.RWMutex
}

func newClientConn(conn *websocket.Conn, onClose func()) *multiplexedClientConn {
	c := &multiplexedClientConn{multiplexedBaseConn: newBaseConn(conn)}
	go c.readIncoming(onClose)
	return c
}

func (c *multiplexedClientConn) newClientSubConn(ctx context.Context, src host2.PeerID, info host2.StreamInfo) (host2.P2PStream, error) {
	sc := c.newSubConn(NewSubConnId())
	logger.Infof("Created client subconn with id [%s]", sc.id)
	spanContext := trace.SpanContextFromContext(ctx)
	marshalledSpanContext, err := tracing.MarshalContext(spanContext)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(StreamMeta{
		ContextID:   info.ContextID,
		SessionID:   info.SessionID,
		PeerID:      src,
		SpanContext: marshalledSpanContext,
	})
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = c.WriteJSON(MultiplexedMessage{ID: sc.id, Msg: payload})
	c.writeMu.Unlock()

	if err != nil {
		return nil, errors.Wrapf(err, "failed to send meta message")
	}
	logger.Debugf("Stream opened to [%s@%s]", info.RemotePeerID, info.RemotePeerAddress)
	return NewWSStream(sc, ctx, info), nil
}

func (c *multiplexedClientConn) readIncoming(closeCallback func()) {
	var mm MultiplexedMessage
	var err error
	for err = c.ReadJSON(&mm); err == nil; err = c.ReadJSON(&mm) {
		c.mu.RLock()
		sc := c.subConns[mm.ID]
		c.mu.RUnlock()
		sc.reads <- result{value: mm.Msg, err: err}
	}
	for _, sc := range c.subConns {
		sc.reads <- streamEOF
	}
	logger.Warnf("Client connection errored: %v", err)
	err = c.Conn.Close()
	logger.Infof("Client connection closed: %v", err)
	closeCallback()
}

type multiplexedServerConn struct {
	*multiplexedBaseConn
}

// Server

func newServerConn(conn *websocket.Conn, newStreamCallback func(pStream host2.P2PStream)) *multiplexedServerConn {
	c := &multiplexedServerConn{newBaseConn(conn)}
	go c.readIncoming(newStreamCallback)
	return c
}

func (c *multiplexedServerConn) readIncoming(newStreamCallback func(pStream host2.P2PStream)) {
	var mm MultiplexedMessage
	var err error
	for err = c.ReadJSON(&mm); err == nil; err = c.ReadJSON(&mm) {
		c.mu.RLock()
		sc, ok := c.subConns[mm.ID]
		c.mu.RUnlock()
		if ok {
			sc.reads <- result{value: mm.Msg, err: err}
		} else {
			c.newServerSubConn(newStreamCallback, mm)
		}
	}
	c.Conn.Close()
	logger.Infof("Server connection closed: %v", err)
}

func (c *multiplexedServerConn) newServerSubConn(newStreamCallback func(pStream host2.P2PStream), mm MultiplexedMessage) {
	var meta StreamMeta
	if err := json.Unmarshal(mm.Msg, &meta); err != nil {
		logger.Errorf("failed to read meta info from [%s]: %v", string(mm.Msg), err)
	}
	logger.Debugf("Read meta info: [%s,%s]: %s", meta.ContextID, meta.SessionID, meta.SpanContext)
	// Propagating the request context will not make a difference (see comment in newClientStream)
	spanContext, err := tracing.UnmarshalContext(meta.SpanContext)
	if err != nil {
		logger.Errorf("failed to unmarshal span context: %v", err)
	}
	sc := c.newSubConn(mm.ID)
	logger.Debugf("Created server subconn with id [%s]", sc.id)

	logger.Debugf("Received response with context: %v", spanContext)
	newStreamCallback(NewWSStream(sc, trace.ContextWithRemoteSpanContext(context.Background(), spanContext), host2.StreamInfo{
		RemotePeerID:      meta.PeerID,
		RemotePeerAddress: c.Conn.RemoteAddr().String(),
		ContextID:         meta.ContextID,
		SessionID:         meta.SessionID,
	}))
}

type multiplexedBaseConn struct {
	*websocket.Conn

	subConns map[SubConnId]*subConn
	writes   chan MultiplexedMessage
	closes   chan SubConnId
	mu       sync.RWMutex
	writeMu  sync.Mutex
	cancel   context.CancelFunc
}

func newBaseConn(conn *websocket.Conn) *multiplexedBaseConn {
	ctx, cancel := context.WithCancel(context.Background())
	c := &multiplexedBaseConn{
		Conn:     conn,
		subConns: make(map[SubConnId]*subConn),
		closes:   make(chan SubConnId, 1000),
		writes:   make(chan MultiplexedMessage, 1000),
		cancel:   cancel,
	}
	go c.readOutgoing(ctx)
	go c.readCloses(ctx)
	return c
}

func (c *multiplexedBaseConn) newSubConn(id SubConnId) *subConn {
	c.mu.Lock()
	defer c.mu.Unlock()
	sc := &subConn{
		id:        id,
		reads:     make(chan result),
		writes:    c.writes,
		closes:    c.closes,
		writeErrs: make(chan error),
	}
	c.subConns[sc.id] = sc
	return sc
}

func (c *multiplexedBaseConn) readCloses(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Stop waiting for closes")
			c.subConns = make(map[SubConnId]*subConn)
			return
		case id := <-c.closes:
			logger.Infof("Closing sub conn [%v]", id)
			c.mu.Lock()
			delete(c.subConns, id)
			if len(c.subConns) == 0 {
				logger.Infof("No sub conns left")
			}
			c.mu.Unlock()
		}
	}
}

func (c *multiplexedBaseConn) readOutgoing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Closing outgoing connections: [%v]", collections.Keys(c.subConns))
			return
		case msg := <-c.writes:
			c.writeMu.Lock()
			err := c.WriteJSON(msg)
			c.writeMu.Unlock()
			c.subConns[msg.ID].writeErrs <- err
		}
	}
}

// Sub connection

type subConn struct {
	id        SubConnId
	reads     chan result
	writes    chan<- MultiplexedMessage
	closes    chan<- SubConnId
	writeErrs chan error
}

func (c *subConn) ReadMessage() (messageType int, p []byte, err error) {
	r := <-c.reads
	return websocket.TextMessage, r.value, r.err
}
func (c *subConn) WriteMessage(_ int, data []byte) error {
	c.writes <- MultiplexedMessage{c.id, data}
	return <-c.writeErrs
}

func (c *subConn) Close() error {
	c.closes <- c.id
	return nil
}
