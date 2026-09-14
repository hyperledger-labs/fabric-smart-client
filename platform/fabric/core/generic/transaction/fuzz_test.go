/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"encoding/json"
	"testing"

	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
)

// This file fuzzes the byte-deserialization entry points that a remote, untrusted
// peer can reach directly: platform/fabric/services/endorser/flow.go's
// receiveTransactionView.Call and platform/fabric/services/state/transaction.go's
// receiveTransactionView.Call both read a raw []byte payload off an inbound P2P
// session and hand it, unvalidated, to Builder.NewTransactionFromBytes /
// NewTransactionFromEnvelopeBytes, which bottom out in the functions fuzzed here.
//
// Two panics that used to be reachable here are now fixed and covered by dedicated
// regression tests, so their inputs are seeded below as ordinary (non-panicking)
// corpus entries:
//   - empty ChaincodeInput.Args -> now returns an error instead of panicking on
//     Args[0], see TestUnpackSignedProposal_EmptyArgsReturnsError,
//     TestTransaction_SetFromBytesReturnsErrorOnEmptyChaincodeArgs,
//     TestUnpackEnvelopePayload_EmptyArgsReturnsError,
//     TestTransaction_SetFromEnvelopeBytesReturnsErrorOnEmptyChaincodeArgs.
//   - empty/header-less envelope payload -> now returns an error instead of a nil
//     pointer dereference in UnpackEnvelopePayload, see
//     TestUnpackEnvelopePayload_NilHeaderReturnsError.
//
// Running with `go test -fuzz <name>` (see the analysis report) continues to explore
// further malformed/adversarial mutations looking for other unrecovered panics.

// FuzzUnpackSignedProposal fuzzes transaction.UnpackSignedProposal's ProposalBytes
// field with arbitrary bytes - this is the payload embedded in every
// TSignedProposal that reaches SetFromBytes from raw wire bytes.
func FuzzUnpackSignedProposal(f *testing.F) {
	validSP := createValidSignedProposal(f)
	emptyArgsSP := createSignedProposalWithArgs(f, [][]byte{})

	f.Add(validSP.ProposalBytes)
	f.Add(emptyArgsSP.ProposalBytes)
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("not a protobuf message"))

	f.Fuzz(func(t *testing.T, proposalBytes []byte) {
		up, err := transaction.UnpackSignedProposal(&pb.SignedProposal{
			ProposalBytes: proposalBytes,
			Signature:     []byte("signature"),
		})
		if err == nil {
			// UnpackSignedProposal is documented to return no zero-ed fields on success.
			require.NotNil(t, up.Input)
			require.NotEmpty(t, up.Input.Args)
			require.NotNil(t, up.ChannelHeader)
			require.NotNil(t, up.SignatureHeader)
		}
	})
}

// FuzzUnpackEnvelopeFromBytes fuzzes transaction.UnpackEnvelopeFromBytes with
// arbitrary bytes - this is exactly the raw []byte a responder passes to
// Transaction.SetFromEnvelopeBytes, which in turn is exactly the payload a remote
// peer controls end to end via receiveTransactionView.Call.
func FuzzUnpackEnvelopeFromBytes(f *testing.F) {
	validEnv := createValidEnvelope(f)

	validEnvBytes, err := proto.Marshal(validEnv)
	require.NoError(f, err)

	emptyArgsEnv := createEnvelopeWithArgs(f, [][]byte{})
	emptyArgsEnvBytes, err := proto.Marshal(emptyArgsEnv)
	require.NoError(f, err)

	f.Add(validEnvBytes)
	f.Add(emptyArgsEnvBytes)
	f.Add([]byte("not a protobuf message"))
	f.Add([]byte(nil))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, raw []byte) {
		upe, _, err := transaction.UnpackEnvelopeFromBytes(raw)
		if err == nil {
			// UnpackEnvelopeFromBytes gives the same no-zero-ed-fields guarantee as
			// UnpackSignedProposal.
			require.NotNil(t, upe.Input)
			require.NotEmpty(t, upe.Input.Args)
			require.NotNil(t, upe.ChannelHeader)
			require.NotNil(t, upe.SignatureHeader)
		}
	})
}

// FuzzTransactionSetFromBytes fuzzes Transaction.SetFromBytes with arbitrary bytes,
// exercising the exact production call chain used by receiveTransactionView.Call ->
// Builder.NewTransactionFromBytes -> Manager.NewTransactionFromBytes ->
// Transaction.SetFromBytes(txRaw.Raw).
func FuzzTransactionSetFromBytes(f *testing.F) {
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)
	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)

	validTx := &transaction.Transaction{TSignedProposal: createValidSignedProposal(f)}
	validRaw, err := json.Marshal(validTx)
	require.NoError(f, err)

	emptyArgsTx := &transaction.Transaction{TSignedProposal: createSignedProposalWithArgs(f, [][]byte{})}
	emptyArgsRaw, err := json.Marshal(emptyArgsTx)
	require.NoError(f, err)

	f.Add(validRaw)
	f.Add(emptyArgsRaw)
	f.Add([]byte(nil))
	f.Add([]byte(""))
	f.Add([]byte("{}"))
	f.Add([]byte("not json at all"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		tx, err := factory.NewTransaction(t.Context(), "channel", nil, nil, "", nil)
		require.NoError(t, err)
		_ = tx.SetFromBytes(raw)
	})
}

// FuzzTransactionSetFromEnvelopeBytes fuzzes Transaction.SetFromEnvelopeBytes with
// arbitrary bytes, exercising the exact production call chain used by
// receiveTransactionView.Call -> Builder.NewTransactionFromEnvelopeBytes ->
// Manager.NewTransactionFromEnvelopeBytes -> Transaction.SetFromEnvelopeBytes(raw).
func FuzzTransactionSetFromEnvelopeBytes(f *testing.F) {
	mockChannelProvider := &mock.ChannelProvider{}
	mockSigService := &mock.SignerService{}
	mockChannel := &mock.Channel{}
	mockChannelProvider.ChannelReturns(mockChannel, nil)
	factory := transaction.NewEndorserTransactionFactory("network", mockChannelProvider, mockSigService)

	validEnv := createValidEnvelope(f)

	validEnvBytes, err := proto.Marshal(validEnv)
	require.NoError(f, err)

	emptyArgsEnv := createEnvelopeWithArgs(f, [][]byte{})
	emptyArgsEnvBytes, err := proto.Marshal(emptyArgsEnv)
	require.NoError(f, err)

	f.Add(validEnvBytes)
	f.Add(emptyArgsEnvBytes)
	f.Add([]byte("not a protobuf message"))
	f.Add([]byte(nil))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, raw []byte) {
		tx, err := factory.NewTransaction(t.Context(), "channel", nil, nil, "", nil)
		require.NoError(t, err)
		_ = tx.SetFromEnvelopeBytes(raw)
	})
}
