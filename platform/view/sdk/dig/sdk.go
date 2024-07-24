/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"errors"
	"reflect"

	"github.com/go-kit/log"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	dig2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	digutils "github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	tracing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/provider"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver"
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

var logger = flogging.MustGetLogger("view-sdk")

type SDK struct {
	*dig2.BaseSDK
	C      *dig.Container
	Config *view.ConfigService
}

func (p *SDK) Container() *dig.Container { return p.C }

func (p *SDK) ConfigService() driver.ConfigService { return p.Config }

func NewSDK(registry node.Registry) *SDK {
	return NewSDKWithContainer(dig.New(), registry)
}

func NewSDKWithContainer(c *dig.Container, registry node.Registry) *SDK {
	sdk := &SDK{
		C:      c,
		Config: view.GetConfigService(registry),
	}
	err := errors.Join(
		sdk.C.Provide(func() node.Registry { return registry }),
		sdk.C.Provide(digutils.Identity[node.Registry](), dig.As(new(driver.ServiceProvider), new(node.Registry), new(view.ServiceProvider), new(finality.Registry))),
		sdk.C.Provide(func() *view.ConfigService { return sdk.Config }),
		sdk.C.Provide(digutils.Identity[*view.ConfigService](), dig.As(new(driver.ConfigService), new(id.ConfigProvider), new(endpoint.ConfigService))),
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
		p.C.Provide(crypto.NewProvider, dig.As(new(hash.Hasher))),
		p.C.Provide(simple.NewEventBus, dig.As(new(events.EventSystem), new(events.Publisher), new(events.Subscriber))),
		p.C.Provide(func(system events.EventSystem) *events.Service { return &events.Service{EventSystem: system} }),
		p.C.Provide(badger.NewDriver, dig.Group("db-drivers")),
		p.C.Provide(badger.NewFileDriver, dig.Group("db-drivers")),
		p.C.Provide(sql.NewDriver, dig.Group("db-drivers")),
		p.C.Provide(mem.NewDriver, dig.Group("db-drivers")),
		p.C.Provide(file.NewDriver, dig.Group("kms-drivers")),
		p.C.Provide(newKVS),
		p.C.Provide(sig.NewDeserializer),
		p.C.Provide(sig.NewDeserializerManager),
		p.C.Provide(sig.NewSignService, dig.As(new(id.SigService), new(driver.SigService), new(driver.SigRegistry), new(driver.AuditRegistry))),
		p.C.Provide(view.NewSigService, dig.As(new(view3.VerifierProvider), new(view3.SignerProvider))),
		p.C.Provide(digutils.Identity[*kvs.KVS](), dig.As(new(sig.KVS))),
		p.C.Provide(func(defaultKVS *kvs.KVS) (*endpoint.Service, error) { return endpoint.NewService(nil, nil, defaultKVS) }),
		p.C.Provide(digutils.Identity[*endpoint.Service](), dig.As(new(driver.EndpointService))),
		p.C.Provide(view.NewEndpointService),
		p.C.Provide(digutils.Identity[*view.EndpointService](), dig.As(new(comm.EndpointService), new(id.EndpointService), new(endpoint.Backend))),
		p.C.Provide(newKMSDriver),
		p.C.Provide(id.NewProvider, dig.As(new(endpoint.IdentityService), new(view3.IdentityProvider), new(driver.IdentityProvider))),
		p.C.Provide(endpoint.NewResolverService),
		p.C.Provide(web.NewServer),
		p.C.Provide(digutils.Identity[web.Server](), dig.As(new(operations.Server))),
		p.C.Provide(web.NewOperationsLogger),
		p.C.Provide(web.NewGRPCServer),
		p.C.Provide(digutils.Identity[operations.OperationsLogger](), dig.As(new(operations.Logger)), dig.As(new(log.Logger))),
		p.C.Provide(web.NewOperationsOptions),
		p.C.Provide(operations.NewOperationSystem),
		p.C.Provide(view3.NewResponseMarshaler, dig.As(new(view3.Marshaller))),
		p.C.Provide(func(o *operations.Options, l operations.OperationsLogger) metrics2.Provider {
			return operations.NewMetricsProvider(o.Metrics, l, true)
		}),
		p.C.Provide(func(metricsProvider metrics2.Provider, configService driver.ConfigService) (trace.TracerProvider, error) {
			base, err := tracing2.NewTracerProvider(configService)
			if err != nil {
				return nil, err
			}
			enhanced := tracing.NewTracerProviderWithBackingProvider(base, metricsProvider)
			nodeName := configService.GetString("fsc.id")
			named := tracing.NewProviderWithNodeName(enhanced, nodeName)
			return tracing2.NewWrappedTracerProvider(named), nil
		}),
		p.C.Provide(view3.NewMetrics),
		p.C.Provide(view3.NewAccessControlChecker, dig.As(new(view3.PolicyChecker))),
		p.C.Provide(view3.NewViewServiceServer, dig.As(new(view3.Service), new(finality.Server))),
		p.C.Provide(manager.New, dig.As(new(ViewManager), new(node.ViewManager), new(driver.ViewManager), new(driver.Registry))),
		p.C.Provide(view.NewManager),

		p.C.Provide(func(hostProvider host.GeneratorProvider, configProvider driver.ConfigService, endpointService *view.EndpointService, identityProvider view3.IdentityProvider, tracerProvider trace.TracerProvider, metricsProvider metrics2.Provider) (*comm.Service, error) {
			return comm.NewService(hostProvider, endpointService, configProvider, identityProvider.DefaultIdentity(), tracerProvider, metricsProvider)
		}),
		p.C.Provide(digutils.Identity[*comm.Service](), dig.As(new(manager.CommLayer))),
		p.C.Provide(provider.NewHostProvider),
		p.C.Provide(view.NewSigService),
		p.C.Provide(func(tracerProvider trace.TracerProvider) *finality.Manager {
			return finality.NewManager(tracerProvider)
		}),
	)
	if err != nil {
		return err
	}

	err = errors.Join(
		digutils.Register[trace.TracerProvider](p.C),
		digutils.Register[driver.EndpointService](p.C),
		digutils.Register[view3.IdentityProvider](p.C),
		digutils.Register[node.ViewManager](p.C), // Need to add it as a field in the node
		digutils.Register[id.SigService](p.C),
	)
	if err != nil {
		return err
	}

	if err := p.C.Invoke(func(resolverService *endpoint.ResolverService) error { return resolverService.LoadResolvers() }); err != nil {
		return err
	}
	if err := p.C.Invoke(func(server finality.Server, manager *finality.Manager) {
		server.RegisterProcessor(reflect.TypeOf(&protos.Command_IsTxFinal{}), manager.IsTxFinal)
	}); err != nil {
		return err
	}
	logger.Debugf("Services installed:\n%s", digutils.Visualize(p.C))
	return nil
}

func (p *SDK) Start(ctx context.Context) error {
	return p.C.Invoke(func(in struct {
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

func newKVS(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []driver2.NamedDriver `group:"db-drivers"`
}) (*kvs.KVS, error) {
	driverName := utils.DefaultString(in.Config.GetString("fsc.kvs.persistence.type"), "memory")
	for _, driver := range in.Drivers {
		if string(driver.Name) == driverName {
			return kvs.NewWithConfig(driver.Driver, "_default", in.Config)
		}
	}
	return nil, errors.New("driver not found")
}

func newKMSDriver(in struct {
	dig.In
	Config  driver.ConfigService
	Drivers []driver3.NamedDriver `group:"kms-drivers"`
}) (*kms.KMS, error) {
	driverName := utils.DefaultString(in.Config.GetString("fsc.identity.type"), "file")
	for _, driver := range in.Drivers {
		if string(driver.Name) == driverName {
			return &kms.KMS{Driver: driver.Driver}, nil
		}
	}
	return nil, errors.New("driver not found")
}
