/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"

	"github.com/go-kit/log"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	driver4 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	sig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/sig"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/registry"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	tracing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver/file"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	view3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/dig"
)

var logger = logging.MustGetLogger("view-sdk")

type SDK struct {
	dig2.SDK
}

type nodeRegistry interface {
	node.Registry
	RegisterEndpointService(service driver.EndpointService)
	RegisterViewManager(manager driver.ViewManager)
	RegisterViewRegistry(registry *view.Registry)
}

func NewSDKFromContainer(c dig2.Container, registry node.Registry) *SDK {
	configService := view.GetConfigService(registry)
	return NewSDKFrom(dig2.NewBaseSDK(c, configService), registry)
}

func NewSDK(registry node.Registry) *SDK {
	return NewSDKFromContainer(NewContainer(), registry)
}

func NewSDKFrom(baseSDK dig2.SDK, registry node.Registry) *SDK {
	sdk := &SDK{SDK: baseSDK}
	err := errors.Join(
		sdk.Container().Provide(func() (node.Registry, nodeRegistry) { return registry, registry.(nodeRegistry) }),
		sdk.Container().Provide(digutils.Identity[node.Registry](), dig.As(new(driver.ServiceProvider), new(node.Registry), new(view.ServiceProvider), new(finality.Registry))),
		sdk.Container().Provide(func() *view.ConfigService { return view.GetConfigService(registry) }),
		sdk.Container().Provide(digutils.Identity[*view.ConfigService](), dig.As(new(driver.ConfigService), new(id.ConfigProvider), new(endpoint.ConfigService))),
		sdk.Container().Provide(view.NewRegistry),
	)
	if err != nil {
		panic(err)
	}
	return sdk
}

type ViewManager interface {
	node.ViewManager
	Start(ctx context.Context)
}

func (p *SDK) Install() error {
	err := errors.Join(
		p.Container().Provide(crypto.NewProvider, dig.As(new(hash.Hasher))),
		p.Container().Provide(simple.NewEventBus, dig.As(new(events.EventSystem), new(events.Publisher), new(events.Subscriber))),
		p.Container().Provide(func(system events.EventSystem) *events.Service { return &events.Service{EventSystem: system} }),
		p.Container().Provide(sql.NewDriver, dig.Group("db-drivers")),
		p.Container().Provide(mem.NewDriver, dig.Group("db-drivers")),
		p.Container().Provide(file.NewDriver, dig.Group("kms-drivers")),
		p.Container().Provide(newKVS),
		p.Container().Provide(sig2.NewDeserializer),
		p.Container().Provide(sig2.NewService, dig.As(new(id.SigService), new(driver.SigService), new(driver.SigRegistry), new(driver.AuditRegistry))),
		p.Container().Provide(view.NewSigService, dig.As(new(view3.VerifierProvider), new(view3.SignerProvider))),
		p.Container().Provide(newBindingStore, dig.As(new(driver4.BindingStore))),
		p.Container().Provide(newSignerInfoStore, dig.As(new(driver4.SignerInfoStore))),
		p.Container().Provide(newAuditInfoStore, dig.As(new(driver4.AuditInfoStore))),
		p.Container().Provide(endpoint.NewService),
		p.Container().Provide(digutils.Identity[*endpoint.Service](), dig.As(new(driver.EndpointService))),
		p.Container().Provide(view.NewEndpointService),
		p.Container().Provide(digutils.Identity[*view.EndpointService](), dig.As(new(comm.EndpointService), new(id.EndpointService), new(endpoint.Backend))),
		p.Container().Provide(newKMSDriver),
		p.Container().Provide(id.NewProvider, dig.As(new(endpoint.IdentityService), new(view3.IdentityProvider), new(driver.IdentityProvider))),
		p.Container().Provide(endpoint.NewResolverService),
		p.Container().Provide(web.NewServer),
		p.Container().Provide(digutils.Identity[web.Server](), dig.As(new(operations.Server))),
		p.Container().Provide(web.NewOperationsLogger),
		p.Container().Provide(web.NewGRPCServer),
		p.Container().Provide(digutils.Identity[operations.OperationsLogger](), dig.As(new(operations.Logger)), dig.As(new(log.Logger))),
		p.Container().Provide(web.NewOperationsOptions),
		p.Container().Provide(operations.NewOperationSystem),
		p.Container().Provide(registry.NewViewProvider),
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
		p.Container().Provide(manager.New, dig.As(new(ViewManager), new(node.ViewManager), new(driver.ViewManager), new(driver.Registry))),
		p.Container().Provide(view.NewManager),

		p.Container().Provide(func(hostProvider host.GeneratorProvider, configProvider driver.ConfigService, endpointService *view.EndpointService, tracerProvider trace.TracerProvider, metricsProvider metrics2.Provider) (*comm.Service, error) {
			return comm.NewService(hostProvider, endpointService, configProvider, tracerProvider, metricsProvider)
		}),
		p.Container().Provide(digutils.Identity[*comm.Service](), dig.As(new(manager.CommLayer))),
		p.Container().Provide(provider.NewHostProvider),
		p.Container().Provide(view.NewSigService),
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
		p.Container().Invoke(func(resolverService *endpoint.ResolverService) error { return resolverService.LoadResolvers() }),
		p.Container().Invoke(func(r nodeRegistry, s driver.EndpointService) { r.RegisterEndpointService(s) }),
		p.Container().Invoke(func(r nodeRegistry, s driver.ViewManager) { r.RegisterViewManager(s) }),
		p.Container().Invoke(func(r nodeRegistry, s *view.Registry) { r.RegisterViewRegistry(s) }),
	)

	if err != nil {
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
		ViewManager    *view.Manager
		ViewManager2   ViewManager
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
		go in.ViewManager2.Start(ctx)

		web.Serve(in.GRPCServer, in.WebServer, in.System, in.KVS, ctx)

		return nil
	})
}

func (p *SDK) PostStart(ctx context.Context) error {
	defer logger.Debugf("Services installed:\n%s", p.Container().Visualize())
	return p.SDK.PostStart(ctx)
}
