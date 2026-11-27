/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/tlog/translog"
)

func TestCosignDecision(t *testing.T) {
	t.Parallel()

	tree, err := translog.NewTree([][]byte{[]byte("a"), []byte("b"), []byte("c")})
	require.NoError(t, err)
	root3, err := tree.Root(3)
	require.NoError(t, err)
	root2, err := tree.Root(2)
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		size uint64
		root []byte
		want string
	}{
		{"same head", 3, root3, ""},
		{"other root", 3, root2, RefusedMismatch},
		{"ahead of the tree", 4, root3, RefusedBehind},
		{"older head", 2, root2, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := cosignDecision(tree, tc.size, tc.root)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
