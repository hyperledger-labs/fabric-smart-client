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

func (d *Driver) NewKVS(driver.Config, ...string) (driver.UnversionedPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewUnversionedPersistence)
}

func (d *Driver) NewBinding(driver.Config, ...string) (driver.BindingPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewBindingPersistence)
}

func (d *Driver) NewSignerInfo(driver.Config, ...string) (driver.SignerInfoPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewSignerInfoPersistence)
}

func (d *Driver) NewAuditInfo(driver.Config, ...string) (driver.AuditInfoPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewAuditInfoPersistence)
}

func (d *Driver) NewEndorseTx(driver.Config, ...string) (driver.EndorseTxPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewEndorseTxPersistence)
}

func (d *Driver) NewMetadata(driver.Config, ...string) (driver.MetadataPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewMetadataPersistence)
}

func (d *Driver) NewEnvelope(driver.Config, ...string) (driver.EnvelopePersistence, error) {
	return newPersistenceWithOpts(sqlite.NewEnvelopePersistence)
}

func (d *Driver) NewVault(driver.Config, ...string) (driver2.VaultStore, error) {
	return newPersistenceWithOpts(sqlite.NewVaultPersistence)
}

func newPersistenceWithOpts[V common.DBObject](constructor common.PersistenceConstructor[sqlite.Opts, V]) (V, error) {
	p, err := constructor(op.GetOpts())
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
	}

	return p, nil
}
