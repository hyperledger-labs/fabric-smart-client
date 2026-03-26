/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"time"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type libp2pConfig interface {
	PrivateKeyPath() string
	Bootstrap() bool
	ListenAddress() host2.PeerIPAddress
	BootstrapListenAddress() host2.PeerIPAddress
	ConnManagerLowWater() int
	ConnManagerHighWater() int
	ConnManagerGracePeriod() time.Duration
}

type configService interface {
	GetString(key string) string
	GetPath(key string) string
	GetInt(key string) int
	IsSet(key string) bool
}

type config struct {
	listenAddress          host2.PeerIPAddress
	bootstrapListenAddress host2.PeerIPAddress
	privateKeyPath         string
	connManagerLowWater    int
	connManagerHighWater   int
	connManagerGracePeriod time.Duration
}

func NewConfig(cs configService) *config {
	lowWater := 100
	if cs.IsSet("fsc.p2p.opts.libp2p.connManager.lowWater") {
		lowWater = cs.GetInt("fsc.p2p.opts.libp2p.connManager.lowWater")
	}

	highWater := 400
	if cs.IsSet("fsc.p2p.opts.libp2p.connManager.highWater") {
		highWater = cs.GetInt("fsc.p2p.opts.libp2p.connManager.highWater")
	}

	gracePeriod := time.Minute
	if cs.IsSet("fsc.p2p.opts.libp2p.connManager.gracePeriod") {
		gracePeriod = time.Duration(cs.GetInt("fsc.p2p.opts.libp2p.connManager.gracePeriod")) * time.Second
	}

	return &config{
		listenAddress:          cs.GetString("fsc.p2p.listenAddress"),
		bootstrapListenAddress: cs.GetString("fsc.p2p.opts.libp2p.bootstrapNode"),
		privateKeyPath:         cs.GetPath("fsc.identity.key.file"),
		connManagerLowWater:    lowWater,
		connManagerHighWater:   highWater,
		connManagerGracePeriod: gracePeriod,
	}
}

func (c *config) PrivateKeyPath() string                      { return c.privateKeyPath }
func (c *config) Bootstrap() bool                             { return len(c.bootstrapListenAddress) == 0 }
func (c *config) ListenAddress() host2.PeerIPAddress          { return c.listenAddress }
func (c *config) BootstrapListenAddress() host2.PeerIPAddress { return c.bootstrapListenAddress }
func (c *config) ConnManagerLowWater() int                    { return c.connManagerLowWater }
func (c *config) ConnManagerHighWater() int                   { return c.connManagerHighWater }
func (c *config) ConnManagerGracePeriod() time.Duration       { return c.connManagerGracePeriod }
