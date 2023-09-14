/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"crypto/rand"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
)

type Factory interface {
	NewTransaction(channel string, nonce []byte, creator []byte, txid string) (driver.Transaction, error)
}

type Manager struct {
	sp        view.ServiceProvider
	fns       driver.FabricNetworkService
	factories map[driver.TransactionType]Factory
}

func NewManager(sp view.ServiceProvider, fns driver.FabricNetworkService) *Manager {
	factories := map[driver.TransactionType]Factory{}
	factories[driver.EndorserTransaction] = &EndorserTransactionFactory{sp: sp, fns: fns}
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

func (m *Manager) NewTransaction(transactionType driver.TransactionType, creator view2.Identity, nonce []byte, txid string, channel string) (driver.Transaction, error) {
	factory, ok := m.factories[transactionType]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", transactionType)
	}
	return factory.NewTransaction(channel, nonce, creator, txid)
}

func (m *Manager) NewTransactionFromBytes(channel string, raw []byte) (driver.Transaction, error) {
	ch, err := m.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	tx := &Transaction{
		sp:         m.sp,
		fns:        m.fns,
		channel:    ch,
		TChannel:   channel,
		TNetwork:   m.fns.Name(),
		TTransient: map[string][]byte{},
	}
	err = tx.SetFromBytes(raw)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (m *Manager) NewTransactionFromEnvelopeBytes(channel string, raw []byte) (driver.Transaction, error) {
	ch, err := m.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	tx := &Transaction{
		sp:         m.sp,
		fns:        m.fns,
		channel:    ch,
		TChannel:   channel,
		TNetwork:   m.fns.Name(),
		TTransient: map[string][]byte{},
	}
	err = tx.SetFromEnvelopeBytes(raw)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (m *Manager) AddFactory(tt driver.TransactionType, factory Factory) {
	m.factories[tt] = factory
}

func getRandomNonce() ([]byte, error) {
	key := make([]byte, 24)

	_, err := rand.Read(key)
	if err != nil {
		return nil, errors.Wrap(err, "error getting random bytes")
	}
	return key, nil
}

type EndorserTransactionFactory struct {
	sp  view.ServiceProvider
	fns driver.FabricNetworkService
}

func (e *EndorserTransactionFactory) NewTransaction(channel string, nonce []byte, creator []byte, txid string) (driver.Transaction, error) {
	ch, err := e.fns.Channel(channel)
	if err != nil {
		return nil, err
	}

	if len(nonce) == 0 {
		nonce, err = getRandomNonce()
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
