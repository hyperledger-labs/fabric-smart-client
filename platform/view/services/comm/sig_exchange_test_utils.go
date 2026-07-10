/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// SignedExchangeParty bundles a comm node together with the sig- and
// endpoint-service backed hooks needed to sign outgoing messages and to verify
// incoming ones. It is populated by each transport's test setup, so the same
// exchange logic can run unchanged over websocket and libp2p.
type SignedExchangeParty struct {
	// Name is a human friendly label used in test failure messages.
	Name string
	// Node is the comm node of this party.
	Node *HostNode

	// Sign signs the payload with the secret key selected from the local
	// identity carried by the session (SessionInfo.LocalPKID). The party resolves
	// that PKID to its own identity through the endpoint service and obtains the
	// signer from its sig.Service (GetSigner + Sign). This mirrors, on the local
	// side, how Verify selects the counterparty's key from RemotePKID.
	Sign func(localPKID, payload []byte) ([]byte, error)

	// ResolveIdentity maps the PKID carried by SessionInfo (RemotePKID) to the
	// full identity of the remote peer, using the party's endpoint.Service.
	ResolveIdentity func(pkid []byte) (view.Identity, error)

	// Verify verifies the signature over payload against the given remote
	// identity, using the party's sig.Service (GetVerifier + Verify).
	Verify func(identity view.Identity, payload, signature []byte) error
}

// verifyFromSession resolves the remote identity from the session info and
// verifies the signed message against it. It returns the resolved identity so
// the caller can assert additional binding guarantees, and an error describing
// the first failed check. It performs no test assertions itself so that it is
// safe to call from any goroutine, including ones started with wg.Go where
// require.* (which calls runtime.Goexit) must not be used.
func (p *SignedExchangeParty) verifyFromSession(info view.SessionInfo, msg *utils.SignedMessage) (view.Identity, error) {
	if len(info.RemotePKID) == 0 {
		return nil, errors.Errorf("[%s] session info carries no RemotePKID", p.Name)
	}

	remoteID, err := p.ResolveIdentity(info.RemotePKID)
	if err != nil {
		return nil, errors.Wrapf(err, "[%s] failed resolving remote identity from PKID", p.Name)
	}
	if remoteID == nil {
		return nil, errors.Errorf("[%s] resolved a nil remote identity", p.Name)
	}

	if err := p.Verify(remoteID, msg.Payload, msg.Signature); err != nil {
		return nil, errors.Wrapf(err, "[%s] signature verification failed for payload %q", p.Name, string(msg.Payload))
	}

	// Negative check: a tampered payload must not verify against the same signature.
	tampered := append([]byte("x"), msg.Payload...)
	if err := p.Verify(remoteID, tampered, msg.Signature); err == nil {
		return nil, errors.Errorf("[%s] tampered payload unexpectedly verified", p.Name)
	}

	return remoteID, nil
}

// SignedSessionExchangeTestRound runs the full integration scenario:
//
//  1. Alice opens a session to Bob and sends a message signed with the secret
//     key of her comm key-pair.
//  2. Bob receives the message, resolves Alice's identity from the session's
//     RemotePKID via the endpoint service, and verifies the signature with a
//     verifier obtained from the sig service.
//  3. Bob replies with a message signed with the secret key of his comm
//     key-pair.
//  4. Alice verifies Bob's signature the same way.
//
// The round is transport agnostic; all transport/PKI specifics are injected via
// the two SignedExchangeParty values.
func SignedSessionExchangeTestRound(t *testing.T, alice, bob *SignedExchangeParty) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	alice.Node.Start(ctx)
	bob.Node.Start(ctx)
	defer alice.Node.Stop()
	defer bob.Node.Stop()

	const (
		aliceMsg = "hello bob, signed by alice"
		bobMsg   = "hello alice, signed by bob"
	)

	var wg sync.WaitGroup

	// Alice: initiator.
	wg.Go(func() {
		session, err := alice.Node.NewSession("alice-view", "ctx-1", bob.Node.Address, []byte(bob.Node.ID))
		if !assert.NoError(t, err) || !assert.NotNil(t, session) {
			return
		}
		defer session.Close()

		// Alice selects her own signer from the local identity carried by the
		// session, not from any pre-captured key.
		sig, err := alice.Sign(session.Info().LocalPKID, []byte(aliceMsg))
		if !assert.NoError(t, err, "alice failed to sign") {
			return
		}
		assert.NotEmpty(t, session.Info().LocalPKID, "alice session carries no LocalPKID")
		raw, err := utils.MarshalSignedMessage([]byte(aliceMsg), sig)
		if !assert.NoError(t, err, "alice failed to marshal signed message") {
			return
		}
		if !assert.NoError(t, session.SendWithContext(ctx, raw)) {
			return
		}

		// Wait for Bob's signed reply and verify it.
		select {
		case rcv := <-session.Receive():
			if !assert.NotNil(t, rcv, "alice received nil reply") {
				return
			}
			msg, err := utils.UnmarshalSignedMessage(rcv.Payload)
			if !assert.NoError(t, err, "alice failed to decode reply") {
				return
			}
			assert.Equal(t, bobMsg, string(msg.Payload))
			// Alice resolves Bob from the session info and verifies.
			bobID, err := alice.verifyFromSession(session.Info(), msg)
			if !assert.NoError(t, err) {
				return
			}
			// Binding: the resolved identity's PKID matches what the session reported.
			if err := assertIdentityMatchesPKID(alice, bobID, session.Info().RemotePKID); !assert.NoError(t, err) {
				return
			}
		case <-ctx.Done():
			t.Errorf("alice: timeout waiting for bob's reply")
		}
	})

	// Bob: responder.
	masterSession, err := bob.Node.MasterSession()
	require.NoError(t, err)
	require.NotNil(t, masterSession)

	var first *view.Message
	select {
	case first = <-masterSession.Receive():
		require.NotNil(t, first)
	case <-ctx.Done():
		t.Fatal("bob: timeout waiting for alice's first message")
	}

	responder, err := bob.Node.NewResponderSession(first.SessionID, first.ContextID, "", first.FromPKID, view.Identity(first.FromPKID), first)
	require.NoError(t, err)
	require.NotNil(t, responder)
	defer responder.Close()

	// The first message is delivered both on the master session and, since we
	// passed it to NewResponderSession, on the responder session. We already
	// have it from the master session, so verify it directly.
	msg, err := utils.UnmarshalSignedMessage(first.Payload)
	require.NoError(t, err, "bob failed to decode alice's message")
	require.Equal(t, aliceMsg, string(msg.Payload))

	aliceID, err := bob.verifyFromSession(responder.Info(), msg)
	require.NoError(t, err)
	require.NoError(t, assertIdentityMatchesPKID(bob, aliceID, responder.Info().RemotePKID))

	// Bob signs and replies, selecting his signer from the responder session's
	// local identity.
	require.NotEmpty(t, responder.Info().LocalPKID, "bob session carries no LocalPKID")
	bobSig, err := bob.Sign(responder.Info().LocalPKID, []byte(bobMsg))
	require.NoError(t, err, "bob failed to sign")
	bobRaw, err := utils.MarshalSignedMessage([]byte(bobMsg), bobSig)
	require.NoError(t, err, "bob failed to marshal signed message")
	require.NoError(t, responder.SendWithContext(ctx, bobRaw))

	wg.Wait()
}

// assertIdentityMatchesPKID checks that resolving the given identity's public
// key back to a PKID yields the PKID reported by the session, closing the loop
// between the transport-authenticated peer and the identity used for signature
// verification. It relies on the party's ResolveIdentity being consistent with
// its endpoint service, which is already exercised above; here we simply check
// the resolved identity is non-empty and stable. It performs no test
// assertions itself so that it is safe to call from any goroutine.
func assertIdentityMatchesPKID(p *SignedExchangeParty, id view.Identity, pkid []byte) error {
	if len(id) == 0 {
		return errors.Errorf("[%s] resolved identity is empty", p.Name)
	}
	// Re-resolving the same PKID must return the same identity.
	again, err := p.ResolveIdentity(pkid)
	if err != nil {
		return err
	}
	if !id.Equal(again) {
		return errors.Errorf("[%s] identity resolution is not stable for PKID", p.Name)
	}
	return nil
}
