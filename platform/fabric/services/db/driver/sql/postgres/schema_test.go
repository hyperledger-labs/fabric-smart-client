/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
)

// pgCatalog describes the columns, constraints and indexes of every table whose
// name starts with prefix+"_", except the versions table, with the prefix
// removed so that two prefixes with the same schema print the same way.
func pgCatalog(t *testing.T, db *sql.DB, prefix string) string {
	t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT table_name, ordinal_position::text, column_name, data_type, is_nullable, coalesce(column_default, '')
		 FROM information_schema.columns WHERE table_schema = 'public' AND table_name LIKE $1
		 ORDER BY 1, 2`,
		`SELECT conrelid::regclass::text, contype::text, '', pg_get_constraintdef(oid), '', ''
		 FROM pg_constraint WHERE conrelid::regclass::text LIKE $1 ORDER BY 1, 2, 4`,
		`SELECT tablename, indexname, '', indexdef, '', '' FROM pg_indexes
		 WHERE schemaname = 'public' AND tablename LIKE $1 ORDER BY 1, 2`,
	} {
		func() {
			rows, err := db.QueryContext(t.Context(), q, prefix+"\\_%")
			require.NoError(t, err)
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var f [6]string
				require.NoError(t, rows.Scan(&f[0], &f[1], &f[2], &f[3], &f[4], &f[5]))
				if strings.HasSuffix(f[0], "_schema_versions") || strings.HasSuffix(f[0], "_schema_openers") {
					continue
				}
				fmt.Fprintln(&b, strings.ReplaceAll(strings.Join(f[:], " | "), prefix+"_", "P_"))
			}
			require.NoError(t, rows.Err())
		}()
	}
	return b.String()
}

// TestPostgresStepOneMatchesCreateSchema: on an empty database, step 1 of every
// FSC store creates the columns, constraints and indexes CreateSchema creates,
// and records every store at version 1. The two schemas use two table prefixes
// in one database.
func TestPostgresStepOneMatchesCreateSchema(t *testing.T) {
	t.Parallel()

	terminate, pgConnStr, err := postgres.StartPostgres(t.Context(), postgres.ConfigFromEnv(), nil)
	require.NoError(t, err)
	t.Cleanup(terminate)
	dbp := postgres.NewDbProvider()

	// Prefix "mig": every store opened through the drivers, which migrate it.
	cp := postgres.NewConfigProvider(drivertesting.MockConfig(postgres.Config{DataSource: pgConnStr, TablePrefix: "mig"}))
	var stores []drivercommon.DBObject
	for _, open := range []func() (drivercommon.DBObject, error){
		func() (drivercommon.DBObject, error) {
			return postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewKeyValueStore)
		},
		func() (drivercommon.DBObject, error) {
			return postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewBindingStore)
		},
		func() (drivercommon.DBObject, error) {
			return postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewSignerInfoStore)
		},
		func() (drivercommon.DBObject, error) {
			return postgres.NewPersistenceWithOpts(cp, dbp, "", postgres.NewAuditInfoStore)
		},
		func() (drivercommon.DBObject, error) { return NewPersistenceWithOpts(cp, dbp, "", NewEndorseTxStore) },
		func() (drivercommon.DBObject, error) { return NewPersistenceWithOpts(cp, dbp, "", NewMetadataStore) },
		func() (drivercommon.DBObject, error) { return NewPersistenceWithOpts(cp, dbp, "", NewEnvelopeStore) },
		func() (drivercommon.DBObject, error) { return NewPersistenceWithOpts(cp, dbp, "", NewVaultStore) },
	} {
		p, err := open()
		require.NoError(t, err)
		stores = append(stores, p)
	}

	// Prefix "cre": every store built directly and prepared with CreateSchema.
	dbs, err := dbp.Get(postgres.Opts{DataSource: pgConnStr})
	require.NoError(t, err)
	viewTables, err := sqlcommon.GetTableNames("cre")
	require.NoError(t, err)
	fabricTables, err := fsqlcommon.GetTableNames("cre")
	require.NoError(t, err)
	for _, build := range []func() (drivercommon.DBObject, error){
		func() (drivercommon.DBObject, error) { return postgres.NewKeyValueStore(dbs, viewTables) },
		func() (drivercommon.DBObject, error) { return postgres.NewBindingStore(dbs, viewTables) },
		func() (drivercommon.DBObject, error) { return postgres.NewSignerInfoStore(dbs, viewTables) },
		func() (drivercommon.DBObject, error) { return postgres.NewAuditInfoStore(dbs, viewTables) },
		func() (drivercommon.DBObject, error) { return NewEndorseTxStore(dbs, fabricTables) },
		func() (drivercommon.DBObject, error) { return NewMetadataStore(dbs, fabricTables) },
		func() (drivercommon.DBObject, error) { return NewEnvelopeStore(dbs, fabricTables) },
		func() (drivercommon.DBObject, error) { return NewVaultStore(dbs, fabricTables) },
	} {
		p, err := build()
		require.NoError(t, err)
		require.NoError(t, p.CreateSchema())
	}

	want := pgCatalog(t, dbs.ReadDB, "cre")
	require.NotEmpty(t, want)
	assert.Equal(t, want, pgCatalog(t, dbs.ReadDB, "mig"))

	for _, s := range stores {
		v, ok := s.(migrate.Versioned)
		require.True(t, ok, "%T is not migrate.Versioned", s)
		var version int
		require.NoError(t, dbs.ReadDB.QueryRowContext(t.Context(), "SELECT version FROM mig_schema_versions WHERE store = $1", v.StoreName()).Scan(&version))
		assert.Equal(t, len(v.Steps()), version, v.StoreName())
	}
}
