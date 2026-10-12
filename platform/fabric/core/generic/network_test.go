/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

// The fakes embed their driver interface: Network only reaches a few methods, and an
// unimplemented one nil-panics rather than silently returning a zero value.

type stubConfigService struct {
	driver.ConfigService
}

func (stubConfigService) DriverName() string         { return "generic" }
func (stubConfigService) IsChannelQuiet(string) bool { return false }

type stubChannel struct {
	driver.Channel

	closeErr error
	closed   bool
}

func (c *stubChannel) Close() error {
	c.closed = true

	return c.closeErr
}

// newTestNetwork builds a Network whose NewChannel hands out the given channels and records
// every name it is asked to allocate.
func newTestNetwork(t *testing.T, channels map[string]*stubChannel) (*Network, *[]string) {
	t.Helper()

	var allocated []string
	n, err := NewNetwork("net", stubConfigService{}, nil, nil, nil, nil,
		func(_ driver.FabricNetworkService, name string, _ bool) (driver.Channel, error) {
			allocated = append(allocated, name)

			return channels[name], nil
		})
	require.NoError(t, err)

	return n, &allocated
}

// TestNetworkClose_OnlyOpenedChannels checks Close closes the opened channels without
// allocating the ones never opened.
func TestNetworkClose_OnlyOpenedChannels(t *testing.T) {
	t.Parallel()

	opened, unopened := &stubChannel{}, &stubChannel{}
	n, allocated := newTestNetwork(t, map[string]*stubChannel{"a": opened, "b": unopened})
	_, err := n.Channel("a")
	require.NoError(t, err)

	require.NoError(t, n.Close())

	assert.True(t, opened.closed)
	assert.False(t, unopened.closed)
	assert.Equal(t, []string{"a"}, *allocated)
}

// TestNetworkClose_ChannelCloseFails checks a channel that fails to close does not prevent
// closing the others and is reported.
func TestNetworkClose_ChannelCloseFails(t *testing.T) {
	t.Parallel()

	failing := &stubChannel{closeErr: errors.New("close exploded")}
	healthy := &stubChannel{}
	n, _ := newTestNetwork(t, map[string]*stubChannel{"a": failing, "b": healthy})
	for _, name := range []string{"a", "b"} {
		_, err := n.Channel(name)
		require.NoError(t, err)
	}

	require.ErrorContains(t, n.Close(), "close exploded")
	assert.True(t, failing.closed)
	assert.True(t, healthy.closed)
}
