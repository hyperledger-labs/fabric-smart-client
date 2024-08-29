/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) ComputeTxID(id *driver.TxID) string {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewEnvelope() driver.Envelope {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewTransaction(transactionType driver.TransactionType, creator view.Identity, nonce []byte, txid string, channel string, rawRequest []byte) (driver.Transaction, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewTransactionFromBytes(channel string, raw []byte) (driver.Transaction, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewTransactionFromEnvelopeBytes(channel string, raw []byte) (driver.Transaction, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) AddTransactionFactory(tt driver.TransactionType, factory driver.TransactionFactory) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (driver.ProcessedTransaction, int32, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewProcessedTransactionFromEnvelopeRaw(envelope []byte) (driver.ProcessedTransaction, error) {
	//TODO implement me
	panic("implement me")
}

func (m *Manager) NewProcessedTransaction(pt []byte) (driver.ProcessedTransaction, error) {
	//TODO implement me
	panic("implement me")
}
