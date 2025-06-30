/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"

	"github.com/go-kit/log"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	tracing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver/file"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/auditinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/binding"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/signerinfo"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"go.opentelemetry.io/otel/trace"
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
		p.Container().Provide(crypto.NewProvider, dig.As(new(hash.Hasher))),
		p.Container().Provide(simple.NewEventBus, dig.As(new(events.EventSystem), new(events.Publisher), new(events.Subscriber))),
		p.Container().Provide(func(system events.EventSystem) *events.Service { return &events.Service{EventSystem: system} }),
		p.Container().Provide(postgres.NewDbProvider),
		p.Container().Provide(postgres.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(sqlite.NewDbProvider),
		p.Container().Provide(sqlite.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(mem.NewNamedDriver, dig.Group("db-drivers")),
		p.Container().Provide(newMultiplexedDriver),
		p.Container().Provide(file.NewDriver, dig.Group("kms-drivers")),
		p.Container().Provide(newKVS),
		p.Container().Provide(sig.NewDeserializer),
		p.Container().Provide(endpoint.NewService),
		p.Container().Provide(
			digutils.Identity[*endpoint.Service](),
			dig.As(new(comm.EndpointService), new(id.EndpointService), new(endpoint.Backend), new(view.EndpointService)),
		),
		p.Container().Provide(binding.NewDefaultStore),
		p.Container().Provide(signerinfo.NewDefaultStore),
		p.Container().Provide(auditinfo.NewDefaultStore),
		p.Container().Provide(newKMSDriver),
		p.Container().Provide(id.NewProvider),
		p.Container().Provide(
			digutils.Identity[*id.Provider](),
			dig.As(new(endpoint.IdentityService), new(view3.IdentityProvider), new(driver.IdentityProvider)),
		),
		p.Container().Provide(endpoint.NewResolverService),
		p.Container().Provide(web.NewServer),
		p.Container().Provide(digutils.Identity[web.Server](), dig.As(new(operations.Server))),
		p.Container().Provide(web.NewOperationsLogger),
		p.Container().Provide(web.NewGRPCServer),
		p.Container().Provide(digutils.Identity[operations.OperationsLogger](), dig.As(new(operations.Logger)), dig.As(new(log.Logger))),
		p.Container().Provide(web.NewOperationsOptions),
		p.Container().Provide(operations.NewOperationSystem),
		p.Container().Provide(view.NewRegistry),
		p.Container().Provide(view3.NewResponseMarshaler, dig.As(new(view3.Marshaller))),
		p.Container().Provide(func(o *operations.Options, l operations.OperationsLogger) metrics2.Provider {
			return operations.NewMetricsProvider(o.Metrics, l, true)
		}),
		p.Container().Provide(func(metricsProvider metrics2.Provider, configService driver.ConfigService) (trace.TracerProvider, error) {
			base, err := tracing2.NewTracerProvider(configService)
			if err != nil {
				return nil, err
			}
			return tracing2.NewWrappedTracerProvider(tracing.NewTracerProviderWithBackingProvider(base, metricsProvider)), nil
		}),
		p.Container().Provide(view3.NewMetrics),
		p.Container().Provide(view3.NewAccessControlChecker, dig.As(new(view3.PolicyChecker))),
		p.Container().Provide(view3.NewViewServiceServer, dig.As(new(view3.Service), new(finality.Server))),
		p.Container().Provide(view.NewManager),
		p.Container().Provide(
			digutils.Identity[*view.Manager](),
			dig.As(new(StartableViewManager), new(ViewManager), new(server.ViewManager)),
		),

		p.Container().Provide(func(
			hostProvider host.GeneratorProvider,
			configProvider driver.ConfigService,
			endpointService *endpoint.Service,
			tracerProvider trace.TracerProvider,
			metricsProvider metrics2.Provider,
		) (*comm.Service, error) {
			return comm.NewService(hostProvider, endpointService, configProvider, metricsProvider)
		}),
		p.Container().Provide(digutils.Identity[*comm.Service](), dig.As(new(view.CommLayer))),
		p.Container().Provide(provider.NewHostProvider),
		p.Container().Provide(sig.NewService),
		p.Container().Provide(
			digutils.Identity[*sig.Service](),
			dig.As(
				new(view.LocalIdentityChecker), new(view3.VerifierProvider), new(view3.SignerProvider),
				new(id.SigService),
			),
		),
		p.Container().Provide(func(tracerProvider trace.TracerProvider) *finality.Manager {
			return finality.NewManager(tracerProvider)
		}),
	)
	if err != nil {
		return err
	}

	if err := p.SDK.Install(); err != nil {
		return err
	}

	err = errors.Join(
		digutils.Register[trace.TracerProvider](p.Container()),
		digutils.Register[ViewManager](p.Container()), // Need to add it as a field in the node
		digutils.Register[id.SigService](p.Container()),
		digutils.Register[*id.Provider](p.Container()),
		digutils.Register[*view.Registry](p.Container()),
		digutils.Register[*endpoint.Service](p.Container()),
	)
	if err != nil {
		return err
	}

	if err := p.Container().Invoke(func(resolverService *endpoint.ResolverService) error { return resolverService.LoadResolvers() }); err != nil {
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
		ConfigProvider driver.ConfigService

		GRPCServer     *grpc.GRPCServer
		ViewManager    StartableViewManager
		ViewService    view3.Service
		CommService    *comm.Service
		WebServer      web.Server
		System         *operations.System
		KVS            *kvs.KVS
		TracerProvider trace.TracerProvider
	}) error {
		if in.GRPCServer != nil {
			protos.RegisterViewServiceServer(in.GRPCServer.Server(), in.ViewService)
		}
		in.CommService.Start(ctx)

		view3.InstallViewHandler(in.ViewManager, in.ViewService, in.TracerProvider)
		go in.ViewManager.Start(ctx)

		web.Serve(in.GRPCServer, in.WebServer, in.System, in.KVS, ctx)

		return nil
	})
}

func (p *SDK) PostStart(ctx context.Context) error {
	defer logger.Debugf("Services installed:\n%s", p.Container().Visualize())
	return p.SDK.PostStart(ctx)
}
