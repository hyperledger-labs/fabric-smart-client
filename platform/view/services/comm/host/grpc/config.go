/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"os"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type configService interface {
	GetString(key string) string
	GetPath(key string) string
	GetStringSlice(key string) []string
	GetBool(key string) bool
	IsSet(key string) bool
	TranslatePath(path string) string
	GetDuration(key string) time.Duration
}

type ExtraCAPoolProvider interface {
	ExtraCAs() [][]byte
}

type Config interface {
	ListenAddress() host2.PeerIPAddress
	CertPath() string
	ClientConfig(caPoolProvider ExtraCAPoolProvider) (grpc2.ClientConfig, error)
	ServerConfig(caPoolProvider ExtraCAPoolProvider) (grpc2.ServerConfig, error)
}

func NewConfig(cs configService) (*config, error) {
	serverRootCAs := make([]string, 0)
	for _, path := range cs.GetStringSlice("fsc.p2p.opts.grpc.tls.serverRootCAs.files") {
		serverRootCAs = append(serverRootCAs, cs.TranslatePath(path))
	}

	clientRootCAs := make([]string, 0)
	for _, path := range cs.GetStringSlice("fsc.p2p.opts.grpc.tls.clientRootCAs.files") {
		clientRootCAs = append(clientRootCAs, cs.TranslatePath(path))
	}

	clientAuthRequired := true
	if cs.IsSet("fsc.p2p.opts.grpc.tls.clientAuthRequired") {
		clientAuthRequired = cs.GetBool("fsc.p2p.opts.grpc.tls.clientAuthRequired")
	}

	connectionTimeout := grpc2.DefaultConnectionTimeout
	if cs.IsSet("fsc.p2p.opts.grpc.connectionTimeout") {
		connectionTimeout = cs.GetDuration("fsc.p2p.opts.grpc.connectionTimeout")
	}

	privateKeyPath := cs.GetPath("fsc.p2p.opts.grpc.tls.key.file")
	if privateKeyPath == "" {
		privateKeyPath = cs.GetPath("fsc.identity.key.file")
	}

	certPath := cs.GetPath("fsc.p2p.opts.grpc.tls.cert.file")
	if certPath == "" {
		certPath = cs.GetPath("fsc.identity.cert.file")
	}

	keyValue := cs.GetString("fsc.p2p.listenAddress")
	listenAddress, err := comm.ConvertAddress(keyValue)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing fsc.p2p.listenAddress [%s]", keyValue)
	}

	return NewConfigFromProperties(
		listenAddress,
		privateKeyPath,
		certPath,
		serverRootCAs,
		clientRootCAs,
		clientAuthRequired,
		connectionTimeout,
	), nil
}

func NewConfigFromProperties(
	listenAddress, privateKeyPath, certPath string,
	serverRootCAs, clientRootCAs []string,
	clientAuthRequired bool,
	connectionTimeout time.Duration,
) *config {
	return &config{
		listenAddress:      listenAddress,
		privateKeyPath:     privateKeyPath,
		certPath:           certPath,
		serverRootCAs:      serverRootCAs,
		clientRootCAs:      clientRootCAs,
		clientAuthRequired: clientAuthRequired,
		connectionTimeout:  connectionTimeout,
	}
}

type config struct {
	listenAddress      host2.PeerIPAddress
	privateKeyPath     string
	certPath           string
	serverRootCAs      []string
	clientRootCAs      []string
	clientAuthRequired bool
	connectionTimeout  time.Duration
}

func (c *config) ListenAddress() host2.PeerIPAddress { return c.listenAddress }

func (c *config) CertPath() string { return c.certPath }

func (c *config) ClientConfig(caPoolProvider ExtraCAPoolProvider) (grpc2.ClientConfig, error) {
	serverRootCAs, err := loadPEMFiles(c.serverRootCAs)
	if err != nil {
		return grpc2.ClientConfig{}, err
	}
	if caPoolProvider != nil {
		serverRootCAs = append(serverRootCAs, caPoolProvider.ExtraCAs()...)
	}

	keyPEM, certPEM, err := loadKeyPair(c.privateKeyPath, c.certPath)
	if err != nil {
		return grpc2.ClientConfig{}, err
	}

	return grpc2.ClientConfig{
		Timeout: c.connectionTimeout,
		SecOpts: grpc2.SecureOptions{
			UseTLS:            true,
			RequireClientCert: c.clientAuthRequired,
			ServerRootCAs:     serverRootCAs,
			Certificate:       certPEM,
			Key:               keyPEM,
		},
	}, nil
}

func (c *config) ServerConfig(caPoolProvider ExtraCAPoolProvider) (grpc2.ServerConfig, error) {
	clientRootCAs, err := loadPEMFiles(c.clientRootCAs)
	if err != nil {
		return grpc2.ServerConfig{}, err
	}
	if caPoolProvider != nil {
		clientRootCAs = append(clientRootCAs, caPoolProvider.ExtraCAs()...)
	}

	keyPEM, certPEM, err := loadKeyPair(c.privateKeyPath, c.certPath)
	if err != nil {
		return grpc2.ServerConfig{}, err
	}

	return grpc2.ServerConfig{
		ConnectionTimeout: c.connectionTimeout,
		SecOpts: grpc2.SecureOptions{
			UseTLS:            true,
			RequireClientCert: c.clientAuthRequired,
			ClientRootCAs:     clientRootCAs,
			Certificate:       certPEM,
			Key:               keyPEM,
		},
	}, nil
}

func loadPEMFiles(paths []string) ([][]byte, error) {
	out := make([][]byte, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read PEM cert in [%s]", path)
		}
		out = append(out, raw)
	}
	return out, nil
}

func loadKeyPair(keyPath, certPath string) ([]byte, []byte, error) {
	if certPath == "" || keyPath == "" {
		return nil, nil, errors.Errorf("both grpc p2p transport key and cert files must be set")
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read key in [%s]", keyPath)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read cert in [%s]", certPath)
	}
	return keyPEM, certPEM, nil
}
