/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package configtest_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/configtest"
)

// TestReferenceSectionsOwned fails when docs/configuration.md documents a top-level section
// that no platform reference test calls RequireComplete on. Each listed section names the
// tests that check it.
func TestReferenceSectionsOwned(t *testing.T) {
	t.Parallel()
	owned := []string{
		"fabric",  // platform/fabric/core/generic/config, platform/fabricx/core/committer/config
		"fsc",     // platform/view/services/config, platform/view/sdk/dig/support/endpoint
		"logging", // platform/view/services/config
	}
	r := configtest.Load(t)
	require.Equal(t, owned, slices.Sorted(maps.Keys(r.Backend.Raw())))
}
