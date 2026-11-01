/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package faultrecovery

import (
	"net"
	"os"
	"strings"
	"time"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/network"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fsc"
)

// KeepaliveTime and KeepaliveTimeout are the gRPC client keepalive SetFaultProxies gives
// alice's Fabric connections. KeepaliveTime is the 60 s minimum ping interval that nwo's
// peers and orderers enforce; a node that pings more often is disconnected. A silent
// partition of the delivery peer is therefore detected within their sum.
const (
	KeepaliveTime    = 60 * time.Second
	KeepaliveTimeout = 10 * time.Second
)

// FaultProxies are the proxies SetFaultProxies puts in the links of alice and bob. Partition
// and Heal on one of them cut and restore that link without either side seeing it close.
type FaultProxies struct {
	// Peer forwards to Org1_peer_0, the only peer alice's delivery and finality use.
	Peer *common.Proxy
	// Postgres forwards to bob's database. Bob's, not alice's: alice's views read alice's
	// vault, so alice could not order a transaction while hers is cut off.
	Postgres *common.Proxy
	// Bob forwards alice's P2P connections to bob.
	Bob *common.Proxy
	// Orderers forward to the orderers, one per entry of Orderers. The channel
	// configuration lists them as the ordering endpoints, so every client of the
	// orderers, FSC nodes and peers alike, broadcasts and pulls blocks through them.
	Orderers []*common.Proxy
}

// Close closes the proxies.
func (p *FaultProxies) Close() {
	for _, proxy := range append([]*common.Proxy{p.Peer, p.Postgres, p.Bob}, p.Orderers...) {
		_ = proxy.Close()
	}
}

// SetFaultProxies starts a proxy in front of alice's peer, bob's P2P port, bob's database
// and each orderer, points the core.yaml of alice and bob at the first three, and enables
// alice's gRPC client keepalive (see KeepaliveTime). FSC reads the ordering endpoints from
// the channel configuration, not from core.yaml, so SetFaultProxies lists the orderer
// proxies as the ordering endpoints in configtx.yaml and builds the channel genesis block
// again. Every other link, the raft traffic between orderers included, is direct. It
// assumes alice and bob have one replica each. Call it after the network is generated and
// before it starts; the proxies forward until Close.
func SetFaultProxies(ii *integration.Infrastructure) *FaultProxies {
	n := fabric.Network(ii.NWOCtx, "default").Network
	peer := n.PeerAddress(n.PeerByName("Org1_peer_0"), network.ListenPort)
	db := faultrecovery.DataSource(ii, "bob")
	bob := ii.FscPlatform.PeerAddress(faultrecovery.Replica(ii, "bob"), fsc.P2PPort)

	p := &FaultProxies{
		Peer:     faultrecovery.StartProxy(peer),
		Postgres: faultrecovery.StartProxy(db.Host + ":" + db.Port),
		Bob:      faultrecovery.StartProxy(bob),
	}
	p.Orderers = proxyOrderers(n)
	_, dbPort, err := net.SplitHostPort(p.Postgres.Addr())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	// Every persistence of bob shares one dataSource.
	faultrecovery.RewriteCore(ii, "bob", nil, "host="+db.Host+" port="+db.Port+" ", "host=127.0.0.1 port="+dbPort+" ")
	faultrecovery.RewriteCore(ii, "alice", func(core map[string]any) {
		network, ok := core["fabric"].(map[string]any)["default"].(map[string]any)
		gomega.Expect(ok).To(gomega.BeTrue(), "core.yaml of alice has no fabric.default")
		network["keepalive"] = map[string]any{
			"time":                  KeepaliveTime.String(),
			"timeout":               KeepaliveTimeout.String(),
			"permit-without-stream": true,
		}
	}, peer, p.Peer.Addr(), bob, p.Bob.Addr())
	return p
}

// proxyOrderers starts one proxy per orderer of n, replaces each orderer's listen address
// with its proxy in the OrdererEndpoints of configtx.yaml, and builds the genesis block of
// every channel again from it. Each listen address must occur exactly once in configtx.yaml,
// so only the ordering endpoint changes.
func proxyOrderers(n *network.Network) []*common.Proxy {
	path := n.ConfigTxConfigPath()
	raw, err := os.ReadFile(path)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	cfg := string(raw)
	proxies := make([]*common.Proxy, len(n.Orderers))
	for i, o := range n.Orderers {
		addr := n.OrdererAddress(o, network.ListenPort)
		gomega.ExpectWithOffset(1, strings.Count(cfg, "- "+addr+"\n")).To(gomega.Equal(1), "ordering endpoint [%s] in configtx.yaml", addr)
		proxies[i] = faultrecovery.StartProxy(addr)
		cfg = strings.Replace(cfg, "- "+addr+"\n", "- "+proxies[i].Addr()+"\n", 1)
	}
	gomega.ExpectWithOffset(1, os.WriteFile(path, []byte(cfg), 0o600)).To(gomega.Succeed())

	for _, c := range n.Channels {
		sess, err := n.ConfigTxGen(commands.OutputBlock{
			NetworkPrefix: n.Prefix,
			ChannelID:     c.Name,
			Profile:       c.Profile,
			ConfigPath:    n.ConfigDir(),
			OutputBlock:   n.OutputBlockPath(c.Name),
		})
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
		gomega.EventuallyWithOffset(1, sess, n.EventuallyTimeout).Should(gexec.Exit(0))
	}
	return proxies
}
