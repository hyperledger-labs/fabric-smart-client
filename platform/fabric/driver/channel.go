/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// Channel gives access to Fabric channel related information
type Channel interface {
	// Name returns the name of the channel this instance is bound to
	Name() string

	Committer() Committer

	Vault() Vault

	Delivery() Delivery

	Ledger() Ledger

	Finality() Finality

	ChannelMembership() ChannelMembership

	ChaincodeManager() ChaincodeManager

	RWSetLoader() RWSetLoader

	EnvelopeService() EnvelopeService

	TransactionService() EndorserTransactionService

	MetadataService() MetadataService

	// ChannelKey returns the key that identifies this channel to other nodes. It
	// fails while block 0 of the channel cannot be read.
	ChannelKey() (*ChannelKey, error)

	Close() error
}
