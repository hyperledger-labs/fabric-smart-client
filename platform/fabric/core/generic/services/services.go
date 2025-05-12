/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

var logger = logging.MustGetLogger()

type clientFactory interface {
	NewPeerClient(cc grpc.ConnectionConfig) (PeerClient, error)
	NewOrdererClient(cc grpc.ConnectionConfig) (OrdererClient, error)
}

type ClientFactory struct {
	factory clientFactory
}

func NewClientFactory(configService driver.ConfigService, signer driver.Signer) *ClientFactory {
	return &ClientFactory{factory: NewCachingClientFactory(configService, signer)}
}

func (c *ClientFactory) NewPeerClient(cc grpc.ConnectionConfig) (PeerClient, error) {
	return c.factory.NewPeerClient(cc)
}

func (c *ClientFactory) NewOrdererClient(cc grpc.ConnectionConfig) (OrdererClient, error) {
	return c.factory.NewOrdererClient(cc)
}
