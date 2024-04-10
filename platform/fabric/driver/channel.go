/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

// Channel gives access to Fabric channel related information
type Channel interface {
	Committer

	Vault() Vault

	Delivery() Delivery

	Ledger() Ledger

	Finality() Finality

	ChannelMembership() ChannelMembership

	TXIDStore() TXIDStore

	ChaincodeManager() ChaincodeManager

	RWSetLoader() RWSetLoader

	// Name returns the name of the channel this instance is bound to
	Name() string

	EnvelopeService() EnvelopeService

	TransactionService() EndorserTransactionService

	MetadataService() MetadataService

	Close() error
}
