/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Subtests use t.Setenv, which panics if combined with t.Parallel(); none of them can run in
// parallel with each other either, since they all read the same process-wide environment.
func TestExternalFromEnv(t *testing.T) { //nolint:paralleltest // t.Setenv is incompatible with t.Parallel()
	t.Run("none set", func(t *testing.T) { //nolint:paralleltest // t.Setenv is incompatible with t.Parallel()
		external, ok, err := ExternalFromEnv()
		require.NoError(t, err)
		require.False(t, ok)
		require.Nil(t, external)
	})

	t.Run("all set", func(t *testing.T) { //nolint:paralleltest // t.Setenv is incompatible with t.Parallel()
		t.Setenv("EVM_ENDPOINT", "http://127.0.0.1:8545")
		t.Setenv("EVM_CHAIN_ID", "298")
		t.Setenv("EVM_FUNDER_KEY", "abcd")

		external, ok, err := ExternalFromEnv()
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, &External{
			Endpoint:  "http://127.0.0.1:8545",
			ChainID:   298,
			FunderKey: "abcd",
		}, external)
	})

	t.Run("partially set", func(t *testing.T) { //nolint:paralleltest // t.Setenv is incompatible with t.Parallel()
		t.Setenv("EVM_ENDPOINT", "http://127.0.0.1:8545")

		external, ok, err := ExternalFromEnv()
		require.Error(t, err)
		require.False(t, ok)
		require.Nil(t, external)
		require.ErrorContains(t, err, "EVM_CHAIN_ID")
		require.ErrorContains(t, err, "EVM_FUNDER_KEY")
	})
}
