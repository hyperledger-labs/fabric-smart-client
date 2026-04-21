/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEndpoint(t *testing.T) {
	t.Parallel()

	t.Run("formatted broadcast endpoint", func(t *testing.T) {
		t.Parallel()
		ep, err := parseEndpoint("id=0,broadcast,orderer.example.com:7050")
		require.NoError(t, err)
		require.Equal(t, &endpoint{ID: 0, Type: OrdererBroadcastType, Endpoint: "orderer.example.com:7050"}, ep)
	})

	t.Run("formatted deliver endpoint", func(t *testing.T) {
		t.Parallel()
		ep, err := parseEndpoint("id=1,deliver,orderer.example.com:7051")
		require.NoError(t, err)
		require.Equal(t, &endpoint{ID: 1, Type: OrdererDeliverType, Endpoint: "orderer.example.com:7051"}, ep)
	})

	t.Run("multi-digit id parses", func(t *testing.T) {
		t.Parallel()
		ep, err := parseEndpoint("id=42,broadcast,orderer:7050")
		require.NoError(t, err)
		require.Equal(t, 42, ep.ID)
		require.Equal(t, OrdererBroadcastType, ep.Type)
		require.Equal(t, "orderer:7050", ep.Endpoint)
	})

	t.Run("endpoint with extra colons is captured fully", func(t *testing.T) {
		t.Parallel()
		// The regex uses `.*` for the endpoint group, so IPv6-style
		// literals with embedded colons round-trip unchanged.
		ep, err := parseEndpoint("id=0,broadcast,[::1]:7050")
		require.NoError(t, err)
		require.Equal(t, "[::1]:7050", ep.Endpoint)
	})

	t.Run("plain host:port falls back to broadcast with id=0", func(t *testing.T) {
		t.Parallel()
		ep, err := parseEndpoint("orderer:7050")
		require.NoError(t, err)
		require.Equal(t, &endpoint{ID: 0, Type: OrdererBroadcastType, Endpoint: "orderer:7050"}, ep)
	})

	t.Run("empty string returns ErrInvalidEndpointFormat", func(t *testing.T) {
		t.Parallel()
		ep, err := parseEndpoint("")
		require.Nil(t, ep)
		require.ErrorIs(t, err, ErrInvalidEndpointFormat)
	})

	t.Run("id overflowing int returns wrapped parse error", func(t *testing.T) {
		t.Parallel()
		ep, err := parseEndpoint("id=99999999999999999999999999,broadcast,orderer:7050")
		require.Nil(t, ep)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid endpoint id")
	})
}
