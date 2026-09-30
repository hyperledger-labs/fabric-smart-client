/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"errors"
	"fmt"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	mspPb "github.com/hyperledger/fabric-protos-go-apiv2/msp"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
)

func createValidSignedProposal(tb testing.TB) *pb.SignedProposal { //nolint:unparam
	tb.Helper()
	return createSignedProposalWithArgs(tb, [][]byte{[]byte("invoke"), []byte("arg1")})
}

// createSignedProposalWithArgs builds a signed proposal identical to
// createValidSignedProposal but with a caller-controlled Args slice, so tests can
// craft the adversarial "zero arguments" case a malicious peer could send.
func createSignedProposalWithArgs(tb testing.TB, args [][]byte) *pb.SignedProposal {
	tb.Helper()
	return buildSignedProposal(tb, func(m *proposalMsgs) {
		m.cis.ChaincodeSpec.Input.Args = args
	})
}

// proposalMsgs holds the layers of a signed chaincode proposal.
type proposalMsgs struct {
	cis      *pb.ChaincodeInvocationSpec
	cpp      *pb.ChaincodeProposalPayload
	ext      *pb.ChaincodeHeaderExtension
	chdr     *cb.ChannelHeader
	shdr     *cb.SignatureHeader
	header   *cb.Header
	proposal *pb.Proposal
}

// buildSignedProposal marshals a valid signed proposal. Each layer is marshaled into
// the byte field of its parent unless mutate already set that field, so a test can
// replace or drop a single layer.
func buildSignedProposal(tb testing.TB, mutate func(*proposalMsgs)) *pb.SignedProposal {
	tb.Helper()
	nonce := []byte("nonce")
	creator := mustMarshal(tb, &mspPb.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("creator")})
	m := &proposalMsgs{
		cis: &pb.ChaincodeInvocationSpec{ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{Name: "mycc", Version: "1.0"},
			Input:       &pb.ChaincodeInput{Args: [][]byte{[]byte("invoke"), []byte("arg1")}},
		}},
		cpp: &pb.ChaincodeProposalPayload{},
		ext: &pb.ChaincodeHeaderExtension{ChaincodeId: &pb.ChaincodeID{Name: "mycc", Version: "1.0"}},
		chdr: &cb.ChannelHeader{
			Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
			TxId:      protoutil.ComputeTxID(nonce, creator),
			ChannelId: "channel",
		},
		shdr:     &cb.SignatureHeader{Creator: creator, Nonce: nonce},
		header:   &cb.Header{},
		proposal: &pb.Proposal{},
	}
	if mutate != nil {
		mutate(m)
	}
	fillBytes(tb, &m.cpp.Input, m.cis)
	fillBytes(tb, &m.proposal.Payload, m.cpp)
	fillBytes(tb, &m.chdr.Extension, m.ext)
	fillBytes(tb, &m.header.ChannelHeader, m.chdr)
	fillBytes(tb, &m.header.SignatureHeader, m.shdr)
	fillBytes(tb, &m.proposal.Header, m.header)
	return &pb.SignedProposal{ProposalBytes: mustMarshal(tb, m.proposal), Signature: []byte("signature")}
}

func mustMarshal(tb testing.TB, msg proto.Message) []byte {
	tb.Helper()
	b, err := proto.Marshal(msg)
	require.NoError(tb, err)
	return b
}

func TestUnpackSignedProposal(t *testing.T) {
	t.Parallel()
	sp := createValidSignedProposal(t)

	up, err := transaction.UnpackSignedProposal(sp)
	require.NoError(t, err)
	require.NotNil(t, up)

	require.Equal(t, "channel", up.ChannelID())
	require.NotEmpty(t, up.TxID())
	require.Equal(t, []byte("nonce"), up.Nonce())
	require.NotEmpty(t, up.ProposalHash)
}

// TestUnpackSignedProposal_EmptyArgsReturnsError demonstrates that UnpackProposal now
// rejects a zero-argument ChaincodeInvocationSpec with an error, instead of succeeding
// and leaving every consumer that reconstructs a transaction from raw bytes to index
// Args[0] without a length check:
//   - Transaction.SetFromBytes (transaction.go): t.TFunction = string(up.Input.Args[0])
//   - UnpackEnvelopePayload (envelope.go): Function: string(cis.ChaincodeSpec.Input.Args[0])
//
// Both of these are reached directly from raw, attacker-controlled bytes coming off
// the wire: platform/fabric/services/endorser/flow.go's receiveTransactionView.Call
// and platform/fabric/services/state/transaction.go's receiveTransactionView.Call
// both read a []byte payload from a P2P session and pass it straight into
// NewTransactionFromBytes. A remote peer sending a proposal/transaction payload whose
// ChaincodeInput has an empty Args slice must get a rejected transaction, not a
// crashed responder goroutine.
func TestUnpackSignedProposal_EmptyArgsReturnsError(t *testing.T) {
	t.Parallel()

	sp := createSignedProposalWithArgs(t, [][]byte{})

	up, err := transaction.UnpackSignedProposal(sp)
	require.Error(t, err, "UnpackSignedProposal must reject a zero-argument ChaincodeInvocationSpec")
	require.Nil(t, up)
}

func TestUnpackSignedProposal_Validate(t *testing.T) {
	t.Parallel()
	sp := createValidSignedProposal(t)

	up, err := transaction.UnpackSignedProposal(sp)
	require.NoError(t, err)

	mockIdentity := &mock.Identity{}
	mockIdentity.ValidateReturns(nil)
	mockIdentity.VerifyReturns(nil)
	mockIdentity.GetMSPIdentifierReturns("Org1MSP")

	mockDeserializer := &mock.IdentityDeserializer{}
	mockDeserializer.DeserializeIdentityReturns(mockIdentity, nil)

	err = up.Validate(mockDeserializer)
	require.NoError(t, err)

	// Error path: txid mismatch
	up.ChannelHeader.TxId = "wrong"
	err = up.Validate(mockDeserializer)
	require.Error(t, err)

	up2, _ := transaction.UnpackSignedProposal(sp)
	mockIdentity.VerifyReturns(fmt.Errorf("verify failed"))
	err = up2.Validate(mockDeserializer)
	require.Error(t, err)

	up3, _ := transaction.UnpackSignedProposal(sp)
	mockDeserializer.DeserializeIdentityReturns(nil, fmt.Errorf("deserialize failed"))
	err = up3.Validate(mockDeserializer)
	require.Error(t, err)
}

func TestUnpackSignedProposal_MalformedInput(t *testing.T) {
	t.Parallel()
	malformed := []byte("invalid")
	tests := []struct {
		name    string
		mutate  func(*proposalMsgs)
		wantErr string
	}{
		{
			name:    "bad header",
			mutate:  func(m *proposalMsgs) { m.proposal.Header = malformed },
			wantErr: "error unmarshalling Header",
		},
		{
			name:    "bad channel header",
			mutate:  func(m *proposalMsgs) { m.header.ChannelHeader = malformed },
			wantErr: "error unmarshalling ChannelHeader",
		},
		{
			name:    "bad signature header",
			mutate:  func(m *proposalMsgs) { m.header.SignatureHeader = malformed },
			wantErr: "error unmarshalling SignatureHeader",
		},
		{
			name:    "bad header extension",
			mutate:  func(m *proposalMsgs) { m.chdr.Extension = malformed },
			wantErr: "error unmarshalling ChaincodeHeaderExtension",
		},
		{
			name:    "nil chaincode id",
			mutate:  func(m *proposalMsgs) { m.ext.ChaincodeId = nil },
			wantErr: "ChaincodeHeaderExtension.ChaincodeId is nil",
		},
		{
			name:    "empty chaincode name",
			mutate:  func(m *proposalMsgs) { m.ext.ChaincodeId.Name = "" },
			wantErr: "ChaincodeHeaderExtension.ChaincodeId.Name is empty",
		},
		{
			name:    "bad chaincode proposal payload",
			mutate:  func(m *proposalMsgs) { m.proposal.Payload = malformed },
			wantErr: "error unmarshalling ChaincodeProposalPayload",
		},
		{
			name:    "bad invocation spec",
			mutate:  func(m *proposalMsgs) { m.cpp.Input = malformed },
			wantErr: "error unmarshalling ChaincodeInvocationSpec",
		},
		{
			name:    "nil chaincode spec",
			mutate:  func(m *proposalMsgs) { m.cis.ChaincodeSpec = nil },
			wantErr: "chaincode invocation spec did not contain chaincode spec",
		},
		{
			name:    "nil input",
			mutate:  func(m *proposalMsgs) { m.cis.ChaincodeSpec.Input = nil },
			wantErr: "chaincode input did not contain any input",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sp := buildSignedProposal(t, tc.mutate)

			var (
				up  *transaction.UnpackedProposal
				err error
			)
			require.NotPanics(t, func() { up, err = transaction.UnpackSignedProposal(sp) })
			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, up)
		})
	}
}

func TestUnpackedProposal_ValidateRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		mutate       func(*transaction.UnpackedProposal)
		validateErr  error
		wantErr      string
		wantDeserial int
	}{
		{
			name:         "config header type is accepted",
			mutate:       func(up *transaction.UnpackedProposal) { up.ChannelHeader.Type = int32(cb.HeaderType_CONFIG) },
			wantDeserial: 1,
		},
		{
			name:    "other header type",
			mutate:  func(up *transaction.UnpackedProposal) { up.ChannelHeader.Type = int32(cb.HeaderType_MESSAGE) },
			wantErr: "invalid header type MESSAGE",
		},
		{
			name:    "non-zero epoch",
			mutate:  func(up *transaction.UnpackedProposal) { up.ChannelHeader.Epoch = 1 },
			wantErr: "epoch is non-zero",
		},
		{
			name:    "empty nonce",
			mutate:  func(up *transaction.UnpackedProposal) { up.SignatureHeader.Nonce = nil },
			wantErr: "nonce is empty",
		},
		{
			name:    "empty creator",
			mutate:  func(up *transaction.UnpackedProposal) { up.SignatureHeader.Creator = nil },
			wantErr: "creator is empty",
		},
		{
			name:    "nil proposal bytes",
			mutate:  func(up *transaction.UnpackedProposal) { up.SignedProposal.ProposalBytes = nil },
			wantErr: "empty proposal bytes",
		},
		{
			name:    "nil signature",
			mutate:  func(up *transaction.UnpackedProposal) { up.SignedProposal.Signature = nil },
			wantErr: "empty signature bytes",
		},
		{
			name: "creator is not a serialized identity",
			mutate: func(up *transaction.UnpackedProposal) {
				up.SignatureHeader.Creator = []byte("invalid")
				up.ChannelHeader.TxId = protoutil.ComputeTxID(up.SignatureHeader.Nonce, up.SignatureHeader.Creator)
			},
			wantErr: "access denied: channel [channel] creator org unknown, creator is malformed",
		},
		{
			name:         "invalid identity",
			validateErr:  errors.New("expired"),
			wantErr:      "access denied: channel [channel] creator org [Org1MSP]",
			wantDeserial: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up, err := transaction.UnpackSignedProposal(createValidSignedProposal(t))
			require.NoError(t, err)
			if tc.mutate != nil {
				tc.mutate(up)
			}
			identity := &mock.Identity{}
			identity.ValidateReturns(tc.validateErr)
			deserializer := &mock.IdentityDeserializer{}
			deserializer.DeserializeIdentityReturns(identity, nil)

			err = up.Validate(deserializer)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
			require.Equal(t, tc.wantDeserial, deserializer.DeserializeIdentityCallCount())
		})
	}
}
