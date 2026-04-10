/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/hyperledger/fabric-x-common/api/msppb"
	"github.com/stretchr/testify/require"
)

// mockProposalResponse is a lightweight test double for driver.ProposalResponse.
// Only the methods used by mergeProposalResponseEndorsements need meaningful values.
// The rest are implemented just to satisfy the interface.
type mockProposalResponse struct {
	payload           []byte
	endorserSignature []byte
}

func (m *mockProposalResponse) Payload() []byte           { return m.payload }
func (m *mockProposalResponse) Results() []byte           { return nil }
func (m *mockProposalResponse) Endorser() []byte          { return nil }
func (m *mockProposalResponse) EndorserSignature() []byte { return m.endorserSignature }
func (m *mockProposalResponse) ResponseStatus() int32     { return 200 }
func (m *mockProposalResponse) ResponseMessage() string   { return "" }
func (m *mockProposalResponse) Bytes() ([]byte, error)    { return nil, nil }

func (m *mockProposalResponse) VerifyEndorsement(_ driver.VerifierProvider) error {
	return nil
}

// TestMergeProposalResponseEndorsements verifies the happy path:
// two proposal responses for the exact same transaction payload, each carrying
// one endorsement for the same namespace from a different MSP, should be merged
// into a single tx containing both endorsements sorted by MSP ID.
func TestMergeProposalResponseEndorsements(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp1 := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org2MSP", "sig-org2"),
		}),
	}
	resp2 := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		}),
	}

	merged, err := mergeProposalResponseEndorsements([]driver.ProposalResponse{resp1, resp2})
	require.NoError(t, err)

	require.Len(t, merged.Endorsements, 1)
	require.Len(t, merged.Endorsements[0].GetEndorsementsWithIdentity(), 2)

	require.Equal(t, "Org1MSP", merged.Endorsements[0].GetEndorsementsWithIdentity()[0].GetIdentity().GetMspId())
	require.Equal(t, []byte("sig-org1"), merged.Endorsements[0].GetEndorsementsWithIdentity()[0].GetEndorsement())

	require.Equal(t, "Org2MSP", merged.Endorsements[0].GetEndorsementsWithIdentity()[1].GetIdentity().GetMspId())
	require.Equal(t, []byte("sig-org2"), merged.Endorsements[0].GetEndorsementsWithIdentity()[1].GetEndorsement())
}

// TestMergeProposalResponseEndorsementsSingleResponse verifies that a single
// proposal response is accepted and its endorsements are preserved.
func TestMergeProposalResponseEndorsementsSingleResponse(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		}),
	}

	merged, err := mergeProposalResponseEndorsements([]driver.ProposalResponse{resp})
	require.NoError(t, err)

	require.Len(t, merged.Endorsements, 1)
	require.Len(t, merged.Endorsements[0].GetEndorsementsWithIdentity(), 1)
	require.Equal(t, "Org1MSP", merged.Endorsements[0].GetEndorsementsWithIdentity()[0].GetIdentity().GetMspId())
	require.Equal(t, []byte("sig-org1"), merged.Endorsements[0].GetEndorsementsWithIdentity()[0].GetEndorsement())
}

// TestMergeProposalResponseEndorsementsDeduplicatesByMSPID verifies that duplicate
// endorsements are not appended twice when multiple proposal responses carry
// endorsements from the same MSP.
func TestMergeProposalResponseEndorsementsDeduplicatesByMSPID(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp1 := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		}),
	}
	resp2 := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org1MSP", "sig-org1-different-bytes"),
		}),
	}

	merged, err := mergeProposalResponseEndorsements([]driver.ProposalResponse{resp1, resp2})
	require.NoError(t, err)

	require.Len(t, merged.Endorsements, 1)
	require.Len(t, merged.Endorsements[0].GetEndorsementsWithIdentity(), 1)
	require.Equal(t, "Org1MSP", merged.Endorsements[0].GetEndorsementsWithIdentity()[0].GetIdentity().GetMspId())
	require.Equal(t, []byte("sig-org1"), merged.Endorsements[0].GetEndorsementsWithIdentity()[0].GetEndorsement())
}

// TestMergeProposalResponseEndorsementsPayloadMismatch verifies that merge fails
// if proposal responses do not refer to the exact same tx payload.
// Endorsements may only be merged across identical transactions.
func TestMergeProposalResponseEndorsementsPayloadMismatch(t *testing.T) {
	t.Parallel()
	tx1 := sampleTx("ns1", "key1", "value1")
	tx2 := sampleTx("ns1", "key2", "value2")

	rawTx1, err := proto.Marshal(tx1)
	require.NoError(t, err)
	rawTx2, err := proto.Marshal(tx2)
	require.NoError(t, err)

	resp1 := &mockProposalResponse{
		payload: rawTx1,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		}),
	}
	resp2 := &mockProposalResponse{
		payload: rawTx2,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			sampleNamespaceEndorsements("Org2MSP", "sig-org2"),
		}),
	}

	_, err = mergeProposalResponseEndorsements([]driver.ProposalResponse{resp1, resp2})
	require.Error(t, err)
	require.Contains(t, err.Error(), "content mismatch")
}

// TestMergeProposalResponseEndorsementsMissingEndorsement verifies that merge
// fails when a namespace endorsement set exists but contains no actual
// EndorsementsWithIdentity entries.
func TestMergeProposalResponseEndorsementsMissingEndorsement(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			{},
		}),
	}

	_, err = mergeProposalResponseEndorsements([]driver.ProposalResponse{resp})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires at least one endorsement")
}

// TestMergeProposalResponseEndorsementsWrongNamespaceCount verifies that merge
// fails when the number of serialized endorsement sets does not match the
// number of namespaces in the tx.
func TestMergeProposalResponseEndorsementsWrongNamespaceCount(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp := &mockProposalResponse{
		payload:           rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{}),
	}

	_, err = mergeProposalResponseEndorsements([]driver.ProposalResponse{resp})
	require.Error(t, err)
	require.Contains(t, err.Error(), "endorsement sets")
}

// TestMergeProposalResponseEndorsementsNilNamespaceEndorsements verifies that
// a nil endorsement object for a namespace is treated as missing endorsements.
func TestMergeProposalResponseEndorsementsNilNamespaceEndorsements(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			nil,
		}),
	}

	_, err = mergeProposalResponseEndorsements([]driver.ProposalResponse{resp})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires at least one endorsement")
}

// TestMergeProposalResponseEndorsementsMissingMSPID verifies that merge fails
// when an endorsement does not carry an MSP ID.
func TestMergeProposalResponseEndorsementsMissingMSPID(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	resp := &mockProposalResponse{
		payload: rawTx,
		endorserSignature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
			{
				EndorsementsWithIdentity: []*applicationpb.EndorsementWithIdentity{
					{
						Identity:    &msppb.Identity{},
						Endorsement: []byte("sig-org1"),
					},
				},
			},
		}),
	}

	_, err = mergeProposalResponseEndorsements([]driver.ProposalResponse{resp})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing MSP ID")
}

// TestMergeProposalResponseEndorsementsNoResponses verifies that merge fails
// when no proposal responses are provided.
func TestMergeProposalResponseEndorsementsNoResponses(t *testing.T) {
	t.Parallel()
	_, err := mergeProposalResponseEndorsements(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no proposal responses provided")
}

// sampleTx creates a minimal transaction with a single namespace and a single
// blind write. This is enough structure for the merge logic, which only needs
// consistent tx payloads and namespace counts.
func sampleTx(namespace, key, value string) *applicationpb.Tx {
	return &applicationpb.Tx{
		Namespaces: []*applicationpb.TxNamespace{
			{
				NsId: namespace,
				BlindWrites: []*applicationpb.Write{
					{
						Key:   []byte(key),
						Value: []byte(value),
					},
				},
			},
		},
	}
}

// sampleNamespaceEndorsements creates one namespace-level endorsement set
// containing a single endorsement entry.
func sampleNamespaceEndorsements(mspID, sig string) *applicationpb.Endorsements {
	return &applicationpb.Endorsements{
		EndorsementsWithIdentity: []*applicationpb.EndorsementWithIdentity{
			{
				Identity: &msppb.Identity{
					MspId: mspID,
				},
				Endorsement: []byte(sig),
			},
		},
	}
}

// mustSerializedEndorsements serializes endorsements exactly the way
// unmarshalEndorsementsFromProposalResponse expects:
//
//  1. each *applicationpb.Endorsements is protobuf-marshaled into []byte
//  2. the slice of []byte values is JSON-marshaled
//
// This matches the production representation stored in
// ProposalResponse.Endorsement.Signature.
func mustSerializedEndorsements(t *testing.T, endorsements []*applicationpb.Endorsements) []byte {
	t.Helper()

	rawItems := make([][]byte, len(endorsements))
	for i, e := range endorsements {
		if e == nil {
			rawItems[i] = nil
			continue
		}

		raw, err := proto.Marshal(e)
		require.NoError(t, err)
		rawItems[i] = raw
	}

	out, err := json.Marshal(rawItems)
	require.NoError(t, err)
	return out
}
