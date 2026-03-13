/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"crypto/tls"
	"io"
	"net"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type clientStreamProvider interface {
	NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error)
	io.Closer
}

type client struct {
	tlsConfig      *tls.Config
	nodeID         host2.PeerID
	streamProvider clientStreamProvider
}

func (c *client) OpenStream(info host2.StreamInfo, ctx context.Context) (host2.P2PStream, error) {
	tlsConfig := c.tlsConfig
	if tlsConfig != nil {
		tlsConfig = tlsConfig.Clone()
		if len(tlsConfig.ServerName) == 0 && len(info.RemotePeerAddress) != 0 {
			// use info.RemotePeerAddress to get the host
			host, _, err := net.SplitHostPort(info.RemotePeerAddress)
			if err == nil {
				tlsConfig.ServerName = host
			} else {
				tlsConfig.ServerName = info.RemotePeerAddress
			}
		}
	}
	return c.streamProvider.NewClientStream(info, ctx, c.nodeID, tlsConfig)
}
