/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package start

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/host/websocket/ws"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

func encoded(t *testing.T, mutate func(p *preamble.Preamble)) []byte {
	t.Helper()
	p := preamble.Local()
	mutate(p)
	raw, err := preamble.Encode(p)
	require.NoError(t, err)
	return raw
}

// Fake peers give the documented exit codes; an unreachable peer wins over an incompatible one.
func TestCheckPeers(t *testing.T) {
	t.Parallel()
	same := preamble.LocalBytes()
	otherEpoch := encoded(t, func(p *preamble.Preamble) { p.FramingEpoch++ })
	disjoint := encoded(t, func(p *preamble.Preamble) {
		p.Ranges[1] = &preamble.ComponentRange{Component: "fabric", Min: 3, Max: 4}
	})
	overlap := encoded(t, func(p *preamble.Preamble) {
		p.Ranges[1] = &preamble.ComponentRange{Component: "fabric", Min: 2, Max: 3}
	})
	withoutView := encoded(t, func(p *preamble.Preamble) { p.Ranges = p.Ranges[1:] })
	withoutFabricx := encoded(t, func(p *preamble.Preamble) { p.Ranges = p.Ranges[:2] })

	for name, tc := range map[string]struct {
		peers map[string][]byte // nil value: no preamble; missing from replies: unreachable
		code  int
		out   string
	}{
		"overlapping":         {map[string][]byte{"a": same, "b": overlap}, 0, "fabric: local [2,2] peer [2,3]: uses 2"},
		"no preamble":         {map[string][]byte{"a": nil}, ExitIncompatible, "no preamble, it runs an FSC release without version ranges\n  view: local [1,1], not announced by the peer, so [1,1]: uses 1\n  fabric: local [2,2], not announced by the peer, so [1,1]: no overlap"},
		"not announced, at 1": {map[string][]byte{"a": withoutView}, 0, "view: local [1,1], not announced by the peer, so [1,1]: uses 1"},
		"not announced":       {map[string][]byte{"a": withoutFabricx}, ExitIncompatible, "fabricx: local [2,2], not announced by the peer, so [1,1]: no overlap"},
		"disjoint":            {map[string][]byte{"a": same, "b": disjoint}, ExitIncompatible, "fabric: local [2,2] peer [3,4]: no overlap"},
		"other epoch":         {map[string][]byte{"a": otherEpoch}, ExitIncompatible, "peer framing epoch 2, local 1"},
		"unreachable":         {map[string][]byte{"a": same, "down": nil}, ExitFailed, "peer [down] at [down:1]: unreachable"},
		"down and disjoint":   {map[string][]byte{"down": nil, "b": disjoint}, ExitFailed, "unreachable"},
		"bad preamble header": {map[string][]byte{"garbled": nil}, ExitIncompatible, "garbled:1]: incompatible"},
		"no peers":            {nil, 0, "no peers configured"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var peers []peer
			for n := range tc.peers {
				peers = append(peers, peer{name: n, id: n, address: n + ":1"})
			}
			dial := func(_ context.Context, id, address string) ([]byte, error) {
				require.Equal(t, id+":1", address)
				switch id {
				case "down":
					return nil, errors.New("connection refused")
				case "garbled":
					return nil, errors.Join(ws.ErrBadPreamble, errors.New("version: malformed preamble header"))
				}
				return tc.peers[id], nil
			}
			var out bytes.Buffer
			require.Equal(t, tc.code, checkPeers(t.Context(), &out, peers, dial), out.String())
			require.Contains(t, out.String(), tc.out)
		})
	}
}

// TestCheckPeersCountsUnannouncedAsVersionOne checks that a peer without a preamble, which
// supports version 1 of every component, is incompatible with a binary whose range excludes 1.
// It changes the process-wide components, so it runs alone and restores them.
func TestCheckPeersCountsUnannouncedAsVersionOne(t *testing.T) { //nolint:paralleltest
	t.Cleanup(func() { require.NoError(t, preamble.SetComponents(preamble.FSCComponents)) })
	require.NoError(t, preamble.SetComponents([]preamble.Component{{Name: "view", Range: preamble.Range{Min: 1, Max: 1}}, {Name: "fabric", Range: preamble.Range{Min: 2, Max: 2}}}))
	noPreamble := func(context.Context, string, string) ([]byte, error) { return nil, nil }

	var out bytes.Buffer
	require.Equal(t, ExitIncompatible, checkPeers(t.Context(), &out, []peer{{name: "old", id: "old", address: "old:1"}}, noPreamble), out.String())
	require.Contains(t, out.String(), "fabric: local [2,2], not announced by the peer, so [1,1]: no overlap")
}

type configuredNode struct {
	*mockStartNode
	cfg driver.ConfigService
	err error
}

func (n *configuredNode) Config() (driver.ConfigService, error) { return n.cfg, n.err }

func rawConfig(t *testing.T, yaml string) driver.ConfigService {
	t.Helper()
	cfg, err := (&config.Provider{}).ProvideFromRaw([]byte(yaml))
	require.NoError(t, err)
	return cfg
}

// Invalid arguments exit 2 and never read the configuration, so nothing is dialed; a missing
// configuration or an unsupported transport exits 1.
func TestPeersCmd(t *testing.T) {
	t.Parallel()
	run := func(n Configured, args ...string) error {
		cmd := Cmd(&struct {
			*mockStartNode
			Configured
		}{newMockStartNode(nil), n})
		cmd.SetArgs(append([]string{"peers"}, args...))
		cmd.SetOut(&bytes.Buffer{})
		return cmd.Execute()
	}
	code := func(err error) int {
		var e *ExitError
		require.ErrorAs(t, err, &e)
		return e.Code
	}

	poisoned := &configuredNode{err: errors.New("must not be read")}
	for _, args := range [][]string{{}, {"--check", "extra"}, {"--check", "--force"}, {"--check=maybe"}} {
		require.Equal(t, ExitUsage, code(run(poisoned, args...)), "%v", args)
	}

	require.Equal(t, ExitFailed, code(run(&configuredNode{err: config.ErrNotFound}, "--check")))
	err := run(&configuredNode{cfg: rawConfig(t, "fsc:\n  p2p:\n    type: libp2p\n")}, "--check")
	require.Equal(t, ExitFailed, code(err))
	require.ErrorContains(t, err, "fsc.p2p.type [libp2p] is not supported")

	require.Len(t, Cmd(newMockStartNode(nil)).Commands(), 1, "a node without Config gets no peers command")
}

// configuredPeers skips the node itself, resolvers without a P2P address and later entries of a
// name, as the node does, trusts each resolver's self-signed identity, also of resolvers it does
// not dial, and its dialer reaches a peer only if the peer's trusted TLS certificate is that
// resolver's identity.
func TestConfiguredPeers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name string, raw []byte) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0o600))
	}
	newIdentity := func(name string) tls.Certificate {
		cert, key, err := websocket.GenerateTestCert(name)
		require.NoError(t, err)
		write(name+".crt", cert)
		write(name+".key", key)
		pair, err := tls.X509KeyPair(cert, key)
		require.NoError(t, err)
		return pair
	}
	newIdentity("self")
	responder := newIdentity("responder")
	impostor := newIdentity("impostor")
	newIdentity("viewonly")

	serve := func(cert tls.Certificate) string {
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if conn, err := (&gwebsocket.Upgrader{}).Upgrade(w, r, nil); err == nil {
				_ = conn.Close()
			}
		}))
		srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAnyClientCert, MinVersion: tls.VersionTLS13}
		srv.StartTLS()
		t.Cleanup(srv.Close)
		return strings.TrimPrefix(srv.URL, "https://")
	}
	good, bad := serve(responder), serve(impostor)

	write("core.yaml", []byte(`
fsc:
  identity:
    cert: {file: self.crt}
    key: {file: self.key}
  p2p:
    type: websocket
    listenAddress: /ip4/127.0.0.1/tcp/9000
    opts:
      websocket:
        tls:
          enabled: true
          clientRootCAs: {files: [self.crt]}
  endpoint:
    resolvers:
    - {name: self, identity: {path: self.crt}, addresses: {P2P: "127.0.0.1:1"}}
    - {name: viewonly, identity: {path: viewonly.crt}, addresses: {View: "127.0.0.1:2"}}
    - {name: responder, identity: {path: responder.crt}, addresses: {P2P: "`+good+`"}}
    - {name: impostor, identity: {path: responder.crt}, addresses: {P2P: "`+bad+`"}}
    - {name: trusted, identity: {path: impostor.crt}}
    - {name: responder, identity: {path: impostor.crt}, addresses: {P2P: "127.0.0.1:3"}}
`))
	cfg, err := config.NewProvider(dir)
	require.NoError(t, err)
	peers, dial, err := configuredPeers(cfg)
	require.NoError(t, err)
	require.Len(t, peers, 2)
	require.Equal(t, "responder", peers[0].name)
	require.Equal(t, "impostor", peers[1].name)

	raw, err := dial(t.Context(), peers[0].id, peers[0].address)
	require.NoError(t, err)
	require.Nil(t, raw, "the test server sends no preamble")
	_, err = dial(t.Context(), peers[1].id, peers[1].address)
	require.ErrorContains(t, err, "peer identity binding failed")

	var out bytes.Buffer
	require.Equal(t, ExitFailed, checkPeers(t.Context(), &out, peers, dial), out.String())
}

// node peers --check sends fsc.p2p.opts.websocket.tls.serverNameOverride as SNI, and the host of
// the dialed address when it is unset.
func TestConfiguredPeersSendServerNameOverride(t *testing.T) {
	t.Parallel()
	for override, want := range map[string]string{"": "localhost", "peer.example.com": "peer.example.com"} {
		t.Run("override="+override, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			identity := func(name string) tls.Certificate {
				cert, key, err := websocket.GenerateTestCert(name)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, name+".crt"), cert, 0o600))
				require.NoError(t, os.WriteFile(filepath.Join(dir, name+".key"), key, 0o600))
				pair, err := tls.X509KeyPair(cert, key)
				require.NoError(t, err)
				return pair
			}
			identity("self")
			responder := identity("responder")
			sni := make(chan string, 1)
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if conn, err := (&gwebsocket.Upgrader{}).Upgrade(w, r, nil); err == nil {
					_ = conn.Close()
				}
			}))
			srv.TLS = &tls.Config{
				Certificates: []tls.Certificate{responder},
				ClientAuth:   tls.RequireAnyClientCert,
				MinVersion:   tls.VersionTLS13,
				GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
					select {
					case sni <- hello.ServerName:
					default:
					}
					return nil, nil
				},
			}
			srv.StartTLS()
			t.Cleanup(srv.Close)
			// Dial by name: crypto/tls sends no SNI for an IP address.
			_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
			require.NoError(t, err)

			tlsBlock := "          enabled: true\n          clientRootCAs: {files: [self.crt]}\n"
			if override != "" {
				tlsBlock += "          serverNameOverride: " + override + "\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "core.yaml"), []byte(`
fsc:
  identity:
    cert: {file: self.crt}
    key: {file: self.key}
  p2p:
    type: websocket
    listenAddress: /ip4/127.0.0.1/tcp/9000
    opts:
      websocket:
        tls:
`+tlsBlock+`  endpoint:
    resolvers:
    - {name: responder, identity: {path: responder.crt}, addresses: {P2P: "localhost:`+port+`"}}
`), 0o600))
			cfg, err := config.NewProvider(dir)
			require.NoError(t, err)
			peers, dial, err := configuredPeers(cfg)
			require.NoError(t, err)
			require.Len(t, peers, 1)

			_, err = dial(t.Context(), peers[0].id, peers[0].address)
			require.NoError(t, err)
			require.Equal(t, want, <-sni)
		})
	}
}

// configuredPeers refuses the resolver entries the node's resolver loader refuses. Address keys
// that differ only in case already fail when the configuration loads.
func TestConfiguredPeersRefusesBadResolvers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cert, key, err := websocket.GenerateTestCert("self")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "self.crt"), cert, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "self.key"), key, 0o600))
	crt := filepath.Join(dir, "self.crt")
	for entry, want := range map[string]string{
		`{identity: {path: ` + crt + `}, addresses: {P2P: "h:1"}}`:                "fsc.endpoint.resolvers[0]: name is required",
		`{name: a, addresses: {P2P: "h:1"}}`:                                      "identity.path is required",
		`{name: a, identity: {path: ` + crt + `}, addresses: {P2PAdress: "h:1"}}`: "unknown address key [p2padress]",
	} {
		cfg := rawConfig(t, `
fsc:
  identity:
    cert: {file: `+filepath.Join(dir, "self.crt")+`}
    key: {file: `+filepath.Join(dir, "self.key")+`}
  p2p:
    type: websocket
    listenAddress: /ip4/127.0.0.1/tcp/9000
    opts:
      websocket:
        tls:
          enabled: true
          clientRootCAs: {files: [`+filepath.Join(dir, "self.crt")+`]}
  endpoint:
    resolvers:
    - `+entry+`
`)
		_, _, err := configuredPeers(cfg)
		require.ErrorContains(t, err, want, entry)
	}
}
