/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger/fabric-x-common/api/applicationpb"
	"github.com/stretchr/testify/require"
)

// TestMarshalAndUnmarshalEndorsementsForProposalResponse verifies the happy path:
// endorsements are marshaled into the proposal-response representation and then
// unmarshaled back without losing content.
func TestMarshalAndUnmarshalEndorsementsForProposalResponse(t *testing.T) {
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
	_, err := unmarshalEndorsementsFromProposalResponse([]byte("not-json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal serialized endorsements")
}

// TestUnmarshalEndorsementsFromProposalResponseInvalidProto verifies that
// invalid protobuf bytes inside the JSON array are rejected.
func TestUnmarshalEndorsementsFromProposalResponseInvalidProto(t *testing.T) {
	raw, err := json.Marshal([][]byte{
		[]byte("not-a-protobuf"),
	})
	require.NoError(t, err)

	_, err = unmarshalEndorsementsFromProposalResponse(raw)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal endorsement at index 0")
}
