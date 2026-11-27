/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package translog_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/sumdb/note"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
)

// checkpointSize is the log size every test checkpoint claims.
const checkpointSize = 7

// orgKey is one org's log signing key, with its note signer and verifier.
type orgKey struct {
	signer   note.Signer
	verifier note.Verifier
}

// newOrgKey generates a fresh P-256 key and wraps it as a checkpoint
// signer/verifier pair, the way each org's node would for its own key.
func newOrgKey(t *testing.T) orgKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer, err := translog.NewSigner(priv)
	require.NoError(t, err)
	verifier, err := translog.NewVerifier(&priv.PublicKey)
	require.NoError(t, err)
	return orgKey{signer: signer, verifier: verifier}
}

// checkpointRoot returns a deterministic, distinguishable 32-byte root hash.
func checkpointRoot(seed string) []byte {
	h := sha256.Sum256([]byte(seed))
	return h[:]
}

// signedNote signs the (checkpointSize, root) checkpoint with key.
func signedNote(t *testing.T, key orgKey, root []byte) []byte {
	t.Helper()
	n, err := translog.SignCheckpoint(checkpointSize, root, key.signer)
	require.NoError(t, err)
	return n
}

func TestVerifyAllAcceptsThreeSignatures(t *testing.T) {
	t.Parallel()
	org1, org2, org3 := newOrgKey(t), newOrgKey(t), newOrgKey(t)
	root := checkpointRoot("root")
	verifiers := []note.Verifier{org1.verifier, org2.verifier, org3.verifier}

	merged, err := translog.MergeSignatures(verifiers,
		signedNote(t, org1, root), signedNote(t, org2, root), signedNote(t, org3, root))
	require.NoError(t, err)

	cp, err := translog.VerifyAll(merged, verifiers)
	require.NoError(t, err)
	require.Equal(t, uint64(checkpointSize), cp.Size)
	require.Equal(t, root, cp.Hash)
}

func TestVerifyAllRejectsTwoSignatures(t *testing.T) {
	t.Parallel()
	org1, org2, org3 := newOrgKey(t), newOrgKey(t), newOrgKey(t)
	root := checkpointRoot("root")
	allVerifiers := []note.Verifier{org1.verifier, org2.verifier, org3.verifier}

	merged, err := translog.MergeSignatures([]note.Verifier{org1.verifier, org2.verifier},
		signedNote(t, org1, root), signedNote(t, org2, root))
	require.NoError(t, err)

	_, err = translog.VerifyAll(merged, allVerifiers)
	require.ErrorContains(t, err, "missing signature")
}

func TestVerifyAllRejectsOnlyThirdSignature(t *testing.T) {
	t.Parallel()
	org1, org2, org3 := newOrgKey(t), newOrgKey(t), newOrgKey(t)
	root := checkpointRoot("root")

	// A checkpoint only org3 signed lacks org1's signature too, which
	// ParseCheckpoint's check of the first verifier's (the log's) signature
	// would report on its own terms; VerifyAll counts the signatures first.
	_, err := translog.VerifyAll(signedNote(t, org3, root), []note.Verifier{org1.verifier, org2.verifier, org3.verifier})
	require.ErrorContains(t, err, "missing signature")
}

func TestVerifyAllIgnoresUnknownSigner(t *testing.T) {
	t.Parallel()
	org1, org2, org3, outsider := newOrgKey(t), newOrgKey(t), newOrgKey(t), newOrgKey(t)
	root := checkpointRoot("root")
	knownVerifiers := []note.Verifier{org1.verifier, org2.verifier, org3.verifier}
	allVerifiers := append(append([]note.Verifier{}, knownVerifiers...), outsider.verifier)

	merged, err := translog.MergeSignatures(allVerifiers,
		signedNote(t, org1, root), signedNote(t, org2, root), signedNote(t, org3, root), signedNote(t, outsider, root))
	require.NoError(t, err)

	cp, err := translog.VerifyAll(merged, knownVerifiers)
	require.NoError(t, err)
	require.Equal(t, root, cp.Hash)
}

func TestVerifyAllRejectsTamperedSignature(t *testing.T) {
	t.Parallel()
	org1, org2, org3 := newOrgKey(t), newOrgKey(t), newOrgKey(t)
	root := checkpointRoot("root")
	verifiers := []note.Verifier{org1.verifier, org2.verifier, org3.verifier}

	merged, err := translog.MergeSignatures(verifiers,
		signedNote(t, org1, root), signedNote(t, org2, root), signedNote(t, org3, root))
	require.NoError(t, err)

	// Flip a byte a few characters into the base64 field of the first
	// signature line ("— tlog.example <base64>\n").
	idx := bytes.Index(merged, []byte("— "))
	require.GreaterOrEqual(t, idx, 0)
	offset := idx + len("— ") + len(translog.Origin) + 1 + 5
	tampered := append([]byte(nil), merged...)
	tampered[offset] ^= 0xff

	_, err = translog.VerifyAll(tampered, verifiers)
	require.Error(t, err)
}

func TestMergeRejectsDifferentTrees(t *testing.T) {
	t.Parallel()
	org1, org2, org3 := newOrgKey(t), newOrgKey(t), newOrgKey(t)
	root1 := checkpointRoot("root-1")
	root2 := checkpointRoot("root-2")
	verifiers := []note.Verifier{org1.verifier, org2.verifier, org3.verifier}

	_, err := translog.MergeSignatures(verifiers,
		signedNote(t, org1, root1), signedNote(t, org2, root2), signedNote(t, org3, root1))
	require.Error(t, err)
}
