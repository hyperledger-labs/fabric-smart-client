/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type configService interface {
	GetString(key string) string
	GetPath(key string) string
	GetStringSlice(key string) []string
	GetBool(key string) bool
	IsSet(key string) bool
	TranslatePath(path string) string
	GetInt(key string) int
}

type Config interface {
	ListenAddress() host2.PeerIPAddress
	ClientTLSConfig(caPoolProvider ExtraCAPoolProvider) *tls.Config
	ServerTLSConfig(caPoolProvider ExtraCAPoolProvider) *tls.Config
	CertPath() string
	MaxSubConns() int
	ReadHeaderTimeout() time.Duration
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
	IdleTimeout() time.Duration
	CORSAllowedOrigins() []string
}

type ExtraCAPoolProvider interface {
	ExtraCAs() [][]byte
}

func NewConfig(cs configService) (*config, error) {
	serverRootCAs := make([]string, 0)
	for _, path := range cs.GetStringSlice("fsc.p2p.opts.websocket.tls.serverRootCAs.files") {
		serverRootCAs = append(serverRootCAs, cs.TranslatePath(path))
	}

	clientRootCAs := make([]string, 0)
	for _, path := range cs.GetStringSlice("fsc.p2p.opts.websocket.tls.clientRootCAs.files") {
		clientRootCAs = append(clientRootCAs, cs.TranslatePath(path))
	}

	clientAuthRequired := true
	if cs.IsSet("fsc.p2p.opts.websocket.tls.clientAuthRequired") {
		clientAuthRequired = cs.GetBool("fsc.p2p.opts.websocket.tls.clientAuthRequired")
	}

	maxSubConns := 100
	if cs.IsSet("fsc.p2p.opts.websocket.maxSubConns") {
		maxSubConns = cs.GetInt("fsc.p2p.opts.websocket.maxSubConns")
	}

	var corsAllowedOrigins []string
	if cs.IsSet("fsc.p2p.opts.websocket.corsAllowedOrigins") {
		raw := cs.GetString("fsc.p2p.opts.websocket.corsAllowedOrigins")
		if raw != "" {
			parts := strings.Split(raw, ",")
			for _, p := range parts {
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

	return NewConfigFromProperties(
		listenAddress,
		cs.GetPath("fsc.identity.key.file"),
		cs.GetPath("fsc.identity.cert.file"),
		serverRootCAs,
		clientRootCAs,
		clientAuthRequired,
		maxSubConns,
		corsAllowedOrigins,
	), nil
}

func NewConfigFromProperties(listenAddress string, privateKeyPath, certPath string, serverRootCAs, clientRootCAs []string, clientAuthRequired bool, maxSubConns int, corsAllowedOrigins []string) *config {
	return &config{
		listenAddress:      listenAddress,
		privateKeyPath:     privateKeyPath,
		certPath:           certPath,
		serverRootCAs:      serverRootCAs,
		clientRootCAs:      clientRootCAs,
		clientAuthRequired: clientAuthRequired,
		maxSubConns:        maxSubConns,
		corsAllowedOrigins: corsAllowedOrigins,
	}
}

type config struct {
	listenAddress      host2.PeerIPAddress
	privateKeyPath     string
	certPath           string
	serverRootCAs      []string
	clientRootCAs      []string
	clientAuthRequired bool
	maxSubConns        int
	corsAllowedOrigins []string

	serverRootCAPool *x509.CertPool
	clientRootCAPool *x509.CertPool
	mu               sync.RWMutex
}

func (c *config) ListenAddress() host2.PeerIPAddress { return c.listenAddress }

func (c *config) CertPath() string { return c.certPath }

func (c *config) MaxSubConns() int { return c.maxSubConns }

func (c *config) ReadHeaderTimeout() time.Duration { return 10 * time.Second }

func (c *config) ReadTimeout() time.Duration { return 30 * time.Second }

func (c *config) WriteTimeout() time.Duration { return 30 * time.Second }

func (c *config) IdleTimeout() time.Duration { return 120 * time.Second }

func (c *config) CORSAllowedOrigins() []string { return c.corsAllowedOrigins }

func (c *config) ClientTLSConfig(caPoolProvider ExtraCAPoolProvider) *tls.Config {
	c.mu.Lock()
	if c.serverRootCAPool == nil {
		c.serverRootCAPool = utils.MustGet(NewRootCAPool(c.serverRootCAs, nil))
	}
	serverRootCAPool := c.serverRootCAPool
	c.mu.Unlock()

	return utils.MustGet(newClientTLSConfig(serverRootCAPool, c.privateKeyPath, c.certPath, caPoolProvider))
}

func (c *config) ServerTLSConfig(caPoolProvider ExtraCAPoolProvider) *tls.Config {
	c.mu.Lock()
	if c.clientRootCAPool == nil {
		c.clientRootCAPool = utils.MustGet(NewRootCAPool(c.clientRootCAs, nil))
	}
	clientRootCAPool := c.clientRootCAPool
	c.mu.Unlock()

	return utils.MustGet(newServerTLSConfig(clientRootCAPool, c.privateKeyPath, c.certPath, c.clientAuthRequired, caPoolProvider))
}

func newClientTLSConfig(serverRootCAPool *x509.CertPool, keyFile, certFile string, caPoolProvider ExtraCAPoolProvider) (*tls.Config, error) {
	if serverRootCAPool == nil && keyFile == "" && certFile == "" && caPoolProvider == nil {
		return nil, nil
	}

	if certFile == "" || keyFile == "" {
		return nil, errors.Errorf("both client key and cert files must be set for p2p TLS")
	}

	logger.Debugf("Loading client certificates from [%s,%s]", keyFile, certFile)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load client x509 certificates from [%s,%s]", keyFile, certFile)
	}

	var caCertPool *x509.CertPool
	if caPoolProvider != nil && len(caPoolProvider.ExtraCAs()) > 0 {
		caCertPool = serverRootCAPool.Clone()
		for _, extraCA := range caPoolProvider.ExtraCAs() {
			logger.Debugf("append extra CA [%s]", string(extraCA))
			if !caCertPool.AppendCertsFromPEM(extraCA) {
				return nil, errors.Errorf("failed to append extra cert")
			}
		}
	} else {
		caCertPool = serverRootCAPool
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		// Certificates:       []tls.Certificate{cert},
		RootCAs:            caCertPool,
		InsecureSkipVerify: false,
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

func newServerTLSConfig(clientRootCAPool *x509.CertPool, keyFile, certFile string, clientAuthRequired bool, caPoolProvider ExtraCAPoolProvider) (*tls.Config, error) {
	if clientRootCAPool == nil && keyFile == "" && certFile == "" && caPoolProvider == nil {
		return nil, nil
	}

	if certFile == "" || keyFile == "" {
		return nil, errors.Errorf("both server key and cert files must be set for p2p TLS")
	}

	logger.Debugf("Loading server certificates from [%s,%s]", keyFile, certFile)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load server x509 certificates from [%s,%s]", keyFile, certFile)
	}

	var caCertPool *x509.CertPool
	if caPoolProvider != nil && len(caPoolProvider.ExtraCAs()) > 0 {
		caCertPool = clientRootCAPool.Clone()
		for _, extraCA := range caPoolProvider.ExtraCAs() {
			logger.Debugf("append extra CA [%s]", string(extraCA))
			if !caCertPool.AppendCertsFromPEM(extraCA) {
				return nil, errors.Errorf("failed to append extra cert")
			}
		}
	} else {
		caCertPool = clientRootCAPool
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    caCertPool,
		VerifyConnection: func(cs tls.ConnectionState) error {
			logger.Debugf("Client provided %d certificates", len(cs.PeerCertificates))

			for i, cert := range cs.PeerCertificates {
				logger.Debugf("  Cert %d Subject: %s", i, cert.Subject.String())
				logger.Debugf("  Cert %d Issuer:  %s", i, cert.Issuer.String())
			}

			if clientAuthRequired && len(cs.PeerCertificates) == 0 {
				return errors.New("custom reject: no client cert provided")
			}
			return nil
		}}

	if !clientAuthRequired {
		return tlsConfig, nil
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	if caPoolProvider != nil {
		tlsConfig.GetConfigForClient = func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			extraCAs := caPoolProvider.ExtraCAs()
			if len(extraCAs) == 0 {
				return tlsConfig, nil
			}
			pool := clientRootCAPool.Clone()
			for _, extraCA := range extraCAs {
				logger.Debugf("append extra CA [%s]", string(extraCA))
				if !pool.AppendCertsFromPEM(extraCA) {
					return nil, errors.Errorf("failed to append extra cert")
				}
			}
			conf := tlsConfig.Clone()
			conf.ClientCAs = pool
			return conf, nil
		}
	}

	return tlsConfig, nil
}

func NewRootCAPool(rootCAs []string, extraCAs [][]byte) (*x509.CertPool, error) {
	caCertPool := x509.NewCertPool()
	for _, rootCA := range rootCAs {
		caCert, err := os.ReadFile(rootCA)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read PEM cert in [%s]", rootCA)
		}
		logger.Debugf("append CA [%s]", string(caCert))
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.Errorf("failed to append cert from [%s]", rootCA)
		}
	}
	for _, extraCA := range extraCAs {
		logger.Debugf("append extra CA [%s]", string(extraCA))
		if !caCertPool.AppendCertsFromPEM(extraCA) {
			return nil, errors.Errorf("failed to append extra cert")
		}
	}
	return caCertPool, nil
}
