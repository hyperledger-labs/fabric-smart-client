/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = flogging.MustGetLogger("fabric-sdk.core.generic.peer")

type Manager struct {
	ConnCache ClientFactory
}

func NewPeerManager(configService driver.ConfigService, signer driver.Signer) *Manager {
	return &Manager{
		ConnCache: &CachingClientFactory{
			Cache: map[string]Client{},
			ClientFactory: &GRPCClientFactory{
				ConfigService: configService,
				Singer:        signer,
			},
			Signer: signer,
		},
	}
}

func (c *Manager) NewClient(cc grpc.ConnectionConfig) (Client, error) {
	return c.ConnCache.NewClient(cc)
}
