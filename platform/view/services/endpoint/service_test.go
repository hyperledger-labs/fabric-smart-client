/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endpoint_test

import (
	"sync"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/endpoint/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPKIResolveConcurrency(t *testing.T) {
	bindingStore := &mock.BindingStore{}
	bindingStore.GetLongTermReturns(nil, nil)
	bindingStore.HaveSameBindingReturns(false, nil)
	bindingStore.PutBindingsReturns(nil)

	svc, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)

	extractor := &mock.PublicKeyExtractor{}
	extractor.ExtractPublicKeyReturns([]byte("id"), nil)
	resolver := &endpoint.Resolver{}

	err = svc.AddPublicKeyExtractor(extractor)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			svc.PkiResolve(resolver)
		}()
	}
	wg.Wait()
}

func TestGetIdentity(t *testing.T) {
	// setup
	bindingStore := &mock.BindingStore{}
	bindingStore.PutBindingsReturns(nil)

	service, err := endpoint.NewService(bindingStore)
	require.NoError(t, err)

	extractor := &mock.PublicKeyExtractor{}
	extractor.ExtractPublicKeyReturns([]byte("id"), nil)

	_, err = service.AddResolver(
		"alice",
		"fsc.domain",
		map[string]string{string(endpoint.P2PPort): "localhost:1010"},
		[]string{"apple", "strawberry"},
		[]byte("alice_id"),
	)
	require.NoError(t, err)
	resolvers := service.Resolvers()
	require.Len(t, resolvers, 1)
	require.Equal(t, 0, bindingStore.PutBindingsCallCount())

	_, err = service.AddResolver(
		"alice",
		"fabric.domain",
		map[string]string{string(endpoint.P2PPort): "localhost:1010"},
		[]string{"apricot"},
		[]byte("alice_id2"),
	)
	require.NoError(t, err)
	resolvers = service.Resolvers()
	require.Len(t, resolvers, 1)
	require.Equal(t, 1, bindingStore.PutBindingsCallCount())

	err = service.AddPublicKeyExtractor(extractor)
	require.NoError(t, err)

	// found
	for _, label := range []string{
		"alice",
		"alice.fsc.domain",
		"apple",
		"strawberry",
		"localhost:1010",
		"[::1]:1010",
		"alice_id",
	} {
		resultID, err := service.GetIdentity(label, []byte{})
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), []byte(resultID))

		resultID, _, _, err = service.Resolve(t.Context(), view.Identity(label))
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), []byte(resultID))

		resolver, _, err := service.Resolver(t.Context(), view.Identity(label))
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), resolver.ID)

		resolver, err = service.GetResolver(t.Context(), view.Identity(label))
		require.NoError(t, err)
		require.Equal(t, []byte("alice_id"), resolver.ID)
	}

	// not found
	for _, label := range []string{
		"alice.fabric.domain",
		"pineapple",
		"bob",
		"apricot",
		"localhost:8080",
		"alice_id2",
	} {
		resultID, err := service.GetIdentity(label, []byte("no"))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)
		require.Equal(t, []byte(nil), []byte(resultID))

		_, _, _, err = service.Resolve(t.Context(), view.Identity(label))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)

		_, _, err = service.Resolver(t.Context(), view.Identity(label))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)

		_, err = service.GetResolver(t.Context(), view.Identity(label))
		require.Error(t, err)
		require.ErrorIs(t, err, endpoint.ErrNotFound)
	}

	for _, label := range []string{
		"alice",
		"alice.fsc.domain",
		"apple",
		"strawberry",
		"localhost:1010",
		"[::1]:1010",
		"alice_id",
	} {
		ok, err := service.RemoveResolver(view.Identity(label))
		require.NoError(t, err)
		require.True(t, ok)

		resolvers = service.Resolvers()
		require.Empty(t, resolvers)

		// add again
		_, err = service.AddResolver(
			"alice",
			"fsc.domain",
			map[string]string{string(endpoint.P2PPort): "localhost:1010"},
			[]string{"apple", "strawberry"},
			[]byte("alice_id"),
		)
		require.NoError(t, err)
	}

	// remove something that does not exist
	ok, err := service.RemoveResolver(view.Identity("bob"))
	require.Error(t, err)
	require.ErrorIs(t, err, endpoint.ErrNotFound)
	require.False(t, ok)
}

func TestLookupIP(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		// We can't predict exact resolved IPs, but we can verify format
		checkFunc func(t *testing.T, input, output string)
	}{
		// IPv4 addresses
		{
			name:     "IPv4 with port",
			endpoint: "192.168.1.1:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv4 address should be returned as-is")
			},
		},
		{
			name:     "IPv4 loopback with port",
			endpoint: "127.0.0.1:9000",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv4 loopback should be returned as-is")
			},
		},
		{
			name:     "IPv4 with high port",
			endpoint: "10.0.0.1:65535",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv4 with high port should be returned as-is")
			},
		},

		// IPv6 addresses
		{
			name:     "IPv6 loopback with port",
			endpoint: "[::1]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 loopback should be returned as-is")
			},
		},
		{
			name:     "IPv6 full address with port",
			endpoint: "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:443",
			checkFunc: func(t *testing.T, input, output string) {
				// Go normalizes IPv6 addresses, so the full form gets compressed
				assert.Equal(t, "[2001:db8:85a3::8a2e:370:7334]:443", output,
					"IPv6 full address should be normalized by Go")
			},
		},
		{
			name:     "IPv6 compressed address with port",
			endpoint: "[2001:db8::1]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 compressed address should be returned as-is")
			},
		},
		{
			name:     "IPv6 all zeros compressed with port",
			endpoint: "[::]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 all zeros should be returned as-is")
			},
		},
		{
			name:     "IPv6 link-local with port",
			endpoint: "[fe80::1]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 link-local should be returned as-is")
			},
		},

		// Hostnames (these will actually resolve)
		{
			name:     "localhost with port",
			endpoint: "localhost:8080",
			checkFunc: func(t *testing.T, input, output string) {
				// localhost should resolve to either 127.0.0.1:8080 or [::1]:8080
				assert.Contains(t, []string{"127.0.0.1:8080", "[::1]:8080"}, output,
					"localhost should resolve to loopback address")
			},
		},

		// Malformed inputs (should return as-is)
		{
			name:     "no port",
			endpoint: "192.168.1.1",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "address without port should be returned as-is")
			},
		},
		{
			name:     "IPv6 without brackets",
			endpoint: "::1:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "malformed IPv6 should be returned as-is")
			},
		},
		{
			name:     "empty string",
			endpoint: "",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "empty string should be returned as-is")
			},
		},
		{
			name:     "only port",
			endpoint: ":8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "only port should be returned as-is")
			},
		},
		{
			name:     "invalid port",
			endpoint: "192.168.1.1:invalid",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "invalid port should be returned as-is")
			},
		},
		{
			name:     "multiple colons without brackets",
			endpoint: "2001:db8::1:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 without brackets should be returned as-is")
			},
		},
		{
			name:     "hostname only",
			endpoint: "example.com",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "hostname without port should be returned as-is")
			},
		},

		// Edge cases
		{
			name:     "IPv4 with port 0",
			endpoint: "192.168.1.1:0",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv4 with port 0 should be returned as-is")
			},
		},
		{
			name:     "IPv6 with port 0",
			endpoint: "[::1]:0",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 with port 0 should be returned as-is")
			},
		},
		{
			name:     "IPv4-mapped IPv6 address",
			endpoint: "[::ffff:192.0.2.1]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				// Go converts IPv4-mapped IPv6 addresses to IPv4 format
				assert.Equal(t, "192.0.2.1:8080", output,
					"IPv4-mapped IPv6 should be converted to IPv4 by Go")
			},
		},
		{
			name:     "IPv6 with zone ID",
			endpoint: "[fe80::1%eth0]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				// Zone IDs might not be preserved, so just check it doesn't panic
				assert.NotEmpty(t, output, "IPv6 with zone ID should return something")
			},
		},

		// Unresolvable hostnames (should return as-is)
		{
			name:     "unresolvable hostname",
			endpoint: "this-hostname-does-not-exist-12345.invalid:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "unresolvable hostname should be returned as-is")
			},
		},

		// Special addresses
		{
			name:     "IPv4 broadcast",
			endpoint: "255.255.255.255:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv4 broadcast should be returned as-is")
			},
		},
		{
			name:     "IPv4 any address",
			endpoint: "0.0.0.0:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv4 any address should be returned as-is")
			},
		},
		{
			name:     "IPv6 multicast",
			endpoint: "[ff02::1]:8080",
			checkFunc: func(t *testing.T, input, output string) {
				assert.Equal(t, input, output, "IPv6 multicast should be returned as-is")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := endpoint.LookupIP(tt.endpoint)
			tt.checkFunc(t, tt.endpoint, result)
		})
	}
}

func TestLookupIP_HostnameResolution(t *testing.T) {
	// Test that localhost actually resolves
	result := endpoint.LookupIP("localhost:8080")

	// localhost should resolve to either IPv4 or IPv6 loopback
	validResults := []string{
		"127.0.0.1:8080", // IPv4 loopback
		"[::1]:8080",     // IPv6 loopback
	}

	found := false
	for _, valid := range validResults {
		if result == valid {
			found = true
			break
		}
	}

	assert.True(t, found, "localhost should resolve to a loopback address, got: %s", result)
}

func TestLookupIP_PreservesPort(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantPort string
	}{
		{"IPv4 standard port", "192.168.1.1:8080", "8080"},
		{"IPv4 low port", "192.168.1.1:80", "80"},
		{"IPv4 high port", "192.168.1.1:65535", "65535"},
		{"IPv6 standard port", "[::1]:8080", "8080"},
		{"IPv6 low port", "[::1]:80", "80"},
		{"IPv6 high port", "[::1]:65535", "65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := endpoint.LookupIP(tt.endpoint)
			// Extract port from result
			_, port, err := splitHostPort(result)
			if err == nil {
				assert.Equal(t, tt.wantPort, port, "port should be preserved")
			}
		})
	}
}

// Helper function for tests
func splitHostPort(endpoint string) (host, port string, err error) {
	// Simple implementation for testing
	if len(endpoint) == 0 {
		return "", "", nil
	}

	// Try to find the last colon
	lastColon := -1
	inBrackets := false

	for i, c := range endpoint {
		if c == '[' {
			inBrackets = true
		} else if c == ']' {
			inBrackets = false
		} else if c == ':' && !inBrackets {
			lastColon = i
		}
	}

	if lastColon == -1 {
		return endpoint, "", nil
	}

	host = endpoint[:lastColon]
	port = endpoint[lastColon+1:]

	// Remove brackets from IPv6
	if len(host) > 0 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}

	return host, port, nil
}
