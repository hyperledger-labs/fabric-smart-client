/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	viewsqlite "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

const Persistence cdriver.PersistenceType = "memory"

type Driver struct {
	dbProvider viewsqlite.DbProvider
}

func NewNamedDriver(dbProvider viewsqlite.DbProvider) fdbdriver.NamedDriver {
	return fdbdriver.NamedDriver{
		Name:   Persistence,
		Driver: NewDriverWithDbProvider(dbProvider),
	}
}

func NewDriver() *Driver {
	return NewDriverWithDbProvider(viewsqlite.NewDbProvider())
}

func NewDriverWithDbProvider(dbProvider viewsqlite.DbProvider) *Driver {
	return &Driver{dbProvider: dbProvider}
}

func (d *Driver) NewEndorseTx(_ driver.PersistenceName, params ...string) (fdbdriver.EndorseTxStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(_ driver.PersistenceName, params ...string) (fdbdriver.MetadataStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(_ driver.PersistenceName, params ...string) (fdbdriver.EnvelopeStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(_ driver.PersistenceName, params ...string) (cdriver.VaultStore, error) {
	return newPersistenceWithOpts(d.dbProvider, sqlite.NewVaultStore, params...)
}

// newPersistenceWithOpts constructs a V backed by an in-memory sqlite database, using params to
// compute its table names, and invokes constructor to build the store before preparing its schema with migrate.CreateSchema.
func newPersistenceWithOpts[V common.DBObject](dbProvider viewsqlite.DbProvider, constructor fsqlcommon.PersistenceConstructor[V], params ...string) (V, error) {
	var zero V

	opts := Op.GetOpts(params...)
	dbs, err := dbProvider.Get(opts)
	if err != nil {
		return zero, errors.Wrap(err, "error opening db")
	}
	tables, err := fsqlcommon.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	if err != nil {
		return zero, err
	}
	p, err := constructor(dbs, tables)
	if err != nil {
		return zero, err
	}
	if err := migrate.CreateSchema(context.Background(), dbs.WriteDB, migrate.SQLite, opts.TablePrefix, p); err != nil {
		return zero, err
	}

	return p, nil
}
