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
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/security"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestCollectEndorsementsView(t *testing.T) {
	t.Parallel()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(t.Context())
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
	fakeFNS.ConfigServiceReturns(genericConfig())
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

// endorseFixture is a view context for endorseView whose transaction is a
// valid received transaction for channel ch1 of network net1.
type endorseFixture struct {
	ctx       *mock.Context
	tx        *mock.Transaction
	ts        *mock.EndorserTransactionService
	et        *Transaction
	defaultTS *mock.EndorserTransactionService
}

func newEndorseFixture(t *testing.T) *endorseFixture {
	t.Helper()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(t.Context())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeTx := &mock.Transaction{}
	fakeTx.NetworkReturns("net1")
	fakeTx.ChannelReturns("ch1")
	fakeTx.IDReturns(testTxID)
	fakeTx.NonceReturns(testNonce)
	fakeTx.CreatorReturns(testCreator)
	fakeTx.BytesReturns([]byte("txraw"), nil)
	fakeTx.ProposalResponseReturns([]byte("pr"), nil)

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeCH.ChannelMembershipReturns(&mock.ChannelMembership{})
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeFNS.ConfigServiceReturns(genericConfig())
	fakeTM := &mock.TransactionManager{}
	fakeTM.ComputeTxIDStub = func(id *driver.TxIDComponents) string { return protoutil.ComputeTxID(id.Nonce, id.Creator) }
	fakeFNS.TransactionManagerReturns(fakeTM)

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

	// The default network, net0, has a channel ch1 of its own; the transaction
	// belongs to net1.
	defaultTS := &mock.EndorserTransactionService{}
	defaultCH := &mock.Channel{}
	defaultCH.NameReturns("ch1")
	defaultCH.TransactionServiceReturns(defaultTS)
	defaultFNS := &mock.FabricNetworkService{}
	defaultFNS.NameReturns("net0")
	defaultFNS.ChannelReturns(defaultCH, nil)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceCalls(func(id string) (driver.FabricNetworkService, error) {
		if id == "" || id == "net0" {
			return defaultFNS, nil
		}
		return fakeFNS, nil
	})
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)

	networkServiceProviderType := reflect.TypeFor[*fabric.NetworkServiceProvider]()
	fakeSP.GetServiceCalls(func(v any) (any, error) {
		if v == networkServiceProviderType {
			return fakeNSP, nil
		}
		return nil, nil
	})

	fakeCtx.SessionReturns(&mock.Session{})

	fns := fabric.NewNetworkService(nil, fakeFNS, "net1")
	return &endorseFixture{
		ctx:       fakeCtx,
		tx:        fakeTx,
		ts:        fakeTS,
		defaultTS: defaultTS,
		et:        &Transaction{Transaction: fabric.NewTransaction(fns, fakeTx)},
	}
}

func TestEndorseView(t *testing.T) {
	t.Parallel()
	f := newEndorseFixture(t)
	ev := NewEndorseView(f.et)

	_, err := ev.Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, f.ts.StoreTransactionCallCount())
	require.Zero(t, f.defaultTS.StoreTransactionCallCount(), "stored on the transaction's network, not the default one")

	// Error path: tx.EndorseWithIdentity failure
	f.tx.EndorseWithIdentityReturns(fmt.Errorf("err"))
	_, err = ev.Call(f.ctx)
	require.Error(t, err)
	f.tx.EndorseWithIdentityReturns(nil)

	// Error path: tx.ProposalResponse failure
	f.tx.ProposalResponseReturns(nil, fmt.Errorf("err"))
	_, err = ev.Call(f.ctx)
	require.Error(t, err)
}

// A different transaction already stored under the txID fails the endorsement
// and is counted as a store conflict.
func TestEndorseView_ReportsStoreConflict(t *testing.T) {
	t.Parallel()
	f := newEndorseFixture(t)
	counts := &countingMetrics{}
	rejections := security.NewRejections(counts)
	getService := f.ctx.GetServiceStub
	f.ctx.GetServiceCalls(func(v any) (any, error) {
		if v == reflect.TypeFor[*security.Rejections]() {
			return rejections, nil
		}
		return getService(v)
	})
	f.ts.StoreTransactionReturns(fmt.Errorf("key taken: %w", dbdriver.ErrConflict))

	_, err := NewEndorseView(f.et).Call(f.ctx)
	require.ErrorIs(t, err, dbdriver.ErrConflict)
	require.InDelta(t, 1, counts.count(security.KindStoreConflict), 0)
	require.Zero(t, counts.count(security.KindInvalidTx))
}

func TestEndorseView_RejectsForgedTxIDBeforeSigning(t *testing.T) {
	t.Parallel()
	f := newEndorseFixture(t)
	f.tx.IDReturns("forged")

	_, err := NewEndorseView(f.et).Call(f.ctx)
	require.ErrorIs(t, err, ErrInvalidTransaction)
	require.Zero(t, f.tx.EndorseWithIdentityCallCount())
	require.Zero(t, f.ts.StoreTransactionCallCount())
}

func TestAcceptView(t *testing.T) {
	t.Parallel()
	f := newEndorseFixture(t)

	_, err := NewAcceptView(f.et, []byte("alice")).Call(f.ctx)
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
	fakeFNS.ConfigServiceReturns(genericConfig())
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
	fakeFNS.ConfigServiceReturns(genericConfig())
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
	fakeFNS.ConfigServiceReturns(genericConfig())
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

// newResponderFixture returns a view context and the transaction, on a Fabric network, that
// an endorsementsOnProposalResponderView endorses.
func newResponderFixture() (*mock.Context, *mock.Transaction, *Transaction) {
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(context.Background())
	fakeSP := &mock.Provider{}
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		return fakeSP.GetService(v)
	})

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.ConfigServiceReturns(genericConfig())
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
	return fakeCtx, fakeTx, et
}

func TestEndorsementOnProposalResponderViewInternal(t *testing.T) {
	t.Parallel()
	fakeCtx, fakeTx, et := newResponderFixture()

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

// collectFixture is a collectEndorsementsView that asks "bob" to endorse a tx whose
// results are "results". bob's reply is pushed on msgCh, and tm parses each response.
type collectFixture struct {
	ctx   *mock.Context
	tx    *mock.Transaction
	tm    *mock.TransactionManager
	msgCh chan *view.Message
	view  *collectEndorsementsView
}

func newCollectFixture(t *testing.T) *collectFixture {
	t.Helper()
	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(t.Context())
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
	fakeFNS.ConfigServiceReturns(genericConfig())
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)

	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	fakeNSP := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	// The zero-value BindingStore reports no identity bound to another.
	endpointService, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

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
	et := &Transaction{Transaction: fabric.NewTransaction(fns, fakeTx)}

	fakeCtx.IsMeReturns(false)
	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	msgCh := make(chan *view.Message, 1)
	fakeSession.ReceiveReturns(msgCh)

	return &collectFixture{ctx: fakeCtx, tx: fakeTx, tm: fakeTM, msgCh: msgCh, view: NewCollectEndorsementsView(et, []byte("bob"))}
}

// reply pushes bob's reply carrying responses.
func (f *collectFixture) reply(t *testing.T, responses ...[]byte) {
	t.Helper()
	if responses == nil {
		responses = [][]byte{}
	}
	payload, err := json.Marshal(responses)
	require.NoError(t, err)
	f.msgCh <- &view.Message{Payload: payload}
}

// TestCollectEndorsementsView_RejectsEndorsementFromUnboundParty checks that a validly
// signed endorsement from an identity that is neither the contacted party nor bound to
// it is rejected: signature verification only proves who signed, not that the signer is
// the party asked.
func TestCollectEndorsementsView_RejectsEndorsementFromUnboundParty(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	resp := &mock.ProposalResponse{}
	resp.EndorserReturns([]byte("attacker"))
	resp.ResultsReturns([]byte("results"))
	resp.VerifyEndorsementReturns(nil)
	f.tm.NewProposalResponseFromBytesReturns(resp, nil)
	f.reply(t, []byte("resp-from-attacker"))

	_, err := f.view.Call(f.ctx)
	require.ErrorContains(t, err, "invalid endorsement, expected one signed by")
}

func TestCollectEndorsementsView_RejectsDifferentResults(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	// bob signs validly, but over results other than the ones the tx holds.
	resp := &mock.ProposalResponse{}
	resp.EndorserReturns([]byte("bob"))
	resp.ResultsReturns([]byte("other"))
	resp.VerifyEndorsementReturns(nil)
	f.tm.NewProposalResponseFromBytesReturns(resp, nil)
	f.reply(t, []byte("resp"))

	_, err := f.view.Call(f.ctx)
	require.ErrorContains(t, err, "received different results")
	require.Zero(t, f.tx.AppendProposalResponseCallCount())
}

func TestCollectEndorsementsView_RejectsEmptyResponse(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	f.reply(t)

	_, err := f.view.Call(f.ctx)
	require.ErrorContains(t, err, "invalid endorsement, expected one signed by")
	require.Zero(t, f.tx.AppendProposalResponseCallCount())
}

func TestCollectEndorsementsView_RejectsGarbledResponse(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	f.tm.NewProposalResponseFromBytesReturns(nil, errors.New("garbled"))
	f.reply(t, []byte("garbled"))

	_, err := f.view.Call(f.ctx)
	require.ErrorContains(t, err, "failed unmarshalling received proposal response")
	require.Zero(t, f.tx.AppendProposalResponseCallCount())
}

// A view waiting for a party's endorsement must end as soon as its caller cancels it, not
// when its one-minute timer fires, so that it closes its sessions and the party's
// responder ends too.
func TestCollectEndorsementsView_ReturnsWhenContextEnds(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f.ctx.ContextReturns(ctx)

	start := time.Now()
	_, err := f.view.Call(f.ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), time.Second, "the view waited for its timer instead of the context")
}

// A caller that spreads endorsements over several endorsers tells "the endorser is
// unavailable, try the next one" from any other failure with errors.Is. A party that fails
// before its reply verifies adds nothing to the transaction.
func TestCollectEndorsementsView_ErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("rejection", func(t *testing.T) {
		t.Parallel()
		for reason, want := range map[string]error{
			session.ReasonDraining:    session.ErrDraining,
			session.ReasonBusy:        session.ErrBusy,
			session.ReasonNodeBusy:    session.ErrBusy,
			session.ReasonUnknownView: session.ErrUnknownView,
		} {
			f := newCollectFixture(t)
			f.msgCh <- &view.Message{Status: view.ERROR, Payload: []byte(reason + ": no new work")}
			_, err := f.view.Call(f.ctx)
			require.ErrorIs(t, err, want, reason)
			require.ErrorIs(t, err, session.ErrRejected, reason)
			require.EqualError(t, err, reason+": no new work")
			require.Zero(t, f.tx.AppendProposalResponseCallCount(), reason)
		}
	})

	t.Run("error reply that is not a rejection", func(t *testing.T) {
		t.Parallel()
		f := newCollectFixture(t)
		f.msgCh <- &view.Message{Status: view.ERROR, Payload: []byte("failed endorsing: bad input")}
		_, err := f.view.Call(f.ctx)
		require.EqualError(t, err, "failed endorsing: bad input")
		require.NotErrorIs(t, err, session.ErrRejected)
	})

	t.Run("session closed", func(t *testing.T) {
		t.Parallel()
		f := newCollectFixture(t)
		close(f.msgCh)
		_, err := f.view.Call(f.ctx)
		require.ErrorIs(t, err, comm.ErrSessionClosed)
		require.Regexp(t, "^session to party .+ closed$", err.Error())
	})

	t.Run("no reply before the timeout", func(t *testing.T) {
		t.Parallel()
		// The fixture is built in the bubble, whose clock passes the view's one-minute wait
		// as soon as everything blocks.
		synctest.Test(t, func(t *testing.T) {
			f := newCollectFixture(t)
			_, err := f.view.Call(f.ctx)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Regexp(t, "^Timeout from party .+$", err.Error())
		})
	})
}

// TestEndorsement_HijackerEndorsementRefused shows that a proven binding of a
// recipient identity X to its owner's node defeats a hijack: mallory, who
// planted X -> mallory before X's owner proved X, binds her own Fabric
// identity M to her node and answers CollectEndorsementsView(tx, X) with an
// endorsement signed by M. Once the owner's proven bind replaces the planted
// row, M is not bound to X and the endorsement is refused.
func TestEndorsement_HijackerEndorsementRefused(t *testing.T) {
	t.Parallel()
	// The memory driver shares one database per process, and run 1 ends with X
	// proven-bound to its owner; unique identities keep later runs (-count=N)
	// from starting with that row.
	u := strconv.FormatInt(time.Now().UnixNano(), 10)
	ownerNode, malloryNode := view.Identity("owner-node"+u), view.Identity("mallory-node"+u)
	x, m := view.Identity("x"+u), view.Identity("m"+u)

	store, err := mem.NewDriver().NewBinding("", "hijack")
	require.NoError(t, err)
	endpointService, err := endpoint.NewService(store)
	require.NoError(t, err)
	_, err = endpointService.AddResolver("owner", "", nil, nil, ownerNode)
	require.NoError(t, err)
	_, err = endpointService.AddResolver("mallory", "", nil, nil, malloryNode)
	require.NoError(t, err)
	ctx := t.Context()
	require.NoError(t, endpointService.Bind(ctx, malloryNode, x, m))
	require.True(t, endpointService.IsBoundTo(ctx, m, x), "the planted row makes M pass as X")

	require.NoError(t, endpointService.BindProven(ctx, ownerNode, x))
	require.False(t, endpointService.IsBoundTo(ctx, m, x))

	fakeCtx := &mock.Context{}
	fakeCtx.ContextReturns(ctx)
	fakeTx := &mock.Transaction{}
	fakeTx.ChannelReturns("ch1")
	fakeTx.NetworkReturns("net1")
	fakeRWS := &mock.RWSet{}
	fakeRWS.BytesReturns([]byte("results"), nil)
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	fakeTx.BytesReturns([]byte("bytes"), nil)
	fakeTx.ResultsReturns([]byte("results"), nil)
	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.ConfigServiceReturns(genericConfig())
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeCH.ChannelMembershipReturns(&mock.ChannelMembership{})
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeTM := &mock.TransactionManager{}
	fakeFNS.TransactionManagerReturns(fakeTM)
	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	nsp := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		switch v {
		case reflect.TypeFor[*fabric.NetworkServiceProvider]():
			return nsp, nil
		case reflect.TypeFor[*endpoint.Service]():
			return endpointService, nil
		}
		return nil, nil
	})

	fakeSession := &mock.Session{}
	fakeCtx.GetSessionReturns(fakeSession, nil)
	msgCh := make(chan *view.Message, 1)
	fakeSession.ReceiveReturns(msgCh)
	resp := &mock.ProposalResponse{}
	resp.EndorserReturns(m)
	resp.ResultsReturns([]byte("results"))
	resp.VerifyEndorsementReturns(nil)
	fakeTM.NewProposalResponseFromBytesReturns(resp, nil)
	payload, err := json.Marshal([][]byte{[]byte("resp-from-mallory")})
	require.NoError(t, err)
	msgCh <- &view.Message{Payload: payload}

	et := &Transaction{Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx)}
	_, err = NewCollectEndorsementsView(et, x).Call(fakeCtx)
	require.ErrorContains(t, err, "invalid endorsement, expected one signed by")
}

func TestVerifierProviderWrapper(t *testing.T) {
	t.Parallel()
	v := &verifierProviderWrapper{m: &fabric.MSPManager{}}
	require.Panics(t, func() { _, _ = v.GetVerifier([]byte("alice")) })
	require.NotNil(t, v)
}

type fakeView struct{}

func (*fakeView) Call(_ view.Context) (any, error) { return nil, nil }

// keyedTx is a driver transaction whose channel key cannot be read.
type keyedTx struct {
	*mock.Transaction
	err error
}

func (k *keyedTx) ChannelKey() (*driver.ChannelKey, error) { return nil, k.err }

// A transaction is not sent to another node while the key of its channel cannot
// be read: its bytes would lack the key, and the receiver would build it on its
// default network.
func TestSendViewsRequireChannelKey(t *testing.T) {
	t.Parallel()
	dtx := &keyedTx{Transaction: &mock.Transaction{}, err: errors.New("ledger unavailable")}
	dtx.IDReturns("tx1")
	dtx.ChannelReturns("ch1")
	ch := &mock.Channel{}
	ch.ChannelMembershipReturns(&mock.ChannelMembership{})
	fns := &mock.FabricNetworkService{}
	fns.ConfigServiceReturns(genericConfig())
	fns.ChannelReturns(ch, nil)
	tx := &Transaction{Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fns, "net1"), dtx)}
	rws := &mock.RWSet{}
	rws.BytesReturns([]byte("results"), nil)
	dtx.GetRWSetReturns(rws, nil)
	ctx := &mock.Context{}
	ctx.ContextReturns(t.Context())

	for name, v := range map[string]view.View{
		"collect":  NewCollectEndorsementsView(tx, []byte("bob")),
		"parallel": NewParallelCollectEndorsementsOnProposalView(tx, []byte("bob")),
	} {
		_, err := v.Call(ctx)
		require.ErrorContains(t, err, "cannot send tx [tx1] of channel [ch1] to another node: ledger unavailable", name)
	}
	require.Zero(t, ctx.GetSessionCallCount(), "nothing is sent")
	require.Zero(t, dtx.BytesCallCount())
}

// genericConfig is the configuration of a Fabric network.
func genericConfig() *mock.ConfigService {
	c := &mock.ConfigService{}
	c.DriverNameReturns("generic")
	return c
}

// The parallel view sends each party the form its session negotiated.
func TestParallelCollectEndorsementsOnProposalView_FormPerSession(t *testing.T) {
	t.Parallel()
	f := newParallelEndorsementFixture(nil)
	f.tx.BytesReturns(wireTxRaw, nil)
	sessions := map[string]*versionedSession{"bob": {version: 1}, "carol": {version: 2}}
	for _, s := range sessions {
		ch := make(chan *view.Message, 1)
		ch <- &view.Message{Payload: []byte(`{}`)}
		s.ReceiveReturns(ch)
	}
	f.ctx.GetSessionCalls(func(_ view.View, party view.Identity, _ ...view.View) (view.Session, error) {
		return sessions[string(party)], nil
	})

	_, err := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob"), []byte("carol")).WithTimeout(time.Second).Call(f.ctx)
	require.NoError(t, err)

	_, sent := sessions["bob"].SendArgsForCall(0)
	require.Equal(t, wireTxRaw, sent)
	_, sent = sessions["carol"].SendArgsForCall(0)
	want, err := protos.EnvelopeFromJSON(wireTxRaw)
	require.NoError(t, err)
	require.Equal(t, want, sent)
}

// The sequential view sends a party at version 2 the envelope.
func TestCollectEndorsementsView_SendsEnvelopeAtV2(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	f.tx.BytesReturns(wireTxRaw, nil)
	s := &versionedSession{version: 2}
	s.ReceiveReturns(f.msgCh)
	f.ctx.GetSessionReturns(s, nil)
	f.reply(t)

	// bob's reply carries no endorsement, so Call fails after sending; only what it sent matters.
	_, err := f.view.Call(f.ctx)
	require.Error(t, err)
	require.Equal(t, 1, s.SendCallCount())
	_, sent := s.SendArgsForCall(0)
	want, err := protos.EnvelopeFromJSON(wireTxRaw)
	require.NoError(t, err)
	require.Equal(t, want, sent)
}

// protoResponses is the EndorsementResponse carrying prs, as sent at version 2.
func protoResponses(t *testing.T, prs ...[]byte) []byte {
	t.Helper()
	raw, err := proto.Marshal(&protos.EndorsementResponse{ProposalResponses: prs})
	require.NoError(t, err)
	return raw
}

func TestDecodeResponses(t *testing.T) {
	t.Parallel()
	prs := [][]byte{[]byte("pr1"), []byte("pr2")}
	array, err := json.Marshal(prs)
	require.NoError(t, err)
	object, err := json.Marshal(&Response{ProposalResponses: prs})
	require.NoError(t, err)

	for name, raw := range map[string][]byte{
		"JSON array":  array,
		"JSON object": object,
		"protobuf":    protoResponses(t, prs...),
	} {
		got, err := decodeResponses(raw)
		require.NoError(t, err, name)
		require.Equal(t, prs, got, name)
	}

	got, err := decodeResponses([]byte("null"))
	require.NoError(t, err)
	require.Empty(t, got)

	for name, raw := range map[string][]byte{
		"empty":        nil,
		"bad JSON":     []byte("[1"),
		"bad protobuf": {0x0a, 0x05},
	} {
		_, err := decodeResponses(raw)
		require.Error(t, err, name)
	}
}

// endorseView answers in the encoding its session negotiated: today's JSON array at v1,
// an EndorsementResponse from v2.
func TestEndorseView_ResponseEncoding(t *testing.T) {
	t.Parallel()
	v1, err := json.Marshal([][]byte{[]byte("pr")})
	require.NoError(t, err)
	for version, want := range map[uint32][]byte{1: v1, 2: protoResponses(t, []byte("pr"))} {
		f := newEndorseFixture(t)
		s := &versionedSession{version: version}
		f.ctx.SessionReturns(s)

		_, err := NewEndorseView(f.et).Call(f.ctx)
		require.NoError(t, err)
		require.Equal(t, 1, s.SendCallCount())
		_, sent := s.SendArgsForCall(0)
		require.Equal(t, want, sent, "v%d", version)
		got, err := decodeResponses(sent)
		require.NoError(t, err)
		require.Equal(t, [][]byte{[]byte("pr")}, got)
	}
}

// The sequential view accepts an EndorsementResponse and today's JSON null.
func TestCollectEndorsementsView_DecodesResponses(t *testing.T) {
	t.Parallel()
	f := newCollectFixture(t)
	resp := &mock.ProposalResponse{}
	resp.EndorserReturns([]byte("bob"))
	resp.ResultsReturns([]byte("results"))
	f.tm.NewProposalResponseFromBytesReturns(resp, nil)
	f.msgCh <- &view.Message{Payload: protoResponses(t, []byte("resp1"))}

	_, err := f.view.Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, f.tx.AppendProposalResponseCallCount())
	pr := f.tm.NewProposalResponseFromBytesArgsForCall(0)
	require.Equal(t, []byte("resp1"), pr)

	f.msgCh <- &view.Message{Payload: []byte("null")}
	_, err = f.view.Call(f.ctx)
	require.ErrorContains(t, err, "invalid endorsement, expected one signed by")
}

// The parallel responder answers in the encoding its session negotiated: today's Response
// object at v1, an EndorsementResponse from v2.
func TestEndorsementOnProposalResponderView_ResponseEncoding(t *testing.T) {
	t.Parallel()
	pr := &mock.ProposalResponse{}
	pr.BytesReturns([]byte("pr1"), nil)
	// The bytes the responder sent through session.JSON(viewCtx).Send(&Response{...}).
	v1, err := json.Marshal(&Response{ProposalResponses: [][]byte{[]byte("pr1")}})
	require.NoError(t, err)
	for version, want := range map[uint32][]byte{1: v1, 2: protoResponses(t, []byte("pr1"))} {
		ctx, tx, et := newResponderFixture()
		tx.ProposalResponsesReturns([]driver.ProposalResponse{pr}, nil)
		s := &versionedSession{version: version}
		ctx.SessionReturns(s)

		_, err := NewEndorsementOnProposalResponderView(et).Call(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, s.SendCallCount())
		_, sent := s.SendArgsForCall(0)
		require.Equal(t, want, sent, "v%d", version)
		got, err := decodeResponses(sent)
		require.NoError(t, err)
		require.Equal(t, [][]byte{[]byte("pr1")}, got)
	}
}

// The parallel view accepts an EndorsementResponse.
func TestParallelCollectEndorsementsOnProposalView_DecodesProto(t *testing.T) {
	t.Parallel()
	f := newParallelEndorsementFixture(nil)
	f.answerWith(t, []byte("bob"))
	ch := make(chan *view.Message, 1)
	ch <- &view.Message{Payload: protoResponses(t, []byte("resp1"))}
	f.session.ReceiveReturns(ch)

	_, err := NewParallelCollectEndorsementsOnProposalView(f.ft, []byte("bob")).WithTimeout(time.Second).Call(f.ctx)
	require.NoError(t, err)
	require.Equal(t, 1, f.tx.AppendProposalResponseCallCount())
	require.Equal(t, []byte("resp1"), f.tm.NewProposalResponseFromBytesArgsForCall(0))
}
