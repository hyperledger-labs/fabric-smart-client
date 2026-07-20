/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"encoding/json"
	"errors"
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
	fakeCtx := &mock.Context{}
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.Transaction{}
	fakeTx.ChannelReturns("ch1")
	fakeTx.NetworkReturns("net1")

	fakeRWS := &mock.RWSet{}
	fakeRWS.BytesReturns([]byte("results"), nil)
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	fakeTx.BytesReturns([]byte("bytes"), nil)
	fakeTx.BytesNoTransientReturns([]byte("bytes-no-transient"), nil)
	fakeTx.ResultsReturns([]byte("results"), nil)

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeCM := &mock.ChannelMembership{}
	fakeCH.ChannelMembershipReturns(fakeCM)

	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	fakeBindingStore := &mock.BindingStore{}
	endpointService, _ := endpoint.NewService(fakeBindingStore)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	endpointServiceType := reflect.TypeFor[*endpoint.Service]()

	fakeSP.GetServiceCalls(func(v any) (any, error) {
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
	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	msgCh := make(chan *view.Message, 1)
	fakeSession.ReceiveReturns(msgCh)

	resp := &mock.ProposalResponse{}
	resp.EndorserReturns([]byte("bob"))
	resp.ResultsReturns([]byte("results"))
	fakeTM.NewProposalResponseFromBytesReturns(resp, nil)

	// Mock verification
	fakeVerifier := &mock.Verifier{}
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
	fakeCtx := &mock.Context{}
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.IDReturns("tx1")
	fakeTx.BytesReturns([]byte("txraw"), nil)
	fakeTx.ProposalResponseReturns([]byte("pr"), nil)

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeVault := &mock.Vault{}
	fakeCH.VaultReturns(fakeVault)

	fakeTS := &mock.EndorserTransactionService{}
	fakeCH.TransactionServiceReturns(fakeTS)

	fakeLM := &mock.LocalMembership{}
	fakeLM.DefaultIdentityReturns([]byte("alice"))
	fakeFNS.LocalMembershipReturns(fakeLM)

	fakeIP := &mock.IdentityProvider{}
	fakeIP.IdentityReturns([]byte("alice"), nil)
	fakeFNS.IdentityProviderReturns(fakeIP)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
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

	fakeSession := &mock.Session{}
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
	fakeCtx := &mock.Context{}
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.IDReturns("tx1")
	fakeTx.BytesReturns([]byte("txraw"), nil)
	fakeTx.ProposalResponseReturns([]byte("pr"), nil)

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeVault := &mock.Vault{}
	fakeCH.VaultReturns(fakeVault)
	fakeTS := &mock.EndorserTransactionService{}
	fakeCH.TransactionServiceReturns(fakeTS)

	fakeIP := &mock.IdentityProvider{}
	fakeIP.IdentityReturns([]byte("alice"), nil)
	fakeFNS.IdentityProviderReturns(fakeIP)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
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

	fakeSession := &mock.Session{}
	fakeCtx.SessionReturns(fakeSession)

	ev := NewAcceptView(et, []byte("alice"))
	_, err := ev.Call(fakeCtx)
	require.NoError(t, err)
}

func TestFinalityView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeFinality := &mock.Finality{}
	fakeCH.FinalityReturns(fakeFinality)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeTx := &mock.Transaction{}
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
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fabric.NewNetworkServiceProvider(fakeFNSP, nil), nil
		}
		return nil, nil
	})
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)
	// Reset
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})
}

func TestOrderingView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeOrdering := &mock.Ordering{}
	fakeFNS.OrderingServiceReturns(fakeOrdering)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeTx := &mock.Transaction{}
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
	fakeFinality := &mock.Finality{}
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
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
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
	fakeCtx := &mock.Context{}
	fakeSession := &mock.Session{}
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

// TestReceiveView_TimesOutWhenSilent demonstrates the fix for the DoS in flow.go's
// receiveView.Call: it now bounds the wait on <-ch with a 10-second timeout instead
// of blocking forever. A remote peer that opens a session and never sends anything
// (and never sends an error) no longer parks the responder's view-execution
// goroutine indefinitely.
func TestReceiveView_TimesOutWhenSilent(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeSession := &mock.Session{}
	fakeCtx.SessionReturns(fakeSession)

	// A malicious/silent remote peer: the channel never receives a message.
	msgCh := make(chan *view.Message)
	fakeSession.ReceiveReturns(msgCh)

	rv := &receiveView{}
	done := make(chan error, 1)
	go func() {
		_, err := rv.Call(fakeCtx)
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err, "receiveView.Call must return a timeout error when the remote peer stays silent")
	case <-time.After(15 * time.Second):
		t.Fatal("receiveView.Call did not return within the expected 10-second timeout window")
	}
}

func TestReceiveTransactionView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	rv := &receiveTransactionView{}

	// Case 1: RunView fails
	fakeCtx.RunViewReturns(nil, fmt.Errorf("err"))
	_, err := rv.Call(fakeCtx)
	require.Error(t, err)

	// Case 2: NewTransactionFromBytes fails
	fakeCtx.RunViewReturns([]byte("invalid"), nil)
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})
	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNS := &mock.FabricNetworkService{}
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)
	fakeLM := &mock.LocalMembership{}
	fakeLM.DefaultIdentityReturns([]byte("alice"))
	fakeFNS.LocalMembershipReturns(fakeLM)
	fakeIP := &mock.IdentityProvider{}
	fakeFNS.IdentityProviderReturns(fakeIP)
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
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
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeCM := &mock.ChannelMembership{}
	fakeCH.ChannelMembershipReturns(fakeCM)

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.BytesReturns([]byte("raw"), nil)
	fakeTx.IDReturns("tx1")

	ft := &Transaction{
		Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx),
	}

	v := NewParallelCollectEndorsementsOnProposalView(ft, []byte("bob"))
	v.WithTimeout(1 * time.Second)

	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	fakeCtx.InitiatorReturns(&fakeView{})

	// Case 1: Success
	respPayload, _ := json.Marshal(&Response{
		ProposalResponses: [][]byte{[]byte("resp1")},
	})
	msgCh := make(chan *view.Message, 1)
	msgCh <- &view.Message{Payload: respPayload}
	fakeSession.ReceiveReturns(msgCh)

	fakeResp := &mock.ProposalResponse{}
	fakeResp.EndorserReturns([]byte("bob"))
	fakeResp.VerifyEndorsementReturns(nil)
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

// TestParallelCollectEndorsementsOnProposalView_RejectsUnverifiedResponse demonstrates
// that parallelCollectEndorsementsOnProposalView.Call (endorsement_proposal.go) now
// calls ProposalResponse.VerifyEndorsement, and checks that the endorser is bound to
// the contacted party, before appending a remote-supplied proposal response to the
// transaction - mirroring the sequential collectEndorsementsView.Call, which verifies
// signatures against a set of VerifierProviders.
func TestParallelCollectEndorsementsOnProposalView_RejectsUnverifiedResponse(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	endpointServiceType := reflect.TypeFor[*endpoint.Service]()

	fakeBindingStore := &mock.BindingStore{}
	fakeBindingStore.HaveSameBindingReturns(false, nil)
	endpointService, _ := endpoint.NewService(fakeBindingStore)

	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		if v == endpointServiceType {
			return endpointService, nil
		}
		return nil, nil
	})

	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeCM := &mock.ChannelMembership{}
	fakeCH.ChannelMembershipReturns(fakeCM)

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.BytesReturns([]byte("raw"), nil)
	fakeTx.IDReturns("tx1")

	ft := &Transaction{
		Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx),
	}

	// Party contacted is "bob", but the returned response claims to be endorsed by
	// "mallory" - an identity with no relationship to "bob" whatsoever.
	v := NewParallelCollectEndorsementsOnProposalView(ft, []byte("bob"))
	v.WithTimeout(1 * time.Second)

	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	fakeCtx.InitiatorReturns(&fakeView{})

	respPayload, _ := json.Marshal(&Response{
		ProposalResponses: [][]byte{[]byte("resp-from-mallory")},
	})
	msgCh := make(chan *view.Message, 1)
	msgCh <- &view.Message{Payload: respPayload}
	fakeSession.ReceiveReturns(msgCh)

	// This response is neither signed by "bob" nor bound to "bob", so it must be
	// rejected regardless of what VerifyEndorsement would say.
	fakeResp := &mock.ProposalResponse{}
	fakeResp.EndorserReturns([]byte("mallory"))
	fakeResp.VerifyEndorsementReturns(errors.New("signature verification failed"))
	fakeTM.NewProposalResponseFromBytesReturns(fakeResp, nil)

	_, err := v.Call(fakeCtx)

	require.Error(t, err, "unverified proposal response from an unrelated identity must be rejected")
	require.Equal(t, 0, fakeTx.AppendProposalResponseCallCount())
}

// TestParallelCollectEndorsementsOnProposalView_TimesOutOnSilentParty demonstrates that
// parallelCollectEndorsementsOnProposalView.Call (endorsement_proposal.go) no longer blocks
// forever on <-answerChannel when a contacted party never answers. Each per-party goroutine's
// own ReceiveWithTimeout only bounds the final receive step of collectEndorsement - it does
// not bound session setup/send, and (per WithTimeout's doc) is zero unless the caller
// explicitly opts in, which the only production call site (state.NewParallelCollectEndorsementsOnProposalView)
// never does. Call now applies an aggregate deadline around the wait itself, so the view
// returns a timeout error instead of hanging indefinitely.
func TestParallelCollectEndorsementsOnProposalView_TimesOutOnSilentParty(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeCM := &mock.ChannelMembership{}
	fakeCH.ChannelMembershipReturns(fakeCM)

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.BytesReturns([]byte("raw"), nil)
	fakeTx.IDReturns("tx1")

	ft := &Transaction{
		Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx),
	}

	// A short timeout so the test doesn't wait out defaultParallelEndorsementTimeout; the
	// party's session never delivers a response (empty, never-fed channel), simulating a
	// silent/unresponsive remote peer.
	v := NewParallelCollectEndorsementsOnProposalView(ft, []byte("bob"))
	v.WithTimeout(50 * time.Millisecond)

	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	fakeCtx.InitiatorReturns(&fakeView{})
	fakeSession.ReceiveReturns(make(chan *view.Message))

	done := make(chan struct{})
	var err error
	go func() {
		_, err = v.Call(fakeCtx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Call did not return - it appears to be blocking forever on the silent party")
	}

	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout")
}

func TestEndorsementOnProposalResponderViewInternal(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeLM := &mock.LocalMembership{}
	fakeLM.DefaultIdentityReturns([]byte("alice"))
	fakeFNS.LocalMembershipReturns(fakeLM)
	fakeIP := &mock.IdentityProvider{}
	fakeFNS.IdentityProviderReturns(fakeIP)
	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTM.NewTransactionReturns(fakeTx, nil)

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	ft := fabric.NewTransaction(fns, fakeTx)
	et := &Transaction{
		Transaction: ft,
	}

	ev := NewEndorsementOnProposalResponderView(et)
	fakeSession := &mock.Session{}
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

// TestCollectEndorsementsView_RejectsEndorsementFromUnboundParty demonstrates that
// collectEndorsementsView.Call rejects a cryptographically-valid endorsement from an
// identity that is NOT bound to the expected party. `found` is now initialized to
// `false` before the IsBoundTo check (endorsement.go), so the final
// `if !found { return error }` guard correctly rejects a response whose signer is
// neither the contacted party nor bound to it.
func TestCollectEndorsementsView_RejectsEndorsementFromUnboundParty(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.Transaction{}
	fakeTx.ChannelReturns("ch1")
	fakeTx.NetworkReturns("net1")

	fakeRWS := &mock.RWSet{}
	fakeRWS.BytesReturns([]byte("results"), nil)
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	fakeTx.BytesReturns([]byte("bytes"), nil)
	fakeTx.ResultsReturns([]byte("results"), nil)

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)

	fakeCM := &mock.ChannelMembership{}
	fakeCH.ChannelMembershipReturns(fakeCM)

	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	// A real endpoint.Service backed by a BindingStore that reports the endorser
	// identity is NOT bound to the party we asked to endorse.
	fakeBindingStore := &mock.BindingStore{}
	fakeBindingStore.HaveSameBindingReturns(false, nil)
	endpointService, _ := endpoint.NewService(fakeBindingStore)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	endpointServiceType := reflect.TypeFor[*endpoint.Service]()

	fakeSP.GetServiceCalls(func(v any) (any, error) {
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

	party := view.Identity("expected-party")
	ev := NewCollectEndorsementsView(et, party)

	fakeCtx.IsMeReturns(false)
	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	msgCh := make(chan *view.Message, 1)
	fakeSession.ReceiveReturns(msgCh)

	// The response is endorsed by "attacker", a completely different identity than
	// "expected-party", and the binding store confirms they are not bound together.
	resp := &mock.ProposalResponse{}
	resp.EndorserReturns([]byte("attacker"))
	resp.ResultsReturns([]byte("results"))
	// VerifyEndorsement succeeds unconditionally here: in a real deployment this
	// would succeed whenever "attacker" is any validly-enrolled MSP member, since
	// verification only checks the signature was produced by whoever `Endorser()`
	// claims to be - it says nothing about whether that signer is `party`.
	resp.VerifyEndorsementReturns(nil)
	fakeTM.NewProposalResponseFromBytesReturns(resp, nil)

	payload, _ := json.Marshal([][]byte{[]byte("resp-from-attacker")})
	msgCh <- &view.Message{Payload: payload}

	_, err := ev.Call(fakeCtx)

	// The overall Call must fail because the endorser is neither the contacted
	// party nor bound to it, even though the individual response passed signature
	// verification and was provisionally appended before the post-loop binding check.
	require.ErrorContains(t, err, "invalid endorsement, expected one signed by", "endorsement from an unbound identity must be rejected")
}

func TestVerifierProviderWrapper(t *testing.T) {
	t.Parallel()
	v := &verifierProviderWrapper{m: &fabric.MSPManager{}}
	require.Panics(t, func() { _, _ = v.GetVerifier([]byte("alice")) })
	require.NotNil(t, v)
}

type fakeView struct{}

func (v *fakeView) Call(context view.Context) (any, error) { return nil, nil }
