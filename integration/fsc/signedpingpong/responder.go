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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// BobResponder receives Alice's signed message, verifies it, and replies with
// its own signed message.
type BobResponder struct{}

func (b *BobResponder) Call(viewCtx view.Context) (any, error) {
	session := viewCtx.Session()

	sigSvc, err := sig.GetService(viewCtx)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed getting sig service")
	}
	endpointSvc := endpoint.GetService(viewCtx)

	// Receive Alice's signed message.
	var payload []byte
	select {
	case msg := <-session.Receive():
		if msg.Status == view.ERROR {
			return nil, errors.New(string(msg.Payload))
		}
		payload = msg.Payload
	case <-time.After(1 * time.Minute):
		return nil, errors.New("bob: timeout waiting for alice's message")
	}

	msg, err := utils.UnmarshalSignedMessage(payload)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed unmarshalling alice's message")
	}
	if string(msg.Payload) != aliceMsg {
		return nil, errors.Errorf("bob: expected %q, got %q", aliceMsg, string(msg.Payload))
	}

	// Resolve Alice's identity from the session's RemotePKID and verify.
	aliceID, err := endpointSvc.GetIdentity("", session.Info().RemotePKID)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed resolving alice's identity")
	}
	verifier, err := sigSvc.GetVerifier(aliceID)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed getting verifier for alice")
	}
	if err := verifier.Verify(msg.Payload, msg.Signature); err != nil {
		return nil, errors.Wrap(err, "bob: alice's signature verification failed")
	}
	// Negative check: tampered payload must not verify.
	tampered := append([]byte("x"), msg.Payload...)
	if err := verifier.Verify(tampered, msg.Signature); err == nil {
		return nil, errors.New("bob: tampered payload unexpectedly verified")
	}

	// Re-resolve the same PKID to confirm stability.
	aliceID2, err := endpointSvc.GetIdentity("", session.Info().RemotePKID)
	if err != nil {
		return nil, errors.Wrap(err, "bob: second identity resolution failed")
	}
	if !bytes.Equal(aliceID, aliceID2) {
		return nil, errors.New("bob: identity resolution is not stable")
	}

	// Sign and send the reply using the local identity from the session.
	localID, err := endpointSvc.GetIdentity("", session.Info().LocalPKID)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed resolving local identity")
	}
	signer, err := sigSvc.GetSigner(localID)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed getting signer")
	}
	signature, err := signer.Sign([]byte(bobMsg))
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed signing")
	}
	raw, err := utils.MarshalSignedMessage([]byte(bobMsg), signature)
	if err != nil {
		return nil, errors.Wrap(err, "bob: failed marshalling reply")
	}
	if err := session.SendWithContext(viewCtx.Context(), raw); err != nil {
		return nil, errors.Wrap(err, "bob: failed sending reply")
	}

	return "OK", nil
}
