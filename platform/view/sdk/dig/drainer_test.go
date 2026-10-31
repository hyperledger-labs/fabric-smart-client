/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
)

// The view manager is the node.Drainer Node.Stop finds in the registry.
func TestViewManagerIsRegisteredDrainer(t *testing.T) {
	t.Parallel()
	var _ node.Drainer = (*view.Manager)(nil)
	registry := view.NewServiceProvider()
	m := &view.Manager{}
	require.NoError(t, registry.RegisterService(m))
	d, err := registry.GetService(new(node.Drainer))
	require.NoError(t, err)
	require.Same(t, m, d)
}

// The comm layer recognizes the refusals the P2P service sends while draining by this prefix.
func TestDrainingPrefixesMatch(t *testing.T) {
	t.Parallel()
	require.Equal(t, p2p.DrainingPrefix, comm.DrainingPrefix)
}
