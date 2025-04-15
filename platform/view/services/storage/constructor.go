/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package storage

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/db"
)

func NewConstructor(cp driver.Config, dbDrivers ...driver.NamedDriver) *Constructor {
	return &Constructor{
		dbDrivers: dbDrivers,
		cp:        db.NewConfigProvider(cp),
	}
}

type Constructor struct {
	dbDrivers driver.NamedDrivers
	cp        driver.ConfigProvider
}

func (c *Constructor) NewBinding(params ...string) (driver.BindingPersistence, error) {
	opts, err := c.cp.GetConfig("fsc.binding.persistence", "binding", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewBinding(opts.TableName(), opts.DbOpts())
}

func (c *Constructor) NewAuditInfo(params ...string) (driver.AuditInfoPersistence, error) {
	opts, err := c.cp.GetConfig("fsc.auditinfo.persistence", "aud", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewAuditInfo(opts.TableName(), opts.DbOpts())
}

func (c *Constructor) NewEndorseTx(params ...string) (driver.EndorseTxPersistence, error) {
	opts, err := c.cp.GetConfig("fsc.endorsetx.persistence", "etx", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewEndorseTx(opts.TableName(), opts.DbOpts())
}

func (c *Constructor) NewMetadata(params ...string) (driver.MetadataPersistence, error) {
	opts, err := c.cp.GetConfig("fsc.metadata.persistence", "meta", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewMetadata(opts.TableName(), opts.DbOpts())
}

func (c *Constructor) NewSignerInfo(params ...string) (driver.SignerInfoPersistence, error) {
	opts, err := c.cp.GetConfig("fsc.signerinfo.persistence", "sign", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewSignerInfo(opts.TableName(), opts.DbOpts())
}

func (c *Constructor) NewEnvelope(params ...string) (driver.EnvelopePersistence, error) {
	opts, err := c.cp.GetConfig("fsc.envelope.persistence", "env", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewEnvelope(opts.TableName(), opts.DbOpts())
}

func (c *Constructor) NewKVS(params ...string) (driver.UnversionedPersistence, error) {
	opts, err := c.cp.GetConfig("fsc.kvs.persistence", "kvs", params...)
	if err != nil {
		return nil, err
	}
	d, err := c.dbDrivers.GetDriver(opts.DriverType())
	if err != nil {
		return nil, err
	}
	return d.NewKVS(opts.TableName(), opts.DbOpts())
}
