/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// TestKeyDataStoresPostgres runs common.TestKeyDataStore for each key-data store
// on a schema created on a real Postgres. The envelope and
// endorsement-transaction stores are write-once; the metadata store keeps the
// first value.
func TestKeyDataStoresPostgres(t *testing.T) {
	t.Parallel()

	terminate, pgConnStr, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)

	cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: pgConnStr}))
	dbProvider := postgres.NewDbProvider()

	for name, tc := range map[string]struct {
		open   func(*testing.T) common.KeyDataStore
		strict bool
	}{
		"Envelope": {strict: true, open: func(t *testing.T) common.KeyDataStore {
			t.Helper()
			s, err := NewPersistenceWithOpts(cp, dbProvider, "", NewEnvelopeStore)
			require.NoError(t, err)
			return common.KeyDataStore{Get: s.GetEnvelope, Exists: s.ExistsEnvelope, Put: s.PutEnvelope}
		}},
		"EndorseTx": {strict: true, open: func(t *testing.T) common.KeyDataStore {
			t.Helper()
			s, err := NewPersistenceWithOpts(cp, dbProvider, "", NewEndorseTxStore)
			require.NoError(t, err)
			return common.KeyDataStore{Get: s.GetEndorseTx, Exists: s.ExistsEndorseTx, Put: s.PutEndorseTx}
		}},
		"Metadata": {open: func(t *testing.T) common.KeyDataStore {
			t.Helper()
			s, err := NewPersistenceWithOpts(cp, dbProvider, "", NewMetadataStore)
			require.NoError(t, err)
			return common.KeyDataStore{Get: s.GetMetadata, Exists: s.ExistMetadata, Put: s.PutMetadata}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			common.TestKeyDataStore(t, tc.open(t), tc.strict)
		})
	}
}
