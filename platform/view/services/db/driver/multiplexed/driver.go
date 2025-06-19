/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/pkg/errors"
)

func NewDriver(config driver2.Config, ds ...driver2.NamedDriver) Driver {
	drivers := make(map[driver.PersistenceType]driver2.Driver, len(ds))
	for _, d := range ds {
		drivers[d.Name] = d.Driver
	}
	return Driver{
		drivers: drivers,
		config:  common.NewConfig(config),
	}
}

type Driver struct {
	drivers map[driver.PersistenceType]driver2.Driver
	config  driver2.PersistenceConfig
}

func (d Driver) NewKVS(name driver2.PersistenceName, params ...string) (driver2.KeyValueStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewKVS(name, params...)
}

func (d Driver) NewBinding(name driver2.PersistenceName, params ...string) (driver2.BindingStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewBinding(name, params...)
}

func (d Driver) NewSignerInfo(name driver2.PersistenceName, params ...string) (driver2.SignerInfoStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewSignerInfo(name, params...)
}

func (d Driver) NewAuditInfo(name driver2.PersistenceName, params ...string) (driver2.AuditInfoStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewAuditInfo(name, params...)
}

func (d Driver) getDriver(name driver2.PersistenceName) (driver2.Driver, error) {
	t, err := d.config.GetDriverType(name)
	if err != nil {
		return nil, err
	}
	if len(t) == 0 {
		t = mem.Persistence
	}
	if dr, ok := d.drivers[t]; ok {
		return dr, nil
	}
	return nil, errors.Errorf("driver %s not found [%s]", t, name)
}
