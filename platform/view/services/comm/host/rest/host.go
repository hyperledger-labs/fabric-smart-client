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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

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

func NewHost(nodeID host2.PeerID, listenAddress host2.PeerIPAddress, routing routing2.ServiceDiscovery, tracerProvider trace.TracerProvider, streamProvider StreamProvider, clientConfig, serverConfig *tls.Config) *host {
	logger.Debugf("Create p2p client for node ID [%s] with TLS config [server: %v] [client: %v]", nodeID, serverConfig, clientConfig)
	return &host{
		server: &server{
			srv: &http.Server{
				Addr:      listenAddress,
				TLSConfig: serverConfig,
			},
			streamProvider: streamProvider,
		},
		client: &client{
			tlsConfig:      clientConfig,
			nodeID:         nodeID,
			streamProvider: streamProvider,
		},
		routing: routing,
		tracer: tracerProvider.Tracer("host", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "viewsdk",
			LabelNames: []tracing.LabelName{},
		})),
	}
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
	logger.DebugfContext(ctx, "No address passed for peer [%s]. Resolving...", info.RemotePeerID)
	defer logger.DebugfContext(ctx, "New stream opened")
	// if len(address) == 0 { //TODO
	if info.RemotePeerAddress = h.routing.Lookup(info.RemotePeerID); len(info.RemotePeerAddress) == 0 {
		return nil, errors.Errorf("no address found for peer [%s]", info.RemotePeerID)
	}
	logger.DebugfContext(ctx, "Resolved address of peer [%s]: %s", info.RemotePeerID, info.RemotePeerAddress)
	// }
	return h.client.OpenStream(info, ctx)
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
