/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	comm2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kms/driver/file"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing/optl"
	"github.com/hyperledger/fabric/common/grpclogging"
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

type GRPCServer interface {
}

type SDK struct {
	confPath string
	registry Registry

	webServer WebServer

	grpcServer  *grpc2.GRPCServer
	viewService view2.Service
	viewManager Startable

	context          context.Context
	operationsSystem *operations.System

	commService *comm2.Service
}

func NewSDK(confPath string, registry Registry) *SDK {
	return &SDK{confPath: confPath, registry: registry}
}

func (p *SDK) Install() error {

	logger.Infof("View platform enabled, installing...")

	configProvider, err := config2.NewProvider(p.confPath)
	assert.NoError(err, "failed instantiating config provider")
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
	des, err := sig.NewMultiplexDeserializer(p.registry)
	assert.NoError(err, "failed loading sig verifier deserializer service")
	des.AddDeserializer(&x509.Deserializer{})
	assert.NoError(p.registry.RegisterService(des))
	signerService := sig.NewSignService(p.registry, des, defaultKVS)
	assert.NoError(p.registry.RegisterService(signerService))

	// Set Endpoint Service
	endpointService, err := endpoint.NewService(p.registry, nil, defaultKVS)
	assert.NoError(err, "failed instantiating endpoint service")
	assert.NoError(p.registry.RegisterService(endpointService), "failed registering endpoint service")

	//Get Default KMS Driver
	fscIdentityType := configProvider.GetString("fsc.identity.type")
	if len(fscIdentityType) == 0 {
		// revert to default
		fscIdentityType = "file"
	}
	kmsDriver, err := kms.Get(fscIdentityType)
	assert.NoError(err, "failed getting key management driver [%s]", fscIdentityType)

	// Set Identity Provider
	idProvider := id.NewProvider(configProvider, signerService, endpointService, kmsDriver)
	assert.NoError(idProvider.Load(), "failed loading identities")
	assert.NoError(p.registry.RegisterService(idProvider))

	// Resolver service
	resolverService, err := endpoint.NewResolverService(configProvider, view.GetEndpointService(p.registry), idProvider)
	assert.NoError(err, "failed instantiating endpoint resolver service")
	assert.NoError(resolverService.LoadResolvers(), "failed loading resolvers")

	assert.NoError(p.initWEBServer(), "failed initializing web server")
	assert.NoError(p.initWebOperationEndpointsAndMetrics(), "failed initializing web server endpoints and metrics")

	// View Service Server
	marshaller, err := view2.NewResponseMarshaler(p.registry)
	if err != nil {
		return fmt.Errorf("error creating view service response marshaller: %s", err)
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
		return fmt.Errorf("error creating view service server: %s", err)
	}
	if err := p.registry.RegisterService(p.viewService); err != nil {
		return err
	}

	// View Manager
	viewManager := manager.New(p.registry)
	if err := p.registry.RegisterService(viewManager); err != nil {
		return err
	}
	p.viewManager = viewManager
	if err := p.installTracing(); err != nil {
		return errors.WithMessage(err, "failed installing tracing")
	}

	finality.InstallHandler(p.registry, p.viewService)

	p.initCommLayer()

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

func (p *SDK) initWEBServer() error {
	configProvider := view.GetConfigService(p.registry)

	if !configProvider.GetBool("fsc.web.enabled") {
		p.webServer = web2.NewDummyServer()
		logger.Info("web server not enabled")
		return nil
	}

	listenAddr := configProvider.GetString("fsc.web.address")

	var tlsConfig web2.TLS
	var clientRootCAs []string
	for _, path := range configProvider.GetStringSlice("fsc.web.tls.clientRootCAs.files") {
		clientRootCAs = append(clientRootCAs, configProvider.TranslatePath(path))
	}
	tlsConfig = web2.TLS{
		Enabled:           configProvider.GetBool("fsc.web.tls.enabled"),
		CertFile:          configProvider.GetPath("fsc.web.tls.cert.file"),
		KeyFile:           configProvider.GetPath("fsc.web.tls.key.file"),
		ClientAuth:        configProvider.GetBool("fsc.web.tls.clientAuthRequired"),
		ClientCACertFiles: clientRootCAs,
	}
	p.webServer = web2.NewServer(web2.Options{
		ListenAddress: listenAddr,
		Logger:        logger,
		TLS:           tlsConfig,
	})
	h := web2.NewHttpHandler(logger)
	p.webServer.RegisterHandler("/", h, true)

	d := &web2.Dispatcher{
		Logger:  logger,
		Handler: h,
	}
	web2.InstallViewHandler(logger, p.registry, d)

	return nil
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
	configProvider := view.GetConfigService(p.registry)

	if !configProvider.GetBool("fsc.grpc.enabled") {
		logger.Info("grpc server not enabled")
		return nil
	}

	listenAddr := configProvider.GetString("fsc.grpc.address")
	serverConfig, err := p.getServerConfig()
	if err != nil {
		logger.Fatalf("Error loading secure config for peer (%s)", err)
	}

	serverConfig.Logger = flogging.MustGetLogger("core.comm").With("server", "PeerServer")
	serverConfig.UnaryInterceptors = append(
		serverConfig.UnaryInterceptors,
		grpclogging.UnaryServerInterceptor(flogging.MustGetLogger("comm.grpc.server").Zap()),
	)
	serverConfig.StreamInterceptors = append(
		serverConfig.StreamInterceptors,
		grpclogging.StreamServerInterceptor(flogging.MustGetLogger("comm.grpc.server").Zap()),
	)

	p.grpcServer, err = grpc2.NewGRPCServer(listenAddr, serverConfig)
	assert.NoError(err, "failed creating grpc server")

	return nil
}

func (p *SDK) initCommLayer() {
	configProvider := view.GetConfigService(p.registry)

	k, err := identity.NewCryptoPrivKeyFromMSP(configProvider.GetPath("fsc.identity.key.file"))
	assert.NoError(err, "failed loading p2p node secret key")

	commService, err := comm2.NewService(
		&comm2.PrivateKeyFromCryptoKey{Key: k},
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
	view2.InstallViewHandler(p.registry, p.viewService)
	go p.viewManager.Start(p.context)

	return nil
}

func (p *SDK) serve() error {
	// Start the grpc server. Done in a goroutine
	go func() {
		if p.grpcServer == nil {
			return
		}

		logger.Info("Starting GRPC server...")
		if err := p.grpcServer.Start(); err != nil {
			logger.Fatalf("grpc server stopped with err [%s]", err)
		}
	}()
	go func() {
		logger.Info("Starting WEB server...")
		if err := p.webServer.Start(); err != nil {
			logger.Fatalf("Failed starting WEB server: %v", err)
		}
	}()
	go func() {
		if p.operationsSystem == nil {
			return
		}
		logger.Info("Starting operations system...")
		if err := p.operationsSystem.Start(); err != nil {
			logger.Fatalf("Failed starting operations system: %v", err)
		}
	}()
	go func() {
		<-p.context.Done()
		logger.Info("web server stopping...")
		if err := p.webServer.Stop(); err != nil {
			logger.Errorf("failed stopping web server [%s]", err)
		}
		logger.Info("web server stopping...done")

		if p.grpcServer != nil {
			logger.Info("grpc server stopping...")
			p.grpcServer.Stop()
			logger.Info("grpc server stopping...done")
		}

		logger.Info("kvs stopping...")
		kvs.GetService(p.registry).Stop()
		logger.Info("kvs stopping...done")

		logger.Infof("operations system stopping...")
		if p.operationsSystem != nil {
			if err := p.operationsSystem.Stop(); err != nil {
				logger.Errorf("failed stopping operations system [%s]", err)
			}
		}
	}()
	return nil
}

func (p *SDK) getServerConfig() (grpc2.ServerConfig, error) {
	configProvider := view.GetConfigService(p.registry)

	serverConfig := grpc2.ServerConfig{
		ConnectionTimeout: configProvider.GetDuration("fsc.grpc.connectionTimeout"),
		SecOpts: grpc2.SecureOptions{
			UseTLS: configProvider.GetBool("fsc.grpc.tls.enabled"),
		},
	}
	if serverConfig.SecOpts.UseTLS {
		// get the certs from the file system
		serverKey, err := ioutil.ReadFile(configProvider.GetPath("fsc.grpc.tls.key.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS key (%s)", err)
		}
		serverCert, err := ioutil.ReadFile(configProvider.GetPath("fsc.grpc.tls.cert.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS certificate (%s)", err)
		}
		serverConfig.SecOpts.Certificate = serverCert
		serverConfig.SecOpts.Key = serverKey
		serverConfig.SecOpts.RequireClientCert = configProvider.GetBool("fsc.grpc.tls.clientAuthRequired")
		if serverConfig.SecOpts.RequireClientCert {
			var clientRoots [][]byte
			for _, file := range configProvider.GetStringSlice("fsc.grpc.tls.clientRootCAs.files") {
				clientRoot, err := ioutil.ReadFile(configProvider.TranslatePath(file))
				if err != nil {
					return serverConfig, fmt.Errorf("error loading client root CAs (%s)", err)
				}
				clientRoots = append(clientRoots, clientRoot)
			}
			serverConfig.SecOpts.ClientRootCAs = clientRoots
		}
	}
	// get the default keepalive options
	serverConfig.KaOpts = grpc2.DefaultKeepaliveOptions
	// check to see if interval is set for the env
	if configProvider.IsSet("fsc.grpc.keepalive.interval") {
		serverConfig.KaOpts.ServerInterval = configProvider.GetDuration("fsc.grpc.keepalive.interval")
	}
	// check to see if timeout is set for the env
	if configProvider.IsSet("fsc.grpc.keepalive.timeout") {
		serverConfig.KaOpts.ServerTimeout = configProvider.GetDuration("fsc.grpc.keepalive.timeout")
	}
	// check to see if minInterval is set for the env
	if configProvider.IsSet("fsc.grpc.keepalive.minInterval") {
		serverConfig.KaOpts.ServerMinInterval = configProvider.GetDuration("fsc.grpc.keepalive.minInterval")
	}
	return serverConfig, nil
}

func (p *SDK) installTracing() error {
	confService := view.GetConfigService(p.registry)

	var tracingProvider *tracing.Provider
	providerType := confService.GetString("fsc.tracing.provider")
	switch providerType {
	case "", "none":
		logger.Infof("Tracing disabled")
		tracingProvider = tracing.NewProvider(disabled.New())
	case "optl":
		logger.Infof("Tracing enabled: optl")
		address := confService.GetString("fsc.tracing.optl.address")
		if len(address) == 0 {
			address = "localhost:4319"
			logger.Infof("tracing server address not set, using default: [%s]", address)
		}
		tp := optl.LaunchOptl(address, context.Background())
		tracingProvider = tracing.NewProvider(optl.NewLatencyTracer(tp, optl.LatencyTracerOpts{Name: "FSC-Tracing"}))
	default:
		return errors.Errorf("unknown tracing provider: %s", providerType)
	}
	if err := p.registry.RegisterService(tracingProvider); err != nil {
		return err
	}
	return nil
}

func (p *SDK) initWebOperationEndpointsAndMetrics() error {
	configProvider := view.GetConfigService(p.registry)

	tlsEnabled := false
	if configProvider.IsSet("fsc.web.tls.enabled") {
		tlsEnabled = configProvider.GetBool("fsc.web.tls.enabled")
	}

	provider := configProvider.GetString("fsc.metrics.provider")
	statsdOperationsConfig := &operations.Statsd{}
	if provider == "statsd" && configProvider.IsSet("fsc.metrics.statsd") {
		if err := configProvider.UnmarshalKey("fsc.metrics.statsd", statsdOperationsConfig); err != nil {
			return errors.Wrap(err, "error unmarshalling metrics.statsd config")
		}
	}

	p.operationsSystem = operations.NewSystem(p.webServer, operations.Options{
		Metrics: operations.MetricsOptions{
			Provider: provider,
			Statsd:   statsdOperationsConfig,
		},
		TLS: operations.TLS{
			Enabled: tlsEnabled,
		},
		Version: "1.0.0",
	})
	return p.registry.RegisterService(p.operationsSystem)
}
