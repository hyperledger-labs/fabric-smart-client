/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/tlsconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

// websocketTLSKey is the configuration subtree holding this host's transport TLS.
const websocketTLSKey = "fsc.p2p.opts.websocket.tls"

type configService interface {
	GetString(key string) string
	GetPath(key string) string
	GetStringSlice(key string) []string
	GetBool(key string) bool
	IsSet(key string) bool
	TranslatePath(path string) string
	GetInt(key string) int
	RawSubtree(key string) (map[string]any, bool)
}

// Config is the websocket P2P host's view of its configuration.
type Config interface {
	ListenAddress() host2.PeerIPAddress
	ClientTLSConfig(caPoolProvider ExtraCAPoolProvider) (*tls.Config, error)
	ServerTLSConfig(caPoolProvider ExtraCAPoolProvider) (*tls.Config, error)
	CertPath() string
	MaxSubConns() int
	ReadHeaderTimeout() time.Duration
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
	IdleTimeout() time.Duration
	CORSAllowedOrigins() []string
}

// ExtraCAPoolProvider supplies trust anchors discovered at runtime rather than configured.
// The websocket host uses it to trust the identity certificates of known peers, so an empty
// configured CA pool is normal for this surface.
type ExtraCAPoolProvider interface {
	ExtraCAs() [][]byte
}

// NewConfig returns the websocket P2P configuration, resolving
// fsc.p2p.opts.websocket.tls through [tlsconfig] so an unknown key under it is an error
// rather than a silent discard.
//
// The transport keypair defaults to fsc.identity and CANNOT be given a separate credential:
// the peer ID a host announces is derived from the public key of its verified TLS
// certificate ([ws.expectedPeerIDFromRequest]), and the receiving side rejects any
// connection whose claimed peer ID does not match it. That binding is what prevents peer ID
// spoofing (issues #871, #1037), so the transport certificate and the application identity
// must remain the same key until an application-layer binding replaces it (issue #719).
//
// The block therefore inherits nothing from fsc.tls: that block holds the listener
// certificate, which is a different credential and would break the binding.
func NewConfig(cs configService) (*config, error) {
	identityCertPath := cs.GetPath("fsc.identity.cert.file")

	// This surface's rules all differ from the templates — mandatory mutual TLS, trust
	// anchors supplied at handshake time, and a keypair that must be the node's identity —
	// so tlsconfig gives it its own entry point rather than an options struct.
	serverTLS, clientTLS, err := tlsconfig.ResolveWebsocketP2P(cs, websocketTLSKey,
		&tlsconfig.File{File: identityCertPath},
		&tlsconfig.File{File: cs.GetPath("fsc.identity.key.file")})
	if err != nil {
		return nil, errors.WithMessagef(err, "failed resolving %s", websocketTLSKey)
	}

	maxSubConns := 100
	if cs.IsSet("fsc.p2p.opts.websocket.maxSubConns") {
		maxSubConns = cs.GetInt("fsc.p2p.opts.websocket.maxSubConns")
	}

	var corsAllowedOrigins []string
	if cs.IsSet("fsc.p2p.opts.websocket.corsAllowedOrigins") {
		raw := cs.GetString("fsc.p2p.opts.websocket.corsAllowedOrigins")
		if raw != "" {
			parts := strings.SplitSeq(raw, ",")
			for p := range parts {
				if s := strings.TrimSpace(p); s != "" {
					corsAllowedOrigins = append(corsAllowedOrigins, s)
				}
			}
		}
	}

	keyValue := cs.GetString("fsc.p2p.listenAddress")
	listenAddress, err := comm.ConvertAddress(keyValue)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing fsc.p2p.listenAddress [%s]", keyValue)
	}

	return &config{
		listenAddress: listenAddress,
		// The APPLICATION identity, used for nodeID / ExtractPKI and resolver addressing.
		// Not a transport credential.
		identityCertPath:   identityCertPath,
		serverTLS:          serverTLS,
		clientTLS:          clientTLS,
		maxSubConns:        maxSubConns,
		corsAllowedOrigins: corsAllowedOrigins,
	}, nil
}

type config struct {
	listenAddress host2.PeerIPAddress
	// identityCertPath is the APPLICATION identity certificate, used only to derive the
	// node's P2P identifier. It is never a transport credential.
	identityCertPath string
	// serverTLS and clientTLS carry the resolved TRANSPORT material for the inbound and
	// outbound directions.
	serverTLS          grpc.SecureOptions
	clientTLS          grpc.SecureOptions
	maxSubConns        int
	corsAllowedOrigins []string

	serverRootCAPool *x509.CertPool
	clientRootCAPool *x509.CertPool
	mu               sync.RWMutex
}

// ListenAddress returns the address the P2P host listens on.
func (c *config) ListenAddress() host2.PeerIPAddress { return c.listenAddress }

// CertPath returns the path of the node's application identity certificate, from which
// the host derives its nodeID. It is deliberately NOT the transport certificate.
func (c *config) CertPath() string { return c.identityCertPath }

// MaxSubConns returns the maximum number of multiplexed sub-connections accepted per peer.
func (c *config) MaxSubConns() int { return c.maxSubConns }

// ReadHeaderTimeout returns how long the server waits for request headers.
func (*config) ReadHeaderTimeout() time.Duration { return 10 * time.Second }

// ReadTimeout returns how long the server waits to read a request.
func (*config) ReadTimeout() time.Duration { return 30 * time.Second }

// WriteTimeout returns how long the server allows for writing a response.
func (*config) WriteTimeout() time.Duration { return 30 * time.Second }

// IdleTimeout returns how long an idle connection is kept open.
func (*config) IdleTimeout() time.Duration { return 120 * time.Second }

// CORSAllowedOrigins returns the origins permitted to open a websocket connection. An empty
// result disables CORS.
func (c *config) CORSAllowedOrigins() []string { return c.corsAllowedOrigins }

// ClientTLSConfig returns the TLS configuration for outbound connections, trusting the
// configured root CAs plus any the provider supplies at call time. TLS 1.3 is pinned. It
// returns nil when no TLS material is configured at all, and an error if the configured root
// CAs or the client keypair are invalid.
func (c *config) ClientTLSConfig(caPoolProvider ExtraCAPoolProvider) (*tls.Config, error) {
	c.mu.Lock()
	if c.serverRootCAPool == nil {
		pool, err := NewRootCAPoolFromPEM(c.clientTLS.ServerRootCAs)
		if err != nil {
			c.mu.Unlock()
			return nil, errors.Wrapf(err, "failed to build client root CA pool")
		}
		c.serverRootCAPool = pool
	}
	serverRootCAPool := c.serverRootCAPool
	c.mu.Unlock()

	return newClientTLSConfig(serverRootCAPool, c.clientTLS, caPoolProvider)
}

// ServerTLSConfig returns the TLS configuration for inbound connections. When mutual TLS is
// required the client CA pool is rebuilt per handshake so anchors the provider discovers
// later are honoured. TLS 1.3 is pinned. It returns nil when no TLS material is configured,
// and an error if the configured root CAs or the server keypair are invalid.
func (c *config) ServerTLSConfig(caPoolProvider ExtraCAPoolProvider) (*tls.Config, error) {
	c.mu.Lock()
	if c.clientRootCAPool == nil {
		pool, err := NewRootCAPoolFromPEM(c.serverTLS.ClientRootCAs)
		if err != nil {
			c.mu.Unlock()
			return nil, errors.Wrapf(err, "failed to build server root CA pool")
		}
		c.clientRootCAPool = pool
	}
	clientRootCAPool := c.clientRootCAPool
	c.mu.Unlock()

	return newServerTLSConfig(clientRootCAPool, c.serverTLS, caPoolProvider)
}

func newClientTLSConfig(serverRootCAPool *x509.CertPool, opts grpc.SecureOptions, caPoolProvider ExtraCAPoolProvider) (*tls.Config, error) {
	if serverRootCAPool == nil && len(opts.Certificate) == 0 && len(opts.Key) == 0 && caPoolProvider == nil {
		return nil, nil
	}

	if len(opts.Certificate) == 0 || len(opts.Key) == 0 {
		return nil, errors.Errorf("both client key and cert must be set for p2p TLS")
	}

	cert, err := tls.X509KeyPair(opts.Certificate, opts.Key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load client x509 certificates for p2p TLS")
	}

	caCertPool, err := withExtraCAs(serverRootCAPool, caPoolProvider)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
		// Unlike Certificates, which crypto/tls only presents when the server's AcceptableCAs
		// list its issuer, this always presents the identity certificate and leaves the trust
		// decision to the server.
		GetClientCertificate: func(cri *tls.CertificateRequestInfo) (*tls.Certificate, error) {
			logger.Debugf("Server requested %d Acceptable CAs", len(cri.AcceptableCAs))

			for i, caDER := range cri.AcceptableCAs {
				// AcceptableCAs are raw DER-encoded Distinguished Names (DNs)
				// We can parse them to read the human-readable Subject
				var name pkix.RDNSequence
				if _, err := asn1.Unmarshal(caDER, &name); err == nil {
					logger.Debugf("  Acceptable CA %d: %s", i, name.String())
				} else {
					logger.Debugf("  Acceptable CA %d (raw hex): %x", i, caDER)
				}
			}
			return &cert, nil
		},
	}

	return tlsConfig, nil
}

func newServerTLSConfig(clientRootCAPool *x509.CertPool, opts grpc.SecureOptions, caPoolProvider ExtraCAPoolProvider) (*tls.Config, error) {
	clientAuthRequired := opts.RequireClientCert
	if clientRootCAPool == nil && len(opts.Certificate) == 0 && len(opts.Key) == 0 && caPoolProvider == nil {
		return nil, nil
	}

	if len(opts.Certificate) == 0 || len(opts.Key) == 0 {
		return nil, errors.Errorf("both server key and cert must be set for p2p TLS")
	}

	cert, err := tls.X509KeyPair(opts.Certificate, opts.Key)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load server x509 certificates for p2p TLS")
	}

	caCertPool, err := withExtraCAs(clientRootCAPool, caPoolProvider)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
	}

	if !clientAuthRequired {
		return tlsConfig, nil
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	if caPoolProvider != nil {
		tlsConfig.GetConfigForClient = func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
			pool, err := withExtraCAs(clientRootCAPool, caPoolProvider)
			if err != nil {
				return nil, err
			}
			if pool == clientRootCAPool {
				return tlsConfig, nil
			}
			conf := tlsConfig.Clone()
			conf.ClientCAs = pool
			return conf, nil
		}
	}

	return tlsConfig, nil
}

// withExtraCAs returns pool extended with the provider's extra CAs. The configured pool is
// never modified: it is returned as is when there are no extra CAs, and cloned otherwise.
func withExtraCAs(pool *x509.CertPool, provider ExtraCAPoolProvider) (*x509.CertPool, error) {
	if provider == nil {
		return pool, nil
	}
	extraCAs := provider.ExtraCAs()
	if len(extraCAs) == 0 {
		return pool, nil
	}
	pool = pool.Clone()
	for _, extraCA := range extraCAs {
		logger.Debugf("append extra CA [%s]", string(extraCA))
		if !pool.AppendCertsFromPEM(extraCA) {
			return nil, errors.Errorf("failed to append extra cert")
		}
	}
	return pool, nil
}

// NewRootCAPoolFromPEM builds a certificate pool from PEM material that has already been
// read, which is the form [tlsconfig] resolution produces.
func NewRootCAPoolFromPEM(rootCAs [][]byte) (*x509.CertPool, error) {
	caCertPool := x509.NewCertPool()
	for _, caCert := range rootCAs {
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to append cert: not a valid PEM block")
		}
	}
	return caCertPool, nil
}
