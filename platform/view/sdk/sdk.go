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
	"net"

	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/badger"
	_ "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"

	"github.com/hyperledger/fabric/common/grpclogging"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/id/x509"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/identity"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/crypto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/core/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/manager"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/core/sig"
	api2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/assert"
	comm2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"
)

var logger = flogging.MustGetLogger("view-sdk")

type Registry interface {
	GetService(v interface{}) (interface{}, error)

	RegisterService(service interface{}) error
}

type p struct {
	confPath   string
	registry   Registry
	grpcServer *grpc2.GRPCServer
	viewServer server.Server

	context context.Context
}

func NewSDK(confPath string, registry Registry) *p {
	return &p{confPath: confPath, registry: registry}
}

func (p *p) Install() error {
	logger.Infof("View platform enabled, installing...")

	configProvider, err := config2.NewProvider(p.confPath)
	assert.NoError(err, "failed instantiating config provider")
	assert.NoError(p.registry.RegisterService(configProvider), "failed registering config provider")

	assert.NoError(p.registry.RegisterService(crypto.NewProvider()))

	// Sig Service
	des, err := sig.NewMultiplexDeserializer(p.registry)
	assert.NoError(err, "failed loading sig verifier deserializer service")
	des.AddDeserializer(&x509.Deserializer{})
	assert.NoError(p.registry.RegisterService(des))
	signerService := sig.NewSignService(p.registry, des)
	assert.NoError(p.registry.RegisterService(signerService))

	// Set Endpoint Service
	endpointService, err := endpoint.NewService(p.registry, nil)
	assert.NoError(err, "failed instantiating endpoint service")
	assert.NoError(p.registry.RegisterService(endpointService), "failed registering endpoint service")
	resolverService, err := endpoint.NewResolverService(configProvider, view.GetEndpointService(p.registry))
	assert.NoError(err, "failed instantiating endpoint resolver service")
	assert.NoError(resolverService.LoadResolvers(), "failed loading resolvers")

	// Set Identity Provider
	idProvider := id.NewProvider(configProvider, signerService, endpointService)
	assert.NoError(idProvider.Load(), "failed loading identities")
	assert.NoError(p.registry.RegisterService(idProvider))

	// Server
	marshaller, err := server.NewResponseMarshaler(p.registry)
	if err != nil {
		return fmt.Errorf("error creating view service response marshaller: %s", err)
	}

	p.viewServer, err = server.NewServer(marshaller,
		server.NewAccessControlChecker(
			idProvider,
			view.GetSigService(p.registry),
		),
	)
	if err != nil {
		return fmt.Errorf("error creating view service server: %s", err)
	}
	if err := p.registry.RegisterService(p.viewServer); err != nil {
		return err
	}

	// View Manager
	if err := p.registry.RegisterService(manager.New(p.registry)); err != nil {
		return err
	}

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

	return nil
}

func (p *p) Start(ctx context.Context) error {
	p.context = ctx

	assert.NoError(p.startGRPCServer(), "failed starting grpc server")
	assert.NoError(p.startCommLayer(), "failed starting comm layer")
	assert.NoError(p.startViewServer(), "failed starting view server")
	assert.NoError(p.startViewManager(), "failed starting view manager")

	logger.Infof("Started peer with ID=[%s], network ID=[%s], address=[%s]", view.GetConfigService(p.registry).GetString("fsc.id"))

	return p.serve()
}

func (p *p) startGRPCServer() error {
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

	cs := grpc2.NewCredentialSupport()
	if serverConfig.SecOpts.UseTLS {
		logger.Info("Starting peer with TLS enabled")
		cs = grpc2.NewCredentialSupport(serverConfig.SecOpts.ServerRootCAs...)

		// set the cert to use if client auth is requested by remote endpoints
		clientCert, err := p.getClientCertificate()
		if err != nil {
			logger.Fatalf("Failed to set TLS client certificate (%s)", err)
		}
		cs.SetClientCertificate(clientCert)
	}

	p.grpcServer, err = grpc2.NewGRPCServer(listenAddr, serverConfig)
	assert.NoError(err, "failed creating grpc server")

	return nil
}

func (p *p) startCommLayer() error {
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
	commService.Start(p.context)

	return nil
}

func (p *p) startViewServer() error {
	// Register the ViewService server
	protos.RegisterViewServiceServer(p.grpcServer.Server(), p.viewServer)

	return nil
}

func (p *p) startViewManager() error {
	server.InstallViewHandler(p.registry, p.viewServer)
	go api2.GetViewManager(p.registry).Start(p.context)

	return nil
}

func (p *p) serve() error {
	// Start the grpc server. Done in a goroutine
	go func() {
		logger.Info("Starting server")
		if err := p.grpcServer.Start(); err != nil {
			logger.Errorf("grpc server stopped with err [%s]", err)
		}
	}()
	go func() {
		select {
		case <-p.context.Done():
			logger.Info("Server stopping...")
			p.grpcServer.Stop()
			logger.Info("KVS stopping...")
			kvs.GetService(p.registry).Stop()
		}
	}()
	return nil
}

func (p *p) getLocalAddress() (string, error) {
	configProvider := view.GetConfigService(p.registry)

	peerAddress := configProvider.GetString("fsc.address")
	if peerAddress == "" {
		return "", fmt.Errorf("fsc.address isn't set")
	}
	host, port, err := net.SplitHostPort(peerAddress)
	if err != nil {
		return "", errors.Errorf("fsc.address isn't in host:port format: %s", peerAddress)
	}

	localIP, err := grpc2.GetLocalIP()
	if err != nil {
		logger.Errorf("local IP address not auto-detectable: %s", err)
		return "", err
	}
	autoDetectedIPAndPort := net.JoinHostPort(localIP, port)
	logger.Info("Auto-detected peer address:", autoDetectedIPAndPort)
	// If host is the IPv4 address "0.0.0.0" or the IPv6 address "::",
	// then fallback to auto-detected address
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		logger.Info("Host is", host, ", falling back to auto-detected address:", autoDetectedIPAndPort)
		return autoDetectedIPAndPort, nil
	}

	if configProvider.GetBool("fsc.addressAutoDetect") {
		logger.Info("Auto-detect flag is set, returning", autoDetectedIPAndPort)
		return autoDetectedIPAndPort, nil
	}
	logger.Info("Returning", peerAddress)
	return peerAddress, nil
}

func (p *p) getServerConfig() (grpc2.ServerConfig, error) {
	configProvider := view.GetConfigService(p.registry)

	serverConfig := grpc2.ServerConfig{
		ConnectionTimeout: configProvider.GetDuration("fsc.connectiontimeout"),
		SecOpts: grpc2.SecureOptions{
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
	serverConfig.KaOpts = grpc2.DefaultKeepaliveOptions
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

func (p *p) getClientCertificate() (tls.Certificate, error) {
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
