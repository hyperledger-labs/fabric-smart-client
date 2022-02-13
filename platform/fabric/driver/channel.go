/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// Channel gives access to Fabric channel related information
type Channel interface {
	Committer
	Vault
	Delivery

	Ledger
	Comm
	Finality
	ChannelMembership
	TXIDStore
	ChaincodeManager

	// Name returns the name of the channel this instance is bound to
	Name() string

	EnvelopeService() EnvelopeService

	TransactionService() EndorserTransactionService

	MetadataService() MetadataService

	// NewPeerClientForAddress creates an instance of a Client using the
	// provided peer connection config
	NewPeerClientForAddress(cc grpc.ConnectionConfig) (peer.Client, error)

	// NewPeerClientForIdentity creates an instance of a Client using the
	// provided peer identity
	NewPeerClientForIdentity(peer view.Identity) (peer.Client, error)

	Close() error
}
