/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/stats"
)

// Configuration defaults
var (
	// Max send and receive bytes for grpc clients and servers
	MaxRecvMsgSize = 100 * 1024 * 1024
	MaxSendMsgSize = 100 * 1024 * 1024
	// DefaultTLSCipherSuites is the strong TLS cipher suites
	DefaultTLSCipherSuites = []uint16{
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	}
	// DefaultConnectionTimeout is the default connection timeout
	DefaultConnectionTimeout = 5 * time.Second
)

// ServerKeepAliveEnforcementPolicyConfig describes the server keep alive enforcement policy.
type ServerKeepAliveEnforcementPolicyConfig struct {
	MinTime             time.Duration `yaml:"min-time"`
	PermitWithoutStream bool          `yaml:"permit-without-stream"`
}

// ServerKeepAliveConfig describes the server keep alive parameters.
type ServerKeepAliveConfig struct {
	MaxConnectionIdle     time.Duration                           `yaml:"max-connection-idle"`
	MaxConnectionAge      time.Duration                           `yaml:"max-connection-age"`
	MaxConnectionAgeGrace time.Duration                           `yaml:"max-connection-age-grace"`
	Time                  time.Duration                           `yaml:"time"`
	Timeout               time.Duration                           `yaml:"timeout"`
	EnforcementPolicy     *ServerKeepAliveEnforcementPolicyConfig `yaml:"enforcement-policy"`
}

// ClientKeepAliveConfig describes the client keep alive parameters.
type ClientKeepAliveConfig struct {
	Time                time.Duration `yaml:"time"`
	Timeout             time.Duration `yaml:"timeout"`
	PermitWithoutStream bool          `yaml:"permit-without-stream"`
}

// ConnectionConfig contains data required to establish grpc connection to a peer or orderer
type ConnectionConfig struct {
	Address            string        `yaml:"address,omitempty"`
	ConnectionTimeout  time.Duration `yaml:"connectionTimeout,omitempty"`
	TLSEnabled         bool          `yaml:"tlsEnabled,omitempty"`
	TLSClientSideAuth  bool          `yaml:"tlsClientSideAuth,omitempty"`
	TLSDisabled        bool          `yaml:"tlsDisabled,omitempty"`
	TLSRootCertFile    string        `yaml:"tlsRootCertFile,omitempty"`
	TLSRootCertBytes   [][]byte      `yaml:"tlsRootCertBytes,omitempty"`
	ServerNameOverride string        `yaml:"serverNameOverride,omitempty"`
	Usage              string        `yaml:"usage,omitempty"`
}

// ServerConfig defines the parameters for configuring a GRPCServer instance
type ServerConfig struct {
	// ConnectionTimeout specifies the timeout for connection establishment
	// for all new connections
	ConnectionTimeout time.Duration
	// SecOpts defines the security parameters
	SecOpts SecureOptions
	// KeepAliveConfig defines the keepalive parameters
	KeepAliveConfig *ServerKeepAliveConfig
	// StreamInterceptors specifies a list of interceptors to apply to
	// streaming RPCs.  They are executed in order.
	StreamInterceptors []grpc.StreamServerInterceptor
	// UnaryInterceptors specifies a list of interceptors to apply to unary
	// RPCs.  They are executed in order.
	UnaryInterceptors []grpc.UnaryServerInterceptor
	// Logger specifies the logger the server will use
	Logger logging.Logger
	// HealthCheckEnabled enables the gRPC Health Checking Protocol for the server
	HealthCheckEnabled bool
	// ServerStatsHandler should be set if metrics on connections are to be reported.
	ServerStatsHandler stats.Handler
}

// ClientConfig defines the parameters for configuring a Client instance
type ClientConfig struct {
	// SecOpts defines the security parameters
	SecOpts SecureOptions
	// KeepAliveConfig defines the keepalive parameters
	KeepAliveConfig *ClientKeepAliveConfig
	// Timeout specifies how long the client will block when attempting to
	// establish a connection
	Timeout time.Duration
	// AsyncConnect makes connection creation non blocking
	AsyncConnect bool
}

// Clone clones this ClientConfig
func (cc ClientConfig) Clone() ClientConfig {
	shallowClone := cc
	return shallowClone
}

// SecureOptions defines the security parameters (e.g. TLS) for a
// GRPCServer or Client instance
type SecureOptions struct {
	// VerifyCertificate, if not nil, is called after normal
	// certificate verification by either a TLS client or server.
	// If it returns a non-nil error, the handshake is aborted and that error results.
	VerifyCertificate func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error
	// PEM-encoded X509 public key to be used for TLS communication
	Certificate []byte
	// PEM-encoded private key to be used for TLS communication
	Key []byte
	// Set of PEM-encoded X509 certificate authorities used by clients to
	// verify server certificates
	ServerRootCAs [][]byte
	// Set of PEM-encoded X509 certificate authorities used by servers to
	// verify client certificates
	ClientRootCAs [][]byte
	// Whether or not to use TLS for communication
	UseTLS bool
	// Whether or not TLS client must present certificates for authentication
	RequireClientCert bool
	// CipherSuites is a list of supported cipher suites for TLS
	CipherSuites []uint16
	// TimeShift makes TLS handshakes time sampling shift to the past by a given duration
	TimeShift time.Duration
}

// ServerKeepaliveOptions returns gRPC keepalive options for server.
func ServerKeepaliveOptions(c *ServerKeepAliveConfig) []grpc.ServerOption {
	if c == nil {
		return nil
	}
	var serverOpts []grpc.ServerOption
	kap := keepalive.ServerParameters{
		MaxConnectionIdle:     c.MaxConnectionIdle,
		MaxConnectionAge:      c.MaxConnectionAge,
		MaxConnectionAgeGrace: c.MaxConnectionAgeGrace,
		Time:                  c.Time,
		Timeout:               c.Timeout,
	}
	serverOpts = append(serverOpts, grpc.KeepaliveParams(kap))
	if c.EnforcementPolicy != nil {
		kep := keepalive.EnforcementPolicy{
			MinTime:             c.EnforcementPolicy.MinTime,
			PermitWithoutStream: c.EnforcementPolicy.PermitWithoutStream,
		}
		serverOpts = append(serverOpts, grpc.KeepaliveEnforcementPolicy(kep))
	}
	return serverOpts
}

// ClientKeepaliveOptions returns gRPC keepalive options for clients.
func ClientKeepaliveOptions(c *ClientKeepAliveConfig) []grpc.DialOption {
	if c == nil {
		return nil
	}
	var dialOpts []grpc.DialOption
	kap := keepalive.ClientParameters{
		Time:                c.Time,
		Timeout:             c.Timeout,
		PermitWithoutStream: c.PermitWithoutStream,
	}
	dialOpts = append(dialOpts, grpc.WithKeepaliveParams(kap))
	return dialOpts
}
