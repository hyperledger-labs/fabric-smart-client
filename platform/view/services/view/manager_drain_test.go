/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	grpcmock "github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc/mock"
	servicesmock "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	viewapi "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// blockingView blocks in Call until release is closed, or, if honorCancel, until its context
// is done.
func blockingView(entered chan<- struct{}, release <-chan struct{}, honorCancel bool) *mock.View {
	v := &mock.View{}
	v.CallCalls(func(c viewapi.Context) (any, error) {
		close(entered)
		if honorCancel {
			select {
			case <-release:
			case <-c.Context().Done():
				return nil, c.Context().Err()
			}
			return "ok", nil
		}
		<-release
		return "ok", nil
	})
	return v
}

// initiate runs v with InitiateView in a goroutine and returns the channel of its error.
func initiate(m *view.Manager, v viewapi.View) <-chan error {
	errs := make(chan error, 1)
	go func() {
		_, err := m.InitiateView(context.Background(), v)
		errs <- err
	}()
	return errs
}

func TestManagerDrain_WaitsForInflightViews(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	entered, release := make(chan struct{}), make(chan struct{})
	errs := initiate(m, blockingView(entered, release, false))
	<-entered

	// A responder the P2P service admitted, see Track.
	releaseResponder := m.Track()

	drained := make(chan error, 1)
	go func() { drained <- m.Drain(context.Background()) }()
	require.Eventually(t, m.Draining, time.Second, 10*time.Millisecond)

	close(release)
	require.NoError(t, <-errs)
	select {
	case <-drained:
		t.Fatal("Drain returned while a responder was running")
	case <-time.After(50 * time.Millisecond):
	}
	releaseResponder()
	require.NoError(t, <-drained)
}

func TestManagerDrain_IgnoresLeakedContexts(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	// A context from InitiateContext that nobody deletes runs no view.
	_, err := m.InitiateContext(t.Context(), &mock.View{})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, m.Drain(ctx))
}

func TestManagerDrain_CancelsInitiatorsAtDeadline(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	entered := make(chan struct{})
	errs := initiate(m, blockingView(entered, make(chan struct{}), true))
	<-entered

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, m.Drain(ctx), context.DeadlineExceeded)
	require.ErrorIs(t, <-errs, view.ErrCancelledByShutdown)
}

func TestManagerDrain_ReturnsAtDeadlineWhenViewIgnoresCancel(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	initiate(m, blockingView(entered, release, false))
	<-entered

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	require.ErrorIs(t, m.Drain(ctx), context.DeadlineExceeded)
	require.Less(t, time.Since(start), 2*time.Second, "the deadline plus the one-second grace")
}

func TestManagerDrain_SetsGauge(t *testing.T) {
	t.Parallel()
	draining := &grpcmock.Gauge{}
	p := &grpcmock.Provider{}
	p.NewGaugeReturns(draining)
	p.NewCounterReturns(&grpcmock.Counter{})
	p.NewHistogramReturns(&grpcmock.Histogram{})
	metrics := view.NewMetrics(p)
	require.Equal(t, "fsc", p.NewGaugeArgsForCall(1).Namespace)
	require.Equal(t, "draining", p.NewGaugeArgsForCall(1).Name)
	require.Equal(t, "shutdown", p.NewGaugeArgsForCall(1).Subsystem)

	ip := &mock.IdentityProvider{}
	registry := view.NewRegistry()
	cf := view.NewContextFactory(&servicesmock.ServiceProvider{}, &mock.SessionFactory{}, &mock.EndpointService{}, ip, registry, noop.NewTracerProvider(), metrics, &mock.LocalIdentityChecker{})
	m := view.NewManager(ip, registry, metrics, cf, view.NewDefaultRunner())

	require.NoError(t, m.Drain(t.Context()))
	require.Positive(t, draining.SetCallCount())
	require.InDelta(t, 1.0, draining.SetArgsForCall(draining.SetCallCount()-1), 0)
}

func TestManagerIsParty(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	_, release, err := m.NewResponderContext(t.Context(), "c1", newSession("s1", "alice", "pkid-alice"), nil, nil)
	require.NoError(t, err)
	defer release()
	_, releaseBob, err := m.NewResponderContext(t.Context(), "c1", newSession("s2", "bob", "pkid-bob"), nil, nil)
	require.NoError(t, err)
	defer releaseBob()

	require.True(t, m.IsParty("c1", []byte("pkid-alice")))
	require.True(t, m.IsParty("c1", []byte("pkid-bob")), "an attached session's peer is a party")
	require.False(t, m.IsParty("c1", []byte("pkid-mallory")))
	require.False(t, m.IsParty("unknown", []byte("pkid-alice")))

	// An initiator context is not a responder context.
	ic, err := m.InitiateContext(t.Context(), &mock.View{})
	require.NoError(t, err)
	require.False(t, m.IsParty(ic.ID(), []byte("pkid-alice")))
}

func TestManagerTrack_DoneIsIdempotent(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	done := m.Track()
	done()
	done()
	require.NoError(t, m.Drain(t.Context()), "a second done call must not make the count negative")
}

// Views that start and finish while Drain begins must neither panic nor hold the drain.
func TestManagerDrain_ConcurrentTrack(t *testing.T) {
	t.Parallel()
	for range 100 {
		m, _ := newResponderManager()
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 20 {
					m.Track()()
				}
			})
		}
		wg.Go(func() { require.NoError(t, m.Drain(t.Context())) })
		wg.Wait()
	}
}

// A view's context outlives the view: work it leaves running keeps the caller's context. Only
// the drain deadline cancels it.
func TestManager_InitiatorContextNotCancelledOnReturn(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	var viewCtx context.Context
	v := &mock.View{}
	v.CallCalls(func(c viewapi.Context) (any, error) {
		viewCtx = c.Context()
		return "ok", nil
	})
	_, err := m.InitiateView(context.Background(), v)
	require.NoError(t, err)
	require.NoError(t, viewCtx.Err(), "returning does not cancel the view's context")

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	_ = m.Drain(ctx)
	require.NoError(t, viewCtx.Err(), "the drain cancels only registered contexts; this one was deleted")
}

// TryTrack refuses new work once the drain started, and the drain waits for what it admitted.
func TestManagerTryTrack(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	done, ok := m.TryTrack()
	require.True(t, ok)

	drained := make(chan error, 1)
	go func() { drained <- m.Drain(context.Background()) }()
	require.Eventually(t, m.Draining, time.Second, time.Millisecond)
	_, ok = m.TryTrack()
	require.False(t, ok, "a drain refuses new work")
	select {
	case <-drained:
		t.Fatal("Drain returned while admitted work ran")
	case <-time.After(20 * time.Millisecond):
	}
	done()
	require.NoError(t, <-drained)
}

// A view cancelled by the drain keeps its own error in the chain.
func TestManagerDrain_CancelledKeepsViewError(t *testing.T) {
	t.Parallel()
	m, _ := newResponderManager()
	viewErr := errors.New("submit interrupted")
	entered := make(chan struct{})
	v := &mock.View{}
	v.CallCalls(func(c viewapi.Context) (any, error) {
		close(entered)
		<-c.Context().Done()
		return nil, viewErr
	})
	errs := initiate(m, v)
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_ = m.Drain(ctx)
	err := <-errs
	require.ErrorIs(t, err, view.ErrCancelledByShutdown)
	require.ErrorIs(t, err, viewErr)
}
