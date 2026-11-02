/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mem "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/memory"
	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	fsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	drivertesting "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common/testing"
	viewmem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// openAllStores opens every FSC store type on dataSource and returns the
// first error.
func openAllStores(t *testing.T, dataSource string, skipCreate bool) ([]drivercommon.DBObject, error) {
	t.Helper()
	cp := sqlite.NewConfigProvider(drivertesting.MockConfig(sqlite.Config{DataSource: dataSource, SkipCreateTable: skipCreate}))
	var stores []drivercommon.DBObject
	dbp := sqlite.NewDbProvider()
	for _, open := range []func() (drivercommon.DBObject, error){
		func() (drivercommon.DBObject, error) {
			return sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewKeyValueStore)
		},
		func() (drivercommon.DBObject, error) {
			return sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewBindingStore)
		},
		func() (drivercommon.DBObject, error) {
			return sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewSignerInfoStore)
		},
		func() (drivercommon.DBObject, error) {
			return sqlite.NewPersistenceWithOpts(cp, dbp, "", sqlite.NewAuditInfoStore)
		},
		func() (drivercommon.DBObject, error) {
			return fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEndorseTxStore)
		},
		func() (drivercommon.DBObject, error) {
			return fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewMetadataStore)
		},
		func() (drivercommon.DBObject, error) {
			return fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewEnvelopeStore)
		},
		func() (drivercommon.DBObject, error) {
			return fsqlite.NewPersistenceWithOpts(cp, dbp, "", fsqlite.NewVaultStore)
		},
	} {
		p, err := open()
		if err != nil {
			return nil, err
		}
		stores = append(stores, p)
	}
	return stores, nil
}

// catalog describes every table and index in db except the versions table, in
// a form that two databases with the same schema print the same way.
func catalog(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `
		SELECT type, name, coalesce(sql, '') FROM sqlite_master
		WHERE name NOT LIKE '%schema_versions' AND name NOT LIKE '%schema_openers' AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var b strings.Builder
	for rows.Next() {
		var typ, name, ddl string
		require.NoError(t, rows.Scan(&typ, &name, &ddl))
		b.WriteString(typ + " " + name + ": " + strings.Join(strings.Fields(ddl), " ") + "\n")
	}
	require.NoError(t, rows.Err())

	cols, err := db.QueryContext(t.Context(), `
		SELECT m.name, c.cid, c.name, c.type, c."notnull", coalesce(c.dflt_value, ''), c.pk
		FROM sqlite_master m, pragma_table_info(m.name) c
		WHERE m.type = 'table' AND m.name NOT LIKE '%schema_versions' AND m.name NOT LIKE '%schema_openers'
		ORDER BY m.name, c.cid`)
	require.NoError(t, err)
	defer func() { _ = cols.Close() }()
	for cols.Next() {
		var table, name, typ, dflt string
		var cid, notNull, pk int
		require.NoError(t, cols.Scan(&table, &cid, &name, &typ, &notNull, &dflt, &pk))
		fmt.Fprintf(&b, "column %s.%d %s %s notnull=%d default=%q pk=%d\n", table, cid, name, typ, notNull, dflt, pk)
	}
	require.NoError(t, cols.Err())
	return b.String()
}

// TestStepOneMatchesCreateSchema: on an empty database, step 1 of every FSC
// store creates the tables, columns, primary keys and indexes CreateSchema
// creates, and records every store at version 1.
func TestStepOneMatchesCreateSchema(t *testing.T) {
	t.Parallel()

	migrated := "file:" + filepath.Join(t.TempDir(), "migrated.db")
	stores, err := openAllStores(t, migrated, false)
	require.NoError(t, err)

	// skipCreateTable refuses a pending store, so the stores CreateSchema
	// prepares are opened with the check and built by hand.
	created := "file:" + filepath.Join(t.TempDir(), "created.db")
	plain, err := sqlite.NewDbProvider().Get(sqlite.Opts{DataSource: created})
	require.NoError(t, err)
	viewTables, err := sqlcommon.GetTableNames("")
	require.NoError(t, err)
	fabricTables, err := fsqlcommon.GetTableNames("")
	require.NoError(t, err)
	for _, build := range []func() (drivercommon.DBObject, error){
		func() (drivercommon.DBObject, error) { return sqlite.NewKeyValueStore(plain, viewTables) },
		func() (drivercommon.DBObject, error) { return sqlite.NewBindingStore(plain, viewTables) },
		func() (drivercommon.DBObject, error) { return sqlite.NewSignerInfoStore(plain, viewTables) },
		func() (drivercommon.DBObject, error) { return sqlite.NewAuditInfoStore(plain, viewTables) },
		func() (drivercommon.DBObject, error) { return fsqlite.NewEndorseTxStore(plain, fabricTables) },
		func() (drivercommon.DBObject, error) { return fsqlite.NewMetadataStore(plain, fabricTables) },
		func() (drivercommon.DBObject, error) { return fsqlite.NewEnvelopeStore(plain, fabricTables) },
		func() (drivercommon.DBObject, error) { return fsqlite.NewVaultStore(plain, fabricTables) },
	} {
		p, err := build()
		require.NoError(t, err)
		require.NoError(t, p.CreateSchema())
	}

	mdb, err := sqlite.NewDbProvider().Get(sqlite.Opts{DataSource: migrated})
	require.NoError(t, err)
	assert.Equal(t, catalog(t, plain.ReadDB), catalog(t, mdb.ReadDB))

	for _, s := range stores {
		v, ok := s.(migrate.Versioned)
		require.True(t, ok, "%T is not migrate.Versioned", s)
		var version int
		require.NoError(t, mdb.ReadDB.QueryRowContext(t.Context(), "SELECT version FROM fsc_schema_versions WHERE store = $1", v.StoreName()).Scan(&version))
		assert.Equal(t, len(v.Steps()), version, v.StoreName())
	}
}

// TestSkipCreateTableRefusesPendingStore: with skipCreateTable every FSC store
// fails to open while its steps are pending, naming the store, and opens once
// the steps ran.
func TestSkipCreateTableRefusesPendingStore(t *testing.T) {
	t.Parallel()
	ds := "file:" + filepath.Join(t.TempDir(), "skip.db")

	_, err := openAllStores(t, ds, true)
	require.ErrorIs(t, err, migrate.ErrPending)
	require.ErrorContains(t, err, "store [fsc_kvs]")

	_, err = openAllStores(t, ds, false)
	require.NoError(t, err)
	_, err = openAllStores(t, ds, true)
	require.NoError(t, err)
}

// TestMemoryStoresAreVersioned: the memory drivers hand out the SQLite stores,
// so the catalog test above covers their schema; this checks they are migrated.
func TestMemoryStoresAreVersioned(t *testing.T) {
	t.Parallel()

	kvs, err := viewmem.NewDriver().NewKVS("")
	require.NoError(t, err)
	assert.Implements(t, (*migrate.Versioned)(nil), kvs)
	vault, err := mem.NewDriver().NewVault("", "memtest")
	require.NoError(t, err)
	assert.Implements(t, (*migrate.Versioned)(nil), vault)
}
