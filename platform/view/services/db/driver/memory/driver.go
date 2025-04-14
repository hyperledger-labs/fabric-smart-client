/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const (
	MemoryPersistence driver2.PersistenceType = "memory"
)

type Driver struct{}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   MemoryPersistence,
		Driver: &Driver{},
	}
}

func (d *Driver) NewKVS(tableName string, opts driver.DbOpts) (driver.UnversionedPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewUnversionedPersistence)
}

func (d *Driver) NewBinding(tableName string, opts driver.DbOpts) (driver.BindingPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewBindingPersistence)
}

func (d *Driver) NewSignerInfo(tableName string, opts driver.DbOpts) (driver.SignerInfoPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewSignerInfoPersistence)
}

func (d *Driver) NewAuditInfo(tableName string, opts driver.DbOpts) (driver.AuditInfoPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewAuditInfoPersistence)
}

func (d *Driver) NewEndorseTx(tableName string, opts driver.DbOpts) (driver.EndorseTxPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewEndorseTxPersistence)
}

func (d *Driver) NewMetadata(tableName string, opts driver.DbOpts) (driver.MetadataPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewMetadataPersistence)
}

func (d *Driver) NewEnvelope(tableName string, opts driver.DbOpts) (driver.EnvelopePersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewEnvelopePersistence)
}

func (d *Driver) NewVault(tableName string, opts driver.DbOpts) (driver.VaultPersistence, error) {
	return common.NewPersistenceWithOpts[sqlite.DbOpts](tableName, opts, sqlite.NewVaultPersistence)
}
