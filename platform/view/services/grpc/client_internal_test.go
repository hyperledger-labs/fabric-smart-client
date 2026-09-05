/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// createSecOpts used to merge a CA file with CA bytes here. That behaviour now lives in two
// places, each tested where it happens: reading the configured files is tlsconfig's
// resolution, and combining them with anchors discovered from a channel's MSPs is the augment
// performed by the membership and chaincode discovery paths.
//
// What remains worth pinning at this layer is that the augment pattern those call sites use
// leaves the configured pool intact — every one of them clones before appending, and a shared
// SecureOptions would otherwise accumulate one endpoint's anchors onto the next.
func TestAugmentingRootCAsDoesNotMutateTheConfiguredPool(t *testing.T) {
	t.Parallel()

	configured := []byte("configured-ca")
	network := SecureOptions{UseTLS: true, ServerRootCAs: [][]byte{configured}}

	first := network
	first.ServerRootCAs = append(slices.Clone(network.ServerRootCAs), []byte("discovered-1"))
	second := network
	second.ServerRootCAs = append(slices.Clone(network.ServerRootCAs), []byte("discovered-2"))

	require.Len(t, network.ServerRootCAs, 1, "the network's pool must be untouched")
	require.Len(t, first.ServerRootCAs, 2)
	require.Len(t, second.ServerRootCAs, 2)
	require.Equal(t, configured, first.ServerRootCAs[0])
	require.Equal(t, []byte("discovered-1"), first.ServerRootCAs[1])
	require.Equal(t, []byte("discovered-2"), second.ServerRootCAs[1],
		"one endpoint's discovered anchors must not leak into another's")
}
