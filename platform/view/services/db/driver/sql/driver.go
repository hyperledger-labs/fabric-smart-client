/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"fmt"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/postgres"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

const (
	Postgres common.SQLDriverType = "postgres"
	SQLite   common.SQLDriverType = "sqlite"

	SQLPersistence driver2.PersistenceType = "sql"
)

var logger = logging.MustGetLogger("view-sdk.services.db.driver.sql")

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   SQLPersistence,
		Driver: &Driver{},
	}
}

type Driver struct {
}

type dbObject interface {
	CreateSchema() error
}

type persistenceConstructor[V dbObject] func(common.Opts, string) (V, error)

type transactionalVersionedPersistence interface {
	driver.TransactionalVersionedPersistence
	dbObject
}

var VersionedConstructors = map[common.SQLDriverType]persistenceConstructor[transactionalVersionedPersistence]{
	Postgres: func(o common.Opts, t string) (transactionalVersionedPersistence, error) {
		return postgres.NewVersioned(o, t)
	},
	SQLite: func(o common.Opts, t string) (transactionalVersionedPersistence, error) {
		return sqlite.NewVersioned(o, t)
	},
}

type bindingPersistence interface {
	driver.BindingPersistence
	dbObject
}

type signerInfoPersistence interface {
	driver.SignerInfoPersistence
	dbObject
}

type auditInfoPersistence interface {
	driver.AuditInfoPersistence
	dbObject
}

type endorseTxPersistence interface {
	driver.EndorseTxPersistence
	dbObject
}

type metadataPersistence interface {
	driver.MetadataPersistence
	dbObject
}

type envelopePersistence interface {
	driver.EnvelopePersistence
	dbObject
}

type vaultPersistence interface {
	driver.VaultPersistence
	dbObject
}

var BindingConstructors = map[common.SQLDriverType]persistenceConstructor[bindingPersistence]{
	Postgres: func(o common.Opts, t string) (bindingPersistence, error) { return postgres.NewBindingPersistence(o, t) },
	SQLite:   func(o common.Opts, t string) (bindingPersistence, error) { return sqlite.NewBindingPersistence(o, t) },
}

var SignerInfoConstructors = map[common.SQLDriverType]persistenceConstructor[signerInfoPersistence]{
	Postgres: func(o common.Opts, t string) (signerInfoPersistence, error) {
		return postgres.NewSignerInfoPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (signerInfoPersistence, error) {
		return sqlite.NewSignerInfoPersistence(o, t)
	},
}

var AuditInfoConstructors = map[common.SQLDriverType]persistenceConstructor[auditInfoPersistence]{
	Postgres: func(o common.Opts, t string) (auditInfoPersistence, error) {
		return postgres.NewAuditInfoPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (auditInfoPersistence, error) {
		return sqlite.NewAuditInfoPersistence(o, t)
	},
}

var EndorseTxConstructors = map[common.SQLDriverType]persistenceConstructor[endorseTxPersistence]{
	Postgres: func(o common.Opts, t string) (endorseTxPersistence, error) {
		return postgres.NewEndorseTxPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (endorseTxPersistence, error) {
		return sqlite.NewEndorseTxPersistence(o, t)
	},
}

var MetadataConstructors = map[common.SQLDriverType]persistenceConstructor[metadataPersistence]{
	Postgres: func(o common.Opts, t string) (metadataPersistence, error) {
		return postgres.NewMetadataPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (metadataPersistence, error) {
		return sqlite.NewMetadataPersistence(o, t)
	},
}

var EnvelopeConstructors = map[common.SQLDriverType]persistenceConstructor[envelopePersistence]{
	Postgres: func(o common.Opts, t string) (envelopePersistence, error) {
		return postgres.NewEnvelopePersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (envelopePersistence, error) {
		return sqlite.NewEnvelopePersistence(o, t)
	},
}

var VaultConstructors = map[common.SQLDriverType]persistenceConstructor[vaultPersistence]{
	Postgres: func(o common.Opts, t string) (vaultPersistence, error) {
		return postgres.NewVaultPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (vaultPersistence, error) {
		return sqlite.NewVaultPersistence(o, t)
	},
}

func (d *Driver) NewKVS(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	backend, err := newPersistence(dataSourceName, config, VersionedConstructors)
	if err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: backend}, nil
}

func (d *Driver) NewBinding(dataSourceName string, config driver.Config) (driver.BindingPersistence, error) {
	return newPersistence(dataSourceName, config, BindingConstructors)
}

func (d *Driver) NewSignerInfo(dataSourceName string, config driver.Config) (driver.SignerInfoPersistence, error) {
	return newPersistence(dataSourceName, config, SignerInfoConstructors)
}

func (d *Driver) NewAuditInfo(dataSourceName string, config driver.Config) (driver.AuditInfoPersistence, error) {
	return newPersistence(dataSourceName, config, AuditInfoConstructors)
}

func (d *Driver) NewEndorseTx(dataSourceName string, config driver.Config) (driver.EndorseTxPersistence, error) {
	return newPersistence(dataSourceName, config, EndorseTxConstructors)
}

func (d *Driver) NewMetadata(dataSourceName string, config driver.Config) (driver.MetadataPersistence, error) {
	return newPersistence(dataSourceName, config, MetadataConstructors)
}

func (d *Driver) NewEnvelope(dataSourceName string, config driver.Config) (driver.EnvelopePersistence, error) {
	return newPersistence(dataSourceName, config, EnvelopeConstructors)
}

func (d *Driver) NewVault(dataSourceName string, config driver.Config) (driver.VaultPersistence, error) {
	return newPersistence(dataSourceName, config, VaultConstructors)
}

func newPersistence[V dbObject](dataSourceName string, config driver.Config, constructors map[common.SQLDriverType]persistenceConstructor[V]) (V, error) {
	logger.Infof("opening new transactional database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("failed getting options for datasource: %w", err)
	}

	return NewPersistenceWithOpts(dataSourceName, opts, constructors)
}

func NewPersistenceWithOpts[V dbObject](dataSourceName string, opts common.Opts, constructors map[common.SQLDriverType]persistenceConstructor[V]) (V, error) {
	nc, err := common.NewTableNameCreator(opts.TablePrefix)
	if err != nil {
		return utils.Zero[V](), err
	}

	table, valid := nc.GetTableName(dataSourceName)
	if !valid {
		return utils.Zero[V](), fmt.Errorf("invalid table name [%s]: only letters and underscores allowed: %w", table, err)
	}
	c, ok := constructors[opts.Driver]
	if !ok {
		return utils.Zero[V](), fmt.Errorf("unknown driver: %s", opts.Driver)
	}
	p, err := c(opts, table)
	if err != nil {
		return utils.Zero[V](), err
	}
	if !opts.SkipCreateTable {
		if err := p.CreateSchema(); err != nil {
			return utils.Zero[V](), err
		}
	}
	return p, nil
}

func getOps(config driver.Config) (common.Opts, error) {
	opts, err := common.GetOpts(config, "")
	if err != nil {
		return common.Opts{}, err
	}
	if opts.TablePrefix == "" {
		opts.TablePrefix = "fsc"
	}
	if opts.MaxIdleTime == nil {
		opts.MaxIdleTime = common.CopyPtr(common.DefaultMaxIdleTime)
	}
	if opts.MaxIdleConns == nil {
		opts.MaxIdleConns = common.CopyPtr(common.DefaultMaxIdleConns)
	}
	return *opts, nil
}
