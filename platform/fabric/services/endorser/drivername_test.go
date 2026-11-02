/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx"
)

// The endorser package skips the txID check for networks whose driver is named
// fabricxDriverName; it must match the name the fabricx driver registers.
func TestFabricxDriverNameMatchesFabricx(t *testing.T) {
	t.Parallel()
	require.Equal(t, fabricx.DriverName, endorser.FabricxDriverNameForTest)
}
