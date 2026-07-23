/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fns

import (
	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
)

// NewProvider is the dig constructor for *core.FSNProvider. Drivers is populated from every
// value provided into the "fabric-platform-drivers" dig group, and Validators from every value
// provided into the "fabric-network-config-validators" dig group; either group may be empty.
func NewProvider(in struct {
	dig.In
	ConfigService core.DynamicConfigService
	Drivers       []core.NamedDriver            `group:"fabric-platform-drivers"`
	Validators    []core.NetworkConfigValidator `group:"fabric-network-config-validators"`
},
) (*core.FSNProvider, error) {
	return core.NewFabricNetworkServiceProvider(in.ConfigService, in.Drivers, in.Validators)
}
