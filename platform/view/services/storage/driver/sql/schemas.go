/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package sql lists the schemas of the view SQL stores.
package sql

import (
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

// Schemas returns the steps of every view store in both dialects, read from stores built on an
// empty RWDB: their constructors and Steps touch no database. The tables have the default names,
// so the steps are those of a node without a table prefix and match testdata/steps. The memory
// driver uses the SQLite stores.
func Schemas() []migrate.Schema {
	tables, err := sqlcommon.GetTableNames("")
	if err != nil {
		panic(err) // the default formatter accepts the empty prefix
	}
	dbs := &drivercommon.RWDB{}
	return []migrate.Schema{
		migrate.NewSchema("kvs", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewKeyValueStore(dbs, tables) }),
		migrate.NewSchema("binding", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewBindingStore(dbs, tables) }),
		migrate.NewSchema("signerinfo", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewSignerInfoStore(dbs, tables) }),
		migrate.NewSchema("auditinfo", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewAuditInfoStore(dbs, tables) }),
		migrate.NewSchema("kvs", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewKeyValueStore(dbs, tables) }),
		migrate.NewSchema("binding", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewBindingStore(dbs, tables) }),
		migrate.NewSchema("signerinfo", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewSignerInfoStore(dbs, tables) }),
		migrate.NewSchema("auditinfo", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewAuditInfoStore(dbs, tables) }),
	}
}
