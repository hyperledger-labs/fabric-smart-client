/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type Factory interface {
	NewTransaction(channel string, nonce []byte, creator []byte, txid string, rawRequest []byte) (driver.Transaction, error)
}

type Manager struct {
	sp        view.ServiceProvider
	fns       driver.FabricNetworkService
	factories map[driver.TransactionType]Factory
}

func NewManager(sp view.ServiceProvider, fns driver.FabricNetworkService) *Manager {
	factories := map[driver.TransactionType]Factory{}
	factories[driver.EndorserTransaction] = NewEndorserTransactionFactory(sp, fns)
	return &Manager{sp: sp, fns: fns, factories: factories}
}

func (m *Manager) ComputeTxID(id *driver.TxID) string {
	return ComputeTxID(id)
}

func (m *Manager) NewEnvelope() driver.Envelope {
	return NewEnvelope()
}

func (m *Manager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return NewProposalResponseFromBytes(raw)
}

func (m *Manager) NewTransaction(transactionType driver.TransactionType, creator view2.Identity, nonce []byte, txid string, channel string, rawRequest []byte) (driver.Transaction, error) {
	factory, ok := m.factories[transactionType]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", transactionType)
	}
	tx, err := factory.NewTransaction(channel, nonce, creator, txid, rawRequest)
	if err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: transactionType}, nil
}

func (m *Manager) NewTransactionFromBytes(channel string, raw []byte) (driver.Transaction, error) {
	//logger.Infof("new transaction from bytes [%s]", hash.Hashable(raw))
	txRaw := &SerializedTransaction{}
	if err := json.Unmarshal(raw, txRaw); err != nil {
		return nil, err
	}
	factory, ok := m.factories[txRaw.Type]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", txRaw.Type)
	}
	tx, err := factory.NewTransaction(channel, nil, nil, "", nil)
	if err != nil {
		return nil, err
	}
	if err := tx.SetFromBytes(txRaw.Raw); err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: txRaw.Type}, nil
}

func (m *Manager) NewTransactionFromEnvelopeBytes(channel string, raw []byte) (driver.Transaction, error) {
	cht, err := GetChannelHeaderType(raw)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to extract channel header type")
	}

	factory, ok := m.factories[driver.TransactionType(cht)]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", cht)
	}
	tx, err := factory.NewTransaction(channel, nil, nil, "", nil)
	if err != nil {
		return nil, err
	}
	err = tx.SetFromEnvelopeBytes(raw)
	if err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: driver.TransactionType(cht)}, nil
}

func (m *Manager) AddFactory(tt driver.TransactionType, factory Factory) {
	m.factories[tt] = factory
}

type EndorserTransactionFactory struct {
	sp  view.ServiceProvider
	fns driver.FabricNetworkService
}

func NewEndorserTransactionFactory(sp view.ServiceProvider, fns driver.FabricNetworkService) *EndorserTransactionFactory {
	return &EndorserTransactionFactory{sp: sp, fns: fns}
}

func (e *EndorserTransactionFactory) NewTransaction(channel string, nonce []byte, creator []byte, txid string, rawRequest []byte) (driver.Transaction, error) {
	ch, err := e.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	if len(nonce) == 0 {
		nonce, err = GetRandomNonce()
		if err != nil {
			return nil, err
		}
	}
	if len(txid) == 0 {
		txid = protoutil.ComputeTxID(nonce, creator)
	}

	return &Transaction{
		sp:         e.sp,
		fns:        e.fns,
		channel:    ch,
		TCreator:   creator,
		TNonce:     nonce,
		TTxID:      txid,
		TNetwork:   e.fns.Name(),
		TChannel:   channel,
		TTransient: map[string][]byte{},
	}, nil
}

type WrappedTransaction struct {
	driver.Transaction
	TransactionType driver.TransactionType
}

func (w *WrappedTransaction) Bytes() ([]byte, error) {
	//return w.Transaction.Bytes()
	raw, err := w.Transaction.Bytes()
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(&SerializedTransaction{
		Type: w.TransactionType,
		Raw:  raw,
	})
	//logger.Infof("new transaction from bytes [%s]", hash.Hashable(out))

	return out, err
}

type SerializedTransaction struct {
	Type driver.TransactionType
	Raw  []byte
}
