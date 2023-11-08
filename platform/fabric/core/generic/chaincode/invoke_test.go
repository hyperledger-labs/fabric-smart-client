/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/chaincode/mocks"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	config2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestInvokeQueryFailMatchPolicyDiscoveredPeers(t *testing.T) {
	network := &mocks.Network{}
	cp := &mock2.ConfigProvider{}
	cp.IsSetReturns(false)
	networkConfig, err := config2.New(cp, "default", true)
	assert.NoError(t, err)
	network.ConfigReturns(networkConfig)
	sc := &mocks.SignerService{}
	si := &mocks.SigningIdentity{}
	si.SerializeReturns([]byte("alice"), nil)
	si.SignReturns([]byte("alice's signature"), nil)
	sc.GetSigningIdentityReturns(si, nil)
	network.SignerServiceReturns(sc)
	channel := &mocks.Channel{}
	mspManager := &mocks.MSPManager{}
	mspIdentity := &mocks.MSPIdentity{}
	mspIdentity.GetMSPIdentifierReturns("mspid")
	mspManager.DeserializeIdentityReturns(mspIdentity, nil)
	channel.MSPManagerReturns(mspManager)
	chConfig := &config.Channel{
		Name:       "blueberry",
		Default:    true,
		Quiet:      false,
		NumRetries: 1,
		RetrySleep: 1 * time.Second,
		Chaincodes: nil,
	}
	channel.ConfigReturns(chConfig)
	pc := &mocks.PeerClient{}
	//ec := &mocks.EndorserClient{}
	pc.EndorserReturns(nil, errors.Errorf("endorser not found"))
	channel.NewPeerClientForAddressReturnsOnCall(0, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(1, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(2, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(3, nil, errors.Errorf("peer not found"))
	channel.NewPeerClientForAddressReturnsOnCall(4, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(5, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(6, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(7, nil, errors.Errorf("peer not found"))
	channel.NewPeerClientForAddressReturnsOnCall(8, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(9, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(10, pc, nil)
	channel.NewPeerClientForAddressReturnsOnCall(11, nil, errors.Errorf("peer not found"))
	discover := &mocks.Discover{}
	discover.WithFilterByMSPIDsReturns(discover)
	discover.WithImplicitCollectionsReturns(discover)
	discoveredPeers := []driver.DiscoveredPeer{
		{
			Identity:     []byte("peer1"),
			MSPID:        "mspid",
			Endpoint:     "1.1.1.1",
			TLSRootCerts: nil,
		},
		{
			Identity:     []byte("peer2"),
			MSPID:        "mspid",
			Endpoint:     "2.1.1.1",
			TLSRootCerts: nil,
		},
		{
			Identity:     []byte("peer3"),
			MSPID:        "mspid",
			Endpoint:     "3.1.1.1",
			TLSRootCerts: nil,
		},
		{
			Identity:     []byte("peer4"),
			MSPID:        "mspid",
			Endpoint:     "4.1.1.1",
			TLSRootCerts: nil,
		},
	}
	discover.CallReturns(discoveredPeers, nil)
	ch := chaincode.NewChaincode("pineapple", nil, network, channel)

	invoke := chaincode.NewInvoke(ch, func(chaincode *chaincode.Chaincode) driver.ChaincodeDiscover {
		return discover
	}, "apple")
	invoke.WithEndorsersFromMyOrg()
	invoke.WithQueryPolicy(driver.QueryAll)
	invoke.WithSignerIdentity([]byte("alice"))

	_, err = invoke.Query()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot match query policy with the given discovered peers")
	assert.Contains(t, err.Error(), "query all policy, no errors expected [")

	invoke = chaincode.NewInvoke(ch, func(chaincode *chaincode.Chaincode) driver.ChaincodeDiscover {
		return discover
	}, "apple")
	invoke.WithEndorsersFromMyOrg()
	invoke.WithQueryPolicy(driver.QueryMajority)
	invoke.WithSignerIdentity([]byte("alice"))
	_, err = invoke.Query()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot match query policy with the given peer clients")
	assert.Contains(t, err.Error(), "query majority policy, no majority reached")

	ec := &mocks.EndorserClient{}
	pr := &peer.ProposalResponse{
		Version:   0,
		Timestamp: nil,
		Response: &peer.Response{
			Status:  0,
			Message: "",
			Payload: nil,
		},
		Payload: nil,
		Endorsement: &peer.Endorsement{
			Endorser:  []byte("endorser"),
			Signature: []byte("endorser's signature"),
		},
		Interest: nil,
	}
	ec.ProcessProposalReturnsOnCall(0, pr, nil)
	ec.ProcessProposalReturnsOnCall(1, nil, errors.New("cannot return proposal response"))
	ec.ProcessProposalReturnsOnCall(2, nil, errors.New("cannot return proposal response"))
	pc.EndorserReturns(ec, nil)
	invoke = chaincode.NewInvoke(ch, func(chaincode *chaincode.Chaincode) driver.ChaincodeDiscover {
		return discover
	}, "apple")
	invoke.WithEndorsersFromMyOrg()
	invoke.WithQueryPolicy(driver.QueryOne)
	invoke.WithSignerIdentity([]byte("alice"))
	_, err = invoke.Query()
	assert.NoError(t, err)
}
