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
	"go.yaml.in/yaml/v3"

	"github.com/hyperledger-labs/fabric-smart-client/integration"
	"github.com/hyperledger-labs/fabric-smart-client/integration/fabric/common/faultrecovery"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/common"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric"
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
}

// Close closes the proxies.
func (p *FaultProxies) Close() {
	for _, proxy := range []*common.Proxy{p.Peer, p.Postgres, p.Bob} {
		_ = proxy.Close()
	}
}

// SetFaultProxies starts a proxy in front of alice's peer, bob's P2P port and bob's
// database, points the core.yaml of alice and bob at them, and enables alice's gRPC client
// keepalive (see KeepaliveTime). Every other link is direct. It assumes alice and bob have
// one replica each. Call it after the network is generated and before it starts; the
// proxies forward until Close.
func SetFaultProxies(ii *integration.Infrastructure) *FaultProxies {
	n := fabric.Network(ii.NWOCtx, "default").Network
	peer := n.PeerAddress(n.PeerByName("Org1_peer_0"), network.ListenPort)
	db := faultrecovery.DataSource(ii, "bob")
	bob := ii.FscPlatform.PeerAddress(faultrecovery.Replica(ii, "bob"), fsc.P2PPort)

	p := &FaultProxies{
		Peer:     startProxy(peer),
		Postgres: startProxy(db.Host + ":" + db.Port),
		Bob:      startProxy(bob),
	}
	_, dbPort, err := net.SplitHostPort(p.Postgres.Addr())
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	// Every persistence of bob shares one dataSource.
	rewrite(ii, "bob", nil, "host="+db.Host+" port="+db.Port+" ", "host=127.0.0.1 port="+dbPort+" ")
	rewrite(ii, "alice", func(core map[string]any) {
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

// rewrite replaces each old address with its new one in the core.yaml of node, then
// applies patch, if any, to the parsed configuration. Each old address must occur exactly
// once in the file, so the replacement rewrites only the link it names.
func rewrite(ii *integration.Infrastructure, node string, patch func(core map[string]any), oldnew ...string) {
	path := ii.FscPlatform.NodeConfigPath(faultrecovery.Replica(ii, node))
	raw, err := os.ReadFile(path)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	cfg := string(raw)
	for i := 0; i < len(oldnew); i += 2 {
		gomega.ExpectWithOffset(1, strings.Count(cfg, oldnew[i])).To(gomega.Equal(1), "[%s] in core.yaml of [%s]", oldnew[i], node)
	}
	cfg = strings.NewReplacer(oldnew...).Replace(cfg)
	if patch != nil {
		var core map[string]any
		gomega.ExpectWithOffset(1, yaml.Unmarshal([]byte(cfg), &core)).To(gomega.Succeed())
		patch(core)
		out, err := yaml.Marshal(core)
		gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
		cfg = string(out)
	}
	gomega.ExpectWithOffset(1, os.WriteFile(path, []byte(cfg), 0o600)).To(gomega.Succeed())
}

func startProxy(backend string) *common.Proxy {
	p, err := common.StartProxy("127.0.0.1:0", backend)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return p
}
