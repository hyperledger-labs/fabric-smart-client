/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

var logger = logging.MustGetLogger()

// ServiceDiscovery resolves the ID of a peer to one of its addresses, or the empty address if
// it knows none.
type ServiceDiscovery interface {
	Lookup(id host.PeerID) host.PeerIPAddress
}

// IDRouter returns all known addresses of a peer. The boolean reports whether
// the peer was found.
type IDRouter interface {
	Lookup(id host.PeerID) ([]host.PeerIPAddress, bool)
}
