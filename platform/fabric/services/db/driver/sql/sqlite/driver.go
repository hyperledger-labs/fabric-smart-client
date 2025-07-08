/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sqlite

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	driver3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	common3 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	sqlite2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/sqlite"
)

var logger = logging.MustGetLogger()

const (
	Persistence driver2.PersistenceType = "sqlite"
)

func NewNamedDriver(config driver.Config, dbProvider sqlite2.DbProvider) driver3.NamedDriver {
	return driver3.NamedDriver{
		Name:   Persistence,
		Driver: NewDriverWithDbProvider(config, dbProvider),
	}
}

func NewDriver(config driver.Config) *Driver {
	return NewDriverWithDbProvider(config, sqlite2.NewDbProvider())
}

func NewDriverWithDbProvider(config driver.Config, dbProvider sqlite2.DbProvider) *Driver {
	return &Driver{
		cp:         sqlite2.NewConfigProvider(common2.NewConfig(config)),
		dbProvider: dbProvider,
	}
}

type Driver struct {
	cp         *sqlite2.ConfigProvider
	dbProvider sqlite2.DbProvider
}

func (d *Driver) NewEndorseTx(name driver.PersistenceName, params ...string) (driver3.EndorseTxStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(name driver.PersistenceName, params ...string) (driver3.MetadataStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(name driver.PersistenceName, params ...string) (driver3.EnvelopeStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(name driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return NewPersistenceWithOpts(d.cp, d.dbProvider, name, NewVaultStore, params...)
}

func NewPersistenceWithOpts[V common2.DBObject](cfg *sqlite2.ConfigProvider, dbProvider sqlite2.DbProvider, name driver.PersistenceName, constructor common3.PersistenceConstructor[V], params ...string) (V, error) {
	o, err := cfg.GetOpts(name, params...)
	if err != nil {
		return utils.Zero[V](), err
	}

	opts := sqlite2.Opts{
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
		return utils.Zero[V](), fmt.Errorf("error opening db: %w", err)
	}
	tables := common3.GetTableNames(opts.TablePrefix, opts.TableNameParams...)
	p, err := constructor(dbs, tables)
	if err != nil {
		return utils.Zero[V](), err
	}
	if !o.SkipCreateTable {
		if err := p.CreateSchema(); err != nil {
			return utils.Zero[V](), err
		}
	}
	return p, nil
}
