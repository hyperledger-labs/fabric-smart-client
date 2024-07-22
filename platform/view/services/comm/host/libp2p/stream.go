/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package libp2p

import (
	"context"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/libp2p/go-libp2p/core/network"
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

func (s *stream) Context() context.Context { return context.TODO() }

func (s *stream) Close() error {
	// We don't close the stream here to recycle it later
	return nil
}

func streamHash(info host2.StreamInfo) host2.StreamHash {
	// This allows us to recycle the streams towards the same peer
	return info.RemotePeerID
}
