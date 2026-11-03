/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate/migratetest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// TestSteps checks every view store's steps against testdata/steps/<dialect>-<store>.steps,
// so a released step cannot change unnoticed. The memory driver uses the SQLite stores.
func TestSteps(t *testing.T) {
	t.Parallel()
	tables, err := sqlcommon.GetTableNames("")
	require.NoError(t, err)
	dbs := &drivercommon.RWDB{}
	stores := map[string]func() (any, error){
		"sqlite-kvs":        func() (any, error) { return sqlite.NewKeyValueStore(dbs, tables) },
		"sqlite-binding":    func() (any, error) { return sqlite.NewBindingStore(dbs, tables) },
		"sqlite-signerinfo": func() (any, error) { return sqlite.NewSignerInfoStore(dbs, tables) },
		"sqlite-auditinfo":  func() (any, error) { return sqlite.NewAuditInfoStore(dbs, tables) },
		"postgres-kvs":      func() (any, error) { return postgres.NewKeyValueStore(dbs, tables) },
		"postgres-binding":  func() (any, error) { return postgres.NewBindingStore(dbs, tables) },
		"postgres-signerinfo": func() (any, error) {
			return postgres.NewSignerInfoStore(dbs, tables)
		},
		"postgres-auditinfo": func() (any, error) { return postgres.NewAuditInfoStore(dbs, tables) },
	}
	for name, newStore := range stores {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, err := newStore()
			require.NoError(t, err)
			v, ok := s.(migrate.Versioned)
			require.True(t, ok, "%T is not migrate.Versioned", s)
			migratetest.CheckSteps(t, filepath.Join("testdata", "steps", name+".steps"), v.Steps())
		})
	}
}
