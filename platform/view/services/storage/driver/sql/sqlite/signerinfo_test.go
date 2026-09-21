/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestDriverSignerInfo round-trips signers through a store built with NewDriver: only
// signers that were put are reported as existing, and putting one twice is not an error.
func TestDriverSignerInfo(t *testing.T) {
	t.Parallel()

	d := NewDriver(driverConfig(Config{DataSource: tempDataSource(t, "signerinfo")}))
	s, err := d.NewSignerInfo("")
	require.NoError(t, err)

	alice, bob := view.Identity("alice"), view.Identity("bob")

	require.NoError(t, s.PutSigner(t.Context(), alice))
	require.NoError(t, s.PutSigner(t.Context(), alice), "putting an existing signer is not an error")

	existing, err := s.FilterExistingSigners(t.Context(), alice, bob)
	require.NoError(t, err)
	assert.Equal(t, []view.Identity{alice}, existing)
}
