/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package websocket

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"slices"
	"sync"

	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
)

type clientStreamProvider interface {
	NewClientStream(info host2.StreamInfo, ctx context.Context, src host2.PeerID, config *tls.Config) (host2.P2PStream, error)
	io.Closer
}

type client struct {
	// tlsConfig is the outbound TLS configuration trusting only the configured root CAs, or
	// nil when TLS is disabled. Every stream gets a clone extended with the extra CAs.
	tlsConfig      *tls.Config
	caPoolProvider ExtraCAPoolProvider
	nodeID         host2.PeerID
	streamProvider clientStreamProvider

	// mu guards the cached trust pool. OpenStream runs for every stream, including streams
	// that reuse an established connection, so the pool is rebuilt only when the extra CAs
	// change.
	mu       sync.Mutex
	extraCAs [][]byte
	rootCAs  *x509.CertPool
}

func (c *client) OpenStream(info host2.StreamInfo, ctx context.Context) (host2.P2PStream, error) {
	tlsConfig := c.tlsConfig
	if tlsConfig != nil {
		tlsConfig = tlsConfig.Clone()
		tlsConfig.RootCAs = c.currentRootCAs()
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

// currentRootCAs returns the configured root CAs plus the extra CAs the provider supplies now,
// so peers whose anchors are discovered after startup can be dialed. A withdrawn anchor is no
// longer trusted for new connections; a connection the stream provider already established and
// reuses for later streams stays open.
func (c *client) currentRootCAs() *x509.CertPool {
	var extraCAs [][]byte
	if c.caPoolProvider != nil {
		extraCAs = c.caPoolProvider.ExtraCAs()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rootCAs == nil || !slices.EqualFunc(extraCAs, c.extraCAs, bytes.Equal) {
		c.extraCAs, c.rootCAs = extraCAs, withExtraCAs(c.tlsConfig.RootCAs, extraCAs)
	}
	return c.rootCAs
}
