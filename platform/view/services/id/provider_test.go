/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package id_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	id2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/kms/driver/file"
	mock2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/id/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type fakeEndpointService struct {
	identity view.Identity
	err      error
	label    string
}

func (f *fakeEndpointService) GetIdentity(label string, _ []byte) (view.Identity, error) {
	f.label = label
	return f.identity, f.err
}

type fakeServiceProvider struct {
	service any
	err     error
}

func (f *fakeServiceProvider) GetService(any) (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.service, nil
}

func TestLoad(t *testing.T) {
	t.Parallel()
	cp := &mock2.ConfigProvider{}
	cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/default.pem")
	cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
	cp.GetStringSliceReturnsOnCall(0, []string{
		"./testdata/client/client.pem",
	})
	cp.TranslatePathReturnsOnCall(0, "./testdata/client/client.pem")
	sigService := &mock2.SigService{}

	idProvider, err := id2.NewProvider(cp, sigService, nil, &kms.KMS{Driver: &file.Driver{}})
	require.NoError(t, err, "failed loading identities")

	raw, err := id2.LoadIdentity("./testdata/default/signcerts/default.pem")
	require.NoError(t, err)
	require.Equal(t, raw, []byte(idProvider.DefaultIdentity()))

	raw, err = id2.LoadIdentity("./testdata/client/client.pem")
	require.NoError(t, err)
	require.Len(t, idProvider.Clients(), 1)
	require.Equal(t, raw, []byte(idProvider.Clients()[0]))
	require.Len(t, idProvider.Admins(), 0)
}

func TestNewProviderFailurePaths(t *testing.T) {
	t.Parallel()

	t.Run("default identity load fails", func(t *testing.T) {
		t.Parallel()
		cp := &mock2.ConfigProvider{}
		cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/missing.pem")
		cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
		cp.GetStringSliceReturns([]string{})
		sigService := &mock2.SigService{}

		_, err := id2.NewProvider(cp, sigService, nil, &kms.KMS{Driver: &file.Driver{}})
		require.ErrorContains(t, err, "failed loading identities")
		require.ErrorContains(t, err, "failed loading default identity")
	})

	t.Run("register signer fails", func(t *testing.T) {
		t.Parallel()
		cp := &mock2.ConfigProvider{}
		cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/default.pem")
		cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
		cp.GetStringSliceReturns([]string{})
		sigService := &mock2.SigService{}
		sigService.RegisterSignerReturns(errors.New("register-failed"))

		_, err := id2.NewProvider(cp, sigService, nil, &kms.KMS{Driver: &file.Driver{}})
		require.ErrorContains(t, err, "failed loading identities")
		require.ErrorContains(t, err, "failed registering default identity signer")
	})
}

func TestProviderIdentity(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		cp := &mock2.ConfigProvider{}
		cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/default.pem")
		cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
		cp.GetStringSliceReturns([]string{})
		sigService := &mock2.SigService{}
		endpoint := &fakeEndpointService{identity: view.Identity([]byte("alice-id"))}

		idProvider, err := id2.NewProvider(cp, sigService, endpoint, &kms.KMS{Driver: &file.Driver{}})
		require.NoError(t, err)

		got := idProvider.Identity("alice")
		require.Equal(t, view.Identity([]byte("alice-id")), got)
		require.Equal(t, "alice", endpoint.label)
	})

	t.Run("failure returns nil", func(t *testing.T) {
		t.Parallel()
		cp := &mock2.ConfigProvider{}
		cp.GetPathReturnsOnCall(0, "./testdata/default/signcerts/default.pem")
		cp.GetPathReturnsOnCall(1, "./testdata/default/keystore/priv_sk")
		cp.GetStringSliceReturns([]string{})
		sigService := &mock2.SigService{}
		endpoint := &fakeEndpointService{err: errors.New("lookup-failed")}

		idProvider, err := id2.NewProvider(cp, sigService, endpoint, &kms.KMS{Driver: &file.Driver{}})
		require.NoError(t, err)

		got := idProvider.Identity("bob")
		require.Nil(t, got)
		require.Equal(t, "bob", endpoint.label)
	})
}

func TestGetProvider(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		expected := &id2.Provider{}
		got, err := id2.GetProvider(&fakeServiceProvider{service: expected})
		require.NoError(t, err)
		require.Same(t, expected, got)
	})

	t.Run("provider error", func(t *testing.T) {
		t.Parallel()
		_, err := id2.GetProvider(&fakeServiceProvider{err: errors.New("service-missing")})
		require.ErrorContains(t, err, "service-missing")
	})
}
