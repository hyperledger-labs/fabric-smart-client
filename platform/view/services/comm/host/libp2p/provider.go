/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/libp2p/go-libp2p/core/crypto"
)

type HostProvider interface {
	NewBootstrapHost(listenAddress host2.PeerIPAddress, pk crypto.PrivKey) (host2.P2PHost, error)
	NewHost(listenAddress, bootstrapListenAddress host2.PeerIPAddress, pk crypto.PrivKey) (host2.P2PHost, error)
}

type hostProvider struct {
	metrics *Metrics
}

func NewHostProvider(provider metrics.Provider) *hostProvider {
	return &hostProvider{
		metrics: newMetrics(provider),
	}
}

func (p *hostProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress, pk crypto.PrivKey) (host2.P2PHost, error) {
	return newLibP2PHost(listenAddress, pk, p.metrics, true, "")
}

func (p *hostProvider) NewHost(listenAddress, bootstrapListenAddress host2.PeerIPAddress, pk crypto.PrivKey) (host2.P2PHost, error) {
	return newLibP2PHost(listenAddress, pk, p.metrics, false, bootstrapListenAddress)
}

type hostGeneratorProvider struct {
	*hostProvider
	key crypto.PrivKey
}

func NewHostGeneratorProvider(provider metrics.Provider, mspPath string) (*hostGeneratorProvider, error) {
	k, err := newCryptoPrivKeyFromMSP(mspPath)
	if err != nil {
		return nil, err
	}
	return &hostGeneratorProvider{
		hostProvider: NewHostProvider(provider),
		key:          k,
	}, nil
}

func (p *hostGeneratorProvider) NewBootstrapHost(listenAddress host2.PeerIPAddress) (host2.P2PHost, error) {
	return p.hostProvider.NewBootstrapHost(listenAddress, p.key)
}

func (p *hostGeneratorProvider) NewHost(listenAddress, bootstrapListenAddress host2.PeerIPAddress) (host2.P2PHost, error) {
	return p.hostProvider.NewHost(listenAddress, bootstrapListenAddress, p.key)
}
