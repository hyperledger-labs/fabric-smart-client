/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	commhost "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	viewmetrics "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
)

// P2PCommunicationType is a string identifier for the libp2p implementation of the p2p comm stack.
const P2PCommunicationType = "libp2p"

type hostGeneratorProvider struct {
	metrics *metrics
	config  libp2pConfig
}

func NewHostGeneratorProvider(config libp2pConfig, provider viewmetrics.Provider) *hostGeneratorProvider {
	return &hostGeneratorProvider{
		metrics: newMetrics(provider),
		config:  config,
	}
}

// GetNewHost creates a host with the key at config.PrivateKeyPath.
func (p *hostGeneratorProvider) GetNewHost() (commhost.P2PHost, error) {
	logger.Debugf("libp2p: Generating new host, privateKeyPath [%s]...", p.config.PrivateKeyPath())
	k, err := newCryptoPrivKeyFromMSP(p.config.PrivateKeyPath())
	if err != nil {
		return nil, err
	}
	return newLibP2PHost(p.config, k, p.metrics)
}
