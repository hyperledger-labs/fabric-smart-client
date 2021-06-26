/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"strconv"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/pkg/errors"
)

type Network []*node

func (n Network) Start() {
	ctx := context.Background()
	for _, node := range n {
		node.Node.Start(ctx)
	}
}

type node struct {
	ID          string
	Node        *P2PNode
	Endpoint    string
	DHTEndpoint string
}

func NewVirtualNetwork(port int, numNodes int) (Network, error) {
	var res []*node

	// Setup master
	bootstrapNode, err := newBootstrapNode(port)
	if err != nil {
		return nil, err
	}
	res = append(res, bootstrapNode)

	var nodes []*node
	for i := 0; i < numNodes-1; i++ {
		port++
		n, err := newNode(port, bootstrapNode)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
		res = append(res, n)
	}

	for _, node := range nodes {
		err := eventually(
			func() bool {
				addr, ok := bootstrapNode.Node.Lookup(node.ID)
				if !ok {
					return false
				}

				for _, multiaddr := range addr.Addrs {
					if multiaddr.String() == node.Endpoint {
						return true
					}
				}

				return false
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
				addr, ok := node.Node.Lookup(bootstrapNode.ID)
				if !ok {
					return false
				}

				for _, multiaddr := range addr.Addrs {
					if multiaddr.String() == bootstrapNode.Endpoint {
						return true
					}
				}

				return false
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

func newBootstrapNode(port int) (*node, error) {
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
	p2pNode, err := NewBootstrapNode(nodeEndpoint, &PrivateKeyFromCryptoKey{Key: sk})
	if err != nil {
		return nil, err
	}

	return &node{
		ID:          nodeID,
		Endpoint:    nodeEndpoint,
		DHTEndpoint: nodeDHTEndpoint,
		Node:        p2pNode,
	}, nil
}

func newNode(port int, bootstrapNode *node) (*node, error) {
	sk, pk, err := crypto.GenerateKeyPair(crypto.ECDSA, 0)
	if err != nil {
		return nil, err
	}
	nodeID, err := id(pk)
	if err != nil {
		return nil, err
	}
	nodeEndpoint := "/ip4/127.0.0.1/tcp/" + strconv.Itoa(port)

	p2pNode, err := NewNode(nodeEndpoint, bootstrapNode.DHTEndpoint, &PrivateKeyFromCryptoKey{Key: sk})
	if err != nil {
		return nil, err
	}

	return &node{
		ID:       nodeID,
		Endpoint: nodeEndpoint,
		Node:     p2pNode,
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
