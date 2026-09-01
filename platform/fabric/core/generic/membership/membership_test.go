/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"strings"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var _ driver.MembershipService = (*Service)(nil)

// seed installs cfg as the service's channel configuration at the given
// sequence, standing in for the first successful Update.
func seed(t *testing.T, s *Service, cfg *channelconfig.ChannelConfig, sequence uint64) {
	t.Helper()
	require.NoError(t, s.config.Update(func(*configuration, bool) (*configuration, error) {
		return &configuration{channelConfig: cfg, sequence: sequence}, nil
	}))
}

// TestAccessorsBeforeFirstUpdate is the regression test for the nil dereference
// that crashed nodes when a channel was used before its first configuration
// block arrived. Every accessor must report the condition instead of panicking.
func TestAccessorsBeforeFirstUpdate(t *testing.T) {
	t.Parallel()

	identity := view.Identity("some-identity")

	for _, tc := range []struct {
		name string
		call func(s *Service) error
	}{
		{"IsValid", func(s *Service) error {
			return s.IsValid(identity)
		}},
		{"GetVerifier", func(s *Service) error {
			v, err := s.GetVerifier(identity)
			require.Nil(t, v)
			return err
		}},
		{"GetMSPIDs", func(s *Service) error {
			ids, err := s.GetMSPIDs()
			require.Nil(t, ids)
			return err
		}},
		{"OrdererConfig", func(s *Service) error {
			ct, eps, err := s.OrdererConfig(nil)
			require.Empty(t, ct)
			require.Nil(t, eps)
			return err
		}},
		{"IsIdemixMSP", func(s *Service) error {
			isIdemix, err := s.IsIdemixMSP("Org1MSP")
			require.False(t, isIdemix)
			return err
		}},
		{"MSPManager.DeserializeIdentity", func(s *Service) error {
			id, err := s.MSPManager().DeserializeIdentity(identity)
			require.Nil(t, id)
			return err
		}},
		{"ConfigSequence", func(s *Service) error {
			seq, err := s.ConfigSequence()
			require.Zero(t, seq)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := NewService("mychannel")

			var err error
			require.NotPanics(t, func() { err = tc.call(s) })

			require.Error(t, err)
			require.ErrorIs(t, err, driver.ErrNotInitialized)
			require.True(t, strings.Contains(err.Error(), "mychannel"),
				"error should name the channel, got [%v]", err)
		})
	}
}

// TestMSPManagerIsUsableBeforeFirstUpdate pins the lazy contract: obtaining a
// manager before the configuration exists is allowed, and the failure surfaces
// when it is used rather than when it is fetched.
func TestMSPManagerIsUsableBeforeFirstUpdate(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	var mgr driver.MSPManager
	require.NotPanics(t, func() { mgr = s.MSPManager() })
	require.NotNil(t, mgr)

	_, err := mgr.DeserializeIdentity(view.Identity("some-identity"))
	require.ErrorIs(t, err, driver.ErrNotInitialized)
}

func TestUpdateWithInvalidEnvelopeLeavesServiceWithoutConfig(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	require.Error(t, s.Update(&cb.Envelope{Payload: []byte("not-a-proto")}))

	_, err := s.GetMSPIDs()
	require.ErrorIs(t, err, driver.ErrConfigRejected,
		"a config block the service refused must be reported as a refusal")
	require.NotErrorIs(t, err, driver.ErrNotInitialized,
		"a refusal must not be reported as a startup race the caller can retry out of")
	require.ErrorContains(t, err, "cannot get payload from config transaction",
		"the reason the block was refused must survive")
}

// TestServiceRecoversFromARejectedConfig asserts that a service that refused one
// config block still accepts the next one: a refusal is not a terminal state.
func TestServiceRecoversFromARejectedConfig(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	require.Error(t, s.Update(&cb.Envelope{Payload: []byte("not-a-proto")}))
	seed(t, s, &channelconfig.ChannelConfig{}, 0)

	_, err := s.GetMSPIDs()
	require.NoError(t, err, "an accepted configuration must clear an earlier refusal")
}

// TestLoadedConfigIsDistinguishableFromMissingConfig is the point of the
// refactoring: an empty answer from a loaded configuration and an absent
// configuration are no longer the same result.
func TestLoadedConfigIsDistinguishableFromMissingConfig(t *testing.T) {
	t.Parallel()

	t.Run("GetMSPIDs on a config with no application section", func(t *testing.T) {
		t.Parallel()
		s := NewService("mychannel")
		seed(t, s, &channelconfig.ChannelConfig{}, 0)

		ids, err := s.GetMSPIDs()
		require.NoError(t, err, "a loaded configuration must not report ErrNotInitialized")
		require.Empty(t, ids)
	})

	t.Run("IsIdemixMSP on a config with no application section", func(t *testing.T) {
		t.Parallel()
		s := NewService("mychannel")
		seed(t, s, &channelconfig.ChannelConfig{}, 0)

		isIdemix, err := s.IsIdemixMSP("Org1MSP")
		require.NoError(t, err, "a loaded configuration must not report ErrNotInitialized")
		require.False(t, isIdemix)
	})

	t.Run("OrdererConfig on a config with no orderer section", func(t *testing.T) {
		t.Parallel()
		s := NewService("mychannel")
		seed(t, s, &channelconfig.ChannelConfig{}, 0)

		_, _, err := s.OrdererConfig(nil)
		require.Error(t, err)
		require.False(t, errors.Is(err, driver.ErrNotInitialized),
			"a missing orderer section is a configuration problem, not a startup race")
		require.Contains(t, err.Error(), "mychannel")
	})
}

func TestCheckACLIsNotImplemented(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, NewService("mychannel").CheckACL(nil), driver.ErrNotImplemented)
}

// TestConfigSequenceReportsTheSequenceInForce asserts that the sequence a
// reader sees is the one belonging to the configuration currently held, which
// is what lets a caller tell whether a configuration update has reached this
// node yet.
func TestConfigSequenceReportsTheSequenceInForce(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	seed(t, s, &channelconfig.ChannelConfig{}, 0)
	seq, err := s.ConfigSequence()
	require.NoError(t, err)
	require.Equal(t, uint64(0), seq)

	seed(t, s, &channelconfig.ChannelConfig{}, 7)
	seq, err = s.ConfigSequence()
	require.NoError(t, err)
	require.Equal(t, uint64(7), seq)
}

// TestConfigSequenceSurvivesARejectedUpdate asserts that a refused update
// leaves both halves of the held value alone: reporting the new sequence
// beside the old configuration would tell a caller the node had applied a
// configuration it had in fact rejected.
func TestConfigSequenceSurvivesARejectedUpdate(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")
	seed(t, s, &channelconfig.ChannelConfig{}, 3)

	require.Error(t, s.Update(&cb.Envelope{Payload: []byte("not-a-proto")}))

	seq, err := s.ConfigSequence()
	require.NoError(t, err)
	require.Equal(t, uint64(3), seq, "a rejected update must not advance the sequence")
}

func TestTLSRootCertsByMSPIDNotInitialized(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")

	certs, err := s.TLSRootCertsByMSPID("Org1MSP")
	require.Error(t, err)
	require.Nil(t, certs)
	require.True(t, errors.Is(err, driver.ErrNotInitialized),
		"error must be matchable with errors.Is, got [%v]", err)
}

// TestTLSRootCertsByMSPIDWithoutApplicationConfig asserts that a configuration
// carrying no application section is reported as such, rather than being
// mistaken for an organization that has no certificates configured.
func TestTLSRootCertsByMSPIDWithoutApplicationConfig(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")
	seed(t, s, &channelconfig.ChannelConfig{}, 0)

	certs, err := s.TLSRootCertsByMSPID("Org1MSP")
	require.Error(t, err)
	require.Nil(t, certs)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the channel, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "application config does not exist"),
		"error must be specific to the missing application section, got [%v]", err)
}

// fakeApplicationOrg is a minimal channelconfig.ApplicationOrg double that a
// test can build directly, since the concrete
// *channelconfig.ApplicationOrgConfig cannot be constructed with organizations
// from outside the channelconfig package.
type fakeApplicationOrg struct {
	name  string
	mspID string
	msp   msp.MSP
}

func (f *fakeApplicationOrg) Name() string                  { return f.name }
func (f *fakeApplicationOrg) MSPID() string                 { return f.mspID }
func (f *fakeApplicationOrg) MSP() msp.MSP                  { return f.msp }
func (f *fakeApplicationOrg) AnchorPeers() []*pb.AnchorPeer { return nil }

// fakeApplication is a minimal channelconfig.Application double.
type fakeApplication struct {
	orgs map[string]channelconfig.ApplicationOrg
}

func (f *fakeApplication) Organizations() map[string]channelconfig.ApplicationOrg { return f.orgs }

// TestTLSRootCertsByMSPIDReturnsRootsThenIntermediates asserts that, for an
// application organization with a matching MSP ID, the result concatenates
// the TLS root certificates followed by the TLS intermediate certificates, in
// that order — the ordering that Task 3's TLS dial config depends on.
func TestTLSRootCertsByMSPIDReturnsRootsThenIntermediates(t *testing.T) {
	t.Parallel()

	m := &fake.MSP{}
	m.On("GetTLSRootCerts").Return([][]byte{[]byte("root")})
	m.On("GetTLSIntermediateCerts").Return([][]byte{[]byte("inter")})

	ac := &fakeApplication{orgs: map[string]channelconfig.ApplicationOrg{
		"Org1": &fakeApplicationOrg{name: "Org1", mspID: "Org1MSP", msp: m},
	}}

	certs, err := tlsRootCertsByMSPID(ac, "Org1MSP", "mychannel")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("root"), []byte("inter")}, certs,
		"roots must come first, intermediates second")
}

// TestTLSRootCertsByMSPIDSelectsByMSPIDNotOrgName asserts that the lookup
// matches organizations by their MSP ID, not by the map key under which
// Organizations() stores them (which is the org name) — Organizations()
// returns map[string]ApplicationOrg keyed by org name, so indexing by MSP ID
// would silently miss.
func TestTLSRootCertsByMSPIDSelectsByMSPIDNotOrgName(t *testing.T) {
	t.Parallel()

	m1 := &fake.MSP{}
	m1.On("GetTLSRootCerts").Return([][]byte{[]byte("org1-root")})
	m1.On("GetTLSIntermediateCerts").Return([][]byte{})

	m2 := &fake.MSP{}
	m2.On("GetTLSRootCerts").Return([][]byte{[]byte("org2-root")})
	m2.On("GetTLSIntermediateCerts").Return([][]byte{})

	ac := &fakeApplication{orgs: map[string]channelconfig.ApplicationOrg{
		"Org1": &fakeApplicationOrg{name: "Org1", mspID: "Org1MSP", msp: m1},
		"Org2": &fakeApplicationOrg{name: "Org2", mspID: "Org2MSP", msp: m2},
	}}

	certs, err := tlsRootCertsByMSPID(ac, "Org2MSP", "mychannel")
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("org2-root")}, certs,
		"must select the organization by MSP ID, not by the map key it is stored under")
}

// TestTLSRootCertsByMSPIDUnknownMSPNamesTheMSP asserts that when the channel
// configuration has a populated application section but no organization with
// the requested MSP ID, the error names both the MSP that was asked for and
// the channel, so an operator can tell which discovered peer's organization is
// missing from the channel configuration.
func TestTLSRootCertsByMSPIDUnknownMSPNamesTheMSP(t *testing.T) {
	t.Parallel()

	m := &fake.MSP{}
	ac := &fakeApplication{orgs: map[string]channelconfig.ApplicationOrg{
		"Org1": &fakeApplicationOrg{name: "Org1", mspID: "Org1MSP", msp: m},
	}}

	certs, err := tlsRootCertsByMSPID(ac, "NoSuchMSP", "mychannel")
	require.Error(t, err)
	require.Nil(t, certs)
	require.True(t, strings.Contains(err.Error(), "NoSuchMSP"),
		"error must name the MSP, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the channel, got [%v]", err)
}

// TestUpdateWithoutAConfigIsRejected covers a config envelope that unmarshals
// but carries no Config. Before the sequence was read this dereferenced
// cenv.Config blindly and would panic.
func TestUpdateWithoutAConfigIsRejected(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	payload := &cb.Payload{Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{})}
	env := &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload)}

	var err error
	require.NotPanics(t, func() { err = s.Update(env) })
	require.ErrorContains(t, err, "config envelope carries no config")
}

// TestConfigWaitReleasedByUpdate asserts that a waiting accessor is released as
// soon as a configuration is installed, rather than sitting out its whole wait
// budget.
func TestConfigWaitReleasedByUpdate(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")
	waiting := s.WithConfigWait(5 * time.Second)

	// Install the configuration only after a delay the accessor cannot skip.
	// A resolve() that never waits returns before this fires, and the elapsed
	// assertion below catches it; one that waits cannot return sooner. This is
	// what makes the test deterministic: starting the goroutine and calling
	// seed immediately (the earlier version of this test) left the ordering
	// of "waiter parks" vs. "update lands" a race, so a never-waiting
	// resolve() could still win by finding an already-loaded configuration
	// and returning the very same error a real wait would — indistinguishable
	// from the correct outcome. Asserting elapsed time instead of only the
	// error's shape closes that gap: only a genuine wait can take at least
	// delay to return.
	const delay = 250 * time.Millisecond
	updateErr := make(chan error, 1)
	go func() {
		time.Sleep(delay)
		// Update is called directly rather than through seed: seed uses
		// require, and require.NoError (and t.Fatalf under it) is not safe to
		// call from a goroutine other than the test's own - it can return
		// without actually stopping the test. Capture the error instead and
		// assert it on the main goroutine below.
		updateErr <- s.config.Update(func(*configuration, bool) (*configuration, error) {
			return &configuration{channelConfig: &channelconfig.ChannelConfig{}, sequence: 0}, nil
		})
	}()

	start := time.Now()
	// The assertion is about being RELEASED, not about the lookup result: an
	// empty configuration has no application section, so this returns an
	// error. What matters is that it returns at all instead of waiting out
	// its 5s budget, and that it does not return before the update lands.
	_, err := waiting.TLSRootCertsByMSPID("Org1MSP")
	elapsed := time.Since(start)

	require.NoError(t, <-updateErr, "the seeding update itself must be accepted")

	require.GreaterOrEqual(t, elapsed, delay,
		"accessor must have waited for the configuration to arrive, returned after only [%s]", elapsed)
	require.Error(t, err)
	// The error must be the one that comes from a LOADED configuration with no
	// application section, not the one a non-waiting accessor would return
	// before any configuration ever arrives. A resolve() that never waits
	// would return ErrNotInitialized here instead, immediately, without ever
	// observing the update - asserting only require.Error would not catch
	// that, since both are errors.
	require.NotErrorIs(t, err, driver.ErrNotInitialized,
		"error must come from the installed configuration, not from never having waited for it, got [%v]", err)
	require.True(t, strings.Contains(err.Error(), "application config does not exist"),
		"error must be the one produced by the seeded configuration, got [%v]", err)
}

// TestConfigWaitTimesOut asserts that a waiting accessor gives up once its
// budget elapses, and that the resulting error names what it was waiting for.
func TestConfigWaitTimesOut(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel").WithConfigWait(20 * time.Millisecond)

	_, err := s.TLSRootCertsByMSPID("Org1MSP")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name what it waited for, got [%v]", err)
}

// TestNoConfigWaitStillFailsImmediately asserts that a Service without
// WithConfigWait keeps the existing non-blocking behaviour, so callers that
// already handle ErrNotInitialized are unaffected.
func TestNoConfigWaitStillFailsImmediately(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")

	start := time.Now()
	_, err := s.TLSRootCertsByMSPID("Org1MSP")
	require.Error(t, err)
	require.True(t, errors.Is(err, driver.ErrNotInitialized))
	require.Less(t, time.Since(start), 100*time.Millisecond)
}

// TestWithConfigWaitDoesNotMutateReceiver guards a real startup-deadlock risk:
// the committer installs the channel configuration, so the non-waiting view it
// holds must never become a waiting view as a side effect of a waiting view
// being created for someone else (discovery). If WithConfigWait mutated its
// receiver instead of returning a copy, a node could deadlock at startup
// waiting on the very configuration it is supposed to install.
func TestWithConfigWaitDoesNotMutateReceiver(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")
	_ = s.WithConfigWait(5 * time.Second)

	start := time.Now()
	_, err := s.TLSRootCertsByMSPID("Org1MSP")
	require.Error(t, err)
	require.True(t, errors.Is(err, driver.ErrNotInitialized))
	require.Less(t, time.Since(start), 100*time.Millisecond,
		"the receiver of WithConfigWait must remain non-waiting")
}

// TestMSPManagerConfigWaitTimesOut asserts that the identity-validation path
// through MSPManager also waits, bounded, since that is the path Task 3's
// discovery validation uses. It stays before the first update so it does not
// depend on the fixture's channelconfig.MSPManager() being usable.
func TestMSPManagerConfigWaitTimesOut(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel").WithConfigWait(20 * time.Millisecond)
	mgr := s.MSPManager()

	_, err := mgr.DeserializeIdentity(view.Identity("some-identity"))
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name what it waited for, got [%v]", err)
}

// TestConfigWaitDoesNotWaitOnARejectedConfig asserts that a waiting accessor
// reports a refused configuration immediately rather than holding the caller
// for its whole budget. Waiting cannot turn a refusal into an answer - only a
// later accepted update can - so burning the budget would leave a node with a
// refused configuration hanging on every discovery-driven call instead of
// surfacing the reason.
func TestConfigWaitDoesNotWaitOnARejectedConfig(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")
	require.Error(t, s.Update(&cb.Envelope{Payload: []byte("not-a-proto")}))

	// A budget large enough that waiting it out would be unmistakable.
	waiting := s.WithConfigWait(10 * time.Second)

	start := time.Now()
	_, err := waiting.TLSRootCertsByMSPID("Org1MSP")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, driver.ErrConfigRejected)
	require.Less(t, elapsed, time.Second,
		"a refused configuration must be reported immediately, not after the wait budget, took [%s]", elapsed)
	require.True(t, strings.Contains(err.Error(), "mychannel"),
		"error must name the channel, got [%v]", err)
}

// TestMSPManagerConfigWaitDoesNotWaitOnARejectedConfig is the mspManager
// counterpart to TestConfigWaitDoesNotWaitOnARejectedConfig: the
// identity-validation path must not burn its wait budget on a refused
// configuration either.
func TestMSPManagerConfigWaitDoesNotWaitOnARejectedConfig(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")
	require.Error(t, s.Update(&cb.Envelope{Payload: []byte("not-a-proto")}))

	mgr := s.WithConfigWait(10 * time.Second).MSPManager()

	start := time.Now()
	_, err := mgr.DeserializeIdentity(view.Identity("some-identity"))
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, driver.ErrConfigRejected)
	require.Less(t, elapsed, time.Second,
		"a refused configuration must be reported immediately, not after the wait budget, took [%s]", elapsed)
}
