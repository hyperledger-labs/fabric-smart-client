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

func createValidSignedProposal(t *testing.T) *pb.SignedProposal { //nolint:unparam
	t.Helper()

	cis := &pb.ChaincodeInvocationSpec{
		ChaincodeSpec: &pb.ChaincodeSpec{
			ChaincodeId: &pb.ChaincodeID{
				Name:    "mycc",
				Version: "1.0",
			},
			Input: &pb.ChaincodeInput{
				Args: [][]byte{[]byte("invoke"), []byte("arg1")},
			},
		},
	}
	cisBytes, err := proto.Marshal(cis)
	require.NoError(t, err)

	cpp := &pb.ChaincodeProposalPayload{
		Input: cisBytes,
	}
	cppBytes, err := proto.Marshal(cpp)
	require.NoError(t, err)

	nonce := []byte("nonce")
	creatorBytes := mustMarshal(t, &mspPb.SerializedIdentity{Mspid: "Org1MSP", IdBytes: []byte("creator")})
	txID := protoutil.ComputeTxID(nonce, creatorBytes)

	channelHeader := &cb.ChannelHeader{
		Type:      int32(cb.HeaderType_ENDORSER_TRANSACTION),
		TxId:      txID,
		ChannelId: "channel",
		Extension: mustMarshal(t, &pb.ChaincodeHeaderExtension{
			ChaincodeId: &pb.ChaincodeID{
				Name:    "mycc",
				Version: "1.0",
			},
		}),
	}
	chBytes, err := proto.Marshal(channelHeader)
	require.NoError(t, err)

	signatureHeader := &cb.SignatureHeader{
		Creator: creatorBytes,
		Nonce:   nonce,
	}
	shBytes, err := proto.Marshal(signatureHeader)
	require.NoError(t, err)

	header := &cb.Header{
		ChannelHeader:   chBytes,
		SignatureHeader: shBytes,
	}
	headerBytes, err := proto.Marshal(header)
	require.NoError(t, err)

	proposal := &pb.Proposal{
		Header:  headerBytes,
		Payload: cppBytes,
	}
	proposalBytes, err := proto.Marshal(proposal)
	require.NoError(t, err)

	return &pb.SignedProposal{
		ProposalBytes: proposalBytes,
		Signature:     []byte("signature"),
	}
}

func mustMarshal(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(msg)
	require.NoError(t, err)
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
