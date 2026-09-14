/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/msp/tlsgen"
	viewendpoint "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

type mockConfig struct {
	values       map[string]string
	isSet        map[string]bool
	unmarshalFn  func(key string, val any) error
	translateMap map[string]string
}

func (m *mockConfig) GetString(key string) string {
	return m.values[key]
}

func (m *mockConfig) IsSet(s string) bool {
	return m.isSet[s]
}

func (m *mockConfig) UnmarshalKey(s string, i any) error {
	if m.unmarshalFn != nil {
		return m.unmarshalFn(s, i)
	}
	return nil
}

func (m *mockConfig) TranslatePath(path string) string {
	if m.translateMap != nil {
		if val, ok := m.translateMap[path]; ok {
			return val
		}
	}
	return path
}

type mockIdentityService struct {
	defaultID view.Identity
}

func (m *mockIdentityService) DefaultIdentity() view.Identity {
	return m.defaultID
}

type mockBackend struct {
	addResolverFn func(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error)
	bindFn        func(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error
}

func (m *mockBackend) AddResolver(name, domain string, addresses map[string]string, aliases []string, id []byte) (view.Identity, error) {
	if m.addResolverFn != nil {
		return m.addResolverFn(name, domain, addresses, aliases, id)
	}
	return view.Identity(id), nil
}

func (m *mockBackend) Bind(ctx context.Context, longTerm view.Identity, ephemeral ...view.Identity) error {
	if m.bindFn != nil {
		return m.bindFn(ctx, longTerm, ephemeral...)
	}
	return nil
}

func TestEntry_GetIdentity(t *testing.T) {
	t.Parallel()

	t.Run("without getter", func(t *testing.T) {
		t.Parallel()
		e := &entry{ID: []byte("raw-id")}
		id, err := e.GetIdentity()
		require.NoError(t, err)
		assert.Equal(t, view.Identity("raw-id"), id)
	})

	t.Run("with getter success", func(t *testing.T) {
		t.Parallel()
		e := &entry{
			IdentityGetter: func() (view.Identity, []byte, error) {
				return view.Identity("getter-id"), []byte("audit"), nil
			},
		}
		id, err := e.GetIdentity()
		require.NoError(t, err)
		assert.Equal(t, view.Identity("getter-id"), id)
	})

	t.Run("with getter error", func(t *testing.T) {
		t.Parallel()
		expectedErr := errors.New("getter failure")
		e := &entry{
			IdentityGetter: func() (view.Identity, []byte, error) {
				return nil, nil, expectedErr
			},
		}
		_, err := e.GetIdentity()
		require.ErrorIs(t, err, expectedErr)
	})
}

func TestConvertAddress(t *testing.T) {
	t.Parallel()

	t.Run("converts 0.0.0.0 prefix to 127.0.0.1", func(t *testing.T) {
		t.Parallel()
		addr, err := convertAddress("/ip4/0.0.0.0/tcp/9000")
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1:9000", addr)
	})

	t.Run("keeps non-zero ip address unchanged", func(t *testing.T) {
		t.Parallel()
		addr, err := convertAddress("/ip4/192.168.1.1/tcp/9000")
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.1:9000", addr)
	})

	t.Run("fails on malformed address", func(t *testing.T) {
		t.Parallel()
		_, err := convertAddress("invalid:address")
		require.Error(t, err)
	})
}

func TestNewResolversLoader(t *testing.T) {
	t.Parallel()

	cfg := &mockConfig{values: map[string]string{}}
	is := &mockIdentityService{}
	backend := &mockBackend{}

	loader, err := NewResolversLoader(cfg, backend, is)
	require.NoError(t, err)
	require.NotNil(t, loader)
	assert.Equal(t, cfg, loader.config)
	assert.Equal(t, backend, loader.backend)
	assert.Equal(t, is, loader.is)
}

func TestLoadResolvers_Errors(t *testing.T) {
	t.Parallel()

	t.Run("invalid p2p address fails address conversion", func(t *testing.T) {
		t.Parallel()
		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "bad-addr",
			},
		}
		loader, err := NewResolversLoader(cfg, &mockBackend{}, &mockIdentityService{})
		require.NoError(t, err)
		err = loader.LoadResolvers()
		require.ErrorContains(t, err, "failed to convert address [bad-addr]")
	})

	t.Run("default resolver failure propagates", func(t *testing.T) {
		t.Parallel()
		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/127.0.0.1/tcp/9000",
				"fsc.id":                "node1",
				"fsc.grpc.address":      "127.0.0.1:9001",
			},
		}
		backendErr := errors.New("backend add default resolver failed")
		backend := &mockBackend{
			addResolverFn: func(_, _ string, _ map[string]string, _ []string, _ []byte) (view.Identity, error) {
				return nil, backendErr
			},
		}
		loader, err := NewResolversLoader(cfg, backend, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		err = loader.LoadResolvers()
		require.ErrorContains(t, err, "failed adding default resolver")
	})

	t.Run("unmarshal resolvers error propagates", func(t *testing.T) {
		t.Parallel()
		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/127.0.0.1/tcp/9000",
				"fsc.id":                "node1",
			},
			isSet: map[string]bool{
				"fsc.endpoint.resolvers": true,
			},
			unmarshalFn: func(_ string, _ any) error {
				return errors.New("unmarshal error")
			},
		}
		loader, err := NewResolversLoader(cfg, &mockBackend{}, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		err = loader.LoadResolvers()
		require.ErrorContains(t, err, "failed loading resolvers")
	})

	t.Run("invalid identity path fails", func(t *testing.T) {
		t.Parallel()
		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/127.0.0.1/tcp/9000",
				"fsc.id":                "node1",
			},
			isSet: map[string]bool{
				"fsc.endpoint.resolvers": true,
			},
			unmarshalFn: func(_ string, val any) error {
				target := val.(*[]*entry)
				*target = []*entry{
					{
						Name:     "peer1",
						Identity: Identity{Path: "non-existent.crt"},
					},
				}
				return nil
			},
		}
		loader, err := NewResolversLoader(cfg, &mockBackend{}, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		err = loader.LoadResolvers()
		require.Error(t, err)
	})

	t.Run("adding resolver fails", func(t *testing.T) {
		t.Parallel()
		ca, err := tlsgen.NewCA()
		require.NoError(t, err)
		kp, err := ca.NewServerCertKeyPair("127.0.0.1")
		require.NoError(t, err)
		certFile := filepath.Join(t.TempDir(), "cert.crt")
		require.NoError(t, os.WriteFile(certFile, kp.Cert, 0o600))

		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/127.0.0.1/tcp/9000",
				"fsc.id":                "node1",
			},
			isSet: map[string]bool{
				"fsc.endpoint.resolvers": true,
			},
			unmarshalFn: func(_ string, val any) error {
				target := val.(*[]*entry)
				*target = []*entry{
					{
						Name:     "peer1",
						Identity: Identity{Path: certFile},
					},
				}
				return nil
			},
		}

		backendCalls := 0
		backend := &mockBackend{
			addResolverFn: func(_, _ string, _ map[string]string, _ []string, id []byte) (view.Identity, error) {
				backendCalls++
				if backendCalls == 1 {
					// Default resolver
					return view.Identity(id), nil
				}
				return nil, errors.New("fail adding entry resolver")
			},
		}
		loader, err := NewResolversLoader(cfg, backend, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		err = loader.LoadResolvers()
		require.ErrorContains(t, err, "failed adding resolver")
	})

	t.Run("binding alias fails", func(t *testing.T) {
		t.Parallel()
		ca, err := tlsgen.NewCA()
		require.NoError(t, err)
		kp, err := ca.NewServerCertKeyPair("127.0.0.1")
		require.NoError(t, err)
		certFile := filepath.Join(t.TempDir(), "cert.crt")
		require.NoError(t, os.WriteFile(certFile, kp.Cert, 0o600))

		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/127.0.0.1/tcp/9000",
				"fsc.id":                "node1",
			},
			isSet: map[string]bool{
				"fsc.endpoint.resolvers": true,
			},
			unmarshalFn: func(_ string, val any) error {
				target := val.(*[]*entry)
				*target = []*entry{
					{
						Name:     "peer1",
						Identity: Identity{Path: certFile},
						Aliases:  []string{"alias1"},
					},
				}
				return nil
			},
		}

		backend := &mockBackend{
			bindFn: func(_ context.Context, _ view.Identity, _ ...view.Identity) error {
				return errors.New("bind failure")
			},
		}
		loader, err := NewResolversLoader(cfg, backend, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		err = loader.LoadResolvers()
		require.ErrorContains(t, err, "failed binding identity [peer1] to alias [alias1]")
	})
}

func TestLoadResolvers_Success(t *testing.T) {
	t.Parallel()

	t.Run("without extra resolvers", func(t *testing.T) {
		t.Parallel()
		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/0.0.0.0/tcp/9000",
				"fsc.id":                "node1",
				"fsc.grpc.address":      "127.0.0.1:9001",
			},
			isSet: map[string]bool{
				"fsc.endpoint.resolvers": false,
			},
		}
		var addedName string
		var addedAddresses map[string]string
		backend := &mockBackend{
			addResolverFn: func(name, _ string, addresses map[string]string, _ []string, id []byte) (view.Identity, error) {
				addedName = name
				addedAddresses = addresses
				return view.Identity(id), nil
			},
		}
		loader, err := NewResolversLoader(cfg, backend, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		require.NoError(t, loader.LoadResolvers())
		assert.Equal(t, "node1", addedName)
		assert.Equal(t, "127.0.0.1:9000", addedAddresses[string(viewendpoint.P2PPort)])
		assert.Equal(t, "127.0.0.1:9001", addedAddresses[string(viewendpoint.ViewPort)])
	})

	t.Run("with extra resolvers and aliases", func(t *testing.T) {
		t.Parallel()
		ca, err := tlsgen.NewCA()
		require.NoError(t, err)
		kp, err := ca.NewServerCertKeyPair("127.0.0.1")
		require.NoError(t, err)
		certFile := filepath.Join(t.TempDir(), "cert.crt")
		require.NoError(t, os.WriteFile(certFile, kp.Cert, 0o600))

		cfg := &mockConfig{
			values: map[string]string{
				"fsc.p2p.listenAddress": "/ip4/127.0.0.1/tcp/9000",
				"fsc.id":                "node1",
				"fsc.grpc.address":      "127.0.0.1:9001",
			},
			isSet: map[string]bool{
				"fsc.endpoint.resolvers": true,
			},
			translateMap: map[string]string{
				"relative/cert.crt": certFile,
			},
			unmarshalFn: func(_ string, val any) error {
				target := val.(*[]*entry)
				*target = []*entry{
					{
						Name:     "peer1",
						Domain:   "example.com",
						Identity: Identity{Path: "relative/cert.crt"},
						Addresses: map[string]string{
							"ViewPort": "127.0.0.1:9002",
						},
						Aliases: []string{"alias1", "alias2"},
					},
				}
				return nil
			},
		}

		boundAliases := make([]string, 0)
		backend := &mockBackend{
			bindFn: func(_ context.Context, _ view.Identity, ephemeral ...view.Identity) error {
				for _, eph := range ephemeral {
					boundAliases = append(boundAliases, string(eph))
				}
				return nil
			},
		}
		loader, err := NewResolversLoader(cfg, backend, &mockIdentityService{defaultID: view.Identity("def-id")})
		require.NoError(t, err)
		require.NoError(t, loader.LoadResolvers())
		assert.Equal(t, []string{"alias1", "alias2"}, boundAliases)
	})
}
