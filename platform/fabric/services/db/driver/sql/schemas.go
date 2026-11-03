/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

// Package sql lists the schemas of the Fabric SQL stores.
package sql

import (
	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
)

// Schemas returns the steps of every Fabric store in both dialects, read from stores built on an
// empty RWDB: their constructors and Steps touch no database. The tables have the default names,
// so the steps are those of a node without a table prefix and match testdata/steps. The memory
// driver uses the SQLite stores. Go parts of a step, such as the vault's key conversion, are not
// part of a step's hash.
func Schemas() []migrate.Schema {
	tables, err := fsqlcommon.GetTableNames("")
	if err != nil {
		panic(err) // the default formatter accepts the empty prefix
	}
	dbs := &drivercommon.RWDB{}
	return []migrate.Schema{
		migrate.NewSchema("endorsetx", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewEndorseTxStore(dbs, tables) }),
		migrate.NewSchema("metadata", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewMetadataStore(dbs, tables) }),
		migrate.NewSchema("envelope", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewEnvelopeStore(dbs, tables) }),
		migrate.NewSchema("vault", migrate.SQLite, func() (migrate.Versioned, error) { return sqlite.NewVaultStore(dbs, tables) }),
		migrate.NewSchema("endorsetx", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewEndorseTxStore(dbs, tables) }),
		migrate.NewSchema("metadata", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewMetadataStore(dbs, tables) }),
		migrate.NewSchema("envelope", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewEnvelopeStore(dbs, tables) }),
		migrate.NewSchema("vault", migrate.Postgres, func() (migrate.Versioned, error) { return postgres.NewVaultStore(dbs, tables) }),
	}
}
