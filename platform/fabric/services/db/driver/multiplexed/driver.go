/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	vdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/memory"
	"github.com/pkg/errors"
)

func NewDriver(config vdbdriver.Config, ds ...fdbdriver.NamedDriver) Driver {
	drivers := make(map[driver.PersistenceType]fdbdriver.Driver, len(ds))
	for _, d := range ds {
		drivers[d.Name] = d.Driver
	}
	return Driver{
		drivers: drivers,
		config:  common.NewConfig(config),
	}
}

type Driver struct {
	drivers map[driver.PersistenceType]fdbdriver.Driver
	config  vdbdriver.PersistenceConfig
}

func (d Driver) NewEndorseTx(name vdbdriver.PersistenceName, params ...string) (fdbdriver.EndorseTxStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewEndorseTx(name, params...)
}

func (d Driver) NewMetadata(name vdbdriver.PersistenceName, params ...string) (fdbdriver.MetadataStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewMetadata(name, params...)
}

func (d Driver) NewEnvelope(name vdbdriver.PersistenceName, params ...string) (fdbdriver.EnvelopeStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewEnvelope(name, params...)
}

func (d Driver) NewVault(name vdbdriver.PersistenceName, params ...string) (driver.VaultStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewVault(name, params...)
}

func (d Driver) getDriver(name vdbdriver.PersistenceName) (fdbdriver.Driver, error) {
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
	return nil, errors.Errorf("driver %s not found [%s] in [%v] drivers", t, name, d.drivers)
}
