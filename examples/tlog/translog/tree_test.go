/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package translog_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/transparency-dev/merkle/proof"
	"github.com/transparency-dev/merkle/rfc6962"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
)

// testLeaves returns n distinct leaf byte slices.
func testLeaves(n int) [][]byte {
	leaves := make([][]byte, n)
	for i := range leaves {
		leaves[i] = []byte(fmt.Sprintf("leaf-%d", i))
	}
	return leaves
}

func TestTreeProofsVerify(t *testing.T) {
	t.Parallel()
	const maxSize = 9
	leaves := testLeaves(maxSize)
	tree, err := translog.NewTree(leaves)
	require.NoError(t, err)

	for n := uint64(1); n <= maxSize; n++ {
		rootN, err := tree.Root(n)
		require.NoError(t, err)

		for i := uint64(0); i < n; i++ {
			p, err := tree.InclusionProof(i, n)
			require.NoError(t, err)
			leafHash := rfc6962.DefaultHasher.HashLeaf(leaves[i])
			require.NoError(t, proof.VerifyInclusion(rfc6962.DefaultHasher, i, n, leafHash, p, rootN))
		}

		for m := uint64(1); m <= n; m++ {
			rootM, err := tree.Root(m)
			require.NoError(t, err)
			cp, err := tree.ConsistencyProof(m, n)
			require.NoError(t, err)
			require.NoError(t, proof.VerifyConsistency(rfc6962.DefaultHasher, m, n, cp, rootM, rootN))
		}
	}
}

func TestTreeRootMatchesNewTree(t *testing.T) {
	t.Parallel()
	leaves := testLeaves(9)

	fromNewTree, err := translog.NewTree(leaves)
	require.NoError(t, err)

	appended := &translog.Tree{}
	for _, leaf := range leaves {
		require.NoError(t, appended.Append(leaf))

		want, err := fromNewTree.Root(appended.Size())
		require.NoError(t, err)
		got, err := appended.Root(appended.Size())
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestTreeRejectsOutOfRange(t *testing.T) {
	t.Parallel()
	tree, err := translog.NewTree(testLeaves(5))
	require.NoError(t, err)

	_, err = tree.InclusionProof(5, 5)
	require.Error(t, err)

	_, err = tree.InclusionProof(0, 6)
	require.Error(t, err)

	_, err = tree.Root(6)
	require.Error(t, err)

	_, err = tree.ConsistencyProof(4, 3)
	require.Error(t, err)
}

func TestTreeConsistencyFromEmpty(t *testing.T) {
	t.Parallel()
	tree, err := translog.NewTree(testLeaves(5))
	require.NoError(t, err)

	for n := uint64(1); n <= tree.Size(); n++ {
		rootN, err := tree.Root(n)
		require.NoError(t, err)

		p, err := tree.ConsistencyProof(0, n)
		require.NoError(t, err)
		require.Empty(t, p)
		require.NoError(t, proof.VerifyConsistency(rfc6962.DefaultHasher, 0, n, p, nil, rootN))
	}

	_, err = tree.ConsistencyProof(0, tree.Size()+1)
	require.Error(t, err)
}
