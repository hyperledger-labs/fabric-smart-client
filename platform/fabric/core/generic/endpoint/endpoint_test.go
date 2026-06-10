/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestNewResolver(t *testing.T) {
	t.Parallel()

	rs := &fake.ResolverService{}
	es := &fake.EndpointService{}

	r, err := NewResolver(rs, es)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Equal(t, rs, r.rs)
	require.Equal(t, es, r.es)
}

func TestResolverGetIdentity_LocalResolverHit(t *testing.T) {
	t.Parallel()

	rs := &fake.ResolverService{ID: view.Identity("alice-id")}
	es := &fake.EndpointService{}
	r, err := NewResolver(rs, es)
	require.NoError(t, err)

	id, err := r.GetIdentity("alice")
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice-id"), id)
	require.Equal(t, "alice", rs.LastLabel)
	require.Equal(t, 0, es.CallCount)
}

func TestResolverGetIdentity_FallbackToEndpointService(t *testing.T) {
	t.Parallel()

	rs := &fake.ResolverService{ID: nil}
	es := &fake.EndpointService{ID: view.Identity("alice-from-endpoint")}
	r, err := NewResolver(rs, es)
	require.NoError(t, err)

	id, err := r.GetIdentity("alice")
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice-from-endpoint"), id)
	require.Equal(t, "alice", es.LastLabel)
	require.Equal(t, 1, es.CallCount)
	require.Nil(t, es.LastPKIID)
}

func TestResolverGetIdentity_FallbackError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("endpoint lookup failed")
	rs := &fake.ResolverService{ID: nil}
	es := &fake.EndpointService{Err: expectedErr}
	r, err := NewResolver(rs, es)
	require.NoError(t, err)

	id, err := r.GetIdentity("alice")
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, id)
	require.Equal(t, 1, es.CallCount)
}
