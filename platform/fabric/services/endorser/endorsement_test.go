/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestCollectEndorsementsView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.FakeTransaction{}
	fakeTx.ChannelReturns("ch1")
	fakeTx.NetworkReturns("net1")

	fakeRWS := &mock.FakeRWSet{}
	fakeRWS.BytesReturns([]byte("results"), nil)
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	fakeTx.BytesReturns([]byte("bytes"), nil)
	fakeTx.BytesNoTransientReturns([]byte("bytes-no-transient"), nil)
	fakeTx.ResultsReturns([]byte("results"), nil)

	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeCM := &mock.FakeChannelMembership{}
	fakeCH.ChannelMembershipReturns(fakeCM)

	fakeTM := &mock.FakeTransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	fakeBindingStore := &mock.FakeBindingStore{}
	endpointService, _ := endpoint.NewService(fakeBindingStore)

	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	endpointServiceType := reflect.TypeOf((*endpoint.Service)(nil))

	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		if v == endpointServiceType {
			return endpointService, nil
		}
		return nil, nil
	})

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	// Case 1: Bob is me
	fakeCtx.IsMeReturns(true)
	ev := NewCollectEndorsementsView(et, []byte("bob"))
	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 1, fakeTx.EndorseWithIdentityCallCount())

	// Case 2: Bob is not me, use deleteTransient=true
	fakeCtx.IsMeReturns(false)
	ev = NewCollectApprovesView(et, []byte("bob"))

	// Mock session and message
	fakeSession := &mock.FakeSession{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	msgCh := make(chan *view.Message, 1)
	fakeSession.ReceiveReturns(msgCh)

	resp := &mock.FakeProposalResponse{}
	resp.EndorserReturns([]byte("bob"))
	resp.ResultsReturns([]byte("results"))
	fakeTM.NewProposalResponseFromBytesReturns(resp, nil)

	// Mock verification
	fakeVerifier := &mock.FakeVerifier{}
	fakeCM.GetVerifierReturns(fakeVerifier, nil)

	payload, _ := json.Marshal([][]byte{[]byte("resp1")})
	msgCh <- &view.Message{Payload: payload}

	_, err = ev.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 1, fakeTx.BytesNoTransientCallCount())

	// Error cases
	// Session failure
	fakeCtx.GetSessionReturns(nil, fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)

	// Message receive error
	fakeCtx.GetSessionReturns(fakeSession, nil)
	msgCh <- &view.Message{Status: view.ERROR, Payload: []byte("err")}
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
}

func TestEndorseView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.FakeTransaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.IDReturns("tx1")
	fakeTx.BytesReturns([]byte("txraw"), nil)
	fakeTx.ProposalResponseReturns([]byte("pr"), nil)

	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeVault := &mock.FakeVault{}
	fakeCH.VaultReturns(fakeVault)

	fakeTS := &mock.FakeEndorserTransactionService{}
	fakeCH.TransactionServiceReturns(fakeTS)

	fakeLM := &mock.FakeLocalMembership{}
	fakeLM.DefaultIdentityReturns([]byte("alice"))
	fakeFNS.LocalMembershipReturns(fakeLM)

	fakeIP := &mock.FakeIdentityProvider{}
	fakeIP.IdentityReturns([]byte("alice"), nil)
	fakeFNS.IdentityProviderReturns(fakeIP)

	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	ev := NewEndorseView(et)

	fakeSession := &mock.FakeSession{}
	fakeCtx.SessionReturns(fakeSession)

	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 1, fakeTS.StoreTransactionCallCount())

	// Error path: tx.EndorseWithIdentity failure
	fakeTx.EndorseWithIdentityReturns(fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
	fakeTx.EndorseWithIdentityReturns(nil)

	// Error path: tx.ProposalResponse failure
	fakeTx.ProposalResponseReturns(nil, fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
}

func TestAcceptView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.FakeTransaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.IDReturns("tx1")
	fakeTx.BytesReturns([]byte("txraw"), nil)
	fakeTx.ProposalResponseReturns([]byte("pr"), nil)

	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeVault := &mock.FakeVault{}
	fakeCH.VaultReturns(fakeVault)
	fakeTS := &mock.FakeEndorserTransactionService{}
	fakeCH.TransactionServiceReturns(fakeTS)

	fakeIP := &mock.FakeIdentityProvider{}
	fakeIP.IdentityReturns([]byte("alice"), nil)
	fakeFNS.IdentityProviderReturns(fakeIP)

	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	fakeSession := &mock.FakeSession{}
	fakeCtx.SessionReturns(fakeSession)

	ev := NewAcceptView(et, []byte("alice"))
	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)
}

func TestFinalityView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeFinality := &mock.FakeFinality{}
	fakeCH.FinalityReturns(fakeFinality)

	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeTx := &mock.FakeTransaction{}
	fakeTx.IDReturns("tx1")
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	ev := NewFinalityView(et)
	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 1, fakeFinality.IsFinalCallCount())

	viewWithTimeout := NewFinalityWithTimeoutView(et, 1*time.Second)
	_, err = viewWithTimeout.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 2, fakeFinality.IsFinalCallCount())

	// Test Factory
	factory := &FinalityViewFactory{}
	input, _ := json.Marshal(&Finality{TxID: "tx1", Network: "net1", Channel: "ch1"})
	v, err := factory.NewView(input)
	require.NoError(t, err)
	require.NotNil(t, v)

	// Error path: Unmarshal failure
	_, err = factory.NewView([]byte("invalid"))
	require.Error(t, err)

	// Error path: GetFabricNetworkService failure
	fakeFNSP.FabricNetworkServiceReturns(nil, fmt.Errorf("err"))
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fabric.NewNetworkServiceProvider(fakeFNSP, nil), nil
		}
		return nil, nil
	})
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
	// Reset
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})
}

func TestOrderingView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeOrdering := &mock.FakeOrdering{}
	fakeFNS.OrderingServiceReturns(fakeOrdering)

	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeTx := &mock.FakeTransaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	ev := NewOrderingView(et)
	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 1, fakeOrdering.BroadcastCallCount())

	// With Finality
	fakeFinality := &mock.FakeFinality{}
	fakeCH.FinalityReturns(fakeFinality)
	fakeCtx.RunViewReturns(nil, nil)

	viewWithFinality := NewOrderingAndFinalityView(et)
	_, err = viewWithFinality.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 2, fakeOrdering.BroadcastCallCount())
	require.Equal(t, 1, fakeCtx.RunViewCallCount())

	viewWithFinalityAndTimeout := NewOrderingAndFinalityWithTimeoutView(et, 1*time.Second)
	_, err = viewWithFinalityAndTimeout.Call(fakeCtx)
	require.NoError(t, err)

	// Error path: Broadcast failure
	fakeOrdering.BroadcastReturns(fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)

	// Error path: GetFabricNetworkService failure
	fakeFNSP.FabricNetworkServiceReturns(nil, fmt.Errorf("err"))
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fabric.NewNetworkServiceProvider(fakeFNSP, nil), nil
		}
		return nil, nil
	})
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
}

func TestNamespaces(t *testing.T) {
	t.Parallel()
	ns := Namespaces{"ns1", "ns2", "ns3"}
	require.Equal(t, 3, ns.Count())
	require.True(t, ns.Match(Namespaces{"ns1", "ns2", "ns3"}))
	require.False(t, ns.Match(Namespaces{"ns1", "ns2"}))

	filtered := ns.Filter(func(s string) bool {
		return s != "ns2"
	})
	require.Equal(t, 2, filtered.Count())
	require.False(t, filtered.Match(ns))

	require.Equal(t, "ns1", ns.At(0))
}

func TestReceiveView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeSession := &mock.FakeSession{}
	fakeCtx.SessionReturns(fakeSession)

	msgCh := make(chan *view.Message, 1)
	fakeSession.ReceiveReturns(msgCh)
	msgCh <- &view.Message{Payload: []byte("payload")}

	rv := &receiveView{}
	res, err := rv.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), res)

	// Error case
	msgCh <- &view.Message{Status: view.ERROR, Payload: []byte("error")}
	_, err = rv.Call(fakeCtx)
	require.Error(t, err)
}

func TestReceiveTransactionView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	rv := &receiveTransactionView{}

	// Case 1: RunView fails
	fakeCtx.RunViewReturns(nil, fmt.Errorf("err"))
	_, err := rv.Call(fakeCtx)
	require.Error(t, err)

	// Case 2: NewTransactionFromBytes fails
	fakeCtx.RunViewReturns([]byte("invalid"), nil)
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})
	fakeFNSP := &mock.FakeFabricNetworkServiceProvider{}
	fakeFNS := &mock.FakeFabricNetworkService{}
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeTM := &mock.FakeTransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)
	fakeLM := &mock.FakeLocalMembership{}
	fakeLM.DefaultIdentityReturns([]byte("alice"))
	fakeFNS.LocalMembershipReturns(fakeLM)
	fakeIP := &mock.FakeIdentityProvider{}
	fakeFNS.IdentityProviderReturns(fakeIP)
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	networkServiceProviderType := reflect.TypeOf((*fabric.NetworkServiceProvider)(nil))
	fakeSP.GetServiceCalls(func(v interface{}) (interface{}, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})
	fakeTM.NewTransactionFromBytesReturns(nil, fmt.Errorf("err"))
	_, err = rv.Call(fakeCtx)
	require.Error(t, err)
}

func TestParallelCollectEndorsementsOnProposalViewInternal(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

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

	fakeTM := &mock.FakeTransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeTx := &mock.FakeTransaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.BytesReturns([]byte("raw"), nil)
	fakeTx.IDReturns("tx1")

	ft := &Transaction{
		Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx),
	}

	v := NewParallelCollectEndorsementsOnProposalView(ft, []byte("bob"))
	v.WithTimeout(1 * time.Second)

	fakeSession := &mock.FakeSession{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	fakeCtx.InitiatorReturns(&fakeView{})

	// Case 1: Success
	respPayload, _ := json.Marshal(&Response{
		ProposalResponses: [][]byte{[]byte("resp1")},
	})
	msgCh := make(chan *view.Message, 1)
	msgCh <- &view.Message{Payload: respPayload}
	fakeSession.ReceiveReturns(msgCh)

	fakeResp := &mock.FakeProposalResponse{}
	fakeTM.NewProposalResponseFromBytesReturns(fakeResp, nil)

	_, err := v.Call(fakeCtx)
	require.NoError(t, err)
	require.Equal(t, 1, fakeTx.AppendProposalResponseCallCount())

	// Case 2: Session error
	fakeCtx.GetSessionReturns(nil, fmt.Errorf("err"))
	_, err = v.Call(fakeCtx)
	require.Error(t, err)

	// Case 3: Send error
	fakeCtx.GetSessionReturns(fakeSession, nil)
	fakeSession.SendWithContextReturns(fmt.Errorf("err"))
	_, err = v.Call(fakeCtx)
	require.Error(t, err)

	// Case 4: Receive error
	fakeSession.SendWithContextReturns(nil)
	msgCh = make(chan *view.Message, 1)
	msgCh <- &view.Message{Status: view.ERROR, Payload: []byte("err")}
	fakeSession.ReceiveReturns(msgCh)
	_, err = v.Call(fakeCtx)
	require.Error(t, err)
}

func TestEndorsementOnProposalResponderViewInternal(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.FakeContext{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.FakeProvider{}
	fakeCtx.GetServiceCalls(func(v interface{}) (interface{}, error) {
		return fakeSP.GetService(v)
	})

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
	fakeTM := &mock.FakeTransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)
	fakeCH := &mock.FakeChannel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeTx := &mock.FakeTransaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTM.NewTransactionReturns(fakeTx, nil)

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	ev := NewEndorsementOnProposalResponderView(et)
	fakeSession := &mock.FakeSession{}
	fakeCtx.SessionReturns(fakeSession)

	// Case 1: Success
	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)

	// Case 2: Session send failure
	fakeSession.SendWithContextReturns(fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)

	// Case 3: EndorseProposalResponseWithIdentity failure
	fakeSession.SendWithContextReturns(nil)
	fakeTx.EndorseProposalResponseWithIdentityReturns(fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
}

func TestVerifierProviderWrapper(t *testing.T) {
	t.Parallel()
	v := &verifierProviderWrapper{m: &fabric.MSPManager{}}
	require.Panics(t, func() { _, _ = v.GetVerifier([]byte("alice")) })
	require.NotNil(t, v)
}

type fakeView struct{}

func (v *fakeView) Call(context view.Context) (interface{}, error) { return nil, nil }
