/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/config"
	viewendpoint "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type fakeConfig struct {
	resolvers  []config.Resolver
	resolveErr error
	translate  func(path string) string
}

func (f *fakeConfig) Resolvers() ([]config.Resolver, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return f.resolvers, nil
}

func (f *fakeConfig) TranslatePath(path string) string {
	if f.translate != nil {
		return f.translate(path)
	}
	return path
}

type bindCall struct {
	longTerm  view.Identity
	ephemeral []view.Identity
}

type addResolverCall struct {
	name      string
	domain    string
	addresses map[string]string
	aliases   []string
	id        []byte
}

type fakeResolverServiceBackend struct {
	addPublicKeyExtractorErr error
	addPublicKeyCalls        int

	addResolverErr error
	addResolverFn  func(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
	addResolverIDs []view.Identity
	addCalls       []addResolverCall

	bindErr   error
	bindCalls []bindCall
}

func (f *fakeResolverServiceBackend) Bind(_ context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	f.bindCalls = append(f.bindCalls, bindCall{
		longTerm:  longTerm,
		ephemeral: append([]view.Identity(nil), ephemeral...),
	})
	return f.bindErr
}

func (f *fakeResolverServiceBackend) AddResolver(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	f.addCalls = append(f.addCalls, addResolverCall{
		name:      name,
		domain:    domain,
		addresses: copyStringMap(addresses),
		aliases:   append([]string(nil), aliases...),
		id:        append([]byte(nil), id...),
	})
	if f.addResolverErr != nil {
		return nil, f.addResolverErr
	}
	if f.addResolverFn != nil {
		return f.addResolverFn(name, domain, addresses, aliases, id)
	}
	if len(f.addResolverIDs) > 0 {
		next := f.addResolverIDs[0]
		f.addResolverIDs = f.addResolverIDs[1:]
		return next, nil
	}
	return view.Identity("root-id"), nil
}

func (f *fakeResolverServiceBackend) AddPublicKeyExtractor(_ viewendpoint.PublicKeyExtractor) error {
	f.addPublicKeyCalls++
	return f.addPublicKeyExtractorErr
}

func copyStringMap(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

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
	cfg := &fakeConfig{}
	backend := &fakeResolverServiceBackend{
		addPublicKeyExtractorErr: expectedErr,
	}

	service, err := NewResolverService(cfg, backend)
	require.Error(t, err)
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, service)
	require.Equal(t, 1, backend.addPublicKeyCalls)
}

func TestResolverServiceLoadResolvers_ConfigError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("resolvers not available")
	cfg := &fakeConfig{resolveErr: expectedErr}
	backend := &fakeResolverServiceBackend{}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.ErrorIs(t, err, expectedErr)
	require.Empty(t, backend.addCalls)
	require.Empty(t, backend.bindCalls)
}

func TestResolverServiceLoadResolvers_StatError(t *testing.T) {
	t.Parallel()

	cfg := &fakeConfig{
		resolvers: []config.Resolver{
			{
				Name: "alice",
				Identity: config.MSP{
					MSPID: "apple",
					Path:  "/path/does/not/exist",
				},
			},
		},
	}
	backend := &fakeResolverServiceBackend{}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to stat")
	require.Empty(t, backend.addCalls)
}

func TestResolverServiceLoadResolvers_AddResolverError(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	cfg := &fakeConfig{
		resolvers: []config.Resolver{
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
	backend := &fakeResolverServiceBackend{
		addResolverErr: expectedErr,
	}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.Error(t, err)
	require.ErrorIs(t, err, expectedErr)
	require.Len(t, backend.addCalls, 1)
	require.Empty(t, backend.bindCalls)
}

func TestResolverServiceLoadResolvers_BindAliasError(t *testing.T) {
	t.Parallel()

	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	cfg := &fakeConfig{
		resolvers: []config.Resolver{
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
	backend := &fakeResolverServiceBackend{
		bindErr: expectedErr,
	}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.Error(t, err)
	require.ErrorIs(t, err, expectedErr)
	require.Len(t, backend.addCalls, 1)
	require.Len(t, backend.bindCalls, 1)
	require.Equal(t, view.Identity("alice-alias"), backend.bindCalls[0].ephemeral[0])
}

func TestResolverServiceLoadResolvers_SuccessAndLookups(t *testing.T) {
	t.Parallel()

	mspPath := filepath.Join("..", "msp", "x509", "testdata", "msp")
	certPath := filepath.Join("..", "msp", "x509", "testdata", "msp", "signcerts", "auditor.org1.example.com-cert.pem")
	cfg := &fakeConfig{
		resolvers: []config.Resolver{
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
	backend := &fakeResolverServiceBackend{
		addResolverFn: func(name, _ string, _ map[string]string, _ []string, _ []byte) (view.Identity, error) {
			return view.Identity("root-" + name), nil
		},
	}
	service, err := NewResolverService(cfg, backend)
	require.NoError(t, err)

	err = service.LoadResolvers()
	require.NoError(t, err)
	require.Equal(t, 1, backend.addPublicKeyCalls)
	require.Len(t, backend.addCalls, 2)
	require.Len(t, backend.bindCalls, 2)
	require.Len(t, service.resolvers, 2)

	alice := service.resolvers[0]
	bob := service.resolvers[1]

	require.Equal(t, view.Identity(alice.Id), service.GetIdentity("alice"))
	require.Equal(t, view.Identity(alice.Id), service.GetIdentity("a1"))
	require.Equal(t, view.Identity(alice.Id), service.GetIdentity(view.Identity(alice.Id).UniqueID()))
	require.Equal(t, view.Identity(alice.Id), service.GetIdentity(alice.RootID.UniqueID()))

	require.Equal(t, view.Identity(bob.Id), service.GetIdentity("bob"))
	require.Equal(t, view.Identity(bob.Id), service.GetIdentity("b1"))
	require.Equal(t, view.Identity(bob.Id), service.GetIdentity(view.Identity(bob.Id).UniqueID()))
	require.Equal(t, view.Identity(bob.Id), service.GetIdentity(bob.RootID.UniqueID()))

	require.Nil(t, service.GetIdentity("unknown"))
}
