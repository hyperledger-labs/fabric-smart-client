/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"fmt"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/id"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp"
	mspdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	metrics3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.driver")

type Driver struct{}

func (d *Driver) New(sp view.ServiceProvider, network string, defaultNetwork bool) (driver.FabricNetworkService, error) {
	logger.Debugf("creating new fabric network service for network [%s]", network)
	// bridge services
	configService, err := config.NewService(view.GetConfigService(sp), network, defaultNetwork)
	if err != nil {
		return nil, err
	}
	kvss := kvs.GetService(sp)

	deserialier := sig.NewMultiplexDeserializer()
	sigService := sig.NewService(deserialier, kvss)

	// Endpoint service
	resolverService, err := endpoint.NewResolverService(
		configService,
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
		configService,
		kvss,
		sigService,
		view.GetEndpointService(sp),
		view.GetIdentityProvider(sp).DefaultIdentity(),
		deserialier,
		configService.MSPCacheSize(),
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
	metrics := metrics2.NewMetrics(metrics3.GetProvider(sp))
	net, err := generic.NewNetwork(
		sp,
		network,
		configService,
		idProvider,
		mspService,
		sigService,
		metrics,
		generic.NewChannel,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed instantiating fabric service provider")
	}
	if err := net.Init(); err != nil {
		return nil, errors.Wrap(err, "failed to initialize fabric service provider")
	}

	return net, nil
}

func init() {
	core.Register("generic", &Driver{})
}

func DeserializerManager(sp view.ServiceProvider) mspdriver.DeserializerManager {
	dm, err := sp.GetService(reflect.TypeOf((*mspdriver.DeserializerManager)(nil)))
	if err != nil {
		panic(fmt.Sprintf("failed looking up deserializer manager [%s]", err))
	}
	return dm.(mspdriver.DeserializerManager)
}
