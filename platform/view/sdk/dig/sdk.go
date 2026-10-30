/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	digendpoint "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/dig/support/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver/file"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	viewgrpcserver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/p2p"
)

var logger = logging.MustGetLogger()

type SDK struct {
	commondig.SDK
}

func NewSDKFromContainer(c commondig.Container, registry services.Registry) *SDK {
	configService := config.GetProvider(registry)
	return NewSDKFrom(commondig.NewBaseSDK(c, configService), registry)
}

func NewSDK(registry services.Registry) *SDK {
	return NewSDKFromContainer(NewContainer(), registry)
}

func NewSDKFrom(baseSDK commondig.SDK, registry services.Registry) *SDK {
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
			dig.As(new(driver.ConfigService), new(id.ConfigProvider), new(digendpoint.ConfigService), new(dbdriver.Config), new(p2p.ConfigService)),
		),
	)
	if err != nil {
		panic(err)
	}
	return sdk
}

func (p *SDK) Install() error {
	// Check the listener configuration before Start constructs the servers: NewGRPCServer
	// binds its listener during construction, and a configuration that does not mean what
	// it says must fail with nothing listening, with the check's own error rather than a
	// dependency-injection chain around it.
	if err := p.Container().Invoke(CheckTLSConfig); err != nil {
		return err
	}

	err := errors.Join(
		// Events
		p.Container().Provide(simple.NewEventBus, dig.As(new(events.EventSystem), new(events.Publisher), new(events.Subscriber))),

		// Storage service
		p.Container().Provide(postgres.NewDbProviderWithMetrics),
		p.Container().Provide(postgres.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(sqlite.NewDbProvider),
		p.Container().Provide(sqlite.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(mem.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(newMultiplexedDriver),
		p.Container().Provide(newKVS),
		p.Container().Provide(binding.NewDefaultStore),
		p.Container().Provide(signerinfo.NewDefaultStore),
		p.Container().Provide(auditinfo.NewDefaultStore),

		// Endpoint Service
		p.Container().Provide(endpoint.NewService),
		p.Container().Provide(
			digutils.Identity[*endpoint.Service](),
			dig.As(new(comm.EndpointService), new(id.EndpointService), new(digendpoint.Backend), new(view.EndpointService), new(p2p.EndpointService)),
		),
		p.Container().Provide(digendpoint.NewResolversLoader),

		// Identity Service
		p.Container().Provide(id.NewProvider),
		p.Container().Provide(newKMSDriver),
		p.Container().Provide(file.NewDriver, dig.Group("kms-drivers")),
		p.Container().Provide(
			digutils.Identity[*id.Provider](),
			dig.As(new(digendpoint.IdentityService), new(viewgrpcserver.IdentityProvider), new(view.IdentityProvider), new(p2p.IdentityProvider)),
		),

		// View Manager
		p.Container().Provide(view.NewRegistry),
		p.Container().Provide(view.NewMetrics),
		p.Container().Provide(view.NewContextFactory),
		p.Container().Provide(view.NewManager),
		p.Container().Provide(view.NewDefaultRunner),
		p.Container().Provide(
			digutils.Identity[*view.Manager](),
			dig.As(new(viewgrpcserver.ViewManager), new(p2p.ViewManager)),
		),
		p.Container().Provide(p2p.NewDefaultRunner),
		// A closure, because dig does not fill variadic parameters such as NewService's options.
		p.Container().Provide(func(
			viewManager p2p.ViewManager,
			identityProvider p2p.IdentityProvider,
			commLayer p2p.CommLayer,
			endpointService p2p.EndpointService,
			runner p2p.Runner,
			configService p2p.ConfigService,
			metricsProvider metrics.Provider,
		) *p2p.Service {
			return p2p.NewService(viewManager, identityProvider, commLayer, endpointService, runner, configService, p2p.WithMetrics(metricsProvider))
		}),

		// Comm service
		p.Container().Provide(provider.NewHostProvider),
		p.Container().Provide(func(
			hostProvider host.GeneratorProvider,
			endpointService *endpoint.Service,
			configService driver.ConfigService,
			metricsProvider metrics.Provider,
		) (*comm.Service, error) {
			return comm.NewService(hostProvider, endpointService, configService, metricsProvider)
		}),
		p.Container().Provide(digutils.Identity[*comm.Service](), dig.As(new(p2p.CommLayer), new(view.SessionFactory))),

		// Sig Service
		p.Container().Provide(sig.NewDeserializer),
		p.Container().Provide(sig.NewService),
		p.Container().Provide(
			digutils.Identity[*sig.Service](),
			dig.As(new(view.LocalIdentityChecker), new(viewgrpcserver.VerifierProvider), new(viewgrpcserver.SignerProvider), new(id.SigService)),
		),

		// Tracing
		p.Container().Provide(NewOperationsLogger),
		p.Container().Provide(digutils.Identity[operations.OperationsLogger](), dig.As(new(operations.Logger))),
		p.Container().Provide(NewOperationsOptions),
		p.Container().Provide(operations.NewOperationSystem),
		p.Container().Provide(newTracerProvider),
		// Metrics
		p.Container().Provide(func(o *operations.Options) metrics.Provider {
			return operations.NewMetricsProvider(o.Metrics)
		}),
		p.Container().Provide(viewgrpcserver.NewMetrics),
		p.Container().Provide(connstate.NewMetrics),

		// Listeners. A developer replaces one by decorating its constructor; the SDK calls the
		// constructor only when the listener is enabled.
		p.Container().Provide(func() WebServerConstructor { return NewWebServer }),
		p.Container().Provide(func() GRPCServerConstructor { return NewGRPCServer }),
		p.Container().Provide(enabledWebServer),
		p.Container().Provide(enabledGRPCServer),
		p.Container().Provide(NewOperationsServer),
		// The operations endpoints are served on their own listener when fsc.metrics.address
		// is set, and on the web listener otherwise.
		p.Container().Provide(func(ops OperationsServer) operations.Server { return ops }, dig.As(new(operations.Server))),

		// GRPC server
		p.Container().Provide(viewgrpcserver.NewResponseMarshaler, dig.As(new(viewgrpcserver.Marshaller))),
		p.Container().Provide(viewgrpcserver.NewAccessControlChecker, dig.As(new(viewgrpcserver.PolicyChecker))),
		p.Container().Provide(LoadRemoteConfig),
		p.Container().Provide(NewViewServiceServer, dig.As(new(viewgrpcserver.Service))),
		// Transports of the remote view API, mounted by Start as fsc.view.remote.backends lists.
		p.Container().Provide(newGRPCBackend, dig.Group(BackendsGroup)),
		p.Container().Provide(newWebBackend, dig.Group(BackendsGroup)),
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
		digutils.Register[*comm.Service](p.Container()),
		digutils.Register[*config.Provider](p.Container()),
		digutils.Register[*endpoint.Service](p.Container()),
		digutils.Register[*id.Provider](p.Container()),
		digutils.Register[*sig.Service](p.Container()),
		digutils.Register[tracing.Provider](p.Container()),
		digutils.Register[*view.Manager](p.Container()),
		digutils.Register[*view.Registry](p.Container()),
	)
	if err != nil {
		return err
	}

	// Load endpoint service's resolvers
	if err := p.Container().Invoke(func(resolverService *digendpoint.ResolversLoader) error { return resolverService.LoadResolvers() }); err != nil {
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
		GRPCServer       GRPCServer
		ViewManager      viewgrpcserver.ViewManager
		P2PService       *p2p.Service
		ViewService      viewgrpcserver.Service
		CommService      *comm.Service
		WebServer        WebServer
		OperationsServer OperationsServer
		System           *operations.System
		KVS              *kvs.KVS
		ConfigService    driver.ConfigService
		Remote           RemoteConfig
		ConnStates       *connstate.Metrics
		Backends         []Backend `group:"view-remote-backends"`
	},
	) (err error) {
		// The gRPC listener bound its address when it was built. Until Serve takes it over,
		// a failure here must release it, or a node that never serves keeps the port.
		if in.GRPCServer != nil {
			defer func() {
				if err != nil {
					in.GRPCServer.Stop()
				}
			}()
		}
		viewgrpcserver.InstallViewHandler(in.ViewManager, in.ViewService, RemoteViewTimeout(in.ConfigService))
		mountHealth(in.WebServer, in.ConnStates)
		if err := mountBackends(in.Remote, in.Backends, in.ViewService); err != nil {
			return err
		}
		in.CommService.Start(ctx)

		if err := in.P2PService.Start(ctx); err != nil {
			return err
		}

		Serve(in.GRPCServer, in.WebServer, in.OperationsServer, in.System, in.KVS, ctx)

		return nil
	})
}

func (p *SDK) PostStart(ctx context.Context) error {
	defer logger.Debugf("Services installed:\n%s", p.Container().Visualize())
	return p.SDK.PostStart(ctx)
}
