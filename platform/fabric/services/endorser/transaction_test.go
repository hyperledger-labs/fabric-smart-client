/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

//go:generate counterfeiter -o mock/binding_store.go -fake-name FakeBindingStore github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.BindingStore
//go:generate counterfeiter -o mock/channel.go -fake-name FakeChannel github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Channel
//go:generate counterfeiter -o mock/channel_membership.go -fake-name FakeChannelMembership github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ChannelMembership
//go:generate counterfeiter -o mock/context.go -fake-name FakeContext github.com/hyperledger-labs/fabric-smart-client/platform/view/view.Context
//go:generate counterfeiter -o mock/envelope.go -fake-name FakeEnvelope github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Envelope
//go:generate counterfeiter -o mock/finality.go -fake-name FakeFinality github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Finality
//go:generate counterfeiter -o mock/fns.go -fake-name FakeFabricNetworkService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.FabricNetworkService
//go:generate counterfeiter -o mock/fns_provider.go -fake-name FakeFabricNetworkServiceProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.FabricNetworkServiceProvider
//go:generate counterfeiter -o mock/identity_provider.go -fake-name FakeIdentityProvider github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.IdentityProvider
//go:generate counterfeiter -o mock/local_membership.go -fake-name FakeLocalMembership github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.LocalMembership
//go:generate counterfeiter -o mock/ordering.go -fake-name FakeOrdering github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Ordering
//go:generate counterfeiter -o mock/proposal.go -fake-name FakeProposal github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Proposal
//go:generate counterfeiter -o mock/proposal_response.go -fake-name FakeProposalResponse github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.ProposalResponse
//go:generate counterfeiter -o mock/provider.go -fake-name FakeProvider github.com/hyperledger-labs/fabric-smart-client/platform/view/services.Provider
//go:generate counterfeiter -o mock/rwset.go -fake-name FakeRWSet github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.RWSet
//go:generate counterfeiter -o mock/session.go -fake-name FakeSession github.com/hyperledger-labs/fabric-smart-client/platform/view/view.Session
//go:generate counterfeiter -o mock/signed_proposal.go -fake-name FakeSignedProposal github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.SignedProposal
//go:generate counterfeiter -o mock/transaction.go -fake-name FakeTransaction github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Transaction
//go:generate counterfeiter -o mock/transaction_manager.go -fake-name FakeTransactionManager github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.TransactionManager
//go:generate counterfeiter -o mock/transaction_service.go -fake-name FakeEndorserTransactionService github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.EndorserTransactionService
//go:generate counterfeiter -o mock/vault.go -fake-name FakeVault github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver.Vault
//go:generate counterfeiter -o mock/verifier.go -fake-name FakeVerifier github.com/hyperledger-labs/fabric-smart-client/platform/common/driver.Verifier

func TestTransaction(t *testing.T) {
	t.Parallel()
	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeTx := &mock.FakeTransaction{}

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	// Test Simple Getters
	fakeTx.IDReturns("tx1")
	require.Equal(t, "tx1", et.ID())
	fakeTx.NetworkReturns("net1")
	require.Equal(t, "net1", et.Network())
	fakeTx.ChannelReturns("ch1")
	require.Equal(t, "ch1", et.Channel())

	// Test Parameters
	fakeTx.FunctionReturns("func")
	fakeTx.ParametersReturns([][]byte{[]byte("p1"), []byte("p2")})
	f, p := et.FunctionAndParameters()
	require.Equal(t, "func", f)
	require.Equal(t, []string{"p1", "p2"}, p)

	require.Equal(t, []byte("p1"), et.Parameters()[0])

	et.AppendParameter([]byte("p3"))
	require.Equal(t, 1, fakeTx.AppendParameterCallCount())

	fakeTx.SetParameterAtReturns(nil)
	err := et.SetParameterAt(0, []byte("p1-new"))
	require.NoError(t, err)
	require.Equal(t, 1, fakeTx.SetParameterAtCallCount())

	// Test Proposal
	fakeTx.ChaincodeReturns("cc1")
	fakeTx.ChaincodeVersionReturns("v1")
	cc, ver := et.Chaincode()
	require.Equal(t, "cc1", cc)
	require.Equal(t, "v1", ver)

	et.SetProposal("cc1", "v1", "func", "p1")
	require.Equal(t, 1, fakeTx.SetProposalCallCount())

	// Test RWSet
	fakeRWS := &mock.FakeRWSet{}
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	rws, err := et.RWSet()
	require.NoError(t, err)
	require.NotNil(t, rws)

	fakeRWS.BytesReturns([]byte("results"), nil)
	res, _ := et.Results()
	require.Equal(t, []byte("results"), res)

	// Hash uses Bytes()
	fakeTx.BytesReturns([]byte("bytes"), nil)
	h, _ := et.Hash()
	require.NotNil(t, h)

	// Test Bytes
	fakeTx.BytesReturns([]byte("bytes"), nil)
	b, _ := et.Bytes()
	require.Equal(t, []byte("bytes"), b)

	fakeTx.BytesNoTransientReturns([]byte("bytes-nt"), nil)
	bnt, _ := et.BytesNoTransient()
	require.Equal(t, []byte("bytes-nt"), bnt)

	fakeTx.RawReturns([]byte("raw"), nil)
	raw, _ := et.Raw()
	require.Equal(t, []byte("raw"), raw)

	// Test Creator
	fakeTx.CreatorReturns([]byte("creator"))
	require.Equal(t, []byte("creator"), []byte(et.Creator()))

	// Test Endorsements
	fakeTx.EndorseReturns(nil)
	require.NoError(t, et.Endorse())

	fakeTx.EndorseWithIdentityReturns(nil)
	require.NoError(t, et.EndorseWithIdentity([]byte("id1")))

	fakeTx.EndorseWithSignerReturns(nil)
	require.NoError(t, et.EndorseWithSigner([]byte("id1"), nil))

	fakeTx.EndorseProposalReturns(nil)
	require.NoError(t, et.EndorseProposal())

	fakeTx.EndorseProposalWithIdentityReturns(nil)
	require.NoError(t, et.EndorseProposalWithIdentity([]byte("id1")))

	fakeTx.EndorseProposalResponseReturns(nil)
	require.NoError(t, et.EndorseProposalResponse())

	fakeTx.EndorseProposalResponseWithIdentityReturns(nil)
	require.NoError(t, et.EndorseProposalResponseWithIdentity([]byte("id1")))

	// Test Proposal Responses
	fakeTx.ProposalResponseReturns([]byte("pr"), nil)
	pr, _ := et.ProposalResponse()
	require.Equal(t, []byte("pr"), pr)

	fakePR := &mock.FakeProposalResponse{}
	fakeTx.AppendProposalResponseReturns(nil)
	require.NoError(t, et.AppendProposalResponse(fabric.NewProposalResponse(fakePR)))

	fakeTx.ProposalResponsesReturns([]driver.ProposalResponse{fakePR}, nil)
	prs, _ := et.ProposalResponses()
	require.Equal(t, 1, len(prs))

	// ProposalResponses error
	fakeTx.ProposalResponsesReturns(nil, fmt.Errorf("err"))
	_, err = et.ProposalResponses()
	require.Error(t, err)
	fakeTx.ProposalResponsesReturns([]driver.ProposalResponse{fakePR}, nil)

	// Test HasBeenEndorsedBy
	fakePR.EndorserReturns([]byte("alice"))
	require.NoError(t, et.HasBeenEndorsedBy([]byte("alice")))
	require.Error(t, et.HasBeenEndorsedBy([]byte("bob")))

	// HasBeenEndorsedBy Error paths
	fakeTx.ProposalResponsesReturns(nil, fmt.Errorf("err"))
	require.Error(t, et.HasBeenEndorsedBy([]byte("alice")))

	// Test Signature & Namespaces
	fakeTx.ProposalResponsesReturns([]driver.ProposalResponse{fakePR}, nil)
	fakePR.EndorserReturns([]byte("alice"))
	fakePR.EndorserSignatureReturns([]byte("sig1"))

	sig, _ := et.GetSignatureOf([]byte("alice"))
	require.Equal(t, []byte("sig1"), sig)

	// Party not found
	sig, err = et.GetSignatureOf([]byte("bob"))
	require.NoError(t, err)
	require.Nil(t, sig)

	fakeRWS.NamespacesReturns([]string{"ns1", "ns2"})
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	ns := et.Namespaces()
	require.Equal(t, 2, ns.Count())

	// RWSet failure
	fakeTx.GetRWSetReturns(nil, fmt.Errorf("err"))
	require.Panics(t, func() { et.Namespaces() })

	// Test Results error
	fakeTx.GetRWSetReturns(nil, fmt.Errorf("err"))
	_, err = et.Results()
	require.Error(t, err)
	fakeTx.GetRWSetReturns(fakeRWS, nil)

	// Test Hash error
	fakeTx.BytesReturns(nil, fmt.Errorf("err"))
	_, err = et.Hash()
	require.Error(t, err)
	fakeTx.BytesReturns([]byte("bytes"), nil)

	// Test Transient
	fakeTx.TransientReturns(driver.TransientMap{"k": []byte("v")})
	require.NoError(t, et.SetTransient("k", []byte("v")))
	require.Equal(t, []byte("v"), et.GetTransient("k"))

	fakeTx.TransientReturns(driver.TransientMap{"ks": []byte("\"vs\"")})
	require.NoError(t, et.SetTransientState("ks", "vs"))
	var vs string
	require.NoError(t, et.GetTransientState("ks", &vs))
	require.Equal(t, "vs", vs)
	require.True(t, et.ExistsTransientState("ks"))

	// Test FabricNetworkService
	require.NotNil(t, et.FabricNetworkService())

	// Test AppendVerifierProvider
	et.AppendVerifierProvider(nil)

	// Test Envelope
	fakeTx.EnvelopeReturns(nil, nil)
	env, _ := et.Envelope()
	require.NotNil(t, env)

	// Test SignedProposal
	fakeTx.SignedProposalReturns(nil)
	sp := et.SignedProposal()
	require.NotNil(t, sp)

	fakeTx.SetFromBytesReturns(nil)
	require.NoError(t, et.SetFromBytes([]byte("raw")))

	fakeTx.SetFromEnvelopeBytesReturns(nil)
	require.NoError(t, et.SetFromEnvelopeBytes([]byte("raw")))

	et.Close()
	require.Equal(t, 1, fakeTx.CloseCallCount())
}

func TestBuilder(t *testing.T) {
	t.Parallel()
	fakeSP := &mock.FakeProvider{}
	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)

	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeLM := &mock.FakeLocalMembership{}
	fakeLM.DefaultIdentityReturns([]byte("alice"))
	fakeFNS.LocalMembershipReturns(fakeLM)
	fakeIP := &mock.FakeIdentityProvider{}
	fakeFNS.IdentityProviderReturns(fakeIP)

	builder := NewBuilder(fakeSP)
	require.NotNil(t, builder)

	// Test NewTransaction
	fakeTM := &mock.FakeTransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeTx := &mock.FakeTransaction{}
	fakeTM.NewTransactionReturns(fakeTx, nil)

	tx, err := builder.NewTransaction(context.Background(), fabric.WithChannel("ch1"))
	require.NoError(t, err)
	require.NotNil(t, tx)

	// Test NewTransactionFromBytes
	fakeTM.NewTransactionFromBytesReturns(fakeTx, nil)
	tx, err = builder.NewTransactionFromBytes([]byte("raw"))
	require.NoError(t, err)
	require.NotNil(t, tx)

	// Test NewTransactionFromEnvelopeBytes
	fakeTM.NewTransactionFromEnvelopeBytesReturns(fakeTx, nil)
	tx, err = builder.NewTransactionFromEnvelopeBytes(context.Background(), []byte("raw"))
	require.NoError(t, err)
	require.NotNil(t, tx)
}
