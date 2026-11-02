/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

func (*Manager) ComputeTxID(id *driver.TxIDComponents) string {
	return ComputeTxID(id)
}

func (*Manager) NewEnvelope() driver.Envelope {
	return NewEnvelope()
}

func (*Manager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return NewProposalResponseFromBytes(raw)
}

func (m *Manager) NewTransaction(ctx context.Context, transactionType driver.TransactionType, creator view.Identity, nonce []byte, txid, channel string, rawRequest []byte) (driver.Transaction, error) {
	factory, ok := m.factories[transactionType]
	if !ok {
		return nil, errors.Errorf("transaction type [%d] not recognized", transactionType)
	}
	tx, err := factory.NewTransaction(ctx, channel, nonce, creator, txid, rawRequest)
	if err != nil {
		return nil, err
	}
	return &WrappedTransaction{Transaction: tx, TransactionType: transactionType}, nil
}

// NewTransactionFromBytes decodes a transaction serialized by WrappedTransaction.Bytes
// on this network, bound to the channel the transaction names. It does not check
// that the bytes were built on this network; a caller decoding bytes received from
// another node picks the network by SerializedTransaction.Key first.
func (m *Manager) NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	txRaw := &SerializedTransaction{}
	if err := json.Unmarshal(raw, txRaw); err != nil {
		return nil, err
	}
	factory, ok := m.factories[txRaw.Type]
	if !ok {
		return nil, errors.Errorf("transaction type [%d] not recognized", txRaw.Type)
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
		return nil, errors.WithMessage(err, "failed to extract channel header type")
	}

	factory, ok := m.factories[driver.TransactionType(cht)]
	if !ok {
		return nil, errors.Errorf("transaction type [%d] not recognized", cht)
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

func (*Manager) NewProcessedTransactionFromEnvelopePayload(envelopePayload []byte) (driver.ProcessedTransaction, int32, error) {
	return NewProcessedTransactionFromEnvelopePayload(envelopePayload)
}

func (*Manager) NewProcessedTransactionFromEnvelopeRaw(envelope []byte) (driver.ProcessedTransaction, error) {
	return NewProcessedTransactionFromEnvelopeRaw(envelope)
}

func (*Manager) NewProcessedTransaction(pt []byte) (driver.ProcessedTransaction, error) {
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

func (e *EndorserTransactionFactory) NewTransaction(ctx context.Context, channel string, nonce, creator []byte, txid string, _ []byte) (driver.Transaction, error) {
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

// WrappedTransaction is a driver.Transaction that serializes together with its
// transaction type and the ChannelKey of its channel, so that a receiving node can
// pick both the local network and the transaction factory.
type WrappedTransaction struct {
	driver.Transaction
	TransactionType driver.TransactionType
}

func (w *WrappedTransaction) Bytes() ([]byte, error) {
	raw, err := w.Transaction.Bytes()
	if err != nil {
		return nil, err
	}
	return w.wrap(raw)
}

func (w *WrappedTransaction) BytesNoTransient() ([]byte, error) {
	raw, err := w.Transaction.BytesNoTransient()
	if err != nil {
		return nil, err
	}
	return w.wrap(raw)
}

func (w *WrappedTransaction) wrap(raw []byte) ([]byte, error) {
	return json.Marshal(&SerializedTransaction{Type: w.TransactionType, Raw: raw, Key: ChannelKeyOf(w.Transaction)})
}

// ChannelKeyOf returns the ChannelKey of the channel tx is bound to, or nil if tx
// exposes none or its key cannot be read. Serialization does not fail for a missing
// key: the bytes are also stored and hashed locally, and a receiver treats bytes
// without a key as coming from a node that does not send one.
func ChannelKeyOf(tx driver.Transaction) *driver.ChannelKey {
	keyed, ok := tx.(interface {
		ChannelKey() (*driver.ChannelKey, error)
	})
	if !ok {
		return nil
	}
	key, err := keyed.ChannelKey()
	if err != nil {
		logger.Warnf("tx [%s]: serializing without a channel key: %v", tx.ID(), err)
		return nil
	}
	return key
}

// SerializedTransaction is the encoding of a Fabric transaction sent to another node.
type SerializedTransaction struct {
	Type driver.TransactionType
	Raw  []byte
	// Key identifies the channel of the transaction to the receiving node. It is
	// nil in the bytes of a node that does not send it.
	Key *driver.ChannelKey `json:",omitempty"`
}
