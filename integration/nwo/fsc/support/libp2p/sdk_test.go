/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/libp2p"
)

// TestCommTypeMatchesHost ties fsc.LibP2P, which nwo writes to fsc.p2p.type, to the value
// CustomHostProvider selects the libp2p host on.
func TestCommTypeMatchesHost(t *testing.T) {
	t.Parallel()
	require.Equal(t, libp2p.P2PCommunicationType, fsc.LibP2P)
}
