/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/sdk/finality"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events/simple"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/web"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/pkg/errors"
)

type mySDK struct {
	confPath      string
	registry      Registry
	context       context.Context
	startingQueue []Service
}

func (p *mySDK) Install() error {
	logger.Infof("View platform enabled, installing...")

	// install config provider
	configProvider, err := config.NewProvider(p.confPath)
	assert.NoError(err, "failed instantiating config provider")
	assert.NoError(p.registry.RegisterService(configProvider), "failed registering config provider")

	// install crypto provider
	assert.NoError(p.registry.RegisterService(crypto.NewProvider()))

	// install event service
	assert.NoError(p.registry.RegisterService(&events.Service{EventSystem: simple.NewEventBus()}))

	// KVS
	driverName := view.GetConfigService(p.registry).GetString("fsc.kvs.persistence.type")
	if len(driverName) == 0 {
		driverName = "memory"
	}
	defaultKVS, err := kvs.New(driverName, "_default", p.registry)
	if err != nil {
		return errors.Wrap(err, "failed creating kvs")
	}
	assert.NoError(p.registry.RegisterService(defaultKVS))

	kvsAutoStopper, err := NewServiceWithAutoStop(&kvs.ServiceWrapper{KVS: defaultKVS})
	assert.NoError(err, "cannot create kvsAutoStopper")
	p.startingQueue = append(p.startingQueue, kvsAutoStopper)

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

	// Set Identity Provider
	idProvider := id.NewProvider(configProvider, signerService, endpointService)
	assert.NoError(idProvider.Load(), "failed loading identities")
	assert.NoError(p.registry.RegisterService(idProvider))

	// Resolver service
	resolverService, err := endpoint.NewResolverService(configProvider, view.GetEndpointService(p.registry), idProvider)
	assert.NoError(err, "failed instantiating endpoint resolver service")
	assert.NoError(resolverService.LoadResolvers(), "failed loading resolvers")

	// web service (client API)
	if configProvider.GetBool("fsc.web.enabled") {
		webServer, err := web.New(p.registry)
		assert.NoError(err, "failed instantiating web service")
		assert.NoError(p.registry.RegisterService(webServer), "failed installing endpoint service")

		// TODO split web into actual client handlers

		// wrap as service and enqueue for starting
		webService, err := NewServiceWithoutContext(webServer)
		assert.NoError(err, "failed wrapping webServer as service")
		p.startingQueue = append(p.startingQueue, webService)
	}

	// metrics
	p.installMetrics()

	// install node communication layer
	p.installNodeCommLayer()

	// grpc server
	p.installGRPCServer()

	// View Service Server
	p.installGRPCViewService()

	// View Manager
	viewManager := manager.New(p.registry)
	assert.NoError(p.registry.RegisterService(viewManager), "failed installing view manager")
	viewManagerService, err := NewStoppable(viewManager)
	assert.NoError(err, "failed instantiating web service")
	p.startingQueue = append(p.startingQueue, viewManagerService)

	// tracing service
	p.installTracing()

	return nil
}

func (p *mySDK) Start(ctx context.Context) error {
	p.context = ctx

	for _, service := range p.startingQueue {

		service := service

		go func() {
			if err := service.Start(ctx); err != nil {
				//panic(err)
				logger.Errorf("error while starting: %s", err)
			}
		}()

		go func() {
			select {
			case <-ctx.Done():
				logger.Infof("stop called")
				if err := service.Stop(); err != nil {
					//panic(err)
					logger.Errorf("error while closing: %s", err)
				}
			}
		}()
	}

	logger.Debugf("Totally %d service(s) started", len(p.startingQueue))

	return nil
}

func (p *mySDK) installGRPCViewService() {
	// first fetch the dependencies (idProvider, grpcServer, and metricsProvider
	idProvider := view.GetIdentityProvider(p.registry)
	assert.NotNil(idProvider, "cannot get idProvider")

	grpcServer, err := grpc.GetService(p.registry)
	assert.NoError(err, "cannot get grpcServer")

	metricsProvider := metrics.GetProvider(p.registry)
	assert.NotNil(idProvider, "cannot get metrics provider")

	marshaller, err := view2.NewResponseMarshaler(p.registry)
	assert.NoError(err, "error creating view service response marshaller")

	viewService, err := view2.NewViewServiceServer(marshaller,
		view2.NewAccessControlChecker(idProvider, view.GetSigService(p.registry)),
		view2.NewMetrics(metricsProvider),
	)
	assert.NoError(err, "error creating view service server")
	assert.NoError(p.registry.RegisterService(viewService), "failed installing view service")
	protos.RegisterViewServiceServer(grpcServer.Server(), viewService)

	view2.InstallViewHandler(p.registry, viewService)
	finality.InstallHandler(p.registry, viewService)
}

func (p *mySDK) installGRPCServer() {
	configProvider := view.GetConfigService(p.registry)

	listenAddr := configProvider.GetString("fsc.listenAddress")
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

	cs := grpc.NewCredentialSupport()
	if serverConfig.SecOpts.UseTLS {
		logger.Info("Starting peer with TLS enabled")
		cs = grpc.NewCredentialSupport(serverConfig.SecOpts.ServerRootCAs...)

		// set the cert to use if client auth is requested by remote endpoints
		clientCert, err := p.getClientCertificate()
		if err != nil {
			logger.Fatalf("Failed to set TLS client certificate (%s)", err)
		}
		cs.SetClientCertificate(clientCert)
	}

	grpcServer, err := grpc.NewGRPCServer(listenAddr, serverConfig)
	assert.NoError(err, "failed creating grpc server")

	assert.NoError(p.registry.RegisterService(grpcServer), "failed installing grpc server")

	// wrap as service and enqueue for starting
	grpcService, err := NewServiceWithoutContext(grpcServer)
	assert.NoError(err, "failed wrapping grpcServer as service")
	p.startingQueue = append(p.startingQueue, grpcService)
}

func (p *mySDK) getServerConfig() (grpc.ServerConfig, error) {
	configProvider := view.GetConfigService(p.registry)

	serverConfig := grpc.ServerConfig{
		ConnectionTimeout: configProvider.GetDuration("fsc.connectiontimeout"),
		SecOpts: grpc.SecureOptions{
			UseTLS: configProvider.GetBool("fsc.tls.enabled"),
		},
	}
	if serverConfig.SecOpts.UseTLS {
		// get the certs from the file system
		serverKey, err := ioutil.ReadFile(configProvider.GetPath("fsc.tls.key.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS key (%s)", err)
		}
		serverCert, err := ioutil.ReadFile(configProvider.GetPath("fsc.tls.cert.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS certificate (%s)", err)
		}
		serverConfig.SecOpts.Certificate = serverCert
		serverConfig.SecOpts.Key = serverKey
		serverConfig.SecOpts.RequireClientCert = configProvider.GetBool("fsc.tls.clientAuthRequired")
		if serverConfig.SecOpts.RequireClientCert {
			var clientRoots [][]byte
			for _, file := range configProvider.GetStringSlice("fsc.tls.clientRootCAs.files") {
				clientRoot, err := ioutil.ReadFile(configProvider.TranslatePath(file))
				if err != nil {
					return serverConfig, fmt.Errorf("error loading client root CAs (%s)", err)
				}
				clientRoots = append(clientRoots, clientRoot)
			}
			serverConfig.SecOpts.ClientRootCAs = clientRoots
		}
		// check for root cert
		if configProvider.GetPath("fsc.tls.rootcert.file") != "" {
			rootCert, err := ioutil.ReadFile(configProvider.GetPath("fsc.tls.rootcert.file"))
			if err != nil {
				return serverConfig, fmt.Errorf("error loading TLS root certificate (%s)", err)
			}
			serverConfig.SecOpts.ServerRootCAs = [][]byte{rootCert}
		}
	}
	// get the default keepalive options
	serverConfig.KaOpts = grpc.DefaultKeepaliveOptions
	// check to see if interval is set for the env
	if configProvider.IsSet("fsc.keepalive.interval") {
		serverConfig.KaOpts.ServerInterval = configProvider.GetDuration("fsc.keepalive.interval")
	}
	// check to see if timeout is set for the env
	if configProvider.IsSet("fsc.keepalive.timeout") {
		serverConfig.KaOpts.ServerTimeout = configProvider.GetDuration("fsc.keepalive.timeout")
	}
	// check to see if minInterval is set for the env
	if configProvider.IsSet("fsc.keepalive.minInterval") {
		serverConfig.KaOpts.ServerMinInterval = configProvider.GetDuration("fsc.keepalive.minInterval")
	}
	return serverConfig, nil
}

func (p *mySDK) getClientCertificate() (tls.Certificate, error) {
	configProvider := view.GetConfigService(p.registry)

	cert := tls.Certificate{}

	keyPath := configProvider.GetString("fsc.tls.clientKey.file")
	certPath := configProvider.GetString("fsc.tls.clientCert.file")

	if keyPath != "" || certPath != "" {
		// need both keyPath and certPath to be set
		if keyPath == "" || certPath == "" {
			return cert, errors.New("fsc.tls.clientKey.file and " +
				"fsc.tls.clientCert.file must both be set or must both be empty")
		}
		keyPath = configProvider.GetPath("fsc.tls.clientKey.file")
		certPath = configProvider.GetPath("fsc.tls.clientCert.file")

	} else {
		// use the TLS server keypair
		keyPath = configProvider.GetString("fsc.tls.key.file")
		certPath = configProvider.GetString("fsc.tls.cert.file")

		if keyPath != "" || certPath != "" {
			// need both keyPath and certPath to be set
			if keyPath == "" || certPath == "" {
				return cert, errors.New("fsc.tls.key.file and " +
					"fsc.tls.cert.file must both be set or must both be empty")
			}
			keyPath = configProvider.GetPath("fsc.tls.key.file")
			certPath = configProvider.GetPath("fsc.tls.cert.file")
		} else {
			return cert, errors.New("must set either " +
				"[fsc.tls.key.file and fsc.tls.cert.file] or " +
				"[fsc.tls.clientKey.file and fsc.tls.clientCert.file]" +
				"when fsc.tls.clientAuthEnabled is set to true")
		}
	}
	// get the keypair from the file system
	clientKey, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return cert, errors.WithMessage(err,
			"error loading client TLS key")
	}
	clientCert, err := ioutil.ReadFile(certPath)
	if err != nil {
		return cert, errors.WithMessage(err,
			"error loading client TLS certificate")
	}
	cert, err = tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		return cert, errors.WithMessage(err,
			"error parsing client TLS key pair")
	}
	return cert, nil
}

func (p *mySDK) installMetrics() {
	configProvider := view.GetConfigService(p.registry)

	// get web server where we inject our handler
	webServer, err := web.GetService(p.registry)
	assert.NoError(err, "no web server registered")

	tlsEnabled := false
	if configProvider.IsSet("fsc.web.tls.enabled") {
		tlsEnabled = configProvider.GetBool("fsc.web.tls.enabled")
	} else {
		tlsEnabled = configProvider.GetBool("fsc.tls.enabled")
	}

	statsdOperationsConfig := &operations.Statsd{}
	assert.NoError(configProvider.UnmarshalKey("fsc.metrics.statsd", statsdOperationsConfig), "error unmarshalling metrics.statsd config")

	metricsSystem := operations.NewSystem(webServer, operations.Options{
		Metrics: operations.MetricsOptions{
			Provider: configProvider.GetString("fsc.metrics.provider"),
			Statsd:   statsdOperationsConfig,
		},
		TLS: operations.TLS{
			Enabled: tlsEnabled,
		},
		Version: "1.0.0",
	})

	assert.NoError(p.registry.RegisterService(metricsSystem), "failed installing web server")

	// wrap as service and enqueue for starting
	metricService, err := NewServiceWithoutContext(metricsSystem)
	assert.NoError(err, "failed wrapping metricsSystem as service")
	p.startingQueue = append(p.startingQueue, metricService)
}

func (p *mySDK) installTracing() {
	confService := view.GetConfigService(p.registry)

	provider := confService.GetString("fsc.tracing.provider")
	var agent interface{}
	switch provider {
	case "", "none":
		logger.Infof("Tracing disabled")
		agent = tracing.NewNullAgent()
	case "udp":
		logger.Infof("Tracing enabled: UDP")
		address := confService.GetString("fsc.tracing.udp.address")
		if len(address) == 0 {
			address = "localhost:8125"
			logger.Infof("tracing server address not set, using default: ", address)
		}
		var err error
		agent, err = tracing.NewStatsdAgent(
			tracing.Host(confService.GetString("fsc.id")),
			tracing.StatsDSink(address),
		)
		assert.NoError(err, "error creating tracing agent")
		logger.Infof("tracing enabled, listening on %s", address)
	default:
		assert.Fail("unknown tracing provider: %s", provider)
	}

	assert.NoError(p.registry.RegisterService(agent), "failed to install tracing service")
}

func (p *mySDK) installNodeCommLayer() {
	configProvider := view.GetConfigService(p.registry)

	k, err := identity.NewCryptoPrivKeyFromMSP(configProvider.GetPath("fsc.identity.key.file"))
	assert.NoError(err, "failed loading p2p node secret key")

	commService, err := comm.NewService(
		&comm.PrivateKeyFromCryptoKey{Key: k},
		view.GetEndpointService(p.registry),
		view.GetConfigService(p.registry),
		view.GetIdentityProvider(p.registry).DefaultIdentity(),
	)
	assert.NoError(err, "failed instantiating the communication service")
	assert.NoError(p.registry.RegisterService(commService), "failed installing communication service")

	// queue for starting
	p.startingQueue = append(p.startingQueue, commService)
}
