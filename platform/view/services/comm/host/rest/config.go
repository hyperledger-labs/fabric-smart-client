/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type configService interface {
	GetString(key string) string
	GetPath(key string) string
}

type Config interface {
	ListenAddress() host2.PeerIPAddress
	ClientTLSConfig() *tls.Config
	ServerTLSConfig() *tls.Config
	CertPath() string
}

func NewConfig(cs configService) *config {
	return NewConfigFromProperties(
		cs.GetString("fsc.p2p.listenAddress"),
		cs.GetPath("fsc.identity.key.file"),
		cs.GetPath("fsc.identity.cert.file"),
	)
}

func NewConfigFromProperties(listenAddress string, privateKeyPath, certPath string) *config {
	return &config{
		listenAddress:  listenAddress,
		privateKeyPath: privateKeyPath,
		certPath:       certPath,
	}
}

type config struct {
	listenAddress  host2.PeerIPAddress
	privateKeyPath string
	certPath       string
}

func (c *config) ListenAddress() host2.PeerIPAddress { return c.listenAddress }

func (c *config) CertPath() string { return c.certPath }

func (c *config) ClientTLSConfig() *tls.Config {
	tlsConfig := c.tlsConfig()
	if tlsConfig == nil {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: tlsConfig.InsecureSkipVerify, RootCAs: tlsConfig.RootCAs}
}

func (c *config) ServerTLSConfig() *tls.Config {
	tlsConfig := c.tlsConfig()
	if tlsConfig == nil {
		return nil
	}
	return &tls.Config{Certificates: tlsConfig.Certificates}
}

func (c *config) tlsConfig() *tls.Config {
	return utils.MustGet(newTLSConfig(nil, c.privateKeyPath, c.certPath))
}

func newTLSConfig(rootCACertFiles []string, keyFile, certFile string) (*tls.Config, error) {
	if len(rootCACertFiles) == 0 && len(keyFile) == 0 && len(certFile) == 0 {
		return nil, nil
	}
	caCertPool, err := newRootCACertPool(rootCACertFiles)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read root CA certs")
	}

	var certs []tls.Certificate
	if certFile != "" || keyFile != "" {
		logger.Infof("Loading certificates from [%s,%s]", keyFile, certFile)
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to load x509 certificates from [%s,%s]", keyFile, certFile)
		}
		certs = []tls.Certificate{cert}
	}

	tlsEnabled := len(keyFile) > 0 && len(certFile) > 0
	return &tls.Config{
		InsecureSkipVerify: tlsEnabled && caCertPool == nil,
		RootCAs:            caCertPool,
		Certificates:       certs,
	}, nil
}

func newRootCACertPool(rootCAs []string) (*x509.CertPool, error) {
	if len(rootCAs) == 0 {
		return nil, nil
	}
	caCertPool := x509.NewCertPool()
	for _, rootCA := range rootCAs {
		caCert, err := os.ReadFile(rootCA)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read PEM cert in [%s]", caCert)
		}
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.Errorf("failed to append cert from [%s]", caCert)
		}
	}
	return caCertPool, nil
}
