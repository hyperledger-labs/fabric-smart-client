/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// SessionsNodesTestRound opens numSessionsPerNode sessions from initiator to
// each of nodes, running the per-node sequences in parallel, and fails unless
// every session completes its full message exchange.
func SessionsNodesTestRound(t *testing.T, initiator *HostNode, nodes []*HostNode, numSessionsPerNode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)

	initiator.Start(ctx)
	waits := make([]func(), 0, len(nodes))
	defer func() {
		// Stop the responders and let their in-flight sessions finish before
		// the nodes go away; this also runs when the count check fails.
		cancel()
		for _, wait := range waits {
			wait()
		}
		initiator.Stop()
		for _, n := range nodes {
			n.Stop()
		}
	}()
	for _, n := range nodes {
		waits = append(waits, RunResponder(t, ctx, n))
		n.Start(ctx)
	}

	var wg sync.WaitGroup
	var completed atomic.Int64
	for _, node := range nodes {
		wg.Go(func() {
			completed.Add(int64(runInitiator(t, ctx, initiator, node, numSessionsPerNode)))
		})
	}
	wg.Wait()
	require.Equal(t, len(nodes)*numSessionsPerNode, int(completed.Load()), "sessions completed")
}

// runInitiator opens numSessionsPerNode sessions to targetNode one after the
// other and returns how many of them completed the exchange.
func runInitiator(t *testing.T, ctx context.Context, bootstrapNode, targetNode *HostNode, numSessionsPerNode int) int {
	t.Helper()
	for i := range numSessionsPerNode {
		select {
		case <-ctx.Done():
			return i
		default:
		}

		caller := fmt.Sprintf("caller-%s-%d", targetNode.ID, i)
		contextID := fmt.Sprintf("ctx-%s-%d", targetNode.ID, i)

		s, err := bootstrapNode.NewSession(caller, contextID, targetNode.Address, []byte(targetNode.ID))
		if err != nil {
			t.Errorf("Sender: failed to create session to %s: %v", targetNode.ID, err)
			return i
		}
		messagesToSend := messages(s.Info().ID)
		err = s.Send(ctx, messagesToSend[0])
		if err != nil {
			t.Errorf("Sender [%s]: failed to send first message: %v", s.Info().ID, err)
			return i
		}

		select {
		case msg := <-s.Receive():
			if msg == nil || string(msg.Payload) != "READY" {
				t.Errorf("Sender [%s]: expected READY, got %v", s.Info().ID, msg)
				return i
			}
		case <-ctx.Done():
			t.Errorf("Sender [%s]: Timeout waiting for READY signal", s.Info().ID)
			return i
		}

		for j := 1; j < len(messagesToSend); j++ {
			if err := s.Send(ctx, messagesToSend[j]); err != nil {
				t.Errorf("Sender [%s]: failed to send message %d: %v", s.Info().ID, j, err)
				return i
			}
		}

		select {
		case msg := <-s.Receive():
			if msg == nil || string(msg.Payload) != "FINAL_ACK" {
				t.Errorf("Sender [%s]: expected FINAL_ACK, got %v", s.Info().ID, msg)
				return i
			}
		case <-ctx.Done():
			t.Errorf("Sender [%s]: Timeout waiting for FINAL_ACK", s.Info().ID)
			return i
		}

		s.Close()
	}
	return numSessionsPerNode
}

// RunResponder answers every session opened on node's master session until ctx
// is done. It returns without blocking; the returned wait blocks until the
// accept loop has exited and every session it accepted has been answered.
func RunResponder(t *testing.T, ctx context.Context, node *HostNode) (wait func()) {
	t.Helper()
	masterSession, err := node.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, masterSession)

	// The accept loop is part of wg, so wg.Add for a new session never races
	// with wg.Wait reaching zero.
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case msg := <-masterSession.Receive():
				if msg == nil {
					return
				}
				wg.Go(func() { respondToSession(t, ctx, node, msg) })
			case <-ctx.Done():
				return
			}
		}
	})
	return wg.Wait
}

func respondToSession(t *testing.T, ctx context.Context, node *HostNode, msg *view.Message) {
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

	if err := s.Send(ctx, []byte("READY")); err != nil {
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

	if err := s.Send(ctx, []byte("FINAL_ACK")); err != nil {
		t.Errorf("Responder [%s]: failed to send FINAL_ACK: %v", msg.SessionID, err)
	}
}
