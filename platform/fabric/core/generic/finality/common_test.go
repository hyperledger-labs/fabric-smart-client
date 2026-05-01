/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"reflect"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/mock"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type mockCommitter struct {
	mock.Mock
}

func (m *mockCommitter) Start(ctx context.Context) error { return nil }
func (m *mockCommitter) ProcessNamespace(nss ...cdriver.Namespace) error {
	return nil
}
func (m *mockCommitter) AddTransactionFilter(tf fdriver.TransactionFilter) error { return nil }
func (m *mockCommitter) Status(ctx context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
	return 0, "", nil
}

func (m *mockCommitter) AddFinalityListener(txID string, listener fdriver.FinalityListener) error {
	args := m.Called(txID, listener)
	return args.Error(0)
}

func (m *mockCommitter) RemoveFinalityListener(txID string, listener fdriver.FinalityListener) error {
	args := m.Called(txID, listener)
	return args.Error(0)
}

func (m *mockCommitter) DiscardTx(ctx context.Context, txID cdriver.TxID, message string) error {
	return nil
}

func (m *mockCommitter) CommitTX(ctx context.Context, txID cdriver.TxID, block cdriver.BlockNum, indexInBlock cdriver.TxNum, envelope *common.Envelope) error {
	return nil
}

type mockChannel struct {
	mock.Mock
	fdriver.Channel
}

func (m *mockChannel) Committer() fdriver.Committer {
	args := m.Called()
	return args.Get(0).(fdriver.Committer)
}

func (m *mockChannel) Delivery() fdriver.Delivery {
	args := m.Called()
	return args.Get(0).(fdriver.Delivery)
}

type mockFinalityListener struct {
	mock.Mock
}

func (m *mockFinalityListener) OnStatus(ctx context.Context, txID string, status fdriver.ValidationCode, message string) {
	m.Called(ctx, txID, status, message)
}

type mockDelivery struct {
	mock.Mock
}

func (m *mockDelivery) Start(ctx context.Context) error { return nil }

func (m *mockDelivery) ScanBlock(ctx context.Context, callback fdriver.BlockCallback) error {
	return nil
}

func (m *mockDelivery) ScanBlockFrom(ctx context.Context, block fdriver.BlockNum, callback fdriver.BlockCallback) error {
	return nil
}

func (m *mockDelivery) Scan(ctx context.Context, txID fdriver.TxID, callback fdriver.DeliveryCallback) error {
	args := m.Called(ctx, txID, callback)
	return args.Error(0)
}

func (m *mockDelivery) ScanFromBlock(ctx context.Context, block fdriver.BlockNum, callback fdriver.DeliveryCallback) error {
	args := m.Called(ctx, block, callback)
	return args.Error(0)
}

type mockProcessedTransaction struct {
	mock.Mock
}

func (m *mockProcessedTransaction) TxID() string          { return m.Called().String(0) }
func (m *mockProcessedTransaction) Results() []byte       { return m.Called().Get(0).([]byte) }
func (m *mockProcessedTransaction) ValidationCode() int32 { return m.Called().Get(0).(int32) }
func (m *mockProcessedTransaction) IsValid() bool         { return m.Called().Bool(0) }
func (m *mockProcessedTransaction) Envelope() []byte      { return m.Called().Get(0).([]byte) }

func newProcessedTransaction(pt fdriver.ProcessedTransaction) *fabric.ProcessedTransaction {
	fPT := &fabric.ProcessedTransaction{}
	v := reflect.ValueOf(fPT).Elem()
	f := v.FieldByName("pt")
	ptr := reflect.NewAt(f.Type(), f.Addr().UnsafePointer()).Elem()
	ptr.Set(reflect.ValueOf(pt))
	return fPT
}
