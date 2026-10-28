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

// ServiceDiscovery is the interface that resolves the IP addresses given the ID of a peer
type ServiceDiscovery interface {
	LookupAll(id host.PeerID) ([]host.PeerIPAddress, bool)
	Lookup(id host.PeerID) host.PeerIPAddress
}

// IDRouter returns all known addresses of a peer. The boolean reports whether
// the peer was found.
type IDRouter interface {
	Lookup(id host.PeerID) ([]host.PeerIPAddress, bool)
}
