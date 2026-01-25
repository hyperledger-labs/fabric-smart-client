/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package protoutil_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil/fakes"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
)

func TestGetPayloads(t *testing.T) {
	var txAction *pb.TransactionAction
	var err error

	// good
	ccActionBytes, _ := proto.Marshal(&pb.ChaincodeAction{
		Results: []byte("results"),
	})
	proposalResponsePayload := &pb.ProposalResponsePayload{
		Extension: ccActionBytes,
	}
	proposalResponseBytes, err := proto.Marshal(proposalResponsePayload)
	require.NoError(t, err)
	ccActionPayload := &pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	}
	ccActionPayloadBytes, _ := proto.Marshal(ccActionPayload)
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.NoError(t, err, "Unexpected error getting payload bytes")
	t.Logf("error1 [%s]", err)

	// nil proposal response extension
	proposalResponseBytes, err = proto.Marshal(&pb.ProposalResponsePayload{
		Extension: nil,
	})
	require.NoError(t, err)
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with nil proposal response extension")
	t.Logf("error2 [%s]", err)

	// malformed proposal response payload
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: []byte("bad payload"),
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with malformed proposal response payload")
	t.Logf("error3 [%s]", err)

	// malformed proposal response payload extension
	proposalResponseBytes, _ = proto.Marshal(&pb.ProposalResponsePayload{
		Extension: []byte("bad extension"),
	})
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with malformed proposal response extension")
	t.Logf("error4 [%s]", err)

	// nil proposal response payload extension
	proposalResponseBytes, _ = proto.Marshal(&pb.ProposalResponsePayload{
		ProposalHash: []byte("hash"),
	})
	ccActionPayloadBytes, _ = proto.Marshal(&pb.ChaincodeActionPayload{
		Action: &pb.ChaincodeEndorsedAction{
			ProposalResponsePayload: proposalResponseBytes,
		},
	})
	txAction = &pb.TransactionAction{
		Payload: ccActionPayloadBytes,
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with nil proposal response extension")
	t.Logf("error5 [%s]", err)

	// malformed transaction action payload
	txAction = &pb.TransactionAction{
		Payload: []byte("bad payload"),
	}
	_, _, err = protoutil.GetPayloads(txAction)
	require.Error(t, err, "Expected error with malformed transaction action payload")
	t.Logf("error6 [%s]", err)
}

func TestDeduplicateEndorsements(t *testing.T) {
	signID := &fakes.SignerSerializer{}
	signID.SerializeReturns([]byte("signer"), nil)
	signerBytes, err := signID.Serialize()
	require.NoError(t, err, "Unexpected error serializing signing identity")

	proposal := &pb.Proposal{
		Header: protoutil.MarshalOrPanic(&cb.Header{
			ChannelHeader: protoutil.MarshalOrPanic(&cb.ChannelHeader{
				Extension: protoutil.MarshalOrPanic(&pb.ChaincodeHeaderExtension{}),
			}),
			SignatureHeader: protoutil.MarshalOrPanic(&cb.SignatureHeader{
				Creator: signerBytes,
			}),
		}),
	}
	responses := []*pb.ProposalResponse{
		{Payload: []byte("payload"), Endorsement: &pb.Endorsement{Endorser: []byte{5, 4, 3}}, Response: &pb.Response{Status: int32(200)}},
		{Payload: []byte("payload"), Endorsement: &pb.Endorsement{Endorser: []byte{5, 4, 3}}, Response: &pb.Response{Status: int32(200)}},
	}

	transaction, err := protoutil.CreateSignedTx(proposal, signID, responses...)
	require.NoError(t, err)
	require.True(t, proto.Equal(transaction, transaction), "got: %#v, want: %#v", transaction, transaction)

	pl, err := protoutil.UnmarshalPayload(transaction.Payload)
	require.NoError(t, err)
	tx, err := protoutil.UnmarshalTransaction(pl.Data)
	require.NoError(t, err)
	ccap, err := protoutil.UnmarshalChaincodeActionPayload(tx.Actions[0].Payload)
	require.NoError(t, err)
	require.Len(t, ccap.Action.Endorsements, 1)
	require.Equal(t, []byte{5, 4, 3}, ccap.Action.Endorsements[0].Endorser)
}

func TestCreateSignedTx(t *testing.T) {
	var err error
	prop := &pb.Proposal{}

	signID := &fakes.SignerSerializer{}
	signID.SerializeReturns([]byte("signer"), nil)
	signerBytes, err := signID.Serialize()
	require.NoError(t, err, "Unexpected error serializing signing identity")

	ccHeaderExtensionBytes := protoutil.MarshalOrPanic(&pb.ChaincodeHeaderExtension{})
	chdrBytes := protoutil.MarshalOrPanic(&cb.ChannelHeader{
		Extension: ccHeaderExtensionBytes,
	})
	shdrBytes := protoutil.MarshalOrPanic(&cb.SignatureHeader{
		Creator: signerBytes,
	})
	responses := []*pb.ProposalResponse{{}}

	// malformed signature header
	headerBytes := protoutil.MarshalOrPanic(&cb.Header{
		SignatureHeader: []byte("bad signature header"),
	})
	prop.Header = headerBytes
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with malformed signature header")

	// set up the header bytes for the remaining tests
	headerBytes, _ = proto.Marshal(&cb.Header{
		ChannelHeader:   chdrBytes,
		SignatureHeader: shdrBytes,
	})
	prop.Header = headerBytes

	nonMatchingTests := []struct {
		responses     []*pb.ProposalResponse
		expectedError string
	}{
		// good response followed by bad response
		{
			[]*pb.ProposalResponse{
				{Payload: []byte("payload"), Response: &pb.Response{Status: int32(200)}},
				{Payload: []byte{}, Response: &pb.Response{Status: int32(500), Message: "failed to endorse"}},
			},
			"proposal response was not successful, error code 500, msg failed to endorse",
		},
		// bad response followed by good response
		{
			[]*pb.ProposalResponse{
				{Payload: []byte{}, Response: &pb.Response{Status: int32(500), Message: "failed to endorse"}},
				{Payload: []byte("payload"), Response: &pb.Response{Status: int32(200)}},
			},
			"proposal response was not successful, error code 500, msg failed to endorse",
		},
	}
	for i, nonMatchingTest := range nonMatchingTests {
		_, err = protoutil.CreateSignedTx(prop, signID, nonMatchingTest.responses...)
		require.EqualErrorf(t, err, nonMatchingTest.expectedError, "Expected non-matching response error '%v' for test %d", nonMatchingTest.expectedError, i)
	}

	// good responses, but different payloads
	responses = []*pb.ProposalResponse{
		{Payload: []byte("payload"), Response: &pb.Response{Status: int32(200)}},
		{Payload: []byte("payload2"), Response: &pb.Response{Status: int32(200)}},
	}
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	if err == nil || strings.HasPrefix(err.Error(), "ProposalResponsePayloads do not match (base64):") == false {
		require.FailNow(t, "Error is expected when response payloads do not match")
	}

	// no endorsement
	responses = []*pb.ProposalResponse{{
		Payload: []byte("payload"),
		Response: &pb.Response{
			Status: int32(200),
		},
	}}
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with no endorsements")

	// success
	responses = []*pb.ProposalResponse{{
		Payload:     []byte("payload"),
		Endorsement: &pb.Endorsement{},
		Response: &pb.Response{
			Status: int32(200),
		},
	}}
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.NoError(t, err, "Unexpected error creating signed transaction")
	t.Logf("error: [%s]", err)

	//
	//
	// additional failure cases
	prop = &pb.Proposal{}
	responses = []*pb.ProposalResponse{}
	// no proposal responses
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with no proposal responses")

	// missing proposal header
	responses = append(responses, &pb.ProposalResponse{})
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with no proposal header")

	// bad proposal payload
	prop.Payload = []byte("bad payload")
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with malformed proposal payload")

	// bad payload header
	prop.Header = []byte("bad header")
	_, err = protoutil.CreateSignedTx(prop, signID, responses...)
	require.Error(t, err, "Expected error with malformed proposal header")
}

func TestCreateSignedTxNoSigner(t *testing.T) {
	_, err := protoutil.CreateSignedTx(nil, nil, &pb.ProposalResponse{})
	require.ErrorContains(t, err, "signer is required when creating a signed transaction")
}

func TestCreateSignedTxStatus(t *testing.T) {
	serializedExtension, err := proto.Marshal(&pb.ChaincodeHeaderExtension{})
	require.NoError(t, err)
	serializedChannelHeader, err := proto.Marshal(&cb.ChannelHeader{
		Extension: serializedExtension,
	})
	require.NoError(t, err)

	signingID := &fakes.SignerSerializer{}
	signingID.SerializeReturns([]byte("signer"), nil)
	serializedSigningID, err := signingID.Serialize()
	require.NoError(t, err)
	serializedSignatureHeader, err := proto.Marshal(&cb.SignatureHeader{
		Creator: serializedSigningID,
	})
	require.NoError(t, err)

	header := &cb.Header{
		ChannelHeader:   serializedChannelHeader,
		SignatureHeader: serializedSignatureHeader,
	}

	serializedHeader, err := proto.Marshal(header)
	require.NoError(t, err)

	proposal := &pb.Proposal{
		Header: serializedHeader,
	}

	tests := []struct {
		status      int32
		expectedErr string
	}{
		{status: 0, expectedErr: "proposal response was not successful, error code 0, msg response-message"},
		{status: 199, expectedErr: "proposal response was not successful, error code 199, msg response-message"},
		{status: 200, expectedErr: ""},
		{status: 201, expectedErr: ""},
		{status: 399, expectedErr: ""},
		{status: 400, expectedErr: "proposal response was not successful, error code 400, msg response-message"},
	}
	for _, tc := range tests {
		t.Run(strconv.Itoa(int(tc.status)), func(t *testing.T) {
			response := &pb.ProposalResponse{
				Payload:     []byte("payload"),
				Endorsement: &pb.Endorsement{},
				Response: &pb.Response{
					Status:  tc.status,
					Message: "response-message",
				},
			}

			_, err := protoutil.CreateSignedTx(proposal, signingID, response)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedErr)
			}
		})
	}
}

func TestCreateSignedEnvelope(t *testing.T) {
	var env *cb.Envelope
	channelID := "mychannelID"
	msg := &cb.ConfigEnvelope{}

	id := &fakes.SignerSerializer{}
	id.SignReturnsOnCall(0, []byte("goodsig"), nil)
	id.SignReturnsOnCall(1, nil, errors.New("bad signature"))
	env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID,
		id, msg, int32(1), uint64(1))
	require.NoError(t, err, "Unexpected error creating signed envelope")
	require.NotNil(t, env, "Envelope should not be nil")
	// mock sign returns the bytes to be signed
	require.Equal(t, []byte("goodsig"), env.Signature, "Unexpected signature returned")
	payload := &cb.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	require.NoError(t, err, "Failed to unmarshal payload")
	data := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, data)
	require.NoError(t, err, "Expected payload data to be a config envelope")
	require.True(t, proto.Equal(msg, data), "Payload data does not match expected value")

	_, err = protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID,
		id, &cb.ConfigEnvelope{}, int32(1), uint64(1))
	require.Error(t, err, "Expected sign error")
}

func TestCreateSignedEnvelopeNilSigner(t *testing.T) {
	var env *cb.Envelope
	channelID := "mychannelID"
	msg := &cb.ConfigEnvelope{}

	env, err := protoutil.CreateSignedEnvelope(cb.HeaderType_CONFIG, channelID,
		nil, msg, int32(1), uint64(1))
	require.NoError(t, err, "Unexpected error creating signed envelope")
	require.NotNil(t, env, "Envelope should not be nil")
	require.Empty(t, env.Signature, "Signature should have been empty")
	payload := &cb.Payload{}
	err = proto.Unmarshal(env.Payload, payload)
	require.NoError(t, err, "Failed to unmarshal payload")
	data := &cb.ConfigEnvelope{}
	err = proto.Unmarshal(payload.Data, data)
	require.NoError(t, err, "Expected payload data to be a config envelope")
	require.True(t, proto.Equal(msg, data), "Payload data does not match expected value")
}

func TestGetSignedProposal(t *testing.T) {
	var signedProp *pb.SignedProposal
	var err error

	sig := []byte("signature")

	signID := &fakes.SignerSerializer{}
	signID.SignReturns(sig, nil)

	prop := &pb.Proposal{}
	propBytes, _ := proto.Marshal(prop)
	signedProp, err = protoutil.GetSignedProposal(prop, signID)
	require.NoError(t, err, "Unexpected error getting signed proposal")
	require.Equal(t, propBytes, signedProp.ProposalBytes,
		"Proposal bytes did not match expected value")
	require.Equal(t, sig, signedProp.Signature,
		"Signature did not match expected value")

	_, err = protoutil.GetSignedProposal(nil, signID)
	require.Error(t, err, "Expected error with nil proposal")
	_, err = protoutil.GetSignedProposal(prop, nil)
	require.Error(t, err, "Expected error with nil signing identity")
}

func TestGetBytesProposalPayloadForTx(t *testing.T) {
	input := &pb.ChaincodeProposalPayload{
		Input:        []byte("input"),
		TransientMap: make(map[string][]byte),
	}
	expected, _ := proto.Marshal(&pb.ChaincodeProposalPayload{
		Input: []byte("input"),
	})

	result, err := protoutil.GetBytesProposalPayloadForTx(input)
	require.NoError(t, err, "Unexpected error getting proposal payload")
	require.Equal(t, expected, result, "Payload does not match expected value")

	_, err = protoutil.GetBytesProposalPayloadForTx(nil)
	require.Error(t, err, "Expected error with nil proposal payload")
}
