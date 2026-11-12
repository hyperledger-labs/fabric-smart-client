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
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
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

// openEndorseTxContract returns an endorse-tx store on a Postgres table that exists before the
// store is versioned, so that its required contract step is pending until the returned
// Contract runs it.
func openEndorseTxContract(t *testing.T) common.EndorseTxContract {
	t.Helper()
	terminate, ds, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)
	dbp := postgres.NewDbProvider()
	rw, err := dbp.Get(postgres.Opts{DataSource: ds})
	require.NoError(t, err)
	tables, err := common.GetTableNames("")
	require.NoError(t, err)
	unversioned, err := NewEndorseTxStore(rw, tables)
	require.NoError(t, err)
	require.NoError(t, unversioned.CreateSchema())

	cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: ds}))
	return common.EndorseTxContract{
		Store:      common.KeyDataStore{Get: unversioned.GetEndorseTx, Exists: unversioned.ExistsEndorseTx, Put: unversioned.PutEndorseTx},
		DB:         rw.WriteDB,
		Table:      unversioned.StoreName(),
		Step:       unversioned.Steps()[1].Go,
		Concurrent: rw.WriteDB,
		Contract: func() error {
			return migrate.Migrate(t.Context(), rw.WriteDB, migrate.Postgres, "", unversioned, migrate.WithContract())
		},
		Open: func() error {
			_, err := NewPersistenceWithOpts(cp, dbp, "", NewEndorseTxStore)
			return err
		},
	}
}

func TestEndorseTxStepKeepsConcurrentWritePostgres(t *testing.T) {
	t.Parallel()
	common.TestEndorseTxStepKeepsConcurrentWrite(t, openEndorseTxContract(t))
}

func TestEndorseTxContractStepPostgres(t *testing.T) {
	t.Parallel()
	common.TestEndorseTxContractStep(t, openEndorseTxContract(t))
}

func TestEndorseTxWritesAfterContractPostgres(t *testing.T) {
	t.Parallel()
	common.TestEndorseTxWritesAfterContract(t, openEndorseTxContract(t))
}

// openVaultContract returns a vault on Postgres tables that exist before the vault is versioned,
// so that its required contract step is pending until the returned Contract runs it.
func openVaultContract(t *testing.T) common.VaultContract {
	t.Helper()
	terminate, ds, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)
	dbp := postgres.NewDbProvider()
	rw, err := dbp.Get(postgres.Opts{DataSource: ds})
	require.NoError(t, err)
	tables, err := common.GetTableNames("")
	require.NoError(t, err)
	unversioned, err := NewVaultStore(rw, tables)
	require.NoError(t, err)
	require.NoError(t, unversioned.CreateSchema())

	cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: ds}))
	return common.VaultContract{
		Store:      unversioned,
		DB:         rw.WriteDB,
		Table:      unversioned.StoreName(),
		Step:       unversioned.Steps()[1].Go,
		Concurrent: rw.WriteDB,
		Contract: func() error {
			return migrate.Migrate(t.Context(), rw.WriteDB, migrate.Postgres, "", unversioned, migrate.WithContract())
		},
		Open: func() error {
			_, err := NewPersistenceWithOpts(cp, dbp, "", NewVaultStore)
			return err
		},
	}
}

func TestVaultMetadataContractStepPostgres(t *testing.T) {
	t.Parallel()
	common.TestVaultMetadataContractStep(t, openVaultContract(t))
}

func TestVaultMetadataStepKeepsConcurrentWritePostgres(t *testing.T) {
	t.Parallel()
	common.TestVaultMetadataStepKeepsConcurrentWrite(t, openVaultContract(t))
}
