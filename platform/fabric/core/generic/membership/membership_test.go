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
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

var _ driver.MembershipService = (*Service)(nil)

// seed installs cfg as the service's channel configuration, standing in for the
// first successful Update.
func seed(t *testing.T, s *Service, cfg *channelconfig.ChannelConfig) {
	t.Helper()
	require.NoError(t, s.config.Update(func(*channelconfig.ChannelConfig, bool) (*channelconfig.ChannelConfig, error) {
		return cfg, nil
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

func TestUpdateWithInvalidEnvelopeLeavesServiceUninitialized(t *testing.T) {
	t.Parallel()
	s := NewService("mychannel")

	require.Error(t, s.Update(&cb.Envelope{Payload: []byte("not-a-proto")}))

	_, err := s.GetMSPIDs()
	require.ErrorIs(t, err, driver.ErrNotInitialized,
		"a rejected configuration must leave the service uninitialized")
}

// TestLoadedConfigIsDistinguishableFromMissingConfig is the point of the
// refactoring: an empty answer from a loaded configuration and an absent
// configuration are no longer the same result.
func TestLoadedConfigIsDistinguishableFromMissingConfig(t *testing.T) {
	t.Parallel()

	t.Run("GetMSPIDs on a config with no application section", func(t *testing.T) {
		t.Parallel()
		s := NewService("mychannel")
		seed(t, s, &channelconfig.ChannelConfig{})

		ids, err := s.GetMSPIDs()
		require.NoError(t, err, "a loaded configuration must not report ErrNotInitialized")
		require.Empty(t, ids)
	})

	t.Run("IsIdemixMSP on a config with no application section", func(t *testing.T) {
		t.Parallel()
		s := NewService("mychannel")
		seed(t, s, &channelconfig.ChannelConfig{})

		isIdemix, err := s.IsIdemixMSP("Org1MSP")
		require.NoError(t, err, "a loaded configuration must not report ErrNotInitialized")
		require.False(t, isIdemix)
	})

	t.Run("OrdererConfig on a config with no orderer section", func(t *testing.T) {
		t.Parallel()
		s := NewService("mychannel")
		seed(t, s, &channelconfig.ChannelConfig{})

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
