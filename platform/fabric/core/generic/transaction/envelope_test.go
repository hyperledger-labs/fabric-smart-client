/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
)

func createValidEnvelope(t *testing.T) *common.Envelope { //nolint:unparam
	t.Helper()

	channelHeader := &common.ChannelHeader{
		Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
		TxId:      "txid",
		ChannelId: "channel",
	}
	chBytes, err := proto.Marshal(channelHeader)
	require.NoError(t, err)

	signatureHeader := &common.SignatureHeader{
		Creator: []byte("creator"),
		Nonce:   []byte("nonce"),
	}
	shBytes, err := proto.Marshal(signatureHeader)
	require.NoError(t, err)

	ccAction := &peer.ChaincodeAction{
		Results: []byte("results"),
	}
	ccActionBytes, err := proto.Marshal(ccAction)
	require.NoError(t, err)

	prp := &peer.ProposalResponsePayload{
		Extension: ccActionBytes,
	}
	prpBytes, err := proto.Marshal(prp)
	require.NoError(t, err)

	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{
				Name:    "mycc",
				Version: "1.0",
			},
			Input: &peer.ChaincodeInput{
				Args: [][]byte{[]byte("invoke"), []byte("arg1")},
			},
		},
	}
	cisBytes, err := proto.Marshal(cis)
	require.NoError(t, err)

	cpp := &peer.ChaincodeProposalPayload{
		Input: cisBytes,
	}
	cppBytes, err := proto.Marshal(cpp)
	require.NoError(t, err)

	cap := &peer.ChaincodeActionPayload{
		ChaincodeProposalPayload: cppBytes,
		Action: &peer.ChaincodeEndorsedAction{
			ProposalResponsePayload: prpBytes,
			Endorsements: []*peer.Endorsement{
				{Endorser: []byte("endorser1"), Signature: []byte("sig1")},
			},
		},
	}
	capBytes, err := proto.Marshal(cap)
	require.NoError(t, err)

	txAction := &peer.TransactionAction{
		Payload: capBytes,
	}
	tx := &peer.Transaction{
		Actions: []*peer.TransactionAction{txAction},
	}
	txBytes, err := proto.Marshal(tx)
	require.NoError(t, err)

	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader:   chBytes,
			SignatureHeader: shBytes,
		},
		Data: txBytes,
	}
	payloadBytes, err := proto.Marshal(payload)
	require.NoError(t, err)

	return &common.Envelope{
		Payload: payloadBytes,
	}
}

func TestEnvelope(t *testing.T) { //nolint:paralleltest
	env := createValidEnvelope(t)

	// Test NewEnvelopeFromEnv
	e, err := transaction.NewEnvelopeFromEnv(env)
	require.NoError(t, err)

	// Wait, UnpackEnvelopePayload was hit in other tests, but let's test invalid
	_, _, err = transaction.UnpackEnvelopePayload([]byte("invalid"))
	require.Error(t, err)

	_, err = transaction.GetChannelHeaderType([]byte("invalid"))
	require.Error(t, err)

	_, _, err = transaction.UnpackEnvelope(env)
	require.NoError(t, err)
	envStr := e.String()
	require.NotEmpty(t, envStr)

	require.NoError(t, err)
	require.NotNil(t, e)

	require.Equal(t, "txid", e.TxID())
	require.Equal(t, []byte("nonce"), e.Nonce())
	require.Equal(t, []byte("creator"), e.Creator())
	require.Equal(t, []byte("results"), e.Results())

	b, err := e.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	require.Equal(t, env, e.Envelope())

	s := e.String()
	require.NotEmpty(t, s)

	// Test UnpackEnvelopeFromBytes
	upe, ht, err := transaction.UnpackEnvelopeFromBytes(b)
	require.NoError(t, err)
	require.Equal(t, int32(common.HeaderType_ENDORSER_TRANSACTION), ht)
	require.NotNil(t, upe)

	require.Equal(t, "txid", upe.ID())
	require.Equal(t, "channel", upe.Channel())
	funcName, args := upe.FunctionAndParameters()
	require.Equal(t, "invoke", funcName)
	require.Equal(t, []string{"arg1"}, args)

	// Test GetChannelHeaderType
	htType, err := transaction.GetChannelHeaderType(b)
	require.NoError(t, err)
	require.Equal(t, common.HeaderType_ENDORSER_TRANSACTION, htType)

	// Test FromBytes
	e2 := transaction.NewEnvelope()
	err = e2.FromBytes(b)
	require.NoError(t, err)
	require.Equal(t, "txid", e2.TxID())
}

func TestEnvelope_Errors(t *testing.T) { //nolint:paralleltest
	e := transaction.NewEnvelope()
	err := e.FromBytes([]byte("invalid bytes"))
	require.Error(t, err)

	_, err = transaction.NewEnvelopeFromEnv(&common.Envelope{Payload: []byte("invalid payload")})
	require.Error(t, err)

	_, _, err = transaction.UnpackEnvelopeFromBytes([]byte("invalid envelope bytes"))
	require.Error(t, err)

	_, err = transaction.GetChannelHeaderType([]byte("invalid bytes"))
	require.Error(t, err)
}
