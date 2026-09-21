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

// TestDriverAuditInfo round-trips audit info through a store built with NewDriver.
func TestDriverAuditInfo(t *testing.T) {
	t.Parallel()

	d := NewDriver(driverConfig(Config{DataSource: tempDataSource(t, "auditinfo")}))
	s, err := d.NewAuditInfo("")
	require.NoError(t, err)

	alice := view.Identity("alice")
	info := []byte("audit info")

	require.NoError(t, s.PutAuditInfo(t.Context(), alice, info))

	got, err := s.GetAuditInfo(t.Context(), alice)
	require.NoError(t, err)
	assert.Equal(t, info, got)

	unknown, err := s.GetAuditInfo(t.Context(), view.Identity("bob"))
	require.NoError(t, err)
	assert.Empty(t, unknown, "an identity with no audit info returns nothing")
}
