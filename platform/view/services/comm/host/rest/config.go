/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"crypto/tls"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/pkg/errors"
)

type configService interface {
	GetString(key string) string
	GetPath(key string) string
}

type Config interface {
	ListenAddress() host2.PeerIPAddress
	TLSConfig() (*tls.Config, error)
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

func (c *config) TLSConfig() (*tls.Config, error) {
	return newTLSConfig(nil, c.privateKeyPath, c.certPath)
}

func (c *config) CertPath() string { return c.certPath }

func newTLSConfig(rootCACertFiles []string, keyFile, certFile string) (*tls.Config, error) {
	caCertPool, err := newRootCACertPool(rootCACertFiles)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read root CA certs")
	}

	var certs []tls.Certificate
	if certFile != "" || keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
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
