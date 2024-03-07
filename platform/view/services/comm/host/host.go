/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package host

import (
	"context"
	"io"
)

type PeerID = string
type PeerIPAddress = string

// GeneratorProvider provides the hosts and generates their PKs
type GeneratorProvider interface {
	NewBootstrapHost(listenAddress PeerIPAddress) (P2PHost, error)
	NewHost(listenAddress, bootstrapListenAddress PeerIPAddress) (P2PHost, error)
}

type P2PHost interface {
	// Start starts a new server that accepts connection requests
	Start(newStreamCallback func(stream P2PStream)) error
	// NewStream creates a new stream to a specific address.
	// If no address is passed, it is resolved using the peerID.
	NewStream(ctx context.Context, address PeerIPAddress, peerID PeerID) (P2PStream, error)
	// Lookup resolves all IP addresses that belong to a specific peerID
	Lookup(peerID PeerID) ([]PeerIPAddress, bool)
	// Wait waits until all dependencies are closed after we call P2PHost.Close
	Wait()

	io.Closer
}

type P2PStream interface {
	// RemotePeerID returns the peerID of the peer we are connected to (passed with P2PHost.NewStream)
	RemotePeerID() PeerID
	// RemotePeerAddress returns the IP address that we are connected to (passed or resolved in P2PHost.NewStream)
	RemotePeerAddress() PeerIPAddress

	io.Reader
	io.Writer
	io.Closer
}
