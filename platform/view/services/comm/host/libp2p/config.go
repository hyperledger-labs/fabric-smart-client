/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"

type libp2pConfig interface {
	PrivateKeyPath() string
	Bootstrap() bool
	ListenAddress() host2.PeerIPAddress
	BootstrapListenAddress() host2.PeerIPAddress
}

type configService interface {
	GetString(key string) string
	GetPath(key string) string
}

type config struct {
	listenAddress          host2.PeerIPAddress
	bootstrapListenAddress host2.PeerIPAddress
	privateKeyPath         string
}

func NewConfig(cs configService) *config {
	return &config{
		listenAddress:          cs.GetString("fsc.p2p.listenAddress"),
		bootstrapListenAddress: cs.GetString("fsc.p2p.opts.libp2p.bootstrapNode"),
		privateKeyPath:         cs.GetPath("fsc.identity.key.file"),
	}
}

func (c *config) PrivateKeyPath() string                      { return c.privateKeyPath }
func (c *config) Bootstrap() bool                             { return len(c.bootstrapListenAddress) == 0 }
func (c *config) ListenAddress() host2.PeerIPAddress          { return c.listenAddress }
func (c *config) BootstrapListenAddress() host2.PeerIPAddress { return c.bootstrapListenAddress }
