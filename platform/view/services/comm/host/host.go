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
type StreamHash = string

// GeneratorProvider provides the hosts and generates their PKs
type GeneratorProvider interface {
	NewBootstrapHost(listenAddress PeerIPAddress, privateKeyPath string, certPath string) (P2PHost, error)
	NewHost(listenAddress PeerIPAddress, privateKeyPath string, certPath string, bootstrapListenAddress PeerIPAddress) (P2PHost, error)
}

type StreamInfo struct {
	// Always available
	RemotePeerID PeerID
	// Not available before creation of stream
	RemotePeerAddress PeerIPAddress
	// Not available when web socket returns an error
	ContextID string
	// Not available when web socket returns an error
	SessionID string
}

type P2PHost interface {
	// Start starts a new server that accepts connection requests
	Start(newStreamCallback func(stream P2PStream)) error
	// NewStream creates a new stream to a specific address.
	// If no address is passed, it is resolved using the peerID.
	// The new stream must return a hash (see P2PStream.Hash) identical to the one
	// returned by P2PHost.StreamHash using the same StreamInfo as input.
	NewStream(ctx context.Context, info StreamInfo) (P2PStream, error)
	// Lookup resolves all IP addresses that belong to a specific peerID
	Lookup(peerID PeerID) ([]PeerIPAddress, bool)
	// StreamHash calculates the hash of a session. Must be identical to the one calculated by P2PStream.Hash.
	// Sessions that share the same stream hash will be multiplexed into the same stream.
	// If the hash is empty, a new stream must be created
	StreamHash(info StreamInfo) StreamHash
	// Wait waits until all dependencies are closed after we call P2PHost.Close
	Wait()

	io.Closer
}

type P2PStream interface {
	// RemotePeerID returns the peerID of the peer we are connected to (passed with P2PHost.NewStream)
	RemotePeerID() PeerID
	// RemotePeerAddress returns the IP address that we are connected to (passed or resolved in P2PHost.NewStream)
	RemotePeerAddress() PeerIPAddress
	// Hash calculates the hash of the stream.
	// When we attempt to open a new stream that hashes to the same value, the existing stream will be re-used.
	Hash() StreamHash
	// Context returns the context of the stream
	Context() context.Context

	io.Reader
	io.Writer
	io.Closer
}
