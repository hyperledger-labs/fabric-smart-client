/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	fpostgres "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	fsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate/migratetest"
)

// TestSteps checks every Fabric store's steps against testdata/steps/<dialect>-<store>.steps,
// so a released step cannot change unnoticed. The memory driver uses the SQLite stores. Go
// parts of a step, such as the vault's key conversion, are not covered.
func TestSteps(t *testing.T) {
	t.Parallel()
	tables, err := fsqlcommon.GetTableNames("")
	require.NoError(t, err)
	dbs := &drivercommon.RWDB{}
	stores := map[string]func() (any, error){
		"sqlite-endorsetx":   func() (any, error) { return fsqlite.NewEndorseTxStore(dbs, tables) },
		"sqlite-metadata":    func() (any, error) { return fsqlite.NewMetadataStore(dbs, tables) },
		"sqlite-envelope":    func() (any, error) { return fsqlite.NewEnvelopeStore(dbs, tables) },
		"sqlite-vault":       func() (any, error) { return fsqlite.NewVaultStore(dbs, tables) },
		"postgres-endorsetx": func() (any, error) { return fpostgres.NewEndorseTxStore(dbs, tables) },
		"postgres-metadata":  func() (any, error) { return fpostgres.NewMetadataStore(dbs, tables) },
		"postgres-envelope":  func() (any, error) { return fpostgres.NewEnvelopeStore(dbs, tables) },
		"postgres-vault":     func() (any, error) { return fpostgres.NewVaultStore(dbs, tables) },
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
