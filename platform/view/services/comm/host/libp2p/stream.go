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
	//s.Stream.Close()
	return nil
}

func streamHash(info host2.StreamInfo) host2.StreamHash {
	//return fmt.Sprintf("%s.%s.%s.%s", info.RemotePeerID, info.RemotePeerAddress, info.SessionID, info.ContextID)
	return info.RemotePeerID
}
