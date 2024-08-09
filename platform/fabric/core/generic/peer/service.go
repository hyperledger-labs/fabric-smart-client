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

type Service struct {
	ConnCache ClientFactory
}

func NewService(configService driver.ConfigService, signer driver.Signer) *Service {
	return &Service{ConnCache: NewCachingClientFactory(configService, signer)}
}

func (c *Service) NewClient(cc grpc.ConnectionConfig) (Client, error) {
	return c.ConnCache.NewClient(cc)
}
