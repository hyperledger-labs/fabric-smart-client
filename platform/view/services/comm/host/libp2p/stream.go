/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/libp2p/go-libp2p/core/network"
)

type stream struct {
	network.Stream
}

func (s *stream) RemotePeerID() host2.PeerID {
	return s.Conn().RemotePeer().String()
}
func (s *stream) RemotePeerAddress() host2.PeerIPAddress {
	return s.Conn().RemoteMultiaddr().String()
}

func (s *stream) Hash() host2.StreamHash {
	return streamHash(s.RemotePeerID())
}

func streamHash(peerID host2.PeerID) host2.StreamHash {
	return peerID
}
