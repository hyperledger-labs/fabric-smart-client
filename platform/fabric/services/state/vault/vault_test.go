/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// The fakes below embed the driver interfaces and implement only the methods
// the code under test calls; any other call panics.

type fakeQueryExecutor struct {
	driver.QueryExecutor
	read *driver.VaultRead
	err  error
	done bool
}

func (f *fakeQueryExecutor) GetState(context.Context, driver.Namespace, driver.PKey) (*driver.VaultRead, error) {
	return f.read, f.err
}

func (f *fakeQueryExecutor) Done() error {
	f.done = true
	return nil
}

type fakeStorage struct {
	qe  *fakeQueryExecutor
	err error
}

func (f *fakeStorage) NewQueryExecutor(context.Context) (driver.QueryExecutor, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.qe, nil
}

type fakeDriverVault struct {
	fdriver.Vault
	qe *fakeQueryExecutor
}

func (f *fakeDriverVault) NewQueryExecutor(context.Context) (driver.QueryExecutor, error) {
	return f.qe, nil
}

type fakeChannel struct {
	fdriver.Channel
	vault fdriver.Vault
}

func (*fakeChannel) Name() string                                           { return "ch" }
func (*fakeChannel) Committer() fdriver.Committer                           { return nil }
func (f *fakeChannel) Vault() fdriver.Vault                                 { return f.vault }
func (*fakeChannel) TransactionService() fdriver.EndorserTransactionService { return nil }
func (*fakeChannel) EnvelopeService() fdriver.EnvelopeService               { return nil }
func (*fakeChannel) MetadataService() fdriver.MetadataService               { return nil }

type fakeLocalMembership struct {
	fdriver.LocalMembership
}

func (*fakeLocalMembership) DefaultIdentity() view.Identity { return view.Identity("me-id") }

type fakeFNS struct {
	fdriver.FabricNetworkService
	channel    fdriver.Channel
	channelErr error
}

func (*fakeFNS) Name() string { return "net" }

func (f *fakeFNS) Channel(string) (fdriver.Channel, error) {
	return f.channel, f.channelErr
}

func (*fakeFNS) LocalMembership() fdriver.LocalMembership { return &fakeLocalMembership{} }

type fakeFNSProvider struct {
	fdriver.FabricNetworkServiceProvider
	fns fdriver.FabricNetworkService
	err error
}

func (f *fakeFNSProvider) FabricNetworkService(string) (fdriver.FabricNetworkService, error) {
	return f.fns, f.err
}

type noServices struct{}

func (noServices) GetService(any) (any, error) { return nil, errors.New("service missing") }

func TestVaultGetState(t *testing.T) {
	t.Parallel()

	type asset struct {
		Name string `json:"name"`
	}

	t.Run("query executor error", func(t *testing.T) {
		t.Parallel()
		expected := errors.New("qe failed")
		v := &vault{vaultStore: &fakeStorage{err: expected}}
		require.ErrorIs(t, v.GetState(t.Context(), "ns", "k", &asset{}), expected)
	})

	for _, tc := range []struct {
		name    string
		qe      *fakeQueryExecutor
		want    asset
		wantErr string
	}{
		{name: "success", qe: &fakeQueryExecutor{read: &driver.VaultRead{Raw: []byte(`{"name":"a1"}`)}}, want: asset{Name: "a1"}},
		{name: "get state error", qe: &fakeQueryExecutor{err: errors.New("get failed")}, wantErr: "get failed"},
		{name: "nil read", qe: &fakeQueryExecutor{}, wantErr: "id [k] not found"},
		{name: "empty value", qe: &fakeQueryExecutor{read: &driver.VaultRead{}}, wantErr: "id [k] not found"},
		{name: "malformed value", qe: &fakeQueryExecutor{read: &driver.VaultRead{Raw: []byte("{bad")}}, wantErr: "invalid character"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			v := &vault{vaultStore: &fakeStorage{qe: tc.qe}}
			var got asset
			err := v.GetState(t.Context(), "ns", "k", &got)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
			}
			require.True(t, tc.qe.done, "query executor must be released")
		})
	}
}

func TestVaultGetStateCertificationTransactionError(t *testing.T) {
	t.Parallel()

	v := &vault{sp: noServices{}, network: "net", channel: "ch", localMembership: &fakeLocalMembership{}}
	_, err := v.GetStateCertification(t.Context(), "ns", "k")
	require.ErrorContains(t, err, "failed creating transaction [ns:k]")
}

func TestServiceVault(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		qe := &fakeQueryExecutor{read: &driver.VaultRead{Raw: []byte(`"v"`)}}
		fns := &fakeFNS{channel: &fakeChannel{vault: &fakeDriverVault{qe: qe}}}
		sp := noServices{}
		s := NewService(sp, fabric.NewNetworkServiceProvider(&fakeFNSProvider{fns: fns}, nil))

		// Names come from the resolved services, not from the arguments.
		sv, err := s.Vault("net-alias", "ch-alias")
		require.NoError(t, err)
		v, ok := sv.(*vault)
		require.True(t, ok)
		require.Equal(t, sp, v.sp)
		require.Equal(t, "net", v.network)
		require.Equal(t, "ch", v.channel)
		require.Equal(t, view.Identity("me-id"), v.localMembership.DefaultIdentity())

		// The vault store is the channel's vault.
		var got string
		require.NoError(t, sv.GetState(t.Context(), "ns", "k", &got))
		require.Equal(t, "v", got)
		require.True(t, qe.done)
	})

	t.Run("network error", func(t *testing.T) {
		t.Parallel()
		s := NewService(noServices{}, fabric.NewNetworkServiceProvider(&fakeFNSProvider{err: errors.New("fns failed")}, nil))
		_, err := s.Vault("net", "ch")
		require.ErrorContains(t, err, "fns failed")
	})

	t.Run("channel error", func(t *testing.T) {
		t.Parallel()
		fns := &fakeFNS{channelErr: errors.New("channel failed")}
		s := NewService(noServices{}, fabric.NewNetworkServiceProvider(&fakeFNSProvider{fns: fns}, nil))
		_, err := s.Vault("net", "ch")
		require.EqualError(t, err, "channel failed")
	})
}
