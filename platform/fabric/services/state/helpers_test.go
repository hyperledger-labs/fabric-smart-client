/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package state

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

// helpersProvider serves vs as the vault service and nsp as the fabric network service provider.
func helpersProvider(vs VaultService, nsp *fabric.NetworkServiceProvider) *mockServiceProvider {
	return &mockServiceProvider{getFn: func(v any) (any, error) {
		switch v {
		case reflect.TypeFor[*VaultService]():
			return vs, nil
		case reflect.TypeFor[*fabric.NetworkServiceProvider]():
			return nsp, nil
		}
		return nil, errors.New("service missing")
	}}
}

func TestGetVault(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		v := &testCertificationVault{}
		vs := &testCertificationVaultService{vault: v}
		got, err := GetVault(helpersProvider(vs, recipientsNSP()))
		require.NoError(t, err)
		require.Same(t, v, got)
		require.Equal(t, "fns", vs.networkCalled)
		require.Equal(t, "ch", vs.channelCalled)
	})

	t.Run("vault service error", func(t *testing.T) {
		t.Parallel()
		_, err := GetVault(&mockServiceProvider{getFn: func(any) (any, error) { return nil, errors.New("no vault service") }})
		require.EqualError(t, err, "no vault service")
	})

	t.Run("default channel error", func(t *testing.T) {
		t.Parallel()
		nsp := fabric.NewNetworkServiceProvider(&testFabricDriverFNSProvider{fns: &testFabricDriverFNS{channelErr: errors.New("channel failed")}}, nil)
		vs := &testCertificationVaultService{}
		_, err := GetVault(helpersProvider(vs, nsp))
		require.ErrorContains(t, err, "channel failed")
		require.Empty(t, vs.networkCalled)
	})

	t.Run("vault error", func(t *testing.T) {
		t.Parallel()
		_, err := GetVault(helpersProvider(&testCertificationVaultService{err: errors.New("vault failed")}, recipientsNSP()))
		require.EqualError(t, err, "vault failed")
	})
}

func TestGetVaultForChannel(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		v := &testCertificationVault{}
		vs := &testCertificationVaultService{vault: v}
		got, err := GetVaultForChannel(helpersProvider(vs, recipientsNSP()), "other-ch")
		require.NoError(t, err)
		require.Same(t, v, got)
		require.Equal(t, "fns", vs.networkCalled)
		require.Equal(t, "other-ch", vs.channelCalled)
	})

	t.Run("vault service error", func(t *testing.T) {
		t.Parallel()
		_, err := GetVaultForChannel(&mockServiceProvider{getFn: func(any) (any, error) { return nil, errors.New("no vault service") }}, "ch")
		require.EqualError(t, err, "no vault service")
	})

	t.Run("fns error", func(t *testing.T) {
		t.Parallel()
		vs := &testCertificationVaultService{}
		_, err := GetVaultForChannel(helpersProvider(vs, failingNSP()), "ch")
		require.ErrorContains(t, err, "fns failed")
		require.Empty(t, vs.networkCalled)
	})

	t.Run("vault error", func(t *testing.T) {
		t.Parallel()
		_, err := GetVaultForChannel(helpersProvider(&testCertificationVaultService{err: errors.New("vault failed")}, recipientsNSP()), "ch")
		require.EqualError(t, err, "vault failed")
	})
}
