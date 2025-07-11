/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fns

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"go.uber.org/dig"
)

func NewProvider(in struct {
	dig.In
	ConfigService driver.ConfigService
	Drivers       []core.NamedDriver `group:"fabric-platform-drivers"`
}) (*core.FSNProvider, error) {
	return core.NewFabricNetworkServiceProvider(in.ConfigService, in.Drivers)
}
