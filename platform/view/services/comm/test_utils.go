/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
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
	numSessions := 100
	// numWorkers := 20
	// Set numWorkers to 20 to test true concurrency
	// bootstrapNode.SetNumWorkers(numWorkers)
	// node.SetNumWorkers(numWorkers)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	bootstrapNode.Start(ctx)
	node.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(numSessions)

	// Initiator
	for i := 0; i < numSessions; i++ {
		go func(sessionIndex int) {
			defer wg.Done()

			caller := fmt.Sprintf("caller-%d", sessionIndex)
			contextID := fmt.Sprintf("ctx-%d", sessionIndex)

			s, err := bootstrapNode.NewSession(caller, contextID, node.Address, []byte(node.ID))
			require.NoError(t, err)
			require.NotNil(t, s)

			// Send first message to trigger master session on responder
			messagesToSend := messages(s.Info().ID)
			err = s.Send(messagesToSend[0])
			require.NoError(t, err)

			// Wait for READY signal from responder to ensure dedicated session is active
			select {
			case msg := <-s.Receive():
				require.NotNil(t, msg)
				require.Equal(t, []byte("READY"), msg.Payload)
			case <-ctx.Done():
				t.Errorf("Sender [%s]: Timeout waiting for READY signal", s.Info().ID)
				return
			}

			// Send the rest of the messages
			for j := 1; j < len(messagesToSend); j++ {
				// logger.Infof("send message [%s] on session [%s]", string(messagesToSend[j]), s.Info().ID)
				err = s.Send(messagesToSend[j])
				require.NoError(t, err)
			}

			// Wait for FINAL ACK
			select {
			case msg := <-s.Receive():
				require.NotNil(t, msg)
				require.Equal(t, []byte("FINAL_ACK"), msg.Payload)
			case <-ctx.Done():
				t.Errorf("Sender [%s]: Timeout waiting for FINAL_ACK", s.Info().ID)
				return
			}

			s.Close()
		}(i)
	}

	masterSession, err := node.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, masterSession)

	var responderWg sync.WaitGroup

	// Responder
	go func() {
		for {
			// First message arrives at master session
			var firstMsg *view.Message
			select {
			case firstMsg = <-masterSession.Receive():
				if firstMsg == nil {
					return
				}
			case <-ctx.Done():
				return
			}

			responderWg.Add(1)
			go func(msg *view.Message) {
				defer responderWg.Done()

				messagesToReceive := messages(msg.SessionID)

				payload := msg.Payload
				require.Equal(t, messagesToReceive[0], payload)

				// Create dedicated session to receive the rest
				s, err := node.NewSessionWithID(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil)
				require.NoError(t, err)
				require.NotNil(t, s)

				// Signal READY to sender
				require.NoError(t, s.Send([]byte("READY")))

				sessionMsgs := s.Receive()
				for j := 1; j < len(messagesToReceive); j++ {
					select {
					case m := <-sessionMsgs:
						if m == nil {
							t.Errorf("Responder [%s]: Received nil message at index %d", msg.SessionID, j)
							return
						}
						// logger.Infof("received message [%s] on session [%s]", string(m.Payload), s.Info().ID)
						require.Equal(t, messagesToReceive[j], m.Payload)
					case <-ctx.Done():
						t.Errorf("Responder [%s]: Timeout waiting for message %d on session %s", msg.SessionID, j, msg.SessionID)
						return
					}
				}

				// Send FINAL ACK
				require.NoError(t, s.Send([]byte("FINAL_ACK")))

				s.Close()
			}(firstMsg)
		}
	}()

	wg.Wait()
	responderWg.Wait()

	bootstrapNode.Stop()
	node.Stop()
}

func messages(sessionIndex string) [][]byte {
	return [][]byte{
		[]byte(fmt.Sprintf("msg-0-short-%s", sessionIndex)),
		[]byte(fmt.Sprintf("msg-1-medium-%s-", sessionIndex) + strings.Repeat("a", 1024)),      // 1KB
		[]byte(fmt.Sprintf("msg-2-large-%s-", sessionIndex) + strings.Repeat("b", 100*1024)),   // 100KB
		[]byte(fmt.Sprintf("msg-3-xlarge-%s-", sessionIndex) + strings.Repeat("c", 1000*1024)), // 1MB
		[]byte(fmt.Sprintf("msg-4-end-%s", sessionIndex)),
	}
}
