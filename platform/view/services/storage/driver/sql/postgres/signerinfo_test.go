/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testing2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// TestDriverSignerInfoPostgres round-trips signers through a store whose schema was
// created on a real Postgres: only signers that were put are reported as existing, and
// putting one twice is not an error, which relies on ErrorMapper turning the
// primary-key violation into driver.UniqueKeyViolation.
func TestDriverSignerInfoPostgres(t *testing.T) {
	t.Parallel()

	cp := NewConfigProvider(testing2.MockConfig(Config{DataSource: setupDB(t)}))
	s, err := NewPersistenceWithOpts(cp, NewDbProvider(), "", NewSignerInfoStore)
	require.NoError(t, err)

	alice, bob := view.Identity("alice"), view.Identity("bob")

	require.NoError(t, s.PutSigner(t.Context(), alice))
	require.NoError(t, s.PutSigner(t.Context(), alice), "putting an existing signer is not an error")

	existing, err := s.FilterExistingSigners(t.Context(), alice, bob)
	require.NoError(t, err)
	assert.Equal(t, []view.Identity{alice}, existing)
}
