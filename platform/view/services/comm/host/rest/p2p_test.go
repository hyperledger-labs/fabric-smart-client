/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
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
	provider := newMapRouteProvider(&mapRouter{
		"bootstrap": []host2.PeerIPAddress{"127.0.0.1:1234"},
		"other":     []host2.PeerIPAddress{"127.0.0.1:5678"},
	})
	bootstrap, _ := provider.NewHost("127.0.0.1:1234", "")
	bootstrapNode, err := comm.NewNode(bootstrap)
	assert.NoError(t, err)

	other, _ := provider.NewHost("127.0.0.1:5678", "")
	otherNode, err := comm.NewNode(other)
	assert.NoError(t, err)

	return bootstrapNode, otherNode, "bootstrap", "other"
}
