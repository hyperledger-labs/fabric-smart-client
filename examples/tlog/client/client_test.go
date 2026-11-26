/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"testing"

	rekorv1 "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/views"
)

// orgKey is one org's log signing key, with its note signer and verifier,
// the way each org's node would hold its own key.
type orgKey struct {
	signer   note.Signer
	verifier note.Verifier
}

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

// verifiersOf returns the verifiers of keys, in order.
func verifiersOf(keys []orgKey) []note.Verifier {
	vs := make([]note.Verifier, len(keys))
	for i, k := range keys {
		vs[i] = k.verifier
	}
	return vs
}

// signCheckpoint signs the (size, root) checkpoint with every key in
// signers and merges the signatures, verified against signers' own
// verifiers (not necessarily all three orgs, so a test can build a
// checkpoint missing a signature on purpose).
func signCheckpoint(t *testing.T, size uint64, root []byte, signers []orgKey) []byte {
	t.Helper()
	notes := make([][]byte, len(signers))
	var err error
	for i, k := range signers {
		notes[i], err = translog.SignCheckpoint(size, root, k.signer)
		require.NoError(t, err)
	}
	cp, err := translog.MergeSignatures(verifiersOf(signers), notes...)
	require.NoError(t, err)
	return cp
}

// canonicalEntryFor returns the canonical entry body of artifact, honestly
// signed by a fresh client key, and that key.
func canonicalEntryFor(t *testing.T, artifact []byte) ([]byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	digest := sha256.Sum256(artifact)
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	require.NoError(t, err)
	body, err := translog.CanonicalEntry(translog.NewRequest(digest[:], sig, pub))
	require.NoError(t, err)
	return body, key
}

// buildEntry builds a 4-leaf tree whose entry at index 2 is the canonical,
// honestly-signed entry for artifact, checkpointed at that tree's root and
// cosigned by signers. It returns the entry, with an inclusion proof
// against that checkpoint, and the client key that signed artifact.
func buildEntry(t *testing.T, artifact []byte, signers []orgKey) (*rekorv1.TransparencyLogEntry, *ecdsa.PrivateKey) {
	t.Helper()
	const index = 2
	body, key := canonicalEntryFor(t, artifact)

	leaves := make([][]byte, 4)
	for i := range leaves {
		if i == index {
			leaves[i] = body
			continue
		}
		leaves[i] = fmt.Appendf(nil, "filler-%d", i)
	}
	tree, err := translog.NewTree(leaves)
	require.NoError(t, err)
	root, err := tree.Root(tree.Size())
	require.NoError(t, err)
	hashes, err := tree.InclusionProof(index, tree.Size())
	require.NoError(t, err)
	cp := signCheckpoint(t, tree.Size(), root, signers)

	entry := &rekorv1.TransparencyLogEntry{
		LogIndex:          index,
		CanonicalizedBody: body,
		KindVersion:       &rekorv1.KindVersion{Kind: "hashedrekord", Version: "0.0.2"},
		InclusionProof: &rekorv1.InclusionProof{
			LogIndex:   index,
			TreeSize:   int64(tree.Size()), //nolint:gosec // test tree, never near 2^63
			RootHash:   root,
			Hashes:     hashes,
			Checkpoint: &rekorv1.Checkpoint{Envelope: string(cp)},
		},
	}
	return entry, key
}

func TestVerifyAcceptsHonestEntry(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	artifact := []byte("artifact")
	entry, key := buildEntry(t, artifact, orgs)

	require.NoError(t, client.Verify(entry, artifact, &key.PublicKey, verifiersOf(orgs)))
}

func TestVerifyRejectsRewrittenBody(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	artifact := []byte("artifact")
	entry, key := buildEntry(t, artifact, orgs)

	tampered := append([]byte(nil), entry.CanonicalizedBody...)
	tampered[len(tampered)/2] ^= 0xff
	entry.CanonicalizedBody = tampered

	require.Error(t, client.Verify(entry, artifact, &key.PublicKey, verifiersOf(orgs)))
}

func TestVerifyRejectsTwoSignatureCheckpoint(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	artifact := []byte("artifact")
	// Only two of the three orgs cosign the checkpoint.
	entry, key := buildEntry(t, artifact, orgs[:2])

	err := client.Verify(entry, artifact, &key.PublicKey, verifiersOf(orgs))
	require.ErrorContains(t, err, "missing signature")
}

func TestVerifyRejectsOtherArtifact(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	entry, key := buildEntry(t, []byte("artifact"), orgs)

	err := client.Verify(entry, []byte("some other artifact"), &key.PublicKey, verifiersOf(orgs))
	require.Error(t, err)
}

// fakeNode is a stub api.ViewClient that always answers the next call with
// out, recording the view id and input it was called with.
type fakeNode struct {
	out []byte
	fid string
	in  []byte
}

func (f *fakeNode) CallViewWithContext(_ context.Context, fid string, in []byte) (any, error) {
	f.fid, f.in = fid, in
	return f.out, nil
}

func (f *fakeNode) CallView(fid string, in []byte) (any, error) {
	return f.CallViewWithContext(context.Background(), fid, in)
}

func TestGetRejectsMismatchedIndex(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	// buildEntry always builds the entry at index 2; a node that answers a
	// different requested index with it (honest proof, genuine 3-of-3
	// checkpoint, wrong entry) must be rejected.
	entry, _ := buildEntry(t, []byte("artifact"), orgs)
	raw, err := protojson.Marshal(entry)
	require.NoError(t, err)

	c := &client.Client{Node: &fakeNode{out: raw}, Verifiers: verifiersOf(orgs)}
	_, err = c.Get(context.Background(), 1)
	require.Error(t, err)
}

func TestGetAcceptsMatchingIndex(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	entry, _ := buildEntry(t, []byte("artifact"), orgs)
	raw, err := protojson.Marshal(entry)
	require.NoError(t, err)

	c := &client.Client{Node: &fakeNode{out: raw}, Verifiers: verifiersOf(orgs)}
	got, err := c.Get(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, entry.CanonicalizedBody, got.CanonicalizedBody)
}

func TestAuditFirstCheckpoint(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	root := sha256.Sum256([]byte("root"))
	cp := signCheckpoint(t, 4, root[:], orgs)
	resp, err := json.Marshal(&views.CheckpointResponse{Checkpoint: cp})
	require.NoError(t, err)

	c := &client.Client{Node: &fakeNode{out: resp}, Verifiers: verifiersOf(orgs)}
	require.NoError(t, c.Audit(context.Background()))
	require.Equal(t, cp, c.Latest)
}

// buildCheckpoints builds a 2-leaf tree, checkpoints it, grows it to 4
// leaves, checkpoints it again, and returns both checkpoints (cosigned by
// signers) plus the consistency proof between them.
func buildCheckpoints(t *testing.T, signers []orgKey) (oldCp, newCp []byte, consistency [][]byte) {
	t.Helper()
	tree := &translog.Tree{}
	for i := range 2 {
		require.NoError(t, tree.Append(fmt.Appendf(nil, "leaf-%d", i)))
	}
	oldSize := tree.Size()
	oldRoot, err := tree.Root(oldSize)
	require.NoError(t, err)

	for i := 2; i < 4; i++ {
		require.NoError(t, tree.Append(fmt.Appendf(nil, "leaf-%d", i)))
	}
	newRoot, err := tree.Root(tree.Size())
	require.NoError(t, err)

	consistency, err = tree.ConsistencyProof(oldSize, tree.Size())
	require.NoError(t, err)

	oldCp = signCheckpoint(t, oldSize, oldRoot, signers)
	newCp = signCheckpoint(t, tree.Size(), newRoot, signers)
	return oldCp, newCp, consistency
}

func TestAuditVerifiesConsistency(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	oldCp, newCp, consistency := buildCheckpoints(t, orgs)
	resp, err := json.Marshal(&views.CheckpointResponse{Checkpoint: newCp, Consistency: consistency})
	require.NoError(t, err)

	c := &client.Client{Node: &fakeNode{out: resp}, Verifiers: verifiersOf(orgs), Latest: oldCp}
	require.NoError(t, c.Audit(context.Background()))
	require.Equal(t, newCp, c.Latest)
}

func TestAuditRejectsForgedConsistencyProof(t *testing.T) {
	t.Parallel()
	orgs := []orgKey{newOrgKey(t), newOrgKey(t), newOrgKey(t)}
	oldCp, newCp, consistency := buildCheckpoints(t, orgs)
	forged := append([]byte(nil), consistency[0]...)
	forged[0] ^= 0xff
	resp, err := json.Marshal(&views.CheckpointResponse{Checkpoint: newCp, Consistency: [][]byte{forged}})
	require.NoError(t, err)

	c := &client.Client{Node: &fakeNode{out: resp}, Verifiers: verifiersOf(orgs), Latest: oldCp}
	require.Error(t, c.Audit(context.Background()))
	require.Equal(t, oldCp, c.Latest, "a failed audit must not advance Latest")
}
