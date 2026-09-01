/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package membership

import (
	"strings"
	"testing"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership/channelconfig"
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

// TestUpdateWithoutAConfigIsRejected covers a config envelope that unmarshals
// but carries no Config. Before the sequence was read this dereferenced
// cenv.Config blindly and would panic.
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
}

// TestTLSRootCertsByMSPIDUnknownMSPNamesTheMSP asserts the error names the MSP
// that was asked for, so an operator can tell which discovered peer's
// organization is missing from the channel configuration.
func TestTLSRootCertsByMSPIDUnknownMSPNamesTheMSP(t *testing.T) {
	t.Parallel()

	s := NewService("mychannel")
	seed(t, s, &channelconfig.ChannelConfig{}, 0)

	_, err := s.TLSRootCertsByMSPID("NoSuchMSP")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "NoSuchMSP"),
		"error must name the MSP, got [%v]", err)
}

func TestUpdateWithoutAConfigIsRejected(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	payload := &cb.Payload{Data: protoutil.MarshalOrPanic(&cb.ConfigEnvelope{})}
	env := &cb.Envelope{Payload: protoutil.MarshalOrPanic(payload)}

	var err error
	require.NotPanics(t, func() { err = s.Update(env) })
	require.ErrorContains(t, err, "config envelope carries no config")
}
