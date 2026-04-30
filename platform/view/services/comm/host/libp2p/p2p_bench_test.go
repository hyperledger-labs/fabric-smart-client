/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func BenchmarkSessionRoundTrip(b *testing.B) {
	benchmarkSessionRoundTrip(b, setupTwoNodes)
}

func benchmarkSessionRoundTrip(b *testing.B, setup func(testing.TB) (*comm.HostNode, *comm.HostNode)) {
	b.Helper()

	b.Run("new-session", func(b *testing.B) {
		aliceNode, bobNode := setup(b)
		ctx, cancel := context.WithCancel(b.Context())
		defer cancel()
		aliceNode.Start(ctx)
		bobNode.Start(ctx)

		masterSession, err := bobNode.MasterSession()
		require.NoError(b, err)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-ctx.Done():
					return
				case msg := <-masterSession.Receive():
					if msg == nil {
						continue
					}
					responder, err := bobNode.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, view.Identity(msg.FromPKID), nil)
					if err != nil {
						return
					}
					_ = responder.Send(append([]byte("pong:"), msg.Payload...))
					responder.Close()
				}
			}
		}()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			session, err := aliceNode.NewSession("caller", "ctx", bobNode.Address, []byte(bobNode.ID))
			require.NoError(b, err)
			payload := []byte(fmt.Sprintf("ping-%d", i))
			require.NoError(b, session.Send(payload))
			reply := <-session.Receive()
			require.NotNil(b, reply)
			require.Equal(b, append([]byte("pong:"), payload...), reply.Payload)
			session.Close()
		}
		b.StopTimer()
		cancel()
		aliceNode.Stop()
		bobNode.Stop()
		<-done
	})

	b.Run("reused-session", func(b *testing.B) {
		aliceNode, bobNode := setup(b)
		ctx, cancel := context.WithCancel(b.Context())
		defer cancel()
		aliceNode.Start(ctx)
		bobNode.Start(ctx)

		session, err := aliceNode.NewSession("caller", "ctx", bobNode.Address, []byte(bobNode.ID))
		require.NoError(b, err)
		defer session.Close()

		masterSession, err := bobNode.MasterSession()
		require.NoError(b, err)

		first := []byte("warmup")
		require.NoError(b, session.Send(first))
		msg := <-masterSession.Receive()
		require.NotNil(b, msg)

		responder, err := bobNode.NewResponderSession(msg.SessionID, msg.ContextID, "", msg.FromPKID, view.Identity(msg.FromPKID), nil)
		require.NoError(b, err)
		defer responder.Close()
		require.NoError(b, responder.Send(append([]byte("pong:"), msg.Payload...)))
		reply := <-session.Receive()
		require.NotNil(b, reply)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-ctx.Done():
					return
				case req := <-responder.Receive():
					if req == nil {
						continue
					}
					_ = responder.Send(append([]byte("pong:"), req.Payload...))
				}
			}
		}()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			payload := []byte(fmt.Sprintf("ping-%d", i))
			require.NoError(b, session.Send(payload))
			reply := <-session.Receive()
			require.NotNil(b, reply)
			require.Equal(b, append([]byte("pong:"), payload...), reply.Payload)
		}
		b.StopTimer()
		cancel()
		aliceNode.Stop()
		bobNode.Stop()
		<-done
	})
}
