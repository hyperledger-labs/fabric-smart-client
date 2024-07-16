/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import "time"

type NetworkConfig interface {
	FinalityEventQueueWorkers() int
	FinalityNumRetries() int
	FinalitySleepTime() time.Duration
	PollingTimeout() time.Duration
}

type NetworkConfigProvider interface {
	GetNetworkConfig(network string) (NetworkConfig, error)
}
