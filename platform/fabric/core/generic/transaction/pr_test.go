/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction/mock"
)

func createValidProposalResponse(t *testing.T) *peer.ProposalResponse { //nolint:unparam
	t.Helper()

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

	return &peer.ProposalResponse{
		Payload: prpBytes,
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("signature"),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "OK",
		},
	}
}

func TestProposalResponse(t *testing.T) {
	t.Parallel()
	pr := createValidProposalResponse(t)

	// Test NewProposalResponseFromResponse
	tpr, err := transaction.NewProposalResponseFromResponse(pr)
	require.NoError(t, err)
	require.NotNil(t, tpr)

	require.Equal(t, []byte("endorser"), tpr.Endorser())
	require.Equal(t, []byte("signature"), tpr.EndorserSignature())
	require.Equal(t, pr.Payload, tpr.Payload())
	require.Equal(t, []byte("results"), tpr.Results())
	require.Equal(t, pr, tpr.PR())
	require.Equal(t, int32(200), tpr.ResponseStatus())
	require.Equal(t, "OK", tpr.ResponseMessage())

	b, err := tpr.Bytes()
	require.NoError(t, err)
	require.NotEmpty(t, b)

	// Test VerifyEndorsement
	mockVerifier := &mock.Verifier{}
	mockVerifier.VerifyReturns(nil)
	mockProvider := &mock.VerifierProvider{}
	mockProvider.GetVerifierReturns(mockVerifier, nil)

	err = tpr.VerifyEndorsement(mockProvider)
	require.NoError(t, err)
	require.Equal(t, 1, mockProvider.GetVerifierCallCount())
	require.Equal(t, []byte("endorser"), []byte(mockProvider.GetVerifierArgsForCall(0)))

	require.Equal(t, 1, mockVerifier.VerifyCallCount())
	expectedMsg := append(append([]byte{}, pr.Payload...), []byte("endorser")...)
	msg, sig := mockVerifier.VerifyArgsForCall(0)
	require.Equal(t, expectedMsg, msg)
	require.Equal(t, []byte("signature"), sig)

	// Test VerifyEndorsement Error
	mockProvider.GetVerifierReturns(nil, contextError("verifier error"))
	err = tpr.VerifyEndorsement(mockProvider)
	require.ErrorContains(t, err, "failed getting verifier")
}

func TestProposalResponse_Errors(t *testing.T) {
	t.Parallel()
	// Test empty payload
	pr := &peer.ProposalResponse{
		Payload: []byte("invalid payload"),
	}
	_, err := transaction.NewProposalResponseFromResponse(pr)
	require.ErrorContains(t, err, "GetProposalResponsePayload error")

	// Test NewProposalResponseFromBytes error
	_, err = transaction.NewProposalResponseFromBytes([]byte("invalid bytes"))
	require.Error(t, err)

	// Valid bytes
	validPr := createValidProposalResponse(t)
	b, err := proto.Marshal(validPr)
	require.NoError(t, err)

	tpr, err := transaction.NewProposalResponseFromBytes(b)
	require.NoError(t, err)
	require.Equal(t, int32(200), tpr.ResponseStatus())

	// Test invalid extension
	prp := &peer.ProposalResponsePayload{
		Extension: []byte("invalid extension"),
	}
	prpBytes, err := proto.Marshal(prp)
	require.NoError(t, err)

	prInvalidExt := &peer.ProposalResponse{
		Payload: prpBytes,
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("signature"),
		},
		Response: &peer.Response{
			Status:  200,
			Message: "OK",
		},
	}
	_, err = transaction.NewProposalResponseFromResponse(prInvalidExt)
	require.Error(t, err)
}

// TestProposalResponseNilEndorsementAndResponse verifies that the accessors and
// VerifyEndorsement return zero values or a normal error, instead of
// panicking, when a well-formed proposal response omits Endorsement and/or
// Response.
func TestProposalResponseNilEndorsementAndResponse(t *testing.T) {
	t.Parallel()
	pr := createValidProposalResponse(t)
	pr.Endorsement = nil
	pr.Response = nil

	tpr, err := transaction.NewProposalResponseFromResponse(pr)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		require.Nil(t, tpr.Endorser())
		require.Nil(t, tpr.EndorserSignature())
		require.Equal(t, int32(0), tpr.ResponseStatus())
		require.Empty(t, tpr.ResponseMessage())
	})

	mockProvider := &mock.VerifierProvider{}
	mockProvider.GetVerifierReturns(nil, contextError("unknown identity"))

	var verifyErr error
	require.NotPanics(t, func() {
		verifyErr = tpr.VerifyEndorsement(mockProvider)
	})
	require.ErrorContains(t, verifyErr, "failed getting verifier")
	require.Equal(t, 1, mockProvider.GetVerifierCallCount())
}

// Helper to create errors
type contextError string

func (e contextError) Error() string { return string(e) }
