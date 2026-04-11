/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mock/libp2p_config.go -fake-name LibP2PConfig . libp2pConfig

func TestMain(m *testing.M) {
	spec := os.Getenv("FABRIC_LOGGING_SPEC")
	if len(spec) == 0 {
		spec = os.Getenv("FSC_LOGSPEC")
	}
	if len(spec) == 0 {
		spec = "error"
	}
	logging.Init(logging.Config{
		LogSpec: spec,
	})
	os.Exit(m.Run())
}

func TestP2PLayerTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	comm.P2PLayerTestRound(t, bootstrapNode, node)
}

func TestSessionsTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	comm.SessionsTestRound(t, bootstrapNode, node)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node)
}

func TestSessionsMultipleMessagesTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodes(t)
	comm.SessionsMultipleMessagesTestRound(t, bootstrapNode, node)
}

func TestSessionsTwoNodesTestRound(t *testing.T) {
	bootstrapNode, node1, node2 := setupThreeNodes(t)
	<-time.After(100 * time.Millisecond)

	comm.SessionsNodesTestRound(t, bootstrapNode, []*comm.HostNode{node1, node2}, 2)
}

func generateKey(t *testing.T) (crypto.PrivKey, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privKey, pubKey, err := crypto.ECDSAKeyPairFromKey(priv)
	require.NoError(t, err)
	ID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)
	return privKey, ID.String()
}

func freeLibP2PAddresses(t *testing.T, n int) []string {
	t.Helper()
	listeners := make([]net.Listener, n)
	addresses := make([]string, n)
	for i := range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listeners[i] = l
		addresses[i] = fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", l.Addr().(*net.TCPAddr).Port)
	}
	for _, l := range listeners {
		require.NoError(t, l.Close())
	}
	return addresses
}

func setupTwoNodes(t *testing.T) (*comm.HostNode, *comm.HostNode) {
	t.Helper()
	bootstrapSK, bootstrapID := generateKey(t)
	nodeSK, nodeID := generateKey(t)

	addrs := freeLibP2PAddresses(t, 2)
	bootstrapNodeEndpoint := addrs[0]
	nodeEndpoint := addrs[1]

	bootstrapConfig := &mock.LibP2PConfig{}
	bootstrapConfig.ListenAddressReturns(bootstrapNodeEndpoint)
	bootstrapHost, err := newLibP2PHost(bootstrapConfig, bootstrapSK, newMetrics(&disabled.Provider{}), true, "")
	require.NoError(t, err)
	bootstrapNode, err := comm.NewNode(t.Context(), bootstrapHost, &disabled.Provider{})
	require.NoError(t, err)

	nodeConfig := &mock.LibP2PConfig{}
	nodeConfig.ListenAddressReturns(nodeEndpoint)
	anotherHost, err := newLibP2PHost(nodeConfig, nodeSK, newMetrics(&disabled.Provider{}), false, bootstrapNodeEndpoint+"/p2p/"+bootstrapID)
	require.NoError(t, err)
	anotherNode, err := comm.NewNode(t.Context(), anotherHost, &disabled.Provider{})
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapID, Address: bootstrapNodeEndpoint},
		&comm.HostNode{P2PNode: anotherNode, ID: nodeID, Address: nodeEndpoint}
}

func setupThreeNodes(t *testing.T) (*comm.HostNode, *comm.HostNode, *comm.HostNode) {
	t.Helper()
	bootstrapSK, bootstrapID := generateKey(t)
	node1SK, node1ID := generateKey(t)
	node2SK, node2ID := generateKey(t)

	addrs := freeLibP2PAddresses(t, 3)
	bootstrapNodeEndpoint := addrs[0]
	node1Endpoint := addrs[1]
	node2Endpoint := addrs[2]

	bootstrapConfig := &mock.LibP2PConfig{}
	bootstrapConfig.ListenAddressReturns(bootstrapNodeEndpoint)
	bootstrapHost, err := newLibP2PHost(bootstrapConfig, bootstrapSK, newMetrics(&disabled.Provider{}), true, "")
	require.NoError(t, err)
	bootstrapNode, err := comm.NewNode(t.Context(), bootstrapHost, &disabled.Provider{})
	require.NoError(t, err)

	node1Config := &mock.LibP2PConfig{}
	node1Config.ListenAddressReturns(node1Endpoint)
	node1Host, err := newLibP2PHost(node1Config, node1SK, newMetrics(&disabled.Provider{}), false, bootstrapNodeEndpoint+"/p2p/"+bootstrapID)
	require.NoError(t, err)
	node1, err := comm.NewNode(t.Context(), node1Host, &disabled.Provider{})
	require.NoError(t, err)

	node2Config := &mock.LibP2PConfig{}
	node2Config.ListenAddressReturns(node2Endpoint)
	node2Host, err := newLibP2PHost(node2Config, node2SK, newMetrics(&disabled.Provider{}), false, bootstrapNodeEndpoint+"/p2p/"+bootstrapID)
	require.NoError(t, err)
	node2, err := comm.NewNode(t.Context(), node2Host, &disabled.Provider{})
	require.NoError(t, err)

	time.Sleep(1 * time.Second)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapID, Address: bootstrapNodeEndpoint},
		&comm.HostNode{P2PNode: node1, ID: node1ID, Address: node1Endpoint},
		&comm.HostNode{P2PNode: node2, ID: node2ID, Address: node2Endpoint}
}
