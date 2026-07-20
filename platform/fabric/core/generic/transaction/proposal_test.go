/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
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

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Name:    "mycc",
				Version: "1.0",
			},
			Input: &pb.ChaincodeInput{
				Args: args,
			},
		},
	}
	cisBytes, err := proto.Marshal(cis)
	require.NoError(tb, err)

	cpp := &pb.ChaincodeProposalPayload{
		Input: cisBytes,
	}
	cppBytes, err := proto.Marshal(cpp)
	require.NoError(tb, err)

	nonce := []byte("nonce")
	creatorBytes := mustMarshal(tb, &mspPb.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("creator")})
	txID := protoutil.ComputeTxID(nonce, creatorBytes)

	channelHeader := &cb.ChannelHeader{
		Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
		TxId:      txID,
		ChannelId: "channel",
		Extension: mustMarshal(tb, &pb.ChaincodeHeaderExtension{
			ChaincodeId: &pb.ChaincodeID{
				Name:    "mycc",
				Version: "1.0",
			},
		}),
	}
	chBytes, err := proto.Marshal(channelHeader)
	require.NoError(tb, err)

	signatureHeader := &cb.SignatureHeader{
		Creator: creatorBytes,
		Nonce:   nonce,
	}
	shBytes, err := proto.Marshal(signatureHeader)
	require.NoError(tb, err)

	header := &cb.Header{
		ChannelHeader:   chBytes,
		SignatureHeader: shBytes,
	}
	headerBytes, err := proto.Marshal(header)
	require.NoError(tb, err)

	proposal := &pb.Proposal{
		Header:  headerBytes,
		Payload: cppBytes,
	}
	proposalBytes, err := proto.Marshal(proposal)
	require.NoError(tb, err)

	return &pb.SignedProposal{
		ProposalBytes: proposalBytes,
		Signature:     []byte("signature"),
	}
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
