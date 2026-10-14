/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestResolversRacesWithPkiResolve copies a resolver's PKI while PkiResolve writes it.
// Run with -race.
func TestResolversRacesWithPkiResolve(t *testing.T) {
	t.Parallel()
	service, err := endpoint.NewService(&mock.BindingStore{})
	require.NoError(t, err)

	// no extractor yet, so AddResolver leaves the PKI empty and Resolve computes it later
	aliceID := view.Identity("alice_id")
	_, err = service.AddResolver("alice", "org1", map[string]string{string(endpoint.P2PPort): "localhost:1010"}, nil, aliceID)
	require.NoError(t, err)
	extractor := &mock.PublicKeyExtractor{}
	extractor.ExtractPublicKeyReturns([]byte("alice_pk"), nil)
	require.NoError(t, service.AddPublicKeyExtractor(extractor))

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _, _ = service.Resolve(t.Context(), aliceID)
	}()
	for range 100 {
		_ = service.Resolvers()
	}
	<-done
}
