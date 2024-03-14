/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	metrics2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/libp2p/go-libp2p/core/crypto"
)

func (p *hostProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, pk crypto.PrivKey) (host2.P2PHost, error) {
	return newLibP2PHost(listenAddress, pk, p.metrics, true, "")
}

func (p *hostProvider) NewHost(listenAddress, bootstrapListenAddress host2.PeerIPAddress, pk crypto.PrivKey) (host2.P2PHost, error) {
	return newLibP2PHost(listenAddress, pk, p.metrics, false, bootstrapListenAddress)
}

type hostGeneratorProvider struct {
	*hostProvider
}

func NewHostGeneratorProvider(provider metrics2.Provider) *hostGeneratorProvider {
	return &hostGeneratorProvider{
		hostProvider: newHostProvider(provider),
	}
}

func (p *hostGeneratorProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, privateKeyPath string, certPath string) (host2.P2PHost, error) {
	k, err := newCryptoPrivKeyFromMSP(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return p.hostProvider.NewBootstrapHost(listenAddress, k)
}

func (p *hostGeneratorProvider) NewHost(listenAddress host2.PeerIPAddress, privateKeyPath string, certPath string, bootstrapListenAddress host2.PeerIPAddress) (host2.P2PHost, error) {
	k, err := newCryptoPrivKeyFromMSP(privateKeyPath)
	if err != nil {
		return nil, err
	}
	return p.hostProvider.NewHost(listenAddress, bootstrapListenAddress, k)
}

type hostProvider struct {
	metrics *metrics
}

func newHostProvider(provider metrics2.Provider) *hostProvider {
	return &hostProvider{
		metrics: newMetrics(provider),
	}
}
