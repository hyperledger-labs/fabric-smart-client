/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	glogging "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
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

// Server is a listener the SDK starts and stops as part of the node lifecycle.
type Server interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
	Start() error
	Stop() error
}

// resolveWebTLS resolves fsc.web.tls, inheriting per field from fsc.tls.
func resolveWebTLS(configProvider driver.ConfigService) (grpc2.SecureOptions, error) {
	return tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.web.tls")
}

// CheckTLSConfig resolves and validates the TLS of every enabled fsc listener, including
// the operations one, and the operations endpoints' client authentication, and rejects
// removed configuration keys, without binding any address. It returns an error naming the
// offending key, which makes a core.yaml checkable without starting a node.
func CheckTLSConfig(configProvider driver.ConfigService) error {
	if err := tlsconfig.CheckRemovedKeys(configProvider, "fsc"); err != nil {
		return err
	}
	// Gated per service: a disabled listener's block is not this node's problem, and
	// failing on it would make `validate` reject a configuration that runs fine.
	if configProvider.GetBool("fsc.grpc.enabled") {
		if _, err := NewServerConfig(configProvider); err != nil {
			return errors.WithMessage(err, "invalid fsc.grpc TLS configuration")
		}
	}
	if configProvider.GetBool("fsc.web.enabled") {
		if _, err := resolveWebTLS(configProvider); err != nil {
			return errors.WithMessage(err, "invalid fsc.web TLS configuration")
		}
	}
	if _, _, err := resolveOperationsTLS(configProvider); err != nil {
		return errors.WithMessage(err, "invalid fsc.metrics configuration")
	}
	_, _, err := operationsClientAuth(configProvider)
	return err
}

// NewWebServer returns the node's REST listener, configured from fsc.web. It returns a
// no-op server when fsc.web.enabled is false, and an error when fsc.web.tls does not
// resolve — a listener is never returned with weaker TLS than the configuration describes.
func NewWebServer(configProvider driver.ConfigService, viewManager server.ViewManager, identityProvider server.IdentityProvider, tracerProvider tracing.Provider) (Server, error) {
	if !configProvider.GetBool("fsc.web.enabled") {
		logger.Info("web server not enabled")
		return web.NewDummyServer(), nil
	}

	tlsOpts, err := resolveWebTLS(configProvider)
	if err != nil {
		return nil, errors.WithMessage(err, "failed resolving fsc.web.tls")
	}
	logger.Infof("web listener TLS: enabled=%v clientAuth=%v clientRootCAs=%d",
		tlsOpts.UseTLS, tlsOpts.RequireClientCert, len(tlsOpts.ClientRootCAs))

	webServer := web.NewServer(web.Options{
		ListenAddress: configProvider.GetString("fsc.web.address"),
		TLS:           tlsOpts,
	})
	h := web.NewHttpHandler()
	webServer.RegisterHandler("/", otelhttp.NewHandler(h, "rest-view-call"), true)

	web2.InstallViewHandler(viewManager, identityProvider, h, tracerProvider)

	return webServer, nil
}

// OperationsServer is the listener the operations endpoints (/metrics, /logspec) are served
// on. Own is non-nil when fsc.metrics.address gave them a listener of their own, which the
// SDK is then responsible for starting; when it is nil the endpoints share the view service's
// web listener, as they always have.
type OperationsServer struct {
	Server
	// Own is the listener this SDK must start, non-nil only when fsc.metrics.address gave the
	// operations endpoints one of their own.
	Own Server
}

// NewOperationsServer returns the listener for the operations endpoints.
//
// Setting fsc.metrics.address gives them a listener of their own, resolving fsc.metrics.tls
// independently of fsc.web so a plaintext metrics endpoint behind a TLS web listener is
// expressible. Without an address they stay on the web listener; a fsc.metrics.tls block is
// then an error rather than a warning, because the shared listener cannot honour it and the
// configuration would be claiming transport security it does not have.
func NewOperationsServer(configProvider driver.ConfigService, webServer Server) (OperationsServer, error) {
	addr, tlsOpts, err := resolveOperationsTLS(configProvider)
	if err != nil {
		return OperationsServer{}, err
	}
	if addr == "" {
		return OperationsServer{Server: webServer}, nil
	}
	logger.Infof("metrics listener on [%s] TLS: enabled=%v clientAuth=%v",
		addr, tlsOpts.UseTLS, tlsOpts.RequireClientCert)

	own := web.NewServer(web.Options{ListenAddress: addr, TLS: tlsOpts})
	return OperationsServer{Server: own, Own: own}, nil
}

// resolveOperationsTLS returns fsc.metrics.address and the resolved fsc.metrics.tls, or an
// empty address when the operations endpoints share the fsc.web listener.
func resolveOperationsTLS(configProvider driver.ConfigService) (string, grpc2.SecureOptions, error) {
	addr := configProvider.GetString("fsc.metrics.address")
	if addr == "" {
		if _, ok := configProvider.RawSubtree("fsc.metrics.tls"); ok {
			return "", grpc2.SecureOptions{}, errors.New(
				"fsc.metrics.tls has no effect without fsc.metrics.address: the operations " +
					"endpoints share the fsc.web listener and its TLS. Set fsc.metrics.address " +
					"to give them a listener of their own, or remove fsc.metrics.tls")
		}
		return "", grpc2.SecureOptions{}, nil
	}
	tlsOpts, err := tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.metrics.tls")
	if err != nil {
		return "", grpc2.SecureOptions{}, errors.WithMessage(err, "failed resolving fsc.metrics.tls")
	}
	return addr, tlsOpts, nil
}

// NewOperationsOptions returns the options for the operations system.
//
// fsc.metrics.clientAuthRequired is deliberately separate from any listener's TLS: it demands
// a verified client certificate on /metrics and /logspec specifically, which is stricter than
// the listener may be. NWO relies on exactly that — its web listener verifies a client
// certificate only if one is offered, while scraping metrics requires one. The key defaults
// to true, because /logspec changes the process-wide log spec; only an explicit false
// serves the endpoints without a certificate. See operationsClientAuth for the errors.
func NewOperationsOptions(configProvider driver.ConfigService) (*operations.Options, error) {
	requireClientCert, warning, err := operationsClientAuth(configProvider)
	if err != nil {
		return nil, err
	}
	if warning != "" {
		logger.Warn(warning)
	}
	return &operations.Options{
		Metrics:           operations.MetricsOptions{Provider: configProvider.GetString("fsc.metrics.provider")},
		Version:           "1.0.0", // unchanged from servers.go:90
		Logger:            logging.MustGetLogger().With("server", "MetricsServer"),
		RequireClientCert: requireClientCert,
	}, nil
}

// OperationsClientAuthWarning returns the warning NewOperationsOptions logs for the
// configuration, or an empty string when there is none or the configuration is invalid.
func OperationsClientAuthWarning(configProvider driver.ConfigService) string {
	_, warning, _ := operationsClientAuth(configProvider)
	return warning
}

// operationsClientAuth resolves fsc.metrics.clientAuthRequired, true when absent. A value
// strconv.ParseBool rejects, including null, is an error, so a typo cannot disable the check.
// When the listener serving the operations endpoints never requests a client certificate,
// they reject every request: an explicit true is then an error, and a defaulted one is
// returned as a warning so the node still starts with the endpoints closed.
func operationsClientAuth(configProvider driver.ConfigService) (bool, string, error) {
	const clientAuthKey = "fsc.metrics.clientAuthRequired"
	explicit := configProvider.IsSet(clientAuthKey)
	if explicit {
		require, err := strconv.ParseBool(configProvider.GetString(clientAuthKey))
		if err != nil {
			return false, "", errors.Wrapf(err, "invalid %s", clientAuthKey)
		}
		if !require {
			return false, "", nil
		}
	}
	key, asks, err := operationsListenerAsksForCert(configProvider)
	if err != nil || asks {
		return true, "", err
	}
	endpoints := "/logspec"
	if configProvider.GetString("fsc.metrics.provider") == "prometheus" {
		endpoints = "/metrics and /logspec"
	}
	const remedy = "Enable TLS with clientRootCAs on it, or set " + clientAuthKey + " to false"
	if explicit {
		return false, "", errors.Errorf("%s is true, but the %s listener never requests a client "+
			"certificate, so %s would reject every request. %s", clientAuthKey, key, endpoints, remedy)
	}
	return true, fmt.Sprintf("%s defaults to true, but the %s listener never requests a client "+
		"certificate, so %s reject every request. %s", clientAuthKey, key, endpoints, remedy), nil
}

// operationsListenerAsksForCert reports whether the listener serving the operations
// endpoints requests a client certificate, and names its TLS key. That listener is the one
// of fsc.metrics.address when set, the fsc.web listener otherwise; with neither, no listener
// serves the endpoints and there is nothing to report.
func operationsListenerAsksForCert(configProvider driver.ConfigService) (string, bool, error) {
	addr, tlsOpts, err := resolveOperationsTLS(configProvider)
	if err != nil {
		return "", false, err
	}
	key := "fsc.metrics.tls"
	if addr == "" {
		if !configProvider.GetBool("fsc.web.enabled") {
			return "", true, nil
		}
		key = "fsc.web.tls"
		if tlsOpts, err = resolveWebTLS(configProvider); err != nil {
			return "", false, errors.WithMessagef(err, "failed resolving %s", key)
		}
	}
	// Client root CAs are what make the listener request a certificate; clientAuthRequired
	// without them is already rejected when the TLS block is resolved.
	return key, tlsOpts.UseTLS && len(tlsOpts.ClientRootCAs) > 0, nil
}

// NewOperationsLogger returns the logger the operations system writes to.
func NewOperationsLogger(opts *operations.Options) operations.OperationsLogger {
	return operations.NewOperationsLogger(opts.Logger)
}

// NewGRPCServer returns the node's view-service listener, configured from fsc.grpc, or nil
// when fsc.grpc.enabled is false. The returned server has already bound its listen address.
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

// NewServerConfig returns the gRPC server configuration read from fsc.grpc, with TLS
// resolved and every configured certificate loaded and validated. It returns an error
// rather than a partially configured server when fsc.grpc.tls does not resolve.
func NewServerConfig(configProvider driver.ConfigService) (grpc2.ServerConfig, error) {
	secOpts, err := tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.grpc.tls")
	if err != nil {
		return grpc2.ServerConfig{}, errors.WithMessage(err, "failed resolving fsc.grpc.tls")
	}
	serverConfig := grpc2.ServerConfig{
		ConnectionTimeout: configProvider.GetDuration("fsc.grpc.connectionTimeout"),
		MaxRecvMsgSize:    configProvider.GetInt("fsc.grpc.maxRecvMsgSize"),
		MaxSendMsgSize:    configProvider.GetInt("fsc.grpc.maxSendMsgSize"),
		SecOpts:           secOpts,
		Logger:            logging.MustGetLogger().With("server", "PeerServer"),
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			glogging.UnaryServerInterceptor(logging.MustGetLogger().Zap()),
		},
		StreamInterceptors: []grpc.StreamServerInterceptor{
			glogging.StreamServerInterceptor(logging.MustGetLogger().Zap()),
		},

		ServerStatsHandler: otelgrpc.NewServerHandler(),
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

// Serve starts the gRPC server, web server and operations system, and stops them when ctx
// is done. It returns immediately; each server runs in its own goroutine. A nil server is
// skipped.
func Serve(grpcServer *grpc2.GRPCServer, webServer Server, ops OperationsServer, operationsSystem *operations.System, kvss *kvs.KVS, ctx context.Context) {
	startServe(grpcServer, webServer, ops, operationsSystem, kvss, ctx)
}

// startServe starts the GRPC server, web server, and operations system in their own
// goroutines, plus a shutdown goroutine that stops them when ctx is done. It
// returns a *sync.WaitGroup tracking the three server goroutines so callers
// (and tests) can confirm they have actually stopped before proceeding with
// teardown.
func startServe(grpcServer *grpc2.GRPCServer, webServer Server, ops OperationsServer, operationsSystem *operations.System, kvss *kvs.KVS, ctx context.Context) *sync.WaitGroup {
	var wg sync.WaitGroup

	wg.Go(func() {
		if grpcServer == nil {
			return
		}

		logger.Info("Starting GRPC server...")
		if err := grpcServer.Start(); err != nil {
			logger.Fatalf("grpc server stopped with err [%s]", err)
		}
	})

	wg.Go(func() {
		logger.Info("Starting WEB server...")
		if err := webServer.Start(); err != nil {
			logger.Fatalf("Failed starting WEB server: %v", err)
		}
	})

	wg.Go(func() {
		// Only started when it is a listener of our own; otherwise the web server above
		// already serves these endpoints.
		if ops.Own == nil {
			return
		}
		logger.Info("Starting metrics server...")
		if err := ops.Own.Start(); err != nil {
			logger.Fatalf("Failed starting metrics server: %v", err)
		}
	})

	wg.Go(func() {
		if operationsSystem == nil {
			return
		}
		logger.Info("Starting operations system...")
		if err := operationsSystem.Start(); err != nil {
			logger.Fatalf("Failed starting operations system: %v", err)
		}
	})

	go func() {
		<-ctx.Done()
		logger.Info("web server stopping...")
		if err := webServer.Stop(); err != nil {
			logger.Errorf("failed stopping web server [%s]", err)
		}
		logger.Info("web server stopping...done")

		if ops.Own != nil {
			logger.Info("metrics server stopping...")
			if err := ops.Own.Stop(); err != nil {
				logger.Errorf("failed stopping metrics server [%s]", err)
			}
			logger.Info("metrics server stopping...done")
		}

		if grpcServer != nil {
			logger.Info("grpc server stopping...")
			grpcServer.Stop()
			logger.Info("grpc server stopping...done")
		}

		if operationsSystem != nil {
			logger.Infof("operations system stopping...")
			if err := operationsSystem.Stop(); err != nil {
				logger.Errorf("failed stopping operations system [%s]", err)
			}
		}

		// Every server has been asked to stop, so join their goroutines before tearing
		// down the storage they may still be using: in-flight requests served by the
		// web/grpc servers can touch the KVS, so stopping it any earlier would pull the
		// storage out from under them. This has to come after operationsSystem.Stop(),
		// which is what unblocks the operations system's own Start goroutine.
		wg.Wait()

		if kvss != nil {
			logger.Info("kvs stopping...")
			kvss.Stop()
			logger.Info("kvs stopping...done")
		}
	}()

	return &wg
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
