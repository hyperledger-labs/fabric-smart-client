/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
)

var logger = flogging.MustGetLogger("rest-p2p-routing")

// ServiceDiscovery is the interface that resolves the IP addresses given the ID of a peer
type ServiceDiscovery interface {
	LookupAll(id host2.PeerID) ([]host2.PeerIPAddress, bool)
	Lookup(id host2.PeerID) host2.PeerIPAddress
}

type IDRouter interface {
	Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool)
}

// LabelRouter is an interface to the service discovery based on the label of a peer
type LabelRouter interface {
	Lookup(label string) ([]host2.PeerIPAddress, bool)
}
