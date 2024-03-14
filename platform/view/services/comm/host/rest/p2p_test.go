/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest_test

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/stretchr/testify/assert"
)

func TestP2PLayerTestRound(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodes(t)
	comm.P2PLayerTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func TestSessionsTestRound(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodes(t)
	comm.SessionsTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodes(t)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func setupTwoNodes(t *testing.T) (*comm.P2PNode, *comm.P2PNode, string, string) {
	provider := newStaticRouteHostProvider(&staticRouter{
		"bootstrap": []host2.PeerIPAddress{"127.0.0.1:1234"},
		"other":     []host2.PeerIPAddress{"127.0.0.1:5678"},
	})
	bootstrap, _ := provider.NewHost("127.0.0.1:1234",
		"../libp2p/testdata/msp/user1/keystore/priv_sk",
		"../libp2p/testdata/msp/user1/signcerts/User1@org1.example.com-cert.pem",
		"")
	bootstrapNode, err := comm.NewNode(bootstrap)
	assert.NoError(t, err)

	other, _ := provider.NewHost("127.0.0.1:5678",
		"../libp2p/testdata/msp/user2/keystore/priv_sk",
		"../libp2p/testdata/msp/user2/signcerts/User2@org1.example.com-cert.pem",
		"")
	otherNode, err := comm.NewNode(other)
	assert.NoError(t, err)

	return bootstrapNode, otherNode, "bootstrap", "other"
}

type staticRouter map[host2.PeerID][]host2.PeerIPAddress

func (r staticRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	addr, ok := r[id]
	return addr, ok
}

func (r staticRouter) reverseLookup(ipAddress host2.PeerIPAddress) (host2.PeerID, bool) {
	for id, addrs := range r {
		for _, addr := range addrs {
			if ipAddress == addr {
				return id, true
			}
		}
	}
	return "", false
}

type staticRoutHostProvider struct {
	routes *staticRouter
}

func newStaticRouteHostProvider(routes *staticRouter) *staticRoutHostProvider {
	return &staticRoutHostProvider{routes: routes}
}

func (p *staticRoutHostProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string) (host2.P2PHost, error) {
	nodeID, _ := p.routes.reverseLookup(listenAddress)
	return rest.NewHost(nodeID, listenAddress, p.routes, privateKeyPath, certPath, nil)
}

func (p *staticRoutHostProvider) NewHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string, _ host2.PeerIPAddress) (host2.P2PHost, error) {
	return p.NewBootstrapHost(listenAddress, privateKeyPath, certPath)
}
