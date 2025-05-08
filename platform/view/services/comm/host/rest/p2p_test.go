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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/rest/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestP2PLayerTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t, 1254)
	<-time.After(5 * time.Second)
	comm.P2PLayerTestRound(t, bootstrapNode, node)
}

func TestSessionsTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t, 1256)
	<-time.After(5 * time.Second)
	comm.SessionsTestRound(t, bootstrapNode, node)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t, 1258)
	<-time.After(5 * time.Second)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node)
}

func setupTwoNodes(t *testing.T, port int) (*comm.HostNode, *comm.HostNode) {
	bootstrapAddress := fmt.Sprintf("127.0.0.1:%d", port)
	otherAddress := fmt.Sprintf("127.0.0.1:%d", port+1)
	routes := &routing.StaticIDRouter{
		"bootstrap": []host2.PeerIPAddress{bootstrapAddress},
		"other":     []host2.PeerIPAddress{otherAddress},
	}

	bootstrap, _ := newStaticRouteHostProvider(routes, rest.NewConfigFromProperties(
		bootstrapAddress,
		"../libp2p/testdata/msp/user1/keystore/priv_sk",
		"../libp2p/testdata/msp/user1/signcerts/User1@org1.example.com-cert.pem",
	)).GetNewHost()
	bootstrapNode, err := comm.NewNode(bootstrap, noop.NewTracerProvider(), &disabled.Provider{})
	assert.NoError(t, err)

	other, _ := newStaticRouteHostProvider(routes, rest.NewConfigFromProperties(
		otherAddress,
		"../libp2p/testdata/msp/user2/keystore/priv_sk",
		"../libp2p/testdata/msp/user2/signcerts/User2@org1.example.com-cert.pem",
	)).GetNewHost()
	otherNode, err := comm.NewNode(other, noop.NewTracerProvider(), &disabled.Provider{})
	assert.NoError(t, err)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: "bootstrap", Address: bootstrapAddress},
		&comm.HostNode{P2PNode: otherNode, ID: "other", Address: otherAddress}
}

type staticRoutHostProvider struct {
	routes *routing.StaticIDRouter
	config rest.Config
}

func newStaticRouteHostProvider(routes *routing.StaticIDRouter, config rest.Config) *staticRoutHostProvider {
	return &staticRoutHostProvider{routes: routes, config: config}
}

func (p *staticRoutHostProvider) GetNewHost() (host2.P2PHost, error) {
	nodeID, _ := p.routes.ReverseLookup(p.config.ListenAddress())
	discovery := routing.NewServiceDiscovery(p.routes, routing.RoundRobin[host2.PeerIPAddress]())
	return rest.NewHost(nodeID, p.config.ListenAddress(), discovery, noop.NewTracerProvider(), websocket.NewMultiplexedProvider(noop.NewTracerProvider(), &disabled.Provider{}), p.config.ClientTLSConfig(), p.config.ServerTLSConfig()), nil
}
