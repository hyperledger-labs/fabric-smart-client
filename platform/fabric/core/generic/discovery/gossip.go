/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package discovery

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger/fabric-protos-go-apiv2/gossip"
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
		secretEnvelope = m.Envelope.SecretEnvelope
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
