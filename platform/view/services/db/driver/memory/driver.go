/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const (
	MemoryPersistence driver2.PersistenceType = "memory"
)

var (
	opts = common.Opts{
		Driver:          "sqlite",
		DataSource:      "file::memory:?cache=shared",
		TablePrefix:     "memory",
		SkipCreateTable: false,
		SkipPragmas:     false,
		MaxOpenConns:    10,
		MaxIdleConns:    common.CopyPtr(common.DefaultMaxIdleConns),
		MaxIdleTime:     common.CopyPtr(common.DefaultMaxIdleTime),
	}
)

type Driver struct{}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   MemoryPersistence,
		Driver: &Driver{},
	}
}

func (d *Driver) NewKVS(string, driver.Config) (driver.UnversionedPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewUnversionedPersistence)
}

func (d *Driver) NewBinding(string, driver.Config) (driver.BindingPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewBindingPersistence)
}

func (d *Driver) NewSignerInfo(string, driver.Config) (driver.SignerInfoPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewSignerInfoPersistence)
}

func (d *Driver) NewAuditInfo(string, driver.Config) (driver.AuditInfoPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewAuditInfoPersistence)
}

func (d *Driver) NewEndorseTx(string, driver.Config) (driver.EndorseTxPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewEndorseTxPersistence)
}

func (d *Driver) NewMetadata(string, driver.Config) (driver.MetadataPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewMetadataPersistence)
}

func (d *Driver) NewEnvelope(string, driver.Config) (driver.EnvelopePersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewEnvelopePersistence)
}

func (d *Driver) NewVault(string, driver.Config) (driver.VaultPersistence, error) {
	return common.NewPersistenceWithOpts(utils.GenerateUUIDOnlyLetters(), opts, sqlite.NewVaultPersistence)
}
