/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// stubEndpointService returns whatever it is configured with, and records the label it was
// asked for.
type stubEndpointService struct {
	identity view.Identity
	err      error

	asked []string
}

func (s *stubEndpointService) GetIdentity(label string) (view.Identity, error) {
	s.asked = append(s.asked, label)

	return s.identity, s.err
}

// TestNewProvider checks the provider is built over the endpoint service it is given.
func TestNewProvider(t *testing.T) {
	t.Parallel()

	p, err := NewProvider(&stubEndpointService{})

	require.NoError(t, err)
	require.NotNil(t, p)
}

// TestIdentity checks the identity is resolved through the endpoint service under the label
// it was asked for.
func TestIdentity(t *testing.T) {
	t.Parallel()

	want := view.Identity("some-identity")
	endpoints := &stubEndpointService{identity: want}

	p, err := NewProvider(endpoints)
	require.NoError(t, err)

	got, err := p.Identity("alice")

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, []string{"alice"}, endpoints.asked)
}

// TestIdentityEndpointServiceFails checks a lookup failure is reported with the label that
// could not be resolved, so the caller can tell which one it was.
func TestIdentityEndpointServiceFails(t *testing.T) {
	t.Parallel()

	p, err := NewProvider(&stubEndpointService{err: errors.New("no such endpoint")})
	require.NoError(t, err)

	got, err := p.Identity("bob")

	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "bob", "the error names the label that failed")
	assert.Contains(t, err.Error(), "no such endpoint", "the underlying error is preserved")
}
