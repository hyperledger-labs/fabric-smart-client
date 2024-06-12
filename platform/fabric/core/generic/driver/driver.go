/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driverprovider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
)

type Driver struct{}

func (d *Driver) New(sp view.ServiceProvider, network string, defaultNetwork bool) (driver.FabricNetworkService, error) {
	publisher, err := events.GetPublisher(sp)
	if err != nil {
		return nil, err
	}
	configProvider, err := config.NewProvider(view.GetConfigService(sp))
	if err != nil {
		return nil, err
	}
	deserialier := sig.NewMultiplexDeserializer()

	kvss := kvs.GetService(sp)
	return driverprovider.NewProvider(configProvider, generic.NewProvider(kvs.GetService(sp), publisher, hash.GetHasher(sp), tracing.Get(sp)), identity.NewProvider(configProvider, view.GetEndpointService(sp)), metrics.GetProvider(sp), view.GetEndpointService(sp), sig.NewService(deserialier, kvss), deserialier, view.GetIdentityProvider(sp), kvss).New(nil, network, false)
}

func init() {
	core.Register("generic", &Driver{})
}
