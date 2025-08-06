/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"go.opentelemetry.io/otel/trace"
)

func NewSimpleProvider() *SimpleProvider {
	return &SimpleProvider{}
}

type SimpleProvider struct{}

func (p *SimpleProvider) NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error) {
	logger.Debugf("Creating new stream from [%s] to [%s@%s]...", src, info.RemotePeerID, info.RemotePeerAddress)
	tlsEnabled := config.InsecureSkipVerify || config.RootCAs != nil
	url := url.URL{Scheme: schemes[tlsEnabled], Host: info.RemotePeerAddress, Path: "/p2p"}
	// We use the background context instead of passing the existing context,
	// because the net/http server doesn't monitor connections upgraded to WebSocket.
	// Hence, when the connection is lost, the context will not be cancelled.
	conn, err := web2.OpenWSClientConn(url.String(), config)
	logger.Debugf("Successfully connected to websocket")
	if err != nil {
		logger.Errorf("Dial failed: %s\n", err.Error())
		return nil, err
	}
	spanContext := trace.SpanContextFromContext(ctx)
	marshalledSpanContext, err := tracing.MarshalContext(spanContext)
	if err != nil {
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
		return nil, errors.Wrapf(err, "failed to send meta message")
	}
	logger.Debugf("Stream opened to [%s@%s]", info.RemotePeerID, info.RemotePeerAddress)
	return NewWSStream(conn, ctx, info), nil
}

func (p *SimpleProvider) NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error {
	conn, err := server.OpenWSServerConn(writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to open websocket")
	}
	logger.Debugf("Successfully opened server-side websocket")

	var meta StreamMeta
	if err := conn.ReadJSON(&meta); err != nil {
		return errors.Wrapf(err, "failed to read meta info")
	}
	logger.Debugf("Read meta info: [%s,%s]: %s", meta.ContextID, meta.SessionID, meta.SpanContext)
	// Propagating the request context will not make a difference (see comment in newClientStream)
	spanContext, err := tracing.UnmarshalContext(meta.SpanContext)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal span context")
	}
	logger.Debugf("Received response with context: %v", spanContext)
	newStreamCallback(NewWSStream(conn, trace.ContextWithRemoteSpanContext(context.Background(), spanContext), host2.StreamInfo{
		RemotePeerID:      meta.PeerID,
		RemotePeerAddress: request.RemoteAddr,
		ContextID:         meta.ContextID,
		SessionID:         meta.SessionID,
	}))
	return nil
}
