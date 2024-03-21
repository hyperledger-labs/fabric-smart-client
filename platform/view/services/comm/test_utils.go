/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"testing"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/stretchr/testify/assert"
)

type HostNode struct {
	*P2PNode
	ID      host2.PeerID
	Address host2.PeerIPAddress
}

func P2PLayerTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		messages := bootstrapNode.incomingMessages

		info := host2.StreamInfo{
			RemotePeerID:      node.ID,
			RemotePeerAddress: node.Address,
			ContextID:         "context",
			SessionID:         "session",
		}
		err := bootstrapNode.sendTo(info, &ViewPacket{Payload: []byte("msg1")})
		assert.NoError(t, err)

		err = bootstrapNode.sendTo(info, &ViewPacket{Payload: []byte("msg2")})
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

	info := host2.StreamInfo{
		RemotePeerID:      bootstrapNode.ID,
		RemotePeerAddress: bootstrapNode.Address,
		ContextID:         "context",
		SessionID:         "session",
	}
	err := node.sendTo(info, &ViewPacket{Payload: []byte("msg3")})
	assert.NoError(t, err)

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func SessionsTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		session, err := bootstrapNode.NewSession("", "", node.Address, []byte(node.ID))
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

func SessionsForMPCTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		session, err := bootstrapNode.NewSessionWithID("myawesomempcid", "", bootstrapNode.Address, []byte(node.ID), nil, nil)
		assert.NoError(t, err)
		assert.NotNil(t, session)

		err = session.Send([]byte("ciao"))
		assert.NoError(t, err)

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		session.Close()
	}()

	session, err := node.NewSessionWithID("myawesomempcid", "", bootstrapNode.Address, []byte(bootstrapNode.ID), nil, nil)
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
