/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package translog

import (
	"context"
	"crypto"
	"crypto/ecdsa"

	rtnote "github.com/sigstore/rekor-tiles/v2/pkg/note"
	"github.com/sigstore/rekor-tiles/v2/pkg/verify"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// Origin identifies this demo's log in every checkpoint it signs, the same
// way across all three orgs: their keys differ, but they share Name() and
// the checkpoints they sign.
const Origin = "tlog.example"

// NewSigner returns a note signer for key, under Origin.
func NewSigner(key *ecdsa.PrivateKey) (note.Signer, error) {
	sv, err := signature.LoadECDSASignerVerifier(key, crypto.SHA256)
	if err != nil {
		return nil, errors.Wrap(err, "loading ECDSA signer")
	}
	s, err := rtnote.NewNoteSigner(context.Background(), Origin, sv)
	if err != nil {
		return nil, errors.Wrap(err, "building note signer")
	}
	return s, nil
}

// NewVerifier returns a note verifier for pub, under Origin.
func NewVerifier(pub *ecdsa.PublicKey) (note.Verifier, error) {
	v, err := signature.LoadECDSAVerifier(pub, crypto.SHA256)
	if err != nil {
		return nil, errors.Wrap(err, "loading ECDSA verifier")
	}
	nv, err := rtnote.NewNoteVerifier(Origin, v)
	if err != nil {
		return nil, errors.Wrap(err, "building note verifier")
	}
	return nv, nil
}

// SignCheckpoint signs a checkpoint over root at size with s, returning the
// signed note bytes.
func SignCheckpoint(size uint64, root []byte, s note.Signer) ([]byte, error) {
	cp := log.Checkpoint{Origin: Origin, Size: size, Hash: root}
	signed, err := note.Sign(&note.Note{Text: string(cp.Marshal())}, s)
	if err != nil {
		return nil, errors.Wrap(err, "signing checkpoint")
	}
	return signed, nil
}

// MergeSignatures combines single-signer checkpoint notes into one note
// carrying every signature, verified against vs. It is an error if the
// notes sign different checkpoint texts or a note fails to open.
func MergeSignatures(vs []note.Verifier, notes ...[]byte) ([]byte, error) {
	if len(notes) == 0 {
		return nil, errors.New("no notes to merge")
	}
	verifiers := note.VerifierList(vs...)
	merged, err := note.Open(notes[0], verifiers)
	if err != nil {
		return nil, errors.Wrap(err, "opening checkpoint note")
	}
	for _, raw := range notes[1:] {
		n, err := note.Open(raw, verifiers)
		if err != nil {
			return nil, errors.Wrap(err, "opening checkpoint note")
		}
		if n.Text != merged.Text {
			return nil, errors.New("notes sign different checkpoints")
		}
		merged.Sigs = append(merged.Sigs, n.Sigs...)
	}
	// No signers: note.Sign only re-encodes merged.Sigs and
	// merged.UnverifiedSigs (the first note's signatures by unknown keys), it
	// adds none.
	out, err := note.Sign(merged)
	if err != nil {
		return nil, errors.Wrap(err, "encoding merged note")
	}
	return out, nil
}

// VerifyAll verifies cp as a checkpoint witnessed by vs and requires a valid
// signature from every verifier in vs, matched by Name and KeyHash: the
// 3-of-3 check a client relies on, since formats/log.ParseCheckpoint itself
// only requires the first verifier's signature.
func VerifyAll(cp []byte, vs []note.Verifier) (*log.Checkpoint, error) {
	if len(vs) == 0 {
		return nil, errors.New("no verifiers given")
	}
	// Count the signatures before parsing: the parse rejects a note without
	// vs[0]'s signature on its own terms, hiding which orgs did not sign.
	n, err := note.Open(cp, note.VerifierList(vs...))
	if err != nil {
		return nil, errors.Wrap(err, "verifying checkpoint signatures")
	}
	for _, v := range vs {
		found := false
		for _, sig := range n.Sigs {
			if sig.Name == v.Name() && sig.Hash == v.KeyHash() {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Errorf("missing signature of key %08x", v.KeyHash())
		}
	}
	checkpoint, _, err := verify.VerifyWitnessedCheckpoint(string(cp), vs[0], vs[1:]...)
	if err != nil {
		return nil, errors.Wrap(err, "verifying witnessed checkpoint")
	}
	return checkpoint, nil
}
