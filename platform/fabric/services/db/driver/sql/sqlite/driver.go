/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	fdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	fsqlcommon "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	drivercommon "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

var logger = logging.MustGetLogger()

const (
	Persistence cdriver.PersistenceType = "sqlite"
)

func NewNamedDriver(config driver.Config, dbProvider sqlite.DbProvider) fdbdriver.NamedDriver {
	return fdbdriver.NamedDriver{
		Name:   Persistence,
		Driver: NewDriverWithDbProvider(config, dbProvider),
	}
}

func NewDriverWithDbProvider(config driver.Config, dbProvider sqlite.DbProvider) *Driver {
	return &Driver{
		cp:         sqlite.NewConfigProvider(drivercommon.NewConfig(config)),
		dbProvider: dbProvider,
	}
}

type Driver struct {
	cp         *sqlite.ConfigProvider
	dbProvider sqlite.DbProvider
}

func (d *Driver) NewEndorseTx(name driver.PersistenceName, params ...string) (fdbdriver.EndorseTxStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(name driver.PersistenceName, params ...string) (fdbdriver.MetadataStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(name driver.PersistenceName, params ...string) (fdbdriver.EnvelopeStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(name driver.PersistenceName, params ...string) (cdriver.VaultStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewVaultStore, params...)
}

// NewPersistenceWithOpts constructs a V backed by the sqlite persistence configured under name and
// params: it resolves the connection options and table names, opens the database, and invokes
// constructor to build the store, preparing its schema with migrate.CreateSchema unless configured to skip.
func NewPersistenceWithOpts[V drivercommon.DBObject](cfg *sqlite.ConfigProvider, dbProvider sqlite.DbProvider, name driver.PersistenceName, constructor fsqlcommon.PersistenceConstructor[V], params ...string) (V, error) {
	var zero V

	o, err := cfg.GetOpts(name, params...)
	if err != nil {
		return zero, err
	}

	opts := sqlite.Opts{
		DataSource:      o.DataSource,
		SkipPragmas:     o.SkipPragmas,
		MaxOpenConns:    o.MaxOpenConns,
		MaxIdleConns:    *o.MaxIdleConns,
		MaxIdleTime:     *o.MaxIdleTime,
		TablePrefix:     o.TablePrefix,
		TableNameParams: o.TableNameParams,
		Tracing:         o.Tracing,
	}
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
	if !o.SkipCreateTable {
		if err := migrate.CreateSchema(context.Background(), dbs.WriteDB, migrate.SQLite, o.TablePrefix, p); err != nil {
			return zero, err
		}
	}
	return p, nil
}
