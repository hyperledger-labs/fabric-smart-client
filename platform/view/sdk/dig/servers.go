/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	glogging "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/connstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/operations"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server"
	web "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

const (
	KeepAliveConfigKey = "fsc.grpc.keepalive"
	// WebMaxRecvMsgSizeKey is the largest request body or WebSocket message, in bytes, the
	// web listener's view routes read; 0 or unset means protos.DefaultHTTPMaxRecvMsgSize.
	WebMaxRecvMsgSizeKey = "fsc.web.maxRecvMsgSize"
	// DefaultRemoteViewTimeout is the default execution timeout of views called remotely,
	// through the view service, on every transport.
	DefaultRemoteViewTimeout = 2 * time.Minute
	RemoteViewTimeoutKey     = "fsc.view.remote.timeout"
)

// WebServer is the fsc.web listener. Components mount HTTP handlers on it; secure handlers
// require a verified client certificate. The SDK starts and stops it with the node.
type WebServer interface {
	RegisterHandler(s string, handler http.Handler, secure bool)
	Start() error
	Stop() error
}

// GRPCServer is the fsc.grpc listener. Components register gRPC services on it before the
// SDK starts it with the node; Stop returns once in-flight handlers have returned.
type GRPCServer interface {
	grpc.ServiceRegistrar
	Start() error
	Stop()
}

// WebServerConstructor builds the fsc.web listener from the node's configuration. The SDK
// calls it only when fsc.web.enabled is true. A developer replaces the listener by
// decorating the constructor in the container with one that returns another implementation;
// the decorator returns a constructor rather than a server, so the replaced listener is never
// built and never binds a port. A constructor returns an error or a usable server, never a
// nil pointer of its own type.
type WebServerConstructor func(driver.ConfigService) (WebServer, error)

// GRPCServerConstructor builds the fsc.grpc listener from the node's configuration. The SDK
// calls it only when fsc.grpc.enabled is true. It is replaced the same way as
// WebServerConstructor.
type GRPCServerConstructor func(driver.ConfigService) (GRPCServer, error)

// enabledWebServer returns the fsc.web listener built by ctor, or a no-op server, without calling
// ctor, when fsc.web.enabled is false: the operations endpoints register on it either way.
func enabledWebServer(ctor WebServerConstructor, c driver.ConfigService) (WebServer, error) {
	if !c.GetBool("fsc.web.enabled") {
		logger.Info("web server not enabled")
		return web.NewDummyServer(), nil
	}
	s, err := ctor(c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("fsc.web.enabled is true but the WebServerConstructor returned no server")
	}
	logger.Infof("fsc.web served by %T", s)
	return s, nil
}

// enabledGRPCServer returns the fsc.grpc listener built by ctor, or nil, without calling ctor,
// when fsc.grpc.enabled is false.
func enabledGRPCServer(ctor GRPCServerConstructor, c driver.ConfigService) (GRPCServer, error) {
	if !c.GetBool("fsc.grpc.enabled") {
		logger.Info("grpc server not enabled")
		return nil, nil
	}
	s, err := ctor(c)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("fsc.grpc.enabled is true but the GRPCServerConstructor returned no server")
	}
	logger.Infof("fsc.grpc served by %T", s)
	return s, nil
}

// listenAddress returns <prefix>.address (prefix is "fsc.web" or "fsc.grpc"), or an error
// when it is empty or not host:port. An empty address would make net.Listen bind a random
// port on all interfaces, and a malformed one would only fail once the node starts serving.
func listenAddress(configProvider driver.ConfigService, prefix string) (string, error) {
	addr := configProvider.GetString(prefix + ".address")
	if addr == "" {
		return "", errors.Errorf("%s.enabled is true but %s.address is not set", prefix, prefix)
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return "", errors.Wrapf(err, "invalid %s.address [%s]", prefix, addr)
	}
	return addr, nil
}

// resolveWebTLS resolves fsc.web.tls, inheriting per field from fsc.tls.
func resolveWebTLS(configProvider driver.ConfigService) (grpc2.SecureOptions, error) {
	return tlsconfig.ResolveServer(configProvider, "fsc.tls", "fsc.web.tls")
}

// CheckTLSConfig resolves and validates the TLS of every enabled fsc listener, including
// the operations one, and the operations endpoints' client authentication, requires a
// host:port fsc.grpc.address and fsc.web.address when their listeners are enabled, checks
// fsc.view.remote (LoadRemoteConfig), and rejects removed configuration keys, without binding
// any address. It returns an error naming the offending key, which makes a core.yaml
// checkable without starting a node.
func CheckTLSConfig(configProvider driver.ConfigService) error {
	if err := tlsconfig.CheckRemovedKeys(configProvider, "fsc"); err != nil {
		return err
	}
	// Gated per service: a disabled listener's block is not this node's problem, and
	// failing on it would make `validate` reject a configuration that runs fine.
	if configProvider.GetBool("fsc.grpc.enabled") {
		if _, err := listenAddress(configProvider, "fsc.grpc"); err != nil {
			return err
		}
		if _, err := NewServerConfig(configProvider); err != nil {
			return errors.WithMessage(err, "invalid fsc.grpc TLS configuration")
		}
	}
	if configProvider.GetBool("fsc.web.enabled") {
		if _, err := listenAddress(configProvider, "fsc.web"); err != nil {
			return err
		}
		if n := configProvider.GetInt(WebMaxRecvMsgSizeKey); n < 0 {
			return errors.Errorf("[%s] must not be negative, got [%d]", WebMaxRecvMsgSizeKey, n)
		}
		if _, err := resolveWebTLS(configProvider); err != nil {
			return errors.WithMessage(err, "invalid fsc.web TLS configuration")
		}
	}
	if _, _, err := resolveOperationsTLS(configProvider); err != nil {
		return errors.WithMessage(err, "invalid fsc.metrics configuration")
	}
	if _, _, err := operationsClientAuth(configProvider); err != nil {
		return err
	}
	_, err := LoadRemoteConfig(configProvider)
	return err
}

// NewWebServer builds the node's web listener from fsc.web; it is the default
// WebServerConstructor. It returns an error when fsc.web.address is empty or not host:port,
// or when fsc.web.tls does not resolve: a listener is never returned with weaker TLS than the
// configuration describes.
func NewWebServer(configProvider driver.ConfigService) (WebServer, error) {
	addr, err := listenAddress(configProvider, "fsc.web")
	if err != nil {
		return nil, err
	}
	tlsOpts, err := resolveWebTLS(configProvider)
	if err != nil {
		return nil, errors.WithMessage(err, "failed resolving fsc.web.tls")
	}
	logger.Infof("web listener TLS: enabled=%v clientAuth=%v clientRootCAs=%d",
		tlsOpts.UseTLS, tlsOpts.RequireClientCert, len(tlsOpts.ClientRootCAs))
	return web.NewServer(web.Options{ListenAddress: addr, TLS: tlsOpts}), nil
}

// mountHealth serves /healthz and /readyz on the web listener, with readiness taken from
// connStates. They skip the client-certificate check of the view endpoints, but not the
// listener's TLS handshake. They are mounted by the SDK rather than by a WebServerConstructor,
// so a replaced listener serves them too; a disabled listener is a no-op server.
func mountHealth(ws WebServer, connStates *connstate.Metrics) {
	ws.RegisterHandler("/healthz", http.HandlerFunc(web.Healthz), false)
	ws.RegisterHandler("/readyz", web.Readyz(connStates), false)
}

// OperationsServer is the listener the operations endpoints (/metrics, /logspec) are served
// on. Own is non-nil when fsc.metrics.address gave them a listener of their own, which the
// SDK is then responsible for starting; when it is nil the endpoints share the web listener.
type OperationsServer struct {
	WebServer
	// Own is the listener this SDK must start, non-nil only when fsc.metrics.address gave the
	// operations endpoints one of their own.
	Own WebServer
}

// NewOperationsServer returns the listener for the operations endpoints.
//
// Setting fsc.metrics.address gives them a listener of their own, resolving fsc.metrics.tls
// independently of fsc.web so a plaintext metrics endpoint behind a TLS web listener is
// expressible. Without an address they stay on the web listener; a fsc.metrics.tls block is
// then an error rather than a warning, because the shared listener cannot honour it and the
// configuration would be claiming transport security it does not have.
func NewOperationsServer(configProvider driver.ConfigService, webServer WebServer) (OperationsServer, error) {
	addr, tlsOpts, err := resolveOperationsTLS(configProvider)
	if err != nil {
		return OperationsServer{}, err
	}
	if addr == "" {
		return OperationsServer{WebServer: webServer}, nil
	}
	logger.Infof("metrics listener on [%s] TLS: enabled=%v clientAuth=%v",
		addr, tlsOpts.UseTLS, tlsOpts.RequireClientCert)

	own := web.NewServer(web.Options{ListenAddress: addr, TLS: tlsOpts})
	return OperationsServer{WebServer: own, Own: own}, nil
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
// configuration, or an empty string when there is none. It returns the error
// NewOperationsOptions would fail with when the configuration is invalid.
func OperationsClientAuthWarning(configProvider driver.ConfigService) (string, error) {
	_, warning, err := operationsClientAuth(configProvider)
	return warning, err
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
		"certificate, so %s will reject every request. %s", clientAuthKey, key, endpoints, remedy), nil
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

// NewGRPCServer builds the node's gRPC listener from fsc.grpc; it is the default
// GRPCServerConstructor. The returned server has already bound its listen address.
func NewGRPCServer(configProvider driver.ConfigService) (GRPCServer, error) {
	listenAddr, err := listenAddress(configProvider, "fsc.grpc")
	if err != nil {
		return nil, err
	}
	serverConfig, err := NewServerConfig(configProvider)
	if err != nil {
		return nil, err
	}
	s, err := grpc2.NewGRPCServer(listenAddr, serverConfig)
	if err != nil {
		return nil, err // an untyped nil, not a nil *grpc.GRPCServer in the interface
	}
	return s, nil
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
func Serve(grpcServer GRPCServer, webServer WebServer, ops OperationsServer, operationsSystem *operations.System, kvss *kvs.KVS, ctx context.Context) {
	startServe(grpcServer, webServer, ops, operationsSystem, kvss, ctx)
}

// startServe starts the GRPC server, web server, metrics server and operations system in
// their own goroutines, plus a shutdown goroutine that stops them when ctx is done and then
// stops kvss. The web and gRPC servers' Stop return only once their in-flight handlers have
// returned, within a bound, so the KVS is not closed under a request still using it.
// The returned *sync.WaitGroup tracks the start goroutines of the web server, metrics
// server and operations system: the first two finish once their listener is bound, the
// last once the operations system has stopped.
func startServe(grpcServer GRPCServer, webServer WebServer, ops OperationsServer, operationsSystem *operations.System, kvss *kvs.KVS, ctx context.Context) *sync.WaitGroup {
	var wg sync.WaitGroup

	// Not tracked by wg: Serve returns only once every handler has, so joining it would
	// defeat the bound GRPCServer.Stop puts on that wait.
	if grpcServer != nil {
		go func() {
			logger.Info("Starting GRPC server...")
			if err := grpcServer.Start(); err != nil {
				logger.Fatalf("grpc server stopped with err [%s]", err)
			}
		}()
	}

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

		// Every server has stopped and the web and gRPC Stop calls above have waited, within
		// their bound, for in-flight handlers, which can touch the KVS. Join the start
		// goroutines too before tearing down the storage; this has to come after
		// operationsSystem.Stop(), which is what unblocks the operations system's own Start
		// goroutine.
		wg.Wait()

		if kvss != nil {
			logger.Info("kvs stopping...")
			kvss.Stop()
			logger.Info("kvs stopping...done")
		}
	}()

	return &wg
}

// RemoteViewTimeout returns the execution timeout of views called remotely, through the view
// service, read from fsc.view.remote.timeout; an absent or non-positive value yields
// DefaultRemoteViewTimeout. Local view calls keep their caller's context.
func RemoteViewTimeout(configProvider driver.ConfigService) time.Duration {
	if configProvider.IsSet(RemoteViewTimeoutKey) {
		if d := configProvider.GetDuration(RemoteViewTimeoutKey); d > 0 {
			return d
		}
	}
	return DefaultRemoteViewTimeout
}

// NewViewServiceServer constructs the view service for the DI container. Commands are bound
// to the verified client TLS certificate of the connection they arrive on
// (server.BindToVerifiedClientCert). The command time window is fsc.view.remote.timeWindow,
// and fsc.view.remote.maxMessageSize, when set, bounds commands and stream frames on every
// backend (see LoadRemoteConfig).
func NewViewServiceServer(
	remote RemoteConfig,
	marshaller server.Marshaller,
	policyChecker server.PolicyChecker,
	metrics *server.Metrics,
	tracerProvider tracing.Provider,
) (server.Service, error) {
	return server.NewViewServiceServer(marshaller, policyChecker, metrics, tracerProvider, server.BindToVerifiedClientCert, remote.TimeWindow,
		server.WithMaxMessageSize(remote.MaxMessageSize))
}
