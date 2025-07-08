/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	glogging "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	web2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/web"
	web "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
)

type Server interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
	Start() error
	Stop() error
}

func NewWebServer(configProvider driver.ConfigService, viewManager server.ViewManager, tracerProvider tracing.Provider) Server {
	if !configProvider.GetBool("fsc.web.enabled") {
		logger.Info("web server not enabled")
		return web.NewDummyServer()
	}

	listenAddr := configProvider.GetString("fsc.web.address")

	var tlsConfig web.TLS
	var clientRootCAs []string
	for _, path := range configProvider.GetStringSlice("fsc.web.tls.clientRootCAs.files") {
		clientRootCAs = append(clientRootCAs, configProvider.TranslatePath(path))
	}
	tlsConfig = web.TLS{
		Enabled:           configProvider.GetBool("fsc.web.tls.enabled"),
		CertFile:          configProvider.GetPath("fsc.web.tls.cert.file"),
		KeyFile:           configProvider.GetPath("fsc.web.tls.key.file"),
		ClientAuth:        configProvider.GetBool("fsc.web.tls.clientAuthRequired"),
		ClientCACertFiles: clientRootCAs,
	}
	webServer := web.NewServer(web.Options{
		ListenAddress: listenAddr,
		TLS:           tlsConfig,
	})
	h := web.NewHttpHandler()
	webServer.RegisterHandler("/", otelhttp.NewHandler(h, "rest-view-call"), true)

	web2.InstallViewHandler(viewManager, h, tracerProvider)

	return webServer
}

func NewOperationsOptions(configProvider driver.ConfigService) (*operations.Options, error) {
	tlsEnabled := configProvider.IsSet("fsc.web.tls.enabled") && configProvider.GetBool("fsc.web.tls.enabled")

	provider := configProvider.GetString("fsc.metrics.provider")
	statsdOperationsConfig := &operations.Statsd{}
	if provider == "statsd" && configProvider.IsSet("fsc.metrics.statsd") {
		if err := configProvider.UnmarshalKey("fsc.metrics.statsd", statsdOperationsConfig); err != nil {
			return nil, fmt.Errorf("error unmarshalling metrics.statsd config: %w", err)
		}
	}

	prometheusTls := configProvider.IsSet("fsc.metrics.prometheus.tls") && configProvider.GetBool("fsc.metrics.prometheus.tls")
	logger.Infof("Starting operations with TLS: %v", prometheusTls)

	return &operations.Options{
		Metrics: operations.MetricsOptions{
			Provider: provider,
			Statsd:   statsdOperationsConfig,
			TLS:      prometheusTls,
		},
		TLS: operations.TLS{
			Enabled: tlsEnabled,
		},
		Version: "1.0.0",
	}, nil
}

func NewOperationsLogger(opts *operations.Options) operations.OperationsLogger {
	return operations.NewOperationsLogger(opts.Logger)
}

func NewGRPCServer(configProvider driver.ConfigService) (*grpc2.GRPCServer, error) {
	if !configProvider.GetBool("fsc.grpc.enabled") {
		logger.Info("grpc server not enabled")
		return nil, nil
	}

	listenAddr := configProvider.GetString("fsc.grpc.address")
	serverConfig, err := NewServerConfig(configProvider)
	if err != nil {
		return nil, err
	}
	return grpc2.NewGRPCServer(listenAddr, serverConfig)
}

func NewServerConfig(configProvider driver.ConfigService) (grpc2.ServerConfig, error) {
	serverConfig := grpc2.ServerConfig{
		ConnectionTimeout: configProvider.GetDuration("fsc.grpc.connectionTimeout"),
		SecOpts: grpc2.SecureOptions{
			UseTLS: configProvider.GetBool("fsc.grpc.tls.enabled"),
		},
		Logger: logging.MustGetLogger().With("server", "PeerServer"),
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			glogging.UnaryServerInterceptor(logging.MustGetLogger().Zap()),
		},
		StreamInterceptors: []grpc.StreamServerInterceptor{
			glogging.StreamServerInterceptor(logging.MustGetLogger().Zap()),
		},

		ServerStatsHandler: otelgrpc.NewServerHandler(),
	}
	if serverConfig.SecOpts.UseTLS {
		// get the certs from the file system
		serverKey, err := os.ReadFile(configProvider.GetPath("fsc.grpc.tls.key.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS key (%s)", err)
		}
		serverCert, err := os.ReadFile(configProvider.GetPath("fsc.grpc.tls.cert.file"))
		if err != nil {
			return serverConfig, fmt.Errorf("error loading TLS certificate (%s)", err)
		}
		serverConfig.SecOpts.Certificate = serverCert
		serverConfig.SecOpts.Key = serverKey
		serverConfig.SecOpts.RequireClientCert = configProvider.GetBool("fsc.grpc.tls.clientAuthRequired")
		if serverConfig.SecOpts.RequireClientCert {
			var clientRoots [][]byte
			for _, file := range configProvider.GetStringSlice("fsc.grpc.tls.clientRootCAs.files") {
				clientRoot, err := os.ReadFile(configProvider.TranslatePath(file))
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

func Serve(grpcServer *grpc2.GRPCServer, webServer Server, operationsSystem *operations.System, kvss *kvs.KVS, ctx context.Context) {
	go func() {
		if grpcServer == nil {
			return
		}

		logger.Info("Starting GRPC server...")
		if err := grpcServer.Start(); err != nil {
			logger.Fatalf("grpc server stopped with err [%s]", err)
		}
	}()
	go func() {
		logger.Info("Starting WEB server...")
		if err := webServer.Start(); err != nil {
			logger.Fatalf("Failed starting WEB server: %v", err)
		}
	}()
	go func() {
		if operationsSystem == nil {
			return
		}
		logger.Info("Starting operations system...")
		if err := operationsSystem.Start(); err != nil {
			logger.Fatalf("Failed starting operations system: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		logger.Info("web server stopping...")
		if err := webServer.Stop(); err != nil {
			logger.Errorf("failed stopping web server [%s]", err)
		}
		logger.Info("web server stopping...done")

		if grpcServer != nil {
			logger.Info("grpc server stopping...")
			grpcServer.Stop()
			logger.Info("grpc server stopping...done")
		}

		logger.Info("kvs stopping...")
		kvss.Stop()
		logger.Info("kvs stopping...done")

		logger.Infof("operations system stopping...")
		if operationsSystem == nil {
			return
		}
		if err := operationsSystem.Stop(); err != nil {
			logger.Errorf("failed stopping operations system [%s]", err)
		}

	}()
}
