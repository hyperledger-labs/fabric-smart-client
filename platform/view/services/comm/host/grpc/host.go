/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/routing"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger()

type host struct {
	nodeID  host2.PeerID
	routing routing2.ServiceDiscovery
	server  *server
	client  *client
}

func NewHost(nodeID host2.PeerID, routing routing2.ServiceDiscovery, clientConfig grpc2.ClientConfig, serverConfig grpc2.ServerConfig, listenAddress string) (*host, error) {
	srv, err := newServer(listenAddress, serverConfig)
	if err != nil {
		return nil, err
	}
	cli, err := newClient(nodeID, clientConfig)
	if err != nil {
		return nil, err
	}
	return &host{
		nodeID:  nodeID,
		routing: routing,
		server:  srv,
		client:  cli,
	}, nil
}

func (h *host) Addr() string {
	return h.server.Addr()
}

func (h *host) ID() string {
	return h.nodeID
}

func (h *host) Start(newStreamCallback func(stream host2.P2PStream)) error {
	errChan := make(chan error, 1)
	go func() {
		if err := h.server.Start(newStreamCallback); err != nil {
			errChan <- err
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case err := <-errChan:
			return err
		default:
		}
		if time.Now().After(deadline) {
			return errors.Errorf("timeout waiting for grpc server readiness on [%s]", h.server.Addr())
		}
		if err := waitForServerReady(h.server.Addr()); err == nil {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (h *host) NewStream(ctx context.Context, info host2.StreamInfo) (host2.P2PStream, error) {
	if info.RemotePeerAddress = h.routing.Lookup(info.RemotePeerID); len(info.RemotePeerAddress) == 0 {
		return nil, errors.Errorf("no address found for peer [%s]", info.RemotePeerID)
	}
	return h.client.OpenStream(info, ctx)
}

func (h *host) Lookup(peerID host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return h.routing.LookupAll(peerID)
}

func (h *host) StreamHash(info host2.StreamInfo) host2.StreamHash {
	return StreamHash(info)
}

func (h *host) Close() error {
	var err error
	if h.server != nil {
		err = h.server.Close()
	}
	if h.client != nil {
		if cerr := h.client.Close(); err == nil {
			err = cerr
		}
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

var _ host2.P2PHost = (*host)(nil)
