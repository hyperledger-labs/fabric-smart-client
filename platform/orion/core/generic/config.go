/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
)

type staticNetworkConfigProvider struct {
	*networkConfig
}

func NewStaticNetworkConfigProvider(c *networkConfig) *staticNetworkConfigProvider {
	return &staticNetworkConfigProvider{networkConfig: c}
}

func (p *staticNetworkConfigProvider) GetNetworkConfig(string) (driver.NetworkConfig, error) {
	return p.networkConfig, nil
}

type networkConfig struct {
	finalityEventQueueWorkers int
	finalityNumRetries        int
	finalitySleepTime         time.Duration
	pollingTimeout            time.Duration
}

func (c *networkConfig) FinalityEventQueueWorkers() int {
	return c.finalityEventQueueWorkers
}

func (c *networkConfig) WithFinalityEventQueueWorkers(v int) *networkConfig {
	c.finalityEventQueueWorkers = v
	return c
}

func (c *networkConfig) FinalitySleepTime() time.Duration {
	return c.finalitySleepTime
}

func (c *networkConfig) WithFinalitySleepTime(v time.Duration) *networkConfig {
	c.finalitySleepTime = v
	return c
}

func (c *networkConfig) PollingTimeout() time.Duration {
	return c.pollingTimeout
}

func (c *networkConfig) WithPollingTimeout(v time.Duration) *networkConfig {
	c.pollingTimeout = v
	return c
}

func (c *networkConfig) FinalityNumRetries() int {
	return c.finalityNumRetries
}

func (c *networkConfig) WithFinalityNumRetries(v int) *networkConfig {
	c.finalityNumRetries = v
	return c
}

func NewNetworkConfig() *networkConfig {
	return &networkConfig{
		finalityEventQueueWorkers: 1,
		finalityNumRetries:        3,
		finalitySleepTime:         100 * time.Millisecond,
		pollingTimeout:            100 * time.Millisecond,
	}
}
