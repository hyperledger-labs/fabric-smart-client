/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/preamble"
)

// TestInstallComponents changes process-wide state, so it runs alone and restores the default.
func TestInstallComponents(t *testing.T) { //nolint:paralleltest
	t.Cleanup(func() { require.NoError(t, preamble.SetComponents(preamble.FSCComponents)) })

	// The view SDK and an application SDK wrapping it provide into the same container.
	c := NewContainer()
	require.NoError(t, c.Provide(NewComponentEntry("view", 1, 1)))
	require.NoError(t, c.Provide(NewComponentEntry("iou", 1, 2)))
	require.NoError(t, installComponents(c))
	r, ok := preamble.LocalRange("iou")
	require.True(t, ok, "an entry of a nested SDK is collected")
	require.Equal(t, preamble.Range{Min: 1, Max: 2}, r)

	require.NoError(t, c.Provide(NewComponentEntry("iou", 2, 3)))
	err := installComponents(c)
	require.ErrorContains(t, err, "component [iou] registered twice")
	require.ErrorContains(t, err, "[1, 2]", "the error names both ranges")
	require.ErrorContains(t, err, "[2, 3]")
}
