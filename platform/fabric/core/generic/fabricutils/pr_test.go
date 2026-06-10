/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabricutils

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
)

func TestUnpackedProposalResponseResults(t *testing.T) {
	t.Parallel()

	upr := &UnpackedProposalResponse{
		ChaincodeAction: &peer.ChaincodeAction{
			Results: []byte("results"),
		},
	}

	require.Equal(t, []byte("results"), upr.Results())
}

func TestUnpackProposalResponse(t *testing.T) {
	t.Parallel()

	t.Run("invalid proposal response payload bytes", func(t *testing.T) {
		t.Parallel()
		_, err := UnpackProposalResponse([]byte("bad-proposal-response-payload"))
		require.ErrorContains(t, err, "error unmarshalling ProposalResponsePayload")
	})

	t.Run("invalid chaincode action extension", func(t *testing.T) {
		t.Parallel()
		prp := &peer.ProposalResponsePayload{
			Extension: []byte("bad-extension"),
		}

		_, err := UnpackProposalResponse(mustMarshalProto(t, prp))
		require.ErrorContains(t, err, "error unmarshalling ChaincodeAction")
	})

	t.Run("invalid rwset bytes", func(t *testing.T) {
		t.Parallel()
		chAction := &peer.ChaincodeAction{
			Results: []byte("bad-rwset"),
		}
		prp := &peer.ProposalResponsePayload{
			Extension: mustMarshalProto(t, chAction),
		}

		_, err := UnpackProposalResponse(mustMarshalProto(t, prp))
		require.ErrorContains(t, err, "cannot parse invalid wire-format data")
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		rwsetBytes := buildRWSetBytes(t, "k1", []byte("v1"))
		payload := buildProposalResponsePayload(t, rwsetBytes)

		upr, err := UnpackProposalResponse(payload)
		require.NoError(t, err)
		require.NotNil(t, upr)
		require.NotNil(t, upr.ChaincodeAction)
		require.NotNil(t, upr.TxRwSet)
		require.Equal(t, rwsetBytes, upr.Results())
	})
}
