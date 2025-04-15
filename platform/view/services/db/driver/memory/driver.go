/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"time"

	utils2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const (
	MemoryPersistence   driver2.PersistenceType = "memory"
	DefaultMaxIdleConns                         = 2
	DefaultMaxIdleTime                          = time.Minute
)

type Driver struct{}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   MemoryPersistence,
		Driver: &Driver{},
	}
}

func (d *Driver) NewKVS(string, driver.Config) (driver.UnversionedPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewUnversionedPersistence)
}

func (d *Driver) NewBinding(string, driver.Config) (driver.BindingPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewBindingPersistence)
}

func (d *Driver) NewSignerInfo(string, driver.Config) (driver.SignerInfoPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewSignerInfoPersistence)
}

func (d *Driver) NewAuditInfo(string, driver.Config) (driver.AuditInfoPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewAuditInfoPersistence)
}

func (d *Driver) NewEndorseTx(string, driver.Config) (driver.EndorseTxPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewEndorseTxPersistence)
}

func (d *Driver) NewMetadata(string, driver.Config) (driver.MetadataPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewMetadataPersistence)
}

func (d *Driver) NewEnvelope(string, driver.Config) (driver.EnvelopePersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewEnvelopePersistence)
}

func (d *Driver) NewVault(string, driver.Config) (driver.VaultPersistence, error) {
	return NewPersistenceWithOpts(sqlite.NewVaultPersistence)
}

type dbObject interface {
	CreateSchema() error
}

var memOpts = sqlite.Opts{
	DataSource:   "file::memory:?cache=shared",
	SkipPragmas:  false,
	MaxOpenConns: 10,
	MaxIdleConns: DefaultMaxIdleConns,
	MaxIdleTime:  DefaultMaxIdleTime,
}

type PersistenceConstructor[O any, V dbObject] func(O, string) (V, error)

func NewPersistenceWithOpts[V dbObject](constructor PersistenceConstructor[sqlite.Opts, V]) (V, error) {
	p, err := constructor(memOpts, utils2.GenerateUUIDOnlyLetters())
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
	}

	return p, nil
}
