/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestComputeTxID(t *testing.T) { //nolint:paralleltest
	creator := []byte("creator")
	nonce := []byte("nonce")

	// Test with provided nonce
	id := &driver.TxIDComponents{
		Nonce:   nonce,
		Creator: creator,
	}
	txid := transaction.ComputeTxID(id)
	require.NotEmpty(t, txid)

	// Test with generated nonce
	id2 := &driver.TxIDComponents{
		Creator: creator,
	}
	txid2 := transaction.ComputeTxID(id2)
	require.NotEmpty(t, txid2)
	require.NotEqual(t, txid, txid2)
	require.NotEmpty(t, id2.Nonce)
}

func TestGetRandomNonce(t *testing.T) { //nolint:paralleltest
	nonce, err := transaction.GetRandomNonce()
	require.NoError(t, err)
	require.Len(t, nonce, 24)

	nonce2, err := transaction.GetRandomNonce()
	require.NoError(t, err)
	require.Len(t, nonce2, 24)
	require.NotEqual(t, nonce, nonce2)
}
