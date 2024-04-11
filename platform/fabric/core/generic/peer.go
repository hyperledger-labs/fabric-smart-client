/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
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

	secOpts, err := grpc.CreateSecOpts(cc, grpc.TLSClientConfig{
		TLSClientAuthRequired: c.ConfigService.TLSClientAuthRequired(),
		TLSClientKeyFile:      c.ConfigService.TLSClientKeyFile(),
		TLSClientCertFile:     c.ConfigService.TLSClientCertFile(),
	})
	if err != nil {
		return nil, err
	}

	timeout := c.ConfigService.ClientConnTimeout()
	if timeout <= 0 {
		timeout = grpc.DefaultConnectionTimeout
	}
	clientConfig := &grpc.ClientConfig{
		SecOpts: *secOpts,
		KaOpts: grpc.KeepaliveOptions{
			ClientInterval: c.ConfigService.KeepAliveClientInterval(),
			ClientTimeout:  c.ConfigService.KeepAliveClientTimeout(),
		},
		Timeout: timeout,
	}

	override := cc.ServerNameOverride
	if len(override) == 0 {
		override = c.ConfigService.TLSServerHostOverride()
	}

	return newPeerClientForClientConfig(
		c.Singer,
		cc.Address,
		override,
		*clientConfig,
	)
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
