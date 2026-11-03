/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"maps"
	"slices"
	"sync/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	mem "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/memory"
)

var logger = logging.MustGetLogger()

func NewDriver(config dbdriver.Config, ds ...dbdriver.NamedDriver) Driver {
	drivers := make(map[driver.PersistenceType]dbdriver.Driver, len(ds))
	for _, d := range ds {
		drivers[d.Name] = d.Driver
	}
	return Driver{
		drivers:  drivers,
		config:   common.NewConfig(config),
		fellBack: &atomic.Bool{},
	}
}

type Driver struct {
	drivers  map[driver.PersistenceType]dbdriver.Driver
	config   dbdriver.PersistenceConfig
	fellBack *atomic.Bool
}

// FellBackToMemory reports whether a store created by d uses the memory driver because it
// names no persistence and no default persistence is defined. `node migrate` fails if so.
func (d Driver) FellBackToMemory() bool { return d.fellBack != nil && d.fellBack.Load() }

func (d Driver) NewKVS(name dbdriver.PersistenceName, params ...string) (dbdriver.KeyValueStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewKVS(name, params...)
}

func (d Driver) NewBinding(name dbdriver.PersistenceName, params ...string) (dbdriver.BindingStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewBinding(name, params...)
}

func (d Driver) NewSignerInfo(name dbdriver.PersistenceName, params ...string) (dbdriver.SignerInfoStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewSignerInfo(name, params...)
}

func (d Driver) NewAuditInfo(name dbdriver.PersistenceName, params ...string) (dbdriver.AuditInfoStore, error) {
	dr, err := d.getDriver(name)
	if err != nil {
		return nil, err
	}
	return dr.NewAuditInfo(name, params...)
}

func (d Driver) getDriver(name dbdriver.PersistenceName) (dbdriver.Driver, error) {
	return GetDriverTracked(d.config, d.drivers, name, d.fellBack)
}

// GetDriver returns the driver in drivers registered for the type of the named persistence.
// A store that names no persistence while no default persistence is defined falls back to the
// memory driver, so its state does not survive a restart.
func GetDriver[D any](config dbdriver.PersistenceConfig, drivers map[driver.PersistenceType]D, name dbdriver.PersistenceName) (D, error) {
	return GetDriverTracked(config, drivers, name, nil)
}

// GetDriverTracked is GetDriver that also sets fellBack, if not nil, when it falls back to
// memory. A multiplexed driver passes its own flag, which its FellBackToMemory reads.
func GetDriverTracked[D any](config dbdriver.PersistenceConfig, drivers map[driver.PersistenceType]D, name dbdriver.PersistenceName, fellBack *atomic.Bool) (D, error) {
	var zero D
	t, err := config.GetDriverType(name)
	if err != nil {
		return zero, err
	}
	if len(t) == 0 {
		logger.Warnf("no persistence configured and no fsc.persistences.default defined, using %s persistence: data is lost on restart", mem.Persistence)
		if fellBack != nil {
			fellBack.Store(true)
		}
		t = mem.Persistence
	}
	if dr, ok := drivers[t]; ok {
		return dr, nil
	}
	return zero, errors.Errorf("driver %s not found [%s], registered drivers %v", t, name, slices.Sorted(maps.Keys(drivers)))
}
