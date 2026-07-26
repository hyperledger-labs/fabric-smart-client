/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	mspx509 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/x509"
)

// SignedGossipMessage contains a GossipMessage and the Envelope from which it
// came from
type SignedGossipMessage struct {
	*gossip.Envelope
	*gossip.GossipMessage
}

// Sign signs a GossipMessage with given Signer.
// Returns an Envelope on success, panics on failure.
func (m *SignedGossipMessage) Sign(signer Signer) (*gossip.Envelope, error) {
	// If we have a secretEnvelope, don't override it.
	// Back it up, and restore it later
	var secretEnvelope *gossip.SecretEnvelope
	if m.Envelope != nil {
		secretEnvelope = m.SecretEnvelope
	}
	m.Envelope = nil
	if m.GossipMessage == nil {
		return nil, errors.New("proto: Marshal called with nil")
	}
	payload, err := proto.Marshal(m.GossipMessage)
	if err != nil {
		return nil, err
	}
	sig, err := signer(payload)
	if err != nil {
		return nil, err
	}

	e := &gossip.Envelope{
		Payload:        payload,
		Signature:      sig,
		SecretEnvelope: secretEnvelope,
	}
	m.Envelope = e
	return e, nil
}

// EnvelopeToGossipMessage un-marshals a given envelope and creates a
// SignedGossipMessage out of it.
// Returns an error if un-marshaling fails.
func EnvelopeToGossipMessage(e *gossip.Envelope) (*SignedGossipMessage, error) {
	if e == nil {
		return nil, errors.New("nil envelope")
	}
	msg := &gossip.GossipMessage{}
	err := proto.Unmarshal(e.Payload, msg)
	if err != nil {
		return nil, errors.Wrap(err, "failed unmarshalling GossipMessage from envelope")
	}
	return &SignedGossipMessage{
		GossipMessage: msg,
		Envelope:      e,
	}, nil
}

// verifyEnvelopeSignature checks that e.Signature is a valid signature over
// e.Payload produced by the private key corresponding to the given
// serialized identity. This binds a relayed gossip envelope to the specific
// identity it is shipped alongside in the same discovery response, so an
// envelope whose signature does not verify against that identity's key -
// including a forged/garbage signature, or one produced by a different key -
// is rejected instead of being trusted unconditionally.
//
// allowEmptySignature must be true only for AliveMessage/MembershipInfo
// envelopes. A real Fabric peer signs its own self-referential AliveMessage
// with protoext.NoopSign (gossip/discovery's Self()), which produces a nil
// Signature by design, and the Discovery service exposes that self-entry
// as-is (discovery/support/gossip's Peers(), via SelfMembershipInfo()). This
// is inherent to how real Fabric peers answer Discovery queries - even
// upstream Fabric's own discovery client performs no verification at all on
// these envelopes - so rejecting a nil signature here would fail every
// Discovery query in which the responding peer is itself among the reported
// peers/endorsers, which is the common case in small networks. StateInfo
// self-entries do not share this exemption: real peers always produce a
// genuine signature for them, even for themselves (see
// gossipChannel.setupSignedStateInfoMessage, which calls a real signer, not
// NoopSign), so callers must pass false for StateInfo envelopes to keep that
// verification strict.
func verifyEnvelopeSignature(e *gossip.Envelope, identity []byte, allowEmptySignature bool) error {
	if e == nil {
		return errors.New("nil envelope")
	}
	if allowEmptySignature && len(e.Signature) == 0 {
		return nil
	}
	_, verifier, err := mspx509.NewIdentityFromBytes(identity)
	if err != nil {
		return errors.Wrap(err, "failed deriving verifier from peer identity")
	}
	if err := verifier.Verify(e.Payload, e.Signature); err != nil {
		return errors.Wrap(err, "signature does not verify against claimed identity")
	}
	return nil
}
