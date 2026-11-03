/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	fsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// TestKeyDataStoresSQLite runs common.TestKeyDataStore for each key-data store
// on a SQLite file database. The envelope and endorsement-transaction stores are
// write-once; the metadata store keeps the first value.
func TestKeyDataStoresSQLite(t *testing.T) {
	t.Parallel()

	dataSource := "file:" + filepath.Join(t.TempDir(), "stores.sqlite") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: dataSource}))
	dbProvider := sqlite.NewDbProvider()

	for name, tc := range map[string]struct {
		open   func(*testing.T) common.KeyDataStore
		strict bool
	}{
		"Envelope": {strict: true, open: func(t *testing.T) common.KeyDataStore {
			t.Helper()
			s, err := fsqlite.NewPersistenceWithOpts(cp, dbProvider, "", fsqlite.NewEnvelopeStore)
			require.NoError(t, err)
			return common.KeyDataStore{Get: s.GetEnvelope, Exists: s.ExistsEnvelope, Put: s.PutEnvelope}
		}},
		"EndorseTx": {strict: true, open: func(t *testing.T) common.KeyDataStore {
			t.Helper()
			s, err := fsqlite.NewPersistenceWithOpts(cp, dbProvider, "", fsqlite.NewEndorseTxStore)
			require.NoError(t, err)
			return common.KeyDataStore{Get: s.GetEndorseTx, Exists: s.ExistsEndorseTx, Put: s.PutEndorseTx}
		}},
		"Metadata": {open: func(t *testing.T) common.KeyDataStore {
			t.Helper()
			s, err := fsqlite.NewPersistenceWithOpts(cp, dbProvider, "", fsqlite.NewMetadataStore)
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
