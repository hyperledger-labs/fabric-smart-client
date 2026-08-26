/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package deferred_test

import (
	"strings"
	"sync"
	"testing"

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
