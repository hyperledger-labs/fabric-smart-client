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

func NewNamedDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   Persistence,
		Driver: NewDriver(),
	}
}

func NewDriver() *Driver {
	return &Driver{}
}

func (d *Driver) NewKVS(_ driver.PersistenceName, params ...string) (driver.KeyValueStore, error) {
	return newPersistenceWithOpts(sqlite.NewKeyValueStore, params...)
}

func (d *Driver) NewBinding(_ driver.PersistenceName, params ...string) (driver.BindingStore, error) {
	return newPersistenceWithOpts(sqlite.NewBindingStore, params...)
}

func (d *Driver) NewSignerInfo(_ driver.PersistenceName, params ...string) (driver.SignerInfoStore, error) {
	return newPersistenceWithOpts(sqlite.NewSignerInfoStore, params...)
}

func (d *Driver) NewAuditInfo(_ driver.PersistenceName, params ...string) (driver.AuditInfoStore, error) {
	return newPersistenceWithOpts(sqlite.NewAuditInfoStore, params...)
}

func (d *Driver) NewEndorseTx(_ driver.PersistenceName, params ...string) (driver.EndorseTxStore, error) {
	return newPersistenceWithOpts(sqlite.NewEndorseTxStore, params...)
}

func (d *Driver) NewMetadata(_ driver.PersistenceName, params ...string) (driver.MetadataStore, error) {
	return newPersistenceWithOpts(sqlite.NewMetadataStore, params...)
}

func (d *Driver) NewEnvelope(_ driver.PersistenceName, params ...string) (driver.EnvelopeStore, error) {
	return newPersistenceWithOpts(sqlite.NewEnvelopeStore, params...)
}

func (d *Driver) NewVault(_ driver.PersistenceName, params ...string) (driver2.VaultStore, error) {
	return newPersistenceWithOpts(sqlite.NewVaultStore, params...)
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
