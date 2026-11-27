/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/iou/contract"
)

var (
	borrower = client.Address{1}
	lender   = client.Address{2}
)

func TestValidateCreate(t *testing.T) {
	t.Parallel()

	ok := contract.Create{ID: [32]byte{7}, Amount: 10, Borrower: borrower, Lender: lender}
	require.NoError(t, ValidateCreate(ok, contract.IOU{}))

	low := ok
	low.Amount = 4
	require.Error(t, ValidateCreate(low, contract.IOU{}))

	same := ok
	same.Lender = borrower
	require.Error(t, ValidateCreate(same, contract.IOU{}))

	require.ErrorContains(t, ValidateCreate(ok, contract.IOU{Version: 1}), "already exists")
}

func TestValidateUpdate(t *testing.T) {
	t.Parallel()

	stored := contract.IOU{Amount: 10, Borrower: borrower, Lender: lender, Version: 1}
	update := func(amount, version uint64) contract.Update {
		return contract.Update{ID: [32]byte{7}, Amount: amount, Version: version}
	}

	require.NoError(t, ValidateUpdate(stored, update(5, 1)))
	require.Error(t, ValidateUpdate(stored, update(10, 1)))
	require.Error(t, ValidateUpdate(stored, update(11, 1)))
	require.Error(t, ValidateUpdate(stored, update(5, 2)))
	require.Error(t, ValidateUpdate(contract.IOU{}, update(5, 0)))
}
