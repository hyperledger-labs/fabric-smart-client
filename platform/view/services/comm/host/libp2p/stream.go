/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"

	"github.com/libp2p/go-libp2p/core/network"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type stream struct {
	network.Stream
	info host2.StreamInfo
}

func (s *stream) RemotePeerID() host2.PeerID {
	return s.Conn().RemotePeer().String()
}

func (s *stream) RemotePeerAddress() host2.PeerIPAddress {
	return s.Conn().RemoteMultiaddr().String()
}

func (s *stream) Hash() host2.StreamHash {
	return streamHash(s.info)
}

func (*stream) Context() context.Context { return context.TODO() }

func (s *stream) Close() error {
	logger.Debugf("libp2p: Closing stream to [%s] (address: [%s], hash: [%s])...", s.RemotePeerID(), s.RemotePeerAddress(), s.Hash())
	err := s.Stream.Close()
	logger.Debugf("libp2p: stream to [%s] closed with error [%v]", s.RemotePeerID(), err)
	return err
}

// streamHash keys streams by remote peer only, so all sessions to a peer share its streams.
// An incoming stream carries no session metadata, so a per-session key would never match it and
// every reply would open a second stream, quickly reaching libp2p's per-peer stream limit.
// Shared streams are not closed when a session closes (see SharedStreams).
func streamHash(info host2.StreamInfo) host2.StreamHash {
	return info.RemotePeerID
}
