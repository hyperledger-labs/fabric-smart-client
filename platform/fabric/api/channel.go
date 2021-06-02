/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package api

// Channel gives access to Fabric channel related information
type Channel interface {
	Committer
	Vault

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
}
