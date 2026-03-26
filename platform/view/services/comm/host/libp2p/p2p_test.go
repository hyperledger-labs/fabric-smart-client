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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

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
	assert.NoError(t, err)
	privKey, pubKey, err := crypto.ECDSAKeyPairFromKey(priv)
	assert.NoError(t, err)
	ID, err := peer.IDFromPublicKey(pubKey)
	assert.NoError(t, err)
	return privKey, ID.String()
}

func freeLibP2PAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	assert.NoError(t, l.Close())
	return fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port)
}

func setupTwoNodes(t *testing.T) (*comm.HostNode, *comm.HostNode) {
	bootstrapSK, bootstrapID := generateKey(t)
	nodeSK, nodeID := generateKey(t)

	bootstrapNodeEndpoint := freeLibP2PAddress(t)
	nodeEndpoint := freeLibP2PAddress(t)

	bootstrapHost, err := newLibP2PHost(bootstrapNodeEndpoint, bootstrapSK, newMetrics(&disabled.Provider{}), true, "")
	assert.NoError(t, err)
	bootstrapNode, err := comm.NewNode(t.Context(), bootstrapHost, &disabled.Provider{})
	assert.NoError(t, err)

	anotherHost, err := newLibP2PHost(nodeEndpoint, nodeSK, newMetrics(&disabled.Provider{}), false, bootstrapNodeEndpoint+"/p2p/"+bootstrapID)
	assert.NoError(t, err)
	anotherNode, err := comm.NewNode(t.Context(), anotherHost, &disabled.Provider{})
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapID, Address: bootstrapNodeEndpoint},
		&comm.HostNode{P2PNode: anotherNode, ID: nodeID, Address: nodeEndpoint}
}

func setupThreeNodes(t *testing.T) (*comm.HostNode, *comm.HostNode, *comm.HostNode) {
	bootstrapSK, bootstrapID := generateKey(t)
	node1SK, node1ID := generateKey(t)
	node2SK, node2ID := generateKey(t)

	bootstrapNodeEndpoint := freeLibP2PAddress(t)
	node1Endpoint := freeLibP2PAddress(t)
	node2Endpoint := freeLibP2PAddress(t)

	bootstrapHost, err := newLibP2PHost(bootstrapNodeEndpoint, bootstrapSK, newMetrics(&disabled.Provider{}), true, "")
	assert.NoError(t, err)
	bootstrapNode, err := comm.NewNode(t.Context(), bootstrapHost, &disabled.Provider{})
	assert.NoError(t, err)

	node1Host, err := newLibP2PHost(node1Endpoint, node1SK, newMetrics(&disabled.Provider{}), false, bootstrapNodeEndpoint+"/p2p/"+bootstrapID)
	assert.NoError(t, err)
	node1, err := comm.NewNode(t.Context(), node1Host, &disabled.Provider{})
	assert.NoError(t, err)

	node2Host, err := newLibP2PHost(node2Endpoint, node2SK, newMetrics(&disabled.Provider{}), false, bootstrapNodeEndpoint+"/p2p/"+bootstrapID)
	assert.NoError(t, err)
	node2, err := comm.NewNode(t.Context(), node2Host, &disabled.Provider{})
	assert.NoError(t, err)

	time.Sleep(1 * time.Second)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapID, Address: bootstrapNodeEndpoint},
		&comm.HostNode{P2PNode: node1, ID: node1ID, Address: node1Endpoint},
		&comm.HostNode{P2PNode: node2, ID: node2ID, Address: node2Endpoint}
}
