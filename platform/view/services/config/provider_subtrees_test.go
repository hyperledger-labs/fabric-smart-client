/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const endpointsYAML = `
fabric:
  mynet:
    orderers:
      - address: o1:7050
        tls:
          serverNameOverride: o1
      - address: o2:7050
`

func TestRawSubtrees(t *testing.T) {
	t.Parallel()

	p, err := (&Provider{}).ProvideFromRaw([]byte(endpointsYAML))
	require.NoError(t, err)

	subs := p.RawSubtrees("fabric.mynet.orderers")
	require.Len(t, subs, 2)
	require.Equal(t, "o1:7050", subs[0]["address"])
	require.Equal(t, map[string]any{"servernameoverride": "o1"}, subs[0]["tls"])
	require.NotContains(t, subs[1], "tls", "an entry without a tls block yields none")

	// An indexed path cannot reach into a slice, which is the whole reason RawSubtrees
	// exists: koanf flattens maps but not slice elements.
	_, ok := p.RawSubtree("fabric.mynet.orderers.0.tls")
	require.False(t, ok)

	require.Empty(t, p.RawSubtrees("fabric.mynet.peers"), "an absent array yields nothing")
	require.Empty(t, p.RawSubtrees("fabric.mynet"), "a map is not an array of maps")
}
