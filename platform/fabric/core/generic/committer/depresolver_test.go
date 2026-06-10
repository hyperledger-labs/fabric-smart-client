/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
)

func TestSerialDependencyResolverResolve(t *testing.T) {
	t.Parallel()

	txs := []CommitTx{
		{TxID: "tx1", Type: common.HeaderType_ENDORSER_TRANSACTION},
		{TxID: "tx2", Type: common.HeaderType_CONFIG},
	}

	resolver := NewSerialDependencyResolver()
	resolved := resolver.Resolve(txs)
	require.Len(t, resolved, 1)
	require.Len(t, resolved[0], 2)
	require.Equal(t, "tx1", resolved[0][0].TxID)
	require.Equal(t, "tx2", resolved[0][1].TxID)
}

func TestParallelDependencyResolverResolve(t *testing.T) {
	t.Parallel()

	txs := []CommitTx{
		{TxID: "tx1", Type: common.HeaderType_ENDORSER_TRANSACTION},
		{TxID: "tx2", Type: common.HeaderType_CONFIG},
		{TxID: "tx3", Type: common.HeaderType_ENDORSER_TRANSACTION},
	}

	resolver := NewParallelDependencyResolver()
	resolved := resolver.Resolve(txs)
	require.Len(t, resolved, 3)
	for i := range txs {
		require.Len(t, resolved[i], 1)
		require.Equal(t, txs[i].TxID, resolved[i][0].TxID)
	}
}
