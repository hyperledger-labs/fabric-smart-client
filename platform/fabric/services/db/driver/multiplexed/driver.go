/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package multiplexed

import (
	"sync/atomic"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/db/driver"
	vdbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/common"
	vmultiplexed "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/multiplexed"
)

func NewDriver(config vdbdriver.Config, ds ...fdbdriver.NamedDriver) Driver {
	drivers := make(map[driver.PersistenceType]fdbdriver.Driver, len(ds))
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
	drivers  map[driver.PersistenceType]fdbdriver.Driver
	config   vdbdriver.PersistenceConfig
	fellBack *atomic.Bool
}

// FellBackToMemory reports whether a store created by d uses the memory driver because it
// names no persistence and no default persistence is defined. `node migrate` fails if so.
func (d Driver) FellBackToMemory() bool { return d.fellBack != nil && d.fellBack.Load() }

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
	return vmultiplexed.GetDriverTracked(d.config, d.drivers, name, d.fellBack)
}
