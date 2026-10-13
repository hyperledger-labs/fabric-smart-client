/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package ws

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

func NewSimpleProvider() *SimpleProvider {
	return &SimpleProvider{}
}

// SimpleProvider opens one websocket per stream. It owns the streams it opens and accepts, and
// Close ends all of them. The zero value is ready to use.
type SimpleProvider struct {
	// mu protects streams and closed. It is never held across a network dial, write, or close.
	mu      sync.Mutex
	streams map[*stream]struct{}
	closed  bool
}

// Close closes the provider and every stream it opened or accepted. Once it starts,
// NewClientStream fails and NewServerStream rejects new websockets. Later calls return nil.
func (p *SimpleProvider) Close() error {
	p.mu.Lock()
	p.closed = true
	streams := make([]*stream, 0, len(p.streams))
	for s := range p.streams {
		streams = append(streams, s)
	}
	clear(p.streams)
	p.mu.Unlock()

	var err error
	for _, s := range streams {
		err = errors.Join(err, s.Close())
	}
	return err
}

// track registers s until it closes, or closes s and fails if the provider is closed.
func (p *SimpleProvider) track(s *stream) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		_ = s.Close()
		return errors.New("provider is closed")
	}
	if p.streams == nil {
		p.streams = make(map[*stream]struct{})
	}
	p.streams[s] = struct{}{}
	p.mu.Unlock()

	// Closing a stream cancels its context.
	context.AfterFunc(s.ctx, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.streams, s)
	})
	return nil
}

func (p *SimpleProvider) NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error) {
	logger.Debugf("Creating new stream from [%s] to [%s@%s]...", src, info.RemotePeerID, info.RemotePeerAddress)
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, errors.New("provider is closed")
	}
	tlsEnabled := config != nil
	url := url.URL{Scheme: schemes[tlsEnabled], Host: info.RemotePeerAddress, Path: "/p2p"}
	// We use the background context instead of passing the existing context,
	// because the net/http server doesn't monitor connections upgraded to WebSocket.
	// Hence, when the connection is lost, the context will not be cancelled.
	conn, err := web2.OpenWSClientConn(url.String(), config)
	if err != nil {
		logger.Errorf("Dial failed: %s", err.Error())
		return nil, errors.Wrapf(err, "failed to open websocket")
	}
	logger.Debugf("Successfully connected to websocket")
	spanContext := trace.SpanContextFromContext(ctx)
	marshalledSpanContext, err := tracing.MarshalContext(spanContext)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	logger.Debugf("Open stream with context: [%v]: %v", spanContext, marshalledSpanContext)
	meta := StreamMeta{
		ContextID:   info.ContextID,
		SessionID:   info.SessionID,
		PeerID:      src,
		SpanContext: marshalledSpanContext,
	}
	err = conn.WriteJSON(&meta)
	if err != nil {
		_ = conn.Close()
		return nil, errors.Wrapf(err, "failed to send meta message")
	}
	logger.Debugf("Stream opened to [%s@%s]", info.RemotePeerID, info.RemotePeerAddress)
	// The stream is cached and reused across multiple calls (e.g. multiple rounds of a
	// protocol), so its lifetime must not be tied to the context of the call that happened
	// to create it: cancelling that per-call context (e.g. on successful completion) would
	// otherwise tear down the connection out from under subsequent, unrelated calls that
	// reuse the same stream. We only propagate the span, as a local parent, for tracing
	// purposes.
	s := NewWSStream(conn, trace.ContextWithSpanContext(context.Background(), spanContext), info)
	if err := p.track(s); err != nil {
		return nil, err
	}
	return s, nil
}

func (p *SimpleProvider) NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error {
	expectedPeerID, err := expectedPeerIDFromRequest(request)
	if err != nil {
		return errors.Wrapf(err, "failed extracting expected peerID from TLS certificate")
	}

	conn, err := server.OpenWSServerConn(writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to open websocket")
	}
	logger.Debugf("Successfully opened server-side websocket")
	streamTransferred := false
	defer func() {
		if !streamTransferred {
			_ = conn.Close()
		}
	}()

	var meta StreamMeta
	if err := conn.ReadJSON(&meta); err != nil {
		return errors.Wrapf(err, "failed to read meta info")
	}

	// SECURITY INVARIANT: We MUST NOT trust the PeerID provided by the client in the metadata.
	// We MUST verify that it matches the identity extracted from the verified TLS certificate.
	// This prevents PeerID spoofing (Issue #871, #1037).
	if meta.PeerID != expectedPeerID {
		return errors.Errorf("peer identity binding failed: claimed [%s], tls [%s]", meta.PeerID, expectedPeerID)
	}
	logger.Debugf("Read meta info: [%s,%s]: %s", meta.ContextID, meta.SessionID, meta.SpanContext)
	// Propagating the request context will not make a difference (see comment in SimpleProvider.NewClientStream)
	spanContext := remoteSpanContext(&meta)
	logger.Debugf("Accepted stream with remote span context: %v", spanContext)
	s := NewWSStream(conn, trace.ContextWithRemoteSpanContext(context.Background(), spanContext), host2.StreamInfo{
		RemotePeerID:      meta.PeerID,
		RemotePeerAddress: request.RemoteAddr,
		ContextID:         meta.ContextID,
		SessionID:         meta.SessionID,
	})
	streamTransferred = true
	if err := p.track(s); err != nil {
		return err
	}
	newStreamCallback(s)
	return nil
}
