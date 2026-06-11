/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
)

func TestUnpackEnvelopeFromBytesAndEnvelope(t *testing.T) {
	t.Parallel()

	results := []byte("rwset-bytes")
	env, _, chdr, creator, function, args := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, results)
	raw := mustMarshalProto(t, env)

	upe, err := UnpackEnvelopeFromBytes("test-network", raw)
	require.NoError(t, err)
	require.Equal(t, "test-network", upe.Network())
	require.Equal(t, chdr.ChannelId, upe.Channel())
	require.Equal(t, chdr.TxId, upe.ID())
	require.Equal(t, "mycc", upe.ChaincodeName)
	require.Equal(t, "v1", upe.ChaincodeVersion)
	require.Equal(t, creator, upe.Creator)
	require.Equal(t, results, upe.Results)
	fn, parsedArgs := upe.FunctionAndParameters()
	require.Equal(t, function, fn)
	require.Equal(t, args, parsedArgs)
	require.Len(t, upe.ProposalResponses, 1)

	upe2, err := UnpackEnvelope("test-network", env)
	require.NoError(t, err)
	require.Equal(t, upe.TxID, upe2.TxID)
	require.Equal(t, upe.NetworkID, upe2.NetworkID)
}

func TestUnpackEnvelopeErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid envelope bytes", func(t *testing.T) {
		t.Parallel()
		_, err := UnpackEnvelopeFromBytes("test-network", []byte("not-proto"))
		require.Error(t, err)
	})

	t.Run("invalid envelope payload", func(t *testing.T) {
		t.Parallel()
		env := &cb.Envelope{Payload: []byte("not-payload")}
		_, err := UnpackEnvelope("test-network", env)
		require.Error(t, err)
	})

	t.Run("wrong header type", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_CONFIG, []byte("rwset"))
		_, err := UnpackEnvelopeFromPayloadAndCHHeader("test-network", payl, chdr)
		require.Error(t, err)
		require.ErrorContains(t, err, "only EndorserClient Transactions are supported")
	})

	t.Run("invalid signature header bytes", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		payl.Header.SignatureHeader = []byte("bad-signature-header")
		_, err := UnpackEnvelopeFromPayloadAndCHHeader("test-network", payl, chdr)
		require.Error(t, err)
	})

	t.Run("invalid transaction bytes", func(t *testing.T) {
		t.Parallel()
		_, payl, chdr, _, _, _ := buildTestEnvelope(t, cb.HeaderType_ENDORSER_TRANSACTION, []byte("rwset"))
		payl.Data = []byte("bad-transaction")
		_, err := UnpackEnvelopeFromPayloadAndCHHeader("test-network", payl, chdr)
		require.Error(t, err)
	})
}
