/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/driver/driver"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	metrics3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/pkg/errors"
)

type Driver struct{}

func newDriverProvider(sp view.ServiceProvider) (driver.Driver, error) {
	publisher, err := events.GetPublisher(sp)
	if err != nil {
		return nil, errors.Wrap(err, "could not find publisher")
	}
	subscriber, err := events.GetSubscriber(sp)
	if err != nil {
		return nil, errors.Wrap(err, "could not find subscriber")
	}
	tracer := tracing.Get(sp).GetTracer()
	committerProvider := generic.NewCommitterProvider(hash.GetHasher(sp), publisher, tracer)
	configProvider, err := driver4.NewConfigProvider(driver2.GetConfigService(sp))
	if err != nil {
		return nil, errors.Wrap(err, "could not create config provider")
	}
	endpointService := driver2.GetEndpointService(sp)
	identityProvider := driver4.NewIdentityProvider(configProvider, endpointService)
	channelProvider := generic.NewGenericChannelProvider(committerProvider, publisher, subscriber)
	return driver4.NewDriverProvider(
		configProvider,
		channelProvider,
		identityProvider,
		metrics3.GetProvider(sp),
		endpointService,
		generic.NewSigService(sp),
		map[string]driver3.IdentityLoader{},
	), nil
}

func (d *Driver) New(sp view.ServiceProvider, network string, defaultNetwork bool) (driver.FabricNetworkService, error) {
	p, err := newDriverProvider(sp)
	if err != nil {
		return nil, err
	}
	return p.New(sp, network, defaultNetwork)
}

func init() {
	core.Register("generic", &Driver{})
}
