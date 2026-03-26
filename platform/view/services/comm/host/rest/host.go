/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
)

var logger = logging.MustGetLogger()

type host struct {
	nodeID  host2.PeerID
	routing routing2.ServiceDiscovery
	server  *server
	client  *client
}

type StreamProvider interface {
	NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error)
	NewServerStream(writer http.ResponseWriter, request *http.Request, newStreamCallback func(host2.P2PStream)) error
	io.Closer
}

func NewHost(nodeID host2.PeerID, routing routing2.ServiceDiscovery, streamProvider StreamProvider, config Config, caPoolProvider ExtraCAPoolProvider) *host {
	clientConfig := config.ClientTLSConfig(caPoolProvider)
	serverConfig := config.ServerTLSConfig(caPoolProvider)
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
			nodeID:         nodeID,
			streamProvider: streamProvider,
		},
		routing: routing,
	}
}

func (h *host) Addr() string {
	return h.server.srv.Addr
}

func (h *host) ID() string {
	return h.nodeID
}

func (h *host) Start(newStreamCallback func(stream host2.P2PStream)) error {
	if err := h.server.Listen(); err != nil {
		return errors.Wrapf(err, "failed to listen")
	}

	errChan := make(chan error, 1)
	go func() {
		if err := h.server.Start(newStreamCallback); err != nil {
			logger.Errorf("error starting server: %s", err)
			errChan <- err
		}
	}()

	waitReady := func() error {
		deadline := time.Now().Add(2 * time.Second)
		for {
			select {
			case err := <-errChan:
				return err
			default:
			}
			addr := h.server.srv.Addr
			if time.Now().After(deadline) {
				return errors.Errorf("timeout waiting for REST server readiness on [%s]", addr)
			}

			target := addr
			host, port, err := net.SplitHostPort(addr)
			if err == nil && (host == "" || host == "0.0.0.0" || host == "::") {
				target = net.JoinHostPort("127.0.0.1", port)
			}
			conn, err := net.DialTimeout("tcp", target, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return nil
			}
			time.Sleep(25 * time.Millisecond)
		}
	}

	select {
	case err := <-errChan:
		return err
	default:
	}
	return waitReady()
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
	err := h.server.Close()
	if h.client != nil && h.client.streamProvider != nil {
		err = errors.Join(err, h.client.streamProvider.Close())
	}
	return err
}

func (h *host) Wait() {}

func StreamHash(info host2.StreamInfo) host2.StreamHash {
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
