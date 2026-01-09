/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type ChannelProvider interface {
	Channel(name string) (driver.Channel, error)
}

type Manager struct {
	factories map[driver.TransactionType]driver.TransactionFactory
}

func NewManager() *Manager {
	return &Manager{factories: map[driver.TransactionType]driver.TransactionFactory{}}
}

func (m *Manager) ComputeTxID(id *driver.TxIDComponents) string {
	return ComputeTxID(id)
}

func (m *Manager) NewEnvelope() driver.Envelope {
	return NewEnvelope()
}

func (m *Manager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return NewProposalResponseFromBytes(raw)
}

func (m *Manager) NewTransaction(ctx context.Context, transactionType driver.TransactionType, creator view2.Identity, nonce []byte, txid string, channel string, rawRequest []byte) (driver.Transaction, error) {
	factory, ok := m.factories[transactionType]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", transactionType)
	}
	tx, err := factory.NewTransaction(ctx, channel, nonce, creator, txid, rawRequest)
	if err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: transactionType}, nil
}

func (m *Manager) NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	// logger.Debugf("new transaction from bytes [%s]", logging.Sha256Base64(raw))
	txRaw := &SerializedTransaction{}
	if err := json.Unmarshal(raw, txRaw); err != nil {
		return nil, err
	}
	factory, ok := m.factories[txRaw.Type]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", txRaw.Type)
	}
	tx, err := factory.NewTransaction(ctx, channel, nil, nil, "", nil)
	if err != nil {
		return nil, err
	}
	if err := tx.SetFromBytes(txRaw.Raw); err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: txRaw.Type}, nil
}

func (m *Manager) NewTransactionFromEnvelopeBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	cht, err := GetChannelHeaderType(raw)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to extract channel header type")
	}

	factory, ok := m.factories[driver.TransactionType(cht)]
	if !ok {
		return nil, errors.Errorf("transaction tyep [%d] not recognized", cht)
	}
	tx, err := factory.NewTransaction(ctx, channel, nil, nil, "", nil)
	if err != nil {
		return nil, err
	}
	err = tx.SetFromEnvelopeBytes(raw)
	if err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: driver.TransactionType(cht)}, nil
}

func (m *Manager) AddTransactionFactory(tt driver.TransactionType, factory driver.TransactionFactory) {
	m.factories[tt] = factory
}

func (m *Manager) NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (driver.ProcessedTransaction, int32, error) {
	return NewProcessedTransactionFromEnvelopePayload(envelopePayload)
}

func (m *Manager) NewProcessedTransactionFromEnvelopeRaw(envelope []byte) (driver.ProcessedTransaction, error) {
	return NewProcessedTransactionFromEnvelopeRaw(envelope)
}

func (m *Manager) NewProcessedTransaction(pt []byte) (driver.ProcessedTransaction, error) {
	return NewProcessedTransaction(pt)
}

type EndorserTransactionFactory struct {
	networkName     string
	channelProvider ChannelProvider
	sigService      driver.SignerService
}

func NewEndorserTransactionFactory(networkName string, channelProvider ChannelProvider, sigService driver.SignerService) *EndorserTransactionFactory {
	return &EndorserTransactionFactory{networkName: networkName, channelProvider: channelProvider, sigService: sigService}
}

func (e *EndorserTransactionFactory) NewTransaction(ctx context.Context, channel string, nonce []byte, creator []byte, txid string, rawRequest []byte) (driver.Transaction, error) {
	ch, err := e.channelProvider.Channel(channel)
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
		ctx:             ctx,
		channelProvider: e.channelProvider,
		sigService:      e.sigService,
		channel:         ch,
		TCreator:        creator,
		TNonce:          nonce,
		TTxID:           txid,
		TNetwork:        e.networkName,
		TChannel:        channel,
		TTransient:      map[string][]byte{},
	}, nil
}

type WrappedTransaction struct {
	driver.Transaction
	TransactionType driver.TransactionType
}

func (w *WrappedTransaction) Bytes() ([]byte, error) {
	// return w.Transaction.Bytes()
	raw, err := w.Transaction.Bytes()
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(&SerializedTransaction{
		Type: w.TransactionType,
		Raw:  raw,
	})
	// logger.Debugf("new transaction from bytes [%s]", logging.Sha256Base64(out))

	return out, err
}

type SerializedTransaction struct {
	Type driver.TransactionType
	Raw  []byte
}
