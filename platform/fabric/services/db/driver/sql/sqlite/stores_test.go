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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
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

// openEndorseTxContract opens an endorse-tx store, through the driver's constructor and
// Migrate, on a SQLite table that exists before the store is versioned, so that its contract
// step is held back until the returned Contract runs it.
func openEndorseTxContract(t *testing.T) common.EndorseTxContract {
	t.Helper()
	ds := "file:" + filepath.Join(t.TempDir(), "etx.sqlite")
	dbp := sqlite.NewDbProvider()
	rw, err := dbp.Get(sqlite.Opts{DataSource: ds})
	require.NoError(t, err)
	tables, err := common.GetTableNames("")
	require.NoError(t, err)
	unversioned, err := fsqlite.NewEndorseTxStore(rw, tables)
	require.NoError(t, err)
	require.NoError(t, unversioned.CreateSchema())

	cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: ds}))
	s, err := fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEndorseTxStore)
	require.NoError(t, err)
	return common.EndorseTxContract{
		Store: common.KeyDataStore{Get: s.GetEndorseTx, Exists: s.ExistsEndorseTx, Put: s.PutEndorseTx},
		DB:    rw.WriteDB,
		Table: s.StoreName(),
		Step:  s.Steps()[1].Go,
		Contract: func() error {
			return migrate.Migrate(t.Context(), rw.WriteDB, migrate.SQLite, "", s, migrate.WithContract())
		},
	}
}

func TestEndorseTxStepKeepsConcurrentWriteSQLite(t *testing.T) {
	t.Parallel()
	common.TestEndorseTxStepKeepsConcurrentWrite(t, openEndorseTxContract(t))
}

func TestEndorseTxContractStepSQLite(t *testing.T) {
	t.Parallel()
	common.TestEndorseTxContractStep(t, openEndorseTxContract(t))
}

func TestEndorseTxWritesAfterContractSQLite(t *testing.T) {
	t.Parallel()
	common.TestEndorseTxWritesAfterContract(t, openEndorseTxContract(t))
}

// openVaultContract opens a vault, through the driver's constructor and Migrate, on SQLite
// tables that exist before the vault is versioned, so that its contract step is held back
// until the returned Contract runs it.
func openVaultContract(t *testing.T) common.VaultContract {
	t.Helper()
	ds := "file:" + filepath.Join(t.TempDir(), "vault.sqlite")
	dbp := sqlite.NewDbProvider()
	rw, err := dbp.Get(sqlite.Opts{DataSource: ds})
	require.NoError(t, err)
	tables, err := common.GetTableNames("")
	require.NoError(t, err)
	unversioned, err := fsqlite.NewVaultStore(rw, tables)
	require.NoError(t, err)
	require.NoError(t, unversioned.CreateSchema())

	cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: ds}))
	s, err := fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewVaultStore)
	require.NoError(t, err)
	return common.VaultContract{
		Store: s,
		DB:    rw.WriteDB,
		Table: s.StoreName(),
		Step:  s.Steps()[1].Go,
		Contract: func() error {
			return migrate.Migrate(t.Context(), rw.WriteDB, migrate.SQLite, "", s, migrate.WithContract())
		},
	}
}

func TestVaultMetadataContractStepSQLite(t *testing.T) {
	t.Parallel()
	common.TestVaultMetadataContractStep(t, openVaultContract(t))
}

func TestVaultMetadataStepKeepsConcurrentWriteSQLite(t *testing.T) {
	t.Parallel()
	common.TestVaultMetadataStepKeepsConcurrentWrite(t, openVaultContract(t))
}
