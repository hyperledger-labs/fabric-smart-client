/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pingpong_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	nwoclient "github.com/hyperledger-labs/fabric-smart-client/integration/nwo/client"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/config"
)

// fixtureNodeDirs are the checked-in nodes; both are shaped to exercise inheritance.
var fixtureNodeDirs = []string{
	"./testdata/fsc/nodes/initiator.0",
	"./testdata/fsc/nodes/responder.0",
}

// The checked-in fixtures exist to exercise per-field TLS inheritance: fsc.tls present,
// fsc.grpc.tls absent entirely, fsc.web.tls a partial override. Starting a node from them
// would still succeed if someone restated the keypair per listener, so the shape needs its
// own assertion — this is the only check that fails when the fixtures stop covering
// inheritance.
func TestFixturesExerciseTLSInheritance(t *testing.T) {
	t.Parallel()
	for _, dir := range fixtureNodeDirs {
		p, err := config.NewProvider(dir)
		require.NoError(t, err, dir)

		parent, ok := p.RawSubtree("fsc.tls")
		require.True(t, ok, "%s: fsc.tls parent must be present", dir)
		require.Equal(t, true, parent["enabled"])
		require.Contains(t, parent, "cert")
		require.Contains(t, parent, "key")

		_, ok = p.RawSubtree("fsc.grpc.tls")
		require.False(t, ok, "%s: fsc.grpc.tls must be absent, inheriting every field", dir)

		web, ok := p.RawSubtree("fsc.web.tls")
		require.True(t, ok, "%s: fsc.web.tls must be a partial override", dir)
		require.Contains(t, web, "clientrootcas")
		require.NotContains(t, web, "cert", "%s: web must inherit the keypair", dir)

		// The trap this fixture exists to hold open: the leaf key is absent, so any code
		// reading it directly sees false. Only resolution through tlsconfig inherits.
		require.False(t, p.GetBool("fsc.web.tls.enabled"),
			"%s: the leaf must be absent, or this fixture stops testing inheritance", dir)
		require.False(t, p.GetBool("fsc.grpc.tls.enabled"), dir)
	}
}

// The web client must see TLS as enabled even though the fixtures inherit
// fsc.web.tls.enabled from fsc.tls. Reading that key directly returns false for an
// inheriting node, and the client then dials ws:// against a TLS listener and hangs on a
// bad handshake — which is exactly how this broke once.
func TestWebClientConfigSeesInheritedTLS(t *testing.T) {
	t.Parallel()
	for _, dir := range fixtureNodeDirs {
		cfg, err := nwoclient.NewWebClientConfigFromFSC(dir)
		require.NoError(t, err, dir)
		require.NotEmpty(t, cfg.CACertRaw, "%s: TLS must be detected through inheritance", dir)
		require.NotEmpty(t, cfg.TLSCertRaw, dir)
		require.NotEmpty(t, cfg.TLSKeyRaw, dir)
		require.True(t, strings.HasPrefix(cfg.WsURL(), "wss://"),
			"%s: must dial wss, got %s", dir, cfg.WsURL())
	}
}
