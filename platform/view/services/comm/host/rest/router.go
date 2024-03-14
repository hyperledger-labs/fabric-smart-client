/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type routing interface {
	Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool)
}

type endpointResolver interface {
	GetIdentity(endpoint string, pkID []byte) (view2.Identity, error)
	Resolve(party view2.Identity) (view2.Identity, map[driver.PortName]string, []byte, error)
}

type endpointServiceRouting struct {
	resolver endpointResolver
}

func (r *endpointServiceRouting) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	logger.Infof("Looking up endpoint of peer [%s]", id)
	identity, err := r.resolver.GetIdentity("", []byte(id))
	if err != nil {
		logger.Errorf("failed getting identity for peer [%s]", id)
		return []host2.PeerIPAddress{}, false
	}
	_, addresses, _, err := r.resolver.Resolve(identity)
	if err != nil {
		logger.Errorf("failed resolving [%s]: %v", id, err)
		return []host2.PeerIPAddress{}, false
	}
	if address, ok := addresses[driver.P2PPort]; ok {
		logger.Infof("Found endpoint of peer [%s]: [%s]", id, address)
		return []host2.PeerIPAddress{address}, true
	}
	logger.Infof("Did not find endpoint of peer [%s]", id)
	return []host2.PeerIPAddress{}, false
}

func convertAddress(addr string) string {
	parts := strings.Split(addr, "/")
	if len(parts) != 5 {
		panic("unexpected address found: " + addr)
	}
	return parts[2] + ":" + parts[4]
}
