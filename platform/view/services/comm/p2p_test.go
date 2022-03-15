/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
)

func idForParty(t *testing.T, keyFile string) string {
	keyBytes, err := ioutil.ReadFile(keyFile)
	assert.NoError(t, err)

	key, err := crypto.UnmarshalECDSAPublicKey(keyBytes)
	assert.NoError(t, err)

	ID, err := peer.IDFromPublicKey(key)
	assert.NoError(t, err)

	return ID.String()
}

func getBootstrapNode(t *testing.T, bootstrapNodeEndpoint string, keyDispenser PrivateKeyDispenser) *P2PNode {
	node, err := NewBootstrapNode(bootstrapNodeEndpoint, keyDispenser)
	assert.NoError(t, err)
	assert.NotNil(t, node)

	return node
}

func getNode(t *testing.T, bootstrapNodeID, bootstrapNodeEndpoint, nodeEndpoint string, keyDispenser PrivateKeyDispenser) *P2PNode {
	bootstrapNodeDHTEndpoint := bootstrapNodeEndpoint + "/p2p/" + bootstrapNodeID
	node, err := NewNode(nodeEndpoint, bootstrapNodeDHTEndpoint, keyDispenser)
	assert.NoError(t, err)
	assert.NotNil(t, node)

	return node
}

func setupTwoNodes(t *testing.T, bootstrapNodeID, bootstrapNodeEndpoint, nodeID, nodeEndpoint string, bootstrapNodeKeyDispenser, nodeKeyDispenser PrivateKeyDispenser) (bootstrapNode *P2PNode, node *P2PNode, err error) {
	// catch panic and return error
	defer func() {
		if r := recover(); r != nil {
			if err2, ok := r.(error); ok {
				err = err2
			}
		}
	}()
	bootstrapNode = getBootstrapNode(t, bootstrapNodeEndpoint, bootstrapNodeKeyDispenser)
	node = getNode(t, bootstrapNodeID, bootstrapNodeEndpoint, nodeEndpoint, nodeKeyDispenser)

	assert.Eventually(
		t,
		func() bool {
			addr, ok := bootstrapNode.Lookup(nodeID)
			if !ok {
				return false
			}

			for _, multiaddr := range addr.Addrs {
				if multiaddr.String() == nodeEndpoint {
					return true
				}
			}

			return false
		},
		60*time.Second,
		500*time.Millisecond,
	)

	assert.Eventually(
		t,
		func() bool {
			addr, ok := node.Lookup(bootstrapNodeID)
			if !ok {
				return false
			}

			for _, multiaddr := range addr.Addrs {
				if multiaddr.String() == bootstrapNodeEndpoint {
					return true
				}
			}

			return false
		},
		60*time.Second,
		500*time.Millisecond,
	)

	return bootstrapNode, node, err
}

func setupTwoNodesFromFiles(t *testing.T) (*P2PNode, *P2PNode, string, string) {
	bootstrapNodePK := "testdata/dht.pub"
	bootstrapNodeSK := "testdata/dht.priv"
	bootstrapNodeID := idForParty(t, bootstrapNodePK)
	nodePK := "testdata/dht1.pub"
	nodeSK := "testdata/dht1.priv"
	nodeID := idForParty(t, nodePK)
	bootstrapNodeEndpoint := "/ip4/127.0.0.1/tcp/1234"
	nodeEndpoint := "/ip4/127.0.0.1/tcp/1235"

	var bootstrapNode, node *P2PNode
	assert.NoError(t, Retry(3, 1*time.Second, func() error {
		var err error
		bootstrapNode, node, err = setupTwoNodes(t, bootstrapNodeID, bootstrapNodeEndpoint, nodeID, nodeEndpoint, &PrivateKeyFromFile{bootstrapNodeSK}, &PrivateKeyFromFile{nodeSK})
		return err
	}), "failed to setup two nodes")

	return bootstrapNode, node, bootstrapNodeID, nodeID
}

func TestP2PLayer(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodesFromFiles(t)

	P2PLayerTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func P2PLayerTestRound(t *testing.T, bootstrapNode, node *P2PNode, bootstrapNodeID, nodeID string) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		messages := bootstrapNode.incomingMessages

		err := bootstrapNode.sendTo(nodeID, &ViewPacket{Payload: []byte("msg1")})
		assert.NoError(t, err)

		err = bootstrapNode.sendTo(nodeID, &ViewPacket{Payload: []byte("msg2")})
		assert.NoError(t, err)

		msg := <-messages
		assert.NotNil(t, msg)
		assert.Equal(t, []byte("msg3"), msg.message.Payload)
	}()

	messages := node.incomingMessages
	msg := <-messages
	assert.NotNil(t, msg)
	assert.Equal(t, []byte("msg1"), msg.message.Payload)

	msg = <-messages
	assert.NotNil(t, msg)
	assert.Equal(t, []byte("msg2"), msg.message.Payload)

	err := node.sendTo(bootstrapNodeID, &ViewPacket{Payload: []byte("msg3")})
	assert.NoError(t, err)

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func TestSessions(t *testing.T) {
	bootstrapNode, node, _, nodeID := setupTwoNodesFromFiles(t)

	SessionsTestRound(t, bootstrapNode, node, nodeID)
}

func SessionsTestRound(t *testing.T, bootstrapNode, node *P2PNode, nodeID string) {
	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		session, err := bootstrapNode.NewSession("", "", "", []byte(nodeID))
		assert.NoError(t, err)
		assert.NotNil(t, session)

		err = session.Send([]byte("ciao"))
		assert.NoError(t, err)

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		err = session.Send([]byte("ciao on session"))
		assert.NoError(t, err)

		session.Close()
	}()

	masterSession, err := node.MasterSession()
	assert.NoError(t, err)
	assert.NotNil(t, masterSession)

	masterSessionMsgs := masterSession.Receive()
	msg := <-masterSessionMsgs
	assert.Equal(t, []byte("ciao"), msg.Payload)

	session, err := node.NewSessionWithID(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, session)

	session.Send([]byte("ciaoback"))

	sessionMsgs := session.Receive()
	msg = <-sessionMsgs
	assert.Equal(t, []byte("ciao on session"), msg.Payload)

	session.Close()

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func TestSessionsForMPC(t *testing.T) {
	bootstrapNode, node, bootstrapNodeID, nodeID := setupTwoNodesFromFiles(t)

	SessionsForMPCTestRound(t, bootstrapNode, node, bootstrapNodeID, nodeID)
}

func SessionsForMPCTestRound(t *testing.T, bootstrapNode, node *P2PNode, bootstrapNodeID, nodeID string) {
	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		session, err := bootstrapNode.NewSessionWithID("myawesomempcid", "", "", []byte(nodeID), nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, session)

		err = session.Send([]byte("ciao"))
		assert.NoError(t, err)

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		session.Close()
	}()

	session, err := node.NewSessionWithID("myawesomempcid", "", "", []byte(bootstrapNodeID), nil, nil)
	assert.NoError(t, err)
	assert.NotNil(t, session)

	sessionMsgs := session.Receive()
	msg := <-sessionMsgs
	assert.Equal(t, []byte("ciao"), msg.Payload)

	session.Send([]byte("ciaoback"))

	session.Close()

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func Retry(attempts int, sleep time.Duration, f func() error) (err error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(sleep)
			sleep *= 2
		}

		err = f()
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("no luck after %d attempts: last error: %v", attempts, err)
}
