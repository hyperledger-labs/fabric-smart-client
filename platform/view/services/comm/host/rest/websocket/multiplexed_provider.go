/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
	"encoding/json"
	errors2 "errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"go.opentelemetry.io/otel/trace"
)

const (
	contextIDLabel        tracing.LabelName = "context_id"
	defaultContextIDLabel string            = "context"
)

type SubConnId = string

type MultiplexedMessage struct {
	ID  SubConnId `json:"id"`
	Msg []byte    `json:"msg"`
	Err string    `json:"err"`
}

type MultiplexedProvider struct {
	// mu protects the clients map
	mu      sync.RWMutex
	clients map[string]*multiplexedClientConn

	tracer      trace.Tracer
	m           *Metrics
	trackerDone chan struct{}
}

func (c *MultiplexedProvider) KillAll() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	close(c.trackerDone)

	var err error
	for _, cl := range c.clients {
		err = errors2.Join(err, cl.Kill())
	}

	// cleanup clients
	clear(c.clients)

	return err
}

func NewMultiplexedProvider(tracerProvider tracing.Provider, metricsProvider metrics.Provider) *MultiplexedProvider {
	p := &MultiplexedProvider{
		clients: make(map[string]*multiplexedClientConn),
		tracer: tracerProvider.Tracer("multiplexed_ws", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []tracing.LabelName{contextIDLabel},
		})),
		m: newMetrics(metricsProvider),
	}

	// spawn ActiveSubConn tracker
	p.trackerDone = StartTracker(func() {
		sum := 0
		p.mu.RLock()
		for _, c := range p.clients {
			c.mu.RLock()
			sum += len(c.subConns)
			c.mu.RUnlock()
		}
		p.mu.RUnlock()
		p.m.ActiveSubConns.Set(float64(sum))
	})

	return p
}

func StartTracker(f func()) chan struct{} {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				f()
			case <-done:
				return
			}
		}
	}()
	return done
}

func (c *MultiplexedProvider) NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (s host2.P2PStream, err error) {
	span := trace.SpanFromContext(ctx)
	defer func() {
		if err != nil {
			span.RecordError(err)
		}
	}()
	logger.Debugf("Creating new stream from [%s] to [%s@%s]...", src, info.RemotePeerID, info.RemotePeerAddress)
	tlsEnabled := config != nil && (config.InsecureSkipVerify || config.RootCAs != nil)
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

	conn = newClientConn(wsConn, c.tracer, c.m, func() {
		logger.Debugf("Closing websocket client for [%s@%s]...", src, info.RemotePeerAddress)
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.clients, url.String())
	})
	c.clients[url.String()] = conn
	c.m.OpenedWebsockets.With(sideLabel, clientSide).Add(1)

	return conn.newClientSubConn(ctx, src, info)
}

func (c *MultiplexedProvider) NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error {
	conn, err := server.OpenWSServerConn(writer, request)
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
	sc := c.newSubConn(strconv.FormatUint(c.subConnId.Add(1), 10))
	c.subConns[sc.id] = sc
	c.mu.Unlock()
	logger.Debugf("Created client subconn with id [%s]", sc.id)
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

	err = c.write(MultiplexedMessage{ID: sc.id, Msg: payload})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to send meta message")
	}
	logger.Debugf("Stream opened to [%s@%s]", info.RemotePeerID, info.RemotePeerAddress)
	return NewWSStream(sc, ctx, info), nil
}

func (c *multiplexedClientConn) readIncoming() {
	defer func() {
		// close everything
		err := c.Kill()
		logger.Debugf("Client connection closed: %v", err)
	}()
	var mm MultiplexedMessage
	for {
		err := c.conn.ReadJSON(&mm)
		if err != nil {
			logger.Debugf("Client connection errored: %v", err)
			return
		}

		c.mu.RLock()
		sc, ok := c.subConns[mm.ID]
		c.mu.RUnlock()

		if !ok && mm.Err == "" {
			// it might happen that we receive a message from the server after we have already closed the sub-connection
			// in this case we just ignore the message and drop it
			logger.Warnf("client sub-connection does not exist mmId=%v, dropping message", mm.ID)
			logger.Debugf("dropping message: `%s`", string(mm.Msg))
		} else if !ok && mm.Err != "" {
			logger.Debugf("client sub-connection does not exist mmId=%v, errored: %v", mm.ID, mm.Err)
		} else if mm.Err != "" {
			logger.Debugf("client sub-connection mmId=%v errored: %v", mm.ID, mm.Err)
		} else {
			sc.receiverChan <- result{value: mm.Msg}
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
		// close everything
		err := c.Kill()
		logger.Debugf("Server connection closed: %v", err)
	}()
	var mm MultiplexedMessage
	for {
		err := c.conn.ReadJSON(&mm)
		if err != nil {
			logger.Debugf("Connection errored: %v", err)

			return
		}

		c.mu.RLock()
		sc, ok := c.subConns[mm.ID]
		c.mu.RUnlock()
		logger.Debugf("subconn for [%s] exists [%v]", mm.ID, ok)
		if !ok && mm.Err == "" {
			c.newServerSubConn(newStreamCallback, mm)
		} else if !ok && mm.Err != "" {
			logger.Debugf("server subconn errored: %v", mm.Err)
		} else if mm.Err != "" {
			logger.Debugf("Server subconn [%s] errored: %v", mm.ID, mm.Err)
			_ = sc.Close()
		} else {
			sc.receiverChan <- result{value: mm.Msg}
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
		logger.Debugf("failed to unmarshal span context: %v", err)
	}
	ctx, span := c.tracer.Start(trace.ContextWithRemoteSpanContext(context.Background(), spanContext), "IncomingViewInvocation", tracing.WithAttributes(
		tracing.String(contextIDLabel, defaultContextIDLabel)))
	defer span.End()
	sc := c.newSubConn(mm.ID)
	c.subConns[mm.ID] = sc
	c.mu.Unlock()
	logger.Debugf("Created server subconn with id [%s]", sc.id)

	logger.Debugf("Received response with context: %v", spanContext)
	newStreamCallback(NewWSStream(sc, ctx, host2.StreamInfo{
		RemotePeerID:      meta.PeerID,
		RemotePeerAddress: c.conn.RemoteAddr().String(),
		ContextID:         meta.ContextID,
		SessionID:         meta.SessionID,
	}))
}

type multiplexedBaseConn struct {
	writeMu sync.Mutex
	conn    *websocket.Conn

	// mu protects concurrent use of our subConns
	mu        sync.RWMutex
	subConns  map[SubConnId]*subConn
	subConnId atomic.Uint64

	tracer trace.Tracer
	m      *Metrics
	side   string
}

func newBaseConn(conn *websocket.Conn, tracer trace.Tracer, metrics *Metrics, side string) *multiplexedBaseConn {
	c := &multiplexedBaseConn{
		conn:     conn,
		subConns: make(map[SubConnId]*subConn),
		tracer:   tracer,
		m:        metrics,
		side:     side,
	}
	return c
}

func (c *multiplexedBaseConn) Kill() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// close sub conns
	var err error
	for _, sc := range c.subConns {
		err = errors2.Join(err, sc.Close())
	}

	// close websocket
	err = errors2.Join(err, c.conn.Close())

	return err
}

func (c *multiplexedBaseConn) newSubConn(id SubConnId) *subConn {
	c.m.OpenedSubConns.With(sideLabel, c.side).Add(1)
	return &subConn{
		id:           id,
		receiverChan: make(chan result),
		parentConn:   c,
	}
}

func (c *multiplexedBaseConn) write(msg any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteJSON(msg)
}

// Sub connection

type subConn struct {
	id           SubConnId
	receiverChan chan result
	parentConn   *multiplexedBaseConn

	mu       sync.Mutex
	isClosed bool
}

func (c *subConn) ID() SubConnId {
	return c.id
}

func (c *subConn) ReadMessage() (messageType int, p []byte, err error) {
	r, ok := <-c.receiverChan
	if !ok {
		return websocket.TextMessage, nil, &websocket.CloseError{
			Code: websocket.CloseAbnormalClosure,
			Text: "Closed",
		}
	}
	return websocket.TextMessage, r.value, r.err
}

func (c *subConn) WriteMessage(_ int, data []byte) error {
	return c.writeMultiplexedMessage(MultiplexedMessage{ID: c.id, Msg: data})
}

func (c *subConn) writeMultiplexedMessage(msg MultiplexedMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isClosed {
		return websocket.ErrCloseSent
	}

	return c.parentConn.write(msg)
}

func (c *subConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isClosed {
		return nil
	}

	c.isClosed = true
	close(c.receiverChan)

	// try to send closing handshake but ignore any error (in case connection is already closed)
	_ = c.parentConn.write(MultiplexedMessage{ID: c.id, Err: io.EOF.Error()})

	// we need to clean up the parents subConns map
	c.parentConn.mu.Lock()
	delete(c.parentConn.subConns, c.id)
	c.parentConn.mu.Unlock()

	return nil
}
