/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"encoding/binary"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// ErrRecipientProof is wrapped by any error returned from VerifyRecipientProof,
// whether the proof is missing, its context is incomplete, or it fails
// verification.
var ErrRecipientProof = errors.New("recipient proof")

// ProofTag names the view and direction a recipient proof is for. A signature
// over a payload with one tag is not valid for any other view or direction,
// so a proof obtained for one exchange can't be replayed into another.
type ProofTag string

const (
	// TagRequestReply tags the reply to RequestRecipientIdentityView.
	TagRequestReply ProofTag = "fsc/recipient/v1/request-reply"
	// TagExchangeRequest tags the initiator's message in the recipient exchange.
	TagExchangeRequest ProofTag = "fsc/recipient/v1/exchange-request"
	// TagExchangeReply tags the responder's message in the recipient exchange.
	TagExchangeReply ProofTag = "fsc/recipient/v1/exchange-reply"
)

// ProofContext identifies the claim a recipient proof attests to: that the
// holder of Identity's key claims it for SignerNode toward VerifierNode, in
// the session named by SessionID and ContextID, in the message direction
// named by Tag.
type ProofContext struct {
	Tag          ProofTag
	SessionID    string
	ContextID    string
	SignerNode   view.Identity
	VerifierNode view.Identity
	Identity     view.Identity
}

// Payload returns the byte sequence a recipient proof signs: 0x00 followed by
// Tag, SessionID, ContextID, SignerNode, VerifierNode, and Identity, each
// prefixed with its length as a uvarint. The leading 0x00 makes the payload
// unparsable as a protobuf message, so it can't be confused with a signature
// over a proposal or envelope from the same key. Length-prefixing every field
// keeps field boundaries unambiguous: without it, ("ab","c") and ("a","bc") in
// adjacent fields would concatenate to the same bytes.
//
// SignerNode and VerifierNode are encoded in the form id.CanonicalIdentity
// returns. A node's own identity is the raw bytes of its certificate file,
// while a peer's view of it is the re-encoded certificate from its resolver
// configuration; canonicalizing both makes signer and verifier sign and check
// the same bytes however each loaded the certificate.
func (p ProofContext) Payload() []byte {
	out := []byte{0x00}
	out = appendField(out, []byte(p.Tag))
	out = appendField(out, []byte(p.SessionID))
	out = appendField(out, []byte(p.ContextID))
	out = appendField(out, id.CanonicalIdentity(p.SignerNode))
	out = appendField(out, id.CanonicalIdentity(p.VerifierNode))
	out = appendField(out, p.Identity)
	return out
}

func appendField(out, field []byte) []byte {
	out = binary.AppendUvarint(out, uint64(len(field)))
	return append(out, field...)
}

// SignRecipientProof signs p.Payload() with signer, the signing identity for
// p.Identity. The resulting signature shows that whoever holds p.Identity's
// key claims it for p.SignerNode toward p.VerifierNode, in the session and
// message direction named by p.
//
// When signer is an idemix signing identity, the resulting signature is
// verifiable by any third party that holds the identity's public material,
// not only by p.VerifierNode. That links the idemix pseudonym to p.SignerNode
// for anyone who observes the proof, which defeats idemix's unlinkability.
// Callers sensitive to that anonymity loss must weigh it before using this
// helper; FSC's own recipient views use it with idemix identities.
func SignRecipientProof(signer driver.Signer, p ProofContext) ([]byte, error) {
	sig, err := signer.Sign(p.Payload())
	if err != nil {
		return nil, errors.Wrap(err, "failed signing recipient proof")
	}
	return sig, nil
}

// VerifyRecipientProof verifies that sig is a signature over p.Payload() by
// the verifier for p.Identity. It returns an error wrapping ErrRecipientProof
// if sig is nil or empty, if p.SignerNode, p.VerifierNode or p.SessionID is
// empty, or if verification fails for any other reason. An empty node or
// session field would let a proof made for no particular counterparty or
// session verify, so p is refused before any signature check.
//
// p must be built from the verifier's own view of the exchange, never from
// fields carried in the message being verified: p.VerifierNode is this
// node's own FSC identity, and p.SignerNode is the transport-authenticated
// counterparty — session.Info().Caller on the responder, or the resolver
// identity of the party dialed on the initiator. p.SessionID and
// p.ContextID come from the local session.
func VerifyRecipientProof(verifier driver.Verifier, p ProofContext, sig []byte) error {
	if len(sig) == 0 {
		return errors.Errorf("%w: missing", ErrRecipientProof)
	}
	if !p.complete() {
		return errors.Errorf("%w: incomplete context", ErrRecipientProof)
	}
	if err := verifier.Verify(p.Payload(), sig); err != nil {
		return errors.Errorf("%w: %s", ErrRecipientProof, err.Error())
	}
	return nil
}

// complete reports whether p names both nodes and the session, the fields a
// proof needs to be bound to one counterparty and one session.
func (p ProofContext) complete() bool {
	return len(p.SignerNode) != 0 && len(p.VerifierNode) != 0 && len(p.SessionID) != 0
}
