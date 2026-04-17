/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// SessionsNodesTestRound tests multiple parallel sessions between an initiator node and multiple other nodes
func SessionsNodesTestRound(t *testing.T, initiator *HostNode, nodes []*HostNode, numSessionsPerNode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	initiator.Start(ctx)
	for _, n := range nodes {
		RunResponder(t, ctx, n)
		n.Start(ctx)
	}

	var wg sync.WaitGroup
	wg.Add(len(nodes))

	// Initiator sessions
	for _, node := range nodes {
		go func(targetNode *HostNode) {
			defer wg.Done()
			runInitiator(t, ctx, initiator, targetNode, numSessionsPerNode)
		}(node)
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	initiator.Stop()
	for _, n := range nodes {
		n.Stop()
	}
}

func runInitiator(t *testing.T, ctx context.Context, bootstrapNode, targetNode *HostNode, numSessionsPerNode int) {
	t.Helper()
	for i := 0; i < numSessionsPerNode; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		caller := fmt.Sprintf("caller-%s-%d", targetNode.ID, i)
		contextID := fmt.Sprintf("ctx-%s-%d", targetNode.ID, i)

		s, err := bootstrapNode.NewSession(caller, contextID, targetNode.Address, []byte(targetNode.ID))
		if err != nil {
			t.Errorf("Sender: failed to create session to %s: %v", targetNode.ID, err)
			return
		}
		assert.NotNil(t, s)

		messagesToSend := messages(s.Info().ID)
		err = s.Send(messagesToSend[0])
		if err != nil {
			t.Errorf("Sender [%s]: failed to send first message: %v", s.Info().ID, err)
			return
		}

		select {
		case msg := <-s.Receive():
			if msg == nil || string(msg.Payload) != "READY" {
				t.Errorf("Sender [%s]: expected READY, got %v", s.Info().ID, msg)
				return
			}
		case <-ctx.Done():
			t.Errorf("Sender [%s]: Timeout waiting for READY signal", s.Info().ID)
			return
		}

		for j := 1; j < len(messagesToSend); j++ {
			if err := s.Send(messagesToSend[j]); err != nil {
				t.Errorf("Sender [%s]: failed to send message %d: %v", s.Info().ID, j, err)
				return
			}
		}

		select {
		case msg := <-s.Receive():
			if msg == nil || string(msg.Payload) != "FINAL_ACK" {
				t.Errorf("Sender [%s]: expected FINAL_ACK, got %v", s.Info().ID, msg)
				return
			}
		case <-ctx.Done():
			t.Errorf("Sender [%s]: Timeout waiting for FINAL_ACK", s.Info().ID)
			return
		}

		s.Close()
	}
}

func RunResponder(t *testing.T, ctx context.Context, node *HostNode) {
	t.Helper()
	masterSession, err := node.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, masterSession)

	var wg sync.WaitGroup
	go func() {
		for {
			var firstMsg *view.Message
			select {
			case firstMsg = <-masterSession.Receive():
				if firstMsg == nil {
					return
				}
			case <-ctx.Done():
				return
			}

			wg.Add(1)
			go func(msg *view.Message) {
				defer wg.Done()
				runResponder(t, ctx, node, msg)
			}(firstMsg)
		}
	}()
	<-ctx.Done()
	wg.Wait()
}

func runResponder(t *testing.T, ctx context.Context, node *HostNode, msg *view.Message) {
	t.Helper()
	messagesToReceive := messages(msg.SessionID)
	if string(messagesToReceive[0]) != string(msg.Payload) {
		t.Errorf("Responder [%s]: expected first message %s, got %s", msg.SessionID, string(messagesToReceive[0]), string(msg.Payload))
		return
	}

	s, err := node.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, nil, nil)
	if err != nil {
		t.Errorf("Responder [%s]: failed to create session: %v", msg.SessionID, err)
		return
	}
	defer s.Close()

	if err := s.Send([]byte("READY")); err != nil {
		t.Errorf("Responder [%s]: failed to send READY: %v", msg.SessionID, err)
		return
	}

	sessionMsgs := s.Receive()
	for j := 1; j < len(messagesToReceive); j++ {
		select {
		case m := <-sessionMsgs:
			if m == nil || string(messagesToReceive[j]) != string(m.Payload) {
				t.Errorf("Responder [%s]: message %d mismatch", msg.SessionID, j)
				return
			}
		case <-ctx.Done():
			return
		}
	}

	time.Sleep(100 * time.Millisecond)
	if err := s.Send([]byte("FINAL_ACK")); err != nil {
		t.Errorf("Responder [%s]: failed to send FINAL_ACK: %v", msg.SessionID, err)
	}
}
