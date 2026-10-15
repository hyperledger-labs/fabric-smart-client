/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"maps"
	"slices"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
)

var logger = logging.MustGetLogger()

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
	return GetDriver(d.config, d.drivers, name)
}

// GetDriver returns the driver in drivers registered for the type of the named persistence.
// A store that names no persistence while no default persistence is defined falls back to the
// memory driver, so its state does not survive a restart.
func GetDriver[D any](config driver2.PersistenceConfig, drivers map[driver.PersistenceType]D, name driver2.PersistenceName) (D, error) {
	var zero D
	t, err := config.GetDriverType(name)
	if err != nil {
		return zero, err
	}
	if len(t) == 0 {
		logger.Warnf("no persistence configured and no fsc.persistences.default defined, using %s persistence: data is lost on restart", mem.Persistence)
		t = mem.Persistence
	}
	if dr, ok := drivers[t]; ok {
		return dr, nil
	}
	return zero, errors.Errorf("driver %s not found [%s], registered drivers %v", t, name, slices.Sorted(maps.Keys(drivers)))
}
