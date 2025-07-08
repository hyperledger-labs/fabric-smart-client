/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"

	"github.com/go-kit/log"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver/file"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	postgres2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	server2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"go.uber.org/dig"
)

var logger = logging.MustGetLogger()

type SDK struct {
	dig2.SDK
}

func NewSDKFromContainer(c dig2.Container, registry services.Registry) *SDK {
	configService := config.GetProvider(registry)
	return NewSDKFrom(dig2.NewBaseSDK(c, configService), registry)
}

func NewSDK(registry services.Registry) *SDK {
	return NewSDKFromContainer(NewContainer(), registry)
}

func NewSDKFrom(baseSDK dig2.SDK, registry services.Registry) *SDK {
	sdk := &SDK{SDK: baseSDK}
	err := errors.Join(
		sdk.Container().Provide(func() services.Registry { return registry }),
		sdk.Container().Provide(
			digutils.Identity[services.Registry](),
			dig.As(new(services.Provider), new(services.Registry)),
		),
		sdk.Container().Provide(func() *config.Provider { return config.GetProvider(registry) }),
		sdk.Container().Provide(
			digutils.Identity[*config.Provider](),
			dig.As(new(driver.ConfigService), new(id.ConfigProvider), new(endpoint.ConfigService), new(dbdriver.Config)),
		),
	)
	if err != nil {
		panic(err)
	}
	return sdk
}

func (p *SDK) Install() error {
	err := errors.Join(
		// Hasher
		p.Container().Provide(hash.NewSHA256Provider, dig.As(new(hash.Hasher), new(server2.Hasher))),

		// Events
		p.Container().Provide(simple.NewEventBus, dig.As(new(events.EventSystem), new(events.Publisher), new(events.Subscriber))),
		p.Container().Provide(func(system events.EventSystem) *events.Service { return &events.Service{EventSystem: system} }),

		// Storage service
		p.Container().Provide(postgres2.NewDbProvider),
		p.Container().Provide(postgres2.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(sqlite2.NewDbProvider),
		p.Container().Provide(sqlite2.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(mem.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(newMultiplexedDriver),
		p.Container().Provide(binding.NewDefaultStore),
		p.Container().Provide(signerinfo.NewDefaultStore),
		p.Container().Provide(auditinfo.NewDefaultStore),

		// Endpoint Service
		p.Container().Provide(endpoint.NewService),
		p.Container().Provide(
			digutils.Identity[*endpoint.Service](),
			dig.As(new(comm.EndpointService), new(id.EndpointService), new(endpoint.Backend), new(view.EndpointService)),
		),
		p.Container().Provide(endpoint.NewResolversLoader),

		// Identity Service
		p.Container().Provide(id.NewProvider),
		p.Container().Provide(newKMSDriver),
		p.Container().Provide(newKVS),
		p.Container().Provide(file.NewDriver, dig.Group("kms-drivers")),
		p.Container().Provide(
			digutils.Identity[*id.Provider](),
			dig.As(new(endpoint.IdentityService), new(server2.IdentityProvider), new(view.IdentityProvider)),
		),

		// View Manager
		p.Container().Provide(view.NewRegistry),
		p.Container().Provide(view.NewManager),
		p.Container().Provide(
			digutils.Identity[*view.Manager](),
			dig.As(new(StartableViewManager), new(server2.ViewManager)),
		),

		// Comm service
		p.Container().Provide(provider.NewHostProvider),
		p.Container().Provide(func(
			hostProvider host.GeneratorProvider,
			configProvider driver.ConfigService,
			endpointService *endpoint.Service,
			tracerProvider tracing.Provider,
			metricsProvider metrics2.Provider,
		) (*comm.Service, error) {
			return comm.NewService(hostProvider, endpointService, configProvider, metricsProvider)
		}),
		p.Container().Provide(digutils.Identity[*comm.Service](), dig.As(new(view.CommLayer))),

		// Sig Service
		p.Container().Provide(sig.NewDeserializer),
		p.Container().Provide(sig.NewService),
		p.Container().Provide(
			digutils.Identity[*sig.Service](),
			dig.As(new(view.LocalIdentityChecker), new(server2.VerifierProvider), new(server2.SignerProvider), new(id.SigService)),
		),

		// Tracing
		p.Container().Provide(NewOperationsLogger),
		p.Container().Provide(digutils.Identity[operations.OperationsLogger](), dig.As(new(operations.Logger)), dig.As(new(log.Logger))),
		p.Container().Provide(NewOperationsOptions),
		p.Container().Provide(operations.NewOperationSystem),
		p.Container().Provide(func(metricsProvider metrics2.Provider, configService driver.ConfigService) (tracing.Provider, error) {
			base, err := tracing.NewProviderFromConfigService(configService)
			if err != nil {
				return nil, err
			}
			return tracing.NewProviderWithBackingProvider(base, metricsProvider), nil
		}),

		// Metrics
		p.Container().Provide(func(o *operations.Options, l operations.OperationsLogger) metrics2.Provider {
			return operations.NewMetricsProvider(o.Metrics, l, true)
		}),
		p.Container().Provide(server2.NewMetrics),

		// Web server
		p.Container().Provide(NewWebServer),
		p.Container().Provide(digutils.Identity[Server](), dig.As(new(operations.Server))),

		// GRPC server
		p.Container().Provide(server2.NewResponseMarshaler, dig.As(new(server2.Marshaller))),
		p.Container().Provide(server2.NewAccessControlChecker, dig.As(new(server2.PolicyChecker))),
		p.Container().Provide(server2.NewViewServiceServer, dig.As(new(server2.Service))),
		p.Container().Provide(NewGRPCServer),
	)
	if err != nil {
		return err
	}

	// Call the parent
	if err := p.SDK.Install(); err != nil {
		return err
	}

	// Register services in services.Registry
	err = errors.Join(
		digutils.Register[tracing.Provider](p.Container()),
		digutils.Register[*view.Manager](p.Container()),
		digutils.Register[*sig.Service](p.Container()),
		digutils.Register[*id.Provider](p.Container()),
		digutils.Register[*view.Registry](p.Container()),
		digutils.Register[*endpoint.Service](p.Container()),
		digutils.Register[*config.Provider](p.Container()),
	)
	if err != nil {
		return err
	}

	// Load endpoint service's resolvers
	if err := p.Container().Invoke(func(resolverService *endpoint.ResolversLoader) error { return resolverService.LoadResolvers() }); err != nil {
		return err
	}
	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	if err := p.SDK.Start(ctx); err != nil {
		return err
	}
	return p.Container().Invoke(func(in struct {
		dig.In
		GRPCServer     *grpc.GRPCServer
		ViewManager    StartableViewManager
		ViewService    server2.Service
		CommService    *comm.Service
		WebServer      Server
		System         *operations.System
		KVS            *kvs.KVS
		TracerProvider tracing.Provider
	}) error {
		if in.GRPCServer != nil {
			protos.RegisterViewServiceServer(in.GRPCServer.Server(), in.ViewService)
		}
		in.CommService.Start(ctx)

		server2.InstallViewHandler(in.ViewManager, in.ViewService, in.TracerProvider)
		go in.ViewManager.Start(ctx)

		Serve(in.GRPCServer, in.WebServer, in.System, in.KVS, ctx)

		return nil
	})
}

func (p *SDK) PostStart(ctx context.Context) error {
	defer logger.Debugf("Services installed:\n%s", p.Container().Visualize())
	return p.SDK.PostStart(ctx)
}
