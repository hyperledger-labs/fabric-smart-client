/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
	"encoding/binary"
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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	contextIDLabel tracing.LabelName = "context_id"
	subconnIDLabel tracing.LabelName = "subconn_id"
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
	tracer  trace.Tracer
	m       *Metrics
	mu      sync.RWMutex
}

func NewMultiplexedProvider(tracerProvider trace.TracerProvider, metricsProvider metrics.Provider) *MultiplexedProvider {
	return &MultiplexedProvider{
		clients: make(map[string]*multiplexedClientConn),
		tracer: tracerProvider.Tracer("multiplexed-ws", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "core",
			LabelNames: []tracing.LabelName{contextIDLabel, subconnIDLabel},
		})),
		m: newMetrics(metricsProvider),
	}
}

func (c *MultiplexedProvider) NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (s host2.P2PStream, err error) {
	newCtx, span := c.tracer.Start(ctx, "client_stream",
		tracing.WithAttributes(tracing.String(contextIDLabel, info.ContextID)),
		tracing.WithAttributes(tracing.String(subconnIDLabel, "")))
	defer func() {
		if err != nil {
			span.RecordError(err)
		} else {
			span.SetAttributes(tracing.String(subconnIDLabel, s.(*stream).conn.ID()))
		}
		span.End()

		c.mu.RLock()
		totalSubConns := 0
		for _, clientConn := range c.clients {
			clientConn.mu.RLock()
			totalSubConns += len(clientConn.subConns)
			clientConn.mu.RUnlock()
		}
		c.mu.RUnlock()
		c.m.TotalSubConns.Set(float64(totalSubConns))
		c.m.TotalSize.Set(float64(binary.Size(c)))
	}()
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
		return conn.newClientSubConn(newCtx, src, info)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if conn, ok = c.clients[url.String()]; ok {
		return conn.newClientSubConn(newCtx, src, info)
	}

	wsConn, err := web2.OpenWSClientConn(url.String(), config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open websocket")
	}
	conn = newClientConn(wsConn, c.tracer, c.m, func() {
		logger.Infof("Closing websocket client for [%s@%s]...", src, info.RemotePeerAddress)
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.clients, url.String())
	})
	c.clients[url.String()] = conn
	c.m.OpenedWebsockets.With(sideLabel, clientSide).Add(1)

	return conn.newClientSubConn(newCtx, src, info)
}

func (c *MultiplexedProvider) NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error {
	conn, err := web.OpenWSServerConn(writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to open websocket")
	}
	c.m.OpenedWebsockets.With(sideLabel, serverSide).Add(1)
	newServerConn(conn, c.tracer, c.m, newStreamCallback)
	return nil
}

// Client

type multiplexedClientConn struct {
	*multiplexedBaseConn
}

func newClientConn(conn *websocket.Conn, tracer trace.Tracer, m *Metrics, onClose func()) *multiplexedClientConn {
	c := &multiplexedClientConn{multiplexedBaseConn: newBaseConn(conn, tracer, m, clientSide)}
	go func() {
		c.readIncoming()
		onClose()
	}()
	return c
}

func (c *multiplexedClientConn) newClientSubConn(ctx context.Context, src host2.PeerID, info host2.StreamInfo) (*stream, error) {
	c.mu.Lock()
	sc := c.newSubConn(NewSubConnId())
	c.subConns[sc.id] = sc
	c.mu.Unlock()
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

func (c *multiplexedClientConn) readIncoming() {
	defer func() {
		c.mu.RLock()
		subConns := collections.Values(c.subConns)
		c.mu.RUnlock()
		for _, sc := range subConns {
			sc.reads <- streamEOF
		}
		err := c.Conn.Close()
		logger.Infof("Client connection closed: %v", err)
	}()
	var mm MultiplexedMessage
	for {
		//c.writeMu.Lock()
		err := c.ReadJSON(&mm)
		//c.writeMu.Unlock()
		if err != nil {
			logger.Warnf("Client connection errored: %v", err)
			return
		}

		c.mu.RLock()
		sc, ok := c.subConns[mm.ID]
		c.mu.RUnlock()
		if ok {
			sc.reads <- result{value: mm.Msg}
		} else {
			panic("subconn not found")
		}
	}
}

type multiplexedServerConn struct {
	*multiplexedBaseConn
}

// Server

func newServerConn(conn *websocket.Conn, tracer trace.Tracer, m *Metrics, newStreamCallback func(pStream host2.P2PStream)) *multiplexedServerConn {
	c := &multiplexedServerConn{newBaseConn(conn, tracer, m, serverSide)}
	go c.readIncoming(newStreamCallback)
	return c
}

func (c *multiplexedServerConn) readIncoming(newStreamCallback func(pStream host2.P2PStream)) {
	defer func() {
		c.mu.RLock()
		subConns := collections.Values(c.subConns)
		c.mu.RUnlock()
		for _, sc := range subConns {
			sc.reads <- streamEOF
		}
		err := c.Conn.Close()
		logger.Infof("Connection closed: %v", err)
	}()
	var mm MultiplexedMessage
	for {
		//c.writeMu.Lock()
		err := c.ReadJSON(&mm)
		//c.writeMu.Unlock()
		if err != nil {
			logger.Warnf("Connection errored: %v", err)
			return
		}

		c.mu.RLock()
		sc, ok := c.subConns[mm.ID]
		c.mu.RUnlock()
		logger.Debugf("subconn for [%s] exists [%v]", mm.ID, ok)
		if ok {
			sc.reads <- result{value: mm.Msg}
		} else {
			c.newServerSubConn(newStreamCallback, mm)
		}
	}
}

func (c *multiplexedServerConn) newServerSubConn(newStreamCallback func(pStream host2.P2PStream), mm MultiplexedMessage) {
	c.mu.Lock()
	if _, ok := c.subConns[mm.ID]; ok {
		c.mu.Unlock()
		return
	}
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
	ctx, span := c.tracer.Start(trace.ContextWithRemoteSpanContext(context.Background(), spanContext), "server_stream", tracing.WithAttributes(
		tracing.String(contextIDLabel, meta.ContextID),
		tracing.String(subconnIDLabel, mm.ID)))
	defer span.End()
	sc := c.newSubConn(mm.ID)
	c.subConns[mm.ID] = sc
	c.mu.Unlock()
	logger.Debugf("Created server subconn with id [%s]", sc.id)

	logger.Debugf("Received response with context: %v", spanContext)
	newStreamCallback(NewWSStream(sc, ctx, host2.StreamInfo{
		RemotePeerID:      meta.PeerID,
		RemotePeerAddress: c.Conn.RemoteAddr().String(),
		ContextID:         meta.ContextID,
		SessionID:         meta.SessionID,
	}))
}

type multiplexedBaseConn struct {
	*websocket.Conn
	tracer trace.Tracer

	subConns map[SubConnId]*subConn
	writes   chan MultiplexedMessage
	closes   chan SubConnId
	mu       sync.RWMutex
	writeMu  sync.Mutex
	cancel   context.CancelFunc

	m    *Metrics
	side string
}

func newBaseConn(conn *websocket.Conn, tracer trace.Tracer, metrics *Metrics, side string) *multiplexedBaseConn {
	ctx, cancel := context.WithCancel(context.Background())
	c := &multiplexedBaseConn{
		Conn:     conn,
		tracer:   tracer,
		subConns: make(map[SubConnId]*subConn),
		closes:   make(chan SubConnId, 1000),
		writes:   make(chan MultiplexedMessage, 1000),
		cancel:   cancel,
		m:        metrics,
		side:     side,
	}
	go c.readOutgoing(ctx)
	go c.readCloses(ctx)
	return c
}

func (c *multiplexedBaseConn) newSubConn(id SubConnId) *subConn {
	c.m.OpenedSubConns.With(sideLabel, c.side).Add(1)
	return &subConn{
		id:        id,
		reads:     make(chan result),
		writes:    c.writes,
		closes:    c.closes,
		writeErrs: make(chan error),
	}
}

func (c *multiplexedBaseConn) readCloses(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Stop waiting for closes")
			c.mu.Lock()
			c.m.ClosedSubConns.With(sideLabel, c.side).Add(float64(len(c.subConns)))
			c.subConns = make(map[SubConnId]*subConn)
			c.mu.Unlock()
			return
		case id := <-c.closes:
			logger.Infof("Closing sub conn [%v]", id)
			c.mu.Lock()
			delete(c.subConns, id)
			c.mu.Unlock()
			c.m.ClosedSubConns.With(sideLabel, c.side).Add(1)
			// TODO: Clean the connection if none left
		}
	}
}

func (c *multiplexedBaseConn) readOutgoing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			logger.Infof("Closing all outgoing connections")
			return
		case msg := <-c.writes:
			c.writeMu.Lock()
			err := c.WriteJSON(msg)
			c.writeMu.Unlock()
			c.mu.RLock()
			sc, ok := c.subConns[msg.ID]
			c.mu.RUnlock()
			if ok {
				sc.writeErrs <- err
			} else {
				panic("could not find sc with id " + msg.ID)
			}
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
	once      sync.Once
}

func (c *subConn) ID() SubConnId {
	return c.id
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
	c.once.Do(func() {
		c.closes <- c.id
	})
	return nil
}
