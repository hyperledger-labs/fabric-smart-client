/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type ConfigService struct {
	driver.ConfigService
	OrderersValue    []*grpc.ConnectionConfig
	PoolSizeValue    int
	RetriesValue     int
	NetworkNameValue string
}

func (c *ConfigService) SetConfigOrderers(orderers []*grpc.ConnectionConfig) error {
	c.OrderersValue = orderers
	return nil
}

func (c *ConfigService) OrdererConnectionPoolSize() int {
	return c.PoolSizeValue
}

func (c *ConfigService) BroadcastNumRetries() int {
	return c.RetriesValue
}

func (c *ConfigService) BroadcastRetryInterval() time.Duration {
	return 0
}

func (c *ConfigService) NetworkName() string {
	return c.NetworkNameValue
}

func (c *ConfigService) Orderers() []*grpc.ConnectionConfig {
	return c.OrderersValue
}

func (c *ConfigService) PickOrderer() *grpc.ConnectionConfig {
	if len(c.OrderersValue) > 0 {
		return c.OrderersValue[0]
	}
	return nil
}
