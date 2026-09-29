/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package routing

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	host2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// fakeEndpointService returns canned identities and resolvers, and counts the
// calls so the label cache can be observed. The counters are atomic so the fake
// is safe to share across concurrent callers and under -race.
type fakeEndpointService struct {
	identity      view.Identity
	identityErr   error
	resolver      *endpoint.Resolver
	resolverErr   error
	identityCalls atomic.Int32
	resolverCalls atomic.Int32
}

func (f *fakeEndpointService) GetIdentity(string, []byte) (view.Identity, error) {
	f.identityCalls.Add(1)

	return f.identity, f.identityErr
}

func (f *fakeEndpointService) GetResolver(context.Context, view.Identity) (*endpoint.Resolver, error) {
	f.resolverCalls.Add(1)

	return f.resolver, f.resolverErr
}

func resolverWith(name, p2pAddress string) *endpoint.Resolver {
	return &endpoint.Resolver{
		Name:      name,
		Addresses: map[endpoint.PortName]string{endpoint.P2PPort: p2pAddress},
	}
}

// --- selection strategies ---

func TestAlwaysFirst(t *testing.T) {
	t.Parallel()
	pick := AlwaysFirst[string]()
	require.Equal(t, "a", pick([]string{"a", "b", "c"}))
	require.Equal(t, "only", pick([]string{"only"}))
}

func TestAlwaysLast(t *testing.T) {
	t.Parallel()
	pick := AlwaysLast[string]()
	require.Equal(t, "c", pick([]string{"a", "b", "c"}))
	require.Equal(t, "only", pick([]string{"only"}))
}

// The counter is incremented before it is used, so the first selection is
// index 1 rather than index 0. Rotation is still even; this pins the sequence
// so a change to the increment order is visible.
func TestRoundRobin(t *testing.T) {
	t.Parallel()
	pick := RoundRobin[string]()
	set := []string{"a", "b", "c"}
	require.Equal(t, []string{"b", "c", "a", "b", "c", "a"}, []string{
		pick(set), pick(set), pick(set), pick(set), pick(set), pick(set),
	})
}

// Each call to RoundRobin returns a closure with its own counter.
func TestRoundRobin_IndependentCounters(t *testing.T) {
	t.Parallel()
	set := []string{"a", "b"}
	first, second := RoundRobin[string](), RoundRobin[string]()
	require.Equal(t, first(set), second(set))
}

func TestRandom(t *testing.T) {
	t.Parallel()
	pick := Random[string]()
	set := []string{"a", "b", "c"}
	seen := map[string]bool{}
	for range 100 {
		got := pick(set)
		require.Contains(t, set, got)
		seen[got] = true
	}
	require.Greater(t, len(seen), 1, "100 draws from 3 values should not all be the same")
}

// --- staticLabelRouter ---

func writeRoutes(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))

	return path
}

func TestNewStaticLabelRouter(t *testing.T) {
	t.Parallel()
	path := writeRoutes(t, `
routes:
  alice:
    - 127.0.0.1:1000
    - 127.0.0.1:1001
  bob:
    - 127.0.0.1:2000
`)
	router, err := newStaticLabelRouter(path)
	require.NoError(t, err)

	addrs, ok := router.Lookup("alice")
	require.True(t, ok)
	require.Equal(t, []host2.PeerIPAddress{"127.0.0.1:1000", "127.0.0.1:1001"}, addrs)

	addrs, ok = router.Lookup("carol")
	require.False(t, ok)
	require.Nil(t, addrs)
}

func TestNewStaticLabelRouter_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := newStaticLabelRouter(filepath.Join(t.TempDir(), "absent.yaml"))
	require.Error(t, err)
}

func TestNewStaticLabelRouter_MalformedYAML(t *testing.T) {
	t.Parallel()
	_, err := newStaticLabelRouter(writeRoutes(t, "routes: [this is not a map"))
	require.Error(t, err)
}

// --- serviceDiscovery ---

type stubIDRouter struct {
	addrs []host2.PeerIPAddress
	ok    bool
}

func (s stubIDRouter) Lookup(host2.PeerID) ([]host2.PeerIPAddress, bool) {
	return s.addrs, s.ok
}

func TestServiceDiscovery_LookupAll(t *testing.T) {
	t.Parallel()
	d := NewServiceDiscovery(stubIDRouter{addrs: []host2.PeerIPAddress{"a", "b"}, ok: true}, AlwaysFirst[host2.PeerIPAddress]())
	addrs, ok := d.LookupAll("peer")
	require.True(t, ok)
	require.Equal(t, []host2.PeerIPAddress{"a", "b"}, addrs)

	d = NewServiceDiscovery(stubIDRouter{ok: false}, AlwaysFirst[host2.PeerIPAddress]())
	_, ok = d.LookupAll("peer")
	require.False(t, ok)
}

func TestServiceDiscovery_Lookup(t *testing.T) {
	t.Parallel()
	d := NewServiceDiscovery(stubIDRouter{addrs: []host2.PeerIPAddress{"a", "b"}, ok: true}, AlwaysLast[host2.PeerIPAddress]())
	require.Equal(t, host2.PeerIPAddress("b"), d.Lookup("peer"))
}

func TestServiceDiscovery_LookupMiss(t *testing.T) {
	t.Parallel()
	d := NewServiceDiscovery(stubIDRouter{ok: false}, AlwaysFirst[host2.PeerIPAddress]())
	require.Equal(t, host2.PeerIPAddress(""), d.Lookup("peer"))
}

// A router can report success with no addresses; the strategy would panic on
// an empty slice, so Lookup guards on the length.
func TestServiceDiscovery_LookupEmptySet(t *testing.T) {
	t.Parallel()
	d := NewServiceDiscovery(stubIDRouter{addrs: []host2.PeerIPAddress{}, ok: true}, AlwaysFirst[host2.PeerIPAddress]())
	require.Equal(t, host2.PeerIPAddress(""), d.Lookup("peer"))
}

// --- EndpointServiceIDRouter ---

func TestEndpointServiceIDRouter_Lookup(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "127.0.0.1:9000")}
	addrs, ok := NewEndpointServiceIDRouter(es).Lookup("peer")
	require.True(t, ok)
	require.Equal(t, []host2.PeerIPAddress{"127.0.0.1:9000"}, addrs)
}

func TestEndpointServiceIDRouter_IdentityFails(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identityErr: errors.New("no identity")}
	addrs, ok := NewEndpointServiceIDRouter(es).LookupWithContext(t.Context(), "peer")
	require.False(t, ok)
	require.Empty(t, addrs)
	require.Equal(t, int32(0), es.resolverCalls.Load(), "must not resolve when the identity lookup failed")
}

func TestEndpointServiceIDRouter_ResolverFails(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identity: view.Identity("id"), resolverErr: errors.New("no resolver")}
	addrs, ok := NewEndpointServiceIDRouter(es).LookupWithContext(t.Context(), "peer")
	require.False(t, ok)
	require.Empty(t, addrs)
}

// A resolver with no P2P address is not a usable route.
func TestEndpointServiceIDRouter_NoP2PAddress(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "")}
	addrs, ok := NewEndpointServiceIDRouter(es).LookupWithContext(t.Context(), "peer")
	require.False(t, ok)
	require.Empty(t, addrs)
}

// --- StaticIDRouter ---

func TestStaticIDRouter(t *testing.T) {
	t.Parallel()
	r := StaticIDRouter{
		"alice": {"127.0.0.1:1000", "127.0.0.1:1001"},
		"bob":   {"127.0.0.1:2000"},
	}

	addrs, ok := r.Lookup("alice")
	require.True(t, ok)
	require.Len(t, addrs, 2)

	_, ok = r.Lookup("carol")
	require.False(t, ok)
}

func TestStaticIDRouter_ReverseLookup(t *testing.T) {
	t.Parallel()
	r := StaticIDRouter{"alice": {"127.0.0.1:1000", "127.0.0.1:1001"}}

	// Any of a peer's addresses maps back to it.
	id, ok := r.ReverseLookup("127.0.0.1:1001")
	require.True(t, ok)
	require.Equal(t, host2.PeerID("alice"), id)

	id, ok = r.ReverseLookup("127.0.0.1:9999")
	require.False(t, ok)
	require.Empty(t, id)
}

// --- LabelResolver and ResolvedStaticIDRouter ---

func TestGetLabel_StripsFSCPrefix(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "")}
	label, err := newLabelResolver(es).getLabel(t.Context(), "peer")
	require.NoError(t, err)
	require.Equal(t, "alice", label)
}

func TestGetLabel_Caches(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "")}
	r := newLabelResolver(es)

	for range 3 {
		label, err := r.getLabel(t.Context(), "peer")
		require.NoError(t, err)
		require.Equal(t, "alice", label)
	}
	require.Equal(t, int32(1), es.identityCalls.Load(), "the label should be resolved once and cached")
}

// getLabel double-checks the cache under a read lock and then a write lock.
// Hammering it from several goroutines gives -race something to inspect and
// pins that the resolve happens once between them.
func TestGetLabel_ConcurrentCallersResolveOnce(t *testing.T) {
	t.Parallel()

	const callers = 16

	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "")}
	r := newLabelResolver(es)

	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			label, err := r.getLabel(t.Context(), "peer")
			assert.NoError(t, err)
			assert.Equal(t, "alice", label)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), es.identityCalls.Load(),
		"concurrent callers must resolve the label once between them")
}

func TestGetLabel_ResolverFails(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identity: view.Identity("id"), resolverErr: errors.New("no resolver")}
	_, err := newLabelResolver(es).getLabel(t.Context(), "peer")
	require.Error(t, err)
}

// getLabel only treats a GetIdentity failure as fatal when the identity is
// also nil. The sibling EndpointServiceIDRouter.LookupWithContext, which makes
// the same call, returns on err alone.
func TestGetLabel_IdentityFails(t *testing.T) {
	t.Parallel()
	es := &fakeEndpointService{identityErr: errors.New("no identity")}
	_, err := newLabelResolver(es).getLabel(t.Context(), "peer")
	require.Error(t, err)
	require.Equal(t, int32(0), es.resolverCalls.Load(), "must not resolve when there is no identity")
}

func TestResolvedStaticIDRouter(t *testing.T) {
	t.Parallel()
	path := writeRoutes(t, "routes:\n  alice:\n    - 127.0.0.1:1000\n")
	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "")}

	r, err := NewResolvedStaticIDRouter(path, es)
	require.NoError(t, err)

	addrs, ok := r.Lookup("peer")
	require.True(t, ok)
	require.Equal(t, []host2.PeerIPAddress{"127.0.0.1:1000"}, addrs)
}

// The label resolves but no route is configured for it.
func TestResolvedStaticIDRouter_UnknownLabel(t *testing.T) {
	t.Parallel()
	path := writeRoutes(t, "routes:\n  bob:\n    - 127.0.0.1:2000\n")
	es := &fakeEndpointService{identity: view.Identity("id"), resolver: resolverWith("fsc.alice", "")}

	r, err := NewResolvedStaticIDRouter(path, es)
	require.NoError(t, err)

	_, ok := r.Lookup("peer")
	require.False(t, ok)
}

func TestResolvedStaticIDRouter_BadConfig(t *testing.T) {
	t.Parallel()
	_, err := NewResolvedStaticIDRouter(filepath.Join(t.TempDir(), "absent.yaml"), &fakeEndpointService{})
	require.Error(t, err)
}

func TestResolvedStaticIDRouter_LabelLookupFails(t *testing.T) {
	t.Parallel()
	path := writeRoutes(t, "routes:\n  alice:\n    - 127.0.0.1:1000\n")
	es := &fakeEndpointService{identityErr: errors.New("no identity")}

	r, err := NewResolvedStaticIDRouter(path, es)
	require.NoError(t, err)

	addrs, ok := r.Lookup("peer")
	require.False(t, ok)
	require.Nil(t, addrs)
}
