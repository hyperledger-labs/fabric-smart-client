/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	contextIDLabel tracing.LabelName = "context_id"
)

var logger = flogging.MustGetLogger("view-sdk.services.comm.rest-p2p-host")

type host struct {
	routing routing2.ServiceDiscovery
	server  *server
	client  *client
	tracer  trace.Tracer
}

type StreamProvider interface {
	NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error)
	NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error
}

func NewHost(nodeID host2.PeerID, listenAddress host2.PeerIPAddress, routing routing2.ServiceDiscovery, tracerProvider trace.TracerProvider, streamProvider StreamProvider, keyFile, certFile string, rootCACertFiles []string) (*host, error) {
	logger.Infof("Creating new host for node [%s] on [%s] with key, cert at: [%s], [%s]", nodeID, listenAddress, keyFile, certFile)
	p2pClient, err := newClient(streamProvider, nodeID, rootCACertFiles, len(keyFile) > 0 && len(certFile) > 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client")
	}
	p2pServer := newServer(streamProvider, listenAddress, keyFile, certFile)
	return &host{
		server:  p2pServer,
		client:  p2pClient,
		routing: routing,
		tracer: tracerProvider.Tracer("host", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{contextIDLabel},
		})),
	}, nil
}

func (h *host) Start(newStreamCallback func(stream host2.P2PStream)) error {
	go func() {
		if err := h.server.Start(newStreamCallback); err != nil {
			panic(err)
		}
	}()
	return nil
}

func (h *host) NewStream(ctx context.Context, info host2.StreamInfo) (host2.P2PStream, error) {
	newCtx, span := h.tracer.Start(ctx, "stream_send",
		tracing.WithAttributes(tracing.String(contextIDLabel, info.ContextID)),
		trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	//if len(address) == 0 { //TODO
	logger.Debugf("No address passed for peer [%s]. Resolving...", info.RemotePeerID)
	if info.RemotePeerAddress = h.routing.Lookup(info.RemotePeerID); len(info.RemotePeerAddress) == 0 {
		return nil, errors.Errorf("no address found for peer [%s]", info.RemotePeerID)
	}
	logger.Debugf("Resolved address of peer [%s]: %s", info.RemotePeerID, info.RemotePeerAddress)
	//}
	return h.client.OpenStream(info, newCtx)
}

func (h *host) Lookup(peerID host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return h.routing.LookupAll(peerID)
}

func (h *host) StreamHash(info host2.StreamInfo) string {
	return StreamHash(info)
}

func (h *host) Close() error {
	return h.server.Close()
}

func (h *host) Wait() {}

func StreamHash(info host2.StreamInfo) host2.StreamHash {
	return fmt.Sprintf("%s.%s.%s.%s", info.RemotePeerID, info.RemotePeerAddress, info.SessionID, info.ContextID)
}
