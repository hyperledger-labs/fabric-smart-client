/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package tlsconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeServerChildOverridesOnlyWhatItSets(t *testing.T) {
	t.Parallel()

	parent := ServerTLS{
		Enabled:            new(true),
		Cert:               &File{File: "parent.crt"},
		Key:                &File{File: "parent.key"},
		ClientAuthRequired: new(true),
	}
	child := ServerTLS{ClientAuthRequired: new(false)}

	got := mergeServer(parent, child)

	require.True(t, *got.Enabled, "enabled must fall through from the parent")
	require.Equal(t, "parent.crt", got.Cert.File)
	require.False(t, *got.ClientAuthRequired, "explicit false must beat an inherited true")
}

func TestMergeServerChildEmptyListBeatsNonEmptyParent(t *testing.T) {
	t.Parallel()

	parent := ServerTLS{ClientRootCAs: &Files{Files: []string{"a.crt"}}}
	child := ServerTLS{ClientRootCAs: &Files{Files: []string{}}}

	got := mergeServer(parent, child)

	require.Empty(t, got.ClientRootCAs.Files, "a set-but-empty child list must win")
}

func TestMergeClientChildOverridesOnlyWhatItSets(t *testing.T) {
	t.Parallel()

	parent := ClientTLS{
		Enabled:            new(true),
		RootCAs:            &Files{Files: []string{"ca.crt"}},
		ClientCert:         &File{File: "c.crt"},
		ClientKey:          &File{File: "c.key"},
		ServerNameOverride: new("orderer.example.com"),
	}
	child := ClientTLS{ClientAuthEnabled: new(false)}

	got := mergeClient(parent, child)

	require.True(t, *got.Enabled)
	require.Equal(t, []string{"ca.crt"}, got.RootCAs.Files)
	require.Equal(t, "orderer.example.com", *got.ServerNameOverride)
	require.False(t, *got.ClientAuthEnabled, "explicit false suppresses inherited creds")
}
