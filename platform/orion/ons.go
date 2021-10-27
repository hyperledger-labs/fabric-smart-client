/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
)

// NetworkService models a Orion Network
type NetworkService struct {
	SP   view2.ServiceProvider
	ons  driver.OrionNetworkService
	name string
}

// Name of this network
func (n *NetworkService) Name() string {
	return n.name
}

func (n *NetworkService) IdentityManager() *IdentityManager {
	return &IdentityManager{n.ons.IdentityManager()}
}

func (n *NetworkService) SessionManager() *SessionManager {
	return &SessionManager{n.ons.SessionManager()}
}

func GetOrionNetworkNames(sp view2.ServiceProvider) []string {
	return core.GetOrionNetworkServiceProvider(sp).Names()
}

// GetOrionNetworkService returns the Orion Network Service for the passed id, nil if not found
func GetOrionNetworkService(sp view2.ServiceProvider, id string) *NetworkService {
	fns, err := core.GetOrionNetworkServiceProvider(sp).OrionNetworkService(id)
	if err != nil {
		return nil
	}
	return &NetworkService{name: fns.Name(), SP: sp, ons: fns}
}

// GetDefaultONS returns the default Orion Network Service
func GetDefaultONS(sp view2.ServiceProvider) *NetworkService {
	return GetOrionNetworkService(sp, "")
}
