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

// parallelEndorsementFixture bundles the fakes that every
// TestParallelCollectEndorsementsOnProposalView_* case needs, so each test only spells out
// the behaviour it is actually about.
type parallelEndorsementFixture struct {
	ctx     *mock.Context
	session *mock.Session
	tx      *mock.Transaction
	tm      *mock.TransactionManager
	ft      *Transaction
}

// newParallelEndorsementFixture wires a view context whose service provider resolves the
// network service for "net1"/"ch1", together with the transaction the view collects
// endorsements for and the session it reaches the parties over. Pass an endpoint.Service to
// have it resolved through the same provider - only the binding check in Call needs one, and
// the cases that do not reach that check never look it up.
func newParallelEndorsementFixture(endpointService *endpoint.Service) *parallelEndorsementFixture {
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
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		if v == endpointServiceType && endpointService != nil {
			return endpointService, nil
		}
		return nil, nil
	})

	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeCH.ChannelMembershipReturns(&mock.ChannelMembership{})

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.BytesReturns([]byte("raw"), nil)
	fakeTx.IDReturns("tx1")

	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	fakeCtx.InitiatorReturns(&fakeView{})

	return &parallelEndorsementFixture{
		ctx:     fakeCtx,
		session: fakeSession,
		tx:      fakeTx,
		tm:      fakeTM,
		ft: &Transaction{
			Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx),
		},
	}
}

// answerWith makes the fixture's session deliver a single Response carrying prs, endorsed by
// endorser, and has the transaction manager unmarshal it into a proposal response that
// verifies. Returns that proposal response so a test can override what it reports.
func (f *parallelEndorsementFixture) answerWith(t *testing.T, endorser view.Identity, prs ...[]byte) *mock.ProposalResponse {
	t.Helper()
	respPayload, err := json.Marshal(&Response{ProposalResponses: prs})
	require.NoError(t, err)
	msgCh := make(chan *view.Message, 1)
	msgCh <- &view.Message{Payload: respPayload}
	f.session.ReceiveReturns(msgCh)

	fakeResp := &mock.ProposalResponse{}
	fakeResp.EndorserReturns(endorser)
	fakeResp.VerifyEndorsementReturns(nil)
	f.tm.NewProposalResponseFromBytesReturns(fakeResp, nil)
	return fakeResp
}

func TestParallelCollectEndorsementsOnProposalViewInternal(t *testing.T) {
	t.Parallel()
	f := newParallelEndorsementFixture(nil)

	v := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob"))
	v.WithTimeout(1 * time.Second)

	// Case 1: Success
	f.answerWith(t, []byte("bob"), []byte("resp1"))

	_, err := v.Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, f.tx.AppendProposalResponseCallCount())

	// Case 2: Session error
	f.ctx.GetSessionReturns(nil, fmt.Errorf("err"))
	_, err = v.Call(f.ctx)
	require.Error(t, err)

	// Case 3: Send error
	f.ctx.GetSessionReturns(f.session, nil)
	f.session.SendReturns(fmt.Errorf("err"))
	_, err = v.Call(f.ctx)
	require.Error(t, err)

	// Case 4: Receive error
	f.session.SendReturns(nil)
	msgCh := make(chan *view.Message, 1)
	msgCh <- &view.Message{Status: view.ERROR, Payload: []byte("err")}
	f.session.ReceiveReturns(msgCh)
	_, err = v.Call(f.ctx)
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
	fakeBindingStore := &mock.BindingStore{}
	fakeBindingStore.HaveSameBindingReturns(false, nil)
	endpointService, err := endpoint.NewService(fakeBindingStore)
	require.NoError(t, err)

	f := newParallelEndorsementFixture(endpointService)

	// Party contacted is "bob", but the returned response claims to be endorsed by
	// "mallory" - an identity with no relationship to "bob" whatsoever.
	v := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob"))
	v.WithTimeout(1 * time.Second)

	// This response is neither signed by "bob" nor bound to "bob", so it must be
	// rejected regardless of what VerifyEndorsement would say.
	fakeResp := f.answerWith(t, []byte("mallory"), []byte("resp-from-mallory"))
	fakeResp.VerifyEndorsementReturns(errors.New("signature verification failed"))

	_, err = v.Call(f.ctx)

	require.Error(t, err, "unverified proposal response from an unrelated identity must be rejected")
	require.Equal(t, 0, f.tx.AppendProposalResponseCallCount())
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
	f := newParallelEndorsementFixture(nil)

	// A short timeout so the test doesn't wait out defaultParallelEndorsementTimeout; the
	// party's session never delivers a response (empty, never-fed channel), simulating a
	// silent/unresponsive remote peer.
	v := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob"))
	v.WithTimeout(50 * time.Millisecond)
	f.session.ReceiveReturns(make(chan *view.Message))

	done := make(chan struct{})
	var err error
	go func() {
		_, err = v.Call(f.ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Call did not return - it appears to be blocking forever on the silent party")
	}

	require.Error(t, err)
	// The per-party ReceiveWithTimeout expires a full parallelEndorsementDeadlineGrace before
	// the aggregate deadline, so barring a pathological scheduling stall the error reported is
	// the per-party one, which names the party that went silent.
	require.Contains(t, err.Error(), "time out reached on session")
	require.Contains(t, err.Error(), "got failure from ["+view.Identity("bob").String()+"]")
}

// TestParallelCollectEndorsementsOnProposalView_TimesOutOnPartyThatNeverAcceptsTheSend covers
// the aggregate deadline itself: collectEndorsement's own ReceiveWithTimeout bounds only the
// receive step, so a party that parks the goroutine in session setup or SendRaw is caught by
// nothing else. Here the send blocks forever, so the per-party timeout never arms and the
// aggregate deadline in Call is the only thing that can return.
func TestParallelCollectEndorsementsOnProposalView_TimesOutOnPartyThatNeverAcceptsTheSend(t *testing.T) {
	t.Parallel()
	f := newParallelEndorsementFixture(nil)

	v := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob"))
	v.WithTimeout(50 * time.Millisecond)

	// Release the parked goroutine when the test ends so it does not outlive the test.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	f.session.ReceiveReturns(make(chan *view.Message))
	f.session.SendCalls(func(context.Context, []byte) error {
		<-release
		return nil
	})

	done := make(chan struct{})
	var err error
	go func() {
		_, err = v.Call(f.ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Call did not return - it appears to be blocking forever on a party stuck in send")
	}

	require.Error(t, err)
	require.Contains(t, err.Error(), "timeout waiting for endorsement from [1] parties")
}

// TestParallelCollectEndorsementsOnProposalView_ReportsAppendFailure guards against
// AppendProposalResponse failures being swallowed: the error must reach the caller rather than
// Call returning a nil transaction and a nil error.
func TestParallelCollectEndorsementsOnProposalView_ReportsAppendFailure(t *testing.T) {
	t.Parallel()
	f := newParallelEndorsementFixture(nil)
	f.tx.AppendProposalResponseReturns(errors.New("cannot append"))

	v := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob"))
	v.WithTimeout(1 * time.Second)

	f.answerWith(t, []byte("bob"), []byte("resp1"))

	tx, err := v.Call(f.ctx)
	require.Error(t, err)
	require.Nil(t, tx)
	require.Contains(t, err.Error(), "failed appending response from ["+view.Identity("bob").String()+"]")
	require.Contains(t, err.Error(), "cannot append")
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
	fakeSession.SendReturns(fmt.Errorf("err"))
	_, err = ev.Call(fakeCtx)
	require.Error(t, err)

	// Case 3: EndorseProposalResponseWithIdentity failure
	fakeSession.SendReturns(nil)
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
