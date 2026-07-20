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

func createValidEnvelope(tb testing.TB) *common.Envelope { //nolint:unparam
	tb.Helper()
	return createEnvelopeWithArgs(tb, [][]byte{[]byte("invoke"), []byte("arg1")})
}

// createEnvelopeWithArgs builds an envelope identical to createValidEnvelope but with
// a caller-controlled chaincode Args slice, so tests can craft the adversarial "zero
// arguments" case a malicious peer could send.
func createEnvelopeWithArgs(tb testing.TB, args [][]byte) *common.Envelope {
	tb.Helper()

	channelHeader := &common.ChannelHeader{
		Type:      int32(common.HeaderType_ENDORSER_TRANSACTION),
		TxId:      "txid",
		ChannelId: "channel",
	}
	chBytes, err := proto.Marshal(channelHeader)
	require.NoError(tb, err)

	signatureHeader := &common.SignatureHeader{
		Creator: []byte("creator"),
		Nonce:   []byte("nonce"),
	}
	shBytes, err := proto.Marshal(signatureHeader)
	require.NoError(tb, err)

	ccAction := &peer.ChaincodeAction{
		Results: []byte("results"),
	}
	ccActionBytes, err := proto.Marshal(ccAction)
	require.NoError(tb, err)

	prp := &peer.ProposalResponsePayload{
		Extension: ccActionBytes,
	}
	prpBytes, err := proto.Marshal(prp)
	require.NoError(tb, err)

	cis := &peer.ChaincodeInvocationSpec{
		ChaincodeSpec: &peer.ChaincodeSpec{
			ChaincodeId: &peer.ChaincodeID{
				Name:    "mycc",
				Version: "1.0",
			},
			Input: &peer.ChaincodeInput{
				Args: args,
			},
		},
	}
	cisBytes, err := proto.Marshal(cis)
	require.NoError(tb, err)

	cpp := &peer.ChaincodeProposalPayload{
		Input: cisBytes,
	}
	cppBytes, err := proto.Marshal(cpp)
	require.NoError(tb, err)

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
	require.NoError(tb, err)

	txAction := &peer.TransactionAction{
		Payload: capBytes,
	}
	tx := &peer.Transaction{
		Actions: []*peer.TransactionAction{txAction},
	}
	txBytes, err := proto.Marshal(tx)
	require.NoError(tb, err)

	payload := &common.Payload{
		Header: &common.Header{
			ChannelHeader:   chBytes,
			SignatureHeader: shBytes,
		},
		Data: txBytes,
	}
	payloadBytes, err := proto.Marshal(payload)
	require.NoError(tb, err)

	return &common.Envelope{
		Payload: payloadBytes,
	}
}

func TestEnvelope(t *testing.T) {
	t.Parallel()
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

// TestUnpackEnvelopePayload_EmptyArgsReturnsError demonstrates that
// UnpackEnvelopePayload now rejects a zero-argument ChaincodeInvocationSpec with an
// error instead of panicking on the unchecked `cis.ChaincodeSpec.Input.Args[0]` index
// (envelope.go).
//
// This is directly responder-reachable: both platform/fabric/services/endorser/flow.go
// and platform/fabric/services/state/transaction.go's receiveTransactionView.Call read a
// raw []byte payload off an inbound P2P session and pass it straight into
// NewTransactionFromEnvelopeBytes / SetFromEnvelopeBytes. A remote peer sending an
// envelope whose ChaincodeInput has an empty Args slice must get a rejected
// transaction, not a crashed responder goroutine.
func TestUnpackEnvelopePayload_EmptyArgsReturnsError(t *testing.T) {
	t.Parallel()

	env := createEnvelopeWithArgs(t, [][]byte{})
	payloadBytes := env.Payload

	_, _, err := transaction.UnpackEnvelopePayload(payloadBytes)
	require.Error(t, err, "UnpackEnvelopePayload must reject a zero-argument ChaincodeInvocationSpec")
}

// TestTransaction_SetFromEnvelopeBytesReturnsErrorOnEmptyChaincodeArgs proves the fix is
// reachable through the exact production call chain used by the responder views:
// Transaction.SetFromEnvelopeBytes -> transaction.NewEnvelopeFromEnv/UnpackEnvelopePayload.
func TestTransaction_SetFromEnvelopeBytesReturnsErrorOnEmptyChaincodeArgs(t *testing.T) {
	t.Parallel()

	env := createEnvelopeWithArgs(t, [][]byte{})
	envBytes, err := proto.Marshal(env)
	require.NoError(t, err)

	tx := &transaction.Transaction{}
	err = tx.SetFromEnvelopeBytes(envBytes)
	require.Error(t, err, "SetFromEnvelopeBytes must reject a zero-argument ChaincodeInvocationSpec")
}

// TestUnpackEnvelopePayload_NilHeaderReturnsError demonstrates the fix for a
// nil-pointer-dereference bug found via fuzzing (FuzzUnpackEnvelopeFromBytes):
// UnpackEnvelopePayload (envelope.go) now checks payl.Header for nil before
// dereferencing it, instead of panicking with "invalid memory address or nil pointer
// dereference" the way protoutil.UnmarshalPayload/proto.Unmarshal's zero-value
// *common.Payload (with a nil Header) used to trigger.
//
// Unlike the Args[0] bug, this requires no crafted ChaincodeInvocationSpec at all:
// an envelope whose Payload is empty (or whose payload bytes simply omit the Header
// field) is enough. It is reachable through the exact same responder call chain:
// platform/fabric/services/endorser/flow.go and
// platform/fabric/services/state/transaction.go's receiveTransactionView.Call feed
// raw, unvalidated bytes from an inbound P2P session into
// NewTransactionFromEnvelopeBytes -> SetFromEnvelopeBytes -> UnpackEnvelopeFromBytes
// -> UnpackEnvelopePayload.
func TestUnpackEnvelopePayload_NilHeaderReturnsError(t *testing.T) {
	t.Parallel()

	_, _, err := transaction.UnpackEnvelopeFromBytes([]byte{})
	require.Error(t, err, "UnpackEnvelopeFromBytes must reject a marshaled payload with no Header")

	// Same fix reachable via the exact production call chain.
	tx := &transaction.Transaction{}
	err = tx.SetFromEnvelopeBytes([]byte{})
	require.Error(t, err, "SetFromEnvelopeBytes must reject an envelope payload with no Header")
}

func TestEnvelope_Errors(t *testing.T) {
	t.Parallel()
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
