/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestCheckLenderID(t *testing.T) {
	t.Parallel()

	ep, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)
	borrowerID, lenderID := view.Identity("borrower_id"), view.Identity("lender_id")
	_, err = ep.AddResolver("borrower", "", nil, nil, borrowerID)
	require.NoError(t, err)
	_, err = ep.AddResolver("lender", "", nil, nil, lenderID)
	require.NoError(t, err)

	require.NoError(t, checkLenderID(t.Context(), ep, borrowerID, lenderID))
	require.ErrorContains(t, checkLenderID(t.Context(), ep, borrowerID, view.Identity("fabricated")), "unknown")
	require.ErrorContains(t, checkLenderID(t.Context(), ep, borrowerID, borrowerID), "borrower's")
}
