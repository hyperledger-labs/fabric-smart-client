/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/sqlite"
)

const Persistence driver2.PersistenceType = "memory"

type Driver struct{}

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: &Driver{},
	}
}

func (d *Driver) NewKVS(string, driver.Config) (driver.UnversionedPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewUnversionedPersistence)
}

func (d *Driver) NewBinding(string, driver.Config) (driver.BindingPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewBindingPersistence)
}

func (d *Driver) NewSignerInfo(string, driver.Config) (driver.SignerInfoPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewSignerInfoPersistence)
}

func (d *Driver) NewAuditInfo(string, driver.Config) (driver.AuditInfoPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewAuditInfoPersistence)
}

func (d *Driver) NewEndorseTx(string, driver.Config) (driver.EndorseTxPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewEndorseTxPersistence)
}

func (d *Driver) NewMetadata(string, driver.Config) (driver.MetadataPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewMetadataPersistence)
}

func (d *Driver) NewEnvelope(string, driver.Config) (driver.EnvelopePersistence, error) {
	return newPersistenceWithOpts(sqlite.NewEnvelopePersistence)
}

func (d *Driver) NewVault(string, driver.Config) (driver.VaultPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewVaultPersistence)
}

func newPersistenceWithOpts[V common.DBObject](constructor common.PersistenceConstructor[sqlite.Opts, V]) (V, error) {
	p, err := constructor(op.GetOpts(), tnc.CreateTableName())
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
	}

	return p, nil
}
