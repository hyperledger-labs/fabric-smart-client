/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/transaction/mocks"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
)

//go:generate counterfeiter -o mocks/verifier_provider.go --fake-name FakeVerifierProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.VerifierProvider
//go:generate counterfeiter -o mocks/verifier.go --fake-name FakeVerifier github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Verifier

// TestNewProposalResponseFromResponse verifies that wrapping a protobuf proposal
// response preserves the underlying pointer.
func TestNewProposalResponseFromResponse(t *testing.T) {
	t.Parallel()
	rawPR := &peer.ProposalResponse{
		Payload: []byte("payload"),
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("signature"),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte("result"),
		},
	}

	pr, err := NewProposalResponseFromResponse(rawPR)
	require.NoError(t, err)
	require.Same(t, rawPR, pr.PR())
}

// TestNewProposalResponseFromBytes verifies the happy path for deserializing a
// protobuf proposal response from bytes.
func TestNewProposalResponseFromBytes(t *testing.T) {
	t.Parallel()
	rawPR := &peer.ProposalResponse{
		Payload: []byte("payload"),
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("signature"),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte("result"),
		},
	}

	raw, err := proto.Marshal(rawPR)
	require.NoError(t, err)

	pr, err := NewProposalResponseFromBytes(raw)
	require.NoError(t, err)
	require.True(t, proto.Equal(rawPR, pr.PR()))
}

// TestNewProposalResponseFromBytesInvalidProto verifies that invalid protobuf
// bytes are rejected.
func TestNewProposalResponseFromBytesInvalidProto(t *testing.T) {
	t.Parallel()
	_, err := NewProposalResponseFromBytes([]byte("not-a-protobuf"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal proposal response")
}

// TestProposalResponseAccessors verifies the simple getters.
func TestProposalResponseAccessors(t *testing.T) {
	t.Parallel()
	rawPR := &peer.ProposalResponse{
		Payload: []byte("payload"),
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("signature"),
		},
		Response: &peer.Response{
			Status:  201,
			Message: "accepted",
			Payload: []byte("result"),
		},
	}

	pr, err := NewProposalResponseFromResponse(rawPR)
	require.NoError(t, err)

	require.Equal(t, []byte("endorser"), pr.Endorser())
	require.Equal(t, []byte("payload"), pr.Payload())
	require.Equal(t, []byte("signature"), pr.EndorserSignature())
	require.Equal(t, []byte("payload"), pr.Results())
	require.Same(t, rawPR, pr.PR())
	require.Equal(t, int32(201), pr.ResponseStatus())
	require.Equal(t, "accepted", pr.ResponseMessage())
}

// TestProposalResponseBytes verifies that a wrapped proposal response can be
// marshaled back into protobuf bytes.
func TestProposalResponseBytes(t *testing.T) {
	t.Parallel()
	rawPR := &peer.ProposalResponse{
		Payload: []byte("payload"),
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("signature"),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte("result"),
		},
	}

	pr, err := NewProposalResponseFromResponse(rawPR)
	require.NoError(t, err)

	raw, err := pr.Bytes()
	require.NoError(t, err)

	decoded := &peer.ProposalResponse{}
	err = proto.Unmarshal(raw, decoded)
	require.NoError(t, err)
	require.True(t, proto.Equal(rawPR, decoded))
}

// TestVerifyEndorsement verifies the happy path:
// the verifier provider returns a verifier, the tx payload is valid, the number
// of endorsement sets matches the number of namespaces, and the verifier accepts
// each namespace signature.
func TestVerifyEndorsement(t *testing.T) {
	t.Parallel()
	txID := "tx1"
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser: mustSerializedIdentity(t, "Org1MSP"),
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
			}),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "ok",
			Payload: []byte(txID),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeVerifier := &mocks.FakeVerifier{}

	fakeProvider.GetVerifierReturns(fakeVerifier, nil)
	fakeVerifier.VerifyReturns(nil)

	err = pr.VerifyEndorsement(fakeProvider)
	require.NoError(t, err)

	require.Equal(t, 1, fakeProvider.GetVerifierCallCount())
	require.Equal(t, 1, fakeVerifier.VerifyCallCount())
}

// TestVerifyEndorsementGetVerifierFails verifies that verification fails when
// the provider cannot return a verifier for the endorser.
func TestVerifyEndorsementGetVerifierFails(t *testing.T) {
	t.Parallel()
	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: []byte("payload"),
		Endorsement: &peer.Endorsement{
			Endorser: mustSerializedIdentity(t, "Org1MSP"),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeProvider.GetVerifierReturns(nil, errors.New("boom"))

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "getting verifier")
}

// TestVerifyEndorsementInvalidPayload verifies that verification fails when the
// proposal response payload is not a valid serialized tx.
func TestVerifyEndorsementInvalidPayload(t *testing.T) {
	t.Parallel()
	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: []byte("not-a-valid-tx"),
		Endorsement: &peer.Endorsement{
			Endorser:  mustSerializedIdentity(t, "Org1MSP"),
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{}),
		},
		Response: &peer.Response{
			Payload: []byte("tx1"),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeVerifier := &mocks.FakeVerifier{}

	fakeProvider.GetVerifierReturns(fakeVerifier, nil)

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal proposal response payload")
}

// TestVerifyEndorsementInvalidSerializedEndorsements verifies that verification
// fails when the serialized endorsement container cannot be decoded.
func TestVerifyEndorsementInvalidSerializedEndorsements(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser:  mustSerializedIdentity(t, "Org1MSP"),
			Signature: []byte("not-json"),
		},
		Response: &peer.Response{
			Payload: []byte("tx1"),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeVerifier := &mocks.FakeVerifier{}

	fakeProvider.GetVerifierReturns(fakeVerifier, nil)

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal endorsement signatures")
}

// TestVerifyEndorsementNamespaceCountMismatch verifies that verification fails
// when the number of endorsement sets does not match the number of namespaces.
func TestVerifyEndorsementNamespaceCountMismatch(t *testing.T) {
	t.Parallel()
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser:  mustSerializedIdentity(t, "Org1MSP"),
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{}),
		},
		Response: &peer.Response{
			Payload: []byte("tx1"),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeVerifier := &mocks.FakeVerifier{}

	fakeProvider.GetVerifierReturns(fakeVerifier, nil)

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mismatch number of signatures and namespaces")
}

// TestVerifyEndorsementMissingNamespaceEndorsement verifies that verification
// fails when a namespace has no endorsement items.
func TestVerifyEndorsementMissingNamespaceEndorsement(t *testing.T) {
	t.Parallel()
	txID := "tx1"
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser: mustSerializedIdentity(t, "Org1MSP"),
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				{},
			}),
		},
		Response: &peer.Response{
			Payload: []byte(txID),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeVerifier := &mocks.FakeVerifier{}

	fakeProvider.GetVerifierReturns(fakeVerifier, nil)

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing endorsement")
}

// TestVerifyEndorsementInvalidNamespaceSignature verifies that verification
// fails when the verifier rejects the namespace signature.
func TestVerifyEndorsementInvalidNamespaceSignature(t *testing.T) {
	t.Parallel()
	txID := "tx1"
	tx := sampleTx("ns1", "key1", "value1")
	rawTx, err := proto.Marshal(tx)
	require.NoError(t, err)

	pr, err := NewProposalResponseFromResponse(&peer.ProposalResponse{
		Payload: rawTx,
		Endorsement: &peer.Endorsement{
			Endorser: mustSerializedIdentity(t, "Org1MSP"),
			Signature: mustSerializedEndorsements(t, []*applicationpb.Endorsements{
				sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
			}),
		},
		Response: &peer.Response{
			Payload: []byte(txID),
		},
	})
	require.NoError(t, err)

	fakeProvider := &mocks.FakeVerifierProvider{}
	fakeVerifier := &mocks.FakeVerifier{}

	fakeProvider.GetVerifierReturns(fakeVerifier, nil)
	fakeVerifier.VerifyReturns(errors.New("bad signature"))

	err = pr.VerifyEndorsement(fakeProvider)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid namespace signature")
}

// TestMarshalAndUnmarshalEndorsementsForProposalResponse verifies the happy path:
// endorsements are marshaled into the proposal-response representation and then
// unmarshaled back without losing content.
func TestMarshalAndUnmarshalEndorsementsForProposalResponse(t *testing.T) {
	t.Parallel()
	endorsements := []*applicationpb.Endorsements{
		sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		sampleNamespaceEndorsements("Org2MSP", "sig-org2"),
	}

	raw, err := marshalEndorsementsForProposalResponse(endorsements)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	result, err := unmarshalEndorsementsFromProposalResponse(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.True(t, proto.Equal(endorsements[0], result[0]))
	require.True(t, proto.Equal(endorsements[1], result[1]))
}

// TestMarshalEndorsementsForProposalResponseNilEntry verifies that a nil
// endorsement entry is serialized as a nil raw item in the JSON array.
func TestMarshalEndorsementsForProposalResponseNilEntry(t *testing.T) {
	t.Parallel()
	endorsements := []*applicationpb.Endorsements{
		sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		nil,
	}

	raw, err := marshalEndorsementsForProposalResponse(endorsements)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	var rawItems [][]byte
	err = json.Unmarshal(raw, &rawItems)
	require.NoError(t, err)
	require.Len(t, rawItems, 2)
	require.NotNil(t, rawItems[0])
	require.Nil(t, rawItems[1])
}

// TestUnmarshalEndorsementsFromProposalResponseNilEntry verifies that a nil
// serialized endorsement entry is converted into an empty endorsements object.
func TestUnmarshalEndorsementsFromProposalResponseNilEntry(t *testing.T) {
	t.Parallel()
	raw := mustSerializedEndorsements(t, []*applicationpb.Endorsements{
		sampleNamespaceEndorsements("Org1MSP", "sig-org1"),
		nil,
	})

	result, err := unmarshalEndorsementsFromProposalResponse(raw)
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.True(t, proto.Equal(sampleNamespaceEndorsements("Org1MSP", "sig-org1"), result[0]))
	require.NotNil(t, result[1])
	require.Empty(t, result[1].GetEndorsementsWithIdentity())
}

// TestUnmarshalEndorsementsFromProposalResponseEmptyBytes verifies that an
// empty raw protobuf entry is treated as an empty endorsements object.
func TestUnmarshalEndorsementsFromProposalResponseEmptyBytes(t *testing.T) {
	t.Parallel()
	rawItems := [][]byte{
		{},
	}

	raw, err := json.Marshal(rawItems)
	require.NoError(t, err)

	result, err := unmarshalEndorsementsFromProposalResponse(raw)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.NotNil(t, result[0])
	require.Empty(t, result[0].GetEndorsementsWithIdentity())
}

// TestUnmarshalEndorsementsFromProposalResponseInvalidJSON verifies that
// invalid JSON input is rejected.
func TestUnmarshalEndorsementsFromProposalResponseInvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := unmarshalEndorsementsFromProposalResponse([]byte("not-json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal serialized endorsements")
}

// TestUnmarshalEndorsementsFromProposalResponseInvalidProto verifies that
// invalid protobuf bytes inside the JSON array are rejected.
func TestUnmarshalEndorsementsFromProposalResponseInvalidProto(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal([][]byte{
		[]byte("not-a-protobuf"),
	})
	require.NoError(t, err)

	_, err = unmarshalEndorsementsFromProposalResponse(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal endorsement at index 0")
}

func mustSerializedIdentity(t *testing.T, mspID string) []byte {
	t.Helper()

	raw, err := proto.Marshal(&msp.SerializedIdentity{
		Mspid:   mspID,
		IdBytes: []byte("cert-bytes"),
	})
	require.NoError(t, err)
	return raw
}
