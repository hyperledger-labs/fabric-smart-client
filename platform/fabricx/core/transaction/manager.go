/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package transaction

import (
	"context"
	"fmt"
	"sync"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/hyperledger/fabric-x-committer/api/protoblocktx"
	"github.com/hyperledger/fabric-x-common/protoutil"
)

type Manager struct {
	transactionFactories sync.Map
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) ComputeTxID(id *driver.TxIDComponents) string {
	return transaction.ComputeTxID(id)
}

func (m *Manager) NewEnvelope() driver.Envelope {
	return NewEmptyEnvelope()
}

func (m *Manager) NewProposalResponseFromBytes(raw []byte) (driver.ProposalResponse, error) {
	return NewProposalResponseFromBytes(raw)
}

func (m *Manager) transactionFactory(transactionType driver.TransactionType) (driver.TransactionFactory, error) {
	logger.Debugf("transactionFactory called with transactionType [%v]", transactionType)
	f, ok := m.transactionFactories.Load(transactionType)
	if !ok {
		return nil, fmt.Errorf("no transaction factory found for transaction type %v", transactionType)
	}

	txFactory, ok := f.(driver.TransactionFactory)
	if !ok {
		panic("retrieved factory is not driver.TransactionFactory")
	}

	return txFactory, nil
}

func (m *Manager) NewTransaction(ctx context.Context, transactionType driver.TransactionType, creator view.Identity, nonce []byte, txID driver2.TxID, channel string, rawRequest []byte) (driver.Transaction, error) {
	txFactory, err := m.transactionFactory(transactionType)
	if err != nil {
		return nil, err
	}

	return txFactory.NewTransaction(ctx, channel, nonce, creator, txID, rawRequest)
}

func (m *Manager) NewTransactionFromBytes(ctx context.Context, channel string, raw []byte) (driver.Transaction, error) {
	// TODO: remove fixed transaction type
	txFactory, err := m.transactionFactory(driver.EndorserTransaction)
	if err != nil {
		return nil, err
	}

	tx, err := txFactory.NewTransaction(ctx, channel, nil, nil, "", nil)
	if err != nil {
		return nil, err
	}

	if err := tx.SetFromBytes(raw); err != nil {
		return nil, err
	}

	return tx, nil
}

func (m *Manager) NewTransactionFromEnvelopeBytes(context.Context, string, []byte) (driver.Transaction, error) {
	// TODO: implement me
	panic("NewTransactionFromEnvelopeBytes >> implement me")
}

func (m *Manager) AddTransactionFactory(transactionType driver.TransactionType, factory driver.TransactionFactory) {
	m.transactionFactories.Store(transactionType, factory)
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

type processedTransaction struct {
	vc  int32
	ue  *UnpackedEnvelope
	env []byte
}

func NewProcessedTransactionFromEnvelopePayload(payload []byte) (*processedTransaction, int32, error) {
	ue, headerType, err := UnpackEnvelopePayload(payload)
	if err != nil {
		return nil, headerType, err
	}
	return &processedTransaction{ue: ue}, headerType, nil
}

func NewProcessedTransactionFromEnvelopeRaw(env []byte) (*processedTransaction, error) {
	ue, _, err := UnpackEnvelopeFromBytes(env)
	if err != nil {
		return nil, err
	}
	return &processedTransaction{ue: ue, env: env}, nil
}

func NewProcessedTransaction(raw []byte) (*processedTransaction, error) {
	pt := &pb.ProcessedTransaction{}
	if err := proto.Unmarshal(raw, pt); err != nil {
		return nil, errors.Wrap(err, "unmarshal failed")
	}
	ue, _, err := UnpackEnvelope(pt.TransactionEnvelope)
	if err != nil {
		return nil, err
	}
	env, err := protoutil.Marshal(pt.TransactionEnvelope)
	if err != nil {
		return nil, err
	}
	return &processedTransaction{vc: pt.ValidationCode, ue: ue, env: env}, nil
}

func (p *processedTransaction) TxID() string {
	return p.ue.TxID
}

func (p *processedTransaction) Results() []byte {
	return p.ue.Results
}

func (p *processedTransaction) IsValid() bool {
	return p.vc == int32(protoblocktx.Status_COMMITTED)
}

func (p *processedTransaction) Envelope() []byte {
	return p.env
}

func (p *processedTransaction) ValidationCode() int32 {
	return p.vc
}
