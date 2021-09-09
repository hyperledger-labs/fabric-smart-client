/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var logger = flogging.MustGetLogger("fabric-sdk.chaincode")

type SignerProvider interface {
	GetSigningIdentity(identity view.Identity) (*view2.SigningIdentity, error)
}

type SerializableSigner interface {
	Sign(message []byte) ([]byte, error)

	Serialize() ([]byte, error)
}

type Network interface {
	Peers() []*grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
	// Broadcast sends the passed blob to the ordering service to be ordered
	Broadcast(blob interface{}) error
	SignerService() driver.SignerService
}

type Channel interface {
	Name() string
	// NewPeerClientForAddress creates an instance of a PeerClient using the
	// provided peer connection config
	NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.PeerClient, error)

	// NewPeerClientForIdentity creates an instance of a PeerClient using the
	// provided peer identity
	NewPeerClientForIdentity(peer view.Identity) (peer.PeerClient, error)

	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(txID string) error

	MSPManager() driver.MSPManager
}
