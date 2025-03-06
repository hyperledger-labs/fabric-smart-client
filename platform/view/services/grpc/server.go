/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"sync"
	"sync/atomic"
	"time"

	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type GRPCServer struct {
	// Listen address for the server specified as hostname:port
	address string
	// Listener for handling network requests
	listener net.Listener
	// GRPC server
	server *grpc.Server
	// Certificate presented by the server for TLS communication
	// stored as an atomic reference
	serverCertificate atomic.Value
	// lock to protect concurrent access to append / remove
	lock *sync.Mutex
	// Set of PEM-encoded X509 certificate authorities used to populate
	// the tlsConfig.ClientCAs indexed by subject
	clientRootCAs map[string]*x509.Certificate
	// TLS configuration used by the grpc server
	tls *TLSConfig
	// Server for gRPC Health Check Protocol.
	healthServer *health.Server
}

// NewGRPCServer creates a new implementation of a GRPCServer given a
// listen address
func NewGRPCServer(address string, serverConfig ServerConfig) (*GRPCServer, error) {
	defer commLogger.Infof("New GRPC Server at [%s], TLS [%v], RequireClientCert [%v]",
		address, serverConfig.SecOpts.UseTLS, serverConfig.SecOpts.RequireClientCert)

	if address == "" {
		return nil, errors.New("missing address parameter")
	}
	//create our listener
	lis, err := net.Listen("tcp", address)

	if err != nil {
		return nil, err
	}
	return NewGRPCServerFromListener(lis, serverConfig)
}

// NewGRPCServerFromListener creates a new implementation of a GRPCServer given
// an existing net.Listener instance using default keepalive
func NewGRPCServerFromListener(listener net.Listener, serverConfig ServerConfig) (*GRPCServer, error) {
	grpcServer := &GRPCServer{
		address:  listener.Addr().String(),
		listener: listener,
		lock:     &sync.Mutex{},
	}

	//set up our server options
	var serverOpts []grpc.ServerOption

	secureConfig := serverConfig.SecOpts
	if secureConfig.UseTLS {
		//both key and cert are required
		if secureConfig.Key != nil && secureConfig.Certificate != nil {
			//load server public and private keys
			commLogger.Debugf("Load server public and private keys")
			cert, err := tls.X509KeyPair(secureConfig.Certificate, secureConfig.Key)
			if err != nil {
				return nil, err
			}

			grpcServer.serverCertificate.Store(cert)

			//set up our TLS config
			if len(secureConfig.CipherSuites) == 0 {
				secureConfig.CipherSuites = DefaultTLSCipherSuites
			}
			getCert := func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
				cert := grpcServer.serverCertificate.Load().(tls.Certificate)
				return &cert, nil
			}

			grpcServer.tls = NewTLSConfig(&tls.Config{
				VerifyPeerCertificate:  secureConfig.VerifyCertificate,
				GetCertificate:         getCert,
				SessionTicketsDisabled: true,
				CipherSuites:           secureConfig.CipherSuites,
			})

			if serverConfig.SecOpts.TimeShift > 0 {
				timeShift := serverConfig.SecOpts.TimeShift
				grpcServer.tls.config.Time = func() time.Time {
					return time.Now().Add((-1) * timeShift)
				}
			}
			grpcServer.tls.config.ClientAuth = tls.RequestClientCert
			//check if client authentication is required
			if secureConfig.RequireClientCert {
				//require TLS client auth
				grpcServer.tls.config.ClientAuth = tls.RequireAndVerifyClientCert
				//if we have client root CAs, create a certPool
				if len(secureConfig.ClientRootCAs) > 0 {
					grpcServer.clientRootCAs = make(map[string]*x509.Certificate)

					grpcServer.tls.config.ClientCAs = x509.NewCertPool()
					for _, clientRootCA := range secureConfig.ClientRootCAs {
						err = grpcServer.appendClientRootCA(clientRootCA)
						if err != nil {
							return nil, err
						}
					}
				}
			}

			// create credentials and add to server options
			creds := NewServerTransportCredentials(grpcServer.tls, serverConfig.Logger)
			serverOpts = append(serverOpts, grpc.Creds(creds))
		} else {
			return nil, errors.New("serverConfig.SecOpts must contain both Key and Certificate when UseTLS is true")
		}
	}
	// set max send and recv msg sizes
	serverOpts = append(serverOpts, grpc.MaxSendMsgSize(MaxSendMsgSize))
	serverOpts = append(serverOpts, grpc.MaxRecvMsgSize(MaxRecvMsgSize))
	// set the keepalive options
	serverOpts = append(serverOpts, ServerKeepaliveOptions(serverConfig.KaOpts)...)
	// set connection timeout
	if serverConfig.ConnectionTimeout <= 0 {
		serverConfig.ConnectionTimeout = DefaultConnectionTimeout
	}
	serverOpts = append(
		serverOpts,
		grpc.ConnectionTimeout(serverConfig.ConnectionTimeout))
	// set the interceptors
	if len(serverConfig.StreamInterceptors) > 0 {
		serverOpts = append(
			serverOpts,
			grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(serverConfig.StreamInterceptors...)),
		)
	}

	if len(serverConfig.UnaryInterceptors) > 0 {
		serverOpts = append(
			serverOpts,
			grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(serverConfig.UnaryInterceptors...)),
		)
	}

	if serverConfig.ServerStatsHandler != nil {
		serverOpts = append(serverOpts, grpc.StatsHandler(serverConfig.ServerStatsHandler))
	}

	grpcServer.server = grpc.NewServer(serverOpts...)

	if serverConfig.HealthCheckEnabled {
		grpcServer.healthServer = health.NewServer()
		healthpb.RegisterHealthServer(grpcServer.server, grpcServer.healthServer)
	}

	return grpcServer, nil
}

// SetServerCertificate assigns the current TLS certificate to be the peer's server certificate
func (gServer *GRPCServer) SetServerCertificate(cert tls.Certificate) {
	gServer.serverCertificate.Store(cert)
}

// Address returns the listen address for this GRPCServer instance
func (gServer *GRPCServer) Address() string {
	return gServer.address
}

// Listener returns the net.Listener for the GRPCServer instance
func (gServer *GRPCServer) Listener() net.Listener {
	return gServer.listener
}

// Server returns the grpc.Server for the GRPCServer instance
func (gServer *GRPCServer) Server() *grpc.Server {
	return gServer.server
}

// ServerCertificate returns the tls.Certificate used by the grpc.Server
func (gServer *GRPCServer) ServerCertificate() tls.Certificate {
	return gServer.serverCertificate.Load().(tls.Certificate)
}

// TLSEnabled is a flag indicating whether or not TLS is enabled for the
// GRPCServer instance
func (gServer *GRPCServer) TLSEnabled() bool {
	return gServer.tls != nil
}

// MutualTLSRequired is a flag indicating whether or not client certificates
// are required for this GRPCServer instance
func (gServer *GRPCServer) MutualTLSRequired() bool {
	return gServer.TLSEnabled() &&
		gServer.tls.Config().ClientAuth == tls.RequireAndVerifyClientCert
}

// Start starts the underlying grpc.Server
func (gServer *GRPCServer) Start() error {
	// if health check is enabled, set the health status for all registered services
	if gServer.healthServer != nil {
		for name := range gServer.server.GetServiceInfo() {
			gServer.healthServer.SetServingStatus(
				name,
				healthpb.HealthCheckResponse_SERVING,
			)
		}

		gServer.healthServer.SetServingStatus(
			"",
			healthpb.HealthCheckResponse_SERVING,
		)
	}
	return gServer.server.Serve(gServer.listener)
}

// Stop stops the underlying grpc.Server
func (gServer *GRPCServer) Stop() {
	gServer.server.Stop()
}

// internal function to add a PEM-encoded clientRootCA
func (gServer *GRPCServer) appendClientRootCA(clientRoot []byte) error {
	certs, subjects, err := pemToX509Certs(clientRoot)
	if err != nil {
		return errors.WithMessage(err, "failed to append client root certificate(s)")
	}

	if len(certs) < 1 {
		return errors.New("no client root certificates found")
	}

	for i, cert := range certs {
		//first add to the ClientCAs
		gServer.tls.AddClientRootCA(cert)
		//add it to our clientRootCAs map using subject as key
		gServer.clientRootCAs[subjects[i]] = cert
	}

	return nil
}

// SetClientRootCAs sets the list of authorities used to verify client
// certificates based on a list of PEM-encoded X509 certificate authorities
func (gServer *GRPCServer) SetClientRootCAs(clientRoots [][]byte) error {
	gServer.lock.Lock()
	defer gServer.lock.Unlock()

	//create a new map and CertPool
	clientRootCAs := make(map[string]*x509.Certificate)
	for _, clientRoot := range clientRoots {
		certs, subjects, err := pemToX509Certs(clientRoot)
		if err != nil {
			return errors.WithMessage(err, "failed to set client root certificate(s)")
		}

		for i, cert := range certs {
			clientRootCAs[subjects[i]] = cert
		}
	}

	//create a new CertPool and populate with the new clientRootCAs
	certPool := x509.NewCertPool()
	for _, clientRoot := range clientRootCAs {
		certPool.AddCert(clientRoot)
	}

	gServer.clientRootCAs = clientRootCAs
	gServer.tls.SetClientCAs(certPool)
	return nil
}
