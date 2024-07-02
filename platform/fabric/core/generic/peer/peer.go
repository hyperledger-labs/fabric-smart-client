/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package peer

import (
	"context"
	"crypto/tls"

	grpc2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger/fabric-protos-go/discovery"
	"github.com/hyperledger/fabric-protos-go/peer"
	dclient "github.com/hyperledger/fabric/discovery/client"
	"google.golang.org/grpc"
)

type ClientFactory interface {
	NewClient(cc grpc2.ConnectionConfig) (Client, error)
}

// DiscoveryClient represents an interface for discovery service
type DiscoveryClient interface {
	// Send sends a request to the discovery service
	Send(ctx context.Context, req *dclient.Request, auth *discovery.AuthInfo) (dclient.Response, error)
}

// Client represents a client interface for the peer
type Client interface {
	// Address returns the address of the peer this client is connected to
	Address() string

	// Certificate returns the tls.Certificate used to make TLS connections
	// when client certificates are required by the server
	Certificate() tls.Certificate

	// Connection creates a new gRPC connection to the peer
	Connection() (*grpc.ClientConn, error)

	// EndorserClient returns an endorser client for the peer
	EndorserClient() (peer.EndorserClient, error)

	// DiscoveryClient returns a discovery client for the peer
	DiscoveryClient() (DiscoveryClient, error)

	// DeliverClient returns a deliver client for the peer
	DeliverClient() (peer.DeliverClient, error)

	// Close closes this client
	Close()
}
