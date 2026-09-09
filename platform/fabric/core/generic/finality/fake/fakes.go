/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fake

import (
	"context"
	"crypto/tls"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	viewgrpc "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type Committer struct {
	mock.Mock
}

func (*Committer) Start(_ context.Context) error { return nil }
func (*Committer) ProcessNamespace(_ ...cdriver.Namespace) error {
	return nil
}
func (*Committer) AddTransactionFilter(_ fdriver.TransactionFilter) error { return nil }
func (*Committer) Status(_ context.Context, _ cdriver.TxID) (fdriver.ValidationCode, string, error) {
	return 0, "", nil
}

func (m *Committer) AddFinalityListener(txID string, listener fdriver.FinalityListener) error {
	args := m.Called(txID, listener)
	return args.Error(0)
}

func (m *Committer) RemoveFinalityListener(txID string, listener fdriver.FinalityListener) error {
	args := m.Called(txID, listener)
	return args.Error(0)
}

func (*Committer) DiscardTx(_ context.Context, _ cdriver.TxID, _ string) error {
	return nil
}

func (*Committer) CommitTX(_ context.Context, _ cdriver.TxID, _ cdriver.BlockNum, _ cdriver.TxNum, _ *common.Envelope) error {
	return nil
}

type Channel struct {
	mock.Mock
	fdriver.Channel
}

func (m *Channel) Committer() fdriver.Committer {
	args := m.Called()
	return args.Get(0).(fdriver.Committer)
}

func (m *Channel) Delivery() fdriver.Delivery {
	args := m.Called()
	return args.Get(0).(fdriver.Delivery)
}

func (m *Channel) Ledger() fdriver.Ledger {
	args := m.Called()
	return args.Get(0).(fdriver.Ledger)
}

func (*Channel) Name() string {
	return "test"
}

type FinalityListener struct {
	mock.Mock
}

func (m *FinalityListener) OnStatus(ctx context.Context, txID string, status fdriver.ValidationCode, message string) {
	m.Called(ctx, txID, status, message)
}

type Delivery struct {
	mock.Mock
}

func (*Delivery) Start(_ context.Context) error { return nil }

func (*Delivery) ScanBlock(_ context.Context, _ fdriver.BlockCallback) error {
	return nil
}

func (*Delivery) ScanBlockFrom(_ context.Context, _ fdriver.BlockNum, _ fdriver.BlockCallback) error {
	return nil
}

func (m *Delivery) Scan(ctx context.Context, txID fdriver.TxID, callback fdriver.DeliveryCallback) error {
	args := m.Called(ctx, txID, callback)
	return args.Error(0)
}

func (m *Delivery) ScanFromBlock(ctx context.Context, block fdriver.BlockNum, callback fdriver.DeliveryCallback) error {
	args := m.Called(ctx, block, callback)
	return args.Error(0)
}

type ProcessedTransaction struct {
	mock.Mock
}

func (m *ProcessedTransaction) TxID() string          { return m.Called().String(0) }
func (m *ProcessedTransaction) Results() []byte       { return m.Called().Get(0).([]byte) }
func (m *ProcessedTransaction) ValidationCode() int32 { return m.Called().Get(0).(int32) }
func (m *ProcessedTransaction) IsValid() bool         { return m.Called().Bool(0) }
func (m *ProcessedTransaction) Envelope() []byte      { return m.Called().Get(0).([]byte) }

type ConfigService struct {
	mock.Mock
	fdriver.ConfigService
}

func (m *ConfigService) PickPeer(role fdriver.PeerFunctionType) *viewgrpc.ConnectionConfig {
	args := m.Called(role)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*viewgrpc.ConnectionConfig)
}

type PeerClient struct {
	mock.Mock
}

func (m *PeerClient) DeliverClient() (pb.DeliverClient, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pb.DeliverClient), args.Error(1)
}

func (*PeerClient) EndorserClient() (pb.EndorserClient, error) {
	return nil, nil
}

func (*PeerClient) DiscoveryClient() (services.DiscoveryClient, error) {
	return nil, nil
}

func (m *PeerClient) Certificate() tls.Certificate { return m.Called().Get(0).(tls.Certificate) }
func (m *PeerClient) Address() string              { return m.Called().String(0) }
func (m *PeerClient) Close()                       { m.Called() }

type DeliverClient struct {
	mock.Mock
	pb.DeliverClient
}

func (m *DeliverClient) Deliver(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverClient, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pb.Deliver_DeliverClient), args.Error(1)
}

func (m *DeliverClient) DeliverFiltered(ctx context.Context, opts ...grpc.CallOption) (pb.Deliver_DeliverFilteredClient, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pb.Deliver_DeliverFilteredClient), args.Error(1)
}

type DeliverFilteredStream struct {
	mock.Mock
	pb.Deliver_DeliverFilteredClient
}

func (m *DeliverFilteredStream) Send(e *common.Envelope) error { return m.Called(e).Error(0) }
func (m *DeliverFilteredStream) Recv() (*pb.DeliverResponse, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.DeliverResponse), args.Error(1)
}
func (m *DeliverFilteredStream) CloseSend() error           { return m.Called().Error(0) }
func (*DeliverFilteredStream) Header() (metadata.MD, error) { return nil, nil }
func (*DeliverFilteredStream) Trailer() metadata.MD         { return nil }
func (*DeliverFilteredStream) CloseRead() error             { return nil }
func (*DeliverFilteredStream) Context() context.Context     { return context.Background() }
func (*DeliverFilteredStream) SendMsg(_ any) error          { return nil }
func (*DeliverFilteredStream) RecvMsg(_ any) error          { return nil }

type SigningIdentity struct {
	mock.Mock
	fdriver.SigningIdentity
}

func (m *SigningIdentity) Serialize() ([]byte, error) {
	args := m.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (m *SigningIdentity) Sign(msg []byte) ([]byte, error) {
	args := m.Called(msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

type Services struct {
	mock.Mock
}

func (m *Services) NewPeerClient(cc viewgrpc.ConnectionConfig) (services.PeerClient, error) {
	args := m.Called(cc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(services.PeerClient), args.Error(1)
}
