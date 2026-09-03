/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deferred_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/deferred"
)

type config struct{ id string }

func TestGetBeforeFirstUpdate(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	v, err := h.Get()
	require.Error(t, err)
	require.Nil(t, v)
	require.True(t, errors.Is(err, deferred.ErrNotLoaded),
		"error must be matchable with errors.Is, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the channel, got [%v]", err)
}

func TestGetAfterUpdate(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))

	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, "first", v.id)
}

func TestFailedUpdateLeavesPreviousValue(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))

	require.EqualError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "second"}, errors.New("boom")
	}), "boom")

	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, "first", v.id, "a failed update must not clobber the held value")
}

func TestFailedFirstUpdateStaysUninitialized(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("boom")
	}))

	v, err := h.Get()
	require.Nil(t, v)
	require.Error(t, err)
}

// TestRejectedFirstUpdateIsNotReportedAsStartup asserts that a holder that was
// offered a configuration and refused it is distinguishable from one that has
// not been offered anything. The two are both empty, but only the second is a
// startup race worth retrying.
func TestRejectedFirstUpdateIsNotReportedAsStartup(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("bad consensus type")
	}))

	_, err := h.Get()
	require.True(t, errors.Is(err, deferred.ErrRejected), "must report the rejection")
	require.False(t, errors.Is(err, deferred.ErrNotLoaded), "must not invite a retry that cannot help")
	require.ErrorContains(t, err, "bad consensus type", "the reason the update was refused must survive")
}

// TestRejectionIsClearedByALaterSuccess asserts that a holder recovers when a
// subsequent configuration is accepted: the earlier rejection must not keep
// being reported once a good value is in place.
func TestRejectionIsClearedByALaterSuccess(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("boom")
	}))
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "good"}, nil
	}))

	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, "good", v.id)
}

// TestRejectionAfterASuccessIsNotReported asserts that a refused update does not
// mask a configuration already in force. Get keeps answering from the held
// value, and the caller learns about the refusal from Update's own error.
func TestRejectionAfterASuccessIsNotReported(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("boom")
	}))

	v, err := h.Get()
	require.NoError(t, err, "a rejected update must not invalidate the configuration in force")
	require.Equal(t, "first", v.id)
}

// TestTryGetIgnoresRejection asserts that TryGet keeps its two-state contract:
// it reports whether a configuration is held, and a rejection is not one.
func TestTryGetIgnoresRejection(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("boom")
	}))

	v, ok := h.TryGet()
	require.False(t, ok)
	require.Nil(t, v)
}

func TestUpdateSeesCurrentValue(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	require.NoError(t, h.Update(func(cur *config, loaded bool) (*config, error) {
		require.False(t, loaded)
		require.Nil(t, cur)
		return &config{id: "first"}, nil
	}))

	require.NoError(t, h.Update(func(cur *config, loaded bool) (*config, error) {
		require.True(t, loaded)
		require.Equal(t, "first", cur.id)
		return &config{id: "second"}, nil
	}))
}

func TestTryGet(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	v, ok := h.TryGet()
	require.False(t, ok)
	require.Nil(t, v)

	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))

	v, ok = h.TryGet()
	require.True(t, ok)
	require.Equal(t, "first", v.id)
}

func TestInterfaceValueIsNotMistakenForLoaded(t *testing.T) {
	t.Parallel()

	type iface interface{ ID() string }

	h := deferred.NewHolder[iface]("channel [mychannel] configuration")
	_, err := h.Get()
	require.True(t, errors.Is(err, deferred.ErrNotLoaded))

	require.NoError(t, h.Update(func(iface, bool) (iface, error) { return nil, nil }))

	v, ok := h.TryGet()
	require.True(t, ok, "an explicit nil update still counts as loaded")
	require.Nil(t, v)
}

func TestConcurrentAccessIsRaceFree(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = h.Get() }()
		go func() {
			defer wg.Done()
			_ = h.Update(func(*config, bool) (*config, error) { return &config{id: "x"}, nil })
		}()
	}
	wg.Wait()

	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, "x", v.id)
}

// TestResetReturnsToEmpty asserts that Reset discards both a held value and a
// recorded refusal, so an owner that rebuilds what it holds starts from the same
// state it was constructed in.
func TestResetReturnsToEmpty(t *testing.T) {
	t.Parallel()

	t.Run("after a value was held", func(t *testing.T) {
		t.Parallel()
		h := deferred.NewHolder[*config]("channel [mychannel] configuration")
		require.NoError(t, h.Update(func(*config, bool) (*config, error) {
			return &config{id: "first"}, nil
		}))

		h.Reset()

		v, err := h.Get()
		require.Nil(t, v)
		require.True(t, errors.Is(err, deferred.ErrNotLoaded))
		_, ok := h.TryGet()
		require.False(t, ok)
	})

	t.Run("after a refusal was recorded", func(t *testing.T) {
		t.Parallel()
		h := deferred.NewHolder[*config]("channel [mychannel] configuration")
		require.Error(t, h.Update(func(*config, bool) (*config, error) {
			return nil, errors.New("boom")
		}))

		h.Reset()

		_, err := h.Get()
		require.True(t, errors.Is(err, deferred.ErrNotLoaded),
			"a reset holder must not keep reporting the old refusal")
		require.False(t, errors.Is(err, deferred.ErrRejected))
	})
}

func TestWaitForValueAlreadyLoaded(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	v, err := h.WaitForValue(ctx)
	require.NoError(t, err)
	require.Equal(t, "first", v.id)
}

func TestWaitForValueReleasedByUpdate(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Several waiters must all be released by a single Update.
	const waiters = 4
	var wg sync.WaitGroup
	errs := make(chan error, waiters)
	for range waiters {
		wg.Go(func() {
			v, err := h.WaitForValue(ctx)
			if err != nil {
				errs <- err
				return
			}
			if v.id != "arrived" {
				errs <- errors.Errorf("unexpected value [%s]", v.id)
			}
		})
	}

	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "arrived"}, nil
	}))

	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// TestWaitForValueTimeoutReportsNotLoaded asserts that a WaitForValue call
// whose context deadline expires before a value arrives is reported as
// ErrNotLoaded, the same sentinel Get uses for a value that has never been
// offered - not as a bare wrapped context error, which satisfies neither
// ErrNotLoaded nor ErrRejected and so is misclassified by callers that
// fail-fast on those two sentinels (see toDiscoveredPeers). The deadline
// cause must still be visible in the message so a timeout is distinguishable
// from a cancellation.
func TestWaitForValueTimeoutReportsNotLoaded(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	v, err := h.WaitForValue(ctx)
	require.Error(t, err)
	require.Nil(t, v)
	require.True(t, errors.Is(err, deferred.ErrNotLoaded),
		"a timed-out wait must report ErrNotLoaded, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the subject, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), context.DeadlineExceeded.Error()),
		"error must still show the deadline-exceeded cause, got [%v]", err)
}

// TestWaitForValueContextCancelled asserts that an explicitly cancelled
// context (as opposed to one whose deadline expired) is also reported as
// ErrNotLoaded, with the cancellation cause visible in the message.
func TestWaitForValueContextCancelled(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	v, err := h.WaitForValue(ctx)
	require.Error(t, err)
	require.Nil(t, v)
	require.True(t, errors.Is(err, deferred.ErrNotLoaded),
		"a cancelled wait must report ErrNotLoaded, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the subject, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), context.Canceled.Error()),
		"error must still show the cancellation cause, got [%v]", err)
}

func TestWaitForValueRejectedDoesNotWait(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("bad config")
	}))

	// A generous deadline: the point is that WaitForValue returns without
	// consuming it, because retrying cannot clear a rejection.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	v, err := h.WaitForValue(ctx)
	require.Error(t, err)
	require.Nil(t, v)
	require.True(t, errors.Is(err, deferred.ErrRejected),
		"error must be matchable with errors.Is, got [%v]", err)
	require.Less(t, time.Since(start), time.Second, "must not wait on a rejected holder")
}

// TestWaitForValueRejectionReleasesParkedWaiter is the regression test for a
// waiter that was already parked when the first configuration was refused. It
// used to sit out its whole budget and then report ErrNotLoaded — "still
// starting up, retry" — for a holder that had in fact been given a
// configuration and rejected it, which is the opposite conclusion. The concrete
// cost was a node whose first config block cannot be parsed answering an
// endorsement request 20s later with the wrong diagnosis.
func TestWaitForValueRejectionReleasesParkedWaiter(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	type result struct {
		err     error
		elapsed time.Duration
	}
	got := make(chan result, 1)
	go func() {
		// A budget far longer than the test's own patience, so that sitting it
		// out is unmistakably a failure rather than a slow pass.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		start := time.Now()
		_, err := h.WaitForValue(ctx)
		got <- result{err: err, elapsed: time.Since(start)}
	}()

	require.Eventually(t, h.HasWaiter, 5*time.Second, time.Millisecond,
		"the waiter never registered")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("bad config")
	}))

	select {
	case r := <-got:
		require.Error(t, r.err)
		require.True(t, errors.Is(r.err, deferred.ErrRejected),
			"a waiter released by a refusal must report ErrRejected, got [%v]", r.err)
		require.False(t, errors.Is(r.err, deferred.ErrNotLoaded),
			"a refusal must not be reported as a startup race, got [%v]", r.err)
		require.Less(t, r.elapsed, 5*time.Second,
			"the waiter must be released by the refusal, not wait out its budget")
	case <-time.After(5 * time.Second):
		t.Fatal("a refused update left the parked waiter to wait out its budget")
	}
}

// TestWaitForValueRejectionDoesNotDisturbAHeldValue asserts the other half of
// Update's rule: a refusal recorded while a value is already in force is the
// updater's business alone, so it must not release waiters with an error. There
// are none to release once a value is held, so this pins that a later waiter
// still gets the held value rather than the refusal.
func TestWaitForValueRejectionDoesNotDisturbAHeldValue(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("bad config")
	}))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	v, err := h.WaitForValue(ctx)
	require.NoError(t, err)
	require.Equal(t, "first", v.id)
}

func TestWaitForValueAfterReset(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))
	h.Reset()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	// Reset must re-arm the wait: a waiter after Reset blocks rather than
	// being released by the pre-Reset update.
	_, err := h.WaitForValue(ctx)
	require.Error(t, err)
	require.True(t, errors.Is(err, deferred.ErrNotLoaded),
		"Reset must re-arm the wait, got [%v]", err)

	// And a later update releases it again.
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "second"}, nil
	}))
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	v, err := h.WaitForValue(ctx2)
	require.NoError(t, err)
	require.Equal(t, "second", v.id)
}

func TestWaitForValueResetReleasesParkedWaiter(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	got := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := h.WaitForValue(ctx)
		got <- err
	}()

	// Reset only once the waiter has registered. Approximating that with a
	// sleep is what makes this kind of test flake: on a loaded machine the
	// Reset lands first, the waiter then takes a fresh channel nobody closes,
	// and the test fails on a schedule rather than on the behaviour.
	require.Eventually(t, h.HasWaiter, 5*time.Second, time.Millisecond,
		"the waiter never registered")
	h.Reset()

	select {
	case err := <-got:
		// Released rather than orphaned. The holder is unloaded, so it reports
		// that instead of handing back a value.
		require.Error(t, err)
		require.True(t, errors.Is(err, deferred.ErrNotLoaded),
			"a waiter released by Reset must report the unloaded holder, got [%v]", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Reset orphaned the parked waiter: it was never released")
	}
}

func TestWaitForValueParkedWaiterNotHungAfterResetThenUpdate(t *testing.T) {
	t.Parallel()

	h := deferred.NewHolder[*config]("channel [mychannel] configuration")

	returned := make(chan bool, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = h.WaitForValue(ctx)
		returned <- true
	}()

	// Reset only once the waiter has registered; see the note in
	// TestWaitForValueResetReleasesParkedWaiter. If Reset does not close the
	// channel, the waiter stays parked even after Update closes a different
	// channel, hanging until the context expires.
	require.Eventually(t, h.HasWaiter, 5*time.Second, time.Millisecond,
		"the waiter never registered")
	h.Reset()

	// Update after reset provides a new value. If the waiter is still parked on
	// the old channel, this Update will not release it.
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "arrived"}, nil
	}))

	select {
	case <-returned:
		// The waiter returned promptly. Reset properly released it rather than
		// orphaning it, so the waiter did not hang.
	case <-time.After(2 * time.Second):
		t.Fatal("waiter timed out: Reset orphaned it, so Update's close was on a different channel")
	}
}
