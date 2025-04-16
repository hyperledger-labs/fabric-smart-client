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

func (d *Driver) NewKVS(_ driver.Config, params ...string) (driver.UnversionedPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewUnversionedPersistence, params...)
}

func (d *Driver) NewBinding(_ driver.Config, params ...string) (driver.BindingPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewBindingPersistence, params...)
}

func (d *Driver) NewSignerInfo(_ driver.Config, params ...string) (driver.SignerInfoPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewSignerInfoPersistence, params...)
}

func (d *Driver) NewAuditInfo(_ driver.Config, params ...string) (driver.AuditInfoPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewAuditInfoPersistence, params...)
}

func (d *Driver) NewEndorseTx(_ driver.Config, params ...string) (driver.EndorseTxPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewEndorseTxPersistence, params...)
}

func (d *Driver) NewMetadata(_ driver.Config, params ...string) (driver.MetadataPersistence, error) {
	return newPersistenceWithOpts(sqlite.NewMetadataPersistence, params...)
}

func (d *Driver) NewEnvelope(_ driver.Config, params ...string) (driver.EnvelopePersistence, error) {
	return newPersistenceWithOpts(sqlite.NewEnvelopePersistence, params...)
}

func (d *Driver) NewVault(_ driver.Config, params ...string) (driver2.VaultStore, error) {
	return newPersistenceWithOpts(sqlite.NewVaultPersistence, params...)
}

func newPersistenceWithOpts[V common.DBObject](constructor common.PersistenceConstructor[sqlite.Opts, V], params ...string) (V, error) {
	p, err := constructor(Op.GetOpts(params...))
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
	}

	return p, nil
}
