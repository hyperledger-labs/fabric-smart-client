/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"os"
	"testing"
	"time"

	assert2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

func TestP2PLayerTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodesFromFiles(t)
	comm.P2PLayerTestRound(t, bootstrapNode, node)
}

func TestSessionsTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodesFromFiles(t)
	comm.SessionsTestRound(t, bootstrapNode, node)
}

func TestSessionsForMPCTestRound(t *testing.T) {
	bootstrapNode, node := setupTwoNodesFromFiles(t)
	comm.SessionsForMPCTestRound(t, bootstrapNode, node)
}

func setupTwoNodesFromFiles(t *testing.T) (*comm.HostNode, *comm.HostNode) {
	bootstrapNodePK := "testdata/dht.pub"
	bootstrapNodeSK := "testdata/dht.priv"
	bootstrapNodeID := idForParty(t, bootstrapNodePK)
	nodePK := "testdata/dht1.pub"
	nodeSK := "testdata/dht1.priv"
	nodeID := idForParty(t, nodePK)
	bootstrapNodeEndpoint := "/ip4/127.0.0.1/tcp/1234"
	nodeEndpoint := "/ip4/127.0.0.1/tcp/1235"

	bootstrapNode, anotherNode, err := setupTwoNodes(t, bootstrapNodeID, bootstrapNodeEndpoint, nodeID, nodeEndpoint, bootstrapNodeSK, nodeSK)
	assert2.NoError(err)

	return &comm.HostNode{P2PNode: bootstrapNode, ID: bootstrapNodeID, Address: ""},
		&comm.HostNode{P2PNode: anotherNode, ID: nodeID, Address: ""}
}

func setupTwoNodes(t *testing.T, bootstrapNodeID, bootstrapNodeEndpoint, nodeID, nodeEndpoint string, bootstrapNodeSK, nodeSK string) (*comm.P2PNode, *comm.P2PNode, error) {
	// catch panic and return error
	var err error
	defer func() {
		if r := recover(); r != nil {
			if err2, ok := r.(error); ok {
				err = err2
			}
		}
	}()

	provider := newHostProvider(&disabled.Provider{})

	bootstrapNodePrivBytes, err := os.ReadFile(bootstrapNodeSK)
	assert.NoError(t, err)
	bootstrapHostKey, err := crypto.UnmarshalECDSAPrivateKey(bootstrapNodePrivBytes)
	assert.NoError(t, err)
	bootstrapHost, err := provider.NewBootstrapHost(bootstrapNodeEndpoint, bootstrapHostKey)
	assert.NoError(t, err)
	bootstrapNode, err := comm.NewNode(bootstrapHost)
	assert.NoError(t, err)
	assert.NotNil(t, bootstrapNode)

	anotherHostPrivBytes, err := os.ReadFile(nodeSK)
	assert.NoError(t, err)
	anotherHostKey, err := crypto.UnmarshalECDSAPrivateKey(anotherHostPrivBytes)
	assert.NoError(t, err)
	anotherHost, err := provider.NewHost(nodeEndpoint, bootstrapNodeEndpoint+"/p2p/"+bootstrapNodeID, anotherHostKey)
	assert.NoError(t, err)
	anotherNode, err := comm.NewNode(anotherHost)
	assert.NoError(t, err)
	assert.NotNil(t, anotherNode)

	assert.Eventually(t, func() bool {
		addrs, ok := bootstrapNode.Lookup(nodeID)
		return ok && slices.Contains(addrs, nodeEndpoint)
	}, 60*time.Second, 500*time.Millisecond)

	assert.Eventually(t, func() bool {
		addrs, ok := anotherNode.Lookup(bootstrapNodeID)
		return ok && slices.Contains(addrs, bootstrapNodeEndpoint)
	}, 60*time.Second, 500*time.Millisecond)

	return bootstrapNode, anotherNode, err
}

func idForParty(t *testing.T, keyFile string) string {
	keyBytes, err := os.ReadFile(keyFile)
	assert.NoError(t, err)

	key, err := crypto.UnmarshalECDSAPublicKey(keyBytes)
	assert.NoError(t, err)

	ID, err := peer.IDFromPublicKey(key)
	assert.NoError(t, err)

	return ID.String()
}
