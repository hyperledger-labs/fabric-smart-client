/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package grpc

import (
	"context"
	"net"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
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

// scriptedCreds fails the client handshake while fail is set.
type scriptedCreds struct {
	credentials.TransportCredentials
	fail bool
}

func (c *scriptedCreds) ClientHandshake(_ context.Context, _ string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	if c.fail {
		return nil, nil, errors.New("handshake failed")
	}
	return conn, nil, nil
}

// TestHandshakeRecorderKeepsOnlyTheLastHandshake checks that a successful handshake clears
// the error of an earlier failed one, so a later failure to dial is not reported as the old
// handshake error.
func TestHandshakeRecorderKeepsOnlyTheLastHandshake(t *testing.T) {
	t.Parallel()
	creds := &scriptedCreds{TransportCredentials: insecure.NewCredentials(), fail: true}
	r := &handshakeRecorder{TransportCredentials: creds, last: &atomic.Pointer[error]{}}
	require.NoError(t, r.lastErr(), "no handshake ran yet")

	_, _, err := r.ClientHandshake(t.Context(), "peer", nil)
	require.Error(t, err)
	require.EqualError(t, r.lastErr(), "handshake failed")

	creds.fail = false
	_, _, err = r.ClientHandshake(t.Context(), "peer", nil)
	require.NoError(t, err)
	require.NoError(t, r.lastErr(), "a successful handshake clears the recorded error")
}
