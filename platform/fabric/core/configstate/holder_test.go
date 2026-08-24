/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configstate_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/configstate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

type config struct{ id string }

func TestGetBeforeFirstUpdate(t *testing.T) {
	t.Parallel()

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")

	v, err := h.Get()
	require.Error(t, err)
	require.Nil(t, v)
	require.True(t, errors.Is(err, driver.ErrNotInitialized),
		"error must be matchable with errors.Is, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the channel, got [%v]", err)
}

func TestGetAfterUpdate(t *testing.T) {
	t.Parallel()

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")
	require.NoError(t, h.Update(func(*config, bool) (*config, error) {
		return &config{id: "first"}, nil
	}))

	v, err := h.Get()
	require.NoError(t, err)
	require.Equal(t, "first", v.id)
}

func TestFailedUpdateLeavesPreviousValue(t *testing.T) {
	t.Parallel()

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")
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

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")
	require.Error(t, h.Update(func(*config, bool) (*config, error) {
		return nil, errors.New("boom")
	}))

	_, err := h.Get()
	require.True(t, errors.Is(err, driver.ErrNotInitialized))
}

func TestUpdateSeesCurrentValue(t *testing.T) {
	t.Parallel()

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")

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

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")

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

	h := configstate.NewHolder[iface]("channel [mychannel] configuration")
	_, err := h.Get()
	require.True(t, errors.Is(err, driver.ErrNotInitialized))

	require.NoError(t, h.Update(func(iface, bool) (iface, error) { return nil, nil }))

	v, ok := h.TryGet()
	require.True(t, ok, "an explicit nil update still counts as loaded")
	require.Nil(t, v)
}

func TestConcurrentAccessIsRaceFree(t *testing.T) {
	t.Parallel()

	h := configstate.NewHolder[*config]("channel [mychannel] configuration")

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
