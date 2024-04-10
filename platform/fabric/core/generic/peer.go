/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"os"
	"time"

	peer2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/pkg/errors"
)

type PeerManager struct {
	ConnCache peer2.CachingEndorserPool
}

func NewPeerManager(configService driver.ConfigService, signer driver.Signer) *PeerManager {
	return &PeerManager{
		ConnCache: peer2.CachingEndorserPool{
			Cache: map[string]peer2.Client{},
			ConnCreator: &connCreator{
				ConfigService: configService,
				Singer:        signer,
			},
			Signer: signer,
		},
	}
}

func (c *PeerManager) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.Client, error) {
	logger.Debugf("NewPeerClientForAddress [%v]", cc)
	return c.ConnCache.NewPeerClientForAddress(cc)
}

type connCreator struct {
	ConfigService driver.ConfigService
	Singer        driver.Signer
}

func (c *connCreator) NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer2.Client, error) {
	logger.Debugf("Creating new peer client for address [%s]", cc.Address)
	var certs [][]byte
	if cc.TLSEnabled {
		switch {
		case len(cc.TLSRootCertFile) != 0:
			logger.Debugf("Loading TLSRootCert from file [%s]", cc.TLSRootCertFile)
			caPEM, err := os.ReadFile(cc.TLSRootCertFile)
			if err != nil {
				logger.Error("unable to load TLS cert from %s", cc.TLSRootCertFile)
				return nil, errors.WithMessagef(err, "unable to load TLS cert from %s", cc.TLSRootCertFile)
			}
			certs = append(certs, caPEM)
		case len(cc.TLSRootCertBytes) != 0:
			logger.Debugf("Loading TLSRootCert from passed bytes [%s[", cc.TLSRootCertBytes)
			certs = cc.TLSRootCertBytes
		default:
			return nil, errors.New("missing TLSRootCertFile in client config")
		}
	}

	clientConfig, override, err := c.GetClientConfig(certs, cc.TLSEnabled)
	if err != nil {
		return nil, err
	}

	if len(cc.ServerNameOverride) != 0 {
		override = cc.ServerNameOverride
	}

	return newPeerClientForClientConfig(
		c.Singer,
		cc.Address,
		override,
		*clientConfig,
	)
}

func (c *connCreator) GetClientConfig(tlsRootCerts [][]byte, UseTLS bool) (*grpc.ClientConfig, string, error) {
	override := c.ConfigService.TLSServerHostOverride()
	clientConfig := &grpc.ClientConfig{}
	clientConfig.Timeout = c.ConfigService.ClientConnTimeout()
	if clientConfig.Timeout == time.Duration(0) {
		clientConfig.Timeout = grpc.DefaultConnectionTimeout
	}

	secOpts := grpc.SecureOptions{
		UseTLS:            UseTLS,
		RequireClientCert: c.ConfigService.TLSClientAuthRequired(),
	}
	if UseTLS {
		secOpts.RequireClientCert = false
	}

	if secOpts.RequireClientCert {
		keyPEM, err := os.ReadFile(c.ConfigService.TLSClientKeyFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientKey.file")
		}
		secOpts.Key = keyPEM
		certPEM, err := os.ReadFile(c.ConfigService.TLSClientCertFile())
		if err != nil {
			return nil, "", errors.WithMessage(err, "unable to load fabric.tls.clientCert.file")
		}
		secOpts.Certificate = certPEM
	}
	clientConfig.SecOpts = secOpts

	if clientConfig.SecOpts.UseTLS {
		if len(tlsRootCerts) == 0 {
			return nil, "", errors.New("tls root cert file must be set")
		}
		clientConfig.SecOpts.ServerRootCAs = tlsRootCerts
	}

	clientConfig.KaOpts = grpc.KeepaliveOptions{
		ClientInterval: c.ConfigService.KeepAliveClientInterval(),
		ClientTimeout:  c.ConfigService.KeepAliveClientTimeout(),
	}

	return clientConfig, override, nil
}

func newPeerClientForClientConfig(signer driver.Signer, address, override string, clientConfig grpc.ClientConfig) (*peer2.PeerClient, error) {
	gClient, err := grpc.NewGRPCClient(clientConfig)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to create Client from config")
	}
	pClient := &peer2.PeerClient{
		Signer: signer.Sign,
		GRPCClient: peer2.GRPCClient{
			Client:  gClient,
			Address: address,
			Sn:      override,
		},
	}
	return pClient, nil
}
