/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recorder records lifecycle events in order.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

// stoppingSDK records Stop calls, and whether its Start context was cancelled by then.
type stoppingSDK struct {
	mockSDK
	name string
	rec  *recorder
	stop func()
	ctx  context.Context
}

func (s *stoppingSDK) Start(ctx context.Context) error {
	s.ctx = ctx
	return nil
}

func (s *stoppingSDK) Stop() error {
	if s.stop != nil {
		s.stop()
	}
	if s.ctx.Err() == nil {
		s.rec.add("stop " + s.name + " before cancel")
		return nil
	}
	s.rec.add("stop " + s.name)
	return nil
}

// drainer records Drain, and blocks it until release is closed or ctx is done.
type drainer struct {
	rec     *recorder
	entered chan struct{}
	release chan struct{}
}

func (d *drainer) Drain(ctx context.Context) error {
	d.rec.add("drain")
	close(d.entered)
	select {
	case <-d.release:
		return nil
	case <-ctx.Done():
		d.rec.add("drain cancelled")
		return ctx.Err()
	}
}

func newDrainer(rec *recorder) *drainer {
	return &drainer{rec: rec, entered: make(chan struct{}), release: make(chan struct{})}
}

func TestNode_Stop_DrainsThenCancelsThenStopsInReverse(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	n := newTestNode()
	d := newDrainer(rec)
	require.NoError(t, n.RegisterService(d))
	n.AddSDK(&stoppingSDK{name: "a", rec: rec})
	n.AddSDK(&stoppingSDK{name: "b", rec: rec})
	require.NoError(t, n.Start())

	stopped := make(chan struct{})
	go func() {
		n.Stop()
		close(stopped)
	}()
	<-d.entered
	require.Equal(t, []string{"drain"}, rec.get(), "nothing is stopped while draining")
	require.NoError(t, n.ctx.Err(), "the node context is not cancelled while draining")
	close(d.release)
	<-stopped

	require.Equal(t, []string{"drain", "stop b", "stop a"}, rec.get())
}

func TestNode_Stop_SecondCallSkipsDrain(t *testing.T) {
	t.Parallel()
	rec := &recorder{}
	n := newTestNode()
	d := newDrainer(rec)
	require.NoError(t, n.RegisterService(d))
	n.AddSDK(&stoppingSDK{name: "a", rec: rec})
	require.NoError(t, n.Start())

	first := make(chan struct{})
	go func() {
		n.Stop()
		close(first)
	}()
	<-d.entered
	n.Stop() // returns only once the first call's shutdown has finished
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("the first Stop did not return")
	}
	require.Equal(t, []string{"drain", "drain cancelled", "stop a"}, rec.get())
}

func TestNode_Stop_SDKWithoutStopperIsCancelled(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	sdk := &mockSDK{}
	n.AddSDK(sdk)
	require.NoError(t, n.Start())
	require.NotPanics(t, n.Stop)
	require.ErrorIs(t, n.ctx.Err(), context.Canceled)
}

func TestNode_Stop_BoundedByteardownTimeout(t *testing.T) {
	t.Parallel()
	n := newTestNode()
	n.config = fixedTimeouts{teardown: 50 * time.Millisecond}
	release := make(chan struct{})
	defer close(release)
	n.AddSDK(&stoppingSDK{name: "stuck", rec: &recorder{}, stop: func() { <-release }})
	require.NoError(t, n.Start())

	start := time.Now()
	n.Stop()
	require.Less(t, time.Since(start), time.Second)
}

func TestShutdownTimeouts(t *testing.T) {
	t.Parallel()
	require.Equal(t, defaultShutdownTimeout, drainTimeout(nil))
	require.Equal(t, defaultShutdownTimeout, teardownTimeout(fixedTimeouts{}))
	require.Equal(t, time.Duration(0), drainTimeout(fixedTimeouts{drain: 0, set: true}))
	require.Equal(t, defaultShutdownTimeout, drainTimeout(fixedTimeouts{drain: -time.Second, set: true}))
	require.Equal(t, 2*time.Second, drainTimeout(fixedTimeouts{drain: 2 * time.Second, set: true}))
}

// fixedTimeouts is a ConfigService with the given shutdown timeouts. With set false, only a
// non-zero value counts as set.
type fixedTimeouts struct {
	drain, teardown time.Duration
	set             bool
}

func (f fixedTimeouts) IsSet(key string) bool {
	return f.set || f.GetDuration(key) != 0
}

func (f fixedTimeouts) GetDuration(key string) time.Duration {
	if key == drainTimeoutKey {
		return f.drain
	}
	return f.teardown
}
