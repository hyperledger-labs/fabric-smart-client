/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	commhost "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
)

var logger = logging.MustGetLogger()

type host struct {
	nodeID  commhost.PeerID
	routing routing.ServiceDiscovery
	server  *server
	client  *client
}

type StreamProvider interface {
	NewClientStream(info commhost.StreamInfo, ctx context.Context, src commhost.PeerID, config *tls.Config) (commhost.P2PStream, error)
	NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(commhost.P2PStream)) error
	io.Closer
}

// NewHost builds a websocket P2P host, resolving its client and server TLS configuration
// from config. The client reads caPoolProvider on every outbound stream, and the server on
// every handshake when mutual TLS is required, so peers whose anchors it discovers later can
// be dialed and can connect. It returns an error if either TLS configuration cannot be built
// (invalid root CAs or an invalid keypair).
func NewHost(nodeID commhost.PeerID, routing routing.ServiceDiscovery, streamProvider StreamProvider, config Config, caPoolProvider ExtraCAPoolProvider) (*host, error) {
	clientConfig, err := config.ClientTLSConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build client TLS config")
	}
	serverConfig, err := config.ServerTLSConfig(caPoolProvider)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build server TLS config")
	}
	logger.Debugf("Create p2p client for node ID [%s] with TLS config [server: %v] [client: %v]", nodeID, serverConfig, clientConfig)

	return &host{
		nodeID: nodeID,
		server: &server{
			srv: &http.Server{
				Addr:              config.ListenAddress(),
				TLSConfig:         serverConfig,
				ReadHeaderTimeout: config.ReadHeaderTimeout(),
				ReadTimeout:       config.ReadTimeout(),
				WriteTimeout:      config.WriteTimeout(),
				IdleTimeout:       config.IdleTimeout(),
			},
			streamProvider:     streamProvider,
			corsAllowedOrigins: config.CORSAllowedOrigins(),
		},
		client: &client{
			tlsConfig:      clientConfig,
			caPoolProvider: caPoolProvider,
			nodeID:         nodeID,
			streamProvider: streamProvider,
		},
		routing: routing,
	}, nil
}

func (h *host) Addr() string {
	return h.server.srv.Addr
}

func (h *host) ID() string {
	return h.nodeID
}

// PeerID returns the local node's public-key identifier (its PKID).
func (h *host) PeerID() commhost.PeerID {
	return h.nodeID
}

// Start binds the listen address and serves connections on it in the background. The
// address accepts connections as soon as Start returns, since the socket is bound and
// listening by then. Start returns an error only if the address cannot be bound; a serve
// failure later on is logged.
func (h *host) Start(newStreamCallback func(stream commhost.P2PStream)) error {
	if err := h.server.Listen(); err != nil {
		return errors.Wrap(err, "failed to listen")
	}
	go func() {
		// Start returns only when serving fails, and nobody waits for it: this line is the
		// only report of that failure.
		if err := h.server.Start(newStreamCallback); err != nil {
			logger.Errorf("websocket host stopped serving on [%s]: %s", h.server.srv.Addr, err)
		}
	}()
	return nil
}

func (h *host) NewStream(ctx context.Context, info commhost.StreamInfo) (commhost.P2PStream, error) {
	logger.DebugfContext(ctx, "No address passed for peer [%s]. Resolving...", info.RemotePeerID)
	// if len(address) == 0 { //TODO
	if info.RemotePeerAddress = h.routing.Lookup(info.RemotePeerID); len(info.RemotePeerAddress) == 0 {
		return nil, errors.Errorf("no address found for peer [%s]", info.RemotePeerID)
	}
	logger.DebugfContext(ctx, "Resolved address of peer [%s]: %s", info.RemotePeerID, info.RemotePeerAddress)
	// }
	stream, err := h.client.OpenStream(info, ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open stream to peer [%s] at [%s]", info.RemotePeerID, info.RemotePeerAddress)
	}
	logger.DebugfContext(ctx, "New stream opened successfully")
	return stream, nil
}

// Retire implements commhost.RetiringHost when the stream provider caches websockets.
func (h *host) Retire(peerID commhost.PeerID) {
	if h.client == nil {
		return
	}
	if r, ok := h.client.streamProvider.(commhost.RetiringHost); ok {
		r.Retire(peerID)
	}
}

func (*host) StreamHash(info commhost.StreamInfo) string {
	return StreamHash(info)
}

func (h *host) Close() error {
	err := h.server.Close()
	if h.client != nil && h.client.streamProvider != nil {
		err = errors.Join(err, h.client.streamProvider.Close())
	}
	return err
}

func StreamHash(info commhost.StreamInfo) commhost.StreamHash {
	var sb strings.Builder
	sb.WriteString(info.RemotePeerID)
	sb.WriteRune('.')
	sb.WriteString(info.RemotePeerAddress)
	sb.WriteRune('.')
	sb.WriteString(info.SessionID)
	sb.WriteRune('.')
	sb.WriteString(info.ContextID)
	return sb.String()
}
