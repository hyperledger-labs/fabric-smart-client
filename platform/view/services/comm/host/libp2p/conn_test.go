/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

type Network []*node

func TestSessionTwoParties(t *testing.T) {
	t.Parallel()
	network, err := NewVirtualNetwork(t, 12345, 2)
	require.NoError(t, err)

	io.SessionTwoParties(t, network[0], network[1])
}

type node struct {
	*comm.P2PNode
	id          string
	endpoint    string
	dhtEndpoint string
}

func (n *node) ID() string {
	return n.id
}

func NewVirtualNetwork(t *testing.T, port int, numNodes int) (Network, error) {
	t.Helper()

	var res []*node

	// Setup master
	bootstrapNode, err := newBootstrapNode(t, port)
	if err != nil {
		return nil, err
	}
	res = append(res, bootstrapNode)

	var nodes []*node
	for i := 0; i < numNodes-1; i++ {
		port++
		n, err := newNode(t, port, bootstrapNode)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
		res = append(res, n)
	}

	for _, node := range nodes {
		err := eventually(
			func() bool {
				addrs, ok := bootstrapNode.Lookup(node.id)
				return ok && slices.Contains(addrs, node.endpoint)
			},
			10*time.Second,
			500*time.Millisecond,
		)
		if err != nil {
			return nil, err
		}
	}

	for _, node := range nodes {
		err := eventually(
			func() bool {
				addrs, ok := node.Lookup(bootstrapNode.id)
				return ok && slices.Contains(addrs, bootstrapNode.endpoint)
			},
			10*time.Second,
			500*time.Millisecond,
		)
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func id(pk crypto.PubKey) (string, error) {
	ID, err := peer.IDFromPublicKey(pk)
	if err != nil {
		return "", err
	}
	return ID.String(), nil
}

func newBootstrapNode(t *testing.T, port int) (*node, error) {
	t.Helper()
	sk, pk, err := crypto.GenerateKeyPair(crypto.ECDSA, 0)
	if err != nil {
		return nil, err
	}
	nodeID, err := id(pk)
	if err != nil {
		return nil, err
	}
	nodeEndpoint := "/ip4/127.0.0.1/tcp/" + strconv.Itoa(port)
	nodeDHTEndpoint := nodeEndpoint + "/p2p/" + nodeID
	config := &mock.LibP2PConfig{}
	config.ListenAddressReturns(nodeEndpoint)
	h, err := newLibP2PHost(config, sk, newMetrics(&disabled.Provider{}), true, "")
	if err != nil {
		return nil, err
	}
	p2pNode, err := comm.NewNode(t.Context(), h, &disabled.Provider{})
	if err != nil {
		return nil, err
	}

	return &node{
		P2PNode:     p2pNode,
		id:          nodeID,
		endpoint:    nodeEndpoint,
		dhtEndpoint: nodeDHTEndpoint,
	}, nil
}

func newNode(t *testing.T, port int, bootstrapNode *node) (*node, error) {
	t.Helper()
	sk, pk, err := crypto.GenerateKeyPair(crypto.ECDSA, 0)
	if err != nil {
		return nil, err
	}
	nodeID, err := id(pk)
	if err != nil {
		return nil, err
	}
	nodeEndpoint := "/ip4/127.0.0.1/tcp/" + strconv.Itoa(port)

	config := &mock.LibP2PConfig{}
	config.ListenAddressReturns(nodeEndpoint)
	h, err := newLibP2PHost(config, sk, newMetrics(&disabled.Provider{}), false, bootstrapNode.dhtEndpoint)
	if err != nil {
		return nil, err
	}
	p2pNode, err := comm.NewNode(t.Context(), h, &disabled.Provider{})
	if err != nil {
		return nil, err
	}

	return &node{
		id:       nodeID,
		endpoint: nodeEndpoint,
		P2PNode:  p2pNode,
	}, nil
}

func eventually(condition func() bool, waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) error {
	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		if condition() {
			return nil
		}
		select {
		case <-timer.C:
			return errors.Errorf("Condition never satisfied %v", msgAndArgs...)
		case <-ticker.C:
		}
	}
}
