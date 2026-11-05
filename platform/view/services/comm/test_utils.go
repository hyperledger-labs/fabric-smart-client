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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type HostNode struct {
	*P2PNode
	ID      host.PeerID
	Address host.PeerIPAddress
}

func P2PLayerTestRound(t *testing.T, initiator, responder *HostNode) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Go(func() {
		messages := initiator.incomingMessages

		info := host.StreamInfo{
			RemotePeerID:      responder.ID,
			RemotePeerAddress: responder.Address,
			ContextID:         "context",
			SessionID:         "session",
		}
		err := initiator.sendTo(t.Context(), info, &ViewPacket{Payload: []byte("msg1")}, nil)
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine

		err = initiator.sendTo(t.Context(), info, &ViewPacket{Payload: []byte("msg2")}, nil)
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine

		msg := <-messages
		assert.NotNil(t, msg)
		assert.Equal(t, []byte("msg3"), msg.message.Payload)
	})

	messages := responder.incomingMessages
	msg := <-messages
	require.NotNil(t, msg)
	require.Equal(t, []byte("msg1"), msg.message.Payload)

	msg = <-messages
	require.NotNil(t, msg)
	require.Equal(t, []byte("msg2"), msg.message.Payload)

	info := host.StreamInfo{
		RemotePeerID:      initiator.ID,
		RemotePeerAddress: initiator.Address,
		ContextID:         "context",
		SessionID:         "session",
	}
	err := responder.sendTo(t.Context(), info, &ViewPacket{Payload: []byte("msg3")}, nil)
	require.NoError(t, err)

	wg.Wait()

	initiator.Stop()
	responder.Stop()
}

func SessionsTestRound(t *testing.T, initiator, responder *HostNode) {
	t.Helper()
	ctx := t.Context()
	initiator.Start(ctx)
	responder.Start(ctx)

	var wg sync.WaitGroup

	wg.Go(func() {
		session, err := initiator.NewSession("", "", responder.Address, []byte(responder.ID))
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine
		assert.NotNil(t, session)

		err = session.Send(ctx, []byte("ciao"))
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		err = session.Send(ctx, []byte("ciao on session"))
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine

		session.Close()
	})

	masterSession, err := responder.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, masterSession)

	masterSessionMsgs := masterSession.Receive()
	msg := <-masterSessionMsgs
	require.Equal(t, []byte("ciao"), msg.Payload)

	session, err := responder.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, session)

	require.NoError(t, session.Send(ctx, []byte("ciaoback")))

	sessionMsgs := session.Receive()
	msg = <-sessionMsgs
	require.Equal(t, []byte("ciao on session"), msg.Payload)

	session.Close()

	wg.Wait()

	initiator.Stop()
	responder.Stop()
}

func SessionsForMPCTestRound(t *testing.T, initiator, responder *HostNode) {
	t.Helper()
	ctx := t.Context()
	initiator.Start(ctx)
	responder.Start(ctx)

	var wg sync.WaitGroup

	wg.Go(func() {
		session, err := initiator.NewResponderSession("myawesomempcid", "", responder.Address, []byte(responder.ID), nil, nil)
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine
		assert.NotNil(t, session)

		err = session.Send(ctx, []byte("ciao"))
		assert.NoError(t, err) //nolint:testifylint // runs inside wg.Go; require.FailNow is unsafe outside the test goroutine

		sessionMsgs := session.Receive()
		msg := <-sessionMsgs
		assert.Equal(t, []byte("ciaoback"), msg.Payload)

		session.Close()
	})

	session, err := responder.NewResponderSession("myawesomempcid", "", initiator.Address, []byte(initiator.ID), nil, nil)
	require.NoError(t, err)
	require.NotNil(t, session)

	sessionMsgs := session.Receive()
	msg := <-sessionMsgs
	require.Equal(t, []byte("ciao"), msg.Payload)

	require.NoError(t, session.Send(ctx, []byte("ciaoback")))

	session.Close()

	wg.Wait()

	initiator.Stop()
	responder.Stop()
}

func SessionsMultipleMessagesTestRound(t *testing.T, initiator, responder *HostNode) {
	t.Helper()
	numSessions := 100

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	initiator.Start(ctx)
	responder.Start(ctx)

	var wg sync.WaitGroup
	wg.Add(numSessions)

	// Initiator
	for i := range numSessions {
		go func(sessionIndex int) {
			defer wg.Done()

			caller := fmt.Sprintf("caller-%d", sessionIndex)
			contextID := fmt.Sprintf("ctx-%d", sessionIndex)

			s, err := initiator.NewSession(caller, contextID, responder.Address, []byte(responder.ID))
			assert.NoError(t, err)
			assert.NotNil(t, s)

			// Send first message to trigger master session on responder
			messagesToSend := messages(s.Info().ID)
			err = s.Send(ctx, messagesToSend[0])
			assert.NoError(t, err)

			// Wait for READY signal from responder to ensure dedicated session is active
			select {
			case msg := <-s.Receive():
				assert.NotNil(t, msg)
				assert.Equal(t, []byte("READY"), msg.Payload)
			case <-ctx.Done():
				t.Errorf("Sender [%s]: Timeout waiting for READY signal", s.Info().ID)
				return
			}

			// Send the rest of the messages
			for j := 1; j < len(messagesToSend); j++ {
				// logger.Infof("send message [%s] on session [%s]", string(messagesToSend[j]), s.Info().ID)
				err = s.Send(ctx, messagesToSend[j])
				assert.NoError(t, err)
			}

			// Wait for FINAL ACK
			select {
			case msg := <-s.Receive():
				assert.NotNil(t, msg)
				assert.Equal(t, []byte("FINAL_ACK"), msg.Payload)
			case <-ctx.Done():
				t.Errorf("Sender [%s]: Timeout waiting for FINAL_ACK", s.Info().ID)
				return
			}

			s.Close()
		}(i)
	}

	masterSession, err := responder.MasterSession()
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
				assert.Equal(t, messagesToReceive[0], payload)

				// Create dedicated session to receive the rest
				s, err := responder.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil)
				assert.NoError(t, err)
				assert.NotNil(t, s)

				// Signal READY to sender
				assert.NoError(t, s.Send(ctx, []byte("READY")))

				sessionMsgs := s.Receive()
				for j := 1; j < len(messagesToReceive); j++ {
					select {
					case m := <-sessionMsgs:
						if m == nil {
							t.Errorf("Responder [%s]: Received nil message at index %d", msg.SessionID, j)
							return
						}
						// logger.Infof("received message [%s] on session [%s]", string(m.Payload), s.Info().ID)
						assert.Equal(t, messagesToReceive[j], m.Payload)
					case <-ctx.Done():
						t.Errorf("Responder [%s]: Timeout waiting for message %d on session %s", msg.SessionID, j, msg.SessionID)
						return
					}
				}

				time.Sleep(time.Second)
				// Send FINAL ACK
				assert.NoError(t, s.Send(ctx, []byte("FINAL_ACK")))

				s.Close()
			}(firstMsg)
		}
	}()

	wg.Wait()
	responderWg.Wait()

	initiator.Stop()
	responder.Stop()
}

func messages(sessionIndex string) [][]byte {
	l := 1
	res := make([][]byte, 0, l*5)
	counter := 0
	for range l {
		res = append(res, fmt.Appendf(nil, "msg-%d-short-%s", counter, sessionIndex))
		counter++
		res = append(res, []byte(fmt.Sprintf("msg-%d-medium-%s-", counter, sessionIndex)+strings.Repeat("a", 1024)))
		counter++
		res = append(res, []byte(fmt.Sprintf("msg-%d-large-%s-", counter, sessionIndex)+strings.Repeat("b", 100*1024)))
		counter++
		res = append(res, []byte(fmt.Sprintf("msg-%d-xlarge-%s-", counter, sessionIndex)+strings.Repeat("c", 1000*1024)))
		counter++
		res = append(res, fmt.Appendf(nil, "msg-%d-end-%s", counter, sessionIndex))
		counter++
	}
	return res
}
