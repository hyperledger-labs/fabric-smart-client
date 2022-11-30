/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.driver")

type Driver struct{}

func (d *Driver) New(sp view.ServiceProvider, network string, defaultNetwork bool) (driver.FabricNetworkService, error) {
	logger.Debugf("creating new fabric network service for network [%s]", network)
	// bridge services
	c, err := config.New(view.GetConfigService(sp), network, defaultNetwork)
	if err != nil {
		return nil, err
	}
	sigService := generic.NewSigService(sp)

	// Endpoint service
	resolverService, err := endpoint.NewResolverService(
		c,
		view.GetEndpointService(sp),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric endpoint resolver")
	}
	if err := resolverService.LoadResolvers(); err != nil {
		return nil, errors.Wrap(err, "failed loading fabric endpoint resolvers")
	}
	endpointService, err := generic.NewEndpointResolver(resolverService, view.GetEndpointService(sp))
	if err != nil {
		return nil, errors.Wrap(err, "failed loading endpoint service")
	}

	// Local MSP Manager
	mspService := msp.NewLocalMSPManager(
		sp,
		c,
		sigService,
		view.GetEndpointService(sp),
		view.GetIdentityProvider(sp).DefaultIdentity(),
		c.MSPCacheSize(),
	)
	if err := mspService.Load(); err != nil {
		return nil, errors.Wrap(err, "failed loading local msp service")
	}

	// Identity Manager
	idProvider, err := id.NewProvider(endpointService)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating id provider")
	}

	// New Network
	net, err := generic.NewNetwork(
		sp,
		network,
		c,
		idProvider,
		mspService,
		sigService,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric service provider")
	}

	return net, nil
}

func init() {
	core.Register("fabric", &Driver{})
}
