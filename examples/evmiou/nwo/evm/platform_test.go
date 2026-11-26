/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package evm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/examples/evmiou/evm/client"
)

func TestAddressFor(t *testing.T) {
	t.Parallel()

	addr, err := client.HexToAddress("0x00112233445566778899aabbccddeeff0011223a")
	require.NoError(t, err)
	addresses := map[string]client.Address{"approver1": addr}

	t.Run("known node", func(t *testing.T) {
		t.Parallel()

		got, err := addressFor(addresses, "approver1")
		require.NoError(t, err)
		require.Equal(t, addr, got)
	})

	t.Run("unknown node", func(t *testing.T) {
		t.Parallel()

		_, err := addressFor(addresses, "approver2")
		require.Error(t, err)
		require.ErrorContains(t, err, "approver2")
	})
}
