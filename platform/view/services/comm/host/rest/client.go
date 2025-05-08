/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rest

import (
	"context"
	"crypto/tls"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type clientStreamProvider interface {
	NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error)
}

type client struct {
	tlsConfig      *tls.Config
	nodeID         host2.PeerID
	streamProvider clientStreamProvider
}

func (c *client) OpenStream(info host2.StreamInfo, ctx context.Context) (host2.P2PStream, error) {
	return c.streamProvider.NewClientStream(info, ctx, c.nodeID, c.tlsConfig)
}
