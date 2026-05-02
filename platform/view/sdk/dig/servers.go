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

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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
)

const (
	KeepAliveConfigKey = "fsc.grpc.keepalive"
)

type Server interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
	Start() error
	Stop() error
}

type MetricsServer struct {
	Server
	enabled bool
}

func (m *MetricsServer) Enabled() bool {
	return m != nil && m.enabled
}

type OperationsServer struct {
	web     Server
	metrics *MetricsServer
}

func (o *OperationsServer) RegisterHandler(path string, handler http.Handler, secure bool) {
	if path == "/metrics" && o.metrics.Enabled() {
		o.metrics.RegisterHandler(path, handler, secure)
		return
	}

	o.web.RegisterHandler(path, handler, secure)
}

func readClientRootCAs(configProvider driver.ConfigService, key string) []string {
	var clientRootCAs []string
	for _, path := range configProvider.GetStringSlice(key) {
		clientRootCAs = append(clientRootCAs, configProvider.TranslatePath(path))
	}
	return clientRootCAs
}

func readTLSConfig(configProvider driver.ConfigService, prefix string) web.TLS {
	return web.TLS{
		Enabled:           configProvider.GetBool(prefix + ".enabled"),
		CertFile:          configProvider.GetPath(prefix + ".cert.file"),
		KeyFile:           configProvider.GetPath(prefix + ".key.file"),
		ClientAuth:        configProvider.GetBool(prefix + ".clientAuthRequired"),
		ClientCACertFiles: readClientRootCAs(configProvider, prefix+".clientRootCAs.files"),
	}
}

func NewWebServer(configProvider driver.ConfigService, viewManager server.ViewManager, identityProvider server.IdentityProvider, tracerProvider tracing.Provider) Server {
	if !configProvider.GetBool("fsc.web.enabled") {
		logger.Info("web server not enabled")
		return web.NewDummyServer()
	}

	listenAddr := configProvider.GetString("fsc.web.address")
	tlsConfig := readTLSConfig(configProvider, "fsc.web.tls")
	webServer := web.NewServer(web.Options{
		ListenAddress: listenAddr,
		TLS:           tlsConfig,
	})
	h := web.NewHttpHandler()
	webServer.RegisterHandler("/", otelhttp.NewHandler(h, "rest-view-call"), true)

	web2.InstallViewHandler(viewManager, identityProvider, h, tracerProvider)

	return webServer
}

func NewMetricsServer(configProvider driver.ConfigService) *MetricsServer {
	listenAddr := configProvider.GetString("fsc.metrics.prometheus.address")
	if listenAddr == "" {
		return &MetricsServer{Server: web.NewDummyServer()}
	}

	metricsServer := web.NewServer(web.Options{
		ListenAddress: listenAddr,
		TLS:           readTLSConfig(configProvider, "fsc.metrics.prometheus.tls"),
	})

	return &MetricsServer{
		Server:  metricsServer,
		enabled: true,
	}
}

func NewOperationsServer(webServer Server, metricsServer *MetricsServer) *OperationsServer {
	return &OperationsServer{
		web:     webServer,
		metrics: metricsServer,
	}
}

func NewOperationsOptions(configProvider driver.ConfigService) (*operations.Options, error) {
	tlsEnabled := configProvider.IsSet("fsc.web.tls.enabled") && configProvider.GetBool("fsc.web.tls.enabled")

	provider := configProvider.GetString("fsc.metrics.provider")
	prometheusTLS := false
	switch {
	case configProvider.IsSet("fsc.metrics.prometheus.tls.clientAuthRequired"):
		prometheusTLS = configProvider.GetBool("fsc.metrics.prometheus.tls.clientAuthRequired")
	case configProvider.IsSet("fsc.metrics.prometheus.tls"):
		prometheusTLS = configProvider.GetBool("fsc.metrics.prometheus.tls")
	}
	logger.Infof("Starting operations with mTLS-protected metrics: %v", prometheusTLS)

	return &operations.Options{
		Metrics: operations.MetricsOptions{
			Provider: provider,
			TLS:      prometheusTLS,
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

	if configProvider.IsSet(KeepAliveConfigKey) {
		keepAliveConfig := &grpc2.ServerKeepAliveConfig{}
		if err := configProvider.UnmarshalKey(KeepAliveConfigKey, keepAliveConfig); err != nil {
			return serverConfig, errors.Wrap(err, "error unmarshalling keep alive config")
		}
		serverConfig.KeepAliveConfig = keepAliveConfig
	}
	return serverConfig, nil
}

func Serve(grpcServer *grpc2.GRPCServer, webServer Server, metricsServer *MetricsServer, operationsSystem *operations.System, kvss *kvs.KVS, ctx context.Context) {
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
		if !metricsServer.Enabled() {
			return
		}
		logger.Info("Starting metrics server...")
		if err := metricsServer.Start(); err != nil {
			logger.Fatalf("Failed starting metrics server: %v", err)
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

		if metricsServer.Enabled() {
			logger.Info("metrics server stopping...")
			if err := metricsServer.Stop(); err != nil {
				logger.Errorf("failed stopping metrics server [%s]", err)
			}
			logger.Info("metrics server stopping...done")
		}

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

// NewViewServiceServer constructs the view gRPC server for the DI container.
// It derives whether mutual TLS is required from grpcServer and wires
// a BindingInspector that enforces the TLS certificate hash included in every
// signed command header when mutual TLS is active.
func NewViewServiceServer(
	marshaller server.Marshaller,
	policyChecker server.PolicyChecker,
	metrics *server.Metrics,
	tracerProvider tracing.Provider,
	grpcServer *grpc2.GRPCServer,
) (server.Service, error) {
	mutualTLS := grpcServer != nil && grpcServer.MutualTLSRequired()
	inspector := grpc2.NewBindingInspector(mutualTLS, server.ExtractTLSCertHashFromCommand)
	return server.NewViewServiceServer(marshaller, policyChecker, metrics, tracerProvider, server.BindingInspector(inspector))
}
