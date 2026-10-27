/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestDriverAuditInfoPostgres round-trips audit info through a store built with NewDriver
// on a real Postgres. A second put for the same identity is not an error and
// keeps the first value, which relies on ErrorMapper turning the primary-key violation
// into driver.UniqueKeyViolation.
func TestDriverAuditInfoPostgres(t *testing.T) {
	t.Parallel()

	d := NewDriver(multiplexed.MockTypeConfig(Persistence, Config{DataSource: setupDB(t)}))
	s, err := d.NewAuditInfo("")
	require.NoError(t, err)

	alice := view.Identity("alice")
	// Not valid UTF-8 and containing a NUL, so a column that is not BYTEA rejects it.
	info := []byte{0x00, 0xff, 'a'}

	require.NoError(t, s.PutAuditInfo(t.Context(), alice, info))
	require.NoError(t, s.PutAuditInfo(t.Context(), alice, []byte("other")), "putting existing audit info is not an error")

	got, err := s.GetAuditInfo(t.Context(), alice)
	require.NoError(t, err)
	assert.Equal(t, info, got, "the first put wins")

	unknown, err := s.GetAuditInfo(t.Context(), view.Identity("bob"))
	require.NoError(t, err)
	assert.Empty(t, unknown, "an identity with no audit info returns nothing")
}
