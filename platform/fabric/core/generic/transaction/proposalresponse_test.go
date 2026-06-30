/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
)

func TestProposalResponse_UnpackProposalResponse(t *testing.T) {
	t.Parallel()
	pr := createValidProposalResponse(t)

	upr, err := transaction.UnpackProposalResponse(pr)
	require.NoError(t, err)
	require.NotNil(t, upr)

	require.Equal(t, []byte("results"), upr.Results())
	require.Equal(t, pr, upr.ProposalResponse)
	require.NotNil(t, upr.ChaincodeAction)
}

func TestProposalResponse_UnpackProposalResponse_Errors(t *testing.T) {
	t.Parallel()
	pr := createValidProposalResponse(t)
	pr.Payload = []byte("invalid payload")

	_, err := transaction.UnpackProposalResponse(pr)
	require.Error(t, err)
}
