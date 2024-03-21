/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	routing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("rest-p2p-host")

type host struct {
	routing routing2.ServiceDiscovery
	server  *server
	client  *client
}

func NewHost(nodeID host2.PeerID, listenAddress host2.PeerIPAddress, routing routing2.ServiceDiscovery, keyFile, certFile string, rootCACertFiles []string) (*host, error) {
	logger.Infof("Creating new host for node [%s] on [%s] with key, cert at: [%s], [%s]", nodeID, listenAddress, keyFile, certFile)
	p2pClient, err := newClient(nodeID, rootCACertFiles, len(keyFile) > 0 && len(certFile) > 0)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create client")
	}
	p2pServer := newServer(listenAddress, keyFile, certFile)
	return &host{
		server:  p2pServer,
		client:  p2pClient,
		routing: routing,
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

func (h *host) NewStream(_ context.Context, info host2.StreamInfo) (host2.P2PStream, error) {
	//if len(address) == 0 { //TODO
	logger.Debugf("No address passed for peer [%s]. Resolving...", info.RemotePeerID)
	if info.RemotePeerAddress = h.routing.Lookup(info.RemotePeerID); len(info.RemotePeerAddress) == 0 {
		return nil, errors.Errorf("no address found for peer [%s]", info.RemotePeerID)
	}
	logger.Debugf("Resolved address of peer [%s]: %s", info.RemotePeerID, info.RemotePeerAddress)
	//}
	return h.client.OpenStream(info)
}

func (h *host) Lookup(peerID host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return h.routing.LookupAll(peerID)
}

func (h *host) StreamHash(info host2.StreamInfo) string {
	logger.Infof("calculate hash from info: %v", info)
	return streamHash(info)
}

func (h *host) Close() error {
	return h.server.Close()
}

func (h *host) Wait() {}
