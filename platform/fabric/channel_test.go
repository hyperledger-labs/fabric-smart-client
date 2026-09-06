/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

type mockChannelSubscriber struct {
	events.Subscriber
}

type mockChannelLedger struct {
	driver.Ledger
}

// mockChaincodeManager is a hand-written driver.ChaincodeManager: the generated
// endorser/mock package has no ChaincodeManager fake. It records the last name it
// was asked for and returns a fixed driver.Chaincode so the test can assert both.
type mockChaincodeManager struct {
	cc       driver.Chaincode
	lastName string
}

func (m *mockChaincodeManager) Chaincode(name string) driver.Chaincode {
	m.lastName = name
	return m.cc
}

func TestChannel(t *testing.T) {
	t.Parallel()

	subscriber := &mockChannelSubscriber{}
	fns := &mock.FabricNetworkService{}
	ch := &mock.Channel{}

	ch.NameReturns("mychannel")

	channel := NewChannel(subscriber, fns, ch)

	require.Equal(t, "mychannel", channel.Name())
	require.Equal(t, 1, ch.NameCallCount())

	require.NotNil(t, channel.Vault())

	// Test Ledger
	mockLedger := &mockChannelLedger{}
	ch.LedgerReturns(mockLedger)
	require.NotNil(t, channel.Ledger())
	require.Equal(t, 1, ch.LedgerCallCount())

	// Test MSPManager
	mockMembership := &mock.ChannelMembership{}
	ch.ChannelMembershipReturns(mockMembership)
	require.NotNil(t, channel.MSPManager())
	require.Equal(t, 1, ch.ChannelMembershipCallCount())

	// Test ConfigSequence
	mockMembership.ConfigSequenceReturns(uint64(42), nil)
	seq, err := channel.ConfigSequence()
	require.NoError(t, err)
	require.Equal(t, uint64(42), seq)
	require.Equal(t, 1, mockMembership.ConfigSequenceCallCount())

	// Test ACLProvider
	require.NotNil(t, channel.ACLProvider())

	// Test Committer
	require.NotNil(t, channel.Committer())

	// Test Finality
	mockFinality := &mock.Finality{}
	ch.FinalityReturns(mockFinality)
	require.NotNil(t, channel.Finality())
	require.Equal(t, 1, ch.FinalityCallCount())

	// Test Delivery. Reuse the package-level mockDelivery (delivery_test.go)
	// instead of a local stub; endorser/mock has no Delivery fake.
	ch.DeliveryReturns(&mockDelivery{})
	require.NotNil(t, channel.Delivery())
	require.Equal(t, 1, ch.DeliveryCallCount())

	// Test MetadataService. Reuse the package-level mockMetaSvc (vault_test.go).
	ch.MetadataServiceReturns(&mockMetaSvc{})
	require.NotNil(t, channel.MetadataService())
	require.Equal(t, 2, ch.MetadataServiceCallCount())

	// Test EnvelopeService. Reuse the package-level mockEnvSvc (vault_test.go).
	ch.EnvelopeServiceReturns(&mockEnvSvc{})
	require.NotNil(t, channel.EnvelopeService())
	require.Equal(t, 2, ch.EnvelopeServiceCallCount())

	// Test Chaincode. The generated mock has no ChaincodeManager fake, so use the
	// hand-written mockChaincodeManager above and reuse the package-level mockChaincode.
	mcc := &mockChaincode{}
	ccMgr := &mockChaincodeManager{cc: mcc}
	ch.ChaincodeManagerReturns(ccMgr)
	chaincode := channel.Chaincode("mycc")
	require.NotNil(t, chaincode)
	require.Same(t, mcc, chaincode.chaincode)
	require.Equal(t, "mycc", ccMgr.lastName)
	require.Equal(t, 1, ch.ChaincodeManagerCallCount())
}
