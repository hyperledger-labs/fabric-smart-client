/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/io"
)

// node is a comm node as io.SessionTwoParties sees it.
type node struct {
	*comm.HostNode
}

func (n node) ID() string      { return n.HostNode.ID }
func (n node) Address() string { return n.HostNode.Address }

func TestSessionTwoParties(t *testing.T) {
	t.Parallel()
	a, b := setupTwoNodes(t)
	io.SessionTwoParties(t, node{a}, node{b})
}
