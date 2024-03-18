/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/routing"
	"github.com/stretchr/testify/assert"
)

func TestP2PLayerTestRound(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodes(t, 1254)
	<-time.After(5 * time.Second)
	comm.P2PLayerTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func TestSessionsTestRound(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodes(t, 1256)
	<-time.After(5 * time.Second)
	comm.SessionsTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodes(t, 1258)
	<-time.After(5 * time.Second)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func setupTwoNodes(t *testing.T, port int) (*comm.P2PNode, *comm.P2PNode, string, string) {
	bootstrapAddress := fmt.Sprintf("127.0.0.1:%d", port)
	otherAddress := fmt.Sprintf("127.0.0.1:%d", port+1)
	provider := newStaticRouteHostProvider(&routing.StaticIDRouter{
		"bootstrap": []host2.PeerIPAddress{bootstrapAddress},
		"other":     []host2.PeerIPAddress{otherAddress},
	})
	bootstrap, _ := provider.NewHost(bootstrapAddress,
		"../libp2p/testdata/msp/user1/keystore/priv_sk",
		"../libp2p/testdata/msp/user1/signcerts/User1@org1.example.com-cert.pem",
		"")
	bootstrapNode, err := comm.NewNode(bootstrap)
	assert.NoError(t, err)

	other, _ := provider.NewHost(otherAddress,
		"../libp2p/testdata/msp/user2/keystore/priv_sk",
		"../libp2p/testdata/msp/user2/signcerts/User2@org1.example.com-cert.pem",
		"")
	otherNode, err := comm.NewNode(other)
	assert.NoError(t, err)

	return bootstrapNode, otherNode, "bootstrap", "other"
}

type staticRoutHostProvider struct {
	routes *routing.StaticIDRouter
}

func newStaticRouteHostProvider(routes *routing.StaticIDRouter) *staticRoutHostProvider {
	return &staticRoutHostProvider{routes: routes}
}

func (p *staticRoutHostProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string) (host2.P2PHost, error) {
	nodeID, _ := p.routes.ReverseLookup(listenAddress)
	return rest.NewHost(nodeID, listenAddress, p.routes, privateKeyPath, certPath, nil)
}

func (p *staticRoutHostProvider) NewHost(listenAddress host2.PeerIPAddress, privateKeyPath, certPath string, _ host2.PeerIPAddress) (host2.P2PHost, error) {
	return p.NewBootstrapHost(listenAddress, privateKeyPath, certPath)
}
