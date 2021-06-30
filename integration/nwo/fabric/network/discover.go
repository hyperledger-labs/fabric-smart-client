/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package network

import (
	"encoding/json"
	"path/filepath"

	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/commands"
	"github.com/hyperledger-labs/fabric-smart-client/integration/nwo/fabric/topology"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

// DiscoveredPeer defines a struct for discovering peers using discovery service.
// each peer in the result will have these fields
type DiscoveredPeer struct {
	MSPID      string   `yaml:"mspid,omitempty"`
	Endpoint   string   `yaml:"endpoint,omitempty"`
	Identity   string   `yaml:"identity,omitempty"`
	Chaincodes []string `yaml:"chaincodes,omitempty"`
}

// running discovery service command discover peers against peer using channel name and user as specified in the
// function arguments. return a slice of the discovered peers
func DiscoverPeers(n *Network, p *topology.Peer, user, channelName string) func() []DiscoveredPeer {
	return func() []DiscoveredPeer {
		peers := commands.Peers{
			NetworkPrefix: n.Prefix,
			UserCert:      n.PeerUserCert(p, user),
			UserKey:       n.PeerUserKey(p, user),
			MSPID:         n.Organization(p.Organization).MSPID,
			Server:        n.PeerAddress(p, ListenPort),
			Channel:       channelName,
		}
		if n.ClientAuthRequired {
			peers.ClientCert = filepath.Join(n.PeerUserTLSDir(p, user), "client.crt")
			peers.ClientKey = filepath.Join(n.PeerUserTLSDir(p, user), "client.key")
		}
		sess, err := n.Discover(peers)
		Expect(err).NotTo(HaveOccurred())
		Eventually(sess, n.EventuallyTimeout).Should(gexec.Exit(0))

		var discovered []DiscoveredPeer
		err = json.Unmarshal(sess.Out.Contents(), &discovered)
		Expect(err).NotTo(HaveOccurred())
		return discovered
	}
}
