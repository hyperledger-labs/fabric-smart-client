/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		err := bootstrapNode.sendTo(context.Background(), info, &ViewPacket{Payload: []byte("msg1")}, nil)
		assert.NoError(t, err)

		err = bootstrapNode.sendTo(context.Background(), info, &ViewPacket{Payload: []byte("msg2")}, nil)
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
	err := node.sendTo(context.Background(), info, &ViewPacket{Payload: []byte("msg3")}, nil)
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

	assert.NoError(t, session.Send([]byte("ciaoback")))

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

	assert.NoError(t, session.Send([]byte("ciaoback")))

	session.Close()

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func SessionsMultipleMessagesTestRound(t *testing.T, bootstrapNode *HostNode, node *HostNode) {
	// Set numWorkers to 1 to ensure ordered delivery through the dispatcher for this test
	bootstrapNode.SetNumWorkers(1)
	node.SetNumWorkers(1)

	ctx := context.Background()
	bootstrapNode.Start(ctx)
	node.Start(ctx)

	messagesToSend := [][]byte{
		[]byte("msg-0-short"),
		[]byte("msg-1-medium-" + strings.Repeat("a", 1024)),      // 1KB
		[]byte("msg-2-large-" + strings.Repeat("b", 100*1024)),   // 100KB
		[]byte("msg-3-xlarge-" + strings.Repeat("c", 1024*1024)), // 1MB
		[]byte("msg-4-end"),
	}

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		s, err := bootstrapNode.NewSession("caller", "ctx", node.Address, []byte(node.ID))
		require.NoError(t, err)
		require.NotNil(t, s)

		// Send first message to trigger master session on responder
		err = s.Send(messagesToSend[0])
		require.NoError(t, err)

		// Wait for READY signal from responder to ensure dedicated session is active
		select {
		case msg := <-s.Receive():
			require.NotNil(t, msg)
			require.Equal(t, []byte("READY"), msg.Payload)
		case <-time.After(10 * time.Second):
			t.Errorf("Sender: Timeout waiting for READY signal")
			return
		}

		// Send the rest of the messages
		for i := 1; i < len(messagesToSend); i++ {
			err = s.Send(messagesToSend[i])
			require.NoError(t, err)
		}

		// Wait for FINAL ACK
		select {
		case msg := <-s.Receive():
			require.NotNil(t, msg)
			require.Equal(t, []byte("FINAL_ACK"), msg.Payload)
		case <-time.After(10 * time.Second):
			t.Errorf("Sender: Timeout waiting for FINAL_ACK")
			return
		}

		s.Close()
	}()

	masterSession, err := node.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, masterSession)

	// First message arrives at master session
	var firstMsg *view.Message
	select {
	case firstMsg = <-masterSession.Receive():
		require.NotNil(t, firstMsg)
		require.Equal(t, messagesToSend[0], firstMsg.Payload)
	case <-time.After(10 * time.Second):
		t.Fatal("Responder: Timeout waiting for first message on master session")
	}

	// Create dedicated session to receive the rest
	s, err := node.NewSessionWithID(firstMsg.SessionID, firstMsg.ContextID, "", firstMsg.FromPKID, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, s)

	// Signal READY to sender
	require.NoError(t, s.Send([]byte("READY")))

	sessionMsgs := s.Receive()
	// Receive the rest (index 1 to end)
	for i := 1; i < len(messagesToSend); i++ {
		select {
		case msg := <-sessionMsgs:
			require.NotNil(t, msg, "Message %d is nil", i)
			if !bytes.Equal(messagesToSend[i], msg.Payload) {
				t.Errorf("Message %d content mismatch. Expected len %d, got len %d", i, len(messagesToSend[i]), len(msg.Payload))
				return
			}
		case <-time.After(10 * time.Second):
			t.Errorf("Responder: Timeout waiting for message %d", i)
			return
		}
	}

	// Send FINAL ACK
	require.NoError(t, s.Send([]byte("FINAL_ACK")))

	s.Close()

	wg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}
