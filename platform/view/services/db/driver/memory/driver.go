/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package mem

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/unversioned"
)

const (
	MemoryPersistence driver2.PersistenceType = "memory"
)

func NewDriver() driver.NamedDriver {
	return driver.NamedDriver{
		Name:   MemoryPersistence,
		Driver: &Driver{},
	}
}

type Driver struct{}

// NewTransactionalVersioned returns a new TransactionalVersionedPersistence for the passed data source and config
func (d *Driver) NewTransactionalVersioned(string, driver.Config) (driver.TransactionalVersionedPersistence, error) {
	panic("not supported")
}

func (d *Driver) NewVersioned(string, driver.Config) (driver.VersionedPersistence, error) {
	return NewVersionedPersistence(), nil
}

func (d *Driver) NewUnversioned(string, driver.Config) (driver.UnversionedPersistence, error) {
	return NewUnversionedPersistence(), nil
}

func (d *Driver) NewTransactionalUnversioned(dataSourceName string, config driver.Config) (driver.TransactionalUnversionedPersistence, error) {
	backend, err := d.NewTransactionalVersioned(dataSourceName, config)
	if err != nil {
		return nil, err
	}
	return &unversioned.Transactional{TransactionalVersioned: backend}, nil
}
