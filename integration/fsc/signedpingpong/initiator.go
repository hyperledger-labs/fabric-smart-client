/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package signedpingpong

import (
	"bytes"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

const (
	aliceMsg = "hello bob, signed by alice"
	bobMsg   = "hello alice, signed by bob"
)

// AliceInitiator opens a session to Bob, sends a signed message, and verifies
// Bob's signed reply using the endpoint and sig services.
type AliceInitiator struct{}

func (a *AliceInitiator) Call(viewCtx view.Context) (any, error) {
	identityProvider, err := id.GetProvider(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting identity provider")
	}
	bob := identityProvider.Identity("bob")

	session, err := viewCtx.GetSession(viewCtx.Initiator(), bob)
	if err != nil {
		return nil, errors.Wrap(err, "failed opening session to bob")
	}

	sigSvc, err := sig.GetService(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "failed getting sig service")
	}
	endpointSvc := endpoint.GetService(viewCtx)

	// Resolve own identity from the session's LocalPKID and sign the payload.
	localID, err := endpointSvc.GetIdentity("", session.Info().LocalPKID)
	if err != nil {
		return nil, errors.Wrap(err, "alice: failed resolving local identity")
	}
	signer, err := sigSvc.GetSigner(localID)
	if err != nil {
		return nil, errors.Wrap(err, "alice: failed getting signer")
	}
	signature, err := signer.Sign([]byte(aliceMsg))
	if err != nil {
		return nil, errors.Wrap(err, "alice: failed signing")
	}

	raw, err := utils.MarshalSignedMessage([]byte(aliceMsg), signature)
	if err != nil {
		return nil, errors.Wrap(err, "alice: failed marshalling signed message")
	}
	if err := session.SendWithContext(viewCtx.Context(), raw); err != nil {
		return nil, errors.Wrap(err, "alice: failed sending")
	}

	// Wait for Bob's signed reply.
	select {
	case msg := <-session.Receive():
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		msgOut, err := utils.UnmarshalSignedMessage(msg.Payload)
		if err != nil {
			return nil, errors.Wrap(err, "alice: failed unmarshalling reply")
		}
		if string(msgOut.Payload) != bobMsg {
			return nil, errors.Errorf("alice: expected %q, got %q", bobMsg, string(msgOut.Payload))
		}

		// Resolve Bob's identity from the session's RemotePKID and verify.
		bobID, err := endpointSvc.GetIdentity("", session.Info().RemotePKID)
		if err != nil {
			return nil, errors.Wrap(err, "alice: failed resolving bob's identity")
		}
		verifier, err := sigSvc.GetVerifier(bobID)
		if err != nil {
			return nil, errors.Wrap(err, "alice: failed getting verifier for bob")
		}
		if err := verifier.Verify(msgOut.Payload, msgOut.Signature); err != nil {
			return nil, errors.Wrap(err, "alice: bob's signature verification failed")
		}
		// Negative check: tampered payload must not verify.
		tampered := append([]byte("x"), msgOut.Payload...)
		if err := verifier.Verify(tampered, msgOut.Signature); err == nil {
			return nil, errors.New("alice: tampered payload unexpectedly verified")
		}

		// Re-resolve the same PKID to confirm identity resolution is stable.
		bobID2, err := endpointSvc.GetIdentity("", session.Info().RemotePKID)
		if err != nil {
			return nil, errors.Wrap(err, "alice: second identity resolution failed")
		}
		if !bytes.Equal(bobID, bobID2) {
			return nil, errors.New("alice: identity resolution is not stable")
		}

	case <-time.After(1 * time.Minute):
		return nil, errors.New("alice: timeout waiting for bob's reply")
	}

	return "OK", nil
}
