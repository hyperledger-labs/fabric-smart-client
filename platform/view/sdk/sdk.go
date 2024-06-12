/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	comm2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver/file"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("view-sdk")

type Registry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type Startable interface {
	Start(ctx context.Context)
}

type Stoppable interface {
	Stop()
}

type WebServer interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
	Start() error
	Stop() error
}

type CommService interface {
	Start(ctx context.Context)
	Stop()
}

type GRPCServer interface {
}

type SDK struct {
	configService driver.ConfigService
	registry      Registry

	webServer WebServer

	grpcServer  *grpc2.GRPCServer
	viewService view2.Service
	viewManager Startable

	context          context.Context
	operationsSystem *operations.System

	commService CommService
}

func NewSDK(configService driver.ConfigService, registry Registry) *SDK {
	return &SDK{configService: configService, registry: registry}
}

func (p *SDK) Install() error {

	logger.Infof("View platform enabled, installing...")

	configProvider := p.configService
	assert.NoError(p.registry.RegisterService(configProvider), "failed registering config provider")

	assert.NoError(p.registry.RegisterService(crypto.NewProvider()))

	assert.NoError(p.registry.RegisterService(&events.Service{EventSystem: simple.NewEventBus()}))

	// KVS
	defaultKVS, err := kvs.New(p.registry, kvs.GetDriverNameFromConf(p.registry), "_default")
	if err != nil {
		return errors.Wrap(err, "failed creating kvs")
	}
	assert.NoError(p.registry.RegisterService(defaultKVS))

	// Sig Service
	des, err := sig.NewDeserializer()
	assert.NoError(err)
	assert.NoError(p.registry.RegisterService(des))
	signerService := sig.NewSignService(p.registry, des, defaultKVS)
	assert.NoError(p.registry.RegisterService(signerService))

	// Set Endpoint Service
	endpointService, err := endpoint.NewService(p.registry, nil, defaultKVS)
	assert.NoError(err, "failed instantiating endpoint service")
	assert.NoError(p.registry.RegisterService(endpointService), "failed registering endpoint service")

	kmsDriver, err := id.NewKMSDriver(configProvider)
	assert.NoError(err, "failed getting key management driver")
	idProvider, err := id.NewProvider(configProvider, signerService, endpointService, kmsDriver)
	assert.NoError(err)
	assert.NoError(p.registry.RegisterService(idProvider))

	operationsOptions, err := web.NewOperationsOptions(view.GetConfigService(p.registry))
	assert.NoError(err)
	logger := operations.NewOperationsLogger(operationsOptions.Logger)
	metricsProvider := operations.NewMetricsProvider(operationsOptions.Metrics, logger)
	assert.NoError(p.registry.RegisterService(metricsProvider))

	// Resolver service
	resolverService, err := endpoint.NewResolverService(configProvider, view.GetEndpointService(p.registry), idProvider)
	assert.NoError(err, "failed instantiating endpoint resolver service")
	assert.NoError(resolverService.LoadResolvers(), "failed loading resolvers")

	// View Service Server
	marshaller, err := view2.NewResponseMarshaler(hash.GetHasher(p.registry), driver.GetIdentityProvider(p.registry), view.GetSigService(p.registry))
	if err != nil {
		return errors.Errorf("error creating view service response marshaller: %s", err)
	}
	p.viewService, err = view2.NewViewServiceServer(
		marshaller,
		view2.NewAccessControlChecker(
			idProvider,
			view.GetSigService(p.registry),
		),
		view2.NewMetrics(metrics.GetProvider(p.registry)),
	)
	if err != nil {
		return errors.Errorf("error creating view service server: %s", err)
	}
	if err := p.registry.RegisterService(p.viewService); err != nil {
		return err
	}

	p.initCommLayer()
	// View Manager
	viewManager := manager.New(p.registry, manager.GetCommLayer(p.registry), driver.GetEndpointService(p.registry), driver.GetIdentityProvider(p.registry))
	if err := p.registry.RegisterService(viewManager); err != nil {
		return err
	}
	p.viewManager = viewManager
	p.webServer = web.NewServer(view.GetConfigService(p.registry), view.GetManager(p.registry))

	p.operationsSystem = operations.NewOperationSystem(p.webServer, logger, metricsProvider, *operationsOptions)
	assert.NoError(p.registry.RegisterService(p.operationsSystem), "failed initializing web server endpoints and metrics")

	if err := p.installTracing(); err != nil {
		return errors.WithMessage(err, "failed installing tracing")
	}

	finality.InstallHandler(p.registry, p.viewService)

	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	p.context = ctx

	assert.NoError(p.initGRPCServer(), "failed initializing grpc server")
	assert.NoError(p.startCommLayer(), "failed starting comm layer")
	assert.NoError(p.registerViewServiceServer(), "failed registering view service server")
	assert.NoError(p.startViewManager(), "failed starting view manager")

	return p.serve()
}

func (p *SDK) registerViewServiceServer() error {
	if p.grpcServer == nil {
		return nil
	}

	// Register the ViewService server
	protos2.RegisterViewServiceServer(p.grpcServer.Server(), p.viewService)

	return nil
}

func (p *SDK) initGRPCServer() error {
	grpcServer, err := web.NewGRPCServer(view.GetConfigService(p.registry))
	assert.NoError(err, "failed creating grpc server")
	p.grpcServer = grpcServer
	return nil
}

func (p *SDK) initCommLayer() {
	config := driver.GetConfigService(p.registry)
	endpointService := driver.GetEndpointService(p.registry).(*endpoint.Service)
	hostProvider, err := provider.NewHostProvider(config, endpointService, metrics.GetProvider(p.registry))
	assert.NoError(err, "failed creating host provider")
	commService, err := comm2.NewService(
		hostProvider,
		view.GetEndpointService(p.registry),
		view.GetConfigService(p.registry),
		view.GetIdentityProvider(p.registry).DefaultIdentity(),
	)
	assert.NoError(err, "failed instantiating the communication service")
	assert.NoError(p.registry.RegisterService(commService), "failed registering communication service")

	p.commService = commService
}

func (p *SDK) startCommLayer() error {
	p.commService.Start(p.context)

	return nil
}

func (p *SDK) startViewManager() error {
	view2.InstallViewHandler(view.GetManager(p.registry), p.viewService)
	go p.viewManager.Start(p.context)

	return nil
}

func (p *SDK) serve() error {
	web.Serve(p.grpcServer, p.webServer, p.operationsSystem, kvs.GetService(p.registry), p.context)
	return nil
}

func (p *SDK) installTracing() error {
	tracingProvider, err := metrics2.NewTracingProvider(view.GetConfigService(p.registry))
	if err != nil {
		return err
	}
	return p.registry.RegisterService(tracingProvider)
}
