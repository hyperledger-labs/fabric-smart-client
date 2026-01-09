/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package services

import (
	"context"
	"crypto/tls"

	dclient "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/discovery"
	"github.com/hyperledger/fabric-protos-go-apiv2/discovery"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

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

	// Close closes this client
	Close()
}

type PeerClient interface {
	Client
	// EndorserClient returns an endorser client for the peer
	EndorserClient() (peer.EndorserClient, error)

	// DiscoveryClient returns a discovery client for the peer
	DiscoveryClient() (DiscoveryClient, error)

	// DeliverClient returns a deliver client for the peer
	DeliverClient() (peer.DeliverClient, error)
}

type OrdererClient interface {
	Client

	// OrdererClient returns an orderer client
	OrdererClient() (ab.AtomicBroadcastClient, error)
}
