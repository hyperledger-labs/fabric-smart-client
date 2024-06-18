/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"strconv"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/exp/slices"
)

type Network []*node

func TestSessionTwoParties(t *testing.T) {
	network, err := NewVirtualNetwork(12345, 2)
	assert.NoError(t, err)

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

func NewVirtualNetwork(port int, numNodes int) (Network, error) {
	var res []*node

	// Setup master
	provider := newHostProvider(&disabled.Provider{})
	bootstrapNode, err := newBootstrapNode(port, provider)
	if err != nil {
		return nil, err
	}
	res = append(res, bootstrapNode)

	var nodes []*node
	for i := 0; i < numNodes-1; i++ {
		port++
		n, err := newNode(port, bootstrapNode, provider)
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
			60*time.Second,
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
			60*time.Second,
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

func newBootstrapNode(port int, provider *hostProvider) (*node, error) {
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
	h, err := provider.NewBootstrapHost(nodeEndpoint, sk)
	if err != nil {
		return nil, err
	}
	p2pNode, err := comm.NewNode(h, noop.NewTracerProvider())
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

func newNode(port int, bootstrapNode *node, provider *hostProvider) (*node, error) {
	sk, pk, err := crypto.GenerateKeyPair(crypto.ECDSA, 0)
	if err != nil {
		return nil, err
	}
	nodeID, err := id(pk)
	if err != nil {
		return nil, err
	}
	nodeEndpoint := "/ip4/127.0.0.1/tcp/" + strconv.Itoa(port)

	h, err := provider.NewHost(nodeEndpoint, bootstrapNode.dhtEndpoint, sk)
	if err != nil {
		return nil, err
	}
	p2pNode, err := comm.NewNode(h, noop.NewTracerProvider())
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
	ch := make(chan bool, 1)

	timer := time.NewTimer(waitFor)
	defer timer.Stop()

	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for tick := ticker.C; ; {
		select {
		case <-timer.C:
			return errors.Errorf("Condition never satisfied %v", msgAndArgs...)
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case v := <-ch:
			if v {
				return nil
			}
			tick = ticker.C
		}
	}
}
