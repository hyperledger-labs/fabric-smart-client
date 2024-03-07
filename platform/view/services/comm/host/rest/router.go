/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"

type routing interface {
	Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool)
}

type mapRouter map[host2.PeerID][]host2.PeerIPAddress

func (r mapRouter) Lookup(id host2.PeerID) ([]host2.PeerIPAddress, bool) {
	addr, ok := r[id]
	return addr, ok
}

func (r mapRouter) reverseLookup(ipAddress host2.PeerIPAddress) (host2.PeerID, bool) {
	for id, addrs := range r {
		for _, addr := range addrs {
			if ipAddress == addr {
				return id, true
			}
		}
	}
	return "", false
}
