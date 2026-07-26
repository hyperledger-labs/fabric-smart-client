/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mock"
)

// multiEndorsementNamespace builds a namespace endorsement set carrying
// MULTIPLE EndorsementWithIdentity entries, exactly the shape a malicious or
// compromised endorsing peer could return: a first entry that is a real,
// correctly-signed endorsement, followed by one or more attacker-injected
// entries claiming additional MSP IDs with arbitrary/forged signatures.
func multiEndorsementNamespace(entries ...*applicationpb.EndorsementWithIdentity) *applicationpb.Endorsements {
	return &applicationpb.Endorsements{EndorsementsWithIdentity: entries}
}

func endorsementEntry(mspID, sig string) *applicationpb.EndorsementWithIdentity {
	return &applicationpb.EndorsementWithIdentity{
		Identity:    &msppb.Identity{MspId: mspID},
		Endorsement: []byte(sig),
	}
}

// TestVerifyEndorsementRejectsMultipleEndorsementsPerNamespace proves the fix
// for the original "only checks items[0]" gap: a namespace endorsement set
// carrying more than one EndorsementWithIdentity entry -- exactly the shape a
// malicious or compromised endorsing peer would return to smuggle a forged,
// never-verified entry past the first, genuinely-checked one -- is now
// rejected outright, rather than silently accepted with only the first entry
// checked.
func TestVerifyEndorsementRejectsMultipleEndorsementsPerNamespace(t *testing.T) {
	t.Parallel()

	txID := "tx1"
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	endorser, identity := mustEndorserAndIdentity(t, "Org1MSP")

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser: endorser,
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				multiEndorsementNamespace(
					&applicationpb.EndorsementWithIdentity{Identity: identity, Endorsement: []byte("sig-org1-real")},
					endorsementEntry("Org2MSP", "sig-org2-forged"),
				),
			}),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte(txID),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mock.VerifierProvider{}
	fakeVerifier := &mock.Verifier{}
	fakeProvider.GetVerifierReturns(fakeVerifier, nil)
	fakeVerifier.VerifyStub = func(_ []byte, sig []byte) error {
		if string(sig) == "sig-org2-forged" {
			return assert.AnError
		}
		return nil
	}

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err, "a namespace carrying more than one endorsement entry must be rejected")
	require.Contains(t, err.Error(), "expected exactly one endorsement")
	require.Equal(t, 0, fakeVerifier.VerifyCallCount(), "no signature should be verified once the entry count is rejected")
}

// TestMergeProposalResponseEndorsementsSmugglesUnverifiedEndorsement documents
// that mergeProposalResponseEndorsements, taken in isolation, still performs
// no cryptographic verification of its own and will fold every
// EndorsementWithIdentity entry present into the final applicationpb.Tx. This
// is safe in practice because VerifyEndorsement (see
// TestVerifyEndorsementRejectsMultipleEndorsementsPerNamespace and
// TestEndorsementVerificationBypassIsRejectedEndToEnd) is the gate that must
// pass before a response is ever appended and handed to the merge step -- see
// platform/fabric/services/endorser/endorsement.go and endorsement_proposal.go.
func TestMergeProposalResponseEndorsementsSmugglesUnverifiedEndorsement(t *testing.T) {
	t.Parallel()

	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			multiEndorsementNamespace(
				endorsementEntry("Org1MSP", "sig-org1-real"),
				endorsementEntry("Org2MSP", "sig-org2-forged"),
			),
		}),
	}

	merged, err := mergeProposalResponseEndorsements([]driver.ProposalResponse{resp})
	require.NoError(t, err)

	require.Len(t, merged.Endorsements, 1)
	items := merged.Endorsements[0].GetEndorsementsWithIdentity()
	require.Len(t, items, 2, "merge itself does not verify; VerifyEndorsement is the gate that must reject this response first")
}

// TestEndorsementVerificationBypassIsRejectedEndToEnd chains both halves
// together to prove the full, end-to-end fix: a malicious proposal response
// carrying a genuine first entry plus a second, forged EndorsementWithIdentity
// entry is now rejected by VerifyEndorsement -- the check every caller relies
// on before accepting a peer's response, see
// platform/fabric/services/endorser/endorsement.go and
// endorsement_proposal.go -- so it never reaches AppendProposalResponse or
// mergeProposalResponseEndorsements at all.
func TestEndorsementVerificationBypassIsRejectedEndToEnd(t *testing.T) {
	t.Parallel()

	txID := "tx1"
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	endorser, identity := mustEndorserAndIdentity(t, "Org1MSP")

	maliciousSignature := mustSerializedEndorsements(t, []*applicationpb.Endorsements{
		multiEndorsementNamespace(
			&applicationpb.EndorsementWithIdentity{Identity: identity, Endorsement: []byte("sig-org1-real")},
			endorsementEntry("Org2MSP", "sig-org2-forged"),
		),
	})

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser:  endorser,
			Signature: maliciousSignature,
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte(txID),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mock.VerifierProvider{}
	fakeVerifier := &mock.Verifier{}
	fakeProvider.GetVerifierReturns(fakeVerifier, nil)
	fakeVerifier.VerifyStub = func(_ []byte, sig []byte) error {
		if string(sig) == "sig-org2-forged" {
			return assert.AnError
		}
		return nil
	}

	// The client-side verification gate now rejects the malicious response
	// outright, before it can ever be appended and merged into the final
	// transaction that would be signed and submitted.
	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err, "the malicious response must be rejected by VerifyEndorsement")
	require.Contains(t, err.Error(), "expected exactly one endorsement")
}

// TestVerifyEndorsementRejectsMismatchedClaimedIdentity proves the fix for the
// second, independent gap: VerifyEndorsement resolves the cryptographic
// verifier from the ProposalResponse's OWN endorser identity
// (p.pr.Endorsement.Endorser), and now also cross-checks that the identity
// claimed by the single EndorsementWithIdentity entry actually corresponds to
// that same endorser, rather than accepting it at face value.
//
// Without this check, a single malicious (or compromised) endorser, using
// nothing but its own genuine signing key, could unilaterally mislabel its
// own valid signature as having come from a different organization.
func TestVerifyEndorsementRejectsMismatchedClaimedIdentity(t *testing.T) {
	t.Parallel()

	txID := "tx1"
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	// The ProposalResponse's real endorser is Org1MSP (this is who
	// GetVerifier below will resolve a verifier for, and who genuinely
	// produced the signature). But the endorsement's claimed Identity says
	// "Org2MSP" -- a lie the fix must catch.
	endorser, _ := mustEndorserAndIdentity(t, "Org1MSP")

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser: endorser,
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				multiEndorsementNamespace(
					endorsementEntry("Org2MSP", "sig-actually-from-org1"),
				),
			}),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte(txID),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mock.VerifierProvider{}
	fakeVerifier := &mock.Verifier{}
	fakeProvider.GetVerifierReturns(fakeVerifier, nil)
	fakeVerifier.VerifyReturns(nil) // Org1MSP's key genuinely signed this digest.

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err, "verification must reject a signature whose claimed identity does not match the actual endorser")
	require.Contains(t, err.Error(), "does not correspond to endorser")

	// The verifier provider was still asked to resolve a verifier for
	// Org1MSP (the true endorser)...
	require.Equal(t, 1, fakeProvider.GetVerifierCallCount())
	// ...but the signature was never actually checked, because the identity
	// mismatch is caught first.
	require.Equal(t, 0, fakeVerifier.VerifyCallCount())
}
