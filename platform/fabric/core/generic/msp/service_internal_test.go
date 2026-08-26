/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package msp

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
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
	defaultMSP  string
	msps        []config.MSP
}

func (c *testConfig) NetworkName() string           { return c.networkName }
func (c *testConfig) DefaultMSP() string            { return c.defaultMSP }
func (c *testConfig) MSPs() ([]config.MSP, error)   { return c.msps, nil }
func (c *testConfig) TranslatePath(p string) string { return p }

// loaderFunc turns a function into a driver.IdentityLoader.
type loaderFunc func(manager driver.Manager, c config.MSP) error

func (f loaderFunc) Load(manager driver.Manager, c config.MSP) error { return f(manager, c) }

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
	require.NoError(t, s.defaultMSP.Update(func(string, bool) (string, error) {
		return defaultMSP, nil
	}))
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

// TestSetDefaultIdentityRefusesMissingSigner asserts that an identity without
// the signing identity that goes with it is refused. Installing the pair
// half-built would pass every startup check and then fail at the first
// signature, in whatever component called DefaultSigningIdentity.
func TestSetDefaultIdentityRefusesMissingSigner(t *testing.T) {
	t.Parallel()
	s := newInternalTestService(t, "SampleOrg")

	s.SetDefaultIdentity("SampleOrg", view.Identity("me"), nil)

	_, err := s.defaults.Get()
	require.ErrorIs(t, err, fdriver.ErrConfigRejected)
	require.ErrorContains(t, err, "supplied no signing identity")
	require.Nil(t, s.DefaultIdentity())
	require.Nil(t, s.DefaultSigningIdentity())
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
			// assert, not require: require calls t.FailNow, which is only valid
			// on the test's own goroutine and would Goexit this one instead of
			// failing the test.
			if d, loaded := s.defaults.TryGet(); loaded {
				assert.Equal(t, view.Identity("me"), d.id)
				assert.Same(t, signing, d.signing)
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

// TestSetDefaultIdentityBeforeDefaultMSPIsResolved asserts that a caller that
// arrives before loadLocalMSPs has resolved the default MSP installs nothing.
// An empty identifier must not compare equal to an unresolved one: MSP
// identifiers are nowhere validated as non-empty.
func TestSetDefaultIdentityBeforeDefaultMSPIsResolved(t *testing.T) {
	t.Parallel()
	s := NewLocalMSPManager(&testConfig{networkName: "testnet"}, nil, nil, nil, nil, nil, 0)

	s.SetDefaultIdentity("", view.Identity("me"), &fakeSigningIdentity{})

	_, err := s.defaults.Get()
	require.ErrorIs(t, err, fdriver.ErrNotInitialized,
		"an empty id must not match an unresolved default MSP")
	require.Nil(t, s.DefaultIdentity())
}

// TestSetDefaultIdentityIsSafeWithoutHoldingMspsMutex pins what the holder buys
// over a field guarded by mspsMutex. SetDefaultIdentity is reachable through the
// exported driver.Manager interface, and identity loaders are a
// dependency-injection extension point, so a loader outside this repository can
// retain a Manager and call it from a goroutine holding none of our locks -
// concurrently with a Refresh that is re-resolving the default MSP.
func TestSetDefaultIdentityIsSafeWithoutHoldingMspsMutex(t *testing.T) {
	t.Parallel()
	s := newInternalTestService(t, "SampleOrg")

	var wg sync.WaitGroup
	wg.Go(func() {
		// Stands in for loadLocalMSPs re-resolving the default MSP.
		_ = s.defaultMSP.Update(func(string, bool) (string, error) { return "SampleOrg", nil })
	})
	wg.Go(func() { s.SetDefaultIdentity("SampleOrg", view.Identity("me"), &fakeSigningIdentity{}) })
	wg.Go(func() { _ = s.DefaultIdentity() })
	wg.Wait()
}

type fakeSigningIdentity struct {
	driver.SigningIdentity
}

// TestRefreshWithoutAUsableDefaultIdentityFails asserts that a reload which
// cannot re-establish a default identity is reported as a failure, rather than
// leaving the service claiming success while it serves the identity of an MSP
// the reload just removed.
//
// Refresh clears everything derived from the configured MSPs, and the default
// identity is derived from them, so it has to be cleared too. Leaving it in
// place would also hide the refusal: the holder only records why an update was
// refused while nothing has been accepted.
func TestRefreshWithoutAUsableDefaultIdentityFails(t *testing.T) {
	t.Parallel()

	identity := view.Identity("me")
	cfg := &testConfig{
		networkName: "testnet",
		defaultMSP:  "SampleOrg",
		msps:        []config.MSP{{ID: "SampleOrg", MSPType: "test"}},
	}
	s := NewLocalMSPManager(cfg, nil, nil, nil, nil, nil, 0)
	s.PutIdentityLoader("test", loaderFunc(func(m driver.Manager, c config.MSP) error {
		m.SetDefaultIdentity(c.ID, identity, &fakeSigningIdentity{})
		return nil
	}))

	require.NoError(t, s.Load())
	require.Equal(t, identity, s.DefaultIdentity())

	// The MSP is still configured, but this time it has no usable identity.
	s.PutIdentityLoader("test", loaderFunc(func(m driver.Manager, c config.MSP) error {
		m.SetDefaultIdentity(c.ID, nil, nil)
		return nil
	}))

	require.Error(t, s.Refresh(), "a reload that cannot establish a default identity must fail")
	require.Nil(t, s.DefaultIdentity(), "the stale identity must not survive the reload")
}
