/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// testECDSASigner and testECDSAVerifier implement driver.Signer and
// driver.Verifier over a real ECDSA key, so the proof tests exercise actual
// signature verification rather than a mock.
type testECDSASigner struct {
	key *ecdsa.PrivateKey
}

func (s *testECDSASigner) Sign(message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	return ecdsa.SignASN1(rand.Reader, s.key, digest[:])
}

type testECDSAVerifier struct {
	pub *ecdsa.PublicKey
}

func (v *testECDSAVerifier) Verify(message, sigma []byte) error {
	digest := sha256.Sum256(message)
	if !ecdsa.VerifyASN1(v.pub, digest[:], sigma) {
		return errors.New("signature verification failed")
	}
	return nil
}

func newTestECDSAKeyPair(t *testing.T) (*testECDSASigner, *testECDSAVerifier) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &testECDSASigner{key: key}, &testECDSAVerifier{pub: &key.PublicKey}
}

func TestProofPayload_Unambiguous(t *testing.T) {
	t.Parallel()

	p1 := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "ab",
		ContextID:    "c",
		SignerNode:   view.Identity("signer"),
		VerifierNode: view.Identity("verifier"),
		Identity:     view.Identity("identity"),
	}
	p2 := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "a",
		ContextID:    "bc",
		SignerNode:   view.Identity("signer"),
		VerifierNode: view.Identity("verifier"),
		Identity:     view.Identity("identity"),
	}

	require.NotEqual(t, p1.Payload(), p2.Payload())
}

func TestProofPayload_StartsWithZero(t *testing.T) {
	t.Parallel()

	p := ProofContext{
		Tag:          TagRequestReply,
		SessionID:    "session",
		ContextID:    "context",
		SignerNode:   view.Identity("signer"),
		VerifierNode: view.Identity("verifier"),
		Identity:     view.Identity("identity"),
	}

	payload := p.Payload()
	require.NotEmpty(t, payload)
	require.Equal(t, byte(0x00), payload[0])
}

func TestVerify_ValidProof(t *testing.T) {
	t.Parallel()

	signer, verifier := newTestECDSAKeyPair(t)
	p := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("carol"),
		VerifierNode: view.Identity("carol-verifier"),
		Identity:     view.Identity("carol-recipient"),
	}

	sig, err := SignRecipientProof(signer, p)
	require.NoError(t, err)
	require.NoError(t, VerifyRecipientProof(verifier, p, sig))
}

func TestVerify_WrongVerifierNode(t *testing.T) {
	t.Parallel()

	signer, verifier := newTestECDSAKeyPair(t)
	// The proof is signed naming "mallory" as the verifier (e.g. obtained by
	// a relay from the victim), but a third party, "carol", checks it as the
	// intended recipient. The mismatched verifierNode must be refused.
	signed := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("mallory"),
		Identity:     view.Identity("alice-recipient"),
	}
	sig, err := SignRecipientProof(signer, signed)
	require.NoError(t, err)

	checked := signed
	checked.VerifierNode = view.Identity("carol")

	err = VerifyRecipientProof(verifier, checked, sig)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRecipientProof)
}

func TestVerify_WrongTag(t *testing.T) {
	t.Parallel()

	signer, verifier := newTestECDSAKeyPair(t)
	signed := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     view.Identity("alice-recipient"),
	}
	sig, err := SignRecipientProof(signer, signed)
	require.NoError(t, err)

	checked := signed
	checked.Tag = TagExchangeReply

	err = VerifyRecipientProof(verifier, checked, sig)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRecipientProof)
}

func TestVerify_WrongSession(t *testing.T) {
	t.Parallel()

	signer, verifier := newTestECDSAKeyPair(t)
	signed := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     view.Identity("alice-recipient"),
	}
	sig, err := SignRecipientProof(signer, signed)
	require.NoError(t, err)

	checked := signed
	checked.SessionID = "session-2"

	err = VerifyRecipientProof(verifier, checked, sig)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRecipientProof)
}

func TestVerify_Missing(t *testing.T) {
	t.Parallel()

	_, verifier := newTestECDSAKeyPair(t)
	p := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     view.Identity("alice-recipient"),
	}

	err := VerifyRecipientProof(verifier, p, nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.Contains(t, err.Error(), "recipient proof: missing")

	err = VerifyRecipientProof(verifier, p, []byte{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRecipientProof)
	require.Contains(t, err.Error(), "recipient proof: missing")
}

func TestVerify_WrongKey(t *testing.T) {
	t.Parallel()

	signerA, _ := newTestECDSAKeyPair(t)
	_, verifierB := newTestECDSAKeyPair(t)
	p := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     view.Identity("alice-recipient"),
	}

	sig, err := SignRecipientProof(signerA, p)
	require.NoError(t, err)

	err = VerifyRecipientProof(verifierB, p, sig)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrRecipientProof)
}

func TestProofPayload_CanonicalNodeIdentities(t *testing.T) {
	t.Parallel()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(1)}, &x509.Certificate{SerialNumber: big.NewInt(1)}, &key.PublicKey, key)
	require.NoError(t, err)
	canonical := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	nonCanonical := append([]byte("# node cert\r\n"), bytes.ReplaceAll(canonical, []byte("\n"), []byte("\r\n"))...)

	p := func(signer, verifier []byte) ProofContext {
		return ProofContext{Tag: TagExchangeRequest, SessionID: "s", ContextID: "c", SignerNode: signer, VerifierNode: verifier, Identity: view.Identity("x")}
	}
	require.Equal(t, p(canonical, canonical).Payload(), p(nonCanonical, nonCanonical).Payload())
	require.NotEqual(t, p(canonical, canonical).Payload(), p(canonical, []byte("other")).Payload())
}

func TestVerify_IncompleteContext(t *testing.T) {
	t.Parallel()

	signer, verifier := newTestECDSAKeyPair(t)
	full := ProofContext{
		Tag:          TagExchangeRequest,
		SessionID:    "session-1",
		ContextID:    "context-1",
		SignerNode:   view.Identity("alice"),
		VerifierNode: view.Identity("bob"),
		Identity:     view.Identity("alice-recipient"),
	}
	for name, clear := range map[string]func(*ProofContext){
		"signer node":   func(p *ProofContext) { p.SignerNode = nil },
		"verifier node": func(p *ProofContext) { p.VerifierNode = nil },
		"session":       func(p *ProofContext) { p.SessionID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := full
			clear(&p)
			sig, err := SignRecipientProof(signer, p)
			require.NoError(t, err)

			err = VerifyRecipientProof(verifier, p, sig)
			require.ErrorIs(t, err, ErrRecipientProof)
			require.ErrorContains(t, err, "recipient proof: incomplete context")
		})
	}
}
