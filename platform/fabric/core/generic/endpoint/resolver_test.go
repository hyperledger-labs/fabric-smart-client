/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/endpoint/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestResolverGetIdentity_WithIdentityGetter(t *testing.T) {
	t.Parallel()

	r := &Resolver{
		IdentityGetter: func() (view.Identity, []byte, error) {
			return view.Identity("from-getter"), []byte("ignored-info"), nil
		},
	}

	id, err := r.GetIdentity()
	require.NoError(t, err)
	require.Equal(t, view.Identity("from-getter"), id)
}

func TestResolverGetIdentity_WithoutIdentityGetter(t *testing.T) {
	t.Parallel()

	r := &Resolver{Id: []byte("from-id")}
	id, err := r.GetIdentity()
	require.NoError(t, err)
	require.Equal(t, view.Identity("from-id"), id)
}

func TestNewResolverService_AddPublicKeyExtractorError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("extractor registration failed")
	cfg := &fake.Config{}
	backend := &fake.ResolverServiceBackend{
		AddPublicKeyExtractorErr: expectedErr,
	}

	service, err := NewResolverService(cfg, backend)
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, service)
	require.Equal(t, 1, backend.AddPublicKeyCalls)
}

func TestResolverServiceLoadResolvers_ConfigError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("resolvers not available")
	cfg := &fake.Config{ResolveErr: expectedErr}
	backend := &fake.ResolverServiceBackend{}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.ErrorIs(t, err, expectedErr)
	require.Empty(t, backend.AddCalls)
	require.Empty(t, backend.BindCalls)
}

func TestResolverServiceLoadResolvers_StatError(t *testing.T) {
	t.Parallel()

	cfg := &fake.Config{
		ResolversList: []config.Resolver{
			{
				Name: "alice",
				Identity: config.MSP{
					MSPID: "apple",
					Path:  "/path/does/not/exist",
				},
			},
		},
	}
	backend := &fake.ResolverServiceBackend{}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.ErrorContains(t, err, "failed to stat")
	require.Empty(t, backend.AddCalls)
}

func TestResolverServiceLoadResolvers_AddResolverError(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	cfg := &fake.Config{
		ResolversList: []config.Resolver{
			{
				Name: "alice",
				Identity: config.MSP{
					MSPID: "apple",
					Path:  certPath,
				},
			},
		},
	}
	expectedErr := errors.New("add resolver failed")
	backend := &fake.ResolverServiceBackend{
		AddResolverErr: expectedErr,
	}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.ErrorIs(t, err, expectedErr)
	require.Len(t, backend.AddCalls, 1)
	require.Empty(t, backend.BindCalls)
}

func TestResolverServiceLoadResolvers_BindAliasError(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	cfg := &fake.Config{
		ResolversList: []config.Resolver{
			{
				Name: "alice",
				Identity: config.MSP{
					MSPID: "apple",
					Path:  certPath,
				},
				Aliases: []string{"alice-alias"},
			},
		},
	}
	expectedErr := errors.New("bind failed")
	backend := &fake.ResolverServiceBackend{
		BindErr: expectedErr,
	}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.ErrorIs(t, err, expectedErr)
	require.Len(t, backend.AddCalls, 1)
	require.Len(t, backend.BindCalls, 1)
	require.Equal(t, view.Identity("alice-alias"), backend.BindCalls[0].Ephemeral[0])
}

func TestResolverServiceLoadResolvers_SuccessAndLookups(t *testing.T) {
	t.Parallel()

	mspPath := filepath.Join("..", "msp", "x509", "testdata", "msp")
	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	cfg := &fake.Config{
		ResolversList: []config.Resolver{
			{
				Name:    "alice",
				Domain:  "example.com",
				Aliases: []string{"a1"},
				Identity: config.MSP{
					MSPID: "apple",
					Path:  mspPath,
				},
				Addresses: map[string]string{"p2p": "127.0.0.1:1001"},
			},
			{
				Name:    "bob",
				Domain:  "example.com",
				Aliases: []string{"b1"},
				Identity: config.MSP{
					MSPID: "banana",
					Path:  certPath,
				},
				Addresses: map[string]string{"p2p": "127.0.0.1:1002"},
			},
		},
	}
	backend := &fake.ResolverServiceBackend{
		AddResolverFn: func(name, _ string, _ map[string]string, _ []string, _ []byte) (view.Identity, error) {
			return view.Identity("root-" + name), nil
		},
	}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.NoError(t, err)
	require.Equal(t, 1, backend.AddPublicKeyCalls)
	require.Len(t, backend.AddCalls, 2)
	require.Len(t, backend.BindCalls, 2)
	requireResolverLookups(t, cfg.ResolversList, backend.AddCalls, backend.BindCalls, service)

	require.Nil(t, service.GetIdentity("unknown"))
}

func requireResolverLookups(t *testing.T, resolvers []config.Resolver, addCalls []fake.AddResolverCall, bindCalls []fake.BindCall, service *ResolverService) {
	t.Helper()

	callsByName := map[string]fake.AddResolverCall{}
	for _, call := range addCalls {
		callsByName[call.Name] = call
	}

	bindCallsByLongTerm := map[string][]fake.BindCall{}
	for _, call := range bindCalls {
		bindCallsByLongTerm[string(call.LongTerm)] = append(bindCallsByLongTerm[string(call.LongTerm)], call)
	}

	for _, resolver := range resolvers {
		call, ok := callsByName[resolver.Name]
		require.True(t, ok, "missing add resolver call for %s", resolver.Name)
		require.Equal(t, resolver.Domain, call.Domain)
		require.Equal(t, resolver.Addresses, call.Addresses)
		require.Equal(t, resolver.Aliases, call.Aliases)
		require.NotEmpty(t, call.ID)
		require.NotEmpty(t, call.RootID)

		expected := view.Identity(call.ID)
		require.Equal(t, expected, service.GetIdentity(resolver.Name))
		require.Equal(t, expected, service.GetIdentity(expected.UniqueID()))
		require.Equal(t, expected, service.GetIdentity(call.RootID.UniqueID()))

		nameBindCalls := bindCallsByLongTerm[string(expected)]
		require.Len(t, nameBindCalls, len(resolver.Aliases))

		boundAliases := map[string]struct{}{}
		for _, call := range nameBindCalls {
			require.Len(t, call.Ephemeral, 1)
			boundAliases[string(call.Ephemeral[0])] = struct{}{}
		}
		for _, alias := range resolver.Aliases {
			require.Equal(t, expected, service.GetIdentity(alias))
			require.Contains(t, boundAliases, alias)
		}
	}
}
