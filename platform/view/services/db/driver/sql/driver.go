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

type unversionedPersistence interface {
	driver.UnversionedPersistence
	dbObject
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

var UnversionedConstructors = map[common.SQLDriverType]common.PersistenceConstructor[unversionedPersistence]{
	Postgres: func(o common.Opts, t string) (unversionedPersistence, error) {
		return postgres.NewUnversionedPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (unversionedPersistence, error) {
		return sqlite.NewUnversionedPersistence(o, t)
	},
}

var BindingConstructors = map[common.SQLDriverType]common.PersistenceConstructor[bindingPersistence]{
	Postgres: func(o common.Opts, t string) (bindingPersistence, error) { return postgres.NewBindingPersistence(o, t) },
	SQLite:   func(o common.Opts, t string) (bindingPersistence, error) { return sqlite.NewBindingPersistence(o, t) },
}

var SignerInfoConstructors = map[common.SQLDriverType]common.PersistenceConstructor[signerInfoPersistence]{
	Postgres: func(o common.Opts, t string) (signerInfoPersistence, error) {
		return postgres.NewSignerInfoPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (signerInfoPersistence, error) {
		return sqlite.NewSignerInfoPersistence(o, t)
	},
}

var AuditInfoConstructors = map[common.SQLDriverType]common.PersistenceConstructor[auditInfoPersistence]{
	Postgres: func(o common.Opts, t string) (auditInfoPersistence, error) {
		return postgres.NewAuditInfoPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (auditInfoPersistence, error) {
		return sqlite.NewAuditInfoPersistence(o, t)
	},
}

var EndorseTxConstructors = map[common.SQLDriverType]common.PersistenceConstructor[endorseTxPersistence]{
	Postgres: func(o common.Opts, t string) (endorseTxPersistence, error) {
		return postgres.NewEndorseTxPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (endorseTxPersistence, error) {
		return sqlite.NewEndorseTxPersistence(o, t)
	},
}

var MetadataConstructors = map[common.SQLDriverType]common.PersistenceConstructor[metadataPersistence]{
	Postgres: func(o common.Opts, t string) (metadataPersistence, error) {
		return postgres.NewMetadataPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (metadataPersistence, error) {
		return sqlite.NewMetadataPersistence(o, t)
	},
}

var EnvelopeConstructors = map[common.SQLDriverType]common.PersistenceConstructor[envelopePersistence]{
	Postgres: func(o common.Opts, t string) (envelopePersistence, error) {
		return postgres.NewEnvelopePersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (envelopePersistence, error) {
		return sqlite.NewEnvelopePersistence(o, t)
	},
}

var VaultConstructors = map[common.SQLDriverType]common.PersistenceConstructor[vaultPersistence]{
	Postgres: func(o common.Opts, t string) (vaultPersistence, error) {
		return postgres.NewVaultPersistence(o, t)
	},
	SQLite: func(o common.Opts, t string) (vaultPersistence, error) {
		return sqlite.NewVaultPersistence(o, t)
	},
}

func (d *Driver) NewKVS(dataSourceName string, config driver.Config) (driver.UnversionedPersistence, error) {
	return newPersistence(dataSourceName, config, UnversionedConstructors)
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

func newPersistence[V dbObject](dataSourceName string, config driver.Config, constructors map[common.SQLDriverType]common.PersistenceConstructor[V]) (V, error) {
	logger.Debugf("opening new transactional database %s", dataSourceName)
	opts, err := getOps(config)
	if err != nil {
		return utils.Zero[V](), fmt.Errorf("failed getting options for datasource: %w", err)
	}

	c, ok := constructors[opts.Driver]
	if !ok {
		return utils.Zero[V](), fmt.Errorf("unknown driver: %s", opts.Driver)
	}
	return common.NewPersistenceWithOpts[V](dataSourceName, opts, c)
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
