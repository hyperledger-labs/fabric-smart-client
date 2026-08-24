/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/msp/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// These tests reach into the service to check the three states the exported
// accessors cannot tell apart, since DefaultIdentity returns nil for all of
// them: no loader has run yet, a loader offered an identity that was refused,
// and an identity is held. See service_test.go for the exported behaviour.

// testConfig is the minimum driver.Config the service constructor touches.
type testConfig struct {
	driver.Config
	networkName string
}

func (c *testConfig) NetworkName() string { return c.networkName }

func newInternalTestService(t *testing.T, defaultMSP string) *service {
	t.Helper()
	s := NewLocalMSPManager(
		&testConfig{networkName: "testnet"},
		nil, // KVS
		nil, // signerService
		nil, // binderService
		nil, // defaultViewIdentity
		nil, // deserializerManager
		0,   // cacheSize
	)
	s.defaultMSP = defaultMSP
	return s
}

// TestDefaultsBeforeAnyLoader asserts that a service whose identity loaders have
// not run yet reports the absence as a startup state, not as a refusal.
func TestDefaultsBeforeAnyLoader(t *testing.T) {
	t.Parallel()
	s := newInternalTestService(t, "SampleOrg")

	_, err := s.defaults.Get()
	require.ErrorIs(t, err, fdriver.ErrNotInitialized)

	// The exported accessors keep their nil-returning contract.
	require.Nil(t, s.DefaultIdentity())
	require.Nil(t, s.DefaultSigningIdentity())
}

// TestSetDefaultIdentityRefusesEmptyIdentity asserts that an empty identity from
// the default MSP is refused rather than installed, and that the holder reports
// it as a refusal so loadLocalMSPs can explain the failure instead of inviting a
// retry that cannot help.
func TestSetDefaultIdentityRefusesEmptyIdentity(t *testing.T) {
	t.Parallel()

	for name, empty := range map[string]view.Identity{"nil": nil, "zero-length": {}} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newInternalTestService(t, "SampleOrg")

			s.SetDefaultIdentity("SampleOrg", empty, &fakeSigningIdentity{})

			_, err := s.defaults.Get()
			require.ErrorIs(t, err, fdriver.ErrConfigRejected,
				"an empty identity must be refused, not installed")
			require.ErrorContains(t, err, "SampleOrg")
			require.Nil(t, s.DefaultIdentity())
			require.Nil(t, s.DefaultSigningIdentity())
		})
	}
}

// TestSetDefaultIdentityKeepsExistingOnEmpty asserts that a later empty identity
// does not clear a default that was already accepted.
func TestSetDefaultIdentityKeepsExistingOnEmpty(t *testing.T) {
	t.Parallel()
	s := newInternalTestService(t, "SampleOrg")
	signing := &fakeSigningIdentity{}

	s.SetDefaultIdentity("SampleOrg", view.Identity("me"), signing)
	s.SetDefaultIdentity("SampleOrg", nil, nil)

	require.Equal(t, view.Identity("me"), s.DefaultIdentity())
	require.Same(t, signing, s.DefaultSigningIdentity())
}

// TestSetDefaultIdentityIgnoresOtherMSPs asserts that an identity from a
// non-default MSP is neither installed nor recorded as a refusal: it is not this
// MSP's business to nominate the default, so the holder stays untouched.
func TestSetDefaultIdentityIgnoresOtherMSPs(t *testing.T) {
	t.Parallel()
	s := newInternalTestService(t, "SampleOrg")

	s.SetDefaultIdentity("OtherOrg", nil, nil)

	_, err := s.defaults.Get()
	require.ErrorIs(t, err, fdriver.ErrNotInitialized,
		"a non-default MSP must not leave the holder in the refused state")
}

// TestDefaultIdentityHalvesAreInstalledTogether asserts that a reader racing an
// install never observes the identity without its signing identity. It relies on
// the race detector for the concurrency; the assertion covers the pairing.
func TestDefaultIdentityHalvesAreInstalledTogether(t *testing.T) {
	t.Parallel()
	s := newInternalTestService(t, "SampleOrg")
	signing := &fakeSigningIdentity{}

	var wg sync.WaitGroup
	wg.Go(func() { s.SetDefaultIdentity("SampleOrg", view.Identity("me"), signing) })
	for range 4 {
		wg.Go(func() {
			// Either both halves are present or neither is, never one alone.
			if d, loaded := s.defaults.TryGet(); loaded {
				require.Equal(t, view.Identity("me"), d.id)
				require.Same(t, signing, d.signing)
			}
		})
	}
	wg.Wait()

	require.Equal(t, view.Identity("me"), s.DefaultIdentity())
	require.Same(t, signing, s.DefaultSigningIdentity())
}

// TestLoadLocalMSPsErrorIsNotRetryable asserts that a startup failure for want of
// a default identity is reported as permanent. driver.ErrNotInitialized means
// "still starting up, retry", and no retry will produce a default that the
// loaders did not supply.
func TestLoadLocalMSPsErrorIsNotRetryable(t *testing.T) {
	t.Parallel()

	t.Run("never offered", func(t *testing.T) {
		t.Parallel()
		s := newInternalTestService(t, "SampleOrg")

		err := s.defaultIdentityError()
		require.ErrorContains(t, err, "no default identity set for network [testnet]")
		require.False(t, errors.Is(err, fdriver.ErrNotInitialized),
			"a permanent misconfiguration must not look like a startup race")
	})

	t.Run("refused", func(t *testing.T) {
		t.Parallel()
		s := newInternalTestService(t, "SampleOrg")
		s.SetDefaultIdentity("SampleOrg", nil, nil)

		err := s.defaultIdentityError()
		require.ErrorContains(t, err, "no usable default identity for network [testnet]")
		require.ErrorContains(t, err, "supplied an empty identity", "the refusal reason must survive")
		require.False(t, errors.Is(err, fdriver.ErrNotInitialized))
	})

	t.Run("installed", func(t *testing.T) {
		t.Parallel()
		s := newInternalTestService(t, "SampleOrg")
		s.SetDefaultIdentity("SampleOrg", view.Identity("me"), &fakeSigningIdentity{})

		require.NoError(t, s.defaultIdentityError())
	})
}

type fakeSigningIdentity struct {
	driver.SigningIdentity
}
