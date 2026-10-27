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
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

// DiscoveredPeer is one peer of the result of a discovery peers query.
type DiscoveredPeer struct {
	MSPID      string   `yaml:"mspid,omitempty"`
	Endpoint   string   `yaml:"endpoint,omitempty"`
	Identity   string   `yaml:"identity,omitempty"`
	Chaincodes []string `yaml:"chaincodes,omitempty"`
}

// DiscoverPeers returns a function that asks p's discovery service, as user,
// for the peers of channelName. The function returns an error when the query
// fails, as it does while p has not joined the channel yet, so it can be
// polled with gomega.Eventually.
func DiscoverPeers(n *Network, p *topology.Peer, user, channelName string) func() ([]DiscoveredPeer, error) {
	return func() ([]DiscoveredPeer, error) {
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
		if err != nil {
			return nil, err
		}
		if code := sess.Wait(n.EventuallyTimeout).ExitCode(); code != 0 {
			return nil, errors.Errorf("discover peers on [%s] exited with [%d]: %s", p.Name, code, sess.Err.Contents())
		}

		var discovered []DiscoveredPeer
		if err := json.Unmarshal(sess.Out.Contents(), &discovered); err != nil {
			return nil, errors.Wrapf(err, "failed parsing discover peers output of [%s]", p.Name)
		}
		return discovered, nil
	}
}
