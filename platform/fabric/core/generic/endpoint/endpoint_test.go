/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type stubResolverService struct {
	id        view.Identity
	lastLabel string
}

func (s *stubResolverService) GetIdentity(label string) view.Identity {
	s.lastLabel = label
	return s.id
}

type stubEndpointService struct {
	id        view.Identity
	err       error
	callCount int
	lastLabel string
	lastPKIID []byte
}

func (s *stubEndpointService) GetIdentity(label string, pkiID []byte) (view.Identity, error) {
	s.callCount++
	s.lastLabel = label
	s.lastPKIID = pkiID
	return s.id, s.err
}

func TestNewResolver(t *testing.T) {
	t.Parallel()

	rs := &stubResolverService{}
	es := &stubEndpointService{}

	r, err := NewResolver(rs, es)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Equal(t, rs, r.rs)
	require.Equal(t, es, r.es)
}

func TestResolverGetIdentity_LocalResolverHit(t *testing.T) {
	t.Parallel()

	rs := &stubResolverService{id: view.Identity("alice-id")}
	es := &stubEndpointService{}
	r, err := NewResolver(rs, es)
	require.NoError(t, err)

	id, err := r.GetIdentity("alice")
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice-id"), id)
	require.Equal(t, "alice", rs.lastLabel)
	require.Equal(t, 0, es.callCount)
}

func TestResolverGetIdentity_FallbackToEndpointService(t *testing.T) {
	t.Parallel()

	rs := &stubResolverService{id: nil}
	es := &stubEndpointService{id: view.Identity("alice-from-endpoint")}
	r, err := NewResolver(rs, es)
	require.NoError(t, err)

	id, err := r.GetIdentity("alice")
	require.NoError(t, err)
	require.Equal(t, view.Identity("alice-from-endpoint"), id)
	require.Equal(t, "alice", es.lastLabel)
	require.Equal(t, 1, es.callCount)
	require.Nil(t, es.lastPKIID)
}

func TestResolverGetIdentity_FallbackError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("endpoint lookup failed")
	rs := &stubResolverService{id: nil}
	es := &stubEndpointService{err: expectedErr}
	r, err := NewResolver(rs, es)
	require.NoError(t, err)

	id, err := r.GetIdentity("alice")
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, id)
	require.Equal(t, 1, es.callCount)
}
