/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"time"

	commhost "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type libp2pConfig interface {
	PrivateKeyPath() string
	ListenAddress() commhost.PeerIPAddress
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
	listenAddress          commhost.PeerIPAddress
	privateKeyPath         string
	connManagerLowWater    int
	connManagerHighWater   int
	connManagerGracePeriod time.Duration
}

// bootstrapNodeKey names a bootstrap node; libp2p hosts ignore it.
const bootstrapNodeKey = "fsc.p2p.opts.libp2p.bootstrapNode"

func NewConfig(cs configService) *config {
	if v := cs.GetString(bootstrapNodeKey); v != "" {
		logger.Warnf("libp2p: ignoring %s [%s]: libp2p nodes need no bootstrap node, remove the key", bootstrapNodeKey, v)
	}

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
		privateKeyPath:         cs.GetPath("fsc.identity.key.file"),
		connManagerLowWater:    lowWater,
		connManagerHighWater:   highWater,
		connManagerGracePeriod: gracePeriod,
	}
}

func (c *config) PrivateKeyPath() string                { return c.privateKeyPath }
func (c *config) ListenAddress() commhost.PeerIPAddress { return c.listenAddress }
func (c *config) ConnManagerLowWater() int              { return c.connManagerLowWater }
func (c *config) ConnManagerHighWater() int             { return c.connManagerHighWater }
func (c *config) ConnManagerGracePeriod() time.Duration { return c.connManagerGracePeriod }
