/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	ledgermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/ledger/mock"
	endorsermock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
)

type dummySubscriber struct{}

func (d *dummySubscriber) Subscribe(topic string, listener events.Listener)   {}
func (d *dummySubscriber) Unsubscribe(topic string, listener events.Listener) {}

func setupMockContext() (*mock.Context, *ledgermock.ChaincodeInvocation, *endorsermock.Envelope) {
	return setupMockContextWithSubscriber(&dummySubscriber{})
}

func setupMockContextWithSubscriber(sub events.Subscriber) (*mock.Context, *ledgermock.ChaincodeInvocation, *endorsermock.Envelope) {
	mockCtx := &mock.Context{}
	mockCtx.ContextReturns(context.Background())
	mockChaincodeInvocation := &ledgermock.ChaincodeInvocation{}
	mockChaincodeInvocation.WithSignerIdentityReturns(mockChaincodeInvocation)
	mockChaincodeInvocation.WithTransientEntryReturns(mockChaincodeInvocation, nil)
	mockChaincodeInvocation.WithEndorsersByMSPIDsReturns(mockChaincodeInvocation)
	mockChaincodeInvocation.WithEndorsersFromMyOrgReturns(mockChaincodeInvocation)
	mockChaincodeInvocation.WithNumRetriesReturns(mockChaincodeInvocation)
	mockChaincodeInvocation.WithRetrySleepReturns(mockChaincodeInvocation)
	mockChaincodeInvocation.WithMatchEndorsementPolicyReturns(mockChaincodeInvocation)
	mockChaincodeInvocation.WithTxIDReturns(mockChaincodeInvocation)

	mockEnv := &endorsermock.Envelope{}

	mockChaincodeInvocation.SubmitReturns("mock-txid", []byte("mock-result"), nil)
	mockChaincodeInvocation.QueryReturns([]byte("mock-result"), nil)
	mockChaincodeInvocation.EndorseReturns(mockEnv, nil)

	mockChaincode := &ledgermock.Chaincode{}
	mockChaincode.NewInvocationReturns(mockChaincodeInvocation)

	mockChaincodeManager := &ledgermock.ChaincodeManager{}
	mockChaincodeManager.ChaincodeReturns(mockChaincode)

	mockChannel := &endorsermock.Channel{}
	mockChannel.NameReturns("test-channel")
	mockChannel.ChaincodeManagerReturns(mockChaincodeManager)

	mockLocalMembership := &endorsermock.LocalMembership{}
	mockLocalMembership.DefaultIdentityReturns([]byte("id"))

	mockFns := &endorsermock.FabricNetworkService{}
	mockFns.NameReturns("test-network")
	mockFns.ChannelReturns(mockChannel, nil)
	mockFns.LocalMembershipReturns(mockLocalMembership)

	mockFnsProvider := &endorsermock.FabricNetworkServiceProvider{}
	mockFnsProvider.FabricNetworkServiceReturns(mockFns, nil)

	nsp := fabric.NewNetworkServiceProvider(mockFnsProvider, sub)
	mockCtx.GetServiceReturns(nsp, nil)

	return mockCtx, mockChaincodeInvocation, mockEnv
}
